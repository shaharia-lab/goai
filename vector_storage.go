// Package ai provides artificial intelligence utilities including vector storage capabilities.
package ai

import (
	"context"
)

// VectorStorage provides a high-level interface for vector storage operations.
// It acts as a facade for the underlying storage provider implementation.
type VectorStorage struct {
	provider VectorStorageProvider
}

// NewVectorStorage creates a new VectorStorage instance with the specified provider
// and initializes the storage.
//
// Example usage:
//
//	provider := NewPostgresProvider(pgConfig)
//	storage, err := NewVectorStorage(ctx, provider)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewVectorStorage(ctx context.Context, provider VectorStorageProvider) (*VectorStorage, error) {
	if err := provider.Initialize(ctx); err != nil {
		return nil, err
	}

	return &VectorStorage{
		provider: provider,
	}, nil
}

// CreateCollection creates a new vector collection with the specified configuration.
func (s *VectorStorage) CreateCollection(ctx context.Context, config *VectorCollectionConfig) error {
	return s.provider.CreateCollection(ctx, config)
}

// DeleteCollection removes an existing vector collection.
func (s *VectorStorage) DeleteCollection(ctx context.Context, name string) error {
	return s.provider.DeleteCollection(ctx, name)
}

// ListCollections returns a list of all available collections.
func (s *VectorStorage) ListCollections(ctx context.Context) ([]string, error) {
	return s.provider.ListCollections(ctx)
}

// UpsertDocument adds or updates a document in the specified collection.
func (s *VectorStorage) UpsertDocument(ctx context.Context, collection string, doc *VectorDocument) error {
	return s.provider.UpsertDocument(ctx, collection, doc)
}

// UpsertDocuments adds or updates multiple documents in batch.
func (s *VectorStorage) UpsertDocuments(ctx context.Context, collection string, docs []*VectorDocument) error {
	return s.provider.UpsertDocuments(ctx, collection, docs)
}

// GetDocument retrieves a document by its ID.
func (s *VectorStorage) GetDocument(ctx context.Context, collection, id string) (*VectorDocument, error) {
	return s.provider.GetDocument(ctx, collection, id)
}

// DeleteDocument removes a document from the collection.
func (s *VectorStorage) DeleteDocument(ctx context.Context, collection, id string) error {
	return s.provider.DeleteDocument(ctx, collection, id)
}

// SearchByVector performs a vector similarity search.
func (s *VectorStorage) SearchByVector(ctx context.Context, collection string, vector []float32, opts *VectorSearchOptions) ([]VectorSearchResult, error) {
	return s.provider.SearchByVector(ctx, collection, vector, opts)
}

// SearchByID performs a similarity search using an existing document as the query.
func (s *VectorStorage) SearchByID(ctx context.Context, collection, id string, opts *VectorSearchOptions) ([]VectorSearchResult, error) {
	return s.provider.SearchByID(ctx, collection, id, opts)
}

// Close closes the underlying storage provider connection.
func (s *VectorStorage) Close() error {
	return s.provider.Close()
}
