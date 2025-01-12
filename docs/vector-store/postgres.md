# PostgreSQL Vector Storage

## Prerequisites

- PostgreSQL 11+
- pgvector extension

## Installation

```sql
CREATE EXTENSION vector;
```

## Configuration

```go
type PostgresStorageConfig struct {
    ConnectionString string
    MaxDimension    int
    SchemaName      string
}

provider, err := goai.NewPostgresProvider(PostgresStorageConfig{
    ConnectionString: "postgres://user:pass@localhost:5432/dbname",
    MaxDimension:    384,
    SchemaName:      "vectors",
})
```

## Collection Types

```go
type VectorIndexType string
const (
    IndexTypeFlat    VectorIndexType = "flat"
    IndexTypeIVFFlat VectorIndexType = "ivf_flat"
    IndexTypeHNSW    VectorIndexType = "hnsw"
)

type VectorDistanceType string
const (
    DistanceTypeCosine     VectorDistanceType = "cosine"
    DistanceTypeEuclidean  VectorDistanceType = "euclidean"
    DistanceTypeDotProduct VectorDistanceType = "dot_product"
)
```

## Custom Fields

```go
type VectorFieldConfig struct {
    Type     string `json:"type"`
    Required bool   `json:"required"`
    Indexed  bool   `json:"indexed"`
}

config := &ai.VectorCollectionConfig{
    Name:      "documents",
    Dimension: 384,
    CustomFields: map[string]VectorFieldConfig{
        "category": {
            Type:     "string",
            Required: true,
            Indexed:  true,
        },
    },
}
```

## Error Handling

```go
var (
ErrDocumentNotFound   = &VectorError{Code: ErrCodeNotFound}
ErrCollectionNotFound = &VectorError{Code: ErrCodeCollectionNotFound}
ErrCollectionExists   = &VectorError{Code: ErrCodeCollectionExists}
ErrInvalidDimension   = &VectorError{Code: ErrCodeInvalidDimension}
ErrInvalidConfig      = &VectorError{Code: ErrCodeInvalidConfig}
)
```
