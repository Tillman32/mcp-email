package types

import "time"

// EmailAttachment describes a file attached to an email. Only metadata is
// surfaced (never content); use create/send tools to add files.
type EmailAttachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int    `json:"size"`
}

// Email represents an email message
type Email struct {
	ID          int64             `json:"id"`
	AccountID   int               `json:"account_id"`
	AccountName string            `json:"account_name"`
	FolderID    int               `json:"folder_id"`
	FolderPath  string            `json:"folder_path"`
	UID         uint32            `json:"uid"`
	MessageID   string            `json:"message_id"`
	Subject     string            `json:"subject"`
	SenderName  string            `json:"sender_name"`
	SenderEmail string            `json:"sender_email"`
	Recipients  []string          `json:"recipients"`
	Date        time.Time         `json:"date"`
	BodyText    string            `json:"body_text,omitempty"`
	BodyHTML    string            `json:"body_html,omitempty"`
	Attachments []EmailAttachment `json:"attachments,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Flags       []string          `json:"flags,omitempty"`
	CachedAt    time.Time         `json:"cached_at"`
}

// EmailSummary represents a summary of an email (for search results)
type EmailSummary struct {
	ID          int64     `json:"id"`
	AccountName string    `json:"account_name"`
	FolderPath  string    `json:"folder_path"`
	Subject     string    `json:"subject"`
	SenderName  string    `json:"sender_name"`
	SenderEmail string    `json:"sender_email"`
	Date        time.Time `json:"date"`
	Snippet     string    `json:"snippet"`
}

// Folder represents an email folder/mailbox
type Folder struct {
	ID           int        `json:"id"`
	AccountID    int        `json:"account_id"`
	AccountName  string     `json:"account_name"`
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	MessageCount int        `json:"message_count"`
	LastSynced   *time.Time `json:"last_synced,omitempty"`
}
