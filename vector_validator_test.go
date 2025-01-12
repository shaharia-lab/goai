// Package ai provides artificial intelligence utilities including vector storage capabilities.
package ai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewVectorValidator(t *testing.T) {
	validator := NewVectorValidator(384)
	assert.NotNil(t, validator)
	assert.Equal(t, 384, validator.maxDimension)
}

func TestVectorValidator_ValidateCollection(t *testing.T) {
	validator := NewVectorValidator(384)

	tests := []struct {
		name        string
		config      *VectorCollectionConfig
		expectError bool
		errorCode   int
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
		{
			name: "valid config",
			config: &VectorCollectionConfig{
				Name:         "test_collection",
				Dimension:    384,
				IndexType:    IndexTypeHNSW,
				DistanceType: DistanceTypeCosine,
			},
			expectError: false,
		},
		{
			name: "invalid collection name",
			config: &VectorCollectionConfig{
				Name:      "123invalid",
				Dimension: 384,
			},
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
		{
			name: "dimension too small",
			config: &VectorCollectionConfig{
				Name:      "test_collection",
				Dimension: 0,
			},
			expectError: true,
			errorCode:   ErrCodeInvalidDimension,
		},
		{
			name: "dimension too large",
			config: &VectorCollectionConfig{
				Name:      "test_collection",
				Dimension: 1000,
			},
			expectError: true,
			errorCode:   ErrCodeInvalidDimension,
		},
		{
			name: "valid custom fields",
			config: &VectorCollectionConfig{
				Name:      "test_collection",
				Dimension: 384,
				CustomFields: map[string]VectorFieldConfig{
					"field1": {Type: "string", Required: true},
					"field2": {Type: "int", Required: false},
				},
			},
			expectError: false,
		},
		{
			name: "invalid custom field name",
			config: &VectorCollectionConfig{
				Name:      "test_collection",
				Dimension: 384,
				CustomFields: map[string]VectorFieldConfig{
					"123invalid": {Type: "string"},
				},
			},
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
		{
			name: "invalid field type",
			config: &VectorCollectionConfig{
				Name:      "test_collection",
				Dimension: 384,
				CustomFields: map[string]VectorFieldConfig{
					"field1": {Type: "invalid_type"},
				},
			},
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCollection(tt.config)
			if tt.expectError {
				assert.Error(t, err)
				if vecErr, ok := err.(*VectorError); ok {
					assert.Equal(t, tt.errorCode, vecErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVectorValidator_ValidateDocument(t *testing.T) {
	validator := NewVectorValidator(384)
	config := &VectorCollectionConfig{
		Name:      "test_collection",
		Dimension: 3,
		CustomFields: map[string]VectorFieldConfig{
			"category": {Type: "string", Required: true},
			"count":    {Type: "int", Required: false},
			"tags":     {Type: "array", Required: false},
		},
	}

	tests := []struct {
		name        string
		doc         *VectorDocument
		expectError bool
		errorCode   int
	}{
		{
			name:        "nil document",
			doc:         nil,
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
		{
			name: "valid document",
			doc: &VectorDocument{
				ID:      "test_doc",
				Vector:  []float32{0.1, 0.2, 0.3},
				Content: "test content",
				Metadata: map[string]interface{}{
					"category": "test",
					"count":    42,
					"tags":     []interface{}{"tag1", "tag2"},
				},
			},
			expectError: false,
		},
		{
			name: "empty ID",
			doc: &VectorDocument{
				ID:     "",
				Vector: []float32{0.1, 0.2, 0.3},
			},
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
		{
			name: "wrong vector dimension",
			doc: &VectorDocument{
				ID:     "test_doc",
				Vector: []float32{0.1, 0.2, 0.3, 0.4},
			},
			expectError: true,
			errorCode:   ErrCodeInvalidDimension,
		},
		{
			name: "missing required field",
			doc: &VectorDocument{
				ID:       "test_doc",
				Vector:   []float32{0.1, 0.2, 0.3},
				Metadata: map[string]interface{}{},
			},
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
		{
			name: "wrong field type",
			doc: &VectorDocument{
				ID:     "test_doc",
				Vector: []float32{0.1, 0.2, 0.3},
				Metadata: map[string]interface{}{
					"category": 123,  // Should be string
					"count":    "42", // Should be int
				},
			},
			expectError: true,
			errorCode:   ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateDocument(tt.doc, config)
			if tt.expectError {
				assert.Error(t, err)
				if vecErr, ok := err.(*VectorError); ok {
					assert.Equal(t, tt.errorCode, vecErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVectorValidator_ValidateFieldType(t *testing.T) {
	validator := NewVectorValidator(384)

	tests := []struct {
		name         string
		value        interface{}
		expectedType string
		expectError  bool
	}{
		{
			name:         "valid string",
			value:        "test",
			expectedType: "string",
			expectError:  false,
		},
		{
			name:         "invalid string",
			value:        123,
			expectedType: "string",
			expectError:  true,
		},
		{
			name:         "valid int",
			value:        42,
			expectedType: "int",
			expectError:  false,
		},
		{
			name:         "invalid int",
			value:        "42",
			expectedType: "int",
			expectError:  true,
		},
		{
			name:         "valid float32",
			value:        float32(3.14),
			expectedType: "float",
			expectError:  false,
		},
		{
			name:         "valid float64",
			value:        3.14,
			expectedType: "float",
			expectError:  false,
		},
		{
			name:         "invalid float",
			value:        "3.14",
			expectedType: "float",
			expectError:  true,
		},
		{
			name:         "valid bool",
			value:        true,
			expectedType: "bool",
			expectError:  false,
		},
		{
			name:         "invalid bool",
			value:        1,
			expectedType: "bool",
			expectError:  true,
		},
		{
			name:         "valid datetime",
			value:        time.Now(),
			expectedType: "datetime",
			expectError:  false,
		},
		{
			name:         "invalid datetime",
			value:        "2024-01-01",
			expectedType: "datetime",
			expectError:  true,
		},
		{
			name:         "valid array",
			value:        []interface{}{"a", "b", "c"},
			expectedType: "array",
			expectError:  false,
		},
		{
			name:         "invalid array",
			value:        []string{"a", "b", "c"},
			expectedType: "array",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateFieldType(tt.value, tt.expectedType)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
