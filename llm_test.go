package ai

import (
	"context"
	"errors"
	"testing"
)

type mockProvider struct {
	response        LLMResponse
	err             error
	streamResponses []StreamingLLMResponse
	streamErr       error
}

func (m *mockProvider) GetResponse(messages []LLMMessage, _ LLMRequestConfig) (LLMResponse, error) {
	return m.response, m.err
}

func (m *mockProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}

	ch := make(chan StreamingLLMResponse, len(m.streamResponses))
	go func() {
		defer close(ch)
		for _, resp := range m.streamResponses {
			select {
			case <-ctx.Done():
				return
			case ch <- resp:
			}
		}
	}()
	return ch, nil
}

func TestLLMRequest_Generate(t *testing.T) {
	tests := []struct {
		name           string
		config         LLMRequestConfig
		mockResponse   LLMResponse
		mockError      error
		expectedError  bool
		expectedOutput LLMResponse
	}{
		{
			name: "successful generation",
			config: LLMRequestConfig{
				MaxToken:    100,
				TopP:        0.9,
				Temperature: 0.7,
			},
			mockResponse: LLMResponse{
				Text:             "Hello, world!",
				TotalInputToken:  5,
				TotalOutputToken: 3,
				CompletionTime:   0.5,
			},
			expectedOutput: LLMResponse{
				Text:             "Hello, world!",
				TotalInputToken:  5,
				TotalOutputToken: 3,
				CompletionTime:   0.5,
			},
		},
		{
			name: "provider error",
			config: LLMRequestConfig{
				MaxToken:    100,
				TopP:        0.9,
				Temperature: 0.7,
			},
			mockError:     errors.New("provider error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockProvider{
				response: tt.mockResponse,
				err:      tt.mockError,
			}

			request := NewLLMRequest(tt.config, provider)
			response, err := request.Generate([]LLMMessage{{
				Role: "user",
				Text: "test prompt",
			}})

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if response.Text != tt.expectedOutput.Text {
				t.Errorf("expected text %q, got %q", tt.expectedOutput.Text, response.Text)
			}
			if response.TotalInputToken != tt.expectedOutput.TotalInputToken {
				t.Errorf("expected input tokens %d, got %d", tt.expectedOutput.TotalInputToken, response.TotalInputToken)
			}
			if response.TotalOutputToken != tt.expectedOutput.TotalOutputToken {
				t.Errorf("expected output tokens %d, got %d", tt.expectedOutput.TotalOutputToken, response.TotalOutputToken)
			}
		})
	}
}

func TestLLMRequest_GenerateStream(t *testing.T) {
	tests := []struct {
		name            string
		config          LLMRequestConfig
		messages        []LLMMessage
		streamResponses []StreamingLLMResponse
		streamErr       error
		wantErr         bool
	}{
		{
			name: "successful streaming",
			config: LLMRequestConfig{
				MaxToken: 100,
			},
			messages: []LLMMessage{
				{Role: UserRole, Text: "Hello"},
			},
			streamResponses: []StreamingLLMResponse{
				{Text: "Hello", TokenCount: 1},
				{Text: "World", TokenCount: 1},
				{Done: true},
			},
		},
		{
			name: "provider error",
			config: LLMRequestConfig{
				MaxToken: 100,
			},
			messages: []LLMMessage{
				{Role: UserRole, Text: "Hello"},
			},
			streamErr: errors.New("stream error"),
			wantErr:   true,
		},
		{
			name: "context cancellation",
			config: LLMRequestConfig{
				MaxToken: 100,
			},
			messages: []LLMMessage{
				{Role: UserRole, Text: "Hello"},
			},
			streamResponses: []StreamingLLMResponse{
				{Text: "Hello", TokenCount: 1},
				{Error: context.Canceled, Done: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockProvider{
				streamResponses: tt.streamResponses,
				streamErr:       tt.streamErr,
			}

			request := NewLLMRequest(tt.config, provider)
			stream, err := request.GenerateStream(context.Background(), tt.messages)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateStream() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			var got []StreamingLLMResponse
			for resp := range stream {
				got = append(got, resp)
			}

			if len(got) != len(tt.streamResponses) {
				t.Errorf("expected %d responses, got %d", len(tt.streamResponses), len(got))
				return
			}

			for i, want := range tt.streamResponses {
				if got[i].Text != want.Text {
					t.Errorf("response[%d].Text = %v, want %v", i, got[i].Text, want.Text)
				}
				if got[i].Done != want.Done {
					t.Errorf("response[%d].Done = %v, want %v", i, got[i].Done, want.Done)
				}
				if got[i].Error != want.Error {
					t.Errorf("response[%d].Error = %v, want %v", i, got[i].Error, want.Error)
				}
			}
		})
	}
}
