package ai

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestChunkingByLLMProvider_Chunk(t *testing.T) {
	const testDocument = "This is a test document. It has multiple sentences. We want to chunk it properly."

	tests := []struct {
		name           string
		inputText      string
		llmResponse    LLMResponse
		expectedChunks []string
		expectError    bool
	}{
		{
			name:      "successful chunking",
			inputText: testDocument,
			llmResponse: LLMResponse{
				// Aligning with exact string boundaries:
				// "This is a test document." = 24 chars
				// " It has multiple sentences." = 27 chars
				// " We want to chunk it properly." = 30 chars
				Text: `[[0, 24], [24, 51], [51, 81]]`,
			},
			expectedChunks: []string{
				"This is a test document.",       // 0-24
				" It has multiple sentences.",    // 24-51
				" We want to chunk it properly.", // 51-81
			},
			expectError: false,
		},
		{
			name:      "invalid offset response format",
			inputText: "This is a test document.",
			llmResponse: LLMResponse{
				Text: "invalid format",
			},
			expectedChunks: nil,
			expectError:    true,
		},
		{
			name:      "out of bounds offset",
			inputText: "Short text.",
			llmResponse: LLMResponse{
				Text: "[[0, 100]]",
			},
			expectedChunks: nil,
			expectError:    true,
		},
		{
			name:      "invalid offset pairs",
			inputText: "Test document",
			llmResponse: LLMResponse{
				Text: "[[0, 5, 10]]",
			},
			expectedChunks: nil,
			expectError:    true,
		},
		{
			name:      "empty input text",
			inputText: "",
			llmResponse: LLMResponse{
				Text: "[]",
			},
			expectedChunks: []string{},
			expectError:    false,
		},
		{
			name:      "reversed offsets",
			inputText: "Test document",
			llmResponse: LLMResponse{
				Text: "[[5, 2]]",
			},
			expectedChunks: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create NoOps provider with test response
			noOpsProvider := NewNoOpsLLMProvider(WithResponse(tt.llmResponse))
			llm := NewLLMRequest(LLMRequestConfig{}, noOpsProvider)

			// Create chunking provider
			provider := NewChunkingByLLMProvider(llm)

			// Execute chunking
			chunks, err := provider.Chunk(context.Background(), tt.inputText)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check chunks if no error expected
			if !tt.expectError {
				if !reflect.DeepEqual(chunks, tt.expectedChunks) {
					t.Errorf("chunks mismatch\ngot:  %q\nwant: %q", chunks, tt.expectedChunks)
					for i, chunk := range chunks {
						t.Logf("chunk[%d] length: %d, content: %q", i, len(chunk), chunk)
					}
					if len(tt.expectedChunks) > 0 {
						for i, chunk := range tt.expectedChunks {
							t.Logf("expected[%d] length: %d, content: %q", i, len(chunk), chunk)
						}
					}
				}
			}
		})
	}
}

func TestParseOffsets(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        []Offset
		expectError bool
	}{
		{
			name:  "valid offsets",
			input: "[[0, 10], [11, 20]]",
			want: []Offset{
				{Start: 0, End: 10},
				{Start: 11, End: 20},
			},
			expectError: false,
		},
		{
			name:        "invalid JSON format",
			input:       "[0, 10], [11, 20]",
			want:        nil,
			expectError: true,
		},
		{
			name:        "missing brackets",
			input:       "[0, 10]",
			want:        nil,
			expectError: true,
		},
		{
			name:        "invalid array elements",
			input:       "[[0], [11, 20]]",
			want:        nil,
			expectError: true,
		},
		{
			name:        "non-numeric values",
			input:       `[["a", 10], [11, "b"]]`,
			want:        nil,
			expectError: true,
		},
		{
			name:        "empty array",
			input:       "[]",
			want:        []Offset{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOffsets(tt.input)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseOffsets() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChunkingPromptTemplate(t *testing.T) {
	template := &LLMPromptTemplate{
		Template: chunkingPromptTemplate,
		Data: map[string]interface{}{
			"Text": "Sample text for testing",
		},
	}

	prompt, err := template.Parse()
	if err != nil {
		t.Errorf("failed to parse template: %v", err)
	}

	// Verify the prompt contains the input text
	if !strings.Contains(prompt, "Sample text for testing") {
		t.Error("parsed prompt does not contain input text")
	}

	// Verify template markers are replaced
	if strings.Contains(prompt, "{{.Text}}") {
		t.Error("template markers not replaced in parsed prompt")
	}
}
