package goai

import (
	"context"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*SQLiteChatHistoryStorage, func()) {
	tempFile, err := os.CreateTemp("", "chat_history_test_*.db")
	require.NoError(t, err)
	tempFilePath := tempFile.Name()
	tempFile.Close()

	logger := &observability.NullLogger{}
	storage, err := NewSQLiteChatHistoryStorage(tempFilePath, logger)
	require.NoError(t, err)

	cleanup := func() {
		storage.db.Close()
		os.Remove(tempFilePath)
	}

	return storage, cleanup
}

func TestNewSQLiteChatHistoryStorage(t *testing.T) {
	tests := []struct {
		name         string
		databasePath string
		expectError  bool
	}{
		{
			name:         "Valid database path",
			databasePath: t.TempDir() + "/valid.db",
			expectError:  false,
		},
		{
			name:         "Invalid database path",
			databasePath: "/non/existent/directory/invalid.db",
			expectError:  true,
		},
	}

	logger := &observability.NullLogger{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewSQLiteChatHistoryStorage(tt.databasePath, logger)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, storage)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, storage)
				if storage != nil {
					storage.db.Close()
					os.Remove(tt.databasePath)
				}
			}
		})
	}
}

func TestInitSchema(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	var count int
	err := storage.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('chats', 'messages')").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "Expected 2 tables to be created")

	var indexCount int
	err = storage.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_messages_%'").Scan(&indexCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, indexCount, 1, "Expected at least one index to be created")

	err = storage.initSchema(ctx)
	assert.NoError(t, err, "initSchema should handle being called on an existing database")
}

func TestCreateChat(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	chat, err := storage.CreateChat(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, chat)
	assert.NotEmpty(t, chat.SessionID)

	var count int
	err = storage.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chats WHERE uuid = ?", chat.SessionID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestAddMessage(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	chat, err := storage.CreateChat(ctx)
	require.NoError(t, err)

	message := ChatHistoryMessage{
		LLMMessage: LLMMessage{
			Role: "user",
			Text: "Hello, world!",
		},
		GeneratedAt: time.Now(),
		InputToken:  10,
		OutputToken: 5,
		Metadata:    map[string]interface{}{"source": "web"},
	}

	err = storage.AddMessage(ctx, chat.SessionID, message)
	assert.NoError(t, err)

	var count int
	err = storage.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages WHERE chat_uuid = ? AND text = ?",
		chat.SessionID, message.Text).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	invalidChatID := uuid.NewString()
	err = storage.AddMessage(ctx, invalidChatID, message)
	assert.Error(t, err)
}

func TestGetChat(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	chat, err := storage.CreateChat(ctx)
	require.NoError(t, err)

	messages := []ChatHistoryMessage{
		{
			LLMMessage: LLMMessage{
				Role: "user",
				Text: "Hello",
			},
			GeneratedAt: time.Now().Add(-2 * time.Minute),
			InputToken:  5,
		},
		{
			LLMMessage: LLMMessage{
				Role: "assistant",
				Text: "Hi there!",
			},
			GeneratedAt: time.Now().Add(-1 * time.Minute),
			OutputToken: 10,
		},
		{
			LLMMessage: LLMMessage{
				Role: "user",
				Text: "How are you?",
			},
			GeneratedAt: time.Now(),
			InputToken:  12,
		},
	}

	for _, msg := range messages {
		err = storage.AddMessage(ctx, chat.SessionID, msg)
		require.NoError(t, err)
	}

	retrievedChat, err := storage.GetChat(ctx, chat.SessionID)
	assert.NoError(t, err)
	assert.Len(t, retrievedChat.Messages, 3)
	assert.Equal(t, "Hello", retrievedChat.Messages[0].Text)
	assert.Equal(t, "Hi there!", retrievedChat.Messages[1].Text)
	assert.Equal(t, "How are you?", retrievedChat.Messages[2].Text)

	invalidChatID := uuid.NewString()
	nonExistentChat, err := storage.GetChat(ctx, invalidChatID)
	assert.Error(t, err)
	assert.Nil(t, nonExistentChat)
}

func TestListChatHistories(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	chatIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		chat, err := storage.CreateChat(ctx)
		require.NoError(t, err)
		chatIDs[i] = chat.SessionID

		message := ChatHistoryMessage{
			LLMMessage: LLMMessage{
				Role: "user",
				Text: fmt.Sprintf("Message for chat %d", i),
			},
			GeneratedAt: time.Now(),
		}

		err = storage.AddMessage(ctx, chat.SessionID, message)
		require.NoError(t, err)
	}

	chats, err := storage.ListChatHistories(ctx)
	assert.NoError(t, err)
	assert.Len(t, chats, 3)

	for _, chatID := range chatIDs {
		chat, err := storage.GetChat(ctx, chatID)
		assert.NoError(t, err)
		assert.Len(t, chat.Messages, 1, "Each chat should have one message")
	}
}

func TestDeleteChat(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	chat, err := storage.CreateChat(ctx)
	require.NoError(t, err)
	chatID := chat.SessionID

	message := ChatHistoryMessage{
		LLMMessage: LLMMessage{
			Role: "user",
			Text: "Delete me",
		},
		GeneratedAt: time.Now(),
	}
	err = storage.AddMessage(ctx, chatID, message)
	require.NoError(t, err)

	_, err = storage.GetChat(ctx, chatID)
	require.NoError(t, err)

	err = storage.DeleteChat(ctx, chatID)
	assert.NoError(t, err)

	_, err = storage.GetChat(ctx, chatID)
	assert.Error(t, err, "GetChat should error after deletion")
	assert.Contains(t, err.Error(), "not found", "Error should indicate chat doesn't exist")

	nonExistentID := uuid.NewString()
	err = storage.DeleteChat(ctx, nonExistentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist", "Error should indicate chat doesn't exist")
}

func TestConcurrentAccess(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	chat, err := storage.CreateChat(ctx)
	require.NoError(t, err)

	// Test concurrent message additions
	const numGoroutines = 10
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			message := ChatHistoryMessage{
				LLMMessage: LLMMessage{
					Role: "user",
					Text: fmt.Sprintf("Message %d", idx),
				},
				GeneratedAt: time.Now(),
			}
			errCh <- storage.AddMessage(ctx, chat.SessionID, message)
		}(i)
	}

	// Collect errors
	for i := 0; i < numGoroutines; i++ {
		err := <-errCh
		assert.NoError(t, err)
	}

	// Verify all messages were added
	retrievedChat, err := storage.GetChat(ctx, chat.SessionID)
	assert.NoError(t, err)
	assert.Len(t, retrievedChat.Messages, numGoroutines)
}

func TestClose(t *testing.T) {
	storage, cleanup := setupTestDB(t)
	defer cleanup()

	// Test closing the storage
	err := storage.Close()
	assert.NoError(t, err)

	// Verify database connection is closed (trying to use it should fail)
	ctx := context.Background()
	_, err = storage.db.ExecContext(ctx, "SELECT 1")
	assert.Error(t, err)
}
