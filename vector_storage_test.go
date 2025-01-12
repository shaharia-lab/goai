// Package ai provides artificial intelligence utilities including vector storage capabilities.
package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockVectorStorageProvider implements VectorStorageProvider for testing
type MockVectorStorageProvider struct {
	mock.Mock
}

func (m *MockVectorStorageProvider) Initialize(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockVectorStorageProvider) CreateCollection(ctx context.Context, config *VectorCollectionConfig) error {
	args := m.Called(ctx, config)
	return args.Error(0)
}

func (m *MockVectorStorageProvider) DeleteCollection(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *MockVectorStorageProvider) ListCollections(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockVectorStorageProvider) UpsertDocument(ctx context.Context, collection string, doc *VectorDocument) error {
	args := m.Called(ctx, collection, doc)
	return args.Error(0)
}

func (m *MockVectorStorageProvider) UpsertDocuments(ctx context.Context, collection string, docs []*VectorDocument) error {
	args := m.Called(ctx, collection, docs)
	return args.Error(0)
}

func (m *MockVectorStorageProvider) GetDocument(ctx context.Context, collection, id string) (*VectorDocument, error) {
	args := m.Called(ctx, collection, id)
	if v := args.Get(0); v != nil {
		return v.(*VectorDocument), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockVectorStorageProvider) DeleteDocument(ctx context.Context, collection, id string) error {
	args := m.Called(ctx, collection, id)
	return args.Error(0)
}

func (m *MockVectorStorageProvider) SearchByVector(ctx context.Context, collection string, vector []float32, opts *VectorSearchOptions) ([]VectorSearchResult, error) {
	args := m.Called(ctx, collection, vector, opts)
	if v := args.Get(0); v != nil {
		return v.([]VectorSearchResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockVectorStorageProvider) SearchByID(ctx context.Context, collection, id string, opts *VectorSearchOptions) ([]VectorSearchResult, error) {
	args := m.Called(ctx, collection, id, opts)
	if v := args.Get(0); v != nil {
		return v.([]VectorSearchResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockVectorStorageProvider) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewVectorStorage(t *testing.T) {
	ctx := context.Background()

	t.Run("successful initialization", func(t *testing.T) {
		provider := new(MockVectorStorageProvider)
		provider.On("Initialize", ctx).Return(nil)

		storage, err := NewVectorStorage(ctx, provider)
		assert.NoError(t, err)
		assert.NotNil(t, storage)

		provider.AssertExpectations(t)
	})

	t.Run("initialization error", func(t *testing.T) {
		provider := new(MockVectorStorageProvider)
		expectedErr := errors.New("initialization failed")
		provider.On("Initialize", ctx).Return(expectedErr)

		storage, err := NewVectorStorage(ctx, provider)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, storage)

		provider.AssertExpectations(t)
	})
}

func TestVectorStorage_CreateCollection(t *testing.T) {
	ctx := context.Background()
	provider := new(MockVectorStorageProvider)
	storage := &VectorStorage{provider: provider}

	config := &VectorCollectionConfig{
		Name:         "test_collection",
		Dimension:    384,
		IndexType:    IndexTypeHNSW,
		DistanceType: DistanceTypeCosine,
	}

	t.Run("successful creation", func(t *testing.T) {
		provider.On("CreateCollection", ctx, config).Return(nil).Once()

		err := storage.CreateCollection(ctx, config)
		assert.NoError(t, err)

		provider.AssertExpectations(t)
	})

	t.Run("creation error", func(t *testing.T) {
		expectedErr := errors.New("creation failed")
		provider.On("CreateCollection", ctx, config).Return(expectedErr).Once()

		err := storage.CreateCollection(ctx, config)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)

		provider.AssertExpectations(t)
	})
}

func TestVectorStorage_UpsertAndGetDocument(t *testing.T) {
	ctx := context.Background()
	provider := new(MockVectorStorageProvider)
	storage := &VectorStorage{provider: provider}

	doc := &VectorDocument{
		ID:      "test_doc",
		Content: "test content",
		Vector:  []float32{0.1, 0.2, 0.3},
		Metadata: map[string]interface{}{
			"test": "value",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	t.Run("successful upsert and get", func(t *testing.T) {
		provider.On("UpsertDocument", ctx, "test_collection", doc).Return(nil).Once()
		provider.On("GetDocument", ctx, "test_collection", "test_doc").Return(doc, nil).Once()

		// Test upsert
		err := storage.UpsertDocument(ctx, "test_collection", doc)
		assert.NoError(t, err)

		// Test get
		retrievedDoc, err := storage.GetDocument(ctx, "test_collection", "test_doc")
		assert.NoError(t, err)
		assert.Equal(t, doc, retrievedDoc)

		provider.AssertExpectations(t)
	})

	t.Run("document not found", func(t *testing.T) {
		provider.On("GetDocument", ctx, "test_collection", "nonexistent").Return(nil, ErrDocumentNotFound).Once()

		doc, err := storage.GetDocument(ctx, "test_collection", "nonexistent")
		assert.Error(t, err)
		assert.Equal(t, ErrDocumentNotFound, err)
		assert.Nil(t, doc)

		provider.AssertExpectations(t)
	})
}

func TestVectorStorage_SearchOperations(t *testing.T) {
	ctx := context.Background()
	provider := new(MockVectorStorageProvider)
	storage := &VectorStorage{provider: provider}

	vector := []float32{0.1, 0.2, 0.3}
	opts := &VectorSearchOptions{
		Limit:           10,
		IncludeMetadata: true,
	}

	expectedResults := []VectorSearchResult{
		{
			Document: &VectorDocument{
				ID:      "doc1",
				Vector:  vector,
				Content: "test content",
			},
			Score:    0.95,
			Distance: 0.05,
		},
	}

	t.Run("search by vector", func(t *testing.T) {
		provider.On("SearchByVector", ctx, "test_collection", vector, opts).Return(expectedResults, nil).Once()

		results, err := storage.SearchByVector(ctx, "test_collection", vector, opts)
		assert.NoError(t, err)
		assert.Equal(t, expectedResults, results)

		provider.AssertExpectations(t)
	})

	t.Run("search by ID", func(t *testing.T) {
		provider.On("SearchByID", ctx, "test_collection", "doc1", opts).Return(expectedResults, nil).Once()

		results, err := storage.SearchByID(ctx, "test_collection", "doc1", opts)
		assert.NoError(t, err)
		assert.Equal(t, expectedResults, results)

		provider.AssertExpectations(t)
	})
}

func TestVectorStorage_Close(t *testing.T) {
	provider := new(MockVectorStorageProvider)
	storage := &VectorStorage{provider: provider}

	t.Run("successful close", func(t *testing.T) {
		provider.On("Close").Return(nil).Once()

		err := storage.Close()
		assert.NoError(t, err)

		provider.AssertExpectations(t)
	})

	t.Run("close error", func(t *testing.T) {
		expectedErr := errors.New("close failed")
		provider.On("Close").Return(expectedErr).Once()

		err := storage.Close()
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)

		provider.AssertExpectations(t)
	})
}
