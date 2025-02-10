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

// NewPromptManager creates a new PromptManager instance with initial prompts
func NewPromptManager(prompts []Prompt) (*PromptManager, error) {
	p := make(map[string]Prompt)

	for _, prompt := range prompts {
		if err := validatePrompt(prompt); err != nil {
			return nil, fmt.Errorf("invalid prompt: %v", err)
		}
		p[prompt.Name] = prompt
	}

	return &PromptManager{
		prompts: p,
	}, nil
}

// validatePrompt validates a prompt's structure and content
func validatePrompt(prompt Prompt) error {
	if prompt.Name == "" {
		return fmt.Errorf("prompt name cannot be empty")
	}

	if len(prompt.Messages) == 0 {
		return fmt.Errorf("prompt must have at least one message")
	}

	for _, msg := range prompt.Messages {
		if msg.Content.Type != "text" {
			return fmt.Errorf("only text type is supported for prompt content")
		}
		if msg.Content.Text == "" {
			return fmt.Errorf("message content text cannot be empty")
		}
	}

	for _, arg := range prompt.Arguments {
		if arg.Name == "" {
			return fmt.Errorf("argument name cannot be empty")
		}
	}

	return nil
}

// AddPrompt adds a new prompt to the manager
func (pm *PromptManager) AddPrompt(prompt Prompt) error {
	if err := validatePrompt(prompt); err != nil {
		return err
	}

	if _, exists := pm.prompts[prompt.Name]; exists {
		return fmt.Errorf("prompt with name '%s' already exists", prompt.Name)
	}

	pm.prompts[prompt.Name] = prompt
	return nil
}

// GetPrompt retrieves and processes a prompt
func (pm *PromptManager) GetPrompt(params GetPromptParams) (*Prompt, error) {
	prompt, exists := pm.prompts[params.Name]
	if !exists {
		return nil, fmt.Errorf("prompt not found: %s", params.Name)
	}

	return processPrompt(prompt, params.Arguments)
}

// RemovePrompt removes a prompt from the manager
func (pm *PromptManager) RemovePrompt(name string) error {
	if _, exists := pm.prompts[name]; !exists {
		return fmt.Errorf("prompt not found: %s", name)
	}
	delete(pm.prompts, name)
	return nil
}

// processPrompt handles argument substitution in prompts
func processPrompt(prompt Prompt, arguments json.RawMessage) (*Prompt, error) {
	if len(prompt.Arguments) == 0 || len(arguments) == 0 {
		return &prompt, nil
	}

	var providedArgs map[string]interface{}
	if err := json.Unmarshal(arguments, &providedArgs); err != nil {
		return nil, fmt.Errorf("invalid arguments format: %w", err)
	}

	// Verify required arguments
	for _, arg := range prompt.Arguments {
		if arg.Required {
			if _, exists := providedArgs[arg.Name]; !exists {
				return nil, fmt.Errorf("missing required argument: %s", arg.Name)
			}
		}
	}

	// Create processed copy
	promptCopy := prompt
	for i, msg := range promptCopy.Messages {
		for argName, argValue := range providedArgs {
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
