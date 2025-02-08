// sse_transport.go
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type SSETransport struct {
	clients        map[string]*SSEClient
	mu             sync.RWMutex
	handler        MessageHandler
	allowedOrigins []string
}

type SSEClient struct {
	ID      string
	writer  http.ResponseWriter
	flusher http.Flusher
	closed  chan struct{}
}

func NewSSETransport(allowedOrigins []string) *SSETransport {
	return &SSETransport{
		clients:        make(map[string]*SSEClient),
		allowedOrigins: allowedOrigins,
	}
}

func (t *SSETransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check origin
	if !t.checkOrigin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Verify it's an SSE request
	if !t.isSSERequest(r) {
		http.Error(w, "SSE Only", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Setup SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client := &SSEClient{
		ID:      generateClientID(),
		writer:  w,
		flusher: flusher,
		closed:  make(chan struct{}),
	}

	t.registerClient(client)
	defer t.unregisterClient(client.ID)

	// Keep connection alive
	<-client.closed
}

func (t *SSETransport) SendMessage(clientID string, msg Message) error {
	t.mu.RLock()
	client, exists := t.clients[clientID]
	t.mu.RUnlock()

	if !exists {
		return fmt.Errorf("client %s not found", clientID)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send SSE formatted message
	fmt.Fprintf(client.writer, "data: %s\n\n", data)
	client.flusher.Flush()
	return nil
}

func (t *SSETransport) HandleMessage(handler MessageHandler) {
	t.mu.Lock()
	t.handler = handler
	t.mu.Unlock()
}

func (t *SSETransport) Start() error {
	return nil
}

func (t *SSETransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, client := range t.clients {
		close(client.closed)
	}
	t.clients = make(map[string]*SSEClient)
	return nil
}

func (t *SSETransport) registerClient(client *SSEClient) {
	t.mu.Lock()
	t.clients[client.ID] = client
	t.mu.Unlock()
}

func (t *SSETransport) unregisterClient(clientID string) {
	t.mu.Lock()
	if client, exists := t.clients[clientID]; exists {
		close(client.closed)
		delete(t.clients, clientID)
	}
	t.mu.Unlock()
}

func (t *SSETransport) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, allowed := range t.allowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func (t *SSETransport) isSSERequest(r *http.Request) bool {
	return r.Header.Get("Accept") == "text/event-stream"
}
