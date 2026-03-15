package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/internal/email"
	"github.com/Tillman32/mcp-email/internal/tools"
)

// Server represents the MCP server
type Server struct {
	config       *config.Config
	logger       *logrus.Logger
	tools        *tools.Registry
	emailManager *email.Manager
	cacheStore   *cache.Store
}

// NewServer creates a new MCP server instance
func NewServer(cfg *config.Config, emailManager *email.Manager, cacheStore *cache.Store, logger *logrus.Logger) (*Server, error) {
	// Initialize tool registry
	toolRegistry, err := tools.NewRegistry(cfg, emailManager, cacheStore, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool registry: %w", err)
	}

	return &Server{
		config:       cfg,
		logger:       logger,
		tools:        toolRegistry,
		emailManager: emailManager,
		cacheStore:   cacheStore,
	}, nil
}

// Run starts the MCP server with stdio transport
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("Starting MCP server with stdio transport")

	// Simple MCP protocol implementation via stdio
	// This is a basic implementation that handles MCP requests
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var req map[string]interface{}
			if err := decoder.Decode(&req); err != nil {
				if err == io.EOF {
					return nil
				}
				s.logger.WithError(err).Error("Failed to decode request")
				continue
			}

			resp := s.handleRequest(req)
			if resp != nil {
				if err := encoder.Encode(resp); err != nil {
					s.logger.WithError(err).Error("Failed to encode response")
					continue
				}
			}
		}
	}
}

// handleRequest processes an MCP request
func (s *Server) handleRequest(req map[string]interface{}) map[string]interface{} {
	method, ok := req["method"].(string)
	if !ok {
		method = ""
	}
	id := req["id"]

	// Handle initialize request
	if method == "initialize" {
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "mcp-email",
					"version": "1.0.0",
				},
			},
		}
	}

	// Handle notifications/initialized
	if method == "notifications/initialized" {
		return nil
	}

	// Handle tools/list request
	if method == "tools/list" {
		toolDefs := s.tools.GetToolDefinitions()
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"tools": toolDefs,
			},
		}
	}

	// Handle tools/call request
	if method == "tools/call" {
		params, ok := req["params"].(map[string]interface{})
		if !ok {
			params = nil
		}
		toolName, ok := params["name"].(string)
		if !ok {
			toolName = ""
		}
		arguments, ok := params["arguments"].(map[string]interface{})
		if !ok {
			arguments = nil
		}

		tool, exists := s.tools.GetTool(toolName)
		if !exists {
			return map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]interface{}{
					"code":    -32601,
					"message": fmt.Sprintf("Tool not found: %s", toolName),
				},
			}
		}

		result, err := tool.Execute(arguments)
		if err != nil {
			return map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]interface{}{
					"code":    -32603,
					"message": err.Error(),
				},
			}
		}

		// Serialize result to JSON string for text content
		resultJSON, err := json.Marshal(result)
		if err != nil {
			resultJSON = []byte(fmt.Sprintf("%v", result))
		}

		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": string(resultJSON),
					},
				},
			},
		}
	}

	// Unknown method
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    -32601,
			"message": fmt.Sprintf("Method not found: %s", method),
		},
	}
}
