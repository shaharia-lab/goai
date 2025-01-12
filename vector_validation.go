// Package ai provides artificial intelligence utilities including vector storage capabilities.
package ai

import (
	"fmt"
	"regexp"
	"time"
)

var (
	// validCollectionName defines the pattern for valid collection names
	validCollectionName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{2,63}$`)

	// validFieldName defines the pattern for valid field names
	validFieldName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,63}$`)
)

// VectorValidator provides validation utilities for vector storage operations.
type VectorValidator struct {
	maxDimension int
}

// NewVectorValidator creates a new validator with specified constraints.
func NewVectorValidator(maxDimension int) *VectorValidator {
	return &VectorValidator{
		maxDimension: maxDimension,
	}
}

// ValidateCollection validates a collection configuration.
func (v *VectorValidator) ValidateCollection(config *VectorCollectionConfig) error {
	if config == nil {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "collection config cannot be nil",
		}
	}

	// Validate collection name
	if !validCollectionName.MatchString(config.Name) {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "invalid collection name format",
		}
	}

	// Validate dimension
	if config.Dimension <= 0 || config.Dimension > v.maxDimension {
		return &VectorError{
			Code:    ErrCodeInvalidDimension,
			Message: fmt.Sprintf("dimension must be between 1 and %d", v.maxDimension),
		}
	}

	// Validate custom fields
	for name, field := range config.CustomFields {
		if err := v.validateField(name, field); err != nil {
			return err
		}
	}

	return nil
}

// ValidateDocument validates a document before storage.
func (v *VectorValidator) ValidateDocument(doc *VectorDocument, config *VectorCollectionConfig) error {
	if doc == nil {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "document cannot be nil",
		}
	}

	// Validate ID
	if doc.ID == "" {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: "document ID cannot be empty",
		}
	}

	// Validate vector dimension
	if len(doc.Vector) != config.Dimension {
		return &VectorError{
			Code:    ErrCodeInvalidDimension,
			Message: fmt.Sprintf("expected vector dimension %d, got %d", config.Dimension, len(doc.Vector)),
		}
	}

	// Validate timestamps
	now := time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = now
	}

	// Validate custom fields
	if err := v.validateCustomFields(doc.Metadata, config.CustomFields); err != nil {
		return err
	}

	return nil
}

// validateField validates a single custom field configuration.
func (v *VectorValidator) validateField(name string, field VectorFieldConfig) error {
	if !validFieldName.MatchString(name) {
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: fmt.Sprintf("invalid field name format: %s", name),
		}
	}

	// Validate field type
	switch field.Type {
	case "string", "int", "float", "bool", "datetime", "array":
		// Valid types
	default:
		return &VectorError{
			Code:    ErrCodeInvalidConfig,
			Message: fmt.Sprintf("invalid field type: %s", field.Type),
		}
	}

	return nil
}

// validateCustomFields validates document metadata against the schema.
func (v *VectorValidator) validateCustomFields(metadata map[string]interface{}, schema map[string]VectorFieldConfig) error {
	for name, field := range schema {
		value, exists := metadata[name]

		// Check required fields
		if field.Required && !exists {
			return &VectorError{
				Code:    ErrCodeInvalidConfig,
				Message: fmt.Sprintf("required field missing: %s", name),
			}
		}

		if exists {
			// Validate field type
			if err := v.validateFieldType(value, field.Type); err != nil {
				return &VectorError{
					Code:    ErrCodeInvalidConfig,
					Message: fmt.Sprintf("invalid value for field %s: %v", name, err),
				}
			}
		}
	}

	return nil
}

// validateFieldType validates a field value against its expected type.
func (v *VectorValidator) validateFieldType(value interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "int":
		switch value.(type) {
		case int, int32, int64:
			// Valid integer types
		default:
			return fmt.Errorf("expected integer, got %T", value)
		}
	case "float":
		switch value.(type) {
		case float32, float64:
			// Valid float types
		default:
			return fmt.Errorf("expected float, got %T", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "datetime":
		if _, ok := value.(time.Time); !ok {
			return fmt.Errorf("expected datetime, got %T", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	}

	return nil
}
