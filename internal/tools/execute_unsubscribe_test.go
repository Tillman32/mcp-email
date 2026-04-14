package tools

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestExecTool wires an ExecuteUnsubscribeTool with a discarding logger.
// Pass a non-nil httpClient to inject a test-server client (e.g. srv.Client()).
func newTestExecTool(t *testing.T, httpClient *http.Client) *ExecuteUnsubscribeTool {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &ExecuteUnsubscribeTool{
		logger:     log,
		httpClient: httpClient,
	}
}

// ---------------------------------------------------------------------------
// requireHTTPS
// ---------------------------------------------------------------------------

func TestRequireHTTPS(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com/unsub", false},
		{"http://example.com/unsub", true},
		{"ftp://example.com/unsub", true},
		{"not-a-url", true},
	}

	for _, tc := range tests {
		err := requireHTTPS(tc.url)
		if tc.wantErr {
			assert.Error(t, err, "url=%q should be rejected", tc.url)
		} else {
			assert.NoError(t, err, "url=%q should be accepted", tc.url)
		}
	}
}

// ---------------------------------------------------------------------------
// doMailto
// ---------------------------------------------------------------------------

func TestDoMailto(t *testing.T) {
	tool := newTestExecTool(t, nil)

	tests := []struct {
		name      string
		url       string
		wantErr   bool
		wantInMsg string
	}{
		{
			name:      "simple mailto address",
			url:       "mailto:unsub@newsletter.com",
			wantInMsg: "unsub@newsletter.com",
		},
		{
			name:      "mailto with subject and body params",
			url:       "mailto:unsub@newsletter.com?subject=Unsubscribe&body=Please%20remove%20me",
			wantInMsg: "Please remove me",
		},
		{
			name:    "non-mailto URL returns error",
			url:     "https://example.com/unsub",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.doMailto(tc.url, true)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.True(t, res.Success)
			assert.Equal(t, "mailto", res.Method)
			assert.Contains(t, res.Message, tc.wantInMsg)
			// mailto never auto-sends regardless of dry_run flag
			assert.Contains(t, res.Message, "send_email")
		})
	}
}

// ---------------------------------------------------------------------------
// doOneClickPost
// ---------------------------------------------------------------------------

func TestDoOneClickPost(t *testing.T) {
	t.Run("dry_run true — no HTTP call made", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		res, err := tool.doOneClickPost("https://example.com/unsub", true)
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.True(t, res.DryRun)
		assert.Contains(t, res.Message, "DRY RUN")
		assert.Equal(t, "one_click_post", res.Method)
	})

	t.Run("dry_run false — 200 from server", func(t *testing.T) {
		var receivedMethod string
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		tool := newTestExecTool(t, srv.Client())
		res, err := tool.doOneClickPost(srv.URL, false)
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, receivedMethod)
		assert.True(t, res.Success)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.False(t, res.DryRun)
	})

	t.Run("dry_run false — 500 from server", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		tool := newTestExecTool(t, srv.Client())
		res, err := tool.doOneClickPost(srv.URL, false)
		require.NoError(t, err)
		assert.False(t, res.Success)
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	})

	t.Run("http URL rejected before any request", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		_, err := tool.doOneClickPost("http://example.com/unsub", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "https")
	})
}

// ---------------------------------------------------------------------------
// doHTTPGet
// ---------------------------------------------------------------------------

func TestDoHTTPGet(t *testing.T) {
	t.Run("dry_run true — no HTTP call made", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		res, err := tool.doHTTPGet("https://example.com/unsub", true)
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.True(t, res.DryRun)
		assert.Contains(t, res.Message, "DRY RUN")
	})

	t.Run("200 with confirmation signal in body", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "<html><body>You have been successfully unsubscribed.</body></html>")
		}))
		defer srv.Close()

		tool := newTestExecTool(t, srv.Client())
		res, err := tool.doHTTPGet(srv.URL, false)
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Contains(t, res.Message, "confirmation signal")
	})

	t.Run("200 without confirmation signal — success but warns", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "<html><body>Welcome back!</body></html>")
		}))
		defer srv.Close()

		tool := newTestExecTool(t, srv.Client())
		res, err := tool.doHTTPGet(srv.URL, false)
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Contains(t, res.Message, "no confirmation signal")
	})

	t.Run("404 response — failure", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		tool := newTestExecTool(t, srv.Client())
		res, err := tool.doHTTPGet(srv.URL, false)
		require.NoError(t, err)
		assert.False(t, res.Success)
		assert.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("http URL rejected before any request", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		_, err := tool.doHTTPGet("http://example.com/unsub", false)
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Execute — top-level dispatch and parameter validation
// ---------------------------------------------------------------------------

func TestExecuteUnsubscribeTool_Execute(t *testing.T) {
	t.Run("missing url returns error", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		_, err := tool.Execute(map[string]interface{}{"method": "http_get"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})

	t.Run("blank url returns error", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		_, err := tool.Execute(map[string]interface{}{"url": "   ", "method": "http_get"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})

	t.Run("missing method returns error", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		_, err := tool.Execute(map[string]interface{}{"url": "https://example.com"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method is required")
	})

	t.Run("unknown method returns error", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		_, err := tool.Execute(map[string]interface{}{
			"url":    "https://example.com/unsub",
			"method": "carrier_pigeon",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown method")
	})

	t.Run("dry_run defaults to true when omitted", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		res, err := tool.Execute(map[string]interface{}{
			"url":    "https://example.com/unsub",
			"method": "http_get",
			// "dry_run" intentionally omitted
		})
		require.NoError(t, err)
		result, ok := res.(*executeResult)
		require.True(t, ok)
		assert.True(t, result.DryRun, "dry_run should default to true")
		assert.Contains(t, result.Message, "DRY RUN")
	})

	t.Run("dry_run false with mailto executes safely", func(t *testing.T) {
		tool := newTestExecTool(t, nil)
		res, err := tool.Execute(map[string]interface{}{
			"url":     "mailto:unsub@example.com",
			"method":  "mailto",
			"dry_run": false,
		})
		require.NoError(t, err)
		result, ok := res.(*executeResult)
		require.True(t, ok)
		// mailto never auto-sends, but DryRun reflects the param passed
		assert.Equal(t, false, result.DryRun)
		assert.True(t, result.Success)
	})
}
