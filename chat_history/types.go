package chat_history

import (
	"github.com/google/uuid"
	"github.com/shaharia-lab/goai"
	"time"
)

type ChatHistoryMessage struct {
	goai.LLMMessage
	GeneratedAt time.Time `json:"generated_at"`
}

// ChatHistory defines the interface for conversation history storage
type ChatHistory struct {
	UUID      uuid.UUID            `json:"uuid"`
	Messages  []ChatHistoryMessage `json:"messages"`
	CreatedAt time.Time            `json:"created_at"`
}
