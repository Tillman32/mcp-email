package email

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhillyerd/enmime"

	"github.com/Tillman32/mcp-email/internal/config"
)

func testSMTPClient() *SMTPClient {
	return &SMTPClient{config: &config.AccountConfig{SMTPUsername: "test@example.com"}}
}

const testAttachmentName = "notes.txt"

// A message without attachments must keep the simple single-part layout.
func TestBuildMessageNoAttachments(t *testing.T) {
	raw := testSMTPClient().BuildMessage(&EmailMessage{
		To:       []string{"a@example.com"},
		Subject:  "hello",
		BodyText: "body",
	})
	if strings.Contains(string(raw), "multipart/mixed") {
		t.Fatal("plain message should not be multipart")
	}
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse built message: %v", err)
	}
	if strings.TrimSpace(env.Text) != "body" {
		t.Fatalf("body mismatch: %q", env.Text)
	}
	if len(env.Attachments) != 0 {
		t.Fatalf("expected no attachments, got %d", len(env.Attachments))
	}
}

// A message with attachments must round-trip body + files through MIME.
func TestBuildMessageWithAttachments(t *testing.T) {
	raw := testSMTPClient().BuildMessage(&EmailMessage{
		To:       []string{"a@example.com"},
		Subject:  "files",
		BodyText: "see attached",
		Attachments: []Attachment{
			{Filename: testAttachmentName, Content: []byte("hello file"), MimeType: "text/plain"},
			{Filename: "data.bin", Content: []byte{0, 1, 2, 3}, MimeType: "application/octet-stream"},
		},
	})
	if !strings.Contains(string(raw), "multipart/mixed") {
		t.Fatal("message with attachments should be multipart/mixed")
	}
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse built message: %v", err)
	}
	if strings.TrimSpace(env.Text) != "see attached" {
		t.Fatalf("body mismatch: %q", env.Text)
	}
	if len(env.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(env.Attachments))
	}
	if env.Attachments[0].FileName != testAttachmentName || string(env.Attachments[0].Content) != "hello file" {
		t.Fatalf("attachment 0 mismatch: %+v", env.Attachments[0])
	}
	if env.Attachments[1].FileName != "data.bin" {
		t.Fatalf("attachment 1 mismatch: %+v", env.Attachments[1])
	}
}

func TestLoadAttachmentsFromFiles(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, testAttachmentName)
	if err := os.WriteFile(txt, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	atts, err := LoadAttachments([]string{txt})
	if err != nil {
		t.Fatalf("LoadAttachments: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].Filename != testAttachmentName || string(atts[0].Content) != "hello" {
		t.Fatalf("attachment mismatch: %+v", atts[0])
	}
	if !strings.HasPrefix(atts[0].MimeType, "text/plain") {
		t.Fatalf("mime mismatch: %q", atts[0].MimeType)
	}
}

func TestLoadAttachmentsRejectsDirectory(t *testing.T) {
	if _, err := LoadAttachments([]string{t.TempDir()}); err == nil {
		t.Fatal("expected error for directory attachment")
	}
}

func TestLoadAttachmentsRejectsMissing(t *testing.T) {
	if _, err := LoadAttachments([]string{filepath.Join(t.TempDir(), "nope.txt")}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadAttachmentsEnforcesCount(t *testing.T) {
	srcs := make([]string, maxAttachmentCount+1)
	for i := range srcs {
		srcs[i] = "f.txt"
	}
	if _, err := LoadAttachments(srcs); err == nil {
		t.Fatal("expected error for too many attachments")
	}
}

func TestParseAttachmentSources(t *testing.T) {
	got, err := ParseAttachmentSources([]interface{}{"a.txt", "https://example.com/b.pdf"})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, err %v", got, err)
	}
	if _, err := ParseAttachmentSources([]interface{}{"ok.txt", 42}); err == nil {
		t.Fatal("expected error for non-string entry")
	}
	got, err = ParseAttachmentSources("single.txt")
	if err != nil || len(got) != 1 {
		t.Fatalf("single string: got %v, err %v", got, err)
	}
	got, err = ParseAttachmentSources(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("nil: got %v, err %v", got, err)
	}
}
