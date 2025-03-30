package goai

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiModelService defines the interface for interacting with the Gemini model
type GeminiModelService interface {
	StartChat(initialHistory []*genai.Content) ChatSessionService
	ConfigureModel(config *genai.GenerationConfig, tools []*genai.Tool) error
	GenerateContentStream(ctx context.Context, parts ...genai.Part) (StreamIteratorService, error)
}

// ChatSessionService defines the interface for chat session management
type ChatSessionService interface {
	SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error)
	GetHistory() []*genai.Content
	AppendHistory(content *genai.Content)
}

// StreamIteratorService defines the interface for streaming content generation
type StreamIteratorService interface {
	Next() (*genai.GenerateContentResponse, error)
}

// GoogleGeminiService implements GeminiModelService using the genai client
type GoogleGeminiService struct {
	model *genai.GenerativeModel // Keep a reference to the configured model
}

// GoogleGeminiChatSessionService implements ChatSessionService using genai.ChatSession
type GoogleGeminiChatSessionService struct {
	cs *genai.ChatSession // Holds the actual genai chat session
}

// GoogleGeminiStreamIteratorService implements StreamIteratorService
type GoogleGeminiStreamIteratorService struct {
	iter *genai.GenerateContentResponseIterator
}

// NewGoogleGeminiService creates a new instance of GoogleGeminiService
func NewGoogleGeminiService(apiKey, modelName string) (GeminiModelService, error) {
	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create real genai client: %w", err)
	}
	model := client.GenerativeModel(modelName)
	return &GoogleGeminiService{model: model}, nil
}

// ConfigureModel configures the model with the provided generation config and tools
func (g *GoogleGeminiService) ConfigureModel(config *genai.GenerationConfig, tools []*genai.Tool) error {
	g.model.GenerationConfig = *config
	g.model.Tools = tools
	return nil
}

// StartChat initializes a new chat session with the provided initial history
func (g *GoogleGeminiService) StartChat(initialHistory []*genai.Content) ChatSessionService {
	cs := g.model.StartChat()
	cs.History = initialHistory
	return &GoogleGeminiChatSessionService{cs: cs}
}

// GenerateContentStream generates content in a streaming manner
func (g *GoogleGeminiService) GenerateContentStream(ctx context.Context, parts ...genai.Part) (StreamIteratorService, error) {
	iter := g.model.GenerateContentStream(ctx, parts...)
	return &GoogleGeminiStreamIteratorService{iter: iter}, nil
}

// SendMessage sends a message to the chat session and returns the response
func (ggcss *GoogleGeminiChatSessionService) SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	resp, err := ggcss.cs.SendMessage(ctx, parts...)
	return resp, err
}

// GetHistory retrieves the chat history
func (ggcss *GoogleGeminiChatSessionService) GetHistory() []*genai.Content {
	return ggcss.cs.History
}

// AppendHistory appends a new content to the chat history
func (ggcss *GoogleGeminiChatSessionService) AppendHistory(content *genai.Content) {
	ggcss.cs.History = append(ggcss.cs.History, content)
}

// Next retrieves the next content from the streaming iterator
func (ggsis *GoogleGeminiStreamIteratorService) Next() (*genai.GenerateContentResponse, error) {
	return ggsis.iter.Next()
}
