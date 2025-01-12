# Vector Storage

Store and search vector embeddings efficiently using PostgreSQL with pgvector.

## Features
- Collection-based organization
- Similarity search
- Metadata filtering
- Batch operations

## Basic Usage
```go
config := ai.PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
}

provider, err := ai.NewPostgresProvider(config)
if err != nil {
    log.Fatal(err)
}

storage, err := ai.NewVectorStorage(context.Background(), provider)
if err != nil {
    log.Fatal(err)
}
```

See [PostgreSQL Implementation](postgres.md) for detailed setup and usage.