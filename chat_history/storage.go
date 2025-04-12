package chat_history

import (
	"context"
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
