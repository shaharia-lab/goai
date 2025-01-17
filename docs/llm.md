# Language Models (LLM)

The `goai` package provides a flexible interface for working with various
Large Language Models (LLMs). This module supports multiple providers
and offers features like streaming responses and configurable parameters.

## Example

Here's a complete example using OpenAI's GPT-3.5:

```go
package main

import (
    "fmt"
    "github.com/openai/openai-go"
    "github.com/shaharia-lab/goai"
    "os"
)

func main() {
    // Create OpenAI LLM Provider
    llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
        Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
        Model:  openai.ChatModelGPT3_5Turbo,
    })

    // Configure LLM Request
    llm := goai.NewLLMRequest(goai.NewRequestConfig(
        goai.WithMaxToken(100),
        goai.WithTemperature(0.7),
    ), llmProvider)

    // Generate response
    response, err := llm.Generate([]goai.LLMMessage{
        {Role: goai.UserRole, Text: "Explain quantum computing"},
    })

    if err != nil {
        panic(err)
    }

    fmt.Printf("Response: %s\n", response.Text)
    fmt.Printf("Input token: %d, Output token: %d", response.TotalInputToken, response.TotalOutputToken)
}
```

## Streaming Example

For streaming responses:

```go
    // Generate streaming response
    stream, err := llm.GenerateStream(context.Background(), []goai.LLMMessage{
        {Role: goai.UserRole, Text: "Explain quantum computing"},
    })

    if err != nil {
        panic(err)
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

## Configuration Options

```go
config := goai.NewRequestConfig(
    goai.WithMaxToken(1000),    // Set maximum tokens
    goai.WithTopP(0.9),         // Set top-p sampling
    goai.WithTemperature(0.7),  // Set temperature
    goai.WithTopK(50),          // Set top-k sampling
)
```

## Available Providers

### OpenAI

```go
llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
    Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
    Model:  openai.ChatModelGPT3_5Turbo,
})
```

### Anthropic

```go
llmProvider := goai.NewAnthropicLLMProvider(goai.AnthropicProviderConfig{
    Client: goai.NewAnthropicClient(os.Getenv("ANTHROPIC_API_KEY")),
    Model:  anthropic.ModelClaude3_5Sonnet20241022,
})
```

### AWS Bedrock

```go
// Initialize AWS config
awsConfig, err := config.LoadDefaultConfig(ctx)
if err != nil {
    log.Printf("failed to load AWS config: %w", err)
    return
}

llmProvider := goai.NewBedrockLLMProvider(goai.BedrockProviderConfig{
    Client: bedrockruntime.NewFromConfig(awsConfig),
    Model:  "anthropic.claude-3-5-sonnet-20240620-v1:0",
})
```

### OpenAI Compatible

#### Hugging Face

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
