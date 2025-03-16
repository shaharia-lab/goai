package goai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ChatHistoryStorage defines the interface for conversation history storage
type ChatHistoryStorage interface {
	// CreateChat initializes a new chat conversation
	CreateChat(ctx context.Context) (*ChatHistory, error)

	// AddMessage adds a new message to an existing conversation
	AddMessage(ctx context.Context, uuid uuid.UUID, message ChatHistoryMessage) error

	// GetChat retrieves a conversation by its ChatUUID
	GetChat(ctx context.Context, uuid uuid.UUID) (*ChatHistory, error)

	// ListChatHistories returns all stored conversations
	ListChatHistories(ctx context.Context) ([]ChatHistory, error)

	// DeleteChat removes a conversation by its ChatUUID
	DeleteChat(ctx context.Context, uuid uuid.UUID) error
}

// InMemoryChatHistoryStorage is an in-memory implementation of ChatHistoryStorage
type InMemoryChatHistoryStorage struct {
	conversations map[uuid.UUID]*ChatHistory
	mu            sync.RWMutex
}

// NewInMemoryChatHistoryStorage creates a new instance of InMemoryChatHistoryStorage
func NewInMemoryChatHistoryStorage() *InMemoryChatHistoryStorage {
	return &InMemoryChatHistoryStorage{
		conversations: make(map[uuid.UUID]*ChatHistory),
	}
}

// CreateChat initializes a new chat conversation
func (s *InMemoryChatHistoryStorage) CreateChat(ctx context.Context) (*ChatHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat := &ChatHistory{
		UUID:      uuid.New(),
		Messages:  []ChatHistoryMessage{},
		CreatedAt: time.Now(),
	}

	s.conversations[chat.UUID] = chat
	return chat, nil
}

// AddMessage adds a new message to an existing conversation
func (s *InMemoryChatHistoryStorage) AddMessage(ctx context.Context, uuid uuid.UUID, message ChatHistoryMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, exists := s.conversations[uuid]
	if !exists {
		return fmt.Errorf("chat with ID %s not found", uuid)
	}

	chat.Messages = append(chat.Messages, message)
	return nil
}

// GetChat retrieves a conversation by its ChatUUID
func (s *InMemoryChatHistoryStorage) GetChat(ctx context.Context, uuid uuid.UUID) (*ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chat, exists := s.conversations[uuid]
	if !exists {
		return nil, fmt.Errorf("chat with ID %s not found", uuid)
	}

	return chat, nil
}

// ListChatHistories returns all stored conversations
func (s *InMemoryChatHistoryStorage) ListChatHistories(ctx context.Context) ([]ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chats := make([]ChatHistory, 0, len(s.conversations))
	for _, chat := range s.conversations {
		chats = append(chats, *chat)
	}

	return chats, nil
}

// DeleteChat removes a conversation by its ChatUUID
func (s *InMemoryChatHistoryStorage) DeleteChat(ctx context.Context, uuid uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.conversations[uuid]; !exists {
		return fmt.Errorf("chat with ID %s not found", uuid)
	}

	delete(s.conversations, uuid)
	return nil
}
