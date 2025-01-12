// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"context"
	"fmt"
	"time"
)

// LLMMessageRole represents the role of a message in a conversation.
type LLMMessageRole string

const (
	// UserRole represents a message from the user
	UserRole LLMMessageRole = "user"

	// AssistantRole represents a message from the assistant
	AssistantRole LLMMessageRole = "assistant"

	// SystemRole represents a message from the system
	SystemRole LLMMessageRole = "system"
)

// DefaultConfig holds the default values for LLMRequestConfig
var DefaultConfig = LLMRequestConfig{
	MaxToken:    1000, // Default max tokens
	TopP:        0.9,  // Default top-p value
	Temperature: 0.7,  // Default temperature
	TopK:        50,   // Default top-k value
}

// LLMRequestConfig defines configuration parameters for LLM requests.
type LLMRequestConfig struct {
	MaxToken    int64
	TopP        float64
	Temperature float64
	TopK        int64
}

// NewRequestConfig creates a new config with default values.
// Any non-zero values in the provided config will override the defaults.
func NewRequestConfig(opts ...RequestOption) LLMRequestConfig {
	config := DefaultConfig

	// Apply any provided options
	for _, opt := range opts {
		opt(&config)
	}

	return config
}

// RequestOption is a function that modifies the config
type RequestOption func(*LLMRequestConfig)

// WithMaxToken sets the max token value
func WithMaxToken(maxToken int64) RequestOption {
	return func(c *LLMRequestConfig) {
		if maxToken > 0 {
			c.MaxToken = maxToken
		}
	}
}

// WithTopP sets the top-p value
func WithTopP(topP float64) RequestOption {
	return func(c *LLMRequestConfig) {
		if topP > 0 {
			c.TopP = topP
		}
	}
}

// WithTemperature sets the temperature value
func WithTemperature(temp float64) RequestOption {
	return func(c *LLMRequestConfig) {
		if temp > 0 {
			c.Temperature = temp
		}
	}
}

// WithTopK sets the top-k value
func WithTopK(topK int64) RequestOption {
	return func(c *LLMRequestConfig) {
		if topK > 0 {
			c.TopK = topK
		}
	}
}

// LLMResponse encapsulates the response from an LLM provider.
// It includes both the generated text and metadata about the request.
type LLMResponse struct {
	// Text contains the generated response from the model
	Text string

	// TotalInputToken is the number of tokens in the input prompt
	TotalInputToken int

	// TotalOutputToken is the number of tokens in the generated response
	TotalOutputToken int

	// CompletionTime is the total time taken to generate the response in seconds
	CompletionTime float64
}

// LLMError represents errors that occur during LLM operations.
// It provides structured error information including an error code.
type LLMError struct {
	// Code represents the error code (usually HTTP status code for API errors)
	Code int

	// Message provides a detailed description of the error
	Message string
}

// Error implements the error interface for LLMError.
func (e *LLMError) Error() string {
	return fmt.Sprintf("LLMError %d: %s", e.Code, e.Message)
}

// LLMMessage represents a message in a conversation with an LLM.
// It includes the role of the speaker (user, assistant, etc.) and the text of the message.
type LLMMessage struct {
	Role LLMMessageRole
	Text string
}

// StreamingLLMResponse represents a chunk of streaming response from an LLM provider.
// It contains partial text, completion status, any errors, and token usage information.
type StreamingLLMResponse struct {
	// Text contains the partial response text
	Text string
	// Done indicates if this is the final chunk
	Done bool
	// Error contains any error that occurred during streaming
	Error error
	// TokenCount is the number of tokens in this chunk
	TokenCount int
}

// LLMProvider defines the interface that all LLM providers must implement.
// This allows for easy swapping between different LLM providers.
type LLMProvider interface {
	// GetResponse generates a response for the given question using the specified configuration.
	// Returns LLMResponse containing the generated text and metadata, or an error if the operation fails.
	GetResponse(messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error)
	GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error)
}

// VectorDocument represents a document with its embedding vector and metadata.
type VectorDocument struct {
	// ID is the unique identifier for the document
	ID string `json:"id"`

	// Vector is the embedding representation of the document
	Vector []float32 `json:"vector"`

	// Content is the original text content of the document
	Content string `json:"content"`

	// Metadata stores additional information about the document
	Metadata map[string]interface{} `json:"metadata"`

	// CreatedAt is the timestamp when the document was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the timestamp when the document was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// VectorIndexType represents the type of index used for vector similarity search
type VectorIndexType string

const (
	// IndexTypeFlat represents a flat (brute force) index
	IndexTypeFlat VectorIndexType = "flat"

	// IndexTypeIVFFlat represents an IVF (Inverted File) flat index
	IndexTypeIVFFlat VectorIndexType = "ivf_flat"

	// IndexTypeHNSW represents a Hierarchical Navigable Small World graph index
	IndexTypeHNSW VectorIndexType = "hnsw"
)

// VectorDistanceType represents the distance metric used for similarity search
type VectorDistanceType string

const (
	// DistanceTypeCosine represents cosine similarity distance metric
	DistanceTypeCosine VectorDistanceType = "cosine"

	// DistanceTypeEuclidean represents Euclidean distance metric
	DistanceTypeEuclidean VectorDistanceType = "euclidean"

	// DistanceTypeDotProduct represents dot product distance metric
	DistanceTypeDotProduct VectorDistanceType = "dot_product"
)

// VectorCollectionConfig defines the configuration for a vector collection
type VectorCollectionConfig struct {
	// Name is the identifier for the collection
	Name string `json:"name"`

	// Dimension is the size of the vectors in this collection
	Dimension int `json:"dimension"`

	// IndexType specifies the type of index to use for similarity search
	IndexType VectorIndexType `json:"index_type"`

	// DistanceType specifies the distance metric to use for similarity search
	DistanceType VectorDistanceType `json:"distance_type"`

	// CustomFields allows defining additional schema fields
	CustomFields map[string]VectorFieldConfig `json:"custom_fields,omitempty"`
}

// VectorFieldConfig defines the configuration for a custom field in the schema
type VectorFieldConfig struct {
	// Type specifies the data type of the field
	Type string `json:"type"`

	// Required indicates if the field must be present
	Required bool `json:"required"`

	// Indexed indicates if the field should be indexed for searching
	Indexed bool `json:"indexed"`
}

// VectorSearchOptions defines the options for vector similarity search
type VectorSearchOptions struct {
	// Limit specifies the maximum number of results to return
	Limit int `json:"limit"`

	// Offset specifies the number of results to skip
	Offset int `json:"offset"`

	// Filter is an optional query to filter results
	Filter map[string]interface{} `json:"filter,omitempty"`

	// IncludeMetadata indicates whether to include metadata in the results
	IncludeMetadata bool `json:"include_metadata"`

	// IncludeVectors indicates whether to include vectors in the results
	IncludeVectors bool `json:"include_vectors"`
}

// VectorSearchResult represents a single result from a vector similarity search
type VectorSearchResult struct {
	// Document is the matched document
	Document *VectorDocument `json:"document"`

	// Score is the similarity score (lower is more similar)
	Score float32 `json:"score"`

	// Distance is the actual distance value used for scoring
	Distance float32 `json:"distance"`
}

// VectorError represents errors that occur during vector storage operations.
type VectorError struct {
	Code    int
	Message string
	Err     error
}

// Error implements the error interface.
func (e *VectorError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("vector storage error %d: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("vector storage error %d: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error.
func (e *VectorError) Unwrap() error {
	return e.Err
}

// Common vector storage error codes
const (
	ErrCodeNotFound           = 404
	ErrCodeInvalidDimension   = 400
	ErrCodeInvalidConfig      = 401
	ErrCodeCollectionExists   = 402
	ErrCodeCollectionNotFound = 403
	ErrCodeInvalidVector      = 405
	ErrCodeConnectionFailed   = 500
	ErrCodeOperationFailed    = 501
)

// Common vector storage errors
var (
	ErrDocumentNotFound   = &VectorError{Code: ErrCodeNotFound, Message: "document not found"}
	ErrCollectionNotFound = &VectorError{Code: ErrCodeCollectionNotFound, Message: "collection not found"}
	ErrCollectionExists   = &VectorError{Code: ErrCodeCollectionExists, Message: "collection already exists"}
	ErrInvalidDimension   = &VectorError{Code: ErrCodeInvalidDimension, Message: "invalid vector dimension"}
	ErrInvalidConfig      = &VectorError{Code: ErrCodeInvalidConfig, Message: "invalid configuration"}
	ErrConnectionFailed   = &VectorError{Code: ErrCodeConnectionFailed, Message: "failed to connect to storage"}
	ErrInvalidVector      = &VectorError{Code: ErrCodeInvalidVector, Message: "invalid vector format or dimension"}
)

// VectorStorageProvider defines the interface that all storage providers must implement.
type VectorStorageProvider interface {
	// Initialize sets up the required database structure
	Initialize(ctx context.Context) error

	// Collection Operations
	CreateCollection(ctx context.Context, config *VectorCollectionConfig) error
	DeleteCollection(ctx context.Context, name string) error
	ListCollections(ctx context.Context) ([]string, error)

	// Document Operations
	UpsertDocument(ctx context.Context, collection string, doc *VectorDocument) error
	UpsertDocuments(ctx context.Context, collection string, docs []*VectorDocument) error
	GetDocument(ctx context.Context, collection, id string) (*VectorDocument, error)
	DeleteDocument(ctx context.Context, collection, id string) error

	// Search Operations
	SearchByVector(ctx context.Context, collection string, vector []float32, opts *VectorSearchOptions) ([]VectorSearchResult, error)
	SearchByID(ctx context.Context, collection, id string, opts *VectorSearchOptions) ([]VectorSearchResult, error)

	// Lifecycle Operations
	Close() error
}
