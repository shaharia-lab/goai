// Package ai provides artificial intelligence utilities including vector storage capabilities.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

// PostgresProvider implements VectorStorage using PostgreSQL with pgvector extension.
type PostgresProvider struct {
	db        *sql.DB
	validator *VectorValidator
	schema    string
}

// PostgresStorageConfig holds configuration for PostgreSQL vector storage.
type PostgresStorageConfig struct {
	// ConnectionString is the PostgreSQL connection string
	ConnectionString string

	// MaxDimension is the maximum allowed vector dimension
	MaxDimension int

	// SchemaName is the PostgreSQL schema to use (default: public)
	SchemaName string
}

// NewPostgresProvider creates a new PostgreSQL-based vector storage.
func NewPostgresProvider(config PostgresStorageConfig) (*PostgresProvider, error) {
	if config.SchemaName == "" {
		config.SchemaName = "public"
	}

	db, err := sql.Open("postgres", config.ConnectionString)
	if err != nil {
		return nil, &VectorError{
			Code:    ErrCodeConnectionFailed,
			Message: "failed to connect to PostgreSQL",
			Err:     err,
		}
	}

	// Test connection and check pgvector extension
	if err := initializePostgres(db, config.SchemaName); err != nil {
		db.Close()
		return nil, err
	}

	return &PostgresProvider{
		db:        db,
		validator: NewVectorValidator(config.MaxDimension),
		schema:    config.SchemaName,
	}, nil
}

// Initialize implements VectorStorageProvider.Initialize.
func (p *PostgresProvider) Initialize(ctx context.Context) error {
	// Create the vector extension if not exists
	if _, err := p.db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create vector extension",
			Err:     err,
		}
	}

	// Create schema if not exists
	if _, err := p.db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", p.schema)); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create schema",
			Err:     err,
		}
	}

	// Create collections metadata table
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.vector_collections (
			name TEXT PRIMARY KEY,
			schema_name TEXT NOT NULL,
			dimension INTEGER NOT NULL,
			index_type TEXT NOT NULL,
			distance_type TEXT NOT NULL,
			custom_fields JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)
	`, p.schema)

	if _, err := p.db.ExecContext(ctx, query); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create collections metadata table",
			Err:     err,
		}
	}

	return nil
}

// Close closes the database connection.
func (p *PostgresProvider) Close() error {
	return p.db.Close()
}

// CreateCollection implements VectorStorage.CreateCollection.
func (p *PostgresProvider) CreateCollection(ctx context.Context, config *VectorCollectionConfig) error {
	if err := p.validator.ValidateCollection(config); err != nil {
		return err
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create collection table
	tableName := fmt.Sprintf("%s.%s", p.schema, config.Name)
	if err := p.createCollectionTable(ctx, tx, tableName, config); err != nil {
		return err
	}

	// Store collection metadata
	if err := p.storeCollectionMetadata(ctx, tx, config); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteCollection implements VectorStorage.DeleteCollection.
func (p *PostgresProvider) DeleteCollection(ctx context.Context, name string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Drop collection table
	tableName := fmt.Sprintf("%s.%s", p.schema, name)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to drop collection table",
			Err:     err,
		}
	}

	// Remove collection metadata
	if err := p.deleteCollectionMetadata(ctx, tx, name); err != nil {
		return err
	}

	return tx.Commit()
}

// ListCollections implements VectorStorage.ListCollections.
func (p *PostgresProvider) ListCollections(ctx context.Context) ([]string, error) {
	query := `
		SELECT name 
		FROM vector_collections 
		WHERE schema_name = $1
		ORDER BY name`

	rows, err := p.db.QueryContext(ctx, query, p.schema)
	if err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to list collections",
			Err:     err,
		}
	}
	defer rows.Close()

	var collections []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan collection name: %w", err)
		}
		collections = append(collections, name)
	}

	return collections, rows.Err()
}

// UpsertDocument implements VectorStorage.UpsertDocument.
func (p *PostgresProvider) UpsertDocument(ctx context.Context, collection string, doc *VectorDocument) error {
	config, err := p.getCollectionConfig(ctx, collection)
	if err != nil {
		return err
	}

	if err := p.validator.ValidateDocument(doc, config); err != nil {
		return err
	}

	tableName := fmt.Sprintf("%s.%s", p.schema, collection)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, vector, content, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			vector = EXCLUDED.vector,
			content = EXCLUDED.content,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at
	`, tableName)

	metadata, err := json.Marshal(doc.Metadata)
	if err != nil {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "failed to marshal metadata",
			Err:     err,
		}
	}

	vec := pgvector.NewVector(doc.Vector)
	_, err = p.db.ExecContext(ctx, query,
		doc.ID,
		vec,
		doc.Content,
		metadata,
		doc.CreatedAt,
		time.Now(),
	)

	if err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to upsert document",
			Err:     err,
		}
	}

	return nil
}

// UpsertDocuments implements VectorStorage.UpsertDocuments.
func (p *PostgresProvider) UpsertDocuments(ctx context.Context, collection string, docs []*VectorDocument) error {
	if len(docs) == 0 {
		return nil
	}

	config, err := p.getCollectionConfig(ctx, collection)
	if err != nil {
		return err
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := p.prepareBulkInsertStmt(ctx, tx, collection)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, doc := range docs {
		if err := p.validator.ValidateDocument(doc, config); err != nil {
			return err
		}

		metadata, err := json.Marshal(doc.Metadata)
		if err != nil {
			return &VectorError{
				Code:    ErrCodeInvalidConfig,
				Message: "failed to marshal metadata",
				Err:     err,
			}
		}

		vec := pgvector.NewVector(doc.Vector)
		_, err = stmt.ExecContext(ctx,
			doc.ID,
			vec,
			doc.Content,
			metadata,
			doc.CreatedAt,
			time.Now(),
		)
		if err != nil {
			return &VectorError{
				Code:    ErrCodeOperationFailed,
				Message: "failed to execute bulk insert",
				Err:     err,
			}
		}
	}

	return tx.Commit()
}

// GetDocument implements VectorStorage.GetDocument.
func (p *PostgresProvider) GetDocument(ctx context.Context, collection, id string) (*VectorDocument, error) {
	tableName := fmt.Sprintf("%s.%s", p.schema, collection)
	query := fmt.Sprintf(`
		SELECT id, vector, content, metadata, created_at, updated_at
		FROM %s
		WHERE id = $1
	`, tableName)

	var doc VectorDocument
	var metadata []byte
	var vec pgvector.Vector

	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&doc.ID,
		&vec,
		&doc.Content,
		&metadata,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrDocumentNotFound
	} else if err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to get document",
			Err:     err,
		}
	}

	doc.Vector = vec.Slice()
	if err := json.Unmarshal(metadata, &doc.Metadata); err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to unmarshal metadata",
			Err:     err,
		}
	}

	return &doc, nil
}

// DeleteDocument implements VectorStorage.DeleteDocument.
func (p *PostgresProvider) DeleteDocument(ctx context.Context, collection, id string) error {
	tableName := fmt.Sprintf("%s.%s", p.schema, collection)
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", tableName)

	result, err := p.db.ExecContext(ctx, query, id)
	if err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to delete document",
			Err:     err,
		}
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrDocumentNotFound
	}

	return nil
}

// SearchByVector implements VectorStorage.SearchByVector.
func (p *PostgresProvider) SearchByVector(ctx context.Context, collection string, vector []float32, opts *VectorSearchOptions) ([]VectorSearchResult, error) {
	config, err := p.getCollectionConfig(ctx, collection)
	if err != nil {
		return nil, err
	}

	if len(vector) != config.Dimension {
		return nil, ErrInvalidDimension
	}

	if opts == nil {
		opts = &VectorSearchOptions{
			Limit:           10,
			IncludeMetadata: true,
		}
	}

	tableName := fmt.Sprintf("%s.%s", p.schema, collection)
	query := p.buildSearchQuery(tableName, opts)

	vec := pgvector.NewVector(vector)
	rows, err := p.db.QueryContext(ctx, query, vec, opts.Limit, opts.Offset)
	if err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to execute search query",
			Err:     err,
		}
	}
	defer rows.Close()

	return p.scanSearchResults(rows, opts.IncludeMetadata)
}

// SearchByID implements VectorStorage.SearchByID.
func (p *PostgresProvider) SearchByID(ctx context.Context, collection, id string, opts *VectorSearchOptions) ([]VectorSearchResult, error) {
	doc, err := p.GetDocument(ctx, collection, id)
	if err != nil {
		return nil, err
	}

	return p.SearchByVector(ctx, collection, doc.Vector, opts)
}

// Helper functions

func initializePostgres(db *sql.DB, schema string) error {
	// Check pgvector extension
	var hasExtension bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')").Scan(&hasExtension)
	if err != nil || !hasExtension {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "pgvector extension not installed",
			Err:     err,
		}
	}

	// Create schema if not exists
	if _, err := db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create schema",
			Err:     err,
		}
	}

	// Create metadata table if not exists
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.vector_collections (
			name TEXT PRIMARY KEY,
			schema_name TEXT NOT NULL,
			dimension INTEGER NOT NULL,
			index_type TEXT NOT NULL,
			distance_type TEXT NOT NULL,
			custom_fields JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, schema)

	if _, err := db.Exec(query); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create metadata table",
			Err:     err,
		}
	}

	return nil
}

func (p *PostgresProvider) createCollectionTable(ctx context.Context, tx *sql.Tx, tableName string, config *VectorCollectionConfig) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			vector vector(%d),
			content TEXT,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE,
			updated_at TIMESTAMP WITH TIME ZONE
		)`, tableName, config.Dimension)

	if _, err := tx.ExecContext(ctx, query); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create collection table",
			Err:     err,
		}
	}

	// Create vector index based on index type
	indexQuery := p.buildIndexQuery(tableName, config)
	if _, err := tx.ExecContext(ctx, indexQuery); err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to create vector index",
			Err:     err,
		}
	}

	return nil
}

// Helper functions continue...

func (p *PostgresProvider) storeCollectionMetadata(ctx context.Context, tx *sql.Tx, config *VectorCollectionConfig) error {
	customFields, err := json.Marshal(config.CustomFields)
	if err != nil {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "failed to marshal custom fields",
			Err:     err,
		}
	}

	query := fmt.Sprintf(`
		INSERT INTO %s.vector_collections 
		(name, schema_name, dimension, index_type, distance_type, custom_fields)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, p.schema)

	_, err = tx.ExecContext(ctx, query,
		config.Name,
		p.schema,
		config.Dimension,
		config.IndexType,
		config.DistanceType,
		customFields,
	)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return ErrCollectionExists
		}
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to store collection metadata",
			Err:     err,
		}
	}

	return nil
}

func (p *PostgresProvider) deleteCollectionMetadata(ctx context.Context, tx *sql.Tx, name string) error {
	query := fmt.Sprintf(`
		DELETE FROM %s.vector_collections 
		WHERE name = $1 AND schema_name = $2
	`, p.schema)

	result, err := tx.ExecContext(ctx, query, name, p.schema)
	if err != nil {
		return &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to delete collection metadata",
			Err:     err,
		}
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrCollectionNotFound
	}

	return nil
}

func (p *PostgresProvider) getCollectionConfig(ctx context.Context, name string) (*VectorCollectionConfig, error) {
	query := fmt.Sprintf(`
		SELECT dimension, index_type, distance_type, custom_fields
		FROM %s.vector_collections
		WHERE name = $1 AND schema_name = $2
	`, p.schema)

	var config VectorCollectionConfig
	var customFields []byte

	err := p.db.QueryRowContext(ctx, query, name, p.schema).Scan(
		&config.Dimension,
		&config.IndexType,
		&config.DistanceType,
		&customFields,
	)

	if err == sql.ErrNoRows {
		return nil, ErrCollectionNotFound
	} else if err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to get collection config",
			Err:     err,
		}
	}

	config.Name = name
	if err := json.Unmarshal(customFields, &config.CustomFields); err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to unmarshal custom fields",
			Err:     err,
		}
	}

	return &config, nil
}

func (p *PostgresProvider) buildSearchQuery(tableName string, opts *VectorSearchOptions) string {
	var query strings.Builder
	query.WriteString(fmt.Sprintf(`
		SELECT 
			id, 
			content, 
			metadata,
			created_at,
			updated_at,
	`))

	if opts.IncludeVectors {
		query.WriteString("vector,")
	}

	query.WriteString(`
		vector <-> $1 as distance
		FROM %s
	`)

	if len(opts.Filter) > 0 {
		query.WriteString("\nWHERE ")
		conditions := make([]string, 0, len(opts.Filter))
		for key, value := range opts.Filter {
			conditions = append(conditions, fmt.Sprintf("metadata->>'%s' = '%v'", key, value))
		}
		query.WriteString(strings.Join(conditions, " AND "))
	}

	query.WriteString("\nORDER BY distance")
	query.WriteString("\nLIMIT $2 OFFSET $3")

	return fmt.Sprintf(query.String(), tableName)
}

func (p *PostgresProvider) buildIndexQuery(tableName string, config *VectorCollectionConfig) string {
	var operator string
	switch config.DistanceType {
	case DistanceTypeCosine:
		operator = "vector_cosine_ops"
	case DistanceTypeEuclidean:
		operator = "vector_l2_ops"
	case DistanceTypeDotProduct:
		operator = "vector_ip_ops"
	default:
		operator = "vector_cosine_ops"
	}

	switch config.IndexType {
	case IndexTypeIVFFlat:
		return fmt.Sprintf("CREATE INDEX ON %s USING ivfflat (vector %s)", tableName, operator)
	case IndexTypeHNSW:
		return fmt.Sprintf("CREATE INDEX ON %s USING hnsw (vector %s)", tableName, operator)
	default:
		return fmt.Sprintf("CREATE INDEX ON %s USING ivfflat (vector %s)", tableName, operator)
	}
}

func (p *PostgresProvider) prepareBulkInsertStmt(ctx context.Context, tx *sql.Tx, collection string) (*sql.Stmt, error) {
	tableName := fmt.Sprintf("%s.%s", p.schema, collection)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, vector, content, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			vector = EXCLUDED.vector,
			content = EXCLUDED.content,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at
	`, tableName)

	return tx.PrepareContext(ctx, query)
}

func (p *PostgresProvider) scanSearchResults(rows *sql.Rows, includeMetadata bool) ([]VectorSearchResult, error) {
	var results []VectorSearchResult

	for rows.Next() {
		var (
			result    VectorSearchResult
			doc       VectorDocument
			metadata  []byte
			vec       pgvector.Vector
			hasVector bool
			distance  float32
		)

		scanArgs := []interface{}{
			&doc.ID,
			&doc.Content,
			&metadata,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		}

		if hasVector {
			scanArgs = append(scanArgs, &vec)
		}
		scanArgs = append(scanArgs, &distance)

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, &VectorError{
				Code:    ErrCodeOperationFailed,
				Message: "failed to scan search result",
				Err:     err,
			}
		}

		if hasVector {
			doc.Vector = vec.Slice()
		}

		if includeMetadata {
			if err := json.Unmarshal(metadata, &doc.Metadata); err != nil {
				return nil, &VectorError{
					Code:    ErrCodeOperationFailed,
					Message: "failed to unmarshal metadata",
					Err:     err,
				}
			}
		}

		result.Document = &doc
		result.Score = 1 - distance // Convert distance to similarity score
		result.Distance = distance

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, &VectorError{
			Code:    ErrCodeOperationFailed,
			Message: "failed to iterate search results",
			Err:     err,
		}
	}

	return results, nil
}
