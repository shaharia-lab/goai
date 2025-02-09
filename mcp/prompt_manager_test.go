package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPromptManager(t *testing.T) {
	pm := NewPromptManager()
	assert.NotNil(t, pm)
	assert.Empty(t, pm.prompts)
}

func TestRegisterPrompt(t *testing.T) {
	pm := NewPromptManager()

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

		err := pm.RegisterPrompt(prompt)
		assert.NoError(t, err)
		assert.Contains(t, pm.prompts, "test-prompt")
	})

	t.Run("empty name", func(t *testing.T) {
		prompt := Prompt{
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

		err := pm.RegisterPrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt name cannot be empty")
	})

	t.Run("no messages", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
		}

		err := pm.RegisterPrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt must have at least one message")
	})

	t.Run("invalid content type", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "invalid",
						Text: "Hello, world!",
					},
				},
			},
		}

		err := pm.RegisterPrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only text type is supported")
	})

	t.Run("empty message content", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
					},
				},
			},
		}

		err := pm.RegisterPrompt(prompt)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message content text cannot be empty")
	})
}

func TestListPrompts(t *testing.T) {
	pm := NewPromptManager()

	prompts := []Prompt{
		{
			Name: "a_prompt1",
			Messages: []PromptMessage{{
				Content: PromptContent{Type: "text", Text: "test1"},
			}},
		},
		{
			Name: "b_prompt2",
			Messages: []PromptMessage{{
				Content: PromptContent{Type: "text", Text: "test2"},
			}},
		},
		{
			Name: "c_prompt3",
			Messages: []PromptMessage{{
				Content: PromptContent{Type: "text", Text: "test3"},
			}},
		},
	}

	for _, p := range prompts {
		err := pm.RegisterPrompt(p)
		assert.NoError(t, err)
	}

	t.Run("list all prompts", func(t *testing.T) {
		result := pm.ListPrompts("", 50)
		assert.Len(t, result.Prompts, 3)
		assert.Empty(t, result.NextCursor)
	})

	t.Run("list with pagination", func(t *testing.T) {
		result := pm.ListPrompts("", 2)
		assert.Len(t, result.Prompts, 2, "First page should have 2 prompts")
		assert.NotEmpty(t, result.NextCursor, "First page should have a next cursor")

		firstPageNames := make(map[string]bool)
		for _, p := range result.Prompts {
			firstPageNames[p.Name] = true
		}

		nextResult := pm.ListPrompts(result.NextCursor, 2)
		assert.Len(t, nextResult.Prompts, 1, "Second page should have 1 prompt")
		assert.Empty(t, nextResult.NextCursor, "Second page should not have a next cursor")

		for _, p := range nextResult.Prompts {
			assert.False(t, firstPageNames[p.Name],
				"Prompt %s from second page should not be in first page", p.Name)
		}

		allPrompts := make(map[string]bool)
		for _, p := range result.Prompts {
			allPrompts[p.Name] = true
		}
		for _, p := range nextResult.Prompts {
			allPrompts[p.Name] = true
		}
		assert.Len(t, allPrompts, 3, "Total unique prompts should be 3")
	})
}

func TestGetPrompt(t *testing.T) {
	pm := NewPromptManager()

	t.Run("get non-existent prompt", func(t *testing.T) {
		_, err := pm.GetPrompt(GetPromptParams{Name: "non-existent"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt not found")
	})

	t.Run("get prompt with arguments", func(t *testing.T) {
		prompt := Prompt{
			Name: "test-prompt",
			Arguments: []PromptArgument{
				{
					Name:     "name",
					Required: true,
				},
			},
			Messages: []PromptMessage{
				{
					Role: "system",
					Content: PromptContent{
						Type: "text",
						Text: "Hello, {{name}}!",
					},
				},
			},
		}

		err := pm.RegisterPrompt(prompt)
		assert.NoError(t, err)

		args, _ := json.Marshal(map[string]string{"name": "John"})
		result, err := pm.GetPrompt(GetPromptParams{
			Name:      "test-prompt",
			Arguments: args,
		})

		assert.NoError(t, err)
		assert.Equal(t, "Hello, John!", result.Messages[0].Content.Text)
	})
}

func TestDeletePrompt(t *testing.T) {
	pm := NewPromptManager()

	prompt := Prompt{
		Name: "test-prompt",
		Messages: []PromptMessage{
			{
				Content: PromptContent{Type: "text", Text: "test"},
			},
		},
	}

	_ = pm.RegisterPrompt(prompt)

	t.Run("delete existing prompt", func(t *testing.T) {
		err := pm.DeletePrompt("test-prompt")
		assert.NoError(t, err)
		assert.Empty(t, pm.prompts)
	})

	t.Run("delete non-existent prompt", func(t *testing.T) {
		err := pm.DeletePrompt("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "prompt not found")
	})
}
