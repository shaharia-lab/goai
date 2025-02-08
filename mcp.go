// Package goai implements an MCP (Model Context Protocol) server
package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
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

// MCPMethodHandler defines the function signature for method handlers
type MCPMethodHandler func(conn *MCPConnection, params json.RawMessage) (interface{}, error)

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
	capabilities        MCPServerCapabilities
	methodHandlers      map[string]MCPMethodHandler
	handler             MessageHandler
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
	conn   net.Conn
	writer *json.Encoder
	Cancel func()
	mu     sync.Mutex
}

func (c *MCPConnection) SendMessage(msg MCPMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writer == nil {
		return errors.New("connection writer is not initialized")
	}

	err := c.writer.Encode(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	return nil
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
		resourceManager: NewMCPResourceManager(),
		transports:      make([]MCPTransport, 0),
	}

	server.methodHandlers = make(map[string]MCPMethodHandler)
	server.upgrader = websocket.Upgrader{
		CheckOrigin: makeOriginChecker(config.AllowedOrigins),
	}

	server.initializeVersion()
	// Register method handlers
	server.registerMethodHandlers()

	// Setup message handler
	messageHandler := server.createMessageHandler()

	// Initialize transports based on config
	if config.EnableStdio {
		stdioTransport := NewStdioTransport(messageHandler)
		server.AddTransport(stdioTransport)
	}

	if config.EnableSSE {
		sseTransport := NewSSETransport(messageHandler)
		server.AddTransport(sseTransport)
	}

	// Register default capabilities
	server.capabilities = MCPServerCapabilities{
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
	}

	return server
}

func (s *MCPServer) registerMethodHandlers() {
	s.methodHandlers["resources/list"] = func(conn *MCPConnection, params json.RawMessage) (interface{}, error) {
		if s.resourceManager == nil {
			return nil, fmt.Errorf("resource manager not initialized")
		}
		return s.resourceManager.List(""), nil
	}

	// Register other handlers
	s.methodHandlers["resources/get"] = func(conn *MCPConnection, params json.RawMessage) (interface{}, error) {
		var request struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return s.resourceManager.Read(request.URI)
	}

	// Add more method handlers as needed
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
	Start() error
	Stop() error
	Send(msg MCPMessage) error
}

// MCPStdioTransport implements stdio transport
type MCPStdioTransport struct {
	handler   MCPMessageHandler
	decoder   *json.Decoder
	encoder   *json.Encoder
	logger    *logrus.Logger
	done      chan struct{}
	running   bool
	runningMu sync.RWMutex
}

func NewStdioTransport(handler MCPMessageHandler) *MCPStdioTransport {
	return &MCPStdioTransport{
		handler: handler,
		decoder: json.NewDecoder(os.Stdin),
		encoder: json.NewEncoder(os.Stdout),
		logger:  logrus.New(), // Add logger
		done:    make(chan struct{}),
	}
}

func (t *MCPStdioTransport) Start() error {
	t.runningMu.Lock()
	if t.running {
		t.runningMu.Unlock()
		return errors.New("transport already running")
	}
	t.running = true
	t.runningMu.Unlock()

	go t.readLoop()
	return nil
}

func (t *MCPStdioTransport) Stop() error {
	t.runningMu.Lock()
	defer t.runningMu.Unlock()

	if !t.running {
		return nil
	}

	close(t.done)
	t.running = false
	return nil
}

func (t *MCPStdioTransport) Send(msg MCPMessage) error {
	t.runningMu.RLock()
	defer t.runningMu.RUnlock()

	if !t.running {
		return errors.New("transport not running")
	}

	if err := t.encoder.Encode(msg); err != nil {
		t.logger.WithError(err).Error("Failed to encode message")
		return fmt.Errorf("failed to encode message: %w", err)
	}
	return nil
}

func (t *MCPStdioTransport) readLoop() {
	for {
		select {
		case <-t.done:
			return
		default:
			var msg MCPMessage
			if err := t.decoder.Decode(&msg); err != nil {
				if err == io.EOF {
					t.Stop()
					return
				}
				continue
			}

			if err := t.handler(msg); err != nil {
				// Log error but continue processing
				if t.logger != nil {
					t.logger.WithError(err).Error("Error handling message")
				}
			}
		}
	}
}

// MCPSSEConnection represents an SSE client connection
type MCPSSEConnection struct {
	ID     string
	Writer http.ResponseWriter
	Ctx    context.Context
	Cancel context.CancelFunc
}

// MCPSSETransport implements Server-Sent Events transport
type MCPSSETransport struct {
	handler MCPMessageHandler
	clients map[string]*MCPSSEConnection
	mu      sync.RWMutex
	logger  *logrus.Logger
}

func NewSSETransport(handler MCPMessageHandler) *MCPSSETransport {
	return &MCPSSETransport{
		handler: handler,
		clients: make(map[string]*MCPSSEConnection),
		logger:  logrus.New(),
	}
}

func (t *MCPSSETransport) Start() error {
	return nil // SSE transport is passive, starts with first connection
}

func (t *MCPSSETransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, client := range t.clients {
		client.Cancel()
	}
	t.clients = make(map[string]*MCPSSEConnection)
	return nil
}

func (t *MCPSSETransport) Send(msg MCPMessage) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	for _, client := range t.clients {
		select {
		case <-client.Ctx.Done():
			continue
		default:
			if f, ok := client.Writer.(http.Flusher); ok {
				fmt.Fprintf(client.Writer, "data: %s\n\n", data)
				f.Flush()
			}
		}
	}
	return nil
}

func (t *MCPSSETransport) HandleSSERequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx, cancel := context.WithCancel(r.Context())
	connID := uuid.New().String()

	conn := &MCPSSEConnection{
		ID:     connID,
		Writer: w,
		Ctx:    ctx,
		Cancel: cancel,
	}

	t.mu.Lock()
	t.clients[connID] = conn
	t.mu.Unlock()

	<-ctx.Done()

	t.mu.Lock()
	delete(t.clients, connID)
	t.mu.Unlock()
}

// Resource management
type MCPResource struct {
	URI        string          `json:"uri"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Content    string          `json:"content"`
	MimeType   string          `json:"mimeType,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Version    string          `json:"version"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	IsTemplate bool            `json:"isTemplate"`
}

// MCPResourceList represents a paginated list of resources
type MCPResourceList struct {
	Items      []MCPResource `json:"items"`
	TotalCount int           `json:"totalCount"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// MCPResourceListParams represents parameters for listing resources
type MCPResourceListParams struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Filter string `json:"filter,omitempty"`
}

// MCPResourceSubscription represents a resource subscription
type MCPResourceSubscription struct {
	ID       string   `json:"id"`
	Resource string   `json:"resource"`
	Events   []string `json:"events"`
}

// MCPResourceManager handles resource management operations
type MCPResourceManager struct {
	resources   map[string]MCPResource
	contents    map[string][]byte
	subscribers map[string]map[*MCPConnection][]string // resourceID -> connections -> events
	mu          sync.RWMutex
	logger      *logrus.Logger
}

type MCPResourceChange struct {
	ResourceID string          `json:"resourceId"`
	Type       string          `json:"type"`
	Resource   *MCPResource    `json:"resource,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Delta      json.RawMessage `json:"delta,omitempty"`
}

// MCPResourceNotification represents a resource change notification
type MCPResourceNotification struct {
	Type     string      `json:"type"` // "created", "updated", "deleted"
	Resource MCPResource `json:"resource"`
}

// MCPResourceValidationError represents resource validation errors
type MCPResourceValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// NewMCPResourceManager creates a new resource manager
func NewMCPResourceManager() *MCPResourceManager {
	return &MCPResourceManager{
		resources:   make(map[string]MCPResource),
		contents:    make(map[string][]byte),
		subscribers: make(map[string]map[*MCPConnection][]string),
	}
}

// ValidateResource validates a resource
func (rm *MCPResourceManager) ValidateResource(resource MCPResource) []MCPResourceValidationError {
	var errors []MCPResourceValidationError

	// Validate required fields
	if resource.URI == "" {
		errors = append(errors, MCPResourceValidationError{
			Field:   "uri",
			Message: "URI is required",
		})
	}

	if resource.Name == "" {
		errors = append(errors, MCPResourceValidationError{
			Field:   "name",
			Message: "Name is required",
		})
	}

	if resource.Type == "" {
		errors = append(errors, MCPResourceValidationError{
			Field:   "type",
			Message: "Type is required",
		})
	}

	// Validate MIME type format
	if resource.MimeType != "" {
		if !strings.Contains(resource.MimeType, "/") {
			errors = append(errors, MCPResourceValidationError{
				Field:   "mimeType",
				Message: "Invalid MIME type format",
			})
		}
	}

	return errors
}

// List returns resources matching the filter
func (rm *MCPResourceManager) List(filter string) []MCPResource {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]MCPResource, 0)
	for _, resource := range rm.resources {
		if filter == "" || strings.Contains(resource.Name, filter) {
			result = append(result, resource)
		}
	}
	return result
}

// Read returns the content of a resource
func (rm *MCPResourceManager) Read(id string) ([]byte, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	content, exists := rm.contents[id]
	if !exists {
		return nil, fmt.Errorf("resource not found: %s", id)
	}
	return content, nil
}

// UpdateResource updates an existing resource
func (rm *MCPResourceManager) UpdateResource(resource MCPResource, content []byte) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[resource.URI]; !exists {
		return fmt.Errorf("resource not found: %s", resource.URI)
	}

	// Validate resource
	if errors := rm.ValidateResource(resource); len(errors) > 0 {
		return fmt.Errorf("invalid resource: %v", errors)
	}

	// Update resource and content
	resource.UpdatedAt = time.Now()
	rm.resources[resource.URI] = resource
	rm.contents[resource.URI] = content

	// Broadcast update notification
	go rm.broadcastNotification(MCPResourceNotification{
		Type:     "updated",
		Resource: resource,
	})

	return nil
}

// CreateResource creates a new resource
func (rm *MCPResourceManager) CreateResource(resource MCPResource, content []byte) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[resource.URI]; exists {
		return fmt.Errorf("resource already exists: %s", resource.URI)
	}

	// Validate resource
	if errors := rm.ValidateResource(resource); len(errors) > 0 {
		return fmt.Errorf("invalid resource: %v", errors)
	}

	// Store resource and content
	resource.UpdatedAt = time.Now()
	rm.resources[resource.URI] = resource
	rm.contents[resource.URI] = content

	// Broadcast creation notification
	go rm.broadcastNotification(MCPResourceNotification{
		Type:     "created",
		Resource: resource,
	})

	return nil
}

// DeleteResource deletes a resource
func (rm *MCPResourceManager) DeleteResource(id string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	resource, exists := rm.resources[id]
	if !exists {
		return fmt.Errorf("resource not found: %s", id)
	}

	// Remove resource and content
	delete(rm.resources, id)
	delete(rm.contents, id)

	// Broadcast deletion notification
	go rm.broadcastNotification(MCPResourceNotification{
		Type:     "deleted",
		Resource: resource,
	})

	return nil
}

// CleanupConnection removes all subscriptions for a connection
func (rm *MCPResourceManager) CleanupConnection(conn *MCPConnection) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Remove connection from all resource subscriptions
	for resourceID := range rm.subscribers {
		delete(rm.subscribers[resourceID], conn)

		// If no subscribers left for this resource, clean up the map
		if len(rm.subscribers[resourceID]) == 0 {
			delete(rm.subscribers, resourceID)
		}
	}
}

// broadcastNotification sends notifications to all subscribed connections
func (rm *MCPResourceManager) broadcastNotification(notification MCPResourceNotification) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resourceID := notification.Resource.URI
	subscribers := rm.subscribers[resourceID]

	for conn, events := range subscribers {
		if containsEvent(events, notification.Type) {
			// Marshal notification to json.RawMessage
			params, err := json.Marshal(notification)
			if err != nil {
				rm.logger.Errorf("Failed to marshal notification: %v", err)
				continue
			}

			msg := MCPMessage{
				JsonRPC: "2.0",
				Method:  "resource/notification",
				Params:  params,
			}

			go func(c *MCPConnection, m MCPMessage) {
				if err := c.SendMessage(m); err != nil {
					rm.CleanupConnection(c)
				}
			}(conn, msg)
		}
	}
}

// Helper function to check if events slice contains a specific event
func containsEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}

// Subscribe adds a subscription for a resource
func (rm *MCPResourceManager) Subscribe(resourceID string, conn *MCPConnection, events []string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.resources[resourceID]; !exists {
		return fmt.Errorf("resource not found: %s", resourceID)
	}

	if _, exists := rm.subscribers[resourceID]; !exists {
		rm.subscribers[resourceID] = make(map[*MCPConnection][]string)
	}

	rm.subscribers[resourceID][conn] = events
	return nil
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
		if err := transport.Start(); err != nil {
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

type MessageHandler func(MCPMessage) (MCPMessage, error)

func (s *MCPServer) RegisterMessageHandler(handler MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
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

// ListResources handles the resources/list method
func (s *MCPServer) ListResources(params MCPResourceListParams) (*MCPResourceList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if params.Limit <= 0 {
		params.Limit = 50 // default limit
	}

	resources := s.resourceManager.List(params.Filter)

	start := 0
	if params.Cursor != "" {
		for i, r := range resources {
			if r.URI == params.Cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + params.Limit
	if end > len(resources) {
		end = len(resources)
	}

	var nextCursor string
	if end < len(resources) {
		nextCursor = resources[end-1].URI
	}

	return &MCPResourceList{
		Items:      resources[start:end],
		TotalCount: len(resources),
		NextCursor: nextCursor,
	}, nil
}

// ReadResource handles the resources/read method
func (s *MCPServer) ReadResource(id string) ([]byte, error) {
	return s.resourceManager.Read(id)
}

// ListResourceTemplates handles the resources/templates/list method
func (s *MCPServer) ListResourceTemplates() ([]MCPResource, error) {
	resources := s.resourceManager.List("")
	templates := make([]MCPResource, 0)

	for _, r := range resources {
		if r.IsTemplate {
			templates = append(templates, r)
		}
	}
	return templates, nil
}

// SubscribeToResource handles the resources/subscribe method
func (s *MCPServer) SubscribeToResource(conn *MCPConnection, subscription MCPResourceSubscription) error {
	return s.resourceManager.Subscribe(subscription.Resource, conn, subscription.Events)
}

// AddResourceCleanupHandler adds resource cleanup to connection handling
func (s *MCPServer) AddResourceCleanupHandler(conn *MCPConnection) {
	originalCancel := conn.Cancel
	conn.Cancel = func() {
		s.resourceManager.CleanupConnection(conn)
		originalCancel()
	}
}

// HandleResourceUpdate handles resource update requests
func (s *MCPServer) HandleResourceUpdate(resource MCPResource, content []byte) error {
	return s.resourceManager.UpdateResource(resource, content)
}

// HandleResourceCreate handles resource creation requests
func (s *MCPServer) HandleResourceCreate(resource MCPResource, content []byte) error {
	return s.resourceManager.CreateResource(resource, content)
}

// HandleResourceDelete handles resource deletion requests
func (s *MCPServer) HandleResourceDelete(id string) error {
	return s.resourceManager.DeleteResource(id)
}

func (s *MCPServer) AddTransport(transport MCPTransport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transports = append(s.transports, transport)
}
