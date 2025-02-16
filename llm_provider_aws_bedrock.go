package goai

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
func (p *BedrockLLMProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
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

	output, err := p.client.Converse(ctx, input)
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

// GetStreamingResponse generates a streaming response using AWS Bedrock's API for the given messages and configuration.
// It returns a channel that receives chunks of the response as they're generated.
//
// The method supports different message roles (user, assistant) and handles context cancellation.
// The returned channel will be closed when the response is complete or if an error occurs.
//
// The returned StreamingLLMResponse contains:
//   - Text: The text chunk from the model
//   - Done: Boolean indicating if this is the final message
//   - Error: Any error that occurred during streaming
//   - TokenCount: Number of tokens in this chunk
//
// Example:
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//	    "github.com/aws/aws-sdk-go-v2/aws"
//	    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
//	    "github.com/shaharia-lab/goai"
//	)
//
//	func main() {
//	    // Create Bedrock LLM Provider
//	    llmProvider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
//	        Client: bedrockruntime.New(aws.Config{}),
//	        Model:  "anthropic.claude-3-sonnet-20240229-v1:0",
//	    })
//
//	    // Configure LLM Request
//	    llm := goai.NewLLMRequest(goai.NewRequestConfig(
//	        goai.WithMaxToken(100),
//	        goai.WithTemperature(0.7),
//	    ), llmProvider)
//
//	    // Generate streaming response
//	    stream, err := llm.GenerateStream(context.Background(), []goai.LLMMessage{
//	        {Role: goai.UserRole, Text: "Explain quantum computing"},
//	    })
//
//	    if err != nil {
//	        panic(err)
//	    }
//
//	    for resp := range stream {
//	        if resp.Error != nil {
//	            fmt.Printf("Error: %v\n", resp.Error)
//	            break
//	        }
//	        if resp.Done {
//	            break
//	        }
//	        fmt.Print(resp.Text)
//	    }
//	}
//
// Note: The streaming response must be fully consumed or the context must be
// cancelled to prevent resource leaks.
func (p *BedrockLLMProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
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

	input := &bedrockruntime.ConverseStreamInput{
		ModelId:  &p.model,
		Messages: bedrockMessages,
		InferenceConfig: &types.InferenceConfiguration{
			Temperature: aws.Float32(float32(config.Temperature)),
			TopP:        aws.Float32(float32(config.TopP)),
			MaxTokens:   aws.Int32(int32(config.MaxToken)),
		},
	}

	output, err := p.client.ConverseStream(ctx, input)
	if err != nil {
		return nil, err
	}

	responseChan := make(chan StreamingLLMResponse, 100)

	go func() {
		defer close(responseChan)

		for event := range output.GetStream().Events() {
			select {
			case <-ctx.Done():
				responseChan <- StreamingLLMResponse{
					Error: ctx.Err(),
					Done:  true,
				}
				return
			default:
				switch v := event.(type) {
				case *types.ConverseStreamOutputMemberContentBlockDelta:
					if delta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
						responseChan <- StreamingLLMResponse{
							Text:       delta.Value,
							TokenCount: 1,
						}
					}
				case *types.ConverseStreamOutputMemberMessageStop:
					responseChan <- StreamingLLMResponse{Done: true}
					return
				}
			}
		}

		if err := output.GetStream().Err(); err != nil {
			responseChan <- StreamingLLMResponse{
				Error: err,
				Done:  true,
			}
		}
	}()

	return responseChan, nil
}
