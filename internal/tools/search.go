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

// SearchEmailsTool searches cached emails
type SearchEmailsTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewSearchEmailsTool creates a new search emails tool
func NewSearchEmailsTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *SearchEmailsTool {
	return &SearchEmailsTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *SearchEmailsTool) Name() string {
	return "search_emails"
}

// Description returns the tool description
func (t *SearchEmailsTool) Description() string {
	return "Search cached emails with flexible filters (sender, recipient, subject, body, date range)"
}

// InputSchema returns the JSON schema for tool inputs
func (t *SearchEmailsTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Filter by specific account",
			},
			"folder": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Filter by folder/mailbox",
			},
			"sender": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Filter by sender email/name",
			},
			"recipient": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Filter by recipient email",
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Filter by subject (substring match)",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Filter by body content (full-text search)",
			},
			"date_from": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Start date (ISO 8601 format)",
			},
			"date_to": map[string]interface{}{
				"type":        "string",
				"description": "Optional: End date (ISO 8601 format)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Optional: Result limit (default: 100, max: 1000)",
				"minimum":     1,
				"maximum":     1000,
			},
		},
	}
}

// Execute executes the tool
func (t *SearchEmailsTool) Execute(params map[string]interface{}) (interface{}, error) {
	opts := cache.SearchOptions{}

	// Parse account_name
	if accountName, ok := params["account_name"].(string); ok && accountName != "" {
		accountID, err := t.cacheStore.GetAccountID(accountName)
		if err != nil {
			// Account might not be in cache yet, try to sync
			if syncErr := t.emailManager.SyncAccount(accountName, ""); syncErr != nil {
				return nil, fmt.Errorf("failed to sync account: %w", syncErr)
			}
			accountID, err = t.cacheStore.GetAccountID(accountName)
			if err != nil {
				return nil, fmt.Errorf("account not found: %s", accountName)
			}
		}
		opts.AccountID = &accountID

		// Check if account has cached emails, if not, sync
		hasEmails, err := t.cacheStore.HasEmails(accountID)
		if err != nil {
			t.logger.WithError(err).Warn("Failed to check if account has emails")
		} else if !hasEmails {
			t.logger.WithField("account", accountName).Info("No cached emails found, syncing account")
			if err := t.emailManager.SyncAccount(accountName, ""); err != nil {
				t.logger.WithError(err).WithField("account", accountName).Warn("Failed to sync account for search")
				// Continue with search even if sync fails
			}
		}
	} else {
		// No account specified, check if any emails are cached
		hasAnyEmails, err := t.cacheStore.HasAnyEmails()
		if err != nil {
			t.logger.WithError(err).Warn("Failed to check if any emails are cached")
		} else if !hasAnyEmails {
			// Sync all accounts
			t.logger.Info("No cached emails found, syncing all accounts")
			for _, accountName := range t.config.AccountNames() {
				if err := t.emailManager.SyncAccount(accountName, ""); err != nil {
					t.logger.WithError(err).WithField("account", accountName).Warn("Failed to sync account")
				}
			}
		}
	}

	// Parse folder (would need folder ID lookup, simplified for now)
	// TODO: Implement folder name to ID lookup

	// Parse sender
	if sender, ok := params["sender"].(string); ok && sender != "" {
		opts.Sender = &sender
	}

	// Parse recipient
	if recipient, ok := params["recipient"].(string); ok && recipient != "" {
		opts.Recipient = &recipient
	}

	// Parse subject
	if subject, ok := params["subject"].(string); ok && subject != "" {
		opts.Subject = &subject
	}

	// Parse body (full-text search)
	if body, ok := params["body"].(string); ok && body != "" {
		opts.Body = &body
	}

	// Parse date_from
	if dateFromStr, ok := params["date_from"].(string); ok && dateFromStr != "" {
		dateFrom, err := time.Parse(time.RFC3339, dateFromStr)
		if err != nil {
			return nil, fmt.Errorf("invalid date_from format: %w", err)
		}
		opts.DateFrom = &dateFrom
	}

	// Parse date_to
	if dateToStr, ok := params["date_to"].(string); ok && dateToStr != "" {
		dateTo, err := time.Parse(time.RFC3339, dateToStr)
		if err != nil {
			return nil, fmt.Errorf("invalid date_to format: %w", err)
		}
		opts.DateTo = &dateTo
	}

	// Parse limit
	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	} else if limitStr, ok := params["limit"].(string); ok {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			opts.Limit = limit
		}
	}
	if opts.Limit == 0 {
		opts.Limit = t.config.SearchResultLimit
	}

	// Perform search
	results, err := t.cacheStore.Search(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to search emails: %w", err)
	}

	// Convert to JSON-serializable format
	emailList := make([]map[string]interface{}, len(results))
	for i, email := range results {
		emailList[i] = map[string]interface{}{
			"id":           email.ID,
			"account_name": email.AccountName,
			"folder_path":  email.FolderPath,
			"subject":      email.Subject,
			"sender_name":  email.SenderName,
			"sender_email": email.SenderEmail,
			"date":         email.Date.Format(time.RFC3339),
			"snippet":      email.Snippet,
		}
	}

	return emailList, nil
}
