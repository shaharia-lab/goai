# Language Models (LLM)

The AI package provides a flexible interface for working with various Language Learning Models (LLMs). This module supports multiple providers and offers features like streaming responses and configurable parameters.

## Supported Providers
- OpenAI (GPT models)
- Anthropic (Claude models)
- AWS Bedrock

## Common Interface
All LLM providers implement the same interface:
```go
type LLMProvider interface {
    GetResponse(messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error)
    GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error)
}
```

See provider-specific documentation for detailed setup and usage:
- [OpenAI Integration](openai.md)
- [Anthropic Integration](anthropic.md)
- [AWS Bedrock Integration](bedrock.md)