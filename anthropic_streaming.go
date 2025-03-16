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
		toolResults := p.processToolResults(ctx, msg, config, responseChan)

		// Add the current message to the history
		anthropicMessages = append(anthropicMessages, msg.ToParam())

		// If we have tool results, add them as a user message and continue
		if len(toolResults) > 0 {
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(toolResults...))
			keepProcessing = true
		} else {
			// No more tool calls, we're done
			keepProcessing = false
		}

		// Signal completion if the message is done
		if msg.StopReason == anthropic.MessageStopReasonEndTurn || !keepProcessing {
			responseChan <- StreamingLLMResponse{
				Text: "",
				Done: true,
			}
		}
	}

	// Safety check for too many iterations
	if iterations >= maxIterations {
		// Send a warning and mark as done
		responseChan <- StreamingLLMResponse{
			Text: "Warning: Maximum number of tool iterations reached. Stopping.",
			Done: false,
		}
		responseChan <- StreamingLLMResponse{
			Text: "",
			Done: true,
		}
	}
}

// streamAndProcessMessage streams a message from Anthropic API and processes it
// This implementation uses a completely different approach to text streaming to fix whitespace issues
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
	var textBuf strings.Builder

	// Process the stream events
	for stream.Next() {
		event := stream.Current()
		msg.Accumulate(event)

		// Extract text from the latest accumulated message if available
		for _, block := range msg.Content {
			if textBlock, ok := block.AsUnion().(anthropic.TextBlock); ok {
				if current := textBlock.Text; len(current) > textBuf.Len() {
					// Stream only the new characters since last time
					newText := current[textBuf.Len():]
					if newText != "" {
						responseChan <- StreamingLLMResponse{
							Text: newText,
							Done: false,
						}
					}
					// Update our buffer with all text seen so far
					textBuf.Reset()
					textBuf.WriteString(current)
				}
			}
		}
	}

	// Check for streaming errors
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("error in streaming: %w", err)
	}

	return &msg, nil
}

// processToolResults processes tool usage in a message and returns tool results
func (p *AnthropicLLMProvider) processToolResults(
	ctx context.Context,
	msg *anthropic.Message,
	config LLMRequestConfig,
	responseChan chan<- StreamingLLMResponse,
) []anthropic.ContentBlockParamUnion {
	var toolResults []anthropic.ContentBlockParamUnion

	for _, block := range msg.Content {
		if toolBlock, ok := block.AsUnion().(anthropic.ToolUseBlock); ok {
			// Notify the user about tool execution with a special message
			responseChan <- StreamingLLMResponse{
				Text: fmt.Sprintf("\n[Executing tool: %s]\n", toolBlock.Name),
				Done: false,
			}

			// Execute the tool
			result := p.executeToolAndGetResult(ctx, toolBlock, config)
			if result != nil {
				toolResults = append(toolResults, result)
			}
		}
	}

	return toolResults
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
