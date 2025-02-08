package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net"
	"sync"
)

// Connection represents an active client connection
type Connection struct {
	ID     string
	conn   net.Conn
	writer *json.Encoder
	Cancel func()
	mu     sync.Mutex
}

// SendMessage sends a message to the client
func (c *Connection) SendMessage(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writer == nil {
		return errors.New("connection writer is not initialized")
	}

	if err := c.writer.Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	return nil
}

// NewConnection creates a new connection instance
func NewConnection(conn net.Conn, cancel func()) *Connection {
	return &Connection{
		ID:     uuid.New().String(),
		conn:   conn,
		writer: json.NewEncoder(conn),
		Cancel: cancel,
	}
}
