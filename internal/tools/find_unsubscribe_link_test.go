package tools

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/brandon/mcp-email/internal/cache"
)

// newTestFindTool returns a FindUnsubscribeLinkTool wired with a discarding logger.
// Only the parsing methods are exercised; no cache or IMAP calls are made.
func newTestFindTool(t *testing.T) *FindUnsubscribeLinkTool {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	return &FindUnsubscribeLinkTool{logger: log}
}

// ---------------------------------------------------------------------------
// parseEmailID
// ---------------------------------------------------------------------------

func TestParseEmailID(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]interface{}
		want    int64
		wantErr bool
	}{
		{"float64", map[string]interface{}{"email_id": float64(42)}, 42, false},
		{"string numeric", map[string]interface{}{"email_id": "99"}, 99, false},
		{"string non-numeric", map[string]interface{}{"email_id": "abc"}, 0, true},
		{"missing key", map[string]interface{}{}, 0, true},
		{"nil value", map[string]interface{}{"email_id": nil}, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEmailID(tc.params)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// containsAny
// ---------------------------------------------------------------------------

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s    string
		subs []string
		want bool
	}{
		{"click here to unsubscribe from this list", []string{"unsubscribe"}, true},
		{"to opt out of emails visit this link", []string{"unsubscribe", "opt out"}, true},
		{"view in browser | forward to a friend", []string{"unsubscribe", "opt out"}, false},
		{"", []string{"unsubscribe"}, false},
		{"unsubscribe", []string{}, false},
	}

	for _, tc := range tests {
		got := containsAny(tc.s, tc.subs)
		assert.Equal(t, tc.want, got, "containsAny(%q, %v)", tc.s, tc.subs)
	}
}

// ---------------------------------------------------------------------------
// deduplicateBodyLinks
// ---------------------------------------------------------------------------

func TestDeduplicateBodyLinks(t *testing.T) {
	t.Run("no duplicates unchanged", func(t *testing.T) {
		result := &cache.UnsubscribeResult{
			BodyLinks: []cache.UnsubscribeLink{
				{URL: "https://a.com", Confidence: 0.9},
				{URL: "https://b.com", Confidence: 0.7},
			},
		}
		deduplicateBodyLinks(result)
		assert.Len(t, result.BodyLinks, 2)
	})

	t.Run("duplicate URL keeps highest confidence", func(t *testing.T) {
		result := &cache.UnsubscribeResult{
			BodyLinks: []cache.UnsubscribeLink{
				{URL: "https://a.com", Text: "low", Confidence: 0.7},
				{URL: "https://a.com", Text: "high", Confidence: 0.9},
			},
		}
		deduplicateBodyLinks(result)
		require.Len(t, result.BodyLinks, 1)
		assert.Equal(t, 0.9, result.BodyLinks[0].Confidence)
		assert.Equal(t, "high", result.BodyLinks[0].Text)
	})

	t.Run("higher confidence first is preserved", func(t *testing.T) {
		result := &cache.UnsubscribeResult{
			BodyLinks: []cache.UnsubscribeLink{
				{URL: "https://a.com", Text: "high", Confidence: 0.9},
				{URL: "https://a.com", Text: "low", Confidence: 0.7},
			},
		}
		deduplicateBodyLinks(result)
		require.Len(t, result.BodyLinks, 1)
		assert.Equal(t, 0.9, result.BodyLinks[0].Confidence)
	})

	t.Run("empty list remains empty", func(t *testing.T) {
		result := &cache.UnsubscribeResult{}
		deduplicateBodyLinks(result)
		assert.Empty(t, result.BodyLinks)
	})
}

// ---------------------------------------------------------------------------
// parseListUnsubscribeHeader
// ---------------------------------------------------------------------------

func TestParseListUnsubscribeHeader(t *testing.T) {
	tool := newTestFindTool(t)

	tests := []struct {
		name            string
		headers         map[string]string
		wantListEntries []string // nil → expect empty
		wantOneClickURL string
	}{
		{
			name:            "empty headers",
			headers:         map[string]string{},
			wantListEntries: nil,
		},
		{
			name:            "single mailto",
			headers:         map[string]string{"List-Unsubscribe": "<mailto:unsub@example.com>"},
			wantListEntries: []string{"mailto:unsub@example.com"},
		},
		{
			name:            "single https URL",
			headers:         map[string]string{"List-Unsubscribe": "<https://example.com/unsub>"},
			wantListEntries: []string{"https://example.com/unsub"},
		},
		{
			name: "mailto and https, comma-separated",
			headers: map[string]string{
				"List-Unsubscribe": "<mailto:unsub@example.com>, <https://example.com/unsub>",
			},
			wantListEntries: []string{"mailto:unsub@example.com", "https://example.com/unsub"},
		},
		{
			name: "RFC 8058 one-click https",
			headers: map[string]string{
				"List-Unsubscribe":      "<https://example.com/unsub>",
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			},
			wantListEntries: []string{"https://example.com/unsub"},
			wantOneClickURL: "https://example.com/unsub",
		},
		{
			name:            "header key lowercase",
			headers:         map[string]string{"list-unsubscribe": "<mailto:unsub@example.com>"},
			wantListEntries: []string{"mailto:unsub@example.com"},
		},
		{
			name: "RFC 8058 post header present but no https URL — no one-click",
			headers: map[string]string{
				"List-Unsubscribe":      "<mailto:unsub@example.com>",
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			},
			wantListEntries: []string{"mailto:unsub@example.com"},
			wantOneClickURL: "", // mailto only → no one-click
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &cache.UnsubscribeResult{ListUnsubscribe: []string{}}
			tool.parseListUnsubscribeHeader(tc.headers, result)

			if tc.wantListEntries == nil {
				assert.Empty(t, result.ListUnsubscribe)
			} else {
				assert.Equal(t, tc.wantListEntries, result.ListUnsubscribe)
			}

			if tc.wantOneClickURL != "" {
				require.NotNil(t, result.OneClick)
				assert.Equal(t, tc.wantOneClickURL, result.OneClick.URL)
			} else {
				assert.Nil(t, result.OneClick)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseBodyText
// ---------------------------------------------------------------------------

func TestParseBodyText(t *testing.T) {
	tool := newTestFindTool(t)

	tests := []struct {
		name     string
		body     string
		wantURLs []string // nil → assert empty
	}{
		{
			name:     "empty body",
			body:     "",
			wantURLs: nil,
		},
		{
			name:     "URL near unsubscribe keyword",
			body:     "Click here to unsubscribe: https://example.com/unsub",
			wantURLs: []string{"https://example.com/unsub"},
		},
		{
			name:     "URL near opt out variant",
			body:     "To opt out visit https://example.com/optout from our list",
			wantURLs: []string{"https://example.com/optout"},
		},
		{
			name:     "URL far from any keyword",
			body:     "Read our blog at https://example.com/blog — enjoy your subscription!",
			wantURLs: nil,
		},
		{
			name:     "trailing period stripped from URL",
			body:     "To unsubscribe please visit https://example.com/unsub.",
			wantURLs: []string{"https://example.com/unsub"},
		},
		{
			name:     "manage preferences keyword",
			body:     "To manage preferences visit https://example.com/prefs today",
			wantURLs: []string{"https://example.com/prefs"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &cache.UnsubscribeResult{}
			tool.parseBodyText(tc.body, result)

			if tc.wantURLs == nil {
				assert.Empty(t, result.BodyLinks)
				return
			}

			require.Len(t, result.BodyLinks, len(tc.wantURLs))
			for i, link := range result.BodyLinks {
				assert.Equal(t, tc.wantURLs[i], link.URL)
				assert.Equal(t, 0.7, link.Confidence, "body text confidence should be 0.7")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseBodyHTML
// ---------------------------------------------------------------------------

func TestParseBodyHTML(t *testing.T) {
	tool := newTestFindTool(t)

	tests := []struct {
		name        string
		body        string
		wantURLs    []string // nil → assert empty
		wantMinConf float64
	}{
		{
			name:     "empty body",
			body:     "",
			wantURLs: nil,
		},
		{
			name:        "anchor with unsubscribe link text",
			body:        `<a href="https://example.com/unsub">Unsubscribe</a>`,
			wantURLs:    []string{"https://example.com/unsub"},
			wantMinConf: 0.9,
		},
		{
			name:        "anchor with opt-out link text",
			body:        `<a href="https://example.com/optout">Opt out</a>`,
			wantURLs:    []string{"https://example.com/optout"},
			wantMinConf: 0.9,
		},
		{
			name:        "parent element contains unsubscribe text",
			body:        `<p>Click <a href="https://example.com/unsub">here</a> to unsubscribe</p>`,
			wantURLs:    []string{"https://example.com/unsub"},
			wantMinConf: 0.8,
		},
		{
			name:     "fragment href skipped",
			body:     `<a href="#">Unsubscribe</a>`,
			wantURLs: nil,
		},
		{
			name:     "javascript href skipped",
			body:     `<a href="javascript:void(0)">Unsubscribe</a>`,
			wantURLs: nil,
		},
		{
			name:     "ordinary navigation link ignored",
			body:     `<a href="https://example.com/home">Home page</a>`,
			wantURLs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &cache.UnsubscribeResult{}
			tool.parseBodyHTML(tc.body, result)

			if tc.wantURLs == nil {
				assert.Empty(t, result.BodyLinks)
				return
			}

			require.Len(t, result.BodyLinks, len(tc.wantURLs))
			for i, link := range result.BodyLinks {
				assert.Equal(t, tc.wantURLs[i], link.URL)
				assert.GreaterOrEqual(t, link.Confidence, tc.wantMinConf)
			}
		})
	}
}
