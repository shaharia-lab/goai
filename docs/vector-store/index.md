# Vector Storage

## Initialization

```go
config := ai.PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
    SchemaName:      "vectors",
}

provider, err := ai.NewPostgresProvider(config)
storage, err := ai.NewVectorStorage(context.Background(), provider)
```

## Collection Management

```go
// Create collection
config := &ai.VectorCollectionConfig{
    Name:         "documents",
    Dimension:    384,
    IndexType:    ai.IndexTypeHNSW,
    DistanceType: ai.DistanceTypeCosine,
}
err := storage.CreateCollection(ctx, config)

// List collections
collections, err := storage.ListCollections(ctx)

// Delete collection
err = storage.DeleteCollection(ctx, "documents")
```

## Document Operations

```go
// Store document
doc := &ai.VectorDocument{
    ID:      "doc1",
    Vector:  vector,
    Content: "content",
    Metadata: map[string]interface{}{
        "category": "technology",
    },
}
err = storage.UpsertDocument(ctx, "documents", doc)

// Retrieve document
doc, err = storage.GetDocument(ctx, "documents", "doc1")

// Delete document
err = storage.DeleteDocument(ctx, "documents", "doc1")
```

## Search Operations

```go
opts := &ai.VectorSearchOptions{
    Limit: 10,
    Filter: map[string]interface{}{
        "category": "technology",
    },
    IncludeMetadata: true,
}

// Search by vector
results, err := storage.SearchByVector(ctx, "documents", queryVector, opts)

// Search by document ID
results, err := storage.SearchByID(ctx, "documents", "doc1", opts)
```
