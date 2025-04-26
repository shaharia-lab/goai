package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// AnthropicLLMProvider implements the LLMProvider interface using Anthropic's official Go SDK.
// It provides access to Claude models through Anthropic's API.
type AnthropicLLMProvider struct {
	client AnthropicClientProvider
	model  anthropic.Model
	logger Logger
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
func NewAnthropicLLMProvider(config AnthropicProviderConfig, log Logger) *AnthropicLLMProvider {
	if config.Model == "" {
		config.Model = anthropic.ModelClaude3_7SonnetLatest
	}

	return &AnthropicLLMProvider{
		client: config.Client,
		model:  config.Model,
		logger: log,
	}
}

// GetResponse generates a response using Anthropic's API for the given messages and configuration.
// It supports different message roles (user, assistant, system) and handles them appropriately.
// System messages are handled separately through Anthropic's system parameter.
func (p *AnthropicLLMProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	// Initialize token counters
	var totalInputTokens, totalOutputTokens int64

	var systemPromptBlocks []anthropic.TextBlockParam

	// Convert messages to Anthropic messages
	var anthropicMessages []anthropic.MessageParam
	for _, msg := range messages {
		switch msg.Role {
		case UserRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		case AssistantRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
		case SystemRole:
			systemPromptBlocks = append(systemPromptBlocks, anthropic.NewTextBlock(msg.Text))
		}
	}

	// Prepare tools if registry exists
	ools, err := config.toolsProvider.ListTools(ctx, config.allowedTools)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("error listing tools: %w", err)
	}

	var toolUnionParams []anthropic.ToolUnionUnionParam
	for _, ool := range ools {
		// Unmarshal the JSON schema into a map[string]interface{}
		var schema interface{}
		if err := json.Unmarshal(ool.InputSchema, &schema); err != nil {
			return LLMResponse{}, fmt.Errorf("failed to unmarshal tool parameters: %w", err)
		}

		toolUnionParam := anthropic.ToolParam{
			Name:        anthropic.F(ool.Name),
			Description: anthropic.F(ool.Description),
			InputSchema: anthropic.F(schema),
		}
		toolUnionParams = append(toolUnionParams, toolUnionParam)
	}

	var finalResponse string

	msgParams := anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(config.maxToken),
		Messages:  anthropic.F(anthropicMessages),
		Tools:     anthropic.F(toolUnionParams),
	}

	if len(systemPromptBlocks) > 0 {
		msgParams.System = anthropic.F(systemPromptBlocks)
	}

	if config.enableThinking && config.thinkingBudgetToken > 0 {
		msgParams.Thinking = anthropic.F(anthropic.ThinkingConfigParamUnion(anthropic.ThinkingConfigParam{
			Type:         anthropic.F(anthropic.ThinkingConfigParamTypeEnabled),
			BudgetTokens: anthropic.Int(config.thinkingBudgetToken),
		}))
	}

	logFields := map[string]interface{}{
		"model":                    p.model,
		"max_tokens":               config.maxToken,
		"top_p":                    config.topP,
		"temperature":              config.temperature,
		"top_k":                    config.topK,
		"system_prompt_count":      len(systemPromptBlocks),
		"tools_count":              len(toolUnionParams),
		"thinking_budget":          config.thinkingBudgetToken,
		"enable_thinking":          config.enableThinking,
		"allowed_tools":            config.allowedTools,
		"anthropic_messages_count": len(anthropicMessages),
	}
	p.logger.WithFields(logFields).Debug("AnthropicLLMProvider: Sending request to Anthropic API")

	startTime := time.Now()

	// Start conversation loop
	loop := 1
	for {
		message, err := p.client.CreateMessage(ctx, msgParams)
		if err != nil {
			p.logger.WithErr(err).Error("AnthropicLLMProvider: anthropic api error")
			return LLMResponse{}, err
		}

		p.logger.WithFields(map[string]interface{}{
			"conversation_loop_nth": loop,
			"response_length":       len(message.Content),
			"input_tokens":          message.Usage.InputTokens,
			"output_tokens":         message.Usage.OutputTokens,
		}).Debug("AnthropicLLMProvider: Received response from Anthropic API")

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
				p.logger.WithFields(map[string]interface{}{
					"tool_name":  block.Name,
					"tool_input": block.Input,
				}).Debug("AnthropicLLMProvider: executing tool")

				toolResponse, err := config.toolsProvider.ExecuteTool(ctx, CallToolParams{
					Name:      block.Name,
					Arguments: block.Input,
				})
				if err != nil {
					p.logger.WithFields(map[string]interface{}{
						"tool_name":  block.Name,
						"tool_input": block.Input,
					}).WithErr(err).Error("AnthropicLLMProvider: tool execution error. Sending error to the model and continuing")

					anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("tool execution failed for %s. Error: %s", block.Name, err), true)
					continue
				}

				if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
					p.logger.WithFields(map[string]interface{}{
						"tool_name":  block.Name,
						"tool_input": block.Input,
						"loop_nth":   loop,
					}).Debug("AnthropicLLMProvider: tool returned no content")

					anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("tool '%s' returned no content", block.Name), true)
					continue
				}

				// Add tool results to collection
				for _, content := range toolResponse.Content {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(block.ID, content.Text, toolResponse.IsError))
				}
			default:
			}
		}

		// Add the assistant's message to the conversation
		p.logger.WithFields(map[string]interface{}{
			"loop_nth": loop,
		}).Debug("AnthropicLLMProvider: Adding assistant message to conversation")

		anthropicMessages = append(anthropicMessages, message.ToParam())

		// If no tool results, we're done
		if len(toolResults) == 0 {
			p.logger.WithFields(map[string]interface{}{
				"loop_nth": loop,
			}).Debug("AnthropicLLMProvider: No tool results, exiting loop")
			break
		}

		// Add tool results as user message and continue the loop
		p.logger.WithFields(map[string]interface{}{
			"tool_results_count": len(toolResults),
			"loop_nth":           loop,
		}).Debug("AnthropicLLMProvider: Adding tool results to conversation")

		anthropicMessages = append(anthropicMessages,
			anthropic.NewUserMessage(toolResults...))

		loop++
	}

	logFields["total_input_tokens"] = totalInputTokens
	logFields["total_output_tokens"] = totalOutputTokens
	logFields["total_loop"] = loop
	logFields["final_response_length"] = len(finalResponse)
	p.logger.WithFields(logFields).Debug("AnthropicLLMProvider: Returning final response")

	return LLMResponse{
		Text:             strings.TrimSpace(finalResponse),
		TotalInputToken:  int(totalInputTokens),
		TotalOutputToken: int(totalOutputTokens),
		CompletionTime:   time.Since(startTime).Seconds(),
	}, nil
}
