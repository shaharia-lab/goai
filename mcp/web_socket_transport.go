package mcp

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type MessageHandler func(Message)

// WebSocketTransport implements websocket-based transport
type WebSocketTransport struct {
	upgrader websocket.Upgrader
	handler  MessageHandler
	mu       sync.RWMutex
	conns    map[*websocket.Conn]bool
}

func NewWebSocketTransport(allowedOrigins []string) *WebSocketTransport {
	return &WebSocketTransport{
		upgrader: websocket.Upgrader{
			CheckOrigin: makeOriginChecker(allowedOrigins),
		},
		conns: make(map[*websocket.Conn]bool),
	}
}

func (t *WebSocketTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	t.mu.Lock()
	t.conns[conn] = true
	t.mu.Unlock()

	go t.handleConnection(conn)
}

func (t *WebSocketTransport) handleConnection(conn *websocket.Conn) {
	defer func() {
		t.mu.Lock()
		delete(t.conns, conn)
		t.mu.Unlock()
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if t.handler != nil {
			t.handler(msg)
		}
	}
}

// Add this function to web_socket_transport.go
func makeOriginChecker(allowedOrigins []string) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, allowed := range allowedOrigins {
			if allowed == "*" || allowed == origin {
				return true
			}
		}
		return false
	}
}

func (t *WebSocketTransport) Start() error {
	// Nothing specific needed for websocket start
	// The actual handling happens in ServeHTTP
	return nil
}

func (t *WebSocketTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Close all active connections
	for conn := range t.conns {
		if err := conn.Close(); err != nil {
			// Continue closing others even if one fails
			continue
		}
		delete(t.conns, conn)
	}
	return nil
}

func (t *WebSocketTransport) HandleMessage(handler MessageHandler) {
	t.mu.Lock()
	t.handler = handler
	t.mu.Unlock()
}
