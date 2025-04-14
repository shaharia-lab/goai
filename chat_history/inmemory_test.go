package chat_history

import (
	"context"
	"fmt"
	"github.com/shaharia-lab/goai"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewInMemoryChatHistoryStorage(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	assert.NotNil(t, storage)
	assert.NotNil(t, storage.conversations)
}

func TestInMemoryChatHistoryStorage_CreateChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()

	chat, err := storage.CreateChat(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, chat)
	assert.NotEqual(t, uuid.Nil, chat.SessionID)
	assert.Empty(t, chat.Messages)
	assert.NotZero(t, chat.CreatedAt)

	// Verify the chat was stored
	storedChat, exists := storage.conversations[chat.SessionID]
	assert.True(t, exists)
	assert.Equal(t, chat, storedChat)
}

func TestInMemoryChatHistoryStorage_AddMessage(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

	message := ChatHistoryMessage{
		LLMMessage: goai.LLMMessage{
			Role: "user",
			Text: "test message",
		},
		GeneratedAt: time.Now(),
	}

	err := storage.AddMessage(ctx, chat.SessionID, message)
	assert.NoError(t, err)

	// Verify message was added
	storedChat, _ := storage.GetChat(ctx, chat.SessionID)
	assert.Len(t, storedChat.Messages, 1)
	assert.Equal(t, message, storedChat.Messages[0])

	// Test adding message to non-existent chat
	err = storage.AddMessage(ctx, uuid.New().String(), message)
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_GetChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()

	chat, _ := storage.CreateChat(ctx)

	// Test getting existing chat
	storedChat, err := storage.GetChat(ctx, chat.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, chat, storedChat)

	// Test getting non-existent chat
	nonExistentChat, err := storage.GetChat(ctx, uuid.New().String())
	assert.Error(t, err)
	assert.Nil(t, nonExistentChat)
}

func TestInMemoryChatHistoryStorage_ListChatHistories(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()

	// Test empty storage
	chats, err := storage.ListChatHistories(ctx)
	assert.NoError(t, err)
	assert.Empty(t, chats)

	// Create some chats
	chat1, _ := storage.CreateChat(ctx)
	chat2, _ := storage.CreateChat(ctx)

	chats, err = storage.ListChatHistories(ctx)
	assert.NoError(t, err)
	assert.Len(t, chats, 2)

	// Verify both chats are in the list
	chatMap := make(map[string]bool)
	for _, chat := range chats {
		chatMap[chat.SessionID] = true
	}
	assert.True(t, chatMap[chat1.SessionID])
	assert.True(t, chatMap[chat2.SessionID])
}

func TestInMemoryChatHistoryStorage_DeleteChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

	// Test deleting existing chat
	err := storage.DeleteChat(ctx, chat.SessionID)
	assert.NoError(t, err)

	// Verify chat was deleted
	_, exists := storage.conversations[chat.SessionID]
	assert.False(t, exists)

	// Test deleting non-existent chat
	err = storage.DeleteChat(ctx, uuid.New().String())
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_Concurrency(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

	// Test concurrent message adding
	done := make(chan bool)
	messageCount := 100

	for i := 0; i < messageCount; i++ {
		go func(idx int) {
			message := ChatHistoryMessage{
				LLMMessage: goai.LLMMessage{
					Role: "user",
					Text: fmt.Sprintf("message %d", idx),
				},
				GeneratedAt: time.Now(),
			}
			err := storage.AddMessage(ctx, chat.SessionID, message)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < messageCount; i++ {
		<-done
	}

	// Verify all messages were added
	storedChat, err := storage.GetChat(ctx, chat.SessionID)
	assert.NoError(t, err)
	assert.Len(t, storedChat.Messages, messageCount)
}
