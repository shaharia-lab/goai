// Package ai provides artificial intelligence utilities including vector storage capabilities.
package ai

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *PostgresProvider) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock db: %v", err)
	}

	provider := &PostgresProvider{
		db:        db,
		validator: NewVectorValidator(384),
		schema:    "public",
	}

	return db, mock, provider
}

func TestNewPostgresProvider(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer mockDB.Close()

		// First expect the pgvector extension check
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_extension WHERE extname = 'vector'\)`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		// Then expect schema creation
		mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS public").
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Finally expect metadata table creation
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS public.vector_collections").
			WillReturnResult(sqlmock.NewResult(0, 0))

		provider := &PostgresProvider{
			db:        mockDB,
			validator: NewVectorValidator(384),
			schema:    "public",
		}

		err = initializePostgres(mockDB, "public")
		assert.NoError(t, err)
		assert.NotNil(t, provider)

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("pgvector not installed", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer mockDB.Close()

		// Just expect the pgvector check which returns false
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_extension WHERE extname = 'vector'\)`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err = initializePostgres(mockDB, "public")
		assert.Error(t, err)
		assert.Equal(t, ErrCodeInvalidConfig, err.(*VectorError).Code)

		err = mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})
}

func TestPostgresProvider_Initialize(t *testing.T) {
	ctx := context.Background()
	db, mock, provider := setupMockDB(t)
	defer db.Close()

	t.Run("successful initialization", func(t *testing.T) {
		mock.ExpectExec("CREATE EXTENSION").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE SCHEMA").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))

		err := provider.Initialize(ctx)
		assert.NoError(t, err)
	})

	t.Run("initialization error", func(t *testing.T) {
		mock.ExpectExec("CREATE EXTENSION").WillReturnError(sql.ErrConnDone)

		err := provider.Initialize(ctx)
		assert.Error(t, err)
		assert.Equal(t, ErrCodeOperationFailed, err.(*VectorError).Code)
	})
}

func TestPostgresProvider_CreateCollection(t *testing.T) {
	ctx := context.Background()
	db, mock, provider := setupMockDB(t)
	defer db.Close()

	config := &VectorCollectionConfig{
		Name:         "test_collection",
		Dimension:    384,
		IndexType:    IndexTypeHNSW,
		DistanceType: DistanceTypeCosine,
	}

	t.Run("successful creation", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := provider.CreateCollection(ctx, config)
		assert.NoError(t, err)
	})

	t.Run("creation error", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnError(sql.ErrConnDone)
		mock.ExpectRollback()

		err := provider.CreateCollection(ctx, config)
		assert.Error(t, err)
	})
}

func TestPostgresProvider_UpsertDocument(t *testing.T) {
	ctx := context.Background()
	db, mock, provider := setupMockDB(t)
	defer db.Close()

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

	/*config := &VectorCollectionConfig{
		Name:      "test_collection",
		Dimension: 3,
	}*/

	t.Run("successful upsert", func(t *testing.T) {
		mock.ExpectQuery("SELECT dimension").
			WillReturnRows(sqlmock.NewRows([]string{"dimension", "index_type", "distance_type", "custom_fields"}).
				AddRow(3, IndexTypeHNSW, DistanceTypeCosine, "{}"))

		mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(1, 1))

		err := provider.UpsertDocument(ctx, "test_collection", doc)
		assert.NoError(t, err)
	})

	t.Run("collection not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT dimension").WillReturnError(sql.ErrNoRows)

		err := provider.UpsertDocument(ctx, "nonexistent", doc)
		assert.Error(t, err)
		assert.Equal(t, ErrCollectionNotFound, err)
	})
}

func TestPostgresProvider_SearchByVector(t *testing.T) {
	ctx := context.Background()
	db, mock, provider := setupMockDB(t)
	defer db.Close()

	vector := []float32{0.1, 0.2, 0.3}
	opts := &VectorSearchOptions{
		Limit:           10,
		IncludeMetadata: true,
	}

	t.Run("successful search", func(t *testing.T) {
		// Mock getting collection config
		mock.ExpectQuery("SELECT dimension").
			WillReturnRows(sqlmock.NewRows([]string{"dimension", "index_type", "distance_type", "custom_fields"}).
				AddRow(3, IndexTypeHNSW, DistanceTypeCosine, "{}"))

		// Mock search results
		rows := sqlmock.NewRows([]string{"id", "content", "metadata", "created_at", "updated_at", "distance"}).
			AddRow("doc1", "content1", "{}", time.Now(), time.Now(), 0.1)

		mock.ExpectQuery("SELECT(.+)FROM").WillReturnRows(rows)

		results, err := provider.SearchByVector(ctx, "test_collection", vector, opts)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "doc1", results[0].Document.ID)
	})

	t.Run("invalid dimension", func(t *testing.T) {
		mock.ExpectQuery("SELECT dimension").
			WillReturnRows(sqlmock.NewRows([]string{"dimension", "index_type", "distance_type", "custom_fields"}).
				AddRow(5, IndexTypeHNSW, DistanceTypeCosine, "{}"))

		results, err := provider.SearchByVector(ctx, "test_collection", vector, opts)
		assert.Error(t, err)
		assert.Equal(t, ErrInvalidDimension, err)
		assert.Nil(t, results)
	})
}

func TestPostgresProvider_buildSearchQuery(t *testing.T) {
	provider := &PostgresProvider{schema: "public"}

	t.Run("basic query", func(t *testing.T) {
		opts := &VectorSearchOptions{
			Limit:           10,
			IncludeMetadata: true,
		}

		query := provider.buildSearchQuery("test_table", opts)
		assert.Contains(t, query, "SELECT")
		assert.Contains(t, query, "ORDER BY distance")
		assert.Contains(t, query, "LIMIT $2")
	})

	t.Run("query with filters", func(t *testing.T) {
		opts := &VectorSearchOptions{
			Limit:           10,
			IncludeMetadata: true,
			Filter: map[string]interface{}{
				"category": "test",
				"tag":      "value",
			},
		}

		query := provider.buildSearchQuery("test_table", opts)
		assert.Contains(t, query, "WHERE")
		assert.Contains(t, query, "metadata->>'category' = 'test'")
		assert.Contains(t, query, "metadata->>'tag' = 'value'")
	})
}

func TestPostgresProvider_buildIndexQuery(t *testing.T) {
	provider := &PostgresProvider{schema: "public"}

	tests := []struct {
		name           string
		indexType      VectorIndexType
		distanceType   VectorDistanceType
		expectedOp     string
		expectedMethod string
	}{
		{
			name:           "HNSW with cosine",
			indexType:      IndexTypeHNSW,
			distanceType:   DistanceTypeCosine,
			expectedOp:     "vector_cosine_ops",
			expectedMethod: "hnsw",
		},
		{
			name:           "IVFFlat with euclidean",
			indexType:      IndexTypeIVFFlat,
			distanceType:   DistanceTypeEuclidean,
			expectedOp:     "vector_l2_ops",
			expectedMethod: "ivfflat",
		},
		{
			name:           "Default with dot product",
			indexType:      "",
			distanceType:   DistanceTypeDotProduct,
			expectedOp:     "vector_ip_ops",
			expectedMethod: "ivfflat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &VectorCollectionConfig{
				IndexType:    tt.indexType,
				DistanceType: tt.distanceType,
			}

			query := provider.buildIndexQuery("test_table", config)
			assert.Contains(t, query, tt.expectedOp)
			assert.Contains(t, query, tt.expectedMethod)
		})
	}
}
