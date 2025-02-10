package mcp

import (
	"encoding/base64"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewResourceManager(t *testing.T) {
	rm, err := NewResourceManager([]Resource{})
	assert.NoError(t, err)

	if rm == nil {
		t.Fatal("NewResourceManager returned nil")
	}
	if rm.resources == nil {
		t.Fatal("resources map was not initialized")
	}
}

func TestAddResource(t *testing.T) {
	rm, _ := NewResourceManager([]Resource{})

	tests := []struct {
		name        string
		resource    Resource
		expectError bool
	}{
		{
			name: "valid resource",
			resource: Resource{
				URI:      "test://example.com/file.txt",
				Name:     "file.txt",
				MimeType: "text/plain",
			},
			expectError: false,
		},
		{
			name: "empty URI",
			resource: Resource{
				URI:      "",
				Name:     "file.txt",
				MimeType: "text/plain",
			},
			expectError: true,
		},
		{
			name: "missing name should use URI base",
			resource: Resource{
				URI:      "test://example.com/file.txt",
				MimeType: "text/plain",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.AddResource(tt.resource)
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetResource(t *testing.T) {
	rm, _ := NewResourceManager([]Resource{})
	testResource := Resource{
		URI:      "test://example.com/file.txt",
		Name:     "file.txt",
		MimeType: "text/plain",
	}
	rm.AddResource(testResource)

	tests := []struct {
		name        string
		uri         string
		expectError bool
	}{
		{
			name:        "existing resource",
			uri:         "test://example.com/file.txt",
			expectError: false,
		},
		{
			name:        "non-existent resource",
			uri:         "test://example.com/nonexistent.txt",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, err := rm.GetResource(tt.uri)
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resource.URI != tt.uri {
					t.Errorf("expected URI %s, got %s", tt.uri, resource.URI)
				}
			}
		})
	}
}

func TestListResources(t *testing.T) {
	rm, _ := NewResourceManager([]Resource{})

	// Add resources in non-sequential order
	resources := []Resource{
		{URI: "d_res", Name: "d_res", MimeType: "text/plain"},
		{URI: "a_res", Name: "a_res", MimeType: "text/plain"},
		{URI: "c_res", Name: "c_res", MimeType: "text/plain"},
		{URI: "b_res", Name: "b_res", MimeType: "text/plain"},
	}

	for _, r := range resources {
		err := rm.AddResource(r)
		assert.NoError(t, err)
	}

	t.Run("list_with_cursor", func(t *testing.T) {
		// First page
		result := rm.ListResources("", 2)
		assert.Len(t, result.Resources, 2, "First page should have 2 resources")
		assert.Equal(t, "a_res", result.Resources[0].URI)
		assert.Equal(t, "b_res", result.Resources[1].URI)
		assert.Equal(t, "b_res", result.NextCursor)

		// Second page
		result = rm.ListResources(result.NextCursor, 2)
		assert.Len(t, result.Resources, 2, "Second page should have 2 resources")
		assert.Equal(t, "c_res", result.Resources[0].URI)
		assert.Equal(t, "d_res", result.Resources[1].URI)
		assert.Empty(t, result.NextCursor)
	})
}

func TestReadResource(t *testing.T) {
	rm, _ := NewResourceManager([]Resource{})

	textResource := Resource{
		URI:         "file://file.txt",
		Name:        "file.txt",
		MimeType:    "text/plain",
		TextContent: "Hello, World!",
	}

	binaryResource := Resource{
		URI:         "file://file.bin",
		Name:        "file.bin",
		MimeType:    "application/octet-stream",
		TextContent: "binary content",
	}

	rm.AddResource(textResource)
	rm.AddResource(binaryResource)

	tests := []struct {
		name        string
		params      ReadResourceParams
		expectError bool
		checkText   bool
	}{
		{
			name:        "read text resource",
			params:      ReadResourceParams{URI: "file://file.txt"},
			expectError: false,
			checkText:   true,
		},
		{
			name:        "read binary resource",
			params:      ReadResourceParams{URI: "file://file.bin"},
			expectError: false,
			checkText:   false,
		},
		{
			name:        "read non-existent resource",
			params:      ReadResourceParams{URI: "file://nonexistent.txt"},
			expectError: true,
			checkText:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rm.ReadResource(tt.params)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
				return
			}

			if !tt.expectError {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if len(result.Contents) != 1 {
					t.Errorf("expected 1 content, got %d", len(result.Contents))
					return
				}

				content := result.Contents[0]
				if tt.checkText {
					if content.Text == "" {
						t.Error("expected non-empty text content")
					}
				} else {
					if content.Blob == "" {
						t.Error("expected non-empty blob content")
					}
				}
			}
		})
	}
}

func TestValidateResource(t *testing.T) {
	tests := []struct {
		name        string
		resource    Resource
		expectError bool
	}{
		{
			name: "valid resource",
			resource: Resource{
				URI:      "file://example.com/file.txt",
				Name:     "file.txt",
				MimeType: "text/plain",
			},
			expectError: false,
		},
		{
			name: "empty MIME type",
			resource: Resource{
				URI:  "file://example.com/file.txt",
				Name: "file.txt",
			},
			expectError: true,
		},
		{
			name: "empty URI and MIME type",
			resource: Resource{
				Name: "file.txt",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResource(tt.resource)
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestListResourcesEdgeCases(t *testing.T) {
	rm, _ := NewResourceManager([]Resource{})

	t.Run("empty_resources", func(t *testing.T) {
		result := rm.ListResources("", 10)
		assert.Empty(t, result.Resources)
		assert.Empty(t, result.NextCursor)
	})

	t.Run("negative_limit", func(t *testing.T) {
		resource := Resource{
			URI:      "test://example.com/file.txt",
			Name:     "file.txt",
			MimeType: "text/plain",
		}
		rm.AddResource(resource)

		result := rm.ListResources("", -1)
		assert.Len(t, result.Resources, 1)
		assert.Empty(t, result.NextCursor)
	})

	t.Run("invalid_cursor", func(t *testing.T) {
		result := rm.ListResources("nonexistent", 10)
		assert.Empty(t, result.Resources)
		assert.Empty(t, result.NextCursor)
	})
}

func TestReadResourceBinaryContent(t *testing.T) {
	rm, _ := NewResourceManager([]Resource{})

	binaryData := []byte{0x00, 0x01, 0x02, 0x03}
	binaryResource := Resource{
		URI:         "file://file.bin",
		Name:        "file.bin",
		MimeType:    "application/octet-stream",
		TextContent: string(binaryData),
	}

	err := rm.AddResource(binaryResource)
	assert.NoError(t, err)

	result, err := rm.ReadResource(ReadResourceParams{URI: "file://file.bin"})
	assert.NoError(t, err)
	assert.Len(t, result.Contents, 1)

	decodedData, err := base64.StdEncoding.DecodeString(result.Contents[0].Blob)
	assert.NoError(t, err)
	assert.Equal(t, binaryData, decodedData)
}

func TestNewResourceManagerWithMultipleResources(t *testing.T) {
	resources := []Resource{
		{
			URI:      "test://example.com/file1.txt",
			Name:     "file1.txt",
			MimeType: "text/plain",
		},
		{
			URI:      "test://example.com/file2.txt",
			Name:     "file2.txt",
			MimeType: "text/plain",
		},
	}

	rm, err := NewResourceManager(resources)
	assert.NoError(t, err)
	assert.Len(t, rm.resources, 2)

	// Test initialization with invalid resource
	invalidResources := append(resources, Resource{})
	rm, err = NewResourceManager(invalidResources)
	assert.Error(t, err)
}
