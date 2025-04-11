package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/shaharia-lab/goai/mcp"
)

// GetStreamingResponse implements streaming chat with tool calling support for Bedrock,
// accumulating tool input from deltas and correctly formatting history.
func (p *BedrockLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	responseChan := make(chan StreamingLLMResponse, 100)

	initialHistory := make([]types.Message, 0, len(messages))
	var systemPrompts []types.SystemContentBlock
	for _, msg := range messages {
		switch msg.Role {
		case SystemRole:
			systemPrompts = append(systemPrompts, &types.SystemContentBlockMemberText{Value: msg.Text})
		case UserRole:
			initialHistory = append(initialHistory, types.Message{
				Role:    types.ConversationRoleUser,
				Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Text}},
			})
		case AssistantRole:
			initialHistory = append(initialHistory, types.Message{
				Role:    types.ConversationRoleAssistant,
				Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Text}},
			})
		default:
			initialHistory = append(initialHistory, types.Message{
				Role:    types.ConversationRoleUser,
				Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Text}},
			})
		}
	}

	var toolConfig *types.ToolConfiguration
	var toolProvider *ToolsProvider
	if config.toolsProvider != nil {
		toolProvider = config.toolsProvider
		mcpTools, err := config.toolsProvider.ListTools(ctx, config.allowedTools)
		if err != nil {
			return nil, fmt.Errorf("streaming: error listing tools: %w", err)
		}
		if len(mcpTools) > 0 {
			var bedrockTools []types.Tool
			for _, mcpTool := range mcpTools {
				var schemaDoc map[string]interface{}
				inputSchemaBytes := []byte(mcpTool.InputSchema)
				if len(inputSchemaBytes) == 0 || string(inputSchemaBytes) == "null" {
					schemaDoc = make(map[string]interface{})
				} else if err := json.Unmarshal(inputSchemaBytes, &schemaDoc); err != nil {
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
			}
		}
	}

	maxIterations := 10
	if config.maxIterations > 0 {
		maxIterations = config.maxIterations
	}

	go func() {
		defer close(responseChan)

		goroutineHistory := make([]types.Message, len(initialHistory))
		copy(goroutineHistory, initialHistory)

		var currentToolUseID *string
		var currentToolName *string
		var currentToolInputBuffer *strings.Builder

	OuterLoop:
		for turn := 0; turn < maxIterations; turn++ {

			currentToolUseID = nil
			currentToolName = nil
			currentToolInputBuffer = nil

			input := &bedrockruntime.ConverseStreamInput{
				ModelId:  &p.model,
				Messages: goroutineHistory,
				System:   systemPrompts,
				InferenceConfig: &types.InferenceConfiguration{
					Temperature: aws.Float32(float32(config.temperature)),
					TopP:        aws.Float32(float32(config.topP)),
					MaxTokens:   aws.Int32(int32(config.maxToken)),
				},
				ToolConfig: toolConfig,
			}

			output, err := p.client.ConverseStream(ctx, input)
			if err != nil {
				responseChan <- StreamingLLMResponse{Error: fmt.Errorf("ConverseStream API call failed (turn %d): %w", turn, err), Done: true}
				return
			}

			stream := output.GetStream()
			streamClosed := false
			defer func() {
				if !streamClosed {
					_ = stream.Close()
				}
			}()

			toolCallRequested := false
			conversationComplete := false

		InnerLoop:
			for {
				select {
				case <-ctx.Done():
					responseChan <- StreamingLLMResponse{Error: ctx.Err(), Done: true}
					if !streamClosed {
						_ = stream.Close()
						streamClosed = true
					}
					return

				case event, ok := <-stream.Events():
					if !ok {
						if err := stream.Err(); err != nil && ctx.Err() == nil {
							responseChan <- StreamingLLMResponse{Error: fmt.Errorf("stream closed with error: %w", err), Done: true}
							return
						}
						if !toolCallRequested {
							conversationComplete = true
						}
						break InnerLoop
					}

					switch v := event.(type) {
					case *types.ConverseStreamOutputMemberMessageStart:
						currentToolUseID = nil
						currentToolName = nil
						currentToolInputBuffer = nil

					case *types.ConverseStreamOutputMemberContentBlockStart:
						if toolUseStart, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
							currentToolUseID = toolUseStart.Value.ToolUseId
							currentToolName = toolUseStart.Value.Name
							currentToolInputBuffer = &strings.Builder{}
						}

					case *types.ConverseStreamOutputMemberContentBlockDelta:
						if delta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
							responseChan <- StreamingLLMResponse{Text: delta.Value}
						}
						if inputDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
							if currentToolInputBuffer != nil {
								currentToolInputBuffer.WriteString(aws.ToString(inputDelta.Value.Input))
							}
						}

					case *types.ConverseStreamOutputMemberContentBlockStop:

					case *types.ConverseStreamOutputMemberMessageStop:
						stopReason := v.Value.StopReason
						if stopReason == types.StopReasonToolUse {
							toolCallRequested = true
							break InnerLoop
						} else if stopReason == types.StopReasonEndTurn || stopReason == types.StopReasonMaxTokens || stopReason == types.StopReasonStopSequence {
							conversationComplete = true
							break InnerLoop
						} else {
							conversationComplete = true
							break InnerLoop
						}

					case *types.ConverseStreamOutputMemberMetadata:
					}
				}
			}

			if !streamClosed {
				_ = stream.Close()
				streamClosed = true
			}
			if err := stream.Err(); err != nil && ctx.Err() == nil {
				responseChan <- StreamingLLMResponse{Error: fmt.Errorf("stream error after processing: %w", err), Done: true}
				return
			}

			if toolCallRequested {
				toolCallRequested = false

				if toolProvider == nil {
					responseChan <- StreamingLLMResponse{Error: errors.New("streaming: tool use requested but no toolsProvider configured"), Done: true}
					return
				}
				if currentToolUseID == nil || currentToolName == nil || currentToolInputBuffer == nil {
					responseChan <- StreamingLLMResponse{Error: fmt.Errorf("streaming: stop reason ToolUse, but tool details not fully captured (ID:%v Name:%v BufferNil:%t)", currentToolUseID, currentToolName, currentToolInputBuffer == nil), Done: true}
					return
				}

				inputJSONString := currentToolInputBuffer.String()
				inputBytesForExec := []byte(inputJSONString)

				var inputMap map[string]interface{}
				if inputJSONString == "" || inputJSONString == "null" {
					inputMap = make(map[string]interface{})
				} else {
					if err := json.Unmarshal(inputBytesForExec, &inputMap); err != nil {
						responseChan <- StreamingLLMResponse{Error: fmt.Errorf("streaming: failed to unmarshal tool input JSON '%s': %w", inputJSONString, err), Done: true}
						return
					}
				}

				fabricatedToolUseBlock := types.ToolUseBlock{
					ToolUseId: currentToolUseID,
					Name:      currentToolName,
					Input:     document.NewLazyDocument(inputMap),
				}
				assistantMsgWithToolUse := types.Message{
					Role: types.ConversationRoleAssistant,
					Content: []types.ContentBlock{
						&types.ContentBlockMemberToolUse{Value: fabricatedToolUseBlock},
					},
				}
				goroutineHistory = append(goroutineHistory, assistantMsgWithToolUse)

				toolUseIDString := aws.ToString(currentToolUseID)
				toolNameString := aws.ToString(currentToolName)
				toolResponse, execErr := toolProvider.ExecuteTool(ctx, mcp.CallToolParams{
					Name:      toolNameString,
					Arguments: inputBytesForExec,
				})

				var toolResultBlock types.ContentBlockMemberToolResult
				if execErr != nil {
					toolResultBlock = types.ContentBlockMemberToolResult{
						Value: types.ToolResultBlock{
							ToolUseId: aws.String(toolUseIDString),
							Status:    types.ToolResultStatusError,
							Content: []types.ToolResultContentBlock{
								&types.ToolResultContentBlockMemberText{
									Value: fmt.Sprintf("Error executing tool '%s': %v", toolNameString, execErr),
								},
							},
						},
					}
				} else {
					var bedrockResultContent []types.ToolResultContentBlock
					toolResultStatus := types.ToolResultStatusSuccess
					if toolResponse.IsError {
						toolResultStatus = types.ToolResultStatusError
						if len(toolResponse.Content) > 0 {
							for _, mcpContent := range toolResponse.Content {
								bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: mcpContent.Text})
							}
						} else {
							bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: fmt.Sprintf("Tool '%s' reported an error but returned no content.", toolNameString)})
						}
					} else if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
						bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: ""})
					} else {
						for _, mcpContent := range toolResponse.Content {
							bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{Value: mcpContent.Text})
						}
					}
					toolResultBlock = types.ContentBlockMemberToolResult{
						Value: types.ToolResultBlock{
							ToolUseId: aws.String(toolUseIDString),
							Content:   bedrockResultContent,
							Status:    toolResultStatus,
						},
					}
				}

				userToolResultMessage := types.Message{
					Role:    types.ConversationRoleUser,
					Content: []types.ContentBlock{&toolResultBlock},
				}
				goroutineHistory = append(goroutineHistory, userToolResultMessage)

				continue OuterLoop
			}

			if conversationComplete {
				responseChan <- StreamingLLMResponse{Done: true}
				return
			}

			responseChan <- StreamingLLMResponse{Done: true}
			return

		}

		responseChan <- StreamingLLMResponse{Error: fmt.Errorf("streaming: max tool iterations (%d) reached", maxIterations), Done: true}

	}()

	return responseChan, nil
}
