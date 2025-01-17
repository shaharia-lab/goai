# LLM Providers

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

### OpenAPI Compatible

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

## Using Providers

Once you have configured a provider, you can use it to generate responses as shown in the [main documentation](index.md#complete-example).

For streaming responses, refer to the [streaming example](index.md#streaming-example).
