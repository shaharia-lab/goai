# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

GoAI provides a unified interface over multiple LLM providers, embeddings, vector storage, and a full Model Context Protocol (MCP) client/server implementation (module `github.com/shaharia-lab/goai`). It is a **library** — there is no `main` package; runnable usage lives in `_examples/`.

The root package (`package goai`) keeps its source files flat in the repository root, prefixed by concern (`llm_*`, `mcp_*`, `embedding_*`, `vector_*`, `chat_history_*`). Additional functionality is being organized into subpackages — place new self-contained subsystems in their own directory/package rather than adding to the root.

## Commands

```bash
go build ./...                                              # build
go test ./...                                               # run all tests
go test ./... -covermode=atomic -coverprofile=coverage.out # tests with coverage (as CI runs them)
go test -run TestName ./...                                 # run a single test by name
go run _examples/simple_chat.go                             # run an example
golangci-lint run                                           # lint (CI uses super-linter with VALIDATE_GO)
```

There is no Makefile. CI (`.github/workflows/CI.yaml`) runs lint (super-linter), tests + Codecov, then a SonarCloud scan.

## Architecture

The package follows a consistent **facade + provider-interface** pattern in every subsystem: a high-level struct wraps a provider interface, so implementations are swappable and injectable for testing.

- **LLM** (`llm.go`, `types.go`): `LLMRequest` (facade) wraps the `LLMProvider` interface (`GetResponse` / `GetStreamingResponse`). Providers: `llm_provider_openai.go`, `llm_provider_anthropic.go`, `llm_provider_aws_bedrock.go`, `llm_provider_gemini.go`, plus `llm_provider_no_ops.go`. Each provider has a matching thin client wrapper (e.g. `llm_provider_openai_client.go`) that isolates the vendor SDK for mocking. Streaming lives in separate `*_streaming.go` files. `NewLLMRequest` automatically wraps the provider in a `TracingLLMProvider` (decorator pattern, `tracing_llm_provider.go`) unless already traced. Request options are functional options (`WithMaxToken`, `WithTemperature`, etc.) via `NewRequestConfig`.
- **Tools** (`tools_provider.go`): `ToolsProvider` supplies tools to LLM calls either from a local in-process `[]Tool` list **or** from an MCP `Client` — the two are mutually exclusive (adding one blocks the other). `ExecuteTool` tries local tools first, then falls back to the MCP client.
- **Embeddings** (`embedding.go`): `EmbeddingService` wraps the `EmbeddingProvider` interface; validates input and response. Bedrock provider in `embedding_aws_bedrock_provider.go`.
- **Vector storage** (`vector_storage.go`): `VectorStorage` facade wraps `VectorStorageProvider`. Postgres/pgvector implementation in `vector_storage_postgres.go`; validation in `vector_validation.go`. `chunking.go` splits text for embedding.
- **Chat history** (`chat_history_*.go`): `ChatHistoryStorage` interface with in-memory and SQLite implementations.
- **MCP** (`mcp_*.go`): a complete MCP implementation.
  - **Client**: `mcp_client.go` (`Client` + `Transport` interface) with two transports — `mcp_client_sse.go` (HTTP/SSE) and `mcp_client_std.go` (stdio).
  - **Server**: `mcp_server_base.go` (`BaseServer` holds tools/resources/prompts, built with `ServerConfigOption`s like `UseLogger`) wrapped by a transport server — `NewStdIOServer` (`mcp_server_stdio.go`) or `NewSSEServer` (`mcp_server_sse.go`), each with a `Run(ctx)` loop. Wire protocol types are JSON-RPC 2.0 in `mcp_types.go`. See `mcp_doc.go` for an end-to-end server example.

## Conventions

- **Logging**: use the package `Logger` interface (`logger.go`), not the stdlib logger directly. Adapters exist for slog, logrus, and zap (`NewSlogLogger`, `NewLogrusLogger`, `NewZapLogger`), plus `NewDefaultLogger` and `NewNullLogger` (use the null logger in tests).
- **Tracing**: OpenTelemetry spans via `StartSpan` (`tracing.go`). Follow the existing pattern — set attributes, record errors, and `defer span.End()`.
- **Testing**: uses `testify` and `go-sqlmock`; providers are tested by injecting mock clients (see `aws_bedrock_mock_client.go` and the `*_client.go` wrappers). Mock/generated files (`*_mock*.go`, `*_gen.go`) and `_examples/` are excluded from SonarCloud coverage.
- **Docs**: user-facing documentation is in `docs/` and mirrors the subsystems (llm, embeddings, vector-store, mcp, chat_history).
