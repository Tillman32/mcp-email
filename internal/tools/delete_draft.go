package tools

import (
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// DeleteDraftTool permanently deletes a draft by UID.
type DeleteDraftTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewDeleteDraftTool creates a new delete draft tool
func NewDeleteDraftTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *DeleteDraftTool {
	return &DeleteDraftTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *DeleteDraftTool) Name() string {
	return "delete_draft"
}

// Description returns the tool description
func (t *DeleteDraftTool) Description() string {
	return "Permanently delete a draft by UID from the email client's Drafts folder"
}

// InputSchema returns the JSON schema for tool inputs
func (t *DeleteDraftTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Account the draft belongs to",
			},
			"folder": map[string]interface{}{
				"type":        "string",
				"description": "Drafts folder name (default: 'Drafts'; Gmail uses '[Gmail]/Drafts')",
			},
			"uid": map[string]interface{}{
				"type":        "integer",
				"description": "Draft UID (from list_drafts)",
			},
		},
		"required": []string{"account_name", "uid"},
	}
}

// Execute executes the tool
func (t *DeleteDraftTool) Execute(params map[string]interface{}) (interface{}, error) {
	accountName, ok := params["account_name"].(string)
	if !ok || accountName == "" {
		return nil, fmt.Errorf("account_name is required")
	}

	folder := defaultDraftsFolder
	if f, ok := params["folder"].(string); ok && f != "" {
		folder = f
	}

	var uid uint32
	if u, ok := params["uid"].(float64); ok {
		uid = uint32(u)
	} else if uStr, ok := params["uid"].(string); ok {
		u, err := strconv.ParseUint(uStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid uid: %w", err)
		}
		uid = uint32(u)
	} else {
		return nil, fmt.Errorf("uid is required")
	}

	if err := t.emailManager.DeleteDraft(accountName, folder, uid); err != nil {
		return nil, fmt.Errorf("failed to delete draft: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"message": "Draft deleted from " + folder,
	}, nil
}
