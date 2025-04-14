package goai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// InMemoryChatHistoryStorage is an in-memory implementation of ChatHistoryStorage
type InMemoryChatHistoryStorage struct {
	conversations map[string]*ChatHistory
	mu            sync.RWMutex
}

// NewInMemoryChatHistoryStorage creates a new instance of InMemoryChatHistoryStorage
func NewInMemoryChatHistoryStorage() *InMemoryChatHistoryStorage {
	return &InMemoryChatHistoryStorage{
		conversations: make(map[string]*ChatHistory),
	}
}

// CreateChat initializes a new chat conversation
func (s *InMemoryChatHistoryStorage) CreateChat(ctx context.Context) (*ChatHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat := &ChatHistory{
		SessionID: uuid.New().String(),
		Messages:  []ChatHistoryMessage{},
		CreatedAt: time.Now(),
	}

	s.conversations[chat.SessionID] = chat
	return chat, nil
}

// AddMessage adds a new message to an existing conversation
func (s *InMemoryChatHistoryStorage) AddMessage(ctx context.Context, sessionID string, message ChatHistoryMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, exists := s.conversations[sessionID]
	if !exists {
		return fmt.Errorf("chat with ID %s not found", sessionID)
	}

	chat.Messages = append(chat.Messages, message)
	return nil
}

// GetChat retrieves a conversation by its ChatUUID
func (s *InMemoryChatHistoryStorage) GetChat(ctx context.Context, sessionID string) (*ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chat, exists := s.conversations[sessionID]
	if !exists {
		return nil, fmt.Errorf("chat with ID %s not found", sessionID)
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
func (s *InMemoryChatHistoryStorage) DeleteChat(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.conversations[sessionID]; !exists {
		return fmt.Errorf("chat with ID %s not found", sessionID)
	}

	delete(s.conversations, sessionID)
	return nil
}
