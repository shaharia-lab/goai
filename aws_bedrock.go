package goai

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// BedrockClient interface for AWS Bedrock operations
type BedrockClient interface {
	Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
	ConverseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error)
	InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

// BedrockClientWrapper wraps the bedrockruntime.Client to implement the BedrockClient interface
type BedrockClientWrapper struct {
	client *bedrockruntime.Client
}

// Converse implements the BedrockClient interface
func (w *BedrockClientWrapper) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	return w.client.Converse(ctx, params, optFns...)
}

// ConverseStream implements the BedrockClient interface
func (w *BedrockClientWrapper) ConverseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error) {
	return w.client.ConverseStream(ctx, params, optFns...)
}

// InvokeModel implements the BedrockClient interface
func (w *BedrockClientWrapper) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	return w.client.InvokeModel(ctx, params, optFns...)
}
