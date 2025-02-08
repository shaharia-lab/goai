package mcp

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"
)

// ResourceManager handles resource-related operations.
type ResourceManager struct {
	resources map[string]Resource
}

// NewResourceManager creates a new ResourceManager instance.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		resources: make(map[string]Resource),
	}
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
		limit = 50 // Default limit
	}

	var nextCursor string

	// Convert map to slice for pagination
	allResources := make([]Resource, 0, len(rm.resources))
	for _, r := range rm.resources {
		allResources = append(allResources, r)
	}

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, r := range allResources {
			if r.URI == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Get resources for current page
	endIdx := startIdx + limit
	if endIdx > len(allResources) {
		endIdx = len(allResources)
	}

	if endIdx < len(allResources) {
		nextCursor = allResources[endIdx-1].URI
	}

	return ListResourcesResult{
		Resources:  allResources[startIdx:endIdx],
		NextCursor: nextCursor,
	}
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

	// Handle different content types
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
