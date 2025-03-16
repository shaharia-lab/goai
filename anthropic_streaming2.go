package goai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/shaharia-lab/goai/mcp"
)

// GetStreamingResponse generates a streaming response using Anthropic's API.
func (p *AnthropicLLMProvider) GetStreamingResponse_LATEST(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	responseChan := make(chan StreamingLLMResponse, 100)
	log.Printf("🚀 Starting streaming response with %d initial messages", len(messages))

	params := p.prepareMessageParams(messages, config)
	params.Tools = anthropic.F(prepareTools(config)) // Always prepare tools
	log.Printf("🔧 Prepared tools: %d", len(params.Tools.Value))

	go func() {
		defer close(responseChan)
		defer log.Println("🔴 Closing response channel")

		var conversationMessages []anthropic.MessageParam
		if params.Messages.Value != nil {
			conversationMessages = params.Messages.Value
		}
		log.Printf("💬 Initial conversation history: %d messages", len(conversationMessages))

		// Initial request parameters
		currentParams := anthropic.MessageNewParams{
			Model:       params.Model,
			Messages:    anthropic.F(conversationMessages),
			MaxTokens:   params.MaxTokens,
			Tools:       params.Tools, // Initial tools
			TopP:        params.TopP,
			Temperature: params.Temperature,
			System:      params.System,
		}

		// --- No outer loop needed! ---

		stream := p.client.CreateStreamingMessage(ctx, currentParams) // First stream creation
		if stream == nil {
			responseChan <- StreamingLLMResponse{Error: fmt.Errorf("failed to create streaming message"), Done: true}
			return
		}
		log.Println("⏳ Starting stream processing")

		for { // Infinite loop, broken by MessageStopEvent with end_turn

			var jsonBuffer bytes.Buffer
			var activeToolID, activeToolName string
			var currentToolInput json.RawMessage
			var messageContent strings.Builder

			for stream.Next() { // Inner loop for processing stream events
				event := stream.Current()
				log.Printf("🎬 Received event type: %T", event.AsUnion())

				switch evt := event.AsUnion().(type) {
				case anthropic.MessageStartEvent:
					log.Printf("🌱 MessageStart: ID=%s", evt.Message.ID)
					// Reset state for a new message *within* the stream
					jsonBuffer.Reset()
					messageContent.Reset()

				case anthropic.ContentBlockStartEvent:
					log.Printf("🚦 ContentBlockStart: Type=%T, Block: %+v", evt.ContentBlock.AsUnion(), evt.ContentBlock)
					if contentBlock, ok := evt.ContentBlock.AsUnion().(anthropic.ToolUseBlock); ok {
						log.Printf("🛠️ ToolUseBlock detected: ID=%s, Name=%s", contentBlock.ID, contentBlock.Name)
						activeToolID = contentBlock.ID
						activeToolName = contentBlock.Name
						jsonBuffer.Reset()                    // Clear the buffer
						currentToolInput = contentBlock.Input // Store the *initial* input
					}

				case anthropic.ContentBlockDeltaEvent:
					switch delta := evt.Delta.AsUnion().(type) {
					case anthropic.TextDelta:
						log.Printf("📝 TextDelta: %q", delta.Text)
						messageContent.WriteString(delta.Text)
						responseChan <- StreamingLLMResponse{Text: delta.Text, Done: false}
					case anthropic.InputJSONDelta:
						log.Printf("📦 InputJSONDelta: %q", delta.PartialJSON)
						jsonBuffer.WriteString(delta.PartialJSON) // Accumulate JSON
						currentToolInput = jsonBuffer.Bytes()     // Update currentToolInput
					}

				case anthropic.ContentBlockStopEvent:
					log.Printf("🔚 ContentBlockStop")

				case anthropic.MessageDeltaEvent:
					log.Printf("🔼 MessageDelta: StopReason=%s", evt.Delta.StopReason)
					if anthropic.MessageStopReason(evt.Delta.StopReason) == anthropic.MessageStopReasonToolUse {
						log.Printf("🔧 Tool Use Stop Reason Received")

						if activeToolID == "" || activeToolName == "" {
							responseChan <- StreamingLLMResponse{Error: fmt.Errorf("received tool use stop reason without active tool"), Done: true}
							return
						}

						// Execute tool and append result to messages
						toolResponse, err := config.toolsProvider.ExecuteTool(ctx, mcp.CallToolParams{
							Name:      activeToolName,
							Arguments: currentToolInput,
						})
						if err != nil {
							responseChan <- StreamingLLMResponse{Error: fmt.Errorf("error executing tool '%s': %w", activeToolName, err), Done: true}
							return
						}
						log.Printf("%+v", toolResponse)

						if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
							responseChan <- StreamingLLMResponse{Error: fmt.Errorf("tool '%s' returned no content", activeToolName), Done: true}
							return
						}

						// Add tool *use* as ASSISTANT message
						conversationMessages = append(conversationMessages,
							anthropic.NewAssistantMessage(
								anthropic.NewToolUseBlockParam(activeToolID, activeToolName, currentToolInput)))

						// Add tool *result* as USER message
						conversationMessages = append(conversationMessages,
							anthropic.NewUserMessage(
								anthropic.NewToolResultBlock(activeToolID, toolResponse.Content[0].Text, false)))

					}

				case anthropic.MessageStopEvent:
					//log.Printf("🛑 MessageStop: Content=%q, StopReason: %s", messageContent.String(), evt.Delta.StopReason)

					// Add assistant's message to history (if any)
					if messageContent.Len() > 0 {
						conversationMessages = append(conversationMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(messageContent.String())))
					}

					/*if evt.Delta.StopReason == string(anthropic.MessageStopReasonEndTurn) {
						log.Printf("🏁 End Turn Stop Reason Received. Exiting.")
						responseChan <- StreamingLLMResponse{Done: true}
						return // Exit the goroutine
					}*/

					// Prepare new params for the *next* request
					currentParams = anthropic.MessageNewParams{
						Model:       params.Model,
						Messages:    anthropic.F(conversationMessages), // Updated messages
						MaxTokens:   params.MaxTokens,
						Tools:       params.Tools, // Re-include tools
						TopP:        params.TopP,
						Temperature: params.Temperature,
						System:      params.System,
					}

					// Create a *new* stream with the updated parameters
					stream = p.client.CreateStreamingMessage(ctx, currentParams)
					if stream == nil {
						responseChan <- StreamingLLMResponse{Error: fmt.Errorf("failed to create new streaming message"), Done: true}
						return
					}
					log.Println("⏳ New stream created for next turn.")
					break // Break out of the inner (stream.Next()) loop, but *continue* the outer loop
				}

				if err := stream.Err(); err != nil {
					responseChan <- StreamingLLMResponse{Error: err, Done: true}
					log.Printf("🔥 Stream error: %v", err)
					return
				}
			} // Inner loop (stream.Next()) ends here

			// No 'continue' here. We want to loop back to the *top* of the 'for {}' loop,
			// which will now use the *new* stream we created.

		} // Outer loop (infinite) ends here
	}()

	return responseChan, nil
}
