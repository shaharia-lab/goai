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
messages := []goai.LLMMessage{
    {Role: goai.SystemRole, Text: "You are a helpful assistant"},
    {Role: goai.UserRole, Text: "Hello"},
    {Role: goai.AssistantRole, Text: "Hi there!"},
}

config := goai.NewRequestConfig(
    goai.WithMaxToken(1000),
    goai.WithTopP(0.9),
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
