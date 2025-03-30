package goai

import (
	"context"
	"fmt"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiModelService interface {
	StartChat(initialHistory []*genai.Content) ChatSessionService
	ConfigureModel(config *genai.GenerationConfig, tools []*genai.Tool) error
	GenerateContentStream(ctx context.Context, parts ...genai.Part) (StreamIteratorService, error)
}

type ChatSessionService interface {
	SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error)
	GetHistory() []*genai.Content
	AppendHistory(content *genai.Content)
}

type StreamIteratorService interface {
	Next() (*genai.GenerateContentResponse, error)
}

// realGeminiService implements GeminiModelService using the genai client
type realGeminiService struct {
	model *genai.GenerativeModel // Keep a reference to the configured model
}

// realChatSessionService implements ChatSessionService using genai.ChatSession
type realChatSessionService struct {
	cs *genai.ChatSession // Holds the actual genai chat session
}

// realStreamIteratorService implements StreamIteratorService
type realStreamIteratorService struct {
	iter *genai.GenerateContentResponseIterator
}

func NewRealGeminiService(ctx context.Context, apiKey, modelName string) (GeminiModelService, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create real genai client: %w", err)
	}
	// Client closing needs to be handled, maybe return client too or handle in provider Close
	model := client.GenerativeModel(modelName)
	return &realGeminiService{model: model}, nil
	// TODO: Need a way to close the client associated with this service
}

func (r *realGeminiService) ConfigureModel(config *genai.GenerationConfig, tools []*genai.Tool) error {
	r.model.GenerationConfig = *config // Assume config is never nil here based on calling code
	r.model.Tools = tools
	// Potentially return error if configuration is invalid, though SDK might handle lazily
	return nil
}

func (r *realGeminiService) StartChat(initialHistory []*genai.Content) ChatSessionService {
	cs := r.model.StartChat()
	cs.History = initialHistory // Set the initial history provided by the caller
	return &realChatSessionService{cs: cs}
}

func (r *realGeminiService) GenerateContentStream(ctx context.Context, parts ...genai.Part) (StreamIteratorService, error) {
	// Error handling might occur here if config is bad, but often happens during iteration
	iter := r.model.GenerateContentStream(ctx, parts...)
	return &realStreamIteratorService{iter: iter}, nil
}

// --- realChatSessionService Methods ---

func (rcs *realChatSessionService) SendMessage(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	resp, err := rcs.cs.SendMessage(ctx, parts...)
	// IMPORTANT: The real genai.ChatSession automatically updates its internal history.
	// The AppendHistory method might not be strictly necessary for the *real* implementation
	// if we rely on the SDK's internal state, but the interface needs it for mocking/consistency.
	// Let's assume the provider will call AppendHistory based on the response.
	return resp, err
}

func (rcs *realChatSessionService) GetHistory() []*genai.Content {
	// Return a copy to prevent external modification? For now, return direct reference.
	return rcs.cs.History
}

func (rcs *realChatSessionService) AppendHistory(content *genai.Content) {
	// This method reflects the history update that SendMessage does internally,
	// allowing the provider to keep its view consistent or for mocks to work.
	rcs.cs.History = append(rcs.cs.History, content)
}

// --- realStreamIteratorService Methods ---

func (rsi *realStreamIteratorService) Next() (*genai.GenerateContentResponse, error) {
	// Directly call the underlying iterator's Next method
	return rsi.iter.Next()
}
