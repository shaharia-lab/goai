package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SSEServer represents an MCP server that communicates via Server-Sent Events (SSE).
type SSEServer struct {
	protocolVersion    string
	clientCapabilities map[string]any
	logger             *log.Logger
	ServerInfo         struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	capabilities            map[string]any
	tools                   map[string]Tool // Store available tools
	supportsToolListChanged bool
	address                 string // Add address field

	clients      map[string]chan []byte // Map client IDs to their message channels.
	clientsMutex sync.RWMutex           // Protects access to the clients map.
}

// NewSSEServer creates and initializes a new MCP server instance for SSE communication.
func NewSSEServer() *SSEServer {
	logger := log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)

	return &SSEServer{
		protocolVersion: "2024-11-05",
		logger:          logger,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "Resource-SSEServer-Example",
			Version: "0.1.0",
		},
		capabilities: map[string]any{
			"tools": map[string]any{ // Only tools capability for now
				"listChanged": true,
			},
			"logging": map[string]any{},
		},
		tools:                   make(map[string]Tool),
		supportsToolListChanged: false,
		address:                 ":8080", // Default address

		clients:      make(map[string]chan []byte),
		clientsMutex: sync.RWMutex{},
	}
}

// AddTool adds a tool to the server, mirroring the StdIOServer implementation.
func (s *SSEServer) AddTool(tool Tool) {
	s.tools[tool.Name] = tool
	if s.supportsToolListChanged {
		s.SendToolListChangedNotification()
	}
}

// SetAddress allows setting the server's listening address.
func (s *SSEServer) SetAddress(address string) {
	s.address = address
}

// SendToolListChangedNotification broadcasts the tool list changed notification.
func (s *SSEServer) SendToolListChangedNotification() {
	s.broadcastNotification("notifications/tools/list_changed", nil) // Broadcast to all clients
}

// sendResponse sends a JSON-RPC response to a specific client.
func (s *SSEServer) sendResponse(clientID string, id *json.RawMessage, result interface{}, err *Error) {
	response := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	jsonResponse, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		s.logger.Printf("Error marshalling response: %v", marshalErr)
		s.sendError(clientID, id, -32603, "Internal error: failed to marshal response", nil)
		return
	}

	s.sendMessageToClient(clientID, jsonResponse)
}

// sendError sends a JSON-RPC error response to a specific client.
func (s *SSEServer) sendError(clientID string, id *json.RawMessage, code int, message string, data interface{}) {
	errorResponse := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	jsonErrorResponse, err := json.Marshal(errorResponse)
	if err != nil {
		s.logger.Printf("Error marshaling error response: %v", err)
		return
	}
	s.sendMessageToClient(clientID, jsonErrorResponse)
}

// sendNotification sends a JSON-RPC notification to a specific client.  Note the clientID.
func (s *SSEServer) sendNotification(clientID string, method string, params interface{}) {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			s.logger.Printf("Error marshaling notification parameters: %v", err)
			return
		}
		notification.Params = json.RawMessage(paramsBytes)
	}

	jsonNotification, err := json.Marshal(notification)
	if err != nil {
		s.logger.Printf("Error marshaling notification message: %v", err)
		return
	}

	s.sendMessageToClient(clientID, jsonNotification)
}

// broadcastNotification sends a notification to all connected clients.
func (s *SSEServer) broadcastNotification(method string, params interface{}) {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			s.logger.Printf("Error marshaling notification parameters: %v", err)
			return
		}
		notification.Params = json.RawMessage(paramsBytes)
	}

	jsonNotification, err := json.Marshal(notification)
	if err != nil {
		s.logger.Printf("Error marshaling notification message: %v", err)
		return
	}

	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	for _, clientChan := range s.clients {
		clientChan <- jsonNotification // Send without blocking; clients should buffer
	}
}

// LogMessage sends a log message notification.  Similar to StdIOServer but broadcasts.
func (s *SSEServer) LogMessage(level LogLevel, loggerName string, data interface{}) {
	// For simplicity, we log to stderr *and* send as a notification.  A real implementation
	// might have configurable log routing.
	if logLevelSeverity[level] <= logLevelSeverity[LogLevelInfo] { //  Basic filtering.
		s.logger.Printf("[%s] %s: %+v", level, loggerName, data) // Log to server's stderr
	}
	params := LogMessageParams{
		Level:  level,
		Logger: loggerName,
		Data:   data,
	}
	s.broadcastNotification("notifications/message", params) // Broadcast to *all* clients.
}

// handleHTTPRequest handles incoming HTTP requests, including SSE setup and message posting.
func (s *SSEServer) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("Received HTTP request: method=%s, path=%s, query=%s", r.Method, r.URL.Path, r.URL.RawQuery) // Log the full URL

	// CORS handling (Allow all origins - adjust as needed for production).
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization") // Add Authorization
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.URL.Path {
	case "/events":
		s.handleSSEConnection(w, r)
	case "/message": //  The single endpoint for receiving client messages.
		s.handleClientMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleSSEConnection establishes a new SSE connection with a client.
func (s *SSEServer) handleSSEConnection(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Set the headers related to event streaming.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS

	// Each client gets a unique ID.
	clientID := uuid.NewString()
	messageChan := make(chan []byte, 10) // Buffered channel

	s.clientsMutex.Lock()
	s.clients[clientID] = messageChan
	s.clientsMutex.Unlock()

	defer func() {
		s.clientsMutex.Lock()
		delete(s.clients, clientID)
		close(messageChan)
		s.clientsMutex.Unlock()
		s.logger.Printf("Client disconnected: %s", clientID)
	}()

	s.logger.Printf("Client connected: %s", clientID)

	// Send the initial 'endpoint' event.  Critically, this includes the client ID.
	endpointURL := fmt.Sprintf("http://%s/message?clientID=%s", r.Host, clientID)
	// Send *just* the URL string, not a JSON object.
	endpointEvent := fmt.Sprintf("event: endpoint\ndata: %s\n\n", endpointURL) // SIMPLIFIED
	if _, err := fmt.Fprint(w, endpointEvent); err != nil {
		s.logger.Printf("error sending endpoint data. Error: %v", err)
	}
	flusher.Flush()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			return
		case msg := <-messageChan:
			// Got a message to send to the client.
			event := fmt.Sprintf("event: message\ndata: %s\n\n", string(msg))
			if _, err := fmt.Fprint(w, event); err != nil {
				s.logger.Printf("error sending sse data. Error: %v", err)
			}
			flusher.Flush()
		case <-time.After(30 * time.Second):
			// Keep-alive.
			if _, err := fmt.Fprint(w, ":ping\n\n"); err != nil {
				s.logger.Printf("error sending keepalive data. Error: %v", err)
			}
			flusher.Flush()
		}
	}
}

func (s *SSEServer) sendMessageToClient(clientID string, message []byte) {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	if clientChan, ok := s.clients[clientID]; ok {
		select {
		case clientChan <- message: // Send without blocking

		default:
			s.logger.Printf("Client message buffer full, dropping message for: %s", clientID)
			// Consider closing the connection or implementing backpressure.
		}
	} else {
		s.logger.Printf("Client not found: %s", clientID)
	}
}

// handleClientMessage processes incoming messages from clients (received via POST requests).
func (s *SSEServer) handleClientMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID := r.URL.Query().Get("clientID") // Get the client ID from the query parameter.
	if clientID == "" {
		http.Error(w, "Missing clientID", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	s.logger.Printf("Received message from client %s: %s", clientID, string(body))

	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		s.sendError(clientID, nil, -32700, "Parse error", nil)
		return
	}

	var request Request
	if err := json.Unmarshal(raw, &request); err == nil && request.Method != "" && request.ID != nil {
		s.handleRequest(clientID, &request) // Pass the client ID to the request handler.
		return
	}

	var notification Notification
	if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
		s.handleNotification(clientID, &notification) // Pass client ID to notification handler.
		return
	}

	s.sendError(clientID, nil, -32600, "Invalid Request", nil)
}

// handleRequest processes a JSON-RPC request from a client.  Note the clientID parameter.
func (s *SSEServer) handleRequest(clientID string, request *Request) {
	s.logger.Printf("Received request from client %s: method=%s, id=%v", clientID, request.Method, request.ID)

	switch request.Method {
	case "initialize":
		var params InitializeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		if !strings.HasPrefix(params.ProtocolVersion, "2024-11") {
			s.sendError(clientID, request.ID, -32602, "Unsupported protocol version", map[string][]string{"supported": {"2024-11-05"}})
			return
		}

		s.clientCapabilities = params.Capabilities
		result := InitializeResult{
			ProtocolVersion: s.protocolVersion,
			Capabilities:    s.capabilities,
			ServerInfo:      s.ServerInfo,
		}

		// Check and set if server supports tool list changed
		if toolCaps, ok := s.capabilities["tools"].(map[string]any); ok {
			if listChanged, ok := toolCaps["listChanged"].(bool); ok && listChanged {
				s.supportsToolListChanged = true
			}
		}

		s.sendResponse(clientID, request.ID, result, nil)

	case "ping":
		s.sendResponse(clientID, request.ID, map[string]interface{}{}, nil)

	case "tools/list":
		var toolList []Tool
		for _, tool := range s.tools {
			toolList = append(toolList, tool) // Append the tool directly.
		}
		result := ListToolsResult{Tools: toolList}
		s.sendResponse(clientID, request.ID, result, nil)

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}
		tool, ok := s.tools[params.Name]
		if !ok {
			s.sendError(clientID, request.ID, -32602, "Unknown tool", map[string]string{"tool": params.Name})
			return
		}
		// VERY basic argument validation, just checking required fields.
		var input map[string]interface{}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			s.sendError(clientID, request.ID, -32602, "Invalid tool arguments", nil)
			return
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			s.sendError(clientID, request.ID, -32603, "Internal error: invalid tool schema", nil)
			return
		}

		required, hasRequired := schema["required"].([]interface{}) // Use type assertion
		if hasRequired {
			for _, reqField := range required {
				reqFieldName, ok := reqField.(string)
				if !ok {
					continue
				}
				if _, present := input[reqFieldName]; !present {
					s.sendError(clientID, request.ID, -32602, "Missing required argument", map[string]string{"argument": reqFieldName})
					return
				}
			}
		}

		result := CallToolResult{IsError: false}
		if params.Name == "get_weather" {
			location, _ := input["location"].(string) //  We checked presence above
			result.Content = []ToolResultContent{{
				Type: "text",
				Text: fmt.Sprintf("Weather in %s: Sunny, 72°F", location), //  Fake data
			}}

		} else if params.Name == "greet" { //From main.go example
			name, _ := input["name"].(string)
			result.Content = []ToolResultContent{
				{Type: "text", Text: fmt.Sprintf("Hello, %s!", name)},
			}
		} else if params.Name == "another_tool" {
			value, _ := input["value"].(float64)
			result.Content = []ToolResultContent{{
				Type: "text",
				Text: fmt.Sprintf("The value is %f", value),
			}}

		}
		s.sendResponse(clientID, request.ID, result, nil)

	default:
		s.sendError(clientID, request.ID, -32601, "Method not found", nil)
	}
}

// handleNotification processes a JSON-RPC notification from a client.
func (s *SSEServer) handleNotification(clientID string, notification *Notification) {
	s.logger.Printf("Received notification from client %s: method=%s", clientID, notification.Method)
	switch notification.Method {
	case "notifications/initialized": // The client confirms it's initialized.
		// We don't need to *do* anything here, but it's good to log.
		s.logger.Printf("Client %s initialized.", clientID)
	case "notifications/cancelled":
		var cancelParams struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if err := json.Unmarshal(notification.Params, &cancelParams); err == nil {
			s.logger.Printf("Cancellation requested for ID %s from client %s: %s", string(cancelParams.RequestID), clientID, cancelParams.Reason)
			// In a real implementation, you'd have a way to map request IDs to
			// ongoing operations and cancel them.  This is a placeholder.
		}

	default:
		s.logger.Printf("Unhandled notification from client %s: %s", clientID, notification.Method) // Log but don't send error.
	}
}

// Run starts the MCP server, listening for incoming HTTP connections.
func (s *SSEServer) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHTTPRequest)

	// CORS middleware (wrap the main handler)
	corsHandler := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow all origins for development.  Adjust as needed for production.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			h.ServeHTTP(w, r)
		})
	}

	server := &http.Server{
		Addr:    s.address,        // Use the configured address.
		Handler: corsHandler(mux), // Wrap the mux with the CORS handler.
	}

	s.LogMessage(LogLevelInfo, "startup", fmt.Sprintf("Starting SSE server on %s", s.address))

	// Shutdown handling, inspired by the Go blog's graceful shutdown example.
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Println("Context cancelled, shutting down server...")
		// Give the server some time to shut down gracefully.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("error during server shutdown: %w", err)
		}
		s.logger.Println("Server gracefully shut down.")
		return ctx.Err()
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}
