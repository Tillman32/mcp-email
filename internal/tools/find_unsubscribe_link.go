package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"

	"github.com/brandon/mcp-email/internal/cache"
	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/internal/email"
)

// urlRe matches http(s) URLs in plain text.
var urlRe = regexp.MustCompile(`https?://[^\s"'<>]+`)

// unsubKeywords are terms that indicate an unsubscribe link when found nearby.
var unsubKeywords = []string{
	"unsubscribe", "opt out", "opt-out", "remove me", "manage preferences",
	"email preferences", "manage subscriptions", "stop receiving",
}

// FindUnsubscribeLinkTool finds unsubscribe links in an email.
type FindUnsubscribeLinkTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewFindUnsubscribeLinkTool creates a new FindUnsubscribeLinkTool.
func NewFindUnsubscribeLinkTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *FindUnsubscribeLinkTool {
	return &FindUnsubscribeLinkTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

func (t *FindUnsubscribeLinkTool) Name() string { return "find_unsubscribe_link" }

func (t *FindUnsubscribeLinkTool) Description() string {
	return "Find unsubscribe links for an email. Checks List-Unsubscribe headers (including RFC 8058 one-click) and scans the email body. Results are cached."
}

func (t *FindUnsubscribeLinkTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"email_id": map[string]interface{}{
				"type":        "integer",
				"description": "Email ID (from search results)",
			},
		},
		"required": []string{"email_id"},
	}
}

func (t *FindUnsubscribeLinkTool) Execute(params map[string]interface{}) (interface{}, error) {
	emailID, err := parseEmailID(params)
	if err != nil {
		return nil, err
	}

	// Return cached result if available.
	if cached, err := t.cacheStore.GetUnsubscribeLinks(emailID); err != nil {
		t.logger.WithError(err).Warn("Failed to read unsubscribe cache")
	} else if cached != nil {
		t.logger.WithField("email_id", emailID).Debug("Returning cached unsubscribe links")
		return cached, nil
	}

	// Fetch email (triggers IMAP re-fetch if body is empty).
	em, err := t.fetchEmail(emailID)
	if err != nil {
		return nil, err
	}

	result := &cache.UnsubscribeResult{
		ListUnsubscribe: []string{},
		BodyLinks:       []cache.UnsubscribeLink{},
	}

	t.parseListUnsubscribeHeader(em.Headers, result)
	t.parseBodyText(em.BodyText, result)
	t.parseBodyHTML(em.BodyHTML, result)
	deduplicateBodyLinks(result)

	// Persist to cache (best-effort).
	if err := t.cacheStore.UpsertUnsubscribeLinks(emailID, result); err != nil {
		t.logger.WithError(err).Warn("Failed to cache unsubscribe links")
	}

	return result, nil
}

// parseListUnsubscribeHeader extracts entries from the List-Unsubscribe header
// and detects RFC 8058 one-click eligibility via List-Unsubscribe-Post.
func (t *FindUnsubscribeLinkTool) parseListUnsubscribeHeader(headers map[string]string, result *cache.UnsubscribeResult) {
	// Header keys may be capitalised differently depending on the IMAP library.
	var listUnsub, listUnsubPost string
	for k, v := range headers {
		lower := strings.ToLower(k)
		if lower == "list-unsubscribe" {
			listUnsub = v
		}
		if lower == "list-unsubscribe-post" {
			listUnsubPost = v
		}
	}

	if listUnsub == "" {
		return
	}

	// The header value is a comma-separated list of angle-bracket enclosed URIs:
	// <mailto:unsub@example.com>, <https://example.com/unsub>
	for _, raw := range strings.Split(listUnsub, ",") {
		raw = strings.TrimSpace(raw)
		raw = strings.Trim(raw, "<>")
		raw = strings.TrimSpace(raw)
		if raw != "" {
			result.ListUnsubscribe = append(result.ListUnsubscribe, raw)
		}
	}

	// RFC 8058: if List-Unsubscribe-Post is present and the header contains an
	// https URL, populate the one-click field.
	if strings.Contains(strings.ToLower(listUnsubPost), "list-unsubscribe=one-click") {
		for _, entry := range result.ListUnsubscribe {
			if strings.HasPrefix(entry, "https://") {
				result.OneClick = &cache.OneClickPost{
					URL:      entry,
					PostBody: strings.TrimSpace(listUnsubPost),
				}
				break
			}
		}
	}
}

// parseBodyText scans plain-text body for URLs near unsubscribe keywords.
func (t *FindUnsubscribeLinkTool) parseBodyText(body string, result *cache.UnsubscribeResult) {
	if body == "" {
		return
	}

	lower := strings.ToLower(body)
	urls := urlRe.FindAllStringIndex(body, -1)

	for _, loc := range urls {
		url := body[loc[0]:loc[1]]
		// Clean trailing punctuation that regex may have captured.
		url = strings.TrimRight(url, ".,;:)")

		// Check a window of ±150 chars around the URL for keywords.
		start := loc[0] - 150
		if start < 0 {
			start = 0
		}
		end := loc[1] + 150
		if end > len(lower) {
			end = len(lower)
		}
		window := lower[start:end]

		if containsAny(window, unsubKeywords) {
			result.BodyLinks = append(result.BodyLinks, cache.UnsubscribeLink{
				Text:       "unsubscribe",
				URL:        url,
				Confidence: 0.7,
			})
		}
	}
}

// parseBodyHTML parses the HTML body with goquery, finding anchor tags whose
// text or surrounding context contains unsubscribe keywords.
func (t *FindUnsubscribeLinkTool) parseBodyHTML(body string, result *cache.UnsubscribeResult) {
	if body == "" {
		return
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		t.logger.WithError(err).Warn("Failed to parse HTML body")
		return
	}

	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, exists := sel.Attr("href")
		if !exists || href == "" {
			return
		}
		if strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript") {
			return
		}

		linkText := strings.ToLower(strings.TrimSpace(sel.Text()))

		// Check the link text itself.
		if containsAny(linkText, unsubKeywords) {
			result.BodyLinks = append(result.BodyLinks, cache.UnsubscribeLink{
				Text:       strings.TrimSpace(sel.Text()),
				URL:        href,
				Confidence: 0.9,
			})
			return
		}

		// Check the immediate parent element's text for context.
		parentText := strings.ToLower(strings.TrimSpace(sel.Parent().Text()))
		if containsAny(parentText, unsubKeywords) {
			result.BodyLinks = append(result.BodyLinks, cache.UnsubscribeLink{
				Text:       strings.TrimSpace(sel.Text()),
				URL:        href,
				Confidence: 0.8,
			})
		}
	})
}

// deduplicateBodyLinks removes duplicate URLs, keeping the highest-confidence entry.
func deduplicateBodyLinks(result *cache.UnsubscribeResult) {
	seen := make(map[string]int) // url → index in result.BodyLinks
	out := result.BodyLinks[:0]
	for _, link := range result.BodyLinks {
		if idx, exists := seen[link.URL]; exists {
			if link.Confidence > out[idx].Confidence {
				out[idx] = link
			}
		} else {
			seen[link.URL] = len(out)
			out = append(out, link)
		}
	}
	result.BodyLinks = out
}

// fetchEmail returns the email from cache, re-fetching the body from IMAP if empty.
func (t *FindUnsubscribeLinkTool) fetchEmail(emailID int64) (*emailData, error) {
	cached, err := t.cacheStore.GetEmail(emailID)
	if err != nil {
		return nil, fmt.Errorf("email not found: %w", err)
	}

	data := &emailData{
		BodyText: cached.BodyText,
		BodyHTML: cached.BodyHTML,
		Headers:  cached.Headers,
	}

	if data.BodyText == "" && data.BodyHTML == "" {
		t.logger.WithField("email_id", emailID).Info("Body empty, re-fetching from IMAP")
		account, err := t.config.GetAccountByName(cached.AccountName)
		if err != nil {
			t.logger.WithError(err).Warn("Could not get account for re-fetch")
			return data, nil
		}
		imapClient, err := email.NewIMAPClient(account)
		if err != nil {
			t.logger.WithError(err).Warn("Could not create IMAP client for re-fetch")
			return data, nil
		}
		imapClient.SetLogger(t.logger)

		emails, err := imapClient.FetchEmails(cached.FolderPath, cached.UID, cached.UID)
		if err != nil || len(emails) == 0 {
			t.logger.WithError(err).Warn("IMAP re-fetch failed or returned no results")
			return data, nil
		}
		data.BodyText = emails[0].BodyText
		data.BodyHTML = emails[0].BodyHTML
		data.Headers = emails[0].Headers

		cached.BodyText = data.BodyText
		cached.BodyHTML = data.BodyHTML
		cached.Headers = data.Headers
		if err := t.cacheStore.UpsertEmail(cached); err != nil {
			t.logger.WithError(err).Warn("Failed to update email cache after re-fetch")
		}
	}

	return data, nil
}

// emailData is a lightweight view of the fields we need from a cached email.
type emailData struct {
	BodyText string
	BodyHTML string
	Headers  map[string]string
}

// parseEmailID extracts and validates the email_id parameter.
func parseEmailID(params map[string]interface{}) (int64, error) {
	switch v := params["email_id"].(type) {
	case float64:
		return int64(v), nil
	case string:
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid email_id: %w", err)
		}
		return id, nil
	default:
		return 0, fmt.Errorf("email_id is required")
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
