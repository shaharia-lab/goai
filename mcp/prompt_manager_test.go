package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPromptManager(t *testing.T) {
	t.Run("empty prompts", func(t *testing.T) {
		pm, err := NewPromptManager(nil)
		assert.NoError(t, err)
		assert.NotNil(t, pm)
		assert.Empty(t, pm.prompts)
	})

	t.Run("valid prompts", func(t *testing.T) {
		prompts := []Prompt{
			{
				Name: "test-prompt",
				Messages: []PromptMessage{
					{
						Role: "system",
						Content: PromptContent{
							Type: "text",
							Text: "Hello, world!",
						},
					},
				},
			},
		}
		pm, err := NewPromptManager(prompts)
		assert.NoError(t, err)
		assert.NotNil(t, pm)
		assert.Len(t, pm.prompts, 1)
	})

	t.Run("invalid prompt", func(t *testing.T) {
		prompts := []Prompt{
			{
				Name: "", // Invalid - empty name
				Messages: []PromptMessage{
					{
						Role: "system",
						Content: PromptContent{
							Type: "text",
							Text: "Hello, world!",
						},
					},
				},
			},
		}
		pm, err := NewPromptManager(prompts)
		assert.Error(t, err)
		assert.Nil(t, pm)
		assert.Contains(t, err.Error(), "prompt name cannot be empty")
	})
}

func TestAddPrompt(t *testing.T) {
	pm, _ := NewPromptManager(nil)

	t.Run("valid prompt", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
						Text: "Hello, world!",
					},
				},
			},
		}

		err := pm.AddPrompt(prompt)
		assert.NoError(t, err)
		assert.Contains(t, pm.prompts, "test-prompt")
	})

	t.Run("duplicate prompt", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
						Text: "Hello, world!",
					},
				},
			},
		}

		err := pm.AddPrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestRemovePrompt(t *testing.T) {
	pm, _ := NewPromptManager(nil)

	t.Run("remove existing prompt", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello!",
					},
				},
			},
		}
		_ = pm.AddPrompt(prompt)

		err := pm.RemovePrompt("test-prompt")
		assert.NoError(t, err)
		assert.NotContains(t, pm.prompts, "test-prompt")
	})

	t.Run("remove non-existent prompt", func(t *testing.T) {
		err := pm.RemovePrompt("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt not found")
	})
}

func TestValidatePrompt(t *testing.T) {
	t.Run("valid prompt", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
						Text: "Hello, world!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "arg1",
					Required: true,
				},
			},
		}
		err := validatePrompt(prompt)
		assert.NoError(t, err)
	})

	t.Run("empty prompt name", func(t *testing.T) {
		prompt := Prompt{
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello!",
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt name cannot be empty")
	})

	t.Run("empty messages", func(t *testing.T) {
		prompt := Prompt{
			Name:     "test-prompt",
			Messages: []PromptMessage{},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt must have at least one message")
	})

	t.Run("unsupported content type", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "image",
						Text: "some text",
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only text type is supported")
	})

	t.Run("empty content text", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "",
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message content text cannot be empty")
	})

	t.Run("empty argument name", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "",
					Required: true,
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "argument name cannot be empty")
	})

	t.Run("multiple messages validation", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Message 1",
					},
				},
				{
					Content: PromptContent{
						Type: "text",
						Text: "", // Invalid
					},
				},
			},
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message content text cannot be empty")
	})

	t.Run("multiple valid arguments", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello {{name}} and {{greeting}}!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "name",
					Required: true,
				},
				{
					Name:     "greeting",
					Required: false,
				},
			},
		}
		err := validatePrompt(prompt)
		assert.NoError(t, err)
	})

	t.Run("nil messages", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
		}
		err := validatePrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt must have at least one message")
	})
}

func TestGetPromptTwo(t *testing.T) {
	pm, _ := NewPromptManager(nil)

	t.Run("prompt without arguments", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
						Text: "Hello, world!",
					},
				},
			},
		}
		_ = pm.AddPrompt(prompt)

		result, err := pm.GetPrompt(GetPromptParams{Name: "test-prompt"})
		assert.NoError(t, err)
		assert.Equal(t, prompt.Name, result.Name)
		assert.Equal(t, prompt.Messages[0].Content.Text, result.Messages[0].Content.Text)
	})

	t.Run("prompt with arguments", func(t *testing.T) {
		prompt := Prompt{
			Name: "arg-prompt",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello, {{name}}!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "name",
					Required: true,
				},
			},
		}
		_ = pm.AddPrompt(prompt)

		args, _ := json.Marshal(map[string]string{"name": "John"})
		result, err := pm.GetPrompt(GetPromptParams{
			Name:      "arg-prompt",
			Arguments: args,
		})
		assert.NoError(t, err)
		assert.Equal(t, "Hello, John!", result.Messages[0].Content.Text)
	})

	t.Run("missing required argument", func(t *testing.T) {
		prompt := Prompt{
			Name: "required-arg",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello, {{name}}!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "name",
					Required: true,
				},
			},
		}
		_ = pm.AddPrompt(prompt)

		args, _ := json.Marshal(map[string]string{})
		result, err := pm.GetPrompt(GetPromptParams{
			Name:      "required-arg",
			Arguments: args,
		})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "missing required argument")
	})

	t.Run("prompt not found", func(t *testing.T) {
		result, err := pm.GetPrompt(GetPromptParams{Name: "non-existent"})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "prompt not found")
	})

	t.Run("invalid arguments format", func(t *testing.T) {
		prompt := Prompt{
			Name: "invalid-args",
			Messages: []PromptMessage{
				{
					Content: PromptContent{
						Type: "text",
						Text: "Hello, {{name}}!",
					},
				},
			},
			Arguments: []PromptArgument{
				{
					Name:     "name",
					Required: true,
				},
			},
		}
		_ = pm.AddPrompt(prompt)

		result, err := pm.GetPrompt(GetPromptParams{
			Name:      "invalid-args",
			Arguments: []byte("invalid json"),
		})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments format")
	})
}
