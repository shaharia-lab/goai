package goai

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/mock"
)

// MockBedrockClient is a mock type for the BedrockClient interface
type MockBedrockClient struct {
	mock.Mock
}

// Converse provides a mock function with given fields: ctx, params, optFns
func (_m *MockBedrockClient) Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	// We are primarily testing InvokeModel, so this might not need complex logic
	// unless your provider uses it too. Return values can be configured per test.
	args := _m.Called(ctx, params, optFns)
	if rf, ok := args.Get(0).(func(context.Context, *bedrockruntime.ConverseInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}

	var r0 *bedrockruntime.ConverseOutput
	if args.Get(0) != nil {
		r0 = args.Get(0).(*bedrockruntime.ConverseOutput)
	}
	return r0, args.Error(1)
}

// ConverseStream provides a mock function with given fields: ctx, params, optFns
func (_m *MockBedrockClient) ConverseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error) {
	// Mock implementation - configure return values per test
	args := _m.Called(ctx, params, optFns)
	if rf, ok := args.Get(0).(func(context.Context, *bedrockruntime.ConverseStreamInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}
	var r0 *bedrockruntime.ConverseStreamOutput
	if args.Get(0) != nil {
		r0 = args.Get(0).(*bedrockruntime.ConverseStreamOutput)
	}
	return r0, args.Error(1)
}

// InvokeModel provides a mock function with given fields: ctx, params, optFns
func (_m *MockBedrockClient) InvokeModel(ctx context.Context, params *bedrockruntime.InvokeModelInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	// This is the core method we'll use for embedding tests.
	args := _m.Called(ctx, params, optFns)

	// Allows defining a function per test for complex logic if needed
	if rf, ok := args.Get(0).(func(context.Context, *bedrockruntime.InvokeModelInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}

	var r0 *bedrockruntime.InvokeModelOutput
	// Get the first return value (InvokeModelOutput) if it's not nil
	if args.Get(0) != nil {
		r0 = args.Get(0).(*bedrockruntime.InvokeModelOutput)
	}
	// Get the second return value (error)
	return r0, args.Error(1)
}

// --- Ensure MockBedrockClient satisfies the interface ---
var _ BedrockClient = (*MockBedrockClient)(nil)
