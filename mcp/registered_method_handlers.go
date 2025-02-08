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

// Standard method handlers
func (s *Server) handleInitialize(conn *Connection, params json.RawMessage) (interface{}, error) {
	var initParams struct {
		ClientInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}

	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"serverInfo": map[string]string{
			"name":    "goai-mcp",
			"version": s.version.String(),
		},
		"capabilities": s.capabilities,
	}, nil
}
