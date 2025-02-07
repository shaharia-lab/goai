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
	config              MCPServerConfig
	tools               map[string]MCPToolExecutor
	logger              *logrus.Logger
	tracer              trace.Tracer
	upgrader            websocket.Upgrader
	mu                  sync.RWMutex
	cancellationManager *MCPCancellationManager
	authContext         *MCPAuthContext
	version             MCPVersion
	state               MCPLifecycleState
	shutdown            chan struct{}
}

// MCPProgress represents a progress update during tool execution
type MCPProgress struct {
	Percentage int             `json:"percentage"`
	Message    string          `json:"message"`
	Details    json.RawMessage `json:"details,omitempty"`
}

// MCPProgressCallback is a function type for handling progress updates
type MCPProgressCallback func(progress MCPProgress)

// MCPCancellationToken represents a token for cancelling operations
type MCPCancellationToken struct {
	ID     string    `json:"id"`
	Reason string    `json:"reason,omitempty"`
	Time   time.Time `json:"time"`
}

// MCPCancellationManager handles cancellation requests
type MCPCancellationManager struct {
	tokens map[string]*MCPCancellationToken
	mu     sync.RWMutex
}

// MCPToolExecutionContext holds context for tool execution
type MCPToolExecutionContext struct {
	Context  context.Context
	Progress MCPProgressCallback
	Cancel   *MCPCancellationToken
	Resource *MCPResource
}

// MCPVersion represents the protocol version
type MCPVersion struct {
	Version    string   `json:"version"`
	APIVersion string   `json:"apiVersion"`
	Extensions []string `json:"extensions,omitempty"`
}

// MCPVersionInfo provides detailed version information
type MCPVersionInfo struct {
	Protocol MCPVersion      `json:"protocol"`
	Server   json.RawMessage `json:"server,omitempty"`
	Client   json.RawMessage `json:"client,omitempty"`
}

// MCPPingRequest represents a ping request
type MCPPingRequest struct {
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data,omitempty"`
}

// MCPPingResponse represents a ping response
type MCPPingResponse struct {
	Timestamp time.Time `json:"timestamp"`
	RTT       int64     `json:"rtt"`
	Data      string    `json:"data,omitempty"`
}

// MCPAuthConfig represents authorization configuration
type MCPAuthConfig struct {
	Type        string          `json:"type"`
	Scheme      string          `json:"scheme"`
	Token       string          `json:"token,omitempty"`
	Credentials json.RawMessage `json:"credentials,omitempty"`
}

// MCPAuthContext represents the authorization context
type MCPAuthContext struct {
	Config MCPAuthConfig `json:"config"`
	Scope  []string      `json:"scope,omitempty"`
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

// MCPLifecycleState represents server lifecycle states
type MCPLifecycleState int

const (
	MCPStateUninitialized MCPLifecycleState = iota
	MCPStateInitializing
	MCPStateRunning
	MCPStateShuttingDown
	MCPStateStopped
)

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

// Initialize version info during server creation
func (s *MCPServer) initializeVersion() {
	s.version = MCPVersion{
		Version:    "2024-11-05", // Current protocol version
		APIVersion: "1.0.0",
		Extensions: []string{}, // Add supported extensions
	}
}

// Handle progress updates
func (s *MCPServer) handleProgress(conn *MCPConnection, progress MCPProgress) error {
	progressJSON, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("failed to marshal progress: %w", err)
	}

	message := MCPMessage{
		JsonRPC: "2.0",
		Method:  "$/progress",
		Params:  progressJSON,
	}
	return conn.Conn.WriteJSON(message)
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

// MCPInitializeParams represents initialization parameters from client
type MCPInitializeParams struct {
	ClientInfo     json.RawMessage `json:"clientInfo,omitempty"`
	ClientFeatures []string        `json:"clientFeatures"`
	Capabilities   json.RawMessage `json:"capabilities,omitempty"`
}

// MCPInitializeResult represents server response to initialization
type MCPInitializeResult struct {
	ServerInfo      json.RawMessage       `json:"serverInfo,omitempty"`
	Capabilities    MCPServerCapabilities `json:"capabilities"`
	ProtocolVersion string                `json:"protocolVersion"`
}

func (s *MCPServer) handleInitialize(ctx context.Context, params MCPInitializeParams) (*MCPInitializeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Server capabilities
	caps := MCPServerCapabilities{
		SupportedFeatures: []string{
			"tools",
			"resources",
			"progress",
			"cancellation",
			"ping",
		},
		MaxRequestSize: s.config.MaxRequestSize,
		Version:        s.version.Version,
		Extensions:     s.version.Extensions,
	}

	return &MCPInitializeResult{
		ServerInfo: json.RawMessage(`{
            "name": "MCP Go Server",
            "version": "1.0.0"
        }`),
		Capabilities:    caps,
		ProtocolVersion: s.version.Version,
	}, nil
}

func (s *MCPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.state == MCPStateShuttingDown || s.state == MCPStateStopped {
		s.mu.Unlock()
		return nil
	}
	s.state = MCPStateShuttingDown
	s.mu.Unlock()

	// Close all active connections
	close(s.shutdown)

	// Wait for ongoing operations to complete or context to cancel
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.mu.Lock()
		s.state = MCPStateStopped
		s.mu.Unlock()
		return nil
	}
}

func (s *MCPServer) ExecuteTool(ctx context.Context, input MCPToolInput) (*MCPToolResponse, error) {
	s.mu.RLock()
	tool, exists := s.tools[input.Name]
	s.mu.RUnlock()

	if !exists {
		return nil, &MCPError{
			Code:    MCPErrorCodeMethodNotFound,
			Message: fmt.Sprintf("tool %s not found", input.Name),
		}
	}

	// Create execution context with progress and cancellation
	execCtx := &MCPToolExecutionContext{
		Context: ctx,
		Progress: func(p MCPProgress) {
			if err := s.handleProgress(nil, p); err != nil {
				s.logger.WithError(err).Error("failed to send progress update")
			}
		},
	}

	// Execute the tool
	response, err := tool.Execute(execCtx.Context, input.Arguments)
	if err != nil {
		return nil, &MCPError{
			Code:    MCPErrorCodeInternal,
			Message: err.Error(),
		}
	}

	return &response, nil
}

func (s *MCPServer) handlePing(ctx context.Context, req MCPPingRequest) (*MCPPingResponse, error) {
	return &MCPPingResponse{
		Timestamp: time.Now(),
		RTT:       time.Since(req.Timestamp).Milliseconds(),
		Data:      req.Data,
	}, nil
}

func (s *MCPServer) handleCancellation(ctx context.Context, token MCPCancellationToken) error {
	s.cancellationManager.mu.Lock()
	defer s.cancellationManager.mu.Unlock()

	s.cancellationManager.tokens[token.ID] = &token
	return nil
}
