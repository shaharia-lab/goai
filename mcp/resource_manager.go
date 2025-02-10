package mcp

import (
	"encoding/base64"
	"fmt"
	"path"
	"sort"
	"strings"
)

// ResourceManager handles resource-related operations.
type ResourceManager struct {
	resources map[string]Resource
}

// NewResourceManager creates a new ResourceManager instance with validation.
func NewResourceManager(resources []Resource) (*ResourceManager, error) {
	r := make(map[string]Resource)
	rm := &ResourceManager{
		resources: r,
	}

	// Validate and add each resource
	for _, resource := range resources {
		if err := validateResource(resource); err != nil {
			return nil, fmt.Errorf("invalid resource: %v", err)
		}

		// Set default name if not provided
		if resource.Name == "" {
			resource.Name = path.Base(resource.URI)
		}

		r[resource.URI] = resource
	}

	return rm, nil
}

// validateResource checks if a resource is valid
func validateResource(resource Resource) error {
	if resource.URI == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	// Add additional validation rules as needed
	if resource.MimeType == "" {
		return fmt.Errorf("resource MIME type cannot be empty")
	}

	return nil
}

// ListResourcesResult represents the result of listing resources.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceParams represents parameters for reading a resource.
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// ReadResourceResult represents the result of reading a resource.
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// AddResource adds a new resource to the manager.
func (rm *ResourceManager) AddResource(resource Resource) error {
	if resource.URI == "" {
		return fmt.Errorf("resource URI cannot be empty")
	}

	if resource.Name == "" {
		// Use last part of URI as name if not provided
		resource.Name = path.Base(resource.URI)
	}

	rm.resources[resource.URI] = resource
	return nil
}

// GetResource retrieves a resource by its URI.
func (rm *ResourceManager) GetResource(uri string) (Resource, error) {
	resource, exists := rm.resources[uri]
	if !exists {
		return Resource{}, fmt.Errorf("resource not found: %s", uri)
	}
	return resource, nil
}

// ListResources returns a list of all resources, with optional pagination.
func (rm *ResourceManager) ListResources(cursor string, limit int) ListResourcesResult {
	if limit <= 0 {
		limit = 50
	}

	// Initialize with empty slice instead of nil
	result := ListResourcesResult{
		Resources: []Resource{},
	}

	// If there are no resources, return empty result
	if len(rm.resources) == 0 {
		return result
	}

	var uris []string
	for uri := range rm.resources {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	startIdx := 0
	if cursor != "" {
		cursorFound := false
		for i, uri := range uris {
			if uri == cursor {
				startIdx = i + 1
				cursorFound = true
				break
			}
		}
		// Return empty result if cursor not found
		if !cursorFound {
			return result
		}
	}

	// If start index is beyond array length, return empty result
	if startIdx >= len(uris) {
		return result
	}

	endIdx := startIdx + limit
	if endIdx > len(uris) {
		endIdx = len(uris)
	}

	for i := startIdx; i < endIdx; i++ {
		result.Resources = append(result.Resources, rm.resources[uris[i]])
	}

	if endIdx < len(uris) {
		result.NextCursor = uris[endIdx-1]
	}

	return result
}

// ReadResource reads the content of a resource.
func (rm *ResourceManager) ReadResource(params ReadResourceParams) (ReadResourceResult, error) {
	resource, err := rm.GetResource(params.URI)
	if err != nil {
		return ReadResourceResult{}, err
	}

	content := ResourceContent{
		URI:      resource.URI,
		MimeType: resource.MimeType,
	}

	if strings.HasPrefix(resource.MimeType, "text/") {
		content.Text = resource.TextContent
	} else {
		// For non-text content, encode as base64
		content.Blob = base64.StdEncoding.EncodeToString([]byte(resource.TextContent))
	}

	return ReadResourceResult{
		Contents: []ResourceContent{content},
	}, nil
}
