package mcp

import (
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/brandon/mcp-email/internal/cache"
	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/internal/email"
)

// newTestServer builds a full MCP server backed by an in-memory SQLite store.
// IMAP/SMTP clients are lazy-connect so no live credentials are needed.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)

	cfg := &config.Config{
		CachePath:         filepath.Join(t.TempDir(), "test.db"),
		SearchResultLimit: 100,
		LogLevel:          "error",
		Accounts: []config.AccountConfig{
			{
				Name:         "test",
				IMAPHost:     "imap.test.invalid",
				IMAPPort:     993,
				IMAPUsername: "test@test.invalid",
				IMAPPassword: "password",
				SMTPHost:     "smtp.test.invalid",
				SMTPPort:     587,
				SMTPUsername: "test@test.invalid",
				SMTPPassword: "password",
			},
		},
	}

	c, err := cache.NewCache(cfg.CachePath, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	cacheStore := cache.NewStore(c, log)

	emailManager, err := email.NewManager(cfg, cacheStore, log)
	require.NoError(t, err)

	srv, err := NewServer(cfg, emailManager, cacheStore, log)
	require.NoError(t, err)
	return srv
}

// jsonRoundTrip marshals resp and unmarshals it into a generic map so that
// nested type assertions work reliably across all Go types stored as interface{}.
func jsonRoundTrip(t *testing.T, v interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

// ---------------------------------------------------------------------------
// initialize
// ---------------------------------------------------------------------------

func TestHandleRequest_Initialize(t *testing.T) {
	srv := newTestServer(t)
	raw := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	})
	require.NotNil(t, raw)

	resp := jsonRoundTrip(t, raw)
	result := resp["result"].(map[string]interface{})
	assert.Equal(t, "2024-11-05", result["protocolVersion"])
	serverInfo := result["serverInfo"].(map[string]interface{})
	assert.Equal(t, "mcp-email", serverInfo["name"])
}

// ---------------------------------------------------------------------------
// notifications/initialized  — must return nil (no response sent to client)
// ---------------------------------------------------------------------------

func TestHandleRequest_Notification_ReturnsNil(t *testing.T) {
	srv := newTestServer(t)
	resp := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	assert.Nil(t, resp, "notification should produce no response")
}

// ---------------------------------------------------------------------------
// tools/list — unsubscribe tools are registered
// ---------------------------------------------------------------------------

func TestHandleRequest_ToolsList_IncludesUnsubscribeTools(t *testing.T) {
	srv := newTestServer(t)
	raw := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	require.NotNil(t, raw)

	resp := jsonRoundTrip(t, raw)
	result := resp["result"].(map[string]interface{})
	toolsRaw := result["tools"].([]interface{})

	names := make([]string, 0, len(toolsRaw))
	for _, item := range toolsRaw {
		if m, ok := item.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}

	assert.Contains(t, names, "find_unsubscribe_link")
	assert.Contains(t, names, "execute_unsubscribe")
	assert.Contains(t, names, "get_sender_stats")
}

// ---------------------------------------------------------------------------
// tools/call — execute_unsubscribe dry_run (safe, no network)
// ---------------------------------------------------------------------------

func TestHandleRequest_ToolsCall_ExecuteUnsubscribeDryRun(t *testing.T) {
	srv := newTestServer(t)
	raw := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "execute_unsubscribe",
			"arguments": map[string]interface{}{
				"url":    "https://example.com/unsub",
				"method": "http_get",
				// dry_run omitted → defaults to true (no network)
			},
		},
	})
	require.NotNil(t, raw)

	resp := jsonRoundTrip(t, raw)
	result, hasResult := resp["result"]
	require.True(t, hasResult, "expected result, got error: %v", resp["error"])

	resultMap := result.(map[string]interface{})
	content := resultMap["content"].([]interface{})
	require.Len(t, content, 1)

	text := content[0].(map[string]interface{})["text"].(string)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &payload))

	assert.Equal(t, true, payload["dry_run"])
	assert.Equal(t, true, payload["success"])
}

// ---------------------------------------------------------------------------
// tools/call — find_unsubscribe_link with missing email_id returns MCP error
// ---------------------------------------------------------------------------

func TestHandleRequest_ToolsCall_FindUnsubscribeLinkMissingEmailID(t *testing.T) {
	srv := newTestServer(t)
	raw := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "find_unsubscribe_link",
			"arguments": map[string]interface{}{}, // email_id required but absent
		},
	})
	require.NotNil(t, raw)

	resp := jsonRoundTrip(t, raw)
	_, hasError := resp["error"]
	assert.True(t, hasError, "missing email_id should produce an error response")
}

// ---------------------------------------------------------------------------
// tools/call — unknown tool returns -32601
// ---------------------------------------------------------------------------

func TestHandleRequest_ToolsCall_UnknownTool(t *testing.T) {
	srv := newTestServer(t)
	raw := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nonexistent_tool",
		},
	})
	require.NotNil(t, raw)

	resp := jsonRoundTrip(t, raw)
	errField, ok := resp["error"].(map[string]interface{})
	require.True(t, ok, "expected error field")
	assert.Equal(t, float64(-32601), errField["code"])
}

// ---------------------------------------------------------------------------
// Unknown method returns -32601
// ---------------------------------------------------------------------------

func TestHandleRequest_UnknownMethod(t *testing.T) {
	srv := newTestServer(t)
	raw := srv.handleRequest(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      6,
		"method":  "unknown/method",
	})
	require.NotNil(t, raw)

	resp := jsonRoundTrip(t, raw)
	errField, ok := resp["error"].(map[string]interface{})
	require.True(t, ok, "unknown method should produce error response")
	assert.Equal(t, float64(-32601), errField["code"])
}
