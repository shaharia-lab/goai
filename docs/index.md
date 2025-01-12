# GoAI Package

[![Go Reference](https://pkg.go.dev/badge/github.com/shaharia-lab/goai.svg)](https://pkg.go.dev/github.com/shaharia-lab/goai)

Go package for AI operations including LLM integration, embeddings generation, and vector storage.

## Features

- **Language Models**
  - OpenAI, Anthropic, AWS Bedrock support
  - Streaming responses
  - Template-based prompts

- **Embeddings**
  - Multiple model support
  - Batch processing
  - Usage tracking

- **Vector Storage**
  - PostgreSQL/pgvector implementation
  - Collection management
  - Similarity search

## Installation

```bash
go get github.com/shaharia-lab/goai
```

## Quick Start

```go
// Initialize LLM
client := goai.NewRealAnthropicClient("api-key")
provider := goai.NewAnthropicLLMProvider(ai.AnthropicProviderConfig{
    Client: client,
})

// Configure request
config := goai.NewRequestConfig(ai.WithMaxToken(1000))
llm := goai.NewLLMRequest(config, provider)

// Generate response
response, err := llm.Generate([]ai.LLMMessage{
    {Role: ai.UserRole, Text: "Hello"},
})
```

## Documentation

- [Getting Started](getting_started.md)
- Language Models
  - [Overview](llm/index.md)
  - [OpenAI](llm/openai.md)
  - [Anthropic](llm/anthropic.md)
  - [AWS Bedrock](llm/bedrock.md)
- [Embeddings](embeddings/index.md)
- [Vector Storage](vector-store/index.md)
- [Prompt Templates](prompt_template.md)

## Contributing

See [contribution guidelines](CONTRIBUTING.md).

## License

MIT License - see [LICENSE](LICENSE) file.
