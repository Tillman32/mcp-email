package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/brandon/mcp-email/internal/cache"
	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/internal/email"
)

// confirmationSignals are keywords that suggest an unsubscribe action succeeded.
var confirmationSignals = []string{
	"unsubscribed", "successfully unsubscribed", "opted out", "opt-out successful",
	"you have been removed", "removed from", "no longer receive",
	"successfully removed", "email preferences updated", "preference updated",
}

// ExecuteUnsubscribeTool performs an unsubscribe action via HTTP POST, HTTP GET, or mailto.
type ExecuteUnsubscribeTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
	httpClient   *http.Client
}

func NewExecuteUnsubscribeTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *ExecuteUnsubscribeTool {
	return &ExecuteUnsubscribeTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (t *ExecuteUnsubscribeTool) Name() string { return "execute_unsubscribe" }

func (t *ExecuteUnsubscribeTool) Description() string {
	return "Execute an unsubscribe action using a URL found by find_unsubscribe_link. Supports RFC 8058 one-click POST, HTTP GET, and mailto surfacing. dry_run defaults to true — no network requests are made until dry_run is explicitly set to false."
}

func (t *ExecuteUnsubscribeTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The unsubscribe URL to act on (https:// or mailto:)",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"one_click_post", "http_get", "mailto"},
				"description": "How to execute the unsubscribe. one_click_post: RFC 8058 POST with List-Unsubscribe=One-Click body. http_get: follow the URL and detect a confirmation page. mailto: surface the address for manual action (never auto-sends).",
			},
			"email_id": map[string]interface{}{
				"type":        "integer",
				"description": "Optional email ID for logging context",
			},
			"dry_run": map[string]interface{}{
				"type":        "boolean",
				"description": "When true (default), describe the action without performing it. Set to false to execute.",
				"default":     true,
			},
		},
		"required": []string{"url", "method"},
	}
}

// executeResult is the structured response returned to the LLM.
type executeResult struct {
	Success    bool   `json:"success"`
	DryRun     bool   `json:"dry_run"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message"`
}

func (t *ExecuteUnsubscribeTool) Execute(params map[string]interface{}) (interface{}, error) {
	rawURL, ok := params["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("url is required")
	}
	rawURL = strings.TrimSpace(rawURL)

	method, ok := params["method"].(string)
	if !ok || method == "" {
		return nil, fmt.Errorf("method is required")
	}

	// dry_run defaults to true — must be explicitly false to execute.
	dryRun := true
	if v, ok := params["dry_run"].(bool); ok {
		dryRun = v
	}

	// Log optional email_id context.
	logFields := logrus.Fields{"method": method, "dry_run": dryRun}
	if emailID, err := parseEmailID(params); err == nil {
		logFields["email_id"] = emailID
	}
	t.logger.WithFields(logFields).Info("execute_unsubscribe called")

	switch method {
	case "one_click_post":
		return t.doOneClickPost(rawURL, dryRun)
	case "http_get":
		return t.doHTTPGet(rawURL, dryRun)
	case "mailto":
		return t.doMailto(rawURL, dryRun)
	default:
		return nil, fmt.Errorf("unknown method %q — use one_click_post, http_get, or mailto", method)
	}
}

// doOneClickPost performs an RFC 8058 one-click unsubscribe via HTTP POST.
func (t *ExecuteUnsubscribeTool) doOneClickPost(rawURL string, dryRun bool) (*executeResult, error) {
	if err := requireHTTPS(rawURL); err != nil {
		return nil, err
	}

	res := &executeResult{
		Method: "one_click_post",
		URL:    rawURL,
		DryRun: dryRun,
	}

	if dryRun {
		res.Success = true
		res.Message = fmt.Sprintf("DRY RUN: would POST to %s with body List-Unsubscribe=One-Click (RFC 8058)", rawURL)
		return res, nil
	}

	t.logger.WithField("url", rawURL).Info("Executing one-click POST unsubscribe")

	body := strings.NewReader("List-Unsubscribe=One-Click")
	req, err := http.NewRequest(http.MethodPost, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to build POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "mcp-email/1.0 (unsubscribe)")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("POST request failed: %s", err)
		return res, nil
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		res.Success = true
		res.Message = fmt.Sprintf("One-click POST succeeded (HTTP %d)", resp.StatusCode)
	} else {
		res.Success = false
		res.Message = fmt.Sprintf("One-click POST returned HTTP %d — may not have succeeded", resp.StatusCode)
	}

	t.logger.WithFields(logrus.Fields{"url": rawURL, "status": resp.StatusCode, "success": res.Success}).Info("One-click POST complete")
	return res, nil
}

// doHTTPGet follows an unsubscribe URL and checks the response for confirmation signals.
func (t *ExecuteUnsubscribeTool) doHTTPGet(rawURL string, dryRun bool) (*executeResult, error) {
	if err := requireHTTPS(rawURL); err != nil {
		return nil, err
	}

	res := &executeResult{
		Method: "http_get",
		URL:    rawURL,
		DryRun: dryRun,
	}

	if dryRun {
		res.Success = true
		res.Message = fmt.Sprintf("DRY RUN: would GET %s and check response for confirmation signals", rawURL)
		return res, nil
	}

	t.logger.WithField("url", rawURL).Info("Executing HTTP GET unsubscribe")

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build GET request: %w", err)
	}
	req.Header.Set("User-Agent", "mcp-email/1.0 (unsubscribe)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		res.Success = false
		res.Message = fmt.Sprintf("GET request failed: %s", err)
		return res, nil
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode

	// Read up to 64 KB to scan for confirmation signals.
	limitedBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		t.logger.WithError(err).Warn("Failed to read GET response body")
	}

	bodyLower := strings.ToLower(string(limitedBody))
	confirmed := containsAny(bodyLower, confirmationSignals)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 && confirmed {
		res.Success = true
		res.Message = fmt.Sprintf("GET succeeded (HTTP %d) and confirmation signal detected in response", resp.StatusCode)
	} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		res.Success = true
		res.Message = fmt.Sprintf("GET returned HTTP %d but no confirmation signal found — page may require a form interaction (consider Playwright MCP)", resp.StatusCode)
	} else {
		res.Success = false
		res.Message = fmt.Sprintf("GET returned HTTP %d", resp.StatusCode)
	}

	t.logger.WithFields(logrus.Fields{
		"url":       rawURL,
		"status":    resp.StatusCode,
		"confirmed": confirmed,
		"success":   res.Success,
	}).Info("HTTP GET unsubscribe complete")

	return res, nil
}

// doMailto surfaces a mailto: unsubscribe address to the user without sending anything.
func (t *ExecuteUnsubscribeTool) doMailto(rawURL string, dryRun bool) (*executeResult, error) {
	if !strings.HasPrefix(strings.ToLower(rawURL), "mailto:") {
		return nil, fmt.Errorf("expected a mailto: URL for method=mailto, got: %s", rawURL)
	}

	// Parse the recipient address.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mailto URL: %w", err)
	}
	to := parsed.Opaque
	if to == "" {
		to = rawURL
	}

	subject := parsed.Query().Get("subject")
	body := parsed.Query().Get("body")

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("mailto unsubscribe — send an email to: %s", to))
	if subject != "" {
		msg.WriteString(fmt.Sprintf(" | Subject: %s", subject))
	}
	if body != "" {
		msg.WriteString(fmt.Sprintf(" | Body: %s", body))
	}
	msg.WriteString(" — use send_email or your mail client to complete this unsubscribe")

	t.logger.WithField("mailto", to).Info("Surfacing mailto unsubscribe to user")

	return &executeResult{
		Success: true,
		DryRun:  dryRun, // mailto never auto-sends regardless of dry_run
		Method:  "mailto",
		URL:     rawURL,
		Message: msg.String(),
	}, nil
}

// requireHTTPS validates that the URL uses the https scheme.
func requireHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("only https URLs are supported for HTTP methods (got scheme %q)", u.Scheme)
	}
	return nil
}
