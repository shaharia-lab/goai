package goai

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestNewInMemoryChatHistoryStorage(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	assert.NotNil(t, storage)
	assert.NotNil(t, storage.conversations)
}

func TestInMemoryChatHistoryStorage_CreateChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()

	chat, err := storage.CreateChat()

	assert.NoError(t, err)
	assert.NotNil(t, chat)
	assert.NotEqual(t, uuid.Nil, chat.UUID)
	assert.Empty(t, chat.Messages)
	assert.NotZero(t, chat.CreatedAt)

	// Verify the chat was stored
	storedChat, exists := storage.conversations[chat.UUID]
	assert.True(t, exists)
	assert.Equal(t, chat, storedChat)
}

func TestInMemoryChatHistoryStorage_AddMessage(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	chat, _ := storage.CreateChat()

	message := ChatHistoryMessage{
		LLMMessage: LLMMessage{
			Role: "user",
			Text: "test message",
		},
		GeneratedAt: time.Now(),
	}

	err := storage.AddMessage(chat.UUID, message)
	assert.NoError(t, err)

	// Verify message was added
	storedChat, _ := storage.GetChat(chat.UUID)
	assert.Len(t, storedChat.Messages, 1)
	assert.Equal(t, message, storedChat.Messages[0])

	// Test adding message to non-existent chat
	err = storage.AddMessage(uuid.New(), message)
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_GetChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	chat, _ := storage.CreateChat()

	// Test getting existing chat
	storedChat, err := storage.GetChat(chat.UUID)
	assert.NoError(t, err)
	assert.Equal(t, chat, storedChat)

	// Test getting non-existent chat
	nonExistentChat, err := storage.GetChat(uuid.New())
	assert.Error(t, err)
	assert.Nil(t, nonExistentChat)
}

func TestInMemoryChatHistoryStorage_ListChatHistories(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()

	// Test empty storage
	chats, err := storage.ListChatHistories()
	assert.NoError(t, err)
	assert.Empty(t, chats)

	// Create some chats
	chat1, _ := storage.CreateChat()
	chat2, _ := storage.CreateChat()

	chats, err = storage.ListChatHistories()
	assert.NoError(t, err)
	assert.Len(t, chats, 2)

	// Verify both chats are in the list
	chatMap := make(map[uuid.UUID]bool)
	for _, chat := range chats {
		chatMap[chat.UUID] = true
	}
	assert.True(t, chatMap[chat1.UUID])
	assert.True(t, chatMap[chat2.UUID])
}

func TestInMemoryChatHistoryStorage_DeleteChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	chat, _ := storage.CreateChat()

	// Test deleting existing chat
	err := storage.DeleteChat(chat.UUID)
	assert.NoError(t, err)

	// Verify chat was deleted
	_, exists := storage.conversations[chat.UUID]
	assert.False(t, exists)

	// Test deleting non-existent chat
	err = storage.DeleteChat(uuid.New())
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_Concurrency(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	chat, _ := storage.CreateChat()

	// Test concurrent message adding
	done := make(chan bool)
	messageCount := 100

	for i := 0; i < messageCount; i++ {
		go func(idx int) {
			message := ChatHistoryMessage{
				LLMMessage: LLMMessage{
					Role: "user",
					Text: fmt.Sprintf("message %d", idx),
				},
				GeneratedAt: time.Now(),
			}
			err := storage.AddMessage(chat.UUID, message)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < messageCount; i++ {
		<-done
	}

	// Verify all messages were added
	storedChat, err := storage.GetChat(chat.UUID)
	assert.NoError(t, err)
	assert.Len(t, storedChat.Messages, messageCount)
}
