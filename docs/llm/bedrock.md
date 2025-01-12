# AWS Bedrock Provider

## Setup

```go
// Initialize provider with Bedrock client
provider := goai.NewBedrockLLMProvider(ai.BedrockProviderConfig{
    Client: bedrockClient,
    Model:  "anthropic.claude-3-sonnet-20240229-v1:0",
})
```

## Message Handling

```go
messages := []goai.LLMMessage{
    {Role: goai.UserRole, Text: "Hello"},
    {Role: goai.AssistantRole, Text: "Hi there!"},
}

config := goai.NewRequestConfig(
    goai.WithMaxToken(1000),
    goai.WithTemperature(0.7),
    goai.WithTopP(0.9),
)

response, err := provider.GetResponse(messages, config)
if err != nil {
    return err
}

fmt.Printf("Response: %s\n", response.Text)
fmt.Printf("Tokens: Input=%d, Output=%d\n", 
    response.TotalInputToken, 
    response.TotalOutputToken)
```

## Response Structure

```go
type LLMResponse struct {
    Text             string
    TotalInputToken  int
    TotalOutputToken int
    CompletionTime   float64
}
```
