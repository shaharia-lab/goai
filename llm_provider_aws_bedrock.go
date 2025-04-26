package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// NewBedrockClientWrapper creates a new wrapper for bedrockruntime.Client
func NewBedrockClientWrapper(client *bedrockruntime.Client) BedrockClient {
	return &BedrockClientWrapper{client: client}
}

// BedrockLLMProvider implements the LLMProvider interface using AWS Bedrock's official Go SDK.
type BedrockLLMProvider struct {
	client BedrockClient
	model  string
	logger Logger
}

// BedrockProviderConfig holds the configuration options for creating a Bedrock provider
type BedrockProviderConfig struct {
	Client BedrockClient
	Model  string
}

// NewBedrockLLMProvider creates a new Bedrock provider with the specified configuration.
// If no model is specified, it defaults to Claude 3.5 Sonnet.
func NewBedrockLLMProvider(config BedrockProviderConfig, log Logger) *BedrockLLMProvider {
	if config.Model == "" {
		config.Model = "anthropic.claude-3-5-sonnet-20240620-v1:0"
	}

	return &BedrockLLMProvider{
		client: config.Client,
		model:  config.Model,
		logger: log,
	}
}

// GetResponse generates a response using Bedrock's Converse API for the given messages and configuration.
// It supports different message roles (user, assistant), system messages, and tool calling.
// It handles multi-turn conversations automatically when tools are used.
func (p *BedrockLLMProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()

	var totalInputTokens, totalOutputTokens int32

	var bedrockMessages []types.Message
	var systemPrompts []types.SystemContentBlock

	for _, msg := range messages {
		switch msg.Role {
		case SystemRole:
			systemPrompts = append(systemPrompts, &types.SystemContentBlockMemberText{
				Value: msg.Text,
			})
		case UserRole:
			bedrockMessages = append(bedrockMessages, types.Message{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: msg.Text},
				},
			})
		case AssistantRole:
			bedrockMessages = append(bedrockMessages, types.Message{
				Role: types.ConversationRoleAssistant,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: msg.Text},
				},
			})
		default:
		}
	}

	var toolConfig *types.ToolConfiguration
	if config.toolsProvider != nil {
		tools, err := config.toolsProvider.ListTools(ctx, config.allowedTools)
		if err != nil {
			return LLMResponse{}, fmt.Errorf("error listing tools: %w", err)
		}

		if len(tools) > 0 {
			var bedrockTools []types.Tool
			for _, tool := range tools {
				var schemaDoc map[string]interface{}
				if err := json.Unmarshal(tool.InputSchema, &schemaDoc); err != nil {
					p.logger.WithErr(err).WithFields(map[string]interface{}{
						"tool_name": tool.Name,
					}).Error("failed to unmarshal tool input schema for bedrock client")
					return LLMResponse{}, fmt.Errorf("failed to unmarshal tool input schema for '%s': %w", tool.Name, err)
				}

				toolSpec := types.ToolSpecification{
					Name:        aws.String(tool.Name),
					Description: aws.String(tool.Description),
					InputSchema: &types.ToolInputSchemaMemberJson{
						Value: document.NewLazyDocument(schemaDoc),
					},
				}

				bedrockTool := types.ToolMemberToolSpec{
					Value: toolSpec,
				}

				bedrockTools = append(bedrockTools, &bedrockTool)
			}
			toolConfig = &types.ToolConfiguration{
				Tools: bedrockTools,
			}
		}
	}

	var finalResponseTextBuilder strings.Builder
	inferenceCfg := &types.InferenceConfiguration{
		MaxTokens: aws.Int32(int32(config.maxToken)),
	}

	if config.temperature > 0 {
		inferenceCfg.Temperature = aws.Float32(float32(config.temperature))
	}

	if config.topP > 0 {
		inferenceCfg.TopP = aws.Float32(float32(config.topP))
	}

	converseInput := &bedrockruntime.ConverseInput{
		ModelId:         &p.model,
		InferenceConfig: inferenceCfg,
		System:          systemPrompts,
		ToolConfig:      toolConfig,
	}

	if config.enableThinking && config.thinkingBudgetToken > 0 {
		thinkingConfig := map[string]interface{}{
			"thinking": map[string]interface{}{
				"type":          "enabled",
				"budget_tokens": config.thinkingBudgetToken,
			},
		}

		thinkingDoc := document.NewLazyDocument(thinkingConfig)
		converseInput.AdditionalModelRequestFields = thinkingDoc
	}

	logFields := map[string]interface{}{
		"model":               p.model,
		"max_tokens":          config.maxToken,
		"top_p":               config.topP,
		"temperature":         config.temperature,
		"top_k":               config.topK,
		"system_prompt_count": len(systemPrompts),
		"thinking_budget":     config.thinkingBudgetToken,
		"enable_thinking":     config.enableThinking,
	}
	if config.toolsProvider != nil && toolConfig != nil {
		logFields["tools_count"] = len(toolConfig.Tools)
	}

	iterations := 0
	for {
		converseInput.Messages = bedrockMessages

		p.logger.WithFields(logFields).WithFields(map[string]interface{}{
			"iteration":                    iterations,
			"converse_input_message_count": len(bedrockMessages),
		}).Debug("BedrockLLMProvider: Sending request to Bedrock Converse API")

		output, err := p.client.Converse(ctx, converseInput)
		if err != nil {
			p.logger.WithErr(err).Error("BedrockLLMProvider: Bedrock Converse API error")
			return LLMResponse{}, fmt.Errorf("bedrock Converse API call failed: %w", err)
		}

		totalInputTokens += *output.Usage.InputTokens
		totalOutputTokens += *output.Usage.OutputTokens

		msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage)
		if !ok {
			if output.StopReason == types.StopReasonEndTurn || output.StopReason == types.StopReasonMaxTokens {
				break
			}
			if output.StopReason != types.StopReasonToolUse {
				break
			}

			p.logger.WithErr(err).Error("BedrockLLMProvider: unexpected Bedrock response")
			return LLMResponse{}, fmt.Errorf("unexpected Bedrock response: stop reason is %s, but output is not a message", output.StopReason)
		}

		assistantMessage := msgOutput.Value
		bedrockMessages = append(bedrockMessages, assistantMessage)

		var toolResultsContent []types.ContentBlock
		var hasToolUse = false

		for _, block := range assistantMessage.Content {
			switch content := block.(type) {
			case *types.ContentBlockMemberText:
				finalResponseTextBuilder.WriteString(content.Value)

			case *types.ContentBlockMemberToolUse:
				hasToolUse = true
				toolUseID := *content.Value.ToolUseId
				toolName := *content.Value.Name
				toolInput := content.Value.Input

				if config.toolsProvider == nil {
					return LLMResponse{}, fmt.Errorf("model requested tool '%s', but no toolsProvider is configured", toolName)
				}

				inputBytes, err := toolInput.MarshalSmithyDocument()
				if err != nil {
					return LLMResponse{}, fmt.Errorf("failed to marshal tool input for '%s': %w", toolName, err)
				}

				toolResponse, err := config.toolsProvider.ExecuteTool(ctx, CallToolParams{
					Name:      toolName,
					Arguments: inputBytes,
				})
				if err != nil {
					toolResultContent := &types.ContentBlockMemberToolResult{
						Value: types.ToolResultBlock{
							ToolUseId: aws.String(toolUseID),
							Status:    types.ToolResultStatusError,
							Content: []types.ToolResultContentBlock{
								&types.ToolResultContentBlockMemberText{
									Value: fmt.Sprintf("Error executing tool '%s': %v", toolName, err),
								},
							},
						},
					}
					toolResultsContent = append(toolResultsContent, toolResultContent)
					continue
				}

				if toolResponse.Content == nil || len(toolResponse.Content) == 0 {
					toolResultContent := &types.ContentBlockMemberToolResult{
						Value: types.ToolResultBlock{
							ToolUseId: aws.String(toolUseID),
							Status:    types.ToolResultStatusSuccess,
							Content: []types.ToolResultContentBlock{
								&types.ToolResultContentBlockMemberText{
									Value: "",
								},
							},
						},
					}
					toolResultsContent = append(toolResultsContent, toolResultContent)
					continue
				}

				var bedrockResultContent []types.ToolResultContentBlock
				for _, ontent := range toolResponse.Content {
					bedrockResultContent = append(bedrockResultContent, &types.ToolResultContentBlockMemberText{
						Value: ontent.Text,
					})
				}

				toolResultStatus := types.ToolResultStatusSuccess
				if toolResponse.IsError {
					toolResultStatus = types.ToolResultStatusError
				}

				toolResultBlock := &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: aws.String(toolUseID),
						Content:   bedrockResultContent,
						Status:    toolResultStatus,
					},
				}
				toolResultsContent = append(toolResultsContent, toolResultBlock)

			default:
			}
		}

		if !hasToolUse {
			p.logger.WithFields(map[string]interface{}{
				"loop_nth": iterations,
			}).Debug(
				"BedrockLLMProvider: no tool use in response, stopping iteration")
			break
		}

		if len(toolResultsContent) > 0 {
			userToolResultMessage := types.Message{
				Role:    types.ConversationRoleUser,
				Content: toolResultsContent,
			}
			bedrockMessages = append(bedrockMessages, userToolResultMessage)
		}

		if iterations > config.maxIterations {
			return LLMResponse{}, errors.New("max conversation turns exceeded")
		}

		iterations++
	}

	finalResponseText := strings.TrimSpace(finalResponseTextBuilder.String())
	completionTime := time.Since(startTime).Seconds()

	logFields["total_input_tokens"] = totalInputTokens
	logFields["total_output_tokens"] = totalOutputTokens
	logFields["total_iterations"] = iterations
	logFields["final_response_length"] = len(finalResponseText)
	logFields["duration"] = completionTime
	p.logger.WithFields(logFields).Debug("BedrockLLMProvider: Bedrock Converse API response received")

	return LLMResponse{
		Text:             finalResponseText,
		TotalInputToken:  int(totalInputTokens),
		TotalOutputToken: int(totalOutputTokens),
		CompletionTime:   completionTime,
	}, nil
}
