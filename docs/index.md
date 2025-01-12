# AI Package

[![Go Reference](https://pkg.go.dev/badge/github.com/shaharia-lab/goai.svg)](https://pkg.go.dev/github.com/shaharia-lab/goai)

The AI package provides Go developers with tools for integrating language models, generating embeddings, and managing vector storage. It supports multiple providers and offers a consistent interface for AI operations.

## Features

- **Language Models**
   - Multiple provider support (OpenAI, Anthropic, AWS Bedrock)
   - Streaming responses
   - Configurable parameters

- **Text Embeddings**
   - Multiple embedding models
   - Batch processing
   - Usage tracking

- **Vector Storage**
   - PostgreSQL/pgvector support
   - Similarity search
   - Document management

## Quick Start

Install the package:
```shell
go get github.com/shaharia-lab/goai
```

Basic LLM usage:
```go
import "github.com/shaharia-lab/goai"

// Create provider
client := ai.NewRealOpenAIClient("your-api-key")
provider := ai.NewOpenAILLMProvider(ai.OpenAIProviderConfig{
    Client: client,
})

// Configure request
config := ai.NewRequestConfig(ai.WithMaxToken(2000))
request := ai.NewLLMRequest(config, provider)

// Generate response
response, err := request.Generate([]ai.LLMMessage{
    {Role: ai.UserRole, Text: "Hello!"},
})
```

## Documentation

- [Getting Started](getting-started.md)
- Language Models
   - [Overview](llm/index.md)
   - [OpenAI Integration](llm/openai.md)
   - [Anthropic Integration](llm/anthropic.md)
   - [AWS Bedrock Integration](llm/bedrock.md)
- Embeddings
   - [Overview](embeddings/index.md)
   - [Available Models](embeddings/models.md)
- Vector Storage
   - [Overview](vector-store/index.md)
   - [PostgreSQL Implementation](vector-store/postgres.md)
- [Prompt Templates](prompt_template.md)
   - Dynamic prompt generation
   - Template syntax
   - Best practices

## Contributing

We welcome contributions! See our [contribution guidelines](CONTRIBUTING.md).

## License

MIT License - see the [LICENSE](../LICENSE) file for details.