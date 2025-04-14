package chat_history

import (
	"time"

	"github.com/shaharia-lab/goai"
)

type ChatHistoryMessage struct {
	goai.LLMMessage
	GeneratedAt time.Time `json:"generated_at"`
}

// ChatHistory defines the interface for conversation history storage
type ChatHistory struct {
	SessionID string               `json:"session_id"`
	Messages  []ChatHistoryMessage `json:"messages"`
	CreatedAt time.Time            `json:"created_at"`
}
