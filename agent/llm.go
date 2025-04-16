package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LLMProvider defines the interface for interacting with an LLM API
type LLMProvider interface {
	GenerateResponse(ctx context.Context, prompt string, options *RequestOptions) (string, error)
}

// RequestOptions contains configuration for the LLM request
type RequestOptions struct {
	Temperature float64
	MaxTokens   int
	Model       string
	Timeout     time.Duration
}

// DefaultOptions returns sensible default options
func DefaultOptions() *RequestOptions {
	return &RequestOptions{
		Temperature: 0.7,
		MaxTokens:   1000,
		Model:       "gpt-3.5-turbo", // Or whatever model you're using
		Timeout:     time.Minute * 2,
	}
}

// OpenAIProvider implements LLMProvider for the OpenAI API
type OpenAIProvider struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:  apiKey,
		BaseURL: "https://api.openai.com/v1/chat/completions",
		HTTPClient: &http.Client{
			Timeout: time.Minute * 2,
		},
	}
}

// Request and response structures for OpenAI API
type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GenerateResponse sends a request to the OpenAI API and returns the response
func (p *OpenAIProvider) GenerateResponse(ctx context.Context, prompt string, options *RequestOptions) (string, error) {
	if options == nil {
		options = DefaultOptions()
	}

	reqBody := openAIRequest{
		Model: options.Model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		Temperature: options.Temperature,
		MaxTokens:   options.MaxTokens,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL, bytes.NewBuffer(reqData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var apiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return apiResp.Choices[0].Message.Content, nil
}
