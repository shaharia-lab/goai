# Chat History

The chat history implementation provides an interface and in-memory storage for managing conversation histories in Go applications.

## Core Components

### Interfaces and Types

- `ChatHistory` - Main structure containing conversation data:
    - UUID
    - Messages array
    - Creation timestamp

- `ChatHistoryStorage` - Interface defining storage operations:
    - Create chat
    - Add message
    - Get chat
    - List chats
    - Delete chat

### Implementation

`InMemoryChatHistoryStorage` provides a thread-safe, in-memory implementation using:
- Map for storing conversations
- RWMutex for concurrent access
- UUID-based chat identification

## Usage Example

```go
storage := NewInMemoryChatHistoryStorage()

// Create new chat
chat, _ := storage.CreateChat()

// Add message
message := ChatHistoryMessage{
    LLMMessage: LLMMessage{
        Role: "user",
        Content: "Hello",
    },
    GeneratedAt: time.Now(),
}
storage.AddMessage(chat.UUID, message)
```

## Features
- Thread-safe operations
- In-memory storage
- UUID-based chat identification
- Timestamp tracking