package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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

type MCPMessageHandler func(MCPMessage) error

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
	resourceManager     *MCPResourceManager
	transports          []MCPTransport
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
	Params     json.RawMessage `json:"params,omitempty"`
	ResourceID string          `json:"resourceId,omitempty"`
	Tool       string
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
	EnableStdio          bool `json:"enableStdio"`
	EnableSSE            bool `json:"enableSSE"`
	EnableWebSocket      bool `json:"enableWebSocket"`
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

	server := &MCPServer{
		config: config,
		tools:  make(map[string]MCPToolExecutor),
		logger: config.Logger,
		tracer: config.Tracer,
		upgrader: websocket.Upgrader{
			CheckOrigin: makeOriginChecker(config.AllowedOrigins),
		},
	}

	// Setup message handler
	handler := func(msg MCPMessage) error {
		return server.handleMessage(msg)
	}

	// Setup transports
	if config.EnableStdio {
		stdioTransport := NewStdioTransport(handler)
		server.AddTransport(stdioTransport)
	}

	if config.EnableSSE {
		// Setup SSE transport
	}

	if config.EnableWebSocket {
		// Setup WebSocket transport
	}

	return server
}

// Server method to handle transport setup
func (s *MCPServer) AddTransport(transport MCPTransport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transports = append(s.transports, transport)
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

func (c *MCPToolExecutionContext) Deadline() (deadline time.Time, ok bool) {
	return c.Context.Deadline()
}

func (c *MCPToolExecutionContext) Done() <-chan struct{} {
	return c.Context.Done()
}

func (c *MCPToolExecutionContext) Err() error {
	return c.Context.Err()
}

func (c *MCPToolExecutionContext) Value(key interface{}) interface{} {
	return c.Context.Value(key)
}

// MCPInitializeParams represents initialization parameters from client
type MCPInitializeParams struct {
	Auth         *MCPAuthConfig  `json:"authentication,omitempty"` // Use the full package path
	ClientInfo   json.RawMessage `json:"clientInfo,omitempty"`
	Capabilities json.RawMessage `json:"capabilities,omitempty"`
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
		Progress: func(progress MCPProgress) {
			s.sendProgress(ctx, progress)
		},
	}

	// Execute the tool
	response, err := tool.Execute(execCtx.Context, input.Params)
	if err != nil {
		return nil, &MCPError{
			Code:    MCPErrorCodeInternal,
			Message: err.Error(),
		}
	}

	return &response, nil
}

// Server message handling methods
func (s *MCPServer) handleMessage(msg MCPMessage) error {
	s.logger.WithField("method", msg.Method).Debug("Handling message")

	ctx := context.Background()
	if msg.ID != nil {
		ctx = context.WithValue(ctx, "requestID", msg.ID)
	}

	var response MCPMessage
	response.JsonRPC = "2.0"
	response.ID = msg.ID

	var result interface{}
	var err error

	switch msg.Method {
	case "initialize":
		response.Result, err = s.handleInitializeRequest(ctx, msg.Params)
	case "shutdown":
		response.Result, err = s.handleShutdown(ctx, msg.Params)
	case "execute":
		response.Result, err = s.handleToolExecution(ctx, msg.Params)
	case "ping":
		var pingRequest MCPPingRequest
		if err := json.Unmarshal(msg.Params, &pingRequest); err != nil {
			return fmt.Errorf("failed to parse ping request: %w", err)
		}
		result, err = s.handlePing(ctx, pingRequest)
	default:
		err = fmt.Errorf("unknown method: %s", msg.Method)
	}

	if err != nil {
		response.Error = &MCPError{
			Code:    MCPErrorCodeInternal,
			Message: err.Error(),
		}
	} else {
		// Marshal the result to json.RawMessage
		response.Result, err = json.Marshal(result)
		if err != nil {
			response.Error = &MCPError{
				Code:    MCPErrorCodeInternal,
				Message: "failed to marshal response",
			}
		}
	}

	return s.sendResponse(response)
}

func (s *MCPServer) handleInitializeRequest(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var initParams MCPInitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, fmt.Errorf("failed to parse initialize params: %w", err)
	}

	if initParams.Auth == nil {
		return nil, fmt.Errorf("authentication configuration is required")
	}

	if initParams.Auth != nil {
		if err := s.authenticate(initParams.Auth); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	result := MCPInitializeResult{
		ServerInfo: json.RawMessage(`{
            "name": "MCP Server",
            "version": "1.0.0"
        }`),
		Capabilities: MCPServerCapabilities{
			SupportedFeatures: []string{
				"tools",
				"resources",
				"cancellation",
				"progress",
			},
			MaxRequestSize: 10 * 1024 * 1024, // 10MB
			Version:        "1.0.0",
		},
		ProtocolVersion: "1.0",
	}

	return json.Marshal(result)
}

func (s *MCPServer) handleShutdown(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	s.state = MCPStateShuttingDown
	close(s.shutdown)
	return json.Marshal(map[string]string{"status": "shutting_down"})
}

func (s *MCPServer) handleToolExecution(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var toolInput MCPToolInput
	if err := json.Unmarshal(params, &toolInput); err != nil {
		return nil, fmt.Errorf("failed to parse tool input: %w", err)
	}

	tool, exists := s.tools[toolInput.Tool]
	if !exists {
		return nil, fmt.Errorf("tool %s not found", toolInput.Tool)
	}

	execCtx := MCPToolExecutionContext{
		Context: ctx,
		Progress: func(progress MCPProgress) {
			s.sendProgress(ctx, progress)
		},
	}

	result, err := tool.Execute(execCtx.Context, toolInput.Params)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
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

// MCPResourceManager handles resource operations
type MCPResourceManager struct {
	resources   map[string]*MCPResource
	subscribers map[string]map[string]*MCPConnection // resourceID -> connectionID -> connection
	mu          sync.RWMutex
}

// MCPResourceChange represents a change in a resource
type MCPResourceChange struct {
	ResourceID string          `json:"resourceId"`
	Type       string          `json:"type"` // "created", "updated", "deleted"
	Resource   *MCPResource    `json:"resource,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Delta      json.RawMessage `json:"delta,omitempty"`
}

// MCPResourceQuery represents query parameters for listing resources
type MCPResourceQuery struct {
	Types    []string `json:"types,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	MaxItems int      `json:"maxItems,omitempty"`
}

// Add ResourceManager to MCPServer
func (s *MCPServer) initResourceManager() {
	s.resourceManager = &MCPResourceManager{
		resources:   make(map[string]*MCPResource),
		subscribers: make(map[string]map[string]*MCPConnection),
	}
}

// Resource Management methods
func (m *MCPResourceManager) AddResource(resource *MCPResource) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if resource.ID == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}

	resource.UpdatedAt = time.Now()
	m.resources[resource.ID] = resource

	// Notify subscribers
	m.notifyResourceChange(MCPResourceChange{
		ResourceID: resource.ID,
		Type:       "created",
		Resource:   resource,
		Timestamp:  resource.UpdatedAt,
	})

	return nil
}

func (s *MCPServer) sendResponse(response MCPMessage) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastErr error
	for _, transport := range s.transports {
		if err := transport.Send(response); err != nil {
			lastErr = err
			s.logger.WithError(err).Error("Failed to send response through transport")
		}
	}

	return lastErr
}

func (s *MCPServer) sendProgress(ctx context.Context, progress MCPProgress) {
	notification := MCPMessage{
		JsonRPC: "2.0",
		Method:  "$/progress",
		Params:  mustMarshal(progress),
	}

	if err := s.sendResponse(notification); err != nil {
		s.logger.WithError(err).Error("Failed to send progress notification")
	}
}

func (s *MCPServer) authenticate(auth *MCPAuthConfig) error {
	return nil
}

func (m *MCPResourceManager) UpdateResource(id string, updates *MCPResource) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.resources[id]
	if !exists {
		return fmt.Errorf("resource %s not found", id)
	}

	// Create delta for change notification
	delta, _ := json.Marshal(map[string]interface{}{
		"oldContent": existing.Content,
		"newContent": updates.Content,
	})

	updates.ID = id
	updates.UpdatedAt = time.Now()
	m.resources[id] = updates

	// Notify subscribers
	m.notifyResourceChange(MCPResourceChange{
		ResourceID: id,
		Type:       "updated",
		Resource:   updates,
		Timestamp:  updates.UpdatedAt,
		Delta:      delta,
	})

	return nil
}

func (m *MCPResourceManager) DeleteResource(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.resources[id]; !exists {
		return fmt.Errorf("resource %s not found", id)
	}

	delete(m.resources, id)

	// Notify subscribers
	m.notifyResourceChange(MCPResourceChange{
		ResourceID: id,
		Type:       "deleted",
		Timestamp:  time.Now(),
	})

	return nil
}

func (m *MCPResourceManager) ListResources(query MCPResourceQuery) ([]*MCPResource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*MCPResource

	for _, resource := range m.resources {
		if matchesQuery(resource, query) {
			results = append(results, resource)
		}
	}

	if query.MaxItems > 0 && len(results) > query.MaxItems {
		results = results[:query.MaxItems]
	}

	return results, nil
}

// Subscription management
func (m *MCPResourceManager) Subscribe(resourceID string, conn *MCPConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscribers[resourceID]; !exists {
		m.subscribers[resourceID] = make(map[string]*MCPConnection)
	}
	m.subscribers[resourceID][conn.ID] = conn
	return nil
}

func (m *MCPResourceManager) Unsubscribe(resourceID string, conn *MCPConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subs, exists := m.subscribers[resourceID]; exists {
		delete(subs, conn.ID)
		if len(subs) == 0 {
			delete(m.subscribers, resourceID)
		}
	}
}

func (m *MCPResourceManager) notifyResourceChange(change MCPResourceChange) {
	if subs, exists := m.subscribers[change.ResourceID]; exists {
		notification := MCPMessage{
			JsonRPC: "2.0",
			Method:  "$/resource/change",
			Params:  mustMarshal(change),
		}

		for _, conn := range subs {
			// Async notification to avoid blocking
			go func(c *MCPConnection) {
				if err := c.Conn.WriteJSON(notification); err != nil {
					// Handle error (possibly unsubscribe if connection is dead)
					m.Unsubscribe(change.ResourceID, c)
				}
			}(conn)
		}
	}
}

// Helper function to marshal JSON without error (used in notification context)
func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// Helper function to match resources against query
func matchesQuery(resource *MCPResource, query MCPResourceQuery) bool {
	if len(query.Types) > 0 {
		typeMatch := false
		for _, t := range query.Types {
			if resource.Type == t {
				typeMatch = true
				break
			}
		}
		if !typeMatch {
			return false
		}
	}

	if query.Pattern != "" {
		// Implement pattern matching logic here
		// Could use regex or simple string matching depending on requirements
	}

	return true
}

// MCPTransport represents different transport types
type MCPTransport interface {
	Start(context.Context) error
	Stop() error
	Send(message MCPMessage) error
	Receive() (MCPMessage, error)
}

// MCPStdioTransport implements stdio transport
type MCPStdioTransport struct {
	decoder *json.Decoder
	encoder *json.Encoder
	ctx     context.Context
	cancel  context.CancelFunc
	handler MCPMessageHandler
	errChan chan error
	stopped chan struct{}
}

// MCPSSETransport implements Server-Sent Events transport
type MCPSSETransport struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	ctx     context.Context
	cancel  context.CancelFunc
}

// MCPLogLevel represents logging levels
type MCPLogLevel string

const (
	MCPLogTrace   MCPLogLevel = "trace"
	MCPLogDebug   MCPLogLevel = "debug"
	MCPLogInfo    MCPLogLevel = "info"
	MCPLogWarning MCPLogLevel = "warning"
	MCPLogError   MCPLogLevel = "error"
)

// MCPLogMessage represents a log message
type MCPLogMessage struct {
	Level   MCPLogLevel     `json:"level"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// MCPPaginationInfo represents pagination metadata
type MCPPaginationInfo struct {
	Total       int    `json:"total"`
	PageSize    int    `json:"pageSize"`
	CurrentPage int    `json:"currentPage"`
	NextCursor  string `json:"nextCursor,omitempty"`
}

// MCPCompletionItem represents a completion suggestion
type MCPCompletionItem struct {
	Label         string          `json:"label"`
	Kind          string          `json:"kind"`
	Detail        string          `json:"detail,omitempty"`
	Documentation json.RawMessage `json:"documentation,omitempty"`
}

func NewStdioTransport(handler MCPMessageHandler) *MCPStdioTransport {
	ctx, cancel := context.WithCancel(context.Background())
	return &MCPStdioTransport{
		decoder: json.NewDecoder(os.Stdin),
		encoder: json.NewEncoder(os.Stdout),
		ctx:     ctx,
		cancel:  cancel,
		handler: handler,
		errChan: make(chan error, 1),
		stopped: make(chan struct{}),
	}
}

func (t *MCPStdioTransport) Start(ctx context.Context) error {
	// Start reading from stdin
	go func() {
		defer close(t.stopped)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.ctx.Done():
				return
			default:
				msg, err := t.Receive()
				if err != nil {
					if err != io.EOF {
						t.errChan <- fmt.Errorf("receive error: %w", err)
					}
					continue
				}

				if err := t.handler(msg); err != nil {
					t.errChan <- fmt.Errorf("handler error: %w", err)
				}
			}
		}
	}()

	// Monitor for errors
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.ctx.Done():
				return
			case err := <-t.errChan:
				// Log error (assuming we have access to logger)
				fmt.Fprintf(os.Stderr, "Transport error: %v\n", err)
			}
		}
	}()

	return nil
}

func (t *MCPStdioTransport) Send(message MCPMessage) error {
	select {
	case <-t.ctx.Done():
		return errors.New("transport stopped")
	default:
		return t.encoder.Encode(message)
	}
}

func (t *MCPStdioTransport) Receive() (MCPMessage, error) {
	var msg MCPMessage
	err := t.decoder.Decode(&msg)
	return msg, err
}

func (t *MCPStdioTransport) Stop() error {
	t.cancel() // Cancel the context

	// Wait for graceful shutdown
	select {
	case <-t.stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("transport stop timeout")
	}
}

// NewSSETransport SSE Transport implementation
func NewSSETransport(w http.ResponseWriter) (*MCPSSETransport, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &MCPSSETransport{
		writer:  w,
		flusher: flusher,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

func (t *MCPSSETransport) Send(message MCPMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	fmt.Fprintf(t.writer, "data: %s\n\n", data)
	t.flusher.Flush()
	return nil
}

// Enhanced cancellation support
func (cm *MCPCancellationManager) Cancel(tokenID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	token, exists := cm.tokens[tokenID]
	if !exists {
		return fmt.Errorf("cancellation token %s not found", tokenID)
	}

	token.Time = time.Now()
	return nil
}

// Completion support
func (s *MCPServer) handleCompletion(ctx context.Context, params json.RawMessage) ([]MCPCompletionItem, error) {
	var completions []MCPCompletionItem
	// Implementation depends on specific completion requirements
	return completions, nil
}

// Enhanced pagination support
type MCPPaginatedResponse struct {
	Items      interface{}       `json:"items"`
	Pagination MCPPaginationInfo `json:"pagination"`
}

func (s *MCPServer) paginateResults(items interface{}, page, pageSize int, cursor string) MCPPaginatedResponse {
	// Implementation of cursor-based pagination
	return MCPPaginatedResponse{
		Items: items,
		Pagination: MCPPaginationInfo{
			CurrentPage: page,
			PageSize:    pageSize,
			// Set other pagination fields
		},
	}
}

// Enhanced logging support
func (s *MCPServer) log(level MCPLogLevel, message string, details interface{}) {
	logMsg := MCPLogMessage{
		Level:   level,
		Message: message,
	}

	if details != nil {
		if data, err := json.Marshal(details); err == nil {
			logMsg.Details = data
		}
	}

	// Map MCP log levels to logrus levels
	var logrusLevel logrus.Level
	switch level {
	case MCPLogTrace:
		logrusLevel = logrus.TraceLevel
	case MCPLogDebug:
		logrusLevel = logrus.DebugLevel
	case MCPLogInfo:
		logrusLevel = logrus.InfoLevel
	case MCPLogWarning:
		logrusLevel = logrus.WarnLevel
	case MCPLogError:
		logrusLevel = logrus.ErrorLevel
	}

	s.logger.Log(logrusLevel, message)
}
