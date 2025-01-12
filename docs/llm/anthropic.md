# Anthropic Provider

## Setup

```go
// Create client
client := goai.NewRealAnthropicClient("your-api-key")

// Initialize provider
provider := goai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
    Model:  anthropic.ModelClaude_3_5_Sonnet_20240620,
})
```

## Message Handling

```go
messages := []ai.LLMMessage{
    {Role: ai.SystemRole, Text: "You are a helpful assistant"},
    {Role: ai.UserRole, Text: "Hello"},
    {Role: ai.AssistantRole, Text: "Hi there!"},
}

config := goai.NewRequestConfig(
    ai.WithMaxToken(1000),
    ai.WithTopP(0.9),
)

response, err := provider.GetResponse(messages, config)
```

## Streaming

```go
stream, err := provider.GetStreamingResponse(ctx, messages, config)
if err != nil {
    return err
}

for chunk := range stream {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        if chunk.Error != nil {
            return chunk.Error
        }
        fmt.Print(chunk.Text)
    }
}
```
