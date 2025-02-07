package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

// Common MCP error codes
const (
	MCPErrorCodeParseError     = -32700
	MCPErrorCodeInvalidRequest = -32600
	MCPErrorCodeMethodNotFound = -32601
	MCPErrorCodeInvalidParams  = -32602
	MCPErrorCodeInternal       = -32603
)

// MCPServer represents the main MCP server instance
type MCPServer struct {
	config   MCPServerConfig
	tools    map[string]MCPToolExecutor
	logger   *logrus.Logger
	tracer   trace.Tracer
	upgrader websocket.Upgrader
	mu       sync.RWMutex
}

// MCPConnection represents an active client connection
type MCPConnection struct {
	ID     string
	Conn   *websocket.Conn
	Server *MCPServer
	Ctx    context.Context
	Cancel context.CancelFunc
}

// MCPMessage represents a JSON-RPC 2.0 message
type MCPMessage struct {
	JsonRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error
type MCPError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// MCPServerCapabilities represents server capabilities
type MCPServerCapabilities struct {
	SupportedFeatures []string `json:"supportedFeatures"`
	MaxRequestSize    int      `json:"maxRequestSize"`
	Version           string   `json:"version"`
	Extensions        []string `json:"extensions,omitempty"`
}

// MCPResource represents a resource in the protocol
type MCPResource struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Content   string          `json:"content"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Version   string          `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// MCPContentItem represents a content item in a tool response
type MCPContentItem struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// MCPTool represents a tool definition
type MCPTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Version      string          `json:"version"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	Capabilities []string        `json:"capabilities"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

// MCPToolInput represents input parameters for tool execution
type MCPToolInput struct {
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
	ResourceID string          `json:"resourceId,omitempty"`
}

// MCPToolResponse represents the response from tool execution
type MCPToolResponse struct {
	Content  []MCPContentItem `json:"content"`
	IsError  bool             `json:"isError,omitempty"`
	Metadata json.RawMessage  `json:"metadata,omitempty"`
}

// MCPHandlers provides access to individual HTTP handlers
type MCPHandlers struct {
	Initialize    http.HandlerFunc
	ListTools     http.HandlerFunc
	ToolCall      http.HandlerFunc
	GetResource   http.HandlerFunc
	WatchResource http.HandlerFunc
}

// MCPServerConfig holds configuration for the MCP server
type MCPServerConfig struct {
	Logger               *logrus.Logger
	Tracer               trace.Tracer
	EnableMetrics        bool
	MaxToolExecutionTime time.Duration
	MaxRequestSize       int
	AllowedOrigins       []string
}

// MCPToolExecutor defines the interface that all MCP tools must implement
type MCPToolExecutor interface {
	GetDefinition() MCPTool
	Execute(ctx context.Context, input json.RawMessage) (MCPToolResponse, error)
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(config MCPServerConfig) *MCPServer {
	if config.Logger == nil {
		config.Logger = logrus.New()
	}

	return &MCPServer{
		config: config,
		tools:  make(map[string]MCPToolExecutor),
		logger: config.Logger,
		tracer: config.Tracer,
		upgrader: websocket.Upgrader{
			CheckOrigin: makeOriginChecker(config.AllowedOrigins),
		},
	}
}

// RegisterTool registers a new tool with the server
func (s *MCPServer) RegisterTool(tool MCPToolExecutor) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	def := tool.GetDefinition()
	if def.Name == "" {
		return errors.New("tool name cannot be empty")
	}

	s.tools[def.Name] = tool
	return nil
}

// Error implements the error interface
func (e *MCPError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("MCPError %d: %s (data: %s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("MCPError %d: %s", e.Code, e.Message)
}

func makeOriginChecker(allowedOrigins []string) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		if len(allowedOrigins) == 0 {
			return true
		}
		origin := r.Header.Get("Origin")
		for _, allowed := range allowedOrigins {
			if allowed == "*" || allowed == origin {
				return true
			}
		}
		return false
	}
}

// DefaultMCPServerConfig returns a default configuration
func DefaultMCPServerConfig() MCPServerConfig {
	return MCPServerConfig{
		MaxToolExecutionTime: 30 * time.Second,
		MaxRequestSize:       1024 * 1024, // 1MB
	}
}
