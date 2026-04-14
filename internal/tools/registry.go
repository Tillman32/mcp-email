package tools

import (
	"github.com/sirupsen/logrus"

	"github.com/brandon/mcp-email/internal/cache"
	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/internal/email"
)

// Registry manages MCP tools
type Registry struct {
	config       *config.Config
	logger       *logrus.Logger
	emailManager *email.Manager
	cacheStore   *cache.Store
	tools        map[string]Tool
}

// Tool represents an MCP tool
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]interface{}
	Execute(params map[string]interface{}) (interface{}, error)
}

// NewRegistry creates a new tool registry
func NewRegistry(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) (*Registry, error) {
	reg := &Registry{
		config:       cfg,
		logger:       logger,
		emailManager: emailManager,
		cacheStore:   cacheStore,
		tools:        make(map[string]Tool),
	}

	// Register all tools
	reg.registerTools()

	return reg, nil
}

// registerTools registers all available tools
func (r *Registry) registerTools() {
	// Initialize tool implementations
	toolList := []Tool{
		NewListFoldersTool(r.config, r.emailManager, r.cacheStore, r.logger),
		NewSearchEmailsTool(r.config, r.emailManager, r.cacheStore, r.logger),
		NewGetEmailTool(r.config, r.emailManager, r.cacheStore, r.logger),
		NewSendEmailTool(r.config, r.emailManager, r.cacheStore, r.logger),
		NewFindUnsubscribeLinkTool(r.config, r.emailManager, r.cacheStore, r.logger),
		NewGetSenderStatsTool(r.config, r.emailManager, r.cacheStore, r.logger),
		NewExecuteUnsubscribeTool(r.config, r.emailManager, r.cacheStore, r.logger),
	}

	for _, tool := range toolList {
		if tool != nil {
			r.tools[tool.Name()] = tool
			r.logger.WithField("tool", tool.Name()).Debug("Registered tool")
		}
	}

	r.logger.WithField("count", len(r.tools)).Info("Registered tools")
}

// GetTool returns a tool by name
func (r *Registry) GetTool(name string) (Tool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// ListTools returns all registered tools
func (r *Registry) ListTools() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolDefinitions returns tool definitions for MCP
func (r *Registry) GetToolDefinitions() []map[string]interface{} {
	definitions := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"inputSchema": tool.InputSchema(),
		})
	}
	return definitions
}
