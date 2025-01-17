# Language Models (LLM)

## Core Interface

```go
type LLMProvider interface {
    GetResponse(messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error)
    GetStreamingResponse(
        ctx context.Context,
        messages []LLMMessage,
        config LLMRequestConfig,
    ) (<-chan StreamingLLMResponse, error)
}
```

## Supported Providers

### OpenAI

```go
client := goai.NewOpenAIClient("api-key")
provider := goai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "gpt-3.5-turbo",
})
```

### Anthropic

```go
client := goai.NewAnthropicClient("api-key")
provider := goai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
    Model:  "claude-3-sonnet-20240229",
})
```

### AWS Bedrock

```go
provider := goai.NewBedrockLLMProvider(ai.BedrockProviderConfig{
    Client: bedrockClient,
    Model:  "anthropic.claude-3-sonnet-20240229-v1:0",
})
```

## Configuration

```go
config := goai.NewRequestConfig(
    goai.WithMaxToken(1000),
    goai.WithTopP(0.9),
    goai.WithTemperature(0.7),
    goai.WithTopK(50),
)
```

## Message Types

```go
type LLMMessageRole string

const (
    UserRole      LLMMessageRole = "user"
    AssistantRole LLMMessageRole = "assistant"
    SystemRole    LLMMessageRole = "system"
)

type LLMMessage struct {
    Role LLMMessageRole
    Text string
}
```

## Basic Usage

```go
llm := goai.NewLLMRequest(config, provider)

response, err := llm.Generate([]goai.LLMMessage{
    {Role: goai.SystemRole, Text: "You are a helpful assistant"},
    {Role: goai.UserRole, Text: "Hello"},
})

fmt.Printf("Response: %s\n", response.Text)
fmt.Printf("Tokens: Input=%d, Output=%d\n", 
    response.TotalInputToken, 
    response.TotalOutputToken)
```

## Streaming Response

```go
stream, err := llm.GenerateStream(ctx, messages)
if err != nil {
    return err
}

for resp := range stream {
    if resp.Error != nil {
        fmt.Printf("Error: %v\n", resp.Error)
        break
    }
    if resp.Done {
        break
    }
    fmt.Print(resp.Text)
}
```

## Template Usage

```go
template := &ai.LLMPromptTemplate{
    Template: "Hello {{.Name}}!",
    Data: map[string]interface{}{
        "Name": "User",
    },
}

promptText, err := template.Parse()
messages := []goai.LLMMessage{
    {Role: goai.UserRole, Text: promptText},
}
```
