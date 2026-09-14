package tools

import (
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// SendDraftTool sends an existing draft and optionally deletes it afterward.
type SendDraftTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewSendDraftTool creates a new send draft tool
func NewSendDraftTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *SendDraftTool {
	return &SendDraftTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *SendDraftTool) Name() string {
	return "send_draft"
}

// Description returns the tool description
func (t *SendDraftTool) Description() string {
	return "Send an existing draft by UID and (by default) delete it from the Drafts folder"
}

// InputSchema returns the JSON schema for tool inputs
func (t *SendDraftTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Account to send the draft from",
			},
			"folder": map[string]interface{}{
				"type":        "string",
				"description": "Drafts folder name (default: 'Drafts'; Gmail uses '[Gmail]/Drafts')",
			},
			"uid": map[string]interface{}{
				"type":        "integer",
				"description": "Draft UID (from list_drafts)",
			},
			"delete_after_send": map[string]interface{}{
				"type":        "boolean",
				"description": "Delete the draft from the Drafts folder after sending (default: true)",
			},
		},
		"required": []string{"account_name", "uid"},
	}
}

// Execute executes the tool
func (t *SendDraftTool) Execute(params map[string]interface{}) (interface{}, error) {
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

	deleteAfterSend := true
	if d, ok := params["delete_after_send"].(bool); ok {
		deleteAfterSend = d
	}

	if err := t.emailManager.SendDraft(accountName, folder, uid, deleteAfterSend); err != nil {
		return nil, fmt.Errorf("failed to send draft: %w", err)
	}

	message := "Draft sent successfully"
	if deleteAfterSend {
		message += " and removed from " + folder
	}

	return map[string]interface{}{
		"success": true,
		"message": message,
	}, nil
}
