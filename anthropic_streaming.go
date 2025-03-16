package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/shaharia-lab/goai/mcp"
	"log"
)

type activeTool struct {
	ID      string
	Name    string
	Input   json.RawMessage
	Content string
}

// GetStreamingResponse handles streaming LLM responses with tool usage capabilities
func (p *AnthropicLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	// Convert messages to Anthropic format
	anthropicMessages, err := p.convertToAnthropicMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	// Prepare tools if registry exists
	toolUnionParams, err := p.prepareTools(ctx, config)
	if err != nil {
		return nil, err
	}

	// Create response channel
	responseChan := make(chan StreamingLLMResponse)

	// Start processing in a goroutine
	go p.processStreamingResponse(ctx, anthropicMessages, toolUnionParams, config, responseChan)

	return responseChan, nil
}

// convertToAnthropicMessages converts our internal message format to Anthropic's format
func (p *AnthropicLLMProvider) convertToAnthropicMessages(messages []LLMMessage) ([]anthropic.MessageParam, error) {
	var anthropicMessages []anthropic.MessageParam

	for _, msg := range messages {
		switch msg.Role {
		case UserRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		case AssistantRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
		default:
			return nil, fmt.Errorf("unsupported role: %s", msg.Role)
		}
	}

	return anthropicMessages, nil
}

// prepareTools prepares the tool definitions for the Anthropic API
func (p *AnthropicLLMProvider) prepareTools(ctx context.Context, config LLMRequestConfig) ([]anthropic.ToolUnionUnionParam, error) {
	mcpTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
	if err != nil {
		return nil, fmt.Errorf("error listing tools: %w", err)
	}

	var toolUnionParams []anthropic.ToolUnionUnionParam
	for _, mcpTool := range mcpTools {
		// Unmarshal the JSON schema into a map[string]interface{}
		var schema interface{}
		if err := json.Unmarshal(mcpTool.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool parameters for %s: %w", mcpTool.Name, err)
		}

		toolUnionParam := anthropic.ToolParam{
			Name:        anthropic.F(mcpTool.Name),
			Description: anthropic.F(mcpTool.Description),
			InputSchema: anthropic.F(schema),
		}
		toolUnionParams = append(toolUnionParams, toolUnionParam)
	}

	return toolUnionParams, nil
}

// processStreamingResponse handles the streaming response from Anthropic
func (p *AnthropicLLMProvider) processStreamingResponse(
	ctx context.Context,
	anthropicMessages []anthropic.MessageParam,
	toolUnionParams []anthropic.ToolUnionUnionParam,
	config LLMRequestConfig,
	responseChan chan<- StreamingLLMResponse,
) {
	defer close(responseChan)

	// Initialize the stream
	stream := p.client.CreateStreamingMessage(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(config.MaxToken),
		Messages:  anthropic.F(anthropicMessages),
		Tools:     anthropic.F(toolUnionParams),
	})

	// Accumulate the message from stream events
	var msg anthropic.Message
	for stream.Next() {
		event := stream.Current()
		msg.Accumulate(event)
	}

	// Check for streaming errors
	if err := stream.Err(); err != nil {
		responseChan <- StreamingLLMResponse{
			Text:  "",
			Done:  true,
			Error: err,
		}
		return
	}

	// Process the accumulated message with tools
	continueProcessing := true
	for continueProcessing {
		continueProcessing = p.processMessageWithTools(ctx, &msg, &anthropicMessages, toolUnionParams, config, responseChan)
	}
}

// processMessageWithTools processes a message, handles any tool calls, and determines if more processing is needed
func (p *AnthropicLLMProvider) processMessageWithTools(
	ctx context.Context,
	msg *anthropic.Message,
	anthropicMessages *[]anthropic.MessageParam,
	toolUnionParams []anthropic.ToolUnionUnionParam,
	config LLMRequestConfig,
	responseChan chan<- StreamingLLMResponse,
) bool {
	toolResults, hasTextContent := p.processContentBlocks(ctx, msg, config, responseChan)

	// Add the current message to the history
	*anthropicMessages = append(*anthropicMessages, msg.ToParam())

	// If we have tool results, add them as a user message
	if len(toolResults) > 0 {
		*anthropicMessages = append(*anthropicMessages,
			anthropic.NewUserMessage(toolResults...))
	}

	// Check if we need to continue processing
	if msg.StopReason == anthropic.MessageStopReasonEndTurn {
		// Signal completion
		responseChan <- StreamingLLMResponse{Text: "", Done: true}
		return false
	}

	// Only continue if we have tool results
	if len(toolResults) == 0 {
		// If no tool usage but we still have text content, we're done
		if hasTextContent {
			p.sendMessage(responseChan, "", true)
		} else {
			// Signal completion since there's no more processing needed
			responseChan <- StreamingLLMResponse{Text: "", Done: true}
		}
		return false
	}

	// Make another API call with tool results
	newMsg, err := p.createFollowUpMessage(ctx, *anthropicMessages, toolUnionParams, config)
	if err != nil {
		responseChan <- StreamingLLMResponse{
			Text:  "",
			Done:  true,
			Error: err,
		}
		return false
	}

	// Update the message for next iteration
	*msg = *newMsg
	return true
}

// processContentBlocks processes the content blocks in a message
func (p *AnthropicLLMProvider) processContentBlocks(
	ctx context.Context,
	msg *anthropic.Message,
	config LLMRequestConfig,
	responseChan chan<- StreamingLLMResponse,
) ([]anthropic.ContentBlockParamUnion, bool) {
	var toolResults []anthropic.ContentBlockParamUnion
	hasTextContent := false

	for _, block := range msg.Content {
		switch block := block.AsUnion().(type) {
		case anthropic.TextBlock:
			hasTextContent = true
			p.sendMessage(responseChan, block.Text, false)

		case anthropic.ToolUseBlock:
			result := p.executeToolAndGetResult(ctx, block, config)
			if result != nil {
				toolResults = append(toolResults, result)
			}
		}
	}

	return toolResults, hasTextContent
}

// executeToolAndGetResult executes a tool and returns the result
func (p *AnthropicLLMProvider) executeToolAndGetResult(
	ctx context.Context,
	block anthropic.ToolUseBlock,
	config LLMRequestConfig,
) anthropic.ContentBlockParamUnion {
	toolResponse, err := config.toolsProvider.ExecuteTool(ctx, mcp.CallToolParams{
		Name:      block.Name,
		Arguments: block.Input,
	})

	if err != nil {
		log.Printf("Error executing tool '%s': %v", block.Name, err)
		return anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %v", err), true)
	}

	if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
		log.Printf("Tool '%s' returned no content", block.Name)
		return anthropic.NewToolResultBlock(block.ID, "No content returned from tool", true)
	}

	return anthropic.NewToolResultBlock(block.ID, toolResponse.Content[0].Text, toolResponse.IsError)
}

// createFollowUpMessage creates a follow-up message with the updated context
func (p *AnthropicLLMProvider) createFollowUpMessage(
	ctx context.Context,
	anthropicMessages []anthropic.MessageParam,
	toolUnionParams []anthropic.ToolUnionUnionParam,
	config LLMRequestConfig,
) (*anthropic.Message, error) {
	finalMessage, err := p.client.CreateMessage(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(config.MaxToken),
		Messages:  anthropic.F(anthropicMessages),
		Tools:     anthropic.F(toolUnionParams),
	})

	if err != nil {
		return nil, fmt.Errorf("error creating follow-up message: %w", err)
	}

	return finalMessage, nil
}

// sendMessage sends a message to the response channel
func (p *AnthropicLLMProvider) sendMessage(responseChan chan<- StreamingLLMResponse, text string, isDone bool) {
	responseChan <- StreamingLLMResponse{
		Text: text + "\n",
		Done: isDone,
	}
}
