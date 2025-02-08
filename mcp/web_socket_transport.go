package mcp

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Transport defines the interface for different transport mechanisms
type Transport interface {
	Start() error
	Stop() error
	HandleMessage(handler MessageHandler)
}

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
