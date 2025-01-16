# OpenAI Compatible Provider

Providers who are compatible with OpenAI API can be used with the `goai` library.
The following example demonstrates how to use the OpenAI provider
for other providers that are compatible with OpenAI API.

## Hugging Face

```go
// Create client
client := goai.NewOpenAIClient(
    "{HUGGING_FACE_API_KEY}",
    option.WithBaseURL("https://api-inference.huggingface.co/v1/"),
)

// Initialize provider
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "microsoft/phi-4",
})
```

## Message Handling

```go
messages := []goai.LLMMessage{
    {Role: goai.SystemRole, Text: "You are a helpful assistant"},
    {Role: goai.UserRole, Text: "Hello"},
}

config := goai.NewRequestConfig(
    goai.WithMaxToken(1000),
    goai.WithTemperature(0.7),
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
