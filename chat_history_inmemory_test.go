package goai

import (
	"context"
	"fmt"
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
	assert.NotNil(t, chat.Metadata)
	assert.Empty(t, chat.Metadata)

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
		LLMMessage: LLMMessage{
			Role: "user",
			Text: "test message",
		},
		GeneratedAt: time.Now(),
	}

	err := storage.AddMessage(ctx, chat.SessionID, message)
	assert.NoError(t, err)

	storedChat, _ := storage.GetChat(ctx, chat.SessionID)
	assert.Len(t, storedChat.Messages, 1)
	assert.Equal(t, message.LLMMessage, storedChat.Messages[0].LLMMessage)
	assert.Equal(t, message.GeneratedAt.Unix(), storedChat.Messages[0].GeneratedAt.Unix())
	assert.NotNil(t, storedChat.Messages[0].Metadata)

	err = storage.AddMessage(ctx, uuid.New().String(), message)
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_AddMessageWithMetadata(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

	metadata := map[string]interface{}{
		"source":      "user-query",
		"temperature": 0.7,
		"tokens":      1250,
	}

	message := ChatHistoryMessage{
		LLMMessage: LLMMessage{
			Role: "assistant",
			Text: "response with metadata",
		},
		GeneratedAt: time.Now(),
		InputToken:  100,
		OutputToken: 150,
		Metadata:    metadata,
	}

	err := storage.AddMessage(ctx, chat.SessionID, message)
	assert.NoError(t, err)

	storedChat, _ := storage.GetChat(ctx, chat.SessionID)
	assert.Len(t, storedChat.Messages, 1)
	assert.Equal(t, message.LLMMessage, storedChat.Messages[0].LLMMessage)
	assert.Equal(t, metadata, storedChat.Messages[0].Metadata)
}

func TestInMemoryChatHistoryStorage_UpdateChatMetadata(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

	storedChat, _ := storage.GetChat(ctx, chat.SessionID)
	assert.Empty(t, storedChat.Metadata)

	metadata := map[string]interface{}{
		"chat_name":    "Test Chat",
		"created_by":   "test-user",
		"is_important": true,
		"tags":         []string{"test", "example"},
	}

	err := storage.UpdateChatMetadata(ctx, chat.SessionID, metadata)
	assert.NoError(t, err)

	updatedChat, _ := storage.GetChat(ctx, chat.SessionID)
	assert.Equal(t, metadata, updatedChat.Metadata)

	err = storage.UpdateChatMetadata(ctx, uuid.New().String(), metadata)
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_GetChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()

	chat, _ := storage.CreateChat(ctx)

	storedChat, err := storage.GetChat(ctx, chat.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, chat, storedChat)

	nonExistentChat, err := storage.GetChat(ctx, uuid.New().String())
	assert.Error(t, err)
	assert.Nil(t, nonExistentChat)
}

func TestInMemoryChatHistoryStorage_ListChatHistories(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()

	chats, err := storage.ListChatHistories(ctx)
	assert.NoError(t, err)
	assert.Empty(t, chats)

	chat1, _ := storage.CreateChat(ctx)
	chat2, _ := storage.CreateChat(ctx)

	metadata1 := map[string]interface{}{"name": "Chat 1"}
	storage.UpdateChatMetadata(ctx, chat1.SessionID, metadata1)

	chats, err = storage.ListChatHistories(ctx)
	assert.NoError(t, err)
	assert.Len(t, chats, 2)

	chatMap := make(map[string]*ChatHistory)
	for i, chat := range chats {
		chatMap[chat.SessionID] = &chats[i]
	}

	assert.Contains(t, chatMap, chat1.SessionID)
	assert.Contains(t, chatMap, chat2.SessionID)

	assert.Equal(t, metadata1, chatMap[chat1.SessionID].Metadata)
	assert.Empty(t, chatMap[chat2.SessionID].Metadata)
}

func TestInMemoryChatHistoryStorage_DeleteChat(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

	err := storage.DeleteChat(ctx, chat.SessionID)
	assert.NoError(t, err)

	_, exists := storage.conversations[chat.SessionID]
	assert.False(t, exists)

	err = storage.DeleteChat(ctx, uuid.New().String())
	assert.Error(t, err)
}

func TestInMemoryChatHistoryStorage_Concurrency(t *testing.T) {
	storage := NewInMemoryChatHistoryStorage()
	ctx := context.Background()
	chat, _ := storage.CreateChat(ctx)

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
				Metadata: map[string]interface{}{
					"index": idx,
				},
			}
			err := storage.AddMessage(ctx, chat.SessionID, message)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < messageCount; i++ {
		<-done
	}

	storedChat, err := storage.GetChat(ctx, chat.SessionID)
	assert.NoError(t, err)
	assert.Len(t, storedChat.Messages, messageCount)

	indexMap := make(map[int]bool)
	for _, msg := range storedChat.Messages {
		assert.NotNil(t, msg.Metadata)
		if idx, ok := msg.Metadata["index"].(int); ok {
			indexMap[idx] = true
		}
	}

	assert.Len(t, indexMap, messageCount)
	for i := 0; i < messageCount; i++ {
		assert.True(t, indexMap[i], fmt.Sprintf("Missing message with index %d", i))
	}
}
