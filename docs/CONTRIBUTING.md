# Contributing to AI Package

## Development Setup

1. Clone the repository:
```bash
git clone https://github.com/shaharia-lab/goai.git
cd goai
```

2. Install dependencies:
```bash
go mod download
```

3. Run tests:
```bash
go test ./...
```

## Code Guidelines

1. **Documentation**
    - Add godoc comments for exported types and functions
    - Include usage examples in documentation
    - Keep documentation up to date with changes

2. **Testing**
    - Write unit tests for new features
    - Include integration tests where appropriate
    - Maintain test coverage

3. **Style**
    - Follow Go best practices
    - Use gofmt for formatting
    - Pass golangci-lint checks

4. **Pull Requests**
    - Create feature branch from main
    - Include tests and documentation
    - Update CHANGELOG.md
    - Request review from maintainers

## Adding New Features

1. **LLM Providers**
    - Implement LLMProvider interface
    - Add provider-specific configuration
    - Include streaming support
    - Add documentation and examples

2. **Embedding Models**
    - Add model constant
    - Update validation logic
    - Document model characteristics
    - Add usage examples

3. **Vector Storage**
    - Implement VectorStorageProvider interface
    - Add storage-specific configuration
    - Include performance optimizations
    - Add documentation and examples