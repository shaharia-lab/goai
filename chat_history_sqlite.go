package goai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shaharia-lab/goai/observability"
)

// SQLiteChatHistoryStorage is an SQLite implementation of ChatHistoryStorage
type SQLiteChatHistoryStorage struct {
	db     *sql.DB
	mu     sync.RWMutex // Protects against concurrent access issues if needed, though transactions help
	logger observability.Logger
}

// NewSQLiteChatHistoryStorage creates a new instance of SQLiteChatHistoryStorage
// It takes the path to the SQLite database file.
func NewSQLiteChatHistoryStorage(db *sql.DB, logger observability.Logger) (*SQLiteChatHistoryStorage, error) {
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
        created_at DATETIME NOT NULL,
        metadata TEXT DEFAULT '{}'
    );`

	createMessagesTableSQL := `
		CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_uuid TEXT NOT NULL,
		role TEXT NOT NULL,
		text TEXT NOT NULL,
		generated_at DATETIME NOT NULL,
		input_token INTEGER DEFAULT 0,
		output_token INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		FOREIGN KEY (chat_uuid) REFERENCES chats(uuid) ON DELETE CASCADE
	);`

	createMessagesIndexSQL := `
	CREATE INDEX IF NOT EXISTS idx_messages_chat_uuid ON messages (chat_uuid);
	`

	createMessagesTimestampIndexSQL := `
	CREATE INDEX IF NOT EXISTS idx_messages_generated_at ON messages (generated_at);
	`

	tableCheckSQL := `SELECT name FROM sqlite_master WHERE type='table' AND name='chats';`

	var tableName string
	err := s.db.QueryRowContext(ctx, tableCheckSQL).Scan(&tableName)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for schema init: %w", err)
	}
	defer tx.Rollback()

	if tableName == "chats" {
		var hasMetadata int
		metadataCheckSQL := `SELECT COUNT(*) FROM pragma_table_info('chats') WHERE name='metadata';`

		err = tx.QueryRowContext(ctx, metadataCheckSQL).Scan(&hasMetadata)
		if err != nil {
			return fmt.Errorf("failed to check for metadata column: %w", err)
		}

		if hasMetadata == 0 {
			_, err = tx.ExecContext(ctx, `ALTER TABLE chats ADD COLUMN metadata TEXT DEFAULT '{}';`)
			if err != nil {
				return fmt.Errorf("failed to add metadata column to chats table: %w", err)
			}
		}

		metadataCheckSQL = `SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='metadata';`

		err = tx.QueryRowContext(ctx, metadataCheckSQL).Scan(&hasMetadata)
		if err != nil {
			return fmt.Errorf("failed to check for metadata column in messages: %w", err)
		}

		if hasMetadata == 0 {
			_, err = tx.ExecContext(ctx, `ALTER TABLE messages ADD COLUMN metadata TEXT DEFAULT '{}';`)
			if err != nil {
				return fmt.Errorf("failed to add metadata column to messages table: %w", err)
			}
		}
	} else {
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
		SessionID: newUUID.String(),
		Messages:  []ChatHistoryMessage{},
		CreatedAt: createdAt,
		Metadata:  make(map[string]interface{}),
	}

	metadataJSON, err := json.Marshal(chat.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	insertSQL := `INSERT INTO chats (uuid, created_at, metadata) VALUES (?, ?, ?)`

	_, err = s.db.ExecContext(ctx, insertSQL, newUUID.String(), createdAt, string(metadataJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to insert new chat (uuid: %s): %w", newUUID, err)
	}

	return chat, nil
}

// AddMessage adds a new message to an existing conversation in SQLite
func (s *SQLiteChatHistoryStorage) AddMessage(ctx context.Context, sessionID string, message ChatHistoryMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for adding message: %w", err)
	}
	defer tx.Rollback()

	var exists int
	checkSQL := `SELECT COUNT(*) FROM chats WHERE uuid = ?`
	err = tx.QueryRowContext(ctx, checkSQL, sessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check chat existence: %w", err)
	}

	if exists == 0 {
		return errors.New("chat does not exist")
	}

	if message.Metadata == nil {
		message.Metadata = make(map[string]interface{})
	}

	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal message metadata: %w", err)
	}

	insertSQL := `
	INSERT INTO messages (chat_uuid, role, text, generated_at, input_token, output_token, metadata) 
	VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(
		ctx,
		insertSQL,
		sessionID,
		message.Role,
		message.Text,
		message.GeneratedAt,
		message.InputToken,
		message.OutputToken,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return tx.Commit()
}

// UpdateChatMetadata updates the metadata for an existing chat
func (s *SQLiteChatHistoryStorage) UpdateChatMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var exists int
	checkSQL := `SELECT COUNT(*) FROM chats WHERE uuid = ?`
	err := s.db.QueryRowContext(ctx, checkSQL, sessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check chat existence: %w", err)
	}

	if exists == 0 {
		return errors.New("chat does not exist")
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	updateSQL := `UPDATE chats SET metadata = ? WHERE uuid = ?`
	_, err = s.db.ExecContext(ctx, updateSQL, string(metadataJSON), sessionID)
	if err != nil {
		return fmt.Errorf("failed to update chat metadata: %w", err)
	}

	return nil
}

// GetChat retrieves a chat by its session ID
func (s *SQLiteChatHistoryStorage) GetChat(ctx context.Context, sessionID string) (*ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var chat ChatHistory
	var metadataJSON string

	chatSQL := `SELECT uuid, created_at, metadata FROM chats WHERE uuid = ?`
	err := s.db.QueryRowContext(ctx, chatSQL, sessionID).Scan(&chat.SessionID, &chat.CreatedAt, &metadataJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("chat not found")
		}
		return nil, fmt.Errorf("failed to query chat: %w", err)
	}

	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &chat.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal chat metadata: %w", err)
		}
	} else {
		chat.Metadata = make(map[string]interface{})
	}

	messagesSQL := `
	SELECT role, text, generated_at, input_token, output_token, metadata 
	FROM messages 
	WHERE chat_uuid = ? 
	ORDER BY generated_at ASC`

	rows, err := s.db.QueryContext(ctx, messagesSQL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	chat.Messages = []ChatHistoryMessage{}

	for rows.Next() {
		var message ChatHistoryMessage
		var msgMetadataJSON string

		err := rows.Scan(
			&message.Role,
			&message.Text,
			&message.GeneratedAt,
			&message.InputToken,
			&message.OutputToken,
			&msgMetadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}

		if msgMetadataJSON != "" && msgMetadataJSON != "{}" {
			if err := json.Unmarshal([]byte(msgMetadataJSON), &message.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal message metadata: %w", err)
			}
		} else {
			message.Metadata = make(map[string]interface{})
		}

		chat.Messages = append(chat.Messages, message)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows: %w", err)
	}

	return &chat, nil
}

// ListChatHistories returns all chat histories
func (s *SQLiteChatHistoryStorage) ListChatHistories(ctx context.Context) ([]ChatHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chatsSQL := `SELECT uuid, created_at, metadata FROM chats ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, chatsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query chats: %w", err)
	}
	defer rows.Close()

	var chats []ChatHistory

	for rows.Next() {
		var chat ChatHistory
		var metadataJSON string

		err := rows.Scan(&chat.SessionID, &chat.CreatedAt, &metadataJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chat row: %w", err)
		}

		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &chat.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal chat metadata: %w", err)
			}
		} else {
			chat.Metadata = make(map[string]interface{})
		}

		chat.Messages = []ChatHistoryMessage{}
		chats = append(chats, chat)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chat rows: %w", err)
	}

	return chats, nil
}

// DeleteChat removes a chat and its messages
func (s *SQLiteChatHistoryStorage) DeleteChat(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var exists int
	checkSQL := `SELECT COUNT(*) FROM chats WHERE uuid = ?`
	err := s.db.QueryRowContext(ctx, checkSQL, sessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check chat existence: %w", err)
	}

	if exists == 0 {
		return errors.New("chat does not exist")
	}

	deleteSQL := `DELETE FROM chats WHERE uuid = ?`
	_, err = s.db.ExecContext(ctx, deleteSQL, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete chat: %w", err)
	}

	return nil
}

// Close closes the database connection
func (s *SQLiteChatHistoryStorage) Close() error {
	return s.db.Close()
}
