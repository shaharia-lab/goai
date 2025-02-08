package mcp

import (
	"context"
	"sync"
)

// CancellationToken represents a cancellable operation
type CancellationToken struct {
	ID     string
	ctx    context.Context
	cancel context.CancelFunc
}

// CancellationManager manages cancellation tokens
type CancellationManager struct {
	tokens map[string]*CancellationToken
	mu     sync.RWMutex
}

// NewCancellationManager creates a new cancellation manager
func NewCancellationManager() *CancellationManager {
	return &CancellationManager{
		tokens: make(map[string]*CancellationToken),
	}
}

// CreateToken creates a new cancellation token
func (cm *CancellationManager) CreateToken(id string) *CancellationToken {
	ctx, cancel := context.WithCancel(context.Background())
	token := &CancellationToken{
		ID:     id,
		ctx:    ctx,
		cancel: cancel,
	}

	cm.mu.Lock()
	cm.tokens[id] = token
	cm.mu.Unlock()

	return token
}

// CancelToken cancels an operation by token ID
func (cm *CancellationManager) CancelToken(id string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if token, exists := cm.tokens[id]; exists {
		token.cancel()
		delete(cm.tokens, id)
		return true
	}
	return false
}

// RemoveToken removes a token without cancelling
func (cm *CancellationManager) RemoveToken(id string) {
	cm.mu.Lock()
	delete(cm.tokens, id)
	cm.mu.Unlock()
}

// GetToken retrieves a token by ID
func (cm *CancellationManager) GetToken(id string) (*CancellationToken, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	token, exists := cm.tokens[id]
	return token, exists
}
