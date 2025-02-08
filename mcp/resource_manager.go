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

	// Get all URIs and sort them for consistent ordering
	var uris []string
	for uri := range rm.resources {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, uri := range uris {
			if uri == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Calculate end index
	endIdx := startIdx + limit
	if endIdx > len(uris) {
		endIdx = len(uris)
	}

	// Get resources for current page
	var pageResources []Resource
	for i := startIdx; i < endIdx; i++ {
		pageResources = append(pageResources, rm.resources[uris[i]])
	}

	// Set next cursor
	var nextCursor string
	if endIdx < len(uris) {
		nextCursor = uris[endIdx-1]
	}

	return ListResourcesResult{
		Resources:  pageResources,
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
