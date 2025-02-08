// prompt_manager.go
package mcp

import (
	"fmt"
	"sync"
)

type Prompt struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Template    string                 `json:"template"`
	Variables   []PromptVariable       `json:"variables"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type PromptVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type PromptManager struct {
	prompts map[string]*Prompt
	mu      sync.RWMutex
}

func NewPromptManager() *PromptManager {
	return &PromptManager{
		prompts: make(map[string]*Prompt),
	}
}

func (pm *PromptManager) AddPrompt(prompt *Prompt) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.prompts[prompt.ID]; exists {
		return fmt.Errorf("prompt %s already exists", prompt.ID)
	}

	pm.prompts[prompt.ID] = prompt
	return nil
}

func (pm *PromptManager) GetPrompt(id string) (*Prompt, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	prompt, exists := pm.prompts[id]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", id)
	}

	return prompt, nil
}

func (pm *PromptManager) ListPrompts() []*Prompt {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	prompts := make([]*Prompt, 0, len(pm.prompts))
	for _, p := range pm.prompts {
		prompts = append(prompts, p)
	}
	return prompts
}
