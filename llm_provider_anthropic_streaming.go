package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/shaharia-lab/goai/mcp"
)

// GetStreamingResponse handles streaming LLM responses with tool usage capabilities
func (p *AnthropicLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	anthropicMessages, systemMessage, err := p.convertToAnthropicMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	toolUnionParams, err := p.prepareTools(ctx, config)
	if err != nil {
		return nil, err
	}

	responseChan := make(chan StreamingLLMResponse)

	go p.processStreamingResponse(ctx, anthropicMessages, toolUnionParams, config, responseChan, systemMessage)

	return responseChan, nil
}

// convertToAnthropicMessages converts our internal message format to Anthropic's format
func (p *AnthropicLLMProvider) convertToAnthropicMessages(messages []LLMMessage) ([]anthropic.MessageParam, string, error) {
	var anthropicMessages []anthropic.MessageParam

	var systemMessage string

	for _, msg := range messages {
		switch msg.Role {
		case UserRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
		case AssistantRole:
			anthropicMessages = append(anthropicMessages,
				anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
		case SystemRole:
			systemMessage = msg.Text
		default:
			return nil, systemMessage, fmt.Errorf("unsupported role: %s", msg.Role)
		}
	}

	return anthropicMessages, systemMessage, nil
}

// prepareTools prepares the tool definitions for the Anthropic API
func (p *AnthropicLLMProvider) prepareTools(ctx context.Context, config LLMRequestConfig) ([]anthropic.ToolUnionUnionParam, error) {
	mcpTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
	if err != nil {
		return nil, fmt.Errorf("error listing tools: %w", err)
	}

	var toolUnionParams []anthropic.ToolUnionUnionParam
	for _, mcpTool := range mcpTools {
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
	systemMessage string,
) {
	defer close(responseChan)

	iterations := 0
	maxIterations := 10
	keepProcessing := true

	for keepProcessing && iterations < maxIterations {
		iterations++

		msg, err := p.streamAndProcessMessage(ctx, anthropicMessages, toolUnionParams, config, responseChan, systemMessage)
		if err != nil {
			responseChan <- StreamingLLMResponse{
				Text:  "",
				Done:  true,
				Error: err,
			}
			return
		}

		toolResults := p.processToolResults(ctx, msg, config, responseChan)

		anthropicMessages = append(anthropicMessages, msg.ToParam())

		if len(toolResults) > 0 {
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(toolResults...))
			keepProcessing = true
		} else {
			keepProcessing = false
		}

		if msg.StopReason == anthropic.MessageStopReasonEndTurn || !keepProcessing {
			responseChan <- StreamingLLMResponse{
				Text: "",
				Done: true,
			}
		}
	}

	if iterations >= maxIterations {
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
	systemMessage string,
) (*anthropic.Message, error) {
	msgParam := anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(config.MaxToken),
		Messages:  anthropic.F(anthropicMessages),
		Tools:     anthropic.F(toolUnionParams),
	}

	if systemMessage != "" {
		msgParam.System = anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(systemMessage),
		})
	}

	stream := p.client.CreateStreamingMessage(ctx, msgParam)

	var msg anthropic.Message
	//var textBuf strings.Builder

	for stream.Next() {
		event := stream.Current()
		msg.Accumulate(event)

		// we will accumulate the text for processing
		// but at the same time we can send the delta
		switch evt := event.AsUnion().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch delta := evt.Delta.AsUnion().(type) {
			case anthropic.TextDelta:
				responseChan <- StreamingLLMResponse{
					Text: delta.Text,
					Done: false,
				}
			}
		default:

		}
	}

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
			responseChan <- StreamingLLMResponse{
				Text: "",
				Done: false,
			}

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
		return anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %v", err), true)
	}

	if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
		return anthropic.NewToolResultBlock(block.ID, "No content returned from tool", true)
	}

	return anthropic.NewToolResultBlock(block.ID, toolResponse.Content[0].Text, toolResponse.IsError)
}
