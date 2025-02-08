package mcp

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewResourceManager(t *testing.T) {
	rm := NewResourceManager()
	if rm == nil {
		t.Fatal("NewResourceManager returned nil")
	}
	if rm.resources == nil {
		t.Fatal("resources map was not initialized")
	}
}

func TestAddResource(t *testing.T) {
	rm := NewResourceManager()

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
	rm := NewResourceManager()
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
	rm := NewResourceManager()

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
	rm := NewResourceManager()

	textResource := Resource{
		URI:         "test://file.txt",
		Name:        "file.txt",
		MimeType:    "text/plain",
		TextContent: "Hello, World!",
	}

	binaryResource := Resource{
		URI:         "test://file.bin",
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
			params:      ReadResourceParams{URI: "test://file.txt"},
			expectError: false,
			checkText:   true,
		},
		{
			name:        "read binary resource",
			params:      ReadResourceParams{URI: "test://file.bin"},
			expectError: false,
			checkText:   false,
		},
		{
			name:        "read non-existent resource",
			params:      ReadResourceParams{URI: "test://nonexistent.txt"},
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
