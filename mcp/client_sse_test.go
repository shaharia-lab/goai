package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type mockSSEServer struct {
	server      *httptest.Server
	connections map[string]chan string
	mu          sync.RWMutex
	t           *testing.T
}

func newMockSSEServer(t *testing.T) *mockSSEServer {
	mock := &mockSSEServer{
		connections: make(map[string]chan string),
		t:           t,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/events", mock.handleSSE)
	mux.HandleFunc("/message", mock.handleMessage)

	mock.server = httptest.NewServer(mux)
	return mock
}

func (m *mockSSEServer) close() {
	m.server.Close()
}

func (m *mockSSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	clientID := fmt.Sprintf("test-client-%d", time.Now().UnixNano())

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	msgChan := make(chan string, 10)
	m.mu.Lock()
	m.connections[clientID] = msgChan
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.connections, clientID)
		close(msgChan)
		m.mu.Unlock()
	}()

	// Send initial endpoint message
	messageEndpoint := fmt.Sprintf("%s/message?clientID=%s", m.server.URL, clientID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", messageEndpoint)
	flusher.Flush()

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			fmt.Fprint(w, ":ping\n\n")
			flusher.Flush()
		case msg := <-msgChan:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (m *mockSSEServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID := r.URL.Query().Get("clientID")
	if clientID == "" {
		http.Error(w, "Missing clientID", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var request Request
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	m.mu.RLock()
	msgChan, exists := m.connections[clientID]
	m.mu.RUnlock()

	if !exists {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}

	// Handle different request methods
	var response interface{}
	switch request.Method {
	case "initialize":
		response = InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: Capabilities{
				Logging:   CapabilitiesLogging{},
				Prompts:   CapabilitiesPrompts{ListChanged: true},
				Resources: CapabilitiesResources{ListChanged: true, Subscribe: true},
				Tools:     CapabilitiesTools{ListChanged: true},
			},
			ServerInfo: ServerInfo{
				Name:    "test-server",
				Version: "1.0.0",
			},
		}
	case "tools/list":
		response = ListToolsResult{
			Tools: []Tool{
				{
					Name:        "test-tool",
					Description: "A test tool",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`),
				},
			},
		}
	case "notifications/initialized":
		w.WriteHeader(http.StatusOK)
		return
	default:
		http.Error(w, "Method not found", http.StatusNotFound)
		return
	}

	if request.ID != nil {
		jsonResponse := Response{
			JSONRPC: "2.0",
			ID:      request.ID,
			Result:  response,
		}
		responseBytes, _ := json.Marshal(jsonResponse)
		msgChan <- string(responseBytes)
	}

	w.WriteHeader(http.StatusOK)
}

func TestConnectionFlow(t *testing.T) {
	mockServer := newMockSSEServer(t)
	defer mockServer.close()

	client := NewSSEClient(SSEClientConfig{
		URL:           mockServer.server.URL + "/events",
		RetryDelay:    time.Second,
		MaxRetries:    1,
		ClientName:    "test-client",
		ClientVersion: "1.0.0",
	})

	// Test connection
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Verify connection state
	client.mu.RLock()
	state := client.state
	initialized := client.initialized
	client.mu.RUnlock()

	if state != Connected {
		t.Errorf("Expected client state to be Connected, got %v", state)
	}

	if !initialized {
		t.Error("Expected client to be initialized")
	}

	client.Close()
}

func TestToolsList(t *testing.T) {
	mockServer := newMockSSEServer(t)
	defer mockServer.close()

	client := NewSSEClient(SSEClientConfig{
		URL:           mockServer.server.URL + "/events",
		RetryDelay:    time.Second,
		MaxRetries:    1,
		ClientName:    "test-client",
		ClientVersion: "1.0.0",
	})

	// Connect first
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Test ListTools
	tools, err := client.ListTools()
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "test-tool" {
		t.Errorf("Expected tool name test-tool, got %s", tools[0].Name)
	}

	if tools[0].Description != "A test tool" {
		t.Errorf("Expected tool description 'A test tool', got %s", tools[0].Description)
	}

	client.Close()
}
