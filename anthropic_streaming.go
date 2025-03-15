package goai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// GetStreamingResponse generates a streaming response using Anthropic's API.
func (p *AnthropicLLMProvider) GetStreamingResponses(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	ch := make(chan StreamingLLMResponse)

	go func() {
		defer close(ch)

		// Convert our messages to Anthropic messages
		var anthropicMessages []anthropic.MessageParam
		for _, msg := range messages {
			switch msg.Role {
			case UserRole:
				anthropicMessages = append(anthropicMessages,
					anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text)))
			case AssistantRole:
				// Check if the message has tool results that need to be included
				anthropicMessages = append(anthropicMessages,
					anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text)))
			}
		}

		// Prepare tools if registry exists
		mcpTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
		if err != nil {
			ch <- StreamingLLMResponse{Error: fmt.Errorf("error listing tools: %w", err), Done: true}
			return
		}

		var toolUnionParams []anthropic.ToolUnionUnionParam
		for _, mcpTool := range mcpTools {
			var schema interface{}
			if err := json.Unmarshal(mcpTool.InputSchema, &schema); err != nil {
				ch <- StreamingLLMResponse{Error: fmt.Errorf("fail to unmarshal: %w", err), Done: true}
				return
			}

			toolUnionParam := anthropic.ToolParam{
				Name:        anthropic.F(mcpTool.Name),
				Description: anthropic.F(mcpTool.Description),
				InputSchema: anthropic.F(schema),
			}
			toolUnionParams = append(toolUnionParams, toolUnionParam)
		}

		// Create single message params
		messageParams := anthropic.MessageNewParams{
			Model:     anthropic.F(p.model),
			MaxTokens: anthropic.F(config.MaxToken),
			Messages:  anthropic.F(anthropicMessages),
		}

		// Only include tools if we have them
		if len(toolUnionParams) > 0 {
			messageParams.Tools = anthropic.F(toolUnionParams)
		}

		stream := p.client.CreateStreamingMessage(ctx, messageParams)

		var jsonBuffer bytes.Buffer
		var activeToolID, activeToolName string
		var currentToolInput json.RawMessage
		var messageContent strings.Builder

		for stream.Next() {
			event := stream.Current()

			switch evt := event.AsUnion().(type) {
			case anthropic.MessageStartEvent:
				// Reset state for new message
				jsonBuffer.Reset()
				messageContent.Reset()
			case anthropic.ContentBlockStartEvent:
				if contentBlock, ok := evt.ContentBlock.AsUnion().(anthropic.ToolUseBlock); ok {
					activeToolID = contentBlock.ID
					activeToolName = contentBlock.Name
					jsonBuffer.Reset()
					currentToolInput = contentBlock.Input
				}
			case anthropic.ContentBlockDeltaEvent:
				switch delta := evt.Delta.AsUnion().(type) {
				case anthropic.TextDelta:
					messageContent.WriteString(delta.Text)
					ch <- StreamingLLMResponse{Text: delta.Text, Done: false}
				case anthropic.InputJSONDelta:
					jsonBuffer.WriteString(delta.PartialJSON)
					currentToolInput = jsonBuffer.Bytes()
				}
			case anthropic.MessageDeltaEvent:
				if string(evt.Delta.StopReason) == string(anthropic.MessageStopReasonToolUse) {
					// Verify we have an active tool request before executing
					if activeToolID == "" || activeToolName == "" {
						ch <- StreamingLLMResponse{Error: fmt.Errorf("received tool use stop reason without active tool"), Done: true}
						return
					}

					// Execute tool and append result to messages
					toolResponse, err := config.toolsProvider.ExecuteTool(ctx, mcp.CallToolParams{
						Name:      activeToolName,
						Arguments: currentToolInput,
					})
					if err != nil {
						ch <- StreamingLLMResponse{Error: fmt.Errorf("error executing tool '%s': %w", activeToolName, err), Done: true}
						return
					}

					anthropicMessages = append(anthropicMessages,
						anthropic.NewAssistantMessage(
							anthropic.NewToolUseBlockParam(activeToolID, activeToolName, currentToolInput)))

					log.Printf("%+v", toolResponse)

					if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
						ch <- StreamingLLMResponse{Error: fmt.Errorf("tool '%s' returned no content", activeToolName), Done: true}
						return
					}

					anthropicMessages = append(anthropicMessages,
						anthropic.NewUserMessage(
							anthropic.NewToolResultBlock(activeToolID, toolResponse.Content[0].Text, false)))

					messageParams.Messages = anthropic.F(anthropicMessages)
				}
			case anthropic.MessageStopEvent:
				messageParams.Tools = anthropic.F(toolUnionParams)

				log.Printf("%+v", messageParams)

				stream = p.client.CreateStreamingMessage(ctx, messageParams)
			}
		}

		/*messageParams.Messages = anthropic.F(anthropicMessages)
		messageParams.Tools = anthropic.F(toolUnionParams)
		stream = p.client.CreateStreamingMessage(ctx, messageParams)
		// Handle message start and end events only
		for stream.Next() {
			event := stream.Current()

			switch evt := event.AsUnion().(type) {
			case anthropic.MessageStartEvent:
				log.Printf("🌱 MessageStart: ID=%s", evt.Message.ID)

			case anthropic.ContentBlockStartEvent:
				if contentBlock, ok := evt.ContentBlock.AsUnion().(anthropic.ToolUseBlock); ok {
					log.Printf("🔧 ToolUseBlock: ID=%s, Name=%s", contentBlock.ID, contentBlock.Name)
				}
			case anthropic.ContentBlockStopEvent:
				log.Printf("🔚 ContentBlockStop")
			}
		}*/

		if stream.Err() != nil {
			ch <- StreamingLLMResponse{Error: stream.Err(), Done: true}
			return
		}

		ch <- StreamingLLMResponse{Done: true}
	}()

	return ch, nil
}
