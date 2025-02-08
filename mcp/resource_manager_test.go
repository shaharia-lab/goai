package mcp

import (
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

	// Add test resources
	resources := []Resource{
		{URI: "test://1.txt", Name: "1.txt", MimeType: "text/plain"},
		{URI: "test://2.txt", Name: "2.txt", MimeType: "text/plain"},
		{URI: "test://3.txt", Name: "3.txt", MimeType: "text/plain"},
	}

	for _, r := range resources {
		rm.AddResource(r)
	}

	tests := []struct {
		name           string
		cursor         string
		limit          int
		expectedCount  int
		expectNextPage bool
	}{
		{
			name:           "list all with default limit",
			cursor:         "",
			limit:          0,
			expectedCount:  3,
			expectNextPage: false,
		},
		{
			name:           "list with limit 2",
			cursor:         "",
			limit:          2,
			expectedCount:  2,
			expectNextPage: true,
		},
		{
			name:           "list with cursor",
			cursor:         "test://1.txt",
			limit:          2,
			expectedCount:  2,
			expectNextPage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rm.ListResources(tt.cursor, tt.limit)

			if len(result.Resources) != tt.expectedCount {
				t.Errorf("expected %d resources, got %d", tt.expectedCount, len(result.Resources))
			}

			if tt.expectNextPage && result.NextCursor == "" {
				t.Error("expected next cursor but got empty string")
			}

			if !tt.expectNextPage && result.NextCursor != "" {
				t.Error("expected no next cursor but got one")
			}
		})
	}
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
