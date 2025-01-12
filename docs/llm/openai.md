# OpenAI Provider

## Setup

```go
// Create client
client := goai.NewRealOpenAIClient(
    "your-api-key",
    option.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  string(openai.ChatModelGPT3_5Turbo),
})
```

## Message Handling

```go
messages := []ai.LLMMessage{
    {Role: ai.SystemRole, Text: "You are a helpful assistant"},
    {Role: ai.UserRole, Text: "Hello"},
}

config := goai.NewRequestConfig(
    ai.WithMaxToken(1000),
    ai.WithTemperature(0.7),
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
    if chunk.Error != nil {
        break
    }
    fmt.Print(chunk.Text)
}
```
