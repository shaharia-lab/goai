# Contributing to GoAI

## Development Setup

`1.` Clone the repository:

```bash
git clone https://github.com/shaharia-lab/goai.git
cd goai
```

`2.` Install dependencies:

```bash
go mod download
```

`3.` Setup PostgreSQL with pgvector:

```sql
CREATE EXTENSION vector;
```

`4.` Run tests:

```bash
go test ./...
```

## Code Guidelines

### Documentation

- Add godoc comments for all exported types, functions, and methods
- Include usage examples in code comments
- Update /docs directory for major features
- Follow standard Go comment format

### Testing

- Write unit tests for all exported functionality
- Include integration tests for providers
- Add examples in _test.go files
- Test all error conditions

### Style

- Follow Go standard formatting with `gofmt`
- Use meaningful variable names
- Keep functions focused and concise
- Handle errors appropriately

### Pull Requests

- Create feature branches from main
- Include tests and documentation
- Update CHANGELOG.md
- Write clear commit messages

## Adding Features

### LLM Providers

- Implement `LLMProvider` interface
- Support both sync and streaming responses
- Handle rate limits and timeouts
- Follow existing provider patterns

### Vector Storage

- Implement `VectorStorageProvider` interface
- Include proper error types
- Support metadata and filtering
- Optimize for performance

### Error Handling

- Use appropriate error types
- Include error codes
- Provide meaningful error messages
- Document error conditions
