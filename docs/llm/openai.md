# OpenAI Integration

Integration with OpenAI's GPT models.

## Setup
```go
client := ai.NewRealOpenAIClient(
    "your-api-key",
    option.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
)

provider := ai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "gpt-3.5-turbo",
})
```

## Usage
```go
config := ai.NewRequestConfig(ai.WithMaxToken(2000))
request := ai.NewLLMRequest(config, provider)

response, err := request.Generate([]ai.LLMMessage{
    {Role: ai.UserRole, Text: "Hello!"},
})
```