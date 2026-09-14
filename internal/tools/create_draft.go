package tools

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// defaultDraftsFolder is the folder drafts are stored in when no folder is
// specified. Gmail uses "[Gmail]/Drafts" instead.
const defaultDraftsFolder = "Drafts"

// CreateDraftTool stores a new draft in the email account's drafts folder.
type CreateDraftTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewCreateDraftTool creates a new create draft tool
func NewCreateDraftTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *CreateDraftTool {
	return &CreateDraftTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *CreateDraftTool) Name() string {
	return "create_draft"
}

// Description returns the tool description
func (t *CreateDraftTool) Description() string {
	return "Create a draft email in the email client's Drafts folder (not sent)"
}

// InputSchema returns the JSON schema for tool inputs
func (t *CreateDraftTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Account to create the draft in",
			},
			"folder": map[string]interface{}{
				"type":        "string",
				"description": "Drafts folder name (default: 'Drafts'; Gmail uses '[Gmail]/Drafts')",
			},
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Recipient email address(es) (comma-separated)",
			},
			"cc": map[string]interface{}{
				"type":        "string",
				"description": "Optional: CC recipients (comma-separated)",
			},
			"bcc": map[string]interface{}{
				"type":        "string",
				"description": "Optional: BCC recipients (comma-separated)",
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Email subject",
			},
			"body_text": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Plain text body",
			},
			"body_html": map[string]interface{}{
				"type":        "string",
				"description": "Optional: HTML body",
			},
			"reply_to": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Reply-To header",
			},
			"in_reply_to": map[string]interface{}{
				"type":        "string",
				"description": "Optional: In-Reply-To header (for replies)",
			},
		},
		"required": []string{"account_name"},
	}
}

// Execute executes the tool
func (t *CreateDraftTool) Execute(params map[string]interface{}) (interface{}, error) {
	accountName, ok := params["account_name"].(string)
	if !ok || accountName == "" {
		return nil, fmt.Errorf("account_name is required")
	}

	folder := defaultDraftsFolder
	if f, ok := params["folder"].(string); ok && f != "" {
		folder = f
	}

	msg := &email.EmailMessage{}

	// Parse to (optional)
	if toStr, ok := params["to"].(string); ok && toStr != "" {
		to := strings.Split(toStr, ",")
		for i := range to {
			to[i] = strings.TrimSpace(to[i])
		}
		msg.To = to
	}

	// Parse cc (optional)
	if ccStr, ok := params["cc"].(string); ok && ccStr != "" {
		cc := strings.Split(ccStr, ",")
		for i := range cc {
			cc[i] = strings.TrimSpace(cc[i])
		}
		msg.Cc = cc
	}

	// Parse bcc (optional)
	if bccStr, ok := params["bcc"].(string); ok && bccStr != "" {
		bcc := strings.Split(bccStr, ",")
		for i := range bcc {
			bcc[i] = strings.TrimSpace(bcc[i])
		}
		msg.Bcc = bcc
	}

	// Parse subject (optional)
	if subject, ok := params["subject"].(string); ok {
		msg.Subject = subject
	}

	// Parse body_text (optional)
	if bodyText, ok := params["body_text"].(string); ok {
		msg.BodyText = bodyText
	}

	// Parse body_html (optional)
	if bodyHTML, ok := params["body_html"].(string); ok {
		msg.BodyHTML = bodyHTML
	}

	// Require at least a subject or a body so a blank draft isn't created
	if msg.Subject == "" && msg.BodyText == "" && msg.BodyHTML == "" {
		return nil, fmt.Errorf("at least one of subject, body_text, or body_html is required")
	}

	// Parse reply_to (optional)
	if replyTo, ok := params["reply_to"].(string); ok {
		msg.ReplyTo = replyTo
	}

	// Parse in_reply_to (optional)
	if inReplyTo, ok := params["in_reply_to"].(string); ok {
		msg.InReplyTo = inReplyTo
	}

	if err := t.emailManager.CreateDraft(accountName, folder, msg); err != nil {
		return nil, fmt.Errorf("failed to create draft: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"message": "Draft created in " + folder,
		"folder":  folder,
	}, nil
}
