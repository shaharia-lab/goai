package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SSEServer is the MCP server implementation using Server-Sent Events.
type SSEServer struct {
	*BaseServer
	clients      map[string]chan []byte
	clientsMutex sync.RWMutex
	address      string
	initialized  bool
	initMutex    sync.RWMutex
}

// NewSSEServer creates a new SSEServer.
func NewSSEServer(baseServer *BaseServer) *SSEServer {
	s := &SSEServer{
		BaseServer:   baseServer,
		clients:      make(map[string]chan []byte),
		clientsMutex: sync.RWMutex{},
		address:      baseServer.sseServerPort,
		initialized:  false,
		initMutex:    sync.RWMutex{},
	}

	// Set the concrete send methods for SSEServer
	s.sendResp = s.sendResponse
	s.sendErr = s.sendError
	s.sendNoti = s.sendNotification

	return s
}

// SetAddress allows setting the server's listening address.
func (s *SSEServer) SetAddress(address string) {
	s.address = address
}

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
		select {
		case clientChan <- jsonNotification:
		default:
			s.logger.Printf("Client message buffer full, dropping notification")
		}
	}
}

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

func (s *SSEServer) sendNotification(clientID string, method string, params interface{}) {
	if clientID == "" {
		s.broadcastNotification(method, params)
		return
	}
	//Otherwise, send to the specific client
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

// sendMessageToClient sends a message to a specific client.
func (s *SSEServer) sendMessageToClient(clientID string, message []byte) {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	if clientChan, ok := s.clients[clientID]; ok {
		select {
		case clientChan <- message: // Send without blocking
		default:
			s.logger.Printf("Client message buffer full, dropping message for: %s", clientID)
		}
	} else {
		s.logger.Printf("Client not found: %s", clientID)
	}
}

func (s *SSEServer) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("Received HTTP request: method=%s, path=%s, query=%s", r.Method, r.URL.Path, r.URL.RawQuery)

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

// handleSSEConnection establishes a new SSE connection.
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Each client gets a unique ID.
	clientID := uuid.NewString()
	messageChan := make(chan []byte, 10)

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

	endpointURL := fmt.Sprintf("http://%s/message?clientID=%s", r.Host, clientID)
	endpointEvent := fmt.Sprintf("event: endpoint\ndata: %s\n\n", endpointURL)
	if _, err := fmt.Fprint(w, endpointEvent); err != nil {
		s.logger.Printf("error sending endpoint data. Error: %v", err)
	}
	flusher.Flush()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-messageChan:
			event := fmt.Sprintf("event: message\ndata: %s\n\n", string(msg))
			if _, err := fmt.Fprint(w, event); err != nil {
				s.logger.Printf("error sending sse data. Error: %v", err)
			}
			flusher.Flush()
		case <-time.After(pingInterval):
			// Keep-alive.
			if _, err := fmt.Fprint(w, ":ping\n\n"); err != nil {
				s.logger.Printf("error sending keepalive data. Error: %v", err)
			}
			flusher.Flush()
		}
	}
}

// handleClientMessage processes incoming messages from clients (POST to /message).
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
		// Check initialization state for requests
		s.initMutex.RLock()
		initialized := s.initialized
		s.initMutex.RUnlock()

		if request.Method != "initialize" && !initialized {
			s.sendError(clientID, request.ID, -32000, "Server not initialized", nil)
			return
		}
		s.handleRequest(clientID, &request)
		return
	}

	var notification Notification
	if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
		s.initMutex.RLock()
		initialized := s.initialized
		s.initMutex.RUnlock()

		if notification.Method == "notifications/initialized" {
			s.initMutex.Lock()
			s.initialized = true
			s.initMutex.Unlock()
		} else if !initialized {
			s.logger.Printf("Received notification before 'initialized': %s", notification.Method)
			return
		}
		s.handleNotification(clientID, &notification)
		return
	}

	s.sendError(clientID, nil, -32600, "Invalid Request", nil)
}

// Run starts the MCP SSEServer, listening for incoming HTTP connections.
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

	// Shutdown handling
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
