package tools

import (
	"fmt"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// GetEmailTool retrieves a full email by ID
type GetEmailTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewGetEmailTool creates a new get email tool
func NewGetEmailTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *GetEmailTool {
	return &GetEmailTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *GetEmailTool) Name() string {
	return "get_email"
}

// Description returns the tool description
func (t *GetEmailTool) Description() string {
	return "Retrieve full email by ID from cache or IMAP"
}

// InputSchema returns the JSON schema for tool inputs
func (t *GetEmailTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"email_id": map[string]interface{}{
				"type":        "integer",
				"description": "Email ID (from search results)",
			},
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Account name if needed",
			},
		},
		"required": []string{"email_id"},
	}
}

// Execute executes the tool
func (t *GetEmailTool) Execute(params map[string]interface{}) (interface{}, error) {
	// Parse email_id
	var emailID int64
	if id, ok := params["email_id"].(float64); ok {
		emailID = int64(id)
	} else if idStr, ok := params["email_id"].(string); ok {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid email_id: %w", err)
		}
		emailID = id
	} else {
		return nil, fmt.Errorf("email_id is required")
	}

	// Get email from cache
	cachedEmail, err := t.cacheStore.GetEmail(emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get email: %w", err)
	}

	// If body is empty, try to re-fetch from IMAP
	if cachedEmail.BodyText == "" && cachedEmail.BodyHTML == "" {
		t.logger.WithField("email_id", emailID).Info("Email body is empty, re-fetching from IMAP")

		// Get account config
		account, err := t.config.GetAccountByName(cachedEmail.AccountName)
		if err != nil {
			t.logger.WithError(err).Warn("Could not get account config for re-fetch")
		} else {
			// Create IMAP client
			imapClient, err := email.NewIMAPClient(account)
			if err != nil {
				t.logger.WithError(err).Warn("Could not create IMAP client for re-fetch")
			} else {
				imapClient.SetLogger(t.logger)

				// Fetch the specific email
				emails, err := imapClient.FetchEmails(cachedEmail.FolderPath, cachedEmail.UID, cachedEmail.UID)
				if err != nil {
					t.logger.WithError(err).Warn("Could not re-fetch email from IMAP")
				} else if len(emails) > 0 {
					// Update the cached email with the new body content
					cachedEmail.BodyText = emails[0].BodyText
					cachedEmail.BodyHTML = emails[0].BodyHTML
					cachedEmail.Headers = emails[0].Headers

					// Update cache using UpsertEmail
					if err := t.cacheStore.UpsertEmail(cachedEmail); err != nil {
						t.logger.WithError(err).Warn("Could not update email in cache")
					} else {
						t.logger.WithField("email_id", emailID).Info("Successfully re-fetched and updated email")
					}
				}
			}
		}
	}

	// Convert to JSON-serializable format
	result := map[string]interface{}{
		"id":           cachedEmail.ID,
		"account_id":   cachedEmail.AccountID,
		"account_name": cachedEmail.AccountName,
		"folder_id":    cachedEmail.FolderID,
		"folder_path":  cachedEmail.FolderPath,
		"uid":          cachedEmail.UID,
		"message_id":   cachedEmail.MessageID,
		"subject":      cachedEmail.Subject,
		"sender_name":  cachedEmail.SenderName,
		"sender_email": cachedEmail.SenderEmail,
		"recipients":   cachedEmail.Recipients,
		"date":         cachedEmail.Date.Format(time.RFC3339),
		"body_text":    cachedEmail.BodyText,
		"body_html":    cachedEmail.BodyHTML,
		"headers":      cachedEmail.Headers,
		"flags":        cachedEmail.Flags,
		"cached_at":    cachedEmail.CachedAt.Format(time.RFC3339),
	}

	return result, nil
}
