package tools

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// ListDraftsTool lists all drafts in the email client's Drafts folder.
type ListDraftsTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewListDraftsTool creates a new list drafts tool
func NewListDraftsTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *ListDraftsTool {
	return &ListDraftsTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *ListDraftsTool) Name() string {
	return "list_drafts"
}

// Description returns the tool description
func (t *ListDraftsTool) Description() string {
	return "List drafts stored in the email client's Drafts folder"
}

// InputSchema returns the JSON schema for tool inputs
func (t *ListDraftsTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Account to list drafts from",
			},
			"folder": map[string]interface{}{
				"type":        "string",
				"description": "Drafts folder name (default: 'Drafts'; Gmail uses '[Gmail]/Drafts')",
			},
		},
		"required": []string{"account_name"},
	}
}

// Execute executes the tool
func (t *ListDraftsTool) Execute(params map[string]interface{}) (interface{}, error) {
	accountName, ok := params["account_name"].(string)
	if !ok || accountName == "" {
		return nil, fmt.Errorf("account_name is required")
	}

	folder := defaultDraftsFolder
	if f, ok := params["folder"].(string); ok && f != "" {
		folder = f
	}

	drafts, err := t.emailManager.ListDrafts(accountName, folder)
	if err != nil {
		return nil, fmt.Errorf("failed to list drafts: %w", err)
	}

	result := make([]map[string]interface{}, len(drafts))
	for i, draft := range drafts {
		snippet := draft.BodyText
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		result[i] = map[string]interface{}{
			"uid":              draft.UID,
			"subject":          draft.Subject,
			"recipients":       draft.Recipients,
			"sender_name":      draft.SenderName,
			"sender_email":     draft.SenderEmail,
			"date":             draft.Date.Format(time.RFC3339),
			"snippet":          snippet,
			"attachment_count": len(draft.Attachments),
			"attachments":      draft.Attachments,
		}
	}

	return result, nil
}
