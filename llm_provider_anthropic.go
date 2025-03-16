package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shaharia-lab/goai/mcp"

	"github.com/anthropics/anthropic-sdk-go"
)

// AnthropicLLMProvider implements the LLMProvider interface using Anthropic's official Go SDK.
// It provides access to Claude models through Anthropic's API.
type AnthropicLLMProvider struct {
	client AnthropicClientProvider
	model  anthropic.Model
}

// AnthropicProviderConfig holds the configuration options for creating an Anthropic provider.
type AnthropicProviderConfig struct {
	// Client is the AnthropicClientProvider implementation to use
	Client AnthropicClientProvider

	// Model specifies which Anthropic model to use (e.g., "claude-3-opus-20240229", "claude-3-sonnet-20240229")
	Model anthropic.Model
}

// NewAnthropicLLMProvider creates a new Anthropic provider with the specified configuration.
// If no model is specified, it defaults to Claude 3.5 Sonnet.
//
// Example usage:
//
//	client := NewAnthropicClient("your-api-key")
//	provider := NewAnthropicLLMProvider(AnthropicProviderConfig{
//	    Client: client,
//	    Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
//	})
//
//	response, err := provider.GetResponse(messages, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewAnthropicLLMProvider(config AnthropicProviderConfig) *AnthropicLLMProvider {
	if config.Model == "" {
		config.Model = anthropic.ModelClaude_3_5_Sonnet_20240620
	}

	return &AnthropicLLMProvider{
		client: config.Client,
		model:  config.Model,
	}
}

// prepareMessageParams creates the Anthropic message parameters from LLM messages and config.
// This is an internal helper function to reduce code duplication.
func (p *AnthropicLLMProvider) prepareMessageParams(messages []LLMMessage, config LLMRequestConfig) anthropic.MessageNewParams {
	var anthropicMessages []anthropic.MessageParam
	var systemMessage string

	// Process messages based on their role
	for _, msg := range messages {
		switch msg.Role {
		case SystemRole:
			systemMessage = msg.Text
		case UserRole:
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		case AssistantRole:
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
		default:
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		}
	}

	params := anthropic.MessageNewParams{
		Model:       anthropic.F(p.model),
		Messages:    anthropic.F(anthropicMessages),
		MaxTokens:   anthropic.F(config.MaxToken),
		TopP:        anthropic.Float(config.TopP),
		Temperature: anthropic.Float(config.Temperature),
	}

	// Add system message if present
	if systemMessage != "" {
		params.System = anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(systemMessage),
		})
	}

	return params
}

// GetResponse generates a response using Anthropic's API for the given messages and configuration.
// It supports different message roles (user, assistant, system) and handles them appropriately.
// System messages are handled separately through Anthropic's system parameter.
func (p *AnthropicLLMProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()

	// Initialize token counters
	var totalInputTokens, totalOutputTokens int64

	// Convert our messages to Anthropic messages
	var anthropicMessages []anthropic.MessageParam
	for _, msg := range messages {
		switch msg.Role {
		case UserRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		case AssistantRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
		}
	}

	// Prepare tools if registry exists
	mcpTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("error listing tools: %w", err)
	}

	var toolUnionParams []anthropic.ToolUnionUnionParam
	for _, mcpTool := range mcpTools {
		// Unmarshal the JSON schema into a map[string]interface{}
		var schema interface{}
		if err := json.Unmarshal(mcpTool.InputSchema, &schema); err != nil {
			return LLMResponse{}, fmt.Errorf("failed to unmarshal tool parameters: %w", err)
		}

		toolUnionParam := anthropic.ToolParam{
			Name:        anthropic.F(mcpTool.Name),
			Description: anthropic.F(mcpTool.Description),
			InputSchema: anthropic.F(schema),
		}
		toolUnionParams = append(toolUnionParams, toolUnionParam)
	}

	var finalResponse string

	// Start conversation loop
	for {
		message, err := p.client.CreateMessage(ctx, anthropic.MessageNewParams{
			Model:     anthropic.F(p.model),
			MaxTokens: anthropic.F(config.MaxToken),
			Messages:  anthropic.F(anthropicMessages),
			Tools:     anthropic.F(toolUnionParams),
		})
		if err != nil {
			return LLMResponse{}, err
		}

		// Update token counts
		totalInputTokens += message.Usage.InputTokens
		totalOutputTokens += message.Usage.OutputTokens

		// Process message content and collect tool uses
		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range message.Content {
			switch block := block.AsUnion().(type) {
			case anthropic.TextBlock:
				finalResponse += block.Text + "\n"

			case anthropic.ToolUseBlock:
				// Execute the tool
				toolResponse, err := config.toolsProvider.ExecuteTool(ctx, mcp.CallToolParams{
					Name:      block.Name,
					Arguments: block.Input,
				})
				if err != nil {
					return LLMResponse{}, fmt.Errorf("error executing tool '%s': %w", block.Name, err)
				}

				if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
					return LLMResponse{}, fmt.Errorf("tool '%s' returned no content", block.Name)
				}

				// Add tool result to collection
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(block.ID, toolResponse.Content[0].Text, toolResponse.IsError))
			default:
			}
		}

		// Add the assistant's message to the conversation
		anthropicMessages = append(anthropicMessages, message.ToParam())

		// If no tool results, we're done
		if len(toolResults) == 0 {
			break
		}

		// Add tool results as user message and continue the loop
		anthropicMessages = append(anthropicMessages,
			anthropic.NewUserMessage(toolResults...))
	}

	return LLMResponse{
		Text:             strings.TrimSpace(finalResponse),
		TotalInputToken:  int(totalInputTokens),
		TotalOutputToken: int(totalOutputTokens),
		CompletionTime:   time.Since(startTime).Seconds(),
	}, nil
}
