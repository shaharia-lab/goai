package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PromptManager handles prompt-related operations
type PromptManager struct {
	prompts map[string]Prompt
}

// NewPromptManager creates a new PromptManager instance
func NewPromptManager() *PromptManager {
	return &PromptManager{
		prompts: make(map[string]Prompt),
	}
}

// RegisterPrompt adds a new prompt to the manager
func (pm *PromptManager) RegisterPrompt(prompt Prompt) error {
	if prompt.Name == "" {
		return fmt.Errorf("prompt name cannot be empty")
	}

	// Validate that all required fields are present
	if len(prompt.Messages) == 0 {
		return fmt.Errorf("prompt must have at least one message")
	}

	// Validate all messages have valid content
	for _, msg := range prompt.Messages {
		if msg.Content.Type != "text" {
			return fmt.Errorf("only text type is supported for prompt content")
		}
		if msg.Content.Text == "" {
			return fmt.Errorf("message content text cannot be empty")
		}
	}

	// Validate arguments if present
	for _, arg := range prompt.Arguments {
		if arg.Name == "" {
			return fmt.Errorf("argument name cannot be empty")
		}
	}

	pm.prompts[prompt.Name] = prompt
	return nil
}

// ListPrompts returns a paginated list of prompts
func (pm *PromptManager) ListPrompts(cursor string, limit int) ListPromptsResult {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	// Convert map to slice for pagination
	allPrompts := make([]Prompt, 0, len(pm.prompts))
	for _, p := range pm.prompts {
		allPrompts = append(allPrompts, p)
	}

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, p := range allPrompts {
			if p.Name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Get prompts for current page
	endIdx := startIdx + limit
	if endIdx > len(allPrompts) {
		endIdx = len(allPrompts)
	}

	var nextCursor string
	if endIdx < len(allPrompts) {
		nextCursor = allPrompts[endIdx-1].Name
	}

	return ListPromptsResult{
		Prompts:    allPrompts[startIdx:endIdx],
		NextCursor: nextCursor,
	}
}

// GetPrompt retrieves a prompt and processes its arguments
func (pm *PromptManager) GetPrompt(params GetPromptParams) (*Prompt, error) {
	prompt, exists := pm.prompts[params.Name]
	if !exists {
		return nil, fmt.Errorf("prompt not found: %s", params.Name)
	}

	// If the prompt has arguments, process them
	if len(prompt.Arguments) > 0 && len(params.Arguments) > 0 {
		var providedArgs map[string]interface{}
		if err := json.Unmarshal(params.Arguments, &providedArgs); err != nil {
			return nil, fmt.Errorf("invalid arguments format: %w", err)
		}

		// Verify all required arguments are provided
		for _, arg := range prompt.Arguments {
			if arg.Required {
				if _, exists := providedArgs[arg.Name]; !exists {
					return nil, fmt.Errorf("missing required argument: %s", arg.Name)
				}
			}
		}

		// Create a copy of the prompt to modify with arguments
		promptCopy := prompt
		for i, msg := range promptCopy.Messages {
			// Process argument substitutions in text content
			for argName, argValue := range providedArgs {
				// Simple string replacement for now
				if strValue, ok := argValue.(string); ok {
					promptCopy.Messages[i].Content.Text = replaceArgument(
						msg.Content.Text,
						argName,
						strValue,
					)
				}
			}
		}
		return &promptCopy, nil
	}

	return &prompt, nil
}

// replaceArgument replaces argument placeholders in text
func replaceArgument(text, argName, value string) string {
	placeholder := fmt.Sprintf("{{%s}}", argName)
	return strings.Replace(text, placeholder, value, -1)
}

// DeletePrompt removes a prompt from the manager
func (pm *PromptManager) DeletePrompt(name string) error {
	if _, exists := pm.prompts[name]; !exists {
		return fmt.Errorf("prompt not found: %s", name)
	}
	delete(pm.prompts, name)
	return nil
}
