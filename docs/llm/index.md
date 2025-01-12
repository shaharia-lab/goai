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
client := ai.NewRealOpenAIClient("api-key")
provider := ai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
    Model:  "gpt-3.5-turbo",
})
```

### Anthropic

```go
client := ai.NewRealAnthropicClient("api-key")
provider := ai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
    Model:  "claude-3-sonnet-20240229",
})
```

### AWS Bedrock

```go
provider := ai.NewBedrockLLMProvider(ai.BedrockProviderConfig{
    Client: bedrockClient,
    Model:  "anthropic.claude-3-sonnet-20240229-v1:0",
})
```

## Configuration

```go
config := ai.NewRequestConfig(
    ai.WithMaxToken(1000),
    ai.WithTopP(0.9),
    ai.WithTemperature(0.7),
    ai.WithTopK(50),
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
llm := ai.NewLLMRequest(config, provider)

response, err := llm.Generate([]ai.LLMMessage{
    {Role: ai.SystemRole, Text: "You are a helpful assistant"},
    {Role: ai.UserRole, Text: "Hello"},
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
messages := []ai.LLMMessage{
    {Role: ai.UserRole, Text: promptText},
}
```
