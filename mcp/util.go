package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

func validateToolV2(tool Tool) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if tool.Description == "" {
		return fmt.Errorf("tool description cannot be empty")
	}

	if tool.InputSchema != nil {
		loader := gojsonschema.NewStringLoader(string(tool.InputSchema))
		_, err := gojsonschema.NewSchema(loader)
		if err != nil {
			return fmt.Errorf("invalid input schema: %v", err)
		}
	}

	if reflect.ValueOf(tool.Handler).IsNil() {
		return fmt.Errorf("tool handler cannot be nil")
	}

	return nil
}

// processPrompt handles argument substitution in prompts
func processPrompt(prompt Prompt, arguments json.RawMessage) (*Prompt, error) {
	// Return early if no arguments to process
	if len(prompt.Arguments) == 0 || len(arguments) == 0 {
		return &Prompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Messages:    prompt.Messages,
		}, nil
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

	// Create processed copy without arguments field
	promptCopy := Prompt{
		Name:        prompt.Name,
		Description: prompt.Description,
		Messages:    make([]PromptMessage, len(prompt.Messages)),
	}

	// Process each message
	for i, msg := range prompt.Messages {
		text := msg.Content.Text

		// Replace placeholders in the text for all message types
		for _, arg := range prompt.Arguments {
			if value, exists := providedArgs[arg.Name]; exists {
				if strValue, ok := value.(string); ok {
					text = replaceArgument(text, arg.Name, strValue)
				}
			}
		}

		promptCopy.Messages[i] = PromptMessage{
			Role: msg.Role,
			Content: PromptContent{
				Type: msg.Content.Type,
				Text: text,
			},
		}
	}

	return &promptCopy, nil
}

func replaceArgument(text, argName, value string) string {
	placeholder := fmt.Sprintf("{{%s}}", argName)
	return strings.Replace(text, placeholder, value, -1)
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

// validateResource checks if a resource is valid
func validateResource(resource Resource) error {
	if resource.URI == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	// Add additional validation rules as needed
	if resource.MimeType == "" {
		return fmt.Errorf("resource MIME type cannot be empty")
	}

	return nil
}

// Helper function to validate URI schemes
func isValidURIScheme(uri string) bool {
	validSchemes := []string{"file://", "https://", "git://"}
	for _, scheme := range validSchemes {
		if strings.HasPrefix(uri, scheme) {
			return true
		}
	}
	return false
}
