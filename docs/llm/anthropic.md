# Anthropic Integration

Integration with Anthropic's Claude models.

## Setup
```go
client := ai.NewRealAnthropicClient("your-api-key")
provider := ai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
    Model:  "claude-3-sonnet-20240229",
})
```

## Usage
```go
config := ai.NewRequestConfig(ai.WithMaxToken(2000))
request := ai.NewLLMRequest(config, provider)

response, err := request.Generate([]ai.LLMMessage{
    {Role: ai.SystemRole, Text: "You are a helpful assistant"},
    {Role: ai.UserRole, Text: "Hello!"},
})
```