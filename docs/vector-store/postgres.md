# PostgreSQL Vector Storage

## Prerequisites
- PostgreSQL 11+
- pgvector extension

## Setup
1. Install pgvector extension:
```sql
CREATE EXTENSION vector;
```

2. Configure storage:
```go
config := ai.PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
    SchemaName:      "vectors",
}

provider, err := ai.NewPostgresProvider(config)
```

## Operations

1. Create Collection:
```go
err = storage.CreateCollection(ctx, &ai.VectorCollectionConfig{
    Name:         "documents",
    Dimension:    384,
    IndexType:    ai.IndexTypeHNSW,
    DistanceType: ai.DistanceTypeCosine,
})
```

2. Store Documents:
```go
doc := &ai.VectorDocument{
    ID:      "doc1",
    Vector:  embedding.Data[0].Embedding,
    Content: "Content text",
    Metadata: map[string]interface{}{
        "category": "technology",
    },
}
err = storage.UpsertDocument(ctx, "documents", doc)
```

3. Search:
```go
results, err := storage.SearchByVector(ctx, "documents", queryVector, &ai.VectorSearchOptions{
    Limit: 10,
    Filter: map[string]interface{}{
        "category": "technology",
    },
})
```