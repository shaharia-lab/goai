# Vector Storage

## Initialization

```go
config := goai.PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
    SchemaName:      "vectors",
}

provider, err := goai.NewPostgresProvider(config)
storage, err := goai.NewVectorStorage(context.Background(), provider)
```

## Collection Management

```go
// Create collection
config := &goai.VectorCollectionConfig{
    Name:         "documents",
    Dimension:    384,
    IndexType:    goai.IndexTypeHNSW,
    DistanceType: goai.DistanceTypeCosine,
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
doc := &goai.VectorDocument{
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
opts := &goai.VectorSearchOptions{
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
