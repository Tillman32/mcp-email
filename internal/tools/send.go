package tools

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
)

// SendEmailTool sends a new email
type SendEmailTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

// NewSendEmailTool creates a new send email tool
func NewSendEmailTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *SendEmailTool {
	return &SendEmailTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

// Name returns the tool name
func (t *SendEmailTool) Name() string {
	return "send_email"
}

// Description returns the tool description
func (t *SendEmailTool) Description() string {
	return "Send a new email with support for text, HTML, attachments, CC, BCC"
}

// InputSchema returns the JSON schema for tool inputs
func (t *SendEmailTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"account_name": map[string]interface{}{
				"type":        "string",
				"description": "Account to send from",
			},
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Recipient email address(es) (comma-separated)",
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
				"description": "Email subject",
			},
			"body_text": map[string]interface{}{
				"type":        "string",
				"description": "Optional: Plain text body",
			},
			"body_html": map[string]interface{}{
				"type":        "string",
				"description": "Optional: HTML body",
			},
			"attachments": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional: Array of attachment paths/URLs",
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
		"required": []string{"account_name", "to", "subject"},
	}
}

// Execute executes the tool
func (t *SendEmailTool) Execute(params map[string]interface{}) (interface{}, error) {
	// Parse account_name (required)
	accountName, ok := params["account_name"].(string)
	if !ok || accountName == "" {
		return nil, fmt.Errorf("account_name is required")
	}

	// Parse to (required)
	toStr, ok := params["to"].(string)
	if !ok || toStr == "" {
		return nil, fmt.Errorf("to is required")
	}
	to := strings.Split(toStr, ",")
	for i := range to {
		to[i] = strings.TrimSpace(to[i])
	}

	// Parse subject (required)
	subject, ok := params["subject"].(string)
	if !ok || subject == "" {
		return nil, fmt.Errorf("subject is required")
	}

	// Create email message
	msg := &email.EmailMessage{
		To:      to,
		Subject: subject,
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

	// Parse body_text (optional)
	if bodyText, ok := params["body_text"].(string); ok {
		msg.BodyText = bodyText
	}

	// Parse body_html (optional)
	if bodyHTML, ok := params["body_html"].(string); ok {
		msg.BodyHTML = bodyHTML
	}

	// Ensure at least one body is set
	if msg.BodyText == "" && msg.BodyHTML == "" {
		return nil, fmt.Errorf("either body_text or body_html is required")
	}

	// Parse reply_to (optional)
	if replyTo, ok := params["reply_to"].(string); ok {
		msg.ReplyTo = replyTo
	}

	// Parse in_reply_to (optional)
	if inReplyTo, ok := params["in_reply_to"].(string); ok {
		msg.InReplyTo = inReplyTo
	}

	// Send email
	if err := t.emailManager.SendEmail(accountName, msg); err != nil {
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"message": "Email sent successfully",
	}, nil
}
