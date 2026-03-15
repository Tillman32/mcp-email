package tools

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// ListFoldersTool lists available email folders
type ListFoldersTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewListFoldersTool creates a new list folders tool
func NewListFoldersTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *ListFoldersTool {
	return &ListFoldersTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *ListFoldersTool) Name() string {
	return "list_folders"
}

// Description returns the tool description
func (t *ListFoldersTool) Description() string {
	return "List available mailboxes/folders for configured email accounts"
}

// InputSchema returns the JSON schema for tool inputs
func (t *ListFoldersTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Specific account name, or all accounts if omitted",
			},
		},
	}
}

// Execute executes the tool
func (t *ListFoldersTool) Execute(params map[string]interface{}) (interface{}, error) {
	var accountID *int

	if accountName, ok := params["account_name"].(string); ok && accountName != "" {
		id, err := t.cacheStore.GetAccountID(accountName)
		if err != nil {
			// Account might not be in cache yet, try to sync
			if syncErr := t.emailManager.SyncAccount(accountName, ""); syncErr != nil {
				return nil, fmt.Errorf("failed to sync account: %w", syncErr)
			}
			id, err = t.cacheStore.GetAccountID(accountName)
			if err != nil {
				return nil, fmt.Errorf("account not found: %s", accountName)
			}
		}
		accountID = &id
	}

	folders, err := t.cacheStore.ListFolders(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	// Convert to JSON-serializable format
	result := make([]map[string]interface{}, len(folders))
	for i, folder := range folders {
		result[i] = map[string]interface{}{
			"id":            folder.ID,
			"account_id":    folder.AccountID,
			"account_name":  folder.AccountName,
			"name":          folder.Name,
			"path":          folder.Path,
			"message_count": folder.MessageCount,
		}
		if folder.LastSynced != nil {
			result[i]["last_synced"] = folder.LastSynced.Format("2006-01-02T15:04:05Z")
		}
	}

	return result, nil
}
