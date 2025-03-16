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

func (p *AnthropicLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
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
		return nil, fmt.Errorf("error listing tools: %w", err)
	}

	var toolUnionParams []anthropic.ToolUnionUnionParam
	for _, mcpTool := range mcpTools {
		// Unmarshal the JSON schema into a map[string]interface{}
		var schema interface{}
		if err := json.Unmarshal(mcpTool.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool parameters: %w", err)
		}

		toolUnionParam := anthropic.ToolParam{
			Name:        anthropic.F(mcpTool.Name),
			Description: anthropic.F(mcpTool.Description),
			InputSchema: anthropic.F(schema),
		}
		toolUnionParams = append(toolUnionParams, toolUnionParam)
	}

	responseChan := make(chan StreamingLLMResponse)

	stream := p.client.CreateStreamingMessage(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(p.model),
		MaxTokens: anthropic.F(config.MaxToken),
		Messages:  anthropic.F(anthropicMessages),
		Tools:     anthropic.F(toolUnionParams),
	})

	//var eventMessageStart anthropic.Message
	var eventMessageAccumulated []anthropic.Message
	var msg anthropic.Message
	//var jsonBuffer bytes.Buffer
	//var messageContent strings.Builder
	//selectedTool := activeTool{}

	//var toolName string

	// Start a goroutine to handle the streaming response
	go func() {
		defer close(responseChan)

		for stream.Next() {
			event := stream.Current()
			msg.Accumulate(event)
			eventMessageAccumulated = append(eventMessageAccumulated, msg)
		}

		for {
			var toolResults []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				switch block := block.AsUnion().(type) {
				case anthropic.TextBlock:
					p.sendMessage(responseChan, block.Text, false)

				case anthropic.ToolUseBlock:
					toolResponse, err := config.toolsProvider.ExecuteTool(ctx, mcp.CallToolParams{
						Name:      block.Name,
						Arguments: block.Input,
					})
					if err != nil {
						log.Fatalf("error executing tool '%s': %v", block.Name, err)
					}

					if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
						log.Fatalf("tool '%s' returned no content", block.Name)
					}

					// Add tool result to collection
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(block.ID, toolResponse.Content[0].Text, toolResponse.IsError))
				default:
				}
			}

			anthropicMessages = append(anthropicMessages, msg.ToParam())

			// Add tool results as user message and continue the loop
			anthropicMessages = append(anthropicMessages,
				anthropic.NewUserMessage(toolResults...))

			if msg.StopReason == anthropic.MessageStopReasonEndTurn {
				responseChan <- StreamingLLMResponse{
					Text: "",
					Done: true,
				}

				break
			}

			finalMessage, err := p.client.CreateMessage(ctx, anthropic.MessageNewParams{
				Model:     anthropic.F(p.model),
				MaxTokens: anthropic.F(config.MaxToken),
				Messages:  anthropic.F(anthropicMessages),
				Tools:     anthropic.F(toolUnionParams),
			})

			if err != nil {
				responseChan <- StreamingLLMResponse{
					Text:  "",
					Done:  true,
					Error: err,
				}
				return
			}

			msg = *finalMessage

			//p.sendMessage(responseChan, finalMessage.Content[0].AsUnion().(anthropic.TextBlock).Text, false)
		}

		if err != nil {
			log.Fatalf("error creating message: %v", err)
		}

		p.sendMessage(responseChan, "Streaming finished", false)
		p.sendMessage(responseChan, "", true)

		if err := stream.Err(); err != nil {
			responseChan <- StreamingLLMResponse{
				Text:  "",
				Done:  true,
				Error: err,
			}
			return
		}

		responseChan <- StreamingLLMResponse{
			Text: "",
			Done: true,
		}
	}()

	return responseChan, nil
}

func (p *AnthropicLLMProvider) sendMessage(responseChan chan StreamingLLMResponse, text string, isDone bool) {
	responseChan <- StreamingLLMResponse{
		Text: text + "\n",
		Done: isDone,
	}
}
