package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/shaharia-lab/goai/mcp"
)

// GetStreamingResponse implements streaming chat with tool calling support for Bedrock.
func (p *BedrockLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	// --- Initial Setup ---
	responseChan := make(chan StreamingLLMResponse, 100) // Buffered channel

	// Prepare initial messages and system prompts (deep copy history to avoid race conditions if original slice is modified)
	currentHistory := make([]types.Message, 0, len(messages))
	var systemPrompts []types.SystemContentBlock
	for _, msg := range messages {
		switch msg.Role {
		case SystemRole:
			// Ensure SystemContentBlockMemberText has Value field set correctly
			systemPrompts = append(systemPrompts, &types.SystemContentBlockMemberText{Value: msg.Text})
		case UserRole:
			// Ensure ContentBlockMemberText has Value field set correctly
			currentHistory = append(currentHistory, types.Message{
				Role:    types.ConversationRoleUser,
				Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Text}},
			})
		case AssistantRole:
			currentHistory = append(currentHistory, types.Message{
				Role:    types.ConversationRoleAssistant,
				Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Text}},
			})
		default: // Treat unknown as user
			currentHistory = append(currentHistory, types.Message{
				Role:    types.ConversationRoleUser,
				Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Text}},
			})
		}
	}

	// Prepare tool configuration (must happen before goroutine start to return errors early)
	var toolConfig *types.ToolConfiguration
	var toolProvider *ToolsProvider // Keep provider reference for use in goroutine
	if config.toolsProvider != nil {
		toolProvider = config.toolsProvider // Assign for later use
		mcpTools, err := config.toolsProvider.ListTools(ctx, config.allowedTools)
		if err != nil {
			// Return error immediately before starting goroutine
			return nil, fmt.Errorf("streaming: error listing tools: %w", err)
		}
		if len(mcpTools) > 0 {
			var bedrockTools []types.Tool
			for _, mcpTool := range mcpTools {
				var schemaDoc map[string]interface{}
				if mcpTool.InputSchema == nil {
					return nil, fmt.Errorf("streaming: tool '%s' has nil InputSchema", mcpTool.Name)
				}
				if err := json.Unmarshal(mcpTool.InputSchema, &schemaDoc); err != nil {
					return nil, fmt.Errorf("streaming: failed to unmarshal tool input schema for '%s': %w", mcpTool.Name, err)
				}
				toolSpec := types.ToolSpecification{
					Name:        aws.String(mcpTool.Name),
					Description: aws.String(mcpTool.Description),
					InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(schemaDoc)},
				}
				bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{Value: toolSpec})
			}
			toolConfig = &types.ToolConfiguration{
				Tools: bedrockTools,
				// ToolChoice: ... // Optional
			}
		}
	}

	// Define maxIterations (consider adding to LLMRequestConfig or using a sensible default)
	maxIterations := 10 // Max tool call cycles
	if config.maxIterations > 0 {
		maxIterations = config.maxIterations
	}

	// --- Goroutine for Managing Streaming and Tool Calls ---
	go func() {
		defer close(responseChan) // Ensure channel is closed on any exit path

		// Make a mutable copy of the history for the goroutine to manage
		goroutineHistory := make([]types.Message, len(currentHistory))
		copy(goroutineHistory, currentHistory)

		// OuterLoop manages sequential ConverseStream calls if tools are used
	OuterLoop:
		for turn := 0; turn < maxIterations; turn++ {

			// --- Prepare and Start Stream Call ---
			input := &bedrockruntime.ConverseStreamInput{
				ModelId:  &p.model,
				Messages: goroutineHistory, // Use the goroutine's copy of history
				System:   systemPrompts,
				InferenceConfig: &types.InferenceConfiguration{
					Temperature: aws.Float32(float32(config.temperature)),
					TopP:        aws.Float32(float32(config.topP)),
					MaxTokens:   aws.Int32(int32(config.maxToken)),
					// StopSequences: config.stopSequences, // Add if needed
				},
				ToolConfig: toolConfig, // Include tool config
			}

			// --- Make the API Call ---
			output, err := p.client.ConverseStream(ctx, input)
			if err != nil {
				// Send error and exit goroutine
				responseChan <- StreamingLLMResponse{Error: fmt.Errorf("ConverseStream API call failed (turn %d): %w", turn, err), Done: true}
				return
			}

			stream := output.GetStream()
			streamClosed := false // Flag to prevent double closing in defer/error paths
			defer func() {
				// Ensure stream is closed if the goroutine exits unexpectedly
				if !streamClosed {
					_ = stream.Close()
				}
			}()

			var currentAssistantMessageContent []types.ContentBlock                           // Accumulate the *raw* content blocks for the assistant's message in this turn
			var assistantMessageRole types.ConversationRole = types.ConversationRoleAssistant // Default expected role

			toolCallRequested := false
			conversationComplete := false

			// --- Inner Loop: Process Events for the Current Stream ---
		InnerLoop:
			for {
				select {
				case <-ctx.Done():
					responseChan <- StreamingLLMResponse{Error: ctx.Err(), Done: true}
					if !streamClosed {
						_ = stream.Close()
						streamClosed = true
					}
					return // Exit goroutine

				case event, ok := <-stream.Events():
					if !ok { // Stream channel closed by server
						if err := stream.Err(); err != nil && ctx.Err() == nil { // Check for underlying error only if context is okay
							responseChan <- StreamingLLMResponse{Error: fmt.Errorf("stream closed with error: %w", err), Done: true}
							return // Exit goroutine on stream error
						}
						// Stream closed cleanly (no error)
						if !toolCallRequested { // If no tool call was pending, assume completion
							conversationComplete = true
						}
						break InnerLoop // Exit inner loop, outer loop decides next step
					}

					// --- Process Received Event ---
					switch v := event.(type) {
					case *types.ConverseStreamOutputMemberMessageStart:
						assistantMessageRole = v.Value.Role                     // Store the actual role
						currentAssistantMessageContent = []types.ContentBlock{} // Reset for new message

					case *types.ConverseStreamOutputMemberContentBlockStart:
						// This event signals the start of a new content block (e.g., text or tool_use)
						// at index v.Value.ContentBlockIndex.
						// The type of block is in v.Value.Start (e.g., *types.ContentBlockStartMemberToolUse).
						// In this implementation, we primarily rely on MessageStop and inspect the history
						// afterwards, so we don't need complex handling here.
						// We *could* use this to initialize block structures if building the message
						// content incrementally, but that adds complexity.
						// --- Removed erroneous line: currentAssistantMessageContent = append(currentAssistantMessageContent, v.Value.ContentBlock) ---
						// log.Printf("DEBUG: ContentBlockStart received, Index: %d", v.Value.ContentBlockIndex) // Optional logging

					case *types.ConverseStreamOutputMemberContentBlockDelta:
						// If it's a text delta, send it to the user
						if delta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
							responseChan <- StreamingLLMResponse{
								Text: delta.Value,
								// TokenCount is not available per delta
							}
							// --- Update the *last* content block if it's text ---
							// This reconstructs the text content within the block structure
							if len(currentAssistantMessageContent) > 0 {
								lastBlock := currentAssistantMessageContent[len(currentAssistantMessageContent)-1]
								if textBlock, ok := lastBlock.(*types.ContentBlockMemberText); ok {
									// Append delta to existing text block's value
									textBlock.Value += delta.Value
								}
								// If the last block wasn't text, this delta might be unexpected
								// or belong to a newly started text block (handled by ContentBlockStart).
							}
						}
						// Handle other delta types if necessary (e.g., tool input streaming)

					case *types.ConverseStreamOutputMemberContentBlockStop:
						// Indicates a block finished. The currentAssistantMessageContent should reflect the structure.

					case *types.ConverseStreamOutputMemberMessageStop:
						stopReason := v.Value.StopReason

						// --- IMPORTANT: Add completed assistant message to history BEFORE handling tool call ---
						if len(currentAssistantMessageContent) > 0 {
							completedAssistantMessage := types.Message{
								Role:    assistantMessageRole,
								Content: currentAssistantMessageContent, // Content accumulated from blocks/deltas
							}
							goroutineHistory = append(goroutineHistory, completedAssistantMessage)
						}

						// Now check the reason
						if stopReason == types.StopReasonToolUse {
							toolCallRequested = true
							break InnerLoop // Exit inner loop to handle tool call
						} else if stopReason == types.StopReasonEndTurn || stopReason == types.StopReasonMaxTokens || stopReason == types.StopReasonStopSequence {
							conversationComplete = true
							break InnerLoop // Exit inner loop for normal completion
						} else {
							// Unknown stop reason, treat as completion
							conversationComplete = true
							break InnerLoop
						}

					case *types.ConverseStreamOutputMemberMetadata:
						// Metadata might arrive after MessageStop. Indicates end of this specific API call.
						// We don't explicitly signal Done here, wait for outer loop logic.
						// Could potentially extract final token counts for this call if needed for logging.
						// Let the stream close naturally after this.

					default:
						// Ignore unknown event types
					} // end switch
				} // end select
			} // --- End InnerLoop ---

			// --- Post Inner Loop Processing (within OuterLoop turn) ---

			// Ensure stream is closed cleanly after processing its events
			if !streamClosed {
				_ = stream.Close()
				streamClosed = true
			}

			// Check stream error again after loop exit (e.g., if !ok happened with an error)
			if err := stream.Err(); err != nil && ctx.Err() == nil {
				responseChan <- StreamingLLMResponse{Error: fmt.Errorf("stream error after processing: %w", err), Done: true}
				return // Exit goroutine
			}

			// --- Handle Tool Call Request ---
			if toolCallRequested {
				if toolProvider == nil {
					responseChan <- StreamingLLMResponse{Error: errors.New("streaming: tool use requested but no toolsProvider configured"), Done: true}
					return // Exit goroutine
				}

				// Find the tool use block in the *last* message added to history
				if len(goroutineHistory) == 0 {
					responseChan <- StreamingLLMResponse{Error: errors.New("streaming: tool use requested but conversation history is empty"), Done: true}
					return
				}
				lastMessage := goroutineHistory[len(goroutineHistory)-1]
				if lastMessage.Role != types.ConversationRoleAssistant {
					// This shouldn't happen if MessageStart/Stop logic is correct
					responseChan <- StreamingLLMResponse{Error: fmt.Errorf("streaming: tool use requested, but last message role is %s, expected Assistant", lastMessage.Role), Done: true}
					return
				}

				var toolUseBlock *types.ContentBlockMemberToolUse
				for _, block := range lastMessage.Content {
					// Need to check the underlying type correctly after accumulation
					if tuWrapper, ok := block.(*types.ContentBlockMemberToolUse); ok {
						toolUseBlock = tuWrapper
						break
					}
				}

				if toolUseBlock == nil {
					responseChan <- StreamingLLMResponse{Error: errors.New("streaming: stop reason was ToolUse, but no ToolUse block found in last assistant message content"), Done: true}
					return
				}

				// --- Execute Tool ---
				toolUseID := aws.ToString(toolUseBlock.Value.ToolUseId)
				toolName := aws.ToString(toolUseBlock.Value.Name)
				toolInput := toolUseBlock.Value.Input // map[string]interface{} or similar

				inputBytes, err := json.Marshal(toolInput)
				if err != nil {
					responseChan <- StreamingLLMResponse{Error: fmt.Errorf("streaming: failed to marshal tool input for '%s': %w", toolName, err), Done: true}
					return
				}

				toolResponse, execErr := toolProvider.ExecuteTool(ctx, mcp.CallToolParams{
					Name:      toolName,
					Arguments: inputBytes,
				})

				// --- Prepare Tool Result Block ---
				var toolResultBlock types.ContentBlockMemberToolResult
				// (Reusing the robust result formatting logic from GetResponse)
				if execErr != nil {
					toolResultBlock = types.ContentBlockMemberToolResult{
						Value: types.ToolResultBlock{ToolUseId: aws.String(toolUseID), Status: types.ToolResultStatusError, Content: []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{Value: fmt.Sprintf("Error executing tool '%s': %v", toolName, execErr)}}}}
				} else {
					var bedrockResultContent []types.ToolResultContentBlock
					toolResultStatus := types.ToolResultStatusSuccess
					if toolResponse.IsError {
						toolResultStatus = types.ToolResultStatusError
						if len(toolResponse.Content) > 0 {
							for _, mc := range toolResponse.Content {
								bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: mc.Text})
							}
						} else {
							bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: fmt.Sprintf("Tool '%s' reported error but no content", toolName)})
						}
					} else if len(toolResponse.Content) == 0 {
						bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: ""})
					} else {
						for _, mc := range toolResponse.Content {
							bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: mc.Text})
						}
					}
					toolResultBlock = types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{ToolUseId: aws.String(toolUseID), Content: bedrockResultContent, Status: toolResultStatus}}
				}

				// --- Add User Message with Tool Result ---
				userToolResultMessage := types.Message{
					Role:    types.ConversationRoleUser,
					Content: []types.ContentBlock{&toolResultBlock}, // Add the single result block
				}
				goroutineHistory = append(goroutineHistory, userToolResultMessage) // Append to history

				// Continue to the next iteration of the OuterLoop to make a new stream call
				continue OuterLoop // Crucial: starts next turn

			} // --- End Handle Tool Call Request ---

			// --- Handle Normal Conversation Completion ---
			if conversationComplete {
				responseChan <- StreamingLLMResponse{Done: true} // Send final Done signal
				return                                           // Exit goroutine normally
			}

			// --- Handle unexpected inner loop exit ---
			// If the inner loop broke but neither tool use nor explicit completion was signaled.
			// This might mean the stream just ended. Assume completion.
			responseChan <- StreamingLLMResponse{Done: true}
			return // Exit goroutine

		} // --- End OuterLoop ---

		// --- Handle Max Iterations Reached ---
		// If the outer loop finishes without returning, max iterations were hit.
		responseChan <- StreamingLLMResponse{Error: fmt.Errorf("streaming: max tool iterations (%d) reached", maxIterations), Done: true}

	}() // --- End Goroutine ---

	return responseChan, nil // Return the channel immediately
}
