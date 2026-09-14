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

// GetDraftTool retrieves the full contents of a single draft by UID.
type GetDraftTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewGetDraftTool creates a new get draft tool
func NewGetDraftTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *GetDraftTool {
	return &GetDraftTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *GetDraftTool) Name() string {
	return "get_draft"
}

// Description returns the tool description
func (t *GetDraftTool) Description() string {
	return "Retrieve the full contents of a draft by UID (from list_drafts)"
}

// InputSchema returns the JSON schema for tool inputs
func (t *GetDraftTool) InputSchema() map[string]interface{} {
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
func (t *GetDraftTool) Execute(params map[string]interface{}) (interface{}, error) {
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

	draft, err := t.emailManager.GetDraft(accountName, folder, uid)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	return map[string]interface{}{
		"uid":          draft.UID,
		"subject":      draft.Subject,
		"recipients":   draft.Recipients,
		"sender_name":  draft.SenderName,
		"sender_email": draft.SenderEmail,
		"date":         draft.Date.Format(time.RFC3339),
		"body_text":    draft.BodyText,
		"body_html":    draft.BodyHTML,
		"attachments":  draft.Attachments,
		"folder_path":  draft.FolderPath,
	}, nil
}
