package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"strings"
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

	// Process the first streaming request
	iterations := 0
	maxIterations := 10 // Safeguard against infinite loops
	keepProcessing := true

	for keepProcessing && iterations < maxIterations {
		iterations++

		// Create and process stream for current context
		msg, err := p.streamAndProcessMessage(ctx, anthropicMessages, toolUnionParams, config, responseChan)
		if err != nil {
			responseChan <- StreamingLLMResponse{
				Text:  "",
				Done:  true,
				Error: err,
			}
			return
		}

		// Process tool results if any
		toolResults, hasTextContent := p.processContentBlocks(ctx, msg, config, responseChan)

		// Add the current message to the history
		anthropicMessages = append(anthropicMessages, msg.ToParam())

		// If we have tool results, add them as a user message and continue
		if len(toolResults) > 0 {
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(toolResults...))
			keepProcessing = true
		} else {
			// If no tool usage and message is complete or we've exceeded iterations, we're done
			keepProcessing = false
			if !hasTextContent || msg.StopReason != anthropic.MessageStopReasonEndTurn {
				p.sendDone(responseChan)
			}
		}

		// Signal completion if the message is done
		if msg.StopReason == anthropic.MessageStopReasonEndTurn {
			p.sendDone(responseChan)
			keepProcessing = false
		}
	}

	// Safety check for too many iterations
	if iterations >= maxIterations {
		p.sendMessage(responseChan, "Warning: Maximum number of tool iterations reached. Stopping.", false)
		p.sendDone(responseChan)
	}
}

// streamAndProcessMessage streams a message from Anthropic API and processes it
func (p *AnthropicLLMProvider) streamAndProcessMessage(
	ctx context.Context,
	anthropicMessages []anthropic.MessageParam,
	toolUnionParams []anthropic.ToolUnionUnionParam,
	config LLMRequestConfig,
	responseChan chan<- StreamingLLMResponse,
) (*anthropic.Message, error) {
	// Initialize the stream
	stream := p.client.CreateStreamingMessage(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(config.MaxToken),
		Messages:  anthropic.F(anthropicMessages),
		Tools:     anthropic.F(toolUnionParams),
	})

	// Track the current content block for streaming text
	var msg anthropic.Message

	// Process the stream events
	for stream.Next() {
		event := stream.Current()

		// Extract text content based on event type
		switch eventUnion := event.AsUnion().(type) {
		case anthropic.ContentBlockDeltaEvent:
			// Only handle text delta events
			if delta, ok := eventUnion.Delta.AsUnion().(anthropic.TextDelta); ok {
				// Stream text content to the user as it arrives
				if delta.Text != "" {
					p.sendMessage(responseChan, delta.Text, false)
				}
			}
		}

		// Accumulate the message regardless of event type
		msg.Accumulate(event)
	}

	// Check for streaming errors
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("error in streaming: %w", err)
	}

	return &msg, nil
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
			// We've already streamed the text content in streamAndProcessMessage
			// No need to send again

		case anthropic.ToolUseBlock:
			// Let the user know we're executing a tool
			toolCallMsg := fmt.Sprintf(" [Executing tool: %s]", block.Name)
			p.sendMessage(responseChan, toolCallMsg, false)

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

// sendMessage sends a message to the response channel
func (p *AnthropicLLMProvider) sendMessage(responseChan chan<- StreamingLLMResponse, text string, isDone bool) {
	if text == "" && !isDone {
		return // Don't send empty non-final messages
	}

	// Clean up the text - remove leading/trailing whitespace and normalize newlines
	text = strings.TrimSpace(text)

	if text != "" {
		responseChan <- StreamingLLMResponse{
			Text: text,
			Done: isDone,
		}
	} else if isDone {
		responseChan <- StreamingLLMResponse{
			Text: "",
			Done: true,
		}
	}
}

// sendDone sends a done signal to the response channel
func (p *AnthropicLLMProvider) sendDone(responseChan chan<- StreamingLLMResponse) {
	responseChan <- StreamingLLMResponse{
		Text: "",
		Done: true,
	}
}
