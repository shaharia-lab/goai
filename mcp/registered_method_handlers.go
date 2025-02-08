package mcp

import (
	"encoding/json"
	"sync"
)

// MethodHandler defines the function signature for method handlers
type MethodHandler func(conn *Connection, params json.RawMessage) (interface{}, error)

// RegisteredHandlers stores all registered method handlers
type RegisteredHandlers struct {
	handlers map[string]MethodHandler
	mu       sync.RWMutex
}

func NewRegisteredHandlers() *RegisteredHandlers {
	return &RegisteredHandlers{
		handlers: make(map[string]MethodHandler),
	}
}

func (h *RegisteredHandlers) Register(method string, handler MethodHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[method] = handler
}

func (h *RegisteredHandlers) Get(method string) (MethodHandler, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	handler, ok := h.handlers[method]
	return handler, ok
}

func (s *Server) handleInitialize(conn *Connection, params json.RawMessage) (interface{}, error) {
	var initParams InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, NewMCPError(ErrorCodeInvalidParams, "Invalid initialization parameters", nil)
	}

	if initParams.ProtocolVersion != "1.0" {
		return nil, NewMCPError(ErrorCodeInvalidParams, "Unsupported protocol version", nil)
	}

	response := map[string]interface{}{
		"serverInfo": map[string]string{
			"name":            "goai-mcp",
			"version":         s.version.String(),
			"protocolVersion": "1.0",
		},
		"capabilities": s.capabilities,
	}

	// Send initialized notification AFTER response
	go func() {
		initializedNotification, _ := NewRequest(nil, "initialized", nil)
		if err := conn.SendMessage(*initializedNotification); err != nil {
			s.logger.WithError(err).Error("Failed to send initialized notification")
		}
	}()

	return response, nil
}
