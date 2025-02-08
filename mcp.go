// Package goai implements an MCP (Model Context Protocol) server
package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
)

// JSON-RPC 2.0 error codes as defined in the MCP specification
const (
	MCPErrorCodeParseError     = -32700
	MCPErrorCodeInvalidRequest = -32600
	MCPErrorCodeMethodNotFound = -32601
	MCPErrorCodeInvalidParams  = -32602
	MCPErrorCodeInternal       = -32603
)

// MessageHandler type for processing incoming messages
type MCPMessageHandler = func(MCPMessage) error

// MCPServer represents an MCP server instance
type MCPServer struct {
	config              MCPServerConfig
	tools               map[string]MCPToolExecutor
	logger              *logrus.Logger
	tracer              trace.Tracer
	upgrader            websocket.Upgrader
	mu                  sync.RWMutex
	cancellationManager *MCPCancellationManager
	version             MCPVersion
	state               MCPLifecycleState
	shutdown            chan struct{}
	resourceManager     *MCPResourceManager
	transports          []MCPTransport
}

// MCPServerConfig holds configuration options for the MCP server
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

// MCPConnection represents an active client connection
type MCPConnection struct {
	ID     string
	Conn   *websocket.Conn
	Server *MCPServer
	Ctx    context.Context
	Cancel context.CancelFunc
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
		shutdown: make(chan struct{}),
		state:    MCPStateUninitialized,
		cancellationManager: &MCPCancellationManager{
			tokens: make(map[string]*MCPCancellationToken),
		},
	}

	server.initializeVersion()
	server.initResourceManager()

	// Setup message handler
	messageHandler := server.createMessageHandler()

	// Initialize transports based on config
	if config.EnableStdio {
		stdioTransport := NewStdioTransport(messageHandler)
		server.AddTransport(stdioTransport)
	}

	return server
}

// createMessageHandler returns a handler function for processing messages
func (s *MCPServer) createMessageHandler() MCPMessageHandler {
	return func(msg MCPMessage) error {
		return s.handleMessage(msg)
	}
}

// MCPLifecycleState represents the server's lifecycle states
type MCPLifecycleState int

const (
	MCPStateUninitialized MCPLifecycleState = iota
	MCPStateInitializing
	MCPStateRunning
	MCPStateShuttingDown
	MCPStateStopped
)

// MCPVersion represents the protocol version information
type MCPVersion struct {
	Version    string   `json:"version"`
	APIVersion string   `json:"apiVersion"`
	Extensions []string `json:"extensions,omitempty"`
}

func (s *MCPServer) initializeVersion() {
	s.version = MCPVersion{
		Version:    "2024-11-05",
		APIVersion: "1.0.0",
		Extensions: []string{},
	}
}

// MCPServerCapabilities represents the server's supported capabilities
type MCPServerCapabilities struct {
	Resources struct {
		Subscribe   bool `json:"subscribe,omitempty"`
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"resources,omitempty"`
	Tools struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"tools,omitempty"`
	Prompts struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"prompts,omitempty"`
	Logging        struct{} `json:"logging,omitempty"`
	MaxRequestSize int      `json:"maxRequestSize"`
	Version        string   `json:"version"`
	Extensions     []string `json:"extensions,omitempty"`
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

func (e *MCPError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("MCPError %d: %s (data: %s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("MCPError %d: %s", e.Code, e.Message)
}

// MCPInitializeParams represents initialization parameters from client
type MCPInitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ClientInfo      json.RawMessage `json:"clientInfo,omitempty"`
}

// MCPInitializeResult represents server response to initialization
type MCPInitializeResult struct {
	ProtocolVersion string                `json:"protocolVersion"`
	Capabilities    MCPServerCapabilities `json:"capabilities"`
	ServerInfo      json.RawMessage       `json:"serverInfo,omitempty"`
}

// Handle server initialization
func (s *MCPServer) handleInitialize(ctx context.Context, params MCPInitializeParams) (*MCPInitializeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != MCPStateUninitialized && s.state != MCPStateInitializing {
		return nil, &MCPError{
			Code:    MCPErrorCodeInvalidRequest,
			Message: "server already initialized",
		}
	}

	// Version compatibility check
	if params.ProtocolVersion != s.version.Version {
		return nil, &MCPError{
			Code:    MCPErrorCodeInvalidRequest,
			Message: "unsupported protocol version",
			Data: mustMarshal(map[string]interface{}{
				"supported": []string{s.version.Version},
				"requested": params.ProtocolVersion,
			}),
		}
	}

	// Server capabilities
	caps := MCPServerCapabilities{
		Resources: struct {
			Subscribe   bool `json:"subscribe,omitempty"`
			ListChanged bool `json:"listChanged,omitempty"`
		}{
			Subscribe:   true,
			ListChanged: true,
		},
		Tools: struct {
			ListChanged bool `json:"listChanged,omitempty"`
		}{
			ListChanged: true,
		},
		Prompts: struct {
			ListChanged bool `json:"listChanged,omitempty"`
		}{
			ListChanged: true,
		},
		Logging:        struct{}{},
		MaxRequestSize: s.config.MaxRequestSize,
		Version:        s.version.Version,
		Extensions:     s.version.Extensions,
	}

	s.state = MCPStateRunning

	return &MCPInitializeResult{
		ProtocolVersion: s.version.Version,
		Capabilities:    caps,
		ServerInfo: json.RawMessage(`{
			"name": "MCP Go Server",
			"version": "1.0.0"
		}`),
	}, nil
}

// handleShutdown handles server shutdown requests
func (s *MCPServer) handleShutdown(ctx context.Context) (json.RawMessage, error) {
	s.mu.Lock()
	s.state = MCPStateShuttingDown
	s.mu.Unlock()

	close(s.shutdown)

	return json.Marshal(map[string]string{
		"status": "shutting_down",
	})
}

// Core message handling
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
		var params MCPInitializeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return s.sendErrorResponse(msg.ID, MCPErrorCodeParseError, "invalid initialize params", nil)
		}
		result, err = s.handleInitialize(ctx, params)
	case "shutdown":
		result, err = s.handleShutdown(ctx)
	case "$/progress":
		var progress MCPProgress
		if err := json.Unmarshal(msg.Params, &progress); err != nil {
			return s.sendErrorResponse(msg.ID, MCPErrorCodeParseError, "invalid progress params", nil)
		}
		err = s.handleProgress(ctx, progress)
	case "$/cancel":
		var token MCPCancellationToken
		if err := json.Unmarshal(msg.Params, &token); err != nil {
			return s.sendErrorResponse(msg.ID, MCPErrorCodeParseError, "invalid cancellation params", nil)
		}
		err = s.handleCancellation(ctx, token)
	default:
		return s.sendErrorResponse(msg.ID, MCPErrorCodeMethodNotFound, fmt.Sprintf("unknown method: %s", msg.Method), nil)
	}

	if err != nil {
		if mcpErr, ok := err.(*MCPError); ok {
			response.Error = mcpErr
		} else {
			response.Error = &MCPError{
				Code:    MCPErrorCodeInternal,
				Message: err.Error(),
			}
		}
	} else {
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

func (s *MCPServer) sendErrorResponse(id interface{}, code int, message string, data interface{}) error {
	response := MCPMessage{
		JsonRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
		},
	}

	if data != nil {
		response.Error.Data = mustMarshal(data)
	}

	return s.sendResponse(response)
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

// Progress tracking
type MCPProgress struct {
	Percentage int             `json:"percentage"`
	Message    string          `json:"message"`
	Details    json.RawMessage `json:"details,omitempty"`
}

func (s *MCPServer) handleProgress(ctx context.Context, progress MCPProgress) error {
	notification := MCPMessage{
		JsonRPC: "2.0",
		Method:  "$/progress",
		Params:  mustMarshal(progress),
	}

	return s.sendResponse(notification)
}

// Cancellation support
type MCPCancellationToken struct {
	ID     string    `json:"id"`
	Reason string    `json:"reason,omitempty"`
	Time   time.Time `json:"time"`
}

type MCPCancellationManager struct {
	tokens map[string]*MCPCancellationToken
	mu     sync.RWMutex
}

func (s *MCPServer) handleCancellation(ctx context.Context, token MCPCancellationToken) error {
	s.cancellationManager.mu.Lock()
	defer s.cancellationManager.mu.Unlock()

	s.cancellationManager.tokens[token.ID] = &token
	return nil
}

// Transport interfaces and implementations
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

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.ctx.Done():
				return
			case err := <-t.errChan:
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
	t.cancel()
	select {
	case <-t.stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("transport stop timeout")
	}
}

// Resource management
type MCPResource struct {
	URI       string          `json:"uri"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Content   string          `json:"content"`
	MimeType  string          `json:"mimeType,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Version   string          `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type MCPResourceManager struct {
	resources   map[string]*MCPResource
	subscribers map[string]map[string]*MCPConnection
	mu          sync.RWMutex
}

type MCPResourceChange struct {
	ResourceID string          `json:"resourceId"`
	Type       string          `json:"type"`
	Resource   *MCPResource    `json:"resource,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Delta      json.RawMessage `json:"delta,omitempty"`
}

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

	if resource.URI == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	resource.UpdatedAt = time.Now()
	m.resources[resource.URI] = resource

	m.notifyResourceChange(MCPResourceChange{
		ResourceID: resource.URI,
		Type:       "created",
		Resource:   resource,
		Timestamp:  resource.UpdatedAt,
	})

	return nil
}

func (m *MCPResourceManager) UpdateResource(uri string, updates *MCPResource) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.resources[uri]
	if !exists {
		return fmt.Errorf("resource %s not found", uri)
	}

	delta, _ := json.Marshal(map[string]interface{}{
		"oldContent": existing.Content,
		"newContent": updates.Content,
	})

	updates.URI = uri
	updates.UpdatedAt = time.Now()
	m.resources[uri] = updates

	m.notifyResourceChange(MCPResourceChange{
		ResourceID: uri,
		Type:       "updated",
		Resource:   updates,
		Timestamp:  updates.UpdatedAt,
		Delta:      delta,
	})

	return nil
}

func (m *MCPResourceManager) DeleteResource(uri string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.resources[uri]; !exists {
		return fmt.Errorf("resource %s not found", uri)
	}

	delete(m.resources, uri)

	m.notifyResourceChange(MCPResourceChange{
		ResourceID: uri,
		Type:       "deleted",
		Timestamp:  time.Now(),
	})

	return nil
}

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
			go func(c *MCPConnection) {
				if err := c.Conn.WriteJSON(notification); err != nil {
					m.Unsubscribe(change.ResourceID, c)
				}
			}(conn)
		}
	}
}

// Tool related types and implementations
type MCPTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	Version      string          `json:"version"`
	Capabilities []string        `json:"capabilities"`
}

type MCPToolInput struct {
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params,omitempty"`
}

type MCPToolExecutor interface {
	GetDefinition() MCPTool
	Execute(ctx context.Context, input json.RawMessage) (MCPToolResponse, error)
}

type MCPToolResponse struct {
	Content  []MCPContentItem `json:"content"`
	IsError  bool             `json:"isError,omitempty"`
	Metadata json.RawMessage  `json:"metadata,omitempty"`
}

type MCPContentItem struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     string          `json:"data,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Tool execution
func (s *MCPServer) AddTransport(transport MCPTransport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transports = append(s.transports, transport)
}

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

// Utility functions
func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
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
		EnableStdio:          true,
		EnableSSE:            true,
	}
}

// Query support
type MCPResourceQuery struct {
	Types    []string `json:"types,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	MaxItems int      `json:"maxItems,omitempty"`
	Cursor   string   `json:"cursor,omitempty"`
}

// MCPPaginationInfo support
type MCPPaginationInfo struct {
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Start Server lifecycle
func (s *MCPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.state != MCPStateUninitialized {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}
	s.state = MCPStateInitializing // Only set to Initializing here
	s.mu.Unlock()

	for _, transport := range s.transports {
		if err := transport.Start(ctx); err != nil {
			return fmt.Errorf("failed to start transport: %w", err)
		}
	}

	return nil
}

func (s *MCPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.state == MCPStateShuttingDown || s.state == MCPStateStopped {
		s.mu.Unlock()
		return nil
	}
	s.state = MCPStateShuttingDown
	s.mu.Unlock()

	close(s.shutdown)

	// Stop all transports
	for _, transport := range s.transports {
		if err := transport.Stop(); err != nil {
			s.logger.WithError(err).Error("Failed to stop transport")
		}
	}

	s.mu.Lock()
	s.state = MCPStateStopped
	s.mu.Unlock()

	return nil
}

// MCPSSETransport implements SSE transport
type MCPSSETransport struct {
	writer   http.ResponseWriter
	flusher  http.Flusher
	ctx      context.Context
	cancel   context.CancelFunc
	handler  MCPMessageHandler
	errChan  chan error
	stopped  chan struct{}
	endpoint string
}

// NewSSETransport creates a new SSE transport
func NewSSETransport(w http.ResponseWriter, endpoint string, handler MCPMessageHandler) (*MCPSSETransport, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &MCPSSETransport{
		writer:   w,
		flusher:  flusher,
		ctx:      ctx,
		cancel:   cancel,
		handler:  handler,
		errChan:  make(chan error, 1),
		stopped:  make(chan struct{}),
		endpoint: endpoint,
	}, nil
}

func (t *MCPSSETransport) Start(ctx context.Context) error {
	// Set SSE headers
	t.writer.Header().Set("Content-Type", "text/event-stream")
	t.writer.Header().Set("Cache-Control", "no-cache")
	t.writer.Header().Set("Connection", "keep-alive")

	// Send initial endpoint event
	endpointEvent := fmt.Sprintf("event: endpoint\ndata: {\"endpoint\":\"%s\"}\n\n", t.endpoint)
	if _, err := fmt.Fprint(t.writer, endpointEvent); err != nil {
		return fmt.Errorf("failed to send endpoint event: %w", err)
	}
	t.flusher.Flush()

	// Keep connection alive with periodic flushes
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.ctx.Done():
				return
			case <-ticker.C:
				if _, err := fmt.Fprint(t.writer, ": keepalive\n\n"); err != nil {
					t.errChan <- fmt.Errorf("keepalive failed: %w", err)
					return
				}
				t.flusher.Flush()
			}
		}
	}()

	return nil
}

func (t *MCPSSETransport) Send(message MCPMessage) error {
	select {
	case <-t.ctx.Done():
		return errors.New("transport stopped")
	default:
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		eventData := fmt.Sprintf("event: message\ndata: %s\n\n", string(data))
		if _, err := fmt.Fprint(t.writer, eventData); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}

		t.flusher.Flush()
		return nil
	}
}

func (t *MCPSSETransport) Receive() (MCPMessage, error) {
	// SSE is unidirectional (server to client)
	return MCPMessage{}, errors.New("receive not supported for SSE transport")
}

func (t *MCPSSETransport) Stop() error {
	t.cancel()
	close(t.stopped)
	return nil
}

// MCPSSEHandler handles SSE connections
func (s *MCPServer) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate unique endpoint for this connection
	endpoint := fmt.Sprintf("/mcp/message/%s", uuid.New().String())

	transport, err := NewSSETransport(w, endpoint, s.createMessageHandler())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.AddTransport(transport)
	if err := transport.Start(r.Context()); err != nil {
		s.logger.WithError(err).Error("Failed to start SSE transport")
		return
	}
}

// HandleMessage handles incoming messages via POST endpoint
func (s *MCPServer) HandleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var message MCPMessage
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, "Invalid message format", http.StatusBadRequest)
		return
	}

	if err := s.handleMessage(message); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
