package goai

import (
	"time"
)

type ChatHistoryMessage struct {
	LLMMessage
	GeneratedAt time.Time              `json:"generated_at"`
	InputToken  int64                  `json:"input_token"`
	OutputToken int64                  `json:"output_token"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ChatHistory defines the interface for conversation history storage
type ChatHistory struct {
	SessionID string                 `json:"session_id"`
	Messages  []ChatHistoryMessage   `json:"messages"`
	CreatedAt time.Time              `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata"`
}
