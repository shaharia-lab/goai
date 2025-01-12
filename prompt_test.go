package ai

import (
	"testing"
)

func TestLLMPromptTemplate_Parse(t *testing.T) {
	tests := []struct {
		name           string
		template       string
		data           map[string]interface{}
		expectedOutput string
		expectError    bool
	}{
		{
			name:     "simple template",
			template: "Hello, {{.Name}}!",
			data: map[string]interface{}{
				"Name": "World",
			},
			expectedOutput: "Hello, World!",
		},
		{
			name:     "complex template",
			template: "{{if .Premium}}Welcome back, {{.Name}}!{{else}}Hello, {{.Name}}!{{end}}",
			data: map[string]interface{}{
				"Name":    "User",
				"Premium": true,
			},
			expectedOutput: "Welcome back, User!",
		},
		{
			name:        "invalid template syntax",
			template:    "Hello, {{.Name!",
			expectError: true,
		},
		{
			name:     "with missing field",
			template: "Hello, {{.Name}}!",
			data: map[string]interface{}{
				"OtherField": "World",
			},
			expectedOutput: "Hello, <no value>!",
		},
		{
			name:           "with nil data",
			template:       "Hello, {{.Name}}!",
			data:           nil,
			expectedOutput: "Hello, <no value>!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := &LLMPromptTemplate{
				Template: tt.template,
				Data:     tt.data,
			}

			result, err := prompt.Parse()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedOutput {
				t.Errorf("expected %q, got %q", tt.expectedOutput, result)
			}
		})
	}
}
