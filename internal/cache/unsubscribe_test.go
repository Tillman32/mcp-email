package cache

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/pkg/types"
)

// newTestStore creates an isolated SQLite store in a temp directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	c, err := NewCache(dbPath, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return NewStore(c, log)
}

// seedEmail inserts one account, one folder, and one email row, then returns
// the email's auto-assigned row ID. The test fails immediately on any error.
func seedEmail(t *testing.T, s *Store) int64 {
	t.Helper()

	accID, err := s.UpsertAccount(&config.AccountConfig{
		Name:         "test",
		IMAPHost:     "imap.test.invalid",
		IMAPPort:     993,
		IMAPUsername: "test@test.invalid",
		IMAPPassword: "pw",
		SMTPHost:     "smtp.test.invalid",
		SMTPPort:     587,
		SMTPUsername: "test@test.invalid",
		SMTPPassword: "pw",
	})
	require.NoError(t, err)

	fID, err := s.UpsertFolder(accID, "INBOX", "INBOX", 0)
	require.NoError(t, err)

	em := &types.Email{
		AccountID:   accID,
		FolderID:    fID,
		UID:         1,
		MessageID:   "<test@msg>",
		Subject:     "Newsletter",
		SenderEmail: "news@example.com",
		Recipients:  []string{"me@example.com"},
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Headers:     map[string]string{},
		Flags:       []string{},
	}
	require.NoError(t, s.UpsertEmail(em))

	var id int64
	row := s.cache.DB().QueryRow(
		"SELECT id FROM emails WHERE uid = ? AND account_id = ?", 1, accID,
	)
	require.NoError(t, row.Scan(&id))
	return id
}

// ---------------------------------------------------------------------------
// Cache miss
// ---------------------------------------------------------------------------

func TestGetUnsubscribeLinks_Miss(t *testing.T) {
	s := newTestStore(t)
	result, err := s.GetUnsubscribeLinks(99999)
	require.NoError(t, err)
	assert.Nil(t, result, "non-existent email_id should return nil, nil")
}

// ---------------------------------------------------------------------------
// Full round-trip
// ---------------------------------------------------------------------------

func TestUpsertAndGetUnsubscribeLinks_FullRoundTrip(t *testing.T) {
	s := newTestStore(t)
	emailID := seedEmail(t, s)

	want := &UnsubscribeResult{
		ListUnsubscribe: []string{
			"mailto:unsub@example.com",
			"https://example.com/unsub",
		},
		BodyLinks: []UnsubscribeLink{
			{Text: "Unsubscribe", URL: "https://example.com/unsub", Confidence: 0.9},
		},
		OneClick: &OneClickPost{
			URL:      "https://example.com/unsub",
			PostBody: "List-Unsubscribe=One-Click",
		},
	}

	require.NoError(t, s.UpsertUnsubscribeLinks(emailID, want))

	got, err := s.GetUnsubscribeLinks(emailID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want.ListUnsubscribe, got.ListUnsubscribe)
	require.Len(t, got.BodyLinks, 1)
	assert.Equal(t, want.BodyLinks[0], got.BodyLinks[0])
	require.NotNil(t, got.OneClick)
	assert.Equal(t, want.OneClick.URL, got.OneClick.URL)
	assert.Equal(t, want.OneClick.PostBody, got.OneClick.PostBody)
}

// ---------------------------------------------------------------------------
// Second upsert overwrites first (ON CONFLICT DO UPDATE)
// ---------------------------------------------------------------------------

func TestUpsertUnsubscribeLinks_UpdateOverwrites(t *testing.T) {
	s := newTestStore(t)
	emailID := seedEmail(t, s)

	first := &UnsubscribeResult{
		ListUnsubscribe: []string{"mailto:old@example.com"},
		BodyLinks:       []UnsubscribeLink{},
	}
	require.NoError(t, s.UpsertUnsubscribeLinks(emailID, first))

	second := &UnsubscribeResult{
		ListUnsubscribe: []string{"mailto:new@example.com"},
		BodyLinks:       []UnsubscribeLink{},
	}
	require.NoError(t, s.UpsertUnsubscribeLinks(emailID, second))

	got, err := s.GetUnsubscribeLinks(emailID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"mailto:new@example.com"}, got.ListUnsubscribe)
}

// ---------------------------------------------------------------------------
// Nil OneClick survives a round-trip as nil
// ---------------------------------------------------------------------------

func TestGetUnsubscribeLinks_NilOneClickRoundTrip(t *testing.T) {
	s := newTestStore(t)
	emailID := seedEmail(t, s)

	in := &UnsubscribeResult{
		ListUnsubscribe: []string{"mailto:unsub@example.com"},
		BodyLinks:       []UnsubscribeLink{},
		OneClick:        nil,
	}
	require.NoError(t, s.UpsertUnsubscribeLinks(emailID, in))

	got, err := s.GetUnsubscribeLinks(emailID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.OneClick, "nil OneClick should survive a round-trip as nil")
}

// ---------------------------------------------------------------------------
// Empty BodyLinks list survives a round-trip
// ---------------------------------------------------------------------------

func TestGetUnsubscribeLinks_EmptyBodyLinksRoundTrip(t *testing.T) {
	s := newTestStore(t)
	emailID := seedEmail(t, s)

	in := &UnsubscribeResult{
		ListUnsubscribe: []string{"https://example.com/unsub"},
		BodyLinks:       []UnsubscribeLink{},
	}
	require.NoError(t, s.UpsertUnsubscribeLinks(emailID, in))

	got, err := s.GetUnsubscribeLinks(emailID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.BodyLinks)
}
