# AWS Bedrock Integration

Integration with AWS Bedrock service.

## Setup
```go
client := bedrockruntime.New(aws.Config{})
provider := ai.NewBedrockLLMProvider(ai.BedrockProviderConfig{
    Client: client,
    Model:  "anthropic.claude-3-sonnet-20240229-v1:0",
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