package goai

import (
	"fmt"
	"github.com/google/uuid"
	"sync"
	"time"
)

// ChatHistoryStorage defines the interface for conversation history storage
type ChatHistoryStorage interface {
	// CreateChat initializes a new chat conversation
	CreateChat() (*ChatHistory, error)

	// AddMessage adds a new message to an existing conversation
	AddMessage(chatID uuid.UUID, message ChatHistoryMessage) error

	// GetChat retrieves a conversation by its ChatUUID
	GetChat(id uuid.UUID) (*ChatHistory, error)

	// ListChatHistories returns all stored conversations
	ListChatHistories() ([]ChatHistory, error)

	// DeleteChat removes a conversation by its ChatUUID
	DeleteChat(id uuid.UUID) error
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
func (s *InMemoryChatHistoryStorage) CreateChat() (*ChatHistory, error) {
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
func (s *InMemoryChatHistoryStorage) AddMessage(chatID uuid.UUID, message ChatHistoryMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, exists := s.conversations[chatID]
	if !exists {
		return fmt.Errorf("chat with ID %s not found", chatID)
	}

	chat.Messages = append(chat.Messages, message)
	return nil
}

// GetChat retrieves a conversation by its ChatUUID
func (s *InMemoryChatHistoryStorage) GetChat(id uuid.UUID) (*ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chat, exists := s.conversations[id]
	if !exists {
		return nil, fmt.Errorf("chat with ID %s not found", id)
	}

	return chat, nil
}

// ListChatHistories returns all stored conversations
func (s *InMemoryChatHistoryStorage) ListChatHistories() ([]ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chats := make([]ChatHistory, 0, len(s.conversations))
	for _, chat := range s.conversations {
		chats = append(chats, *chat)
	}

	return chats, nil
}

// DeleteChat removes a conversation by its ChatUUID
func (s *InMemoryChatHistoryStorage) DeleteChat(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.conversations[id]; !exists {
		return fmt.Errorf("chat with ID %s not found", id)
	}

	delete(s.conversations, id)
	return nil
}
