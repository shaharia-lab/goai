// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// BedrockLLMProvider implements the LLMProvider interface using AWS Bedrock's official Go SDK.
type BedrockLLMProvider struct {
	client *bedrockruntime.Client
	model  string
}

// BedrockProviderConfig holds the configuration options for creating a Bedrock provider.
type BedrockProviderConfig struct {
	Client *bedrockruntime.Client
	Model  string
}

// NewBedrockLLMProvider creates a new Bedrock provider with the specified configuration.
// If no model is specified, it defaults to Claude 3.5 Sonnet.
func NewBedrockLLMProvider(config BedrockProviderConfig) *BedrockLLMProvider {
	if config.Model == "" {
		config.Model = "anthropic.claude-3-5-sonnet-20240620-v1:0"
	}

	return &BedrockLLMProvider{
		client: config.Client,
		model:  config.Model,
	}
}

// GetResponse generates a response using Bedrock's API for the given messages and configuration.
// It supports different message roles (user, assistant) and handles them appropriately.
func (p *BedrockLLMProvider) GetResponse(messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()

	var bedrockMessages []types.Message
	for _, msg := range messages {
		role := types.ConversationRoleUser
		if msg.Role == "assistant" {
			role = types.ConversationRoleAssistant
		}

		bedrockMessages = append(bedrockMessages, types.Message{
			Role: role,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.Text,
				},
			},
		})
	}

	input := &bedrockruntime.ConverseInput{
		ModelId:  &p.model,
		Messages: bedrockMessages,
		InferenceConfig: &types.InferenceConfiguration{
			Temperature: aws.Float32(float32(config.Temperature)),
			TopP:        aws.Float32(float32(config.TopP)),
			MaxTokens:   aws.Int32(int32(config.MaxToken)),
		},
	}

	output, err := p.client.Converse(context.Background(), input)
	if err != nil {
		return LLMResponse{}, err
	}

	var responseText string
	if msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage); ok {
		for _, block := range msgOutput.Value.Content {
			if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
				responseText += textBlock.Value
			}
		}
	}

	return LLMResponse{
		Text:             responseText,
		TotalInputToken:  int(*output.Usage.InputTokens),
		TotalOutputToken: int(*output.Usage.OutputTokens),
		CompletionTime:   time.Since(startTime).Seconds(),
	}, nil
}
