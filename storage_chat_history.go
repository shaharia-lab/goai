package goai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
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

// Sqlite implementation of ChatHistoryStorage
// --- Adjusted SQLite Implementation ---

// SQLiteChatHistoryStorage is an SQLite implementation of ChatHistoryStorage
type SQLiteChatHistoryStorage struct {
	db     *sql.DB
	mu     sync.RWMutex // Protects against concurrent access issues if needed, though transactions help
	logger observability.Logger
}

// NewSQLiteChatHistoryStorage creates a new instance of SQLiteChatHistoryStorage
// It takes the path to the SQLite database file.
func NewSQLiteChatHistoryStorage(databasePath string, logger observability.Logger) (*SQLiteChatHistoryStorage, error) {
	db, err := sql.Open("sqlite3", databasePath+"?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	storage := &SQLiteChatHistoryStorage{
		db:     db,
		logger: logger,
	}

	if err := storage.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return storage, nil
}

// initSchema creates the necessary tables if they don't exist
func (s *SQLiteChatHistoryStorage) initSchema(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	createChatsTableSQL := `
    CREATE TABLE IF NOT EXISTS chats (
        uuid TEXT PRIMARY KEY,
        created_at DATETIME NOT NULL
    );`

	createMessagesTableSQL := `
    CREATE TABLE IF NOT EXISTS messages (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        chat_uuid TEXT NOT NULL,
        role TEXT NOT NULL, -- Stores LLMMessageRole as TEXT
        text TEXT NOT NULL, -- Stores LLMMessage.Text
        generated_at DATETIME NOT NULL, -- Stores ChatHistoryMessage.GeneratedAt
        FOREIGN KEY (chat_uuid) REFERENCES chats(uuid) ON DELETE CASCADE
    );`

	createMessagesIndexSQL := `
	CREATE INDEX IF NOT EXISTS idx_messages_chat_uuid ON messages (chat_uuid);
	`

	createMessagesTimestampIndexSQL := `
	CREATE INDEX IF NOT EXISTS idx_messages_generated_at ON messages (generated_at);
	`

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for schema init: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, createChatsTableSQL); err != nil {
		return fmt.Errorf("failed to create chats table: %w", err)
	}

	if _, err := tx.ExecContext(ctx, createMessagesTableSQL); err != nil {
		return fmt.Errorf("failed to create messages table: %w", err)
	}

	if _, err := tx.ExecContext(ctx, createMessagesIndexSQL); err != nil {
		return fmt.Errorf("failed to create messages chat index: %w", err)
	}

	if _, err := tx.ExecContext(ctx, createMessagesTimestampIndexSQL); err != nil {
		fmt.Printf("Warning: failed to create messages timestamp index: %v\n", err)
	}

	return tx.Commit()
}

// CreateChat initializes a new chat conversation in SQLite
func (s *SQLiteChatHistoryStorage) CreateChat(ctx context.Context) (*ChatHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newUUID := uuid.New()
	createdAt := time.Now().UTC()

	chat := &ChatHistory{
		UUID:      newUUID,
		Messages:  []ChatHistoryMessage{},
		CreatedAt: createdAt,
	}

	insertSQL := `INSERT INTO chats (uuid, created_at) VALUES (?, ?)`

	_, err := s.db.ExecContext(ctx, insertSQL, newUUID.String(), createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert new chat (uuid: %s): %w", newUUID, err)
	}

	return chat, nil
}

// AddMessage adds a new message to an existing conversation in SQLite
func (s *SQLiteChatHistoryStorage) AddMessage(ctx context.Context, chatUUID uuid.UUID, message ChatHistoryMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for adding message: %w", err)
	}
	defer tx.Rollback()

	var exists int
	checkSQL := `SELECT 1 FROM chats WHERE uuid = ? LIMIT 1`
	err = tx.QueryRowContext(ctx, checkSQL, chatUUID.String()).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("chat with ID %s not found", chatUUID)
		}
		return fmt.Errorf("failed to check chat existence (uuid: %s): %w", chatUUID, err)
	}

	insertSQL := `INSERT INTO messages (chat_uuid, role, text, generated_at) VALUES (?, ?, ?, ?)`
	if message.GeneratedAt.IsZero() {
		message.GeneratedAt = time.Now().UTC() // Ensure timestamp is set, use UTC
	}

	_, err = tx.ExecContext(ctx, insertSQL, chatUUID.String(), string(message.Role), message.Text, message.GeneratedAt)
	if err != nil {
		return fmt.Errorf("failed to insert message for chat %s: %w", chatUUID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for adding message: %w", err)
	}

	return nil
}

// GetChat retrieves a conversation by its ChatUUID from SQLite
func (s *SQLiteChatHistoryStorage) GetChat(ctx context.Context, chatUUID uuid.UUID) (*ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chat := &ChatHistory{
		UUID:     chatUUID,
		Messages: []ChatHistoryMessage{},
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read transaction for getting chat: %w", err)
	}
	defer tx.Rollback()

	// 1. Get chat metadata
	chatSQL := `SELECT created_at FROM chats WHERE uuid = ?`
	err = tx.QueryRowContext(ctx, chatSQL, chatUUID.String()).Scan(&chat.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("chat with ID %s not found", chatUUID)
		}
		return nil, fmt.Errorf("failed to query chat metadata (uuid: %s): %w", chatUUID, err)
	}

	messagesSQL := `SELECT role, text, generated_at FROM messages WHERE chat_uuid = ? ORDER BY generated_at ASC`
	rows, err := tx.QueryContext(ctx, messagesSQL, chatUUID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query messages for chat %s: %w", chatUUID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var msg ChatHistoryMessage
		var roleStr string
		if err := rows.Scan(&roleStr, &msg.Text, &msg.GeneratedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message row for chat %s: %w", chatUUID, err)
		}
		msg.Role = LLMMessageRole(roleStr)
		chat.Messages = append(chat.Messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows for chat %s: %w", chatUUID, err)
	}

	return chat, nil
}

// ListChatHistories returns all stored conversations from SQLite
func (s *SQLiteChatHistoryStorage) ListChatHistories(ctx context.Context) ([]ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	histories := make(map[string]*ChatHistory)
	var result []ChatHistory

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read transaction for listing chats: %w", err)
	}
	defer tx.Rollback()

	chatsSQL := `SELECT uuid, created_at FROM chats ORDER BY created_at DESC`
	chatRows, err := tx.QueryContext(ctx, chatsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query chats list: %w", err)
	}
	defer chatRows.Close()

	chatUUIDs := []string{}
	for chatRows.Next() {
		var uuidStr string
		var createdAt time.Time
		if err := chatRows.Scan(&uuidStr, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan chat row: %w", err)
		}
		parsedUUID, _ := uuid.Parse(uuidStr)
		histories[uuidStr] = &ChatHistory{
			UUID:      parsedUUID,
			CreatedAt: createdAt.UTC(),
			Messages:  []ChatHistoryMessage{},
		}
		chatUUIDs = append(chatUUIDs, uuidStr)
	}
	if err = chatRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chat rows: %w", err)
	}
	chatRows.Close()

	if len(histories) == 0 {
		return result, nil
	}

	messagesSQL := `SELECT chat_uuid, role, text, generated_at FROM messages ORDER BY chat_uuid, generated_at ASC`
	msgRows, err := tx.QueryContext(ctx, messagesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query all messages: %w", err)
	}
	defer msgRows.Close()

	for msgRows.Next() {
		var chatUUIDStr string
		var roleStr string
		var msg ChatHistoryMessage
		if err := msgRows.Scan(&chatUUIDStr, &roleStr, &msg.Text, &msg.GeneratedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message row during list: %w", err)
		}

		if chat, ok := histories[chatUUIDStr]; ok {
			msg.Role = LLMMessageRole(roleStr)
			chat.Messages = append(chat.Messages, msg)
		}
	}
	if err = msgRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows during list: %w", err)
	}

	result = make([]ChatHistory, 0, len(chatUUIDs))
	for _, uuidStr := range chatUUIDs {
		if chat, ok := histories[uuidStr]; ok {
			result = append(result, *chat)
		}
	}

	return result, nil
}

// DeleteChat removes a conversation by its ChatUUID from SQLite
func (s *SQLiteChatHistoryStorage) DeleteChat(ctx context.Context, chatUUID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for deleting chat: %w", err)
	}
	defer tx.Rollback()

	deleteSQL := `DELETE FROM chats WHERE uuid = ?`
	result, err := tx.ExecContext(ctx, deleteSQL, chatUUID.String())
	if err != nil {
		return fmt.Errorf("failed to delete chat (uuid: %s): %w", chatUUID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error("failed to get rows affected for delete chat", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("chat with ID %s not found", chatUUID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for deleting chat: %w", err)
	}

	return nil
}

// Close releases the database connection.
func (s *SQLiteChatHistoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
