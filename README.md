# 🚀 GoAI

![AI Package Banner](docs/banner_image.png)

[![Go Reference](https://pkg.go.dev/badge/github.com/shaharia-lab/goai.svg)](https://pkg.go.dev/github.com/shaharia-lab/goai)
[![CI Status](https://github.com/shaharia-lab/goai/actions/workflows/CI.yaml/badge.svg)](https://github.com/shaharia-lab/goai/actions/workflows/CI.yaml)
[![Go Report Card](https://goreportcard.com/badge/shaharia-lab/goai)](https://goreportcard.com/report/shaharia-lab/goai)
[![codecov](https://codecov.io/gh/shaharia-lab/goai/branch/main/graph/badge.svg)](https://codecov.io/gh/shaharia-lab/goai)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=shaharia-lab_goai&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=shaharia-lab_goai)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=shaharia-lab_goai&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=shaharia-lab_goai)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=shaharia-lab_goai&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=shaharia-lab_goai)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=shaharia-lab_goai&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=shaharia-lab_goai)
[![Bugs](https://sonarcloud.io/api/project_badges/measure?project=shaharia-lab_goai&metric=bugs)](https://sonarcloud.io/summary/new_code?id=shaharia-lab_goai)
[![Code Smells](https://sonarcloud.io/api/project_badges/measure?project=shaharia-lab_goai&metric=code_smells)](https://sonarcloud.io/summary/new_code?id=shaharia-lab_goai)

GoAI is a powerful Go library that seamlessly integrates multiple LLM providers, vector embeddings,
and vector storage capabilities. Built for developers who want a clean, unified interface for AI operations.

## ✨ Features

🤖 **Multiple LLM Providers**

- OpenAI
- Anthropic Claude
- AWS Bedrock

📊 **Vector Operations**

- Efficient embeddings generation
- PostgreSQL vector storage
- Similarity search

🛠 **Developer Experience**

- Consistent interfaces
- Streaming support
- Type-safe operations

## 🚀 Quick Start

### 📦 Installation

```bash
go get github.com/shaharia-lab/goai
```

### Example

You can find more examples in the [_examples](_examples) directory which
you can run simply by `go run _examples/simple_chat.go`

```go
package main

import (
   "github.com/shaharia-lab/goai"
   "log"
   "os"
)

func main() {
    // Initialize LLM
    client := goai.NewAnthropicClient(os.Getenv("ANTHROPIC_API_KEY"))
    provider := goai.NewAnthropicLLMProvider(goai.AnthropicProviderConfig{
        Client: client,
    })

    // Configure request
    llm := goai.NewLLMRequest(goai.NewRequestConfig(
        goai.WithMaxToken(200),
        goai.WithTemperature(0.7),
    ), provider)

    // Generate response
    response, err := llm.Generate([]goai.LLMMessage{
        {Role: goai.UserRole, Text: "Explain quantum computing"},
    })

    if err != nil {
        panic(err)
    }
    
    log.Println(response)
}
```

## 📚 Documentation

Visit our [documentation](docs/index.md) for detailed guides on:

- [Getting Started](docs/index)
- [LLM Integration](docs/llm.md)
- [Embedding Generation](docs/embeddings.md)
- [Vector Operations](docs/vector-store/index.md)
- [MCP (Model Context Protocol) Server in Go](docs/md)
- [API Reference](https://pkg.go.dev/github.com/shaharia-lab/goai)

## 🤝 Contributing

We welcome contributions! See our [Contributing Guide](CONTRIBUTING.md) for details.

## 🔒 Security

Review our [Security Policy](docs/SECURITY.md) for reporting vulnerabilities.

## 📝 License

MIT License - see [LICENSE](LICENSE) for details.

---
Built with ❤️ by [Shaharia Lab](https://github.com/shaharia-lab) and
[Contributors](https://github.com/shaharia-lab/goai/graphs/contributors)
