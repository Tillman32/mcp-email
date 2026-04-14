package tools

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/brandon/mcp-email/internal/cache"
	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/internal/email"
)

// GetSenderStatsTool returns aggregate stats for a sender from the local cache.
type GetSenderStatsTool struct {
	config       *config.Config
	emailManager *email.Manager
	cacheStore   *cache.Store
	logger       *logrus.Logger
}

func NewGetSenderStatsTool(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) *GetSenderStatsTool {
	return &GetSenderStatsTool{
		config:       cfg,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		logger:       logger,
	}
}

func (t *GetSenderStatsTool) Name() string { return "get_sender_stats" }

func (t *GetSenderStatsTool) Description() string {
	return "Return aggregate statistics for a sender from the local email cache: total email count, date range, and folders seen in. Accepts either a sender_email address or an email_id to resolve the sender automatically."
}

func (t *GetSenderStatsTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sender_email": map[string]interface{}{
				"type":        "string",
				"description": "Sender email address to look up",
			},
			"email_id": map[string]interface{}{
				"type":        "integer",
				"description": "Email ID — the sender is resolved from this email's cached record",
			},
		},
	}
}

func (t *GetSenderStatsTool) Execute(params map[string]interface{}) (interface{}, error) {
	senderEmail, err := t.resolveSender(params)
	if err != nil {
		return nil, err
	}

	stats, err := t.cacheStore.GetSenderStats(senderEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender stats: %w", err)
	}
	return stats, nil
}

// resolveSender returns the sender email from params, falling back to looking
// it up from the cache via email_id.
func (t *GetSenderStatsTool) resolveSender(params map[string]interface{}) (string, error) {
	if s, ok := params["sender_email"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s), nil
	}

	emailID, err := parseEmailID(params)
	if err != nil {
		return "", fmt.Errorf("provide sender_email or a valid email_id")
	}

	cached, err := t.cacheStore.GetEmail(emailID)
	if err != nil {
		return "", fmt.Errorf("email not found: %w", err)
	}
	if cached.SenderEmail == "" {
		return "", fmt.Errorf("email %d has no sender address", emailID)
	}
	return cached.SenderEmail, nil
}
