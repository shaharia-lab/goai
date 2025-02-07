// Package goai provides an implementation of the Model Context Protocol (MCP)
// for building extensible AI tool servers.
package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"sync"
	"time"
)

// MCPContentItem represents a content item in a tool response
type MCPContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPTool represents a tool definition
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPToolInput represents input parameters for tool execution
type MCPToolInput struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// MCPToolResponse represents the response from tool execution
type MCPToolResponse struct {
	Content []MCPContentItem `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// MCPHandlers provides access to individual HTTP handlers
type MCPHandlers struct {
	ListTools http.HandlerFunc
	ToolCall  http.HandlerFunc
}

// MCPServerConfig holds configuration for the MCP server
type MCPServerConfig struct {
	Logger               *logrus.Logger // Optional: Logger instance
	Tracer               trace.Tracer   // Optional: OpenTelemetry tracer
	EnableMetrics        bool
	MaxToolExecutionTime time.Duration
}

// DefaultMCPServerConfig returns a default configuration with no logging or tracing
func DefaultMCPServerConfig() MCPServerConfig {
	return MCPServerConfig{
		MaxToolExecutionTime: 30 * time.Second,
	}
}

// MCPToolExecutor defines the interface that all MCP tools must implement.
// Implementations must be thread-safe and handle context cancellation.
//
// Example implementation:
//
//	type MyTool struct{}
//
//	func (t *MyTool) GetDefinition() MCPTool {
//	    return MCPTool{
//	        Name:        "my_tool",
//	        Description: "Does something useful",
//	        Version:     "1.0.0",
//	        InputSchema: json.RawMessage(`{
//	            "type": "object",
//	            "properties": {
//	                "param": {"type": "string"}
//	            }
//	        }`),
//	    }
//	}
//
//	func (t *MyTool) Execute(ctx context.Context, input json.RawMessage) (MCPToolResponse, error) {
//	    var params struct {
//	        Param string `json:"param"`
//	    }
//	    if err := json.Unmarshal(input, &params); err != nil {
//	        return MCPToolResponse{}, err
//	    }
//	    return MCPToolResponse{
//	        Content: []MCPContentItem{{
//	            Type: "text",
//	            Text: fmt.Sprintf("Processed: %s", params.Param),
//	        }},
//	    }, nil
//	}
type MCPToolExecutor interface {
	GetDefinition() MCPTool
	Execute(ctx context.Context, input json.RawMessage) (MCPToolResponse, error)
}

// MCPToolRegistry manages the registration and retrieval of MCP tools
type MCPToolRegistry struct {
	tools map[string]MCPToolExecutor
	mu    sync.RWMutex
}

// MCPServer represents an MCP server
type MCPServer struct {
	registry *MCPToolRegistry
	config   MCPServerConfig
	shutdown chan struct{}
	wg       sync.WaitGroup // Track ongoing requests
	server   *http.Server   // Optional HTTP server reference
}

// WithHTTPServer is an optional method to set HTTP server reference for shutdown
func (s *MCPServer) WithHTTPServer(srv *http.Server) *MCPServer {
	s.server = srv
	return s
}

// Shutdown performs a graceful shutdown of the MCP server.
// HTTP server shutdown is only performed if WithHTTPServer was called.
func (s *MCPServer) Shutdown(ctx context.Context) error {
	// Signal shutdown
	close(s.shutdown)

	// If no HTTP server is set, just wait for ongoing requests
	if s.server == nil {
		s.wg.Wait()
		return nil
	}

	// If HTTP server exists, perform full shutdown
	return s.server.Shutdown(ctx)
}

// isShuttingDown checks if server is in shutdown mode
func (s *MCPServer) isShuttingDown() bool {
	select {
	case <-s.shutdown:
		return true
	default:
		return false
	}
}

// wrapHandler wraps an http.HandlerFunc to track ongoing requests
func (s *MCPServer) wrapHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.wg.Add(1)
		defer s.wg.Done()
		h(w, r)
	}
}

// NewMCPToolRegistry creates a new tool registry
//
// Example:
//
//	registry := goai.NewMCPToolRegistry()
//	myTool := &MyTool{}
//	err := registry.Register(myTool)
func NewMCPToolRegistry() *MCPToolRegistry {
	return &MCPToolRegistry{
		tools: make(map[string]MCPToolExecutor),
	}
}

// Register adds a tool to the registry.
//
// Example:
//
//	registry := goai.NewMCPToolRegistry()
//	myTool := &MyTool{}
//	if err := registry.Register(myTool); err != nil {
//	    log.Fatal(err)
//	}
func (r *MCPToolRegistry) Register(tool MCPToolExecutor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	def := tool.GetDefinition()
	if _, exists := r.tools[def.Name]; exists {
		return fmt.Errorf("tool %s already registered", def.Name)
	}

	r.tools[def.Name] = tool
	return nil
}

// Get retrieves a tool from the registry by name.
//
// Example:
//
//	tool, err := registry.Get(ctx, "my_tool")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (r *MCPToolRegistry) Get(ctx context.Context, name string) (MCPToolExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	return tool, nil
}

// ListTools returns all registered tools' definitions.
//
// Example:
//
//	tools := registry.ListTools(ctx)
//	for _, tool := range tools {
//	    fmt.Printf("Tool: %s (%s)\n", tool.Name, tool.Description)
//	}
func (r *MCPToolRegistry) ListTools(ctx context.Context) []MCPTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]MCPTool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool.GetDefinition())
	}
	return tools
}

// NewMCPServer creates a new MCP server with the given registry and configuration.
//
// Example with minimal configuration:
//
//	config := goai.DefaultMCPServerConfig()
//	server := goai.NewMCPServer(registry, config)
//
// Example with full configuration including logging and tracing:
//
//	config := goai.MCPServerConfig{
//	    Logger:              logrus.New(),
//	    Tracer:             otel.Tracer("my-mcp-server"),
//	    MaxToolExecutionTime: 30 * time.Second,
//	}
//	server := goai.NewMCPServer(registry, config)
func NewMCPServer(registry *MCPToolRegistry, config MCPServerConfig) *MCPServer {
	return &MCPServer{
		registry: registry,
		config:   config,
		shutdown: make(chan struct{}),
	}
}

// HandleMCPListTools handles the tools/list endpoint.
// It returns a JSON response containing all registered tools.
func (s *MCPServer) HandleMCPListTools(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Optional tracing
	if s.config.Tracer != nil {
		var span trace.Span
		ctx, span = s.config.Tracer.Start(ctx, "GoAI_MCP_ListTools")
		defer span.End()
	}

	tools := s.registry.ListTools(ctx)

	response := struct {
		Tools []MCPTool `json:"tools"`
	}{
		Tools: tools,
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.handleError(w, err, http.StatusInternalServerError)
		return
	}
}

// HandleMCPToolCall handles the tools/call endpoint.
// It executes the requested tool and returns the response.
func (s *MCPServer) HandleMCPToolCall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Optional tracing
	if s.config.Tracer != nil {
		var span trace.Span
		ctx, span = s.config.Tracer.Start(ctx, "GoAI_MCP_ToolCall")
		defer span.End()
	}

	w.Header().Set("Content-Type", "application/json")

	var input MCPToolInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		s.handleError(w, err, http.StatusBadRequest)
		return
	}

	tool, err := s.registry.Get(ctx, input.Name)
	if err != nil {
		s.handleError(w, err, http.StatusNotFound)
		return
	}

	response, err := tool.Execute(ctx, input.Arguments)
	if err != nil {
		// Optional logging
		if s.config.Logger != nil {
			s.config.Logger.WithError(err).WithField("tool", input.Name).Error("Tool execution failed")
		}
		response = MCPToolResponse{
			IsError: true,
			Content: []MCPContentItem{{
				Type: "text",
				Text: err.Error(),
			}},
		}
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.handleError(w, err, http.StatusInternalServerError)
	}
}

// GetMCPHandlers returns the individual HTTP handlers that can be used with any router.
//
// Example with Chi router:
//
//	server := goai.NewMCPServer(registry, config)
//	handlers := server.GetMCPHandlers()
//
//	r := chi.NewRouter()
//	r.Get("/tools/list", handlers.ListTools)
//	r.Post("/tools/call", handlers.ToolCall)
func (s *MCPServer) GetMCPHandlers() MCPHandlers {
	return MCPHandlers{
		ListTools: s.wrapHandler(s.HandleMCPListTools),
		ToolCall:  s.wrapHandler(s.HandleMCPToolCall),
	}
}

// AddMCPHandlers adds MCP handlers to an existing http.ServeMux.
//
// Example:
//
//	mux := http.NewServeMux()
//	server.AddMCPHandlers(ctx, mux)
//	http.ListenAndServe(":8080", mux)
func (s *MCPServer) AddMCPHandlers(ctx context.Context, mux *http.ServeMux) {
	handlers := s.GetMCPHandlers()
	mux.HandleFunc("/tools/list", handlers.ListTools)
	mux.HandleFunc("/tools/call", handlers.ToolCall)
}

// handleError is an internal helper method for handling HTTP errors
func (s *MCPServer) handleError(w http.ResponseWriter, err error, status int) {
	// Optional logging
	if s.config.Logger != nil {
		s.config.Logger.WithError(err).Error("MCP error")
	}
	http.Error(w, err.Error(), status)
}
