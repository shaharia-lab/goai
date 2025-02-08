// sampling.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type SamplingManager struct {
	mu       sync.RWMutex
	requests map[string]*SamplingRequest
	server   *Server
}

type SamplingRequest struct {
	ID        string
	Model     string
	Messages  []Message
	MaxTokens int
	Status    string
	Result    *SamplingResult
	Created   time.Time
}

type SamplingResult struct {
	Content string
	Usage   SamplingUsage
}

type SamplingUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func NewSamplingManager(server *Server) *SamplingManager {
	return &SamplingManager{
		requests: make(map[string]*SamplingRequest),
		server:   server,
	}
}

func (sm *SamplingManager) CreateSamplingRequest(ctx context.Context, params json.RawMessage) (*SamplingRequest, error) {
	var req struct {
		Model     string    `json:"model"`
		Messages  []Message `json:"messages"`
		MaxTokens int       `json:"max_tokens"`
	}

	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid sampling request: %w", err)
	}

	request := &SamplingRequest{
		ID:        generateRequestID(),
		Model:     req.Model,
		Messages:  req.Messages,
		MaxTokens: req.MaxTokens,
		Status:    "pending",
		Created:   time.Now(),
	}

	sm.mu.Lock()
	sm.requests[request.ID] = request
	sm.mu.Unlock()

	// Notify client about new sampling request
	notification := NewRequest(nil, "sampling/requested", map[string]interface{}{
		"requestId": request.ID,
		"model":     req.Model,
	})

	if err := sm.server.broadcastMessage(*notification); err != nil {
		return nil, fmt.Errorf("failed to notify about sampling request: %w", err)
	}

	return request, nil
}

func (sm *SamplingManager) HandleSamplingResponse(params json.RawMessage) error {
	var resp struct {
		RequestID string         `json:"requestId"`
		Result    SamplingResult `json:"result"`
	}

	if err := json.Unmarshal(params, &resp); err != nil {
		return fmt.Errorf("invalid sampling response: %w", err)
	}

	sm.mu.Lock()
	if req, exists := sm.requests[resp.RequestID]; exists {
		req.Status = "completed"
		req.Result = &resp.Result
	}
	sm.mu.Unlock()

	return nil
}

func (sm *SamplingManager) GetSamplingRequest(id string) (*SamplingRequest, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	req, exists := sm.requests[id]
	if !exists {
		return nil, fmt.Errorf("sampling request not found: %s", id)
	}

	return req, nil
}
