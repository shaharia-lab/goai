package goai

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test data structures
type TestPerson struct {
	Name    string `json:"name" xml:"name"`
	Age     int    `json:"age" xml:"age"`
	IsAdmin bool   `json:"is_admin" xml:"is_admin"`
}

func TestJSONExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		response    LLMResponse
		target      interface{}
		expected    interface{}
		expectError bool
	}{
		{
			name: "successful extraction from raw JSON",
			response: LLMResponse{
				Text: `{"name": "John Doe", "age": 30, "is_admin": true}`,
			},
			target: &TestPerson{},
			expected: &TestPerson{
				Name:    "John Doe",
				Age:     30,
				IsAdmin: true,
			},
		},
		{
			name: "successful extraction from code block",
			response: LLMResponse{
				Text: "Here's the JSON response:\n```json\n{\"name\": \"John Doe\", \"age\": 30, \"is_admin\": true}\n```\nEnd of response.",
			},
			target: &TestPerson{},
			expected: &TestPerson{
				Name:    "John Doe",
				Age:     30,
				IsAdmin: true,
			},
		},
		{
			name: "invalid JSON",
			response: LLMResponse{
				Text: `{"name": "John Doe", "age": }`,
			},
			target:      &TestPerson{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewJSONExtractor(tt.target)
			result, err := extractor.Extract(tt.response)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestXMLExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		response    LLMResponse
		target      interface{}
		expected    interface{}
		expectError bool
	}{
		{
			name: "successful extraction from XML",
			response: LLMResponse{
				Text: `<person><name>John Doe</name><age>30</age><is_admin>true</is_admin></person>`,
			},
			target: &TestPerson{},
			expected: &TestPerson{
				Name:    "John Doe",
				Age:     30,
				IsAdmin: true,
			},
		},
		{
			name: "successful extraction from code block",
			response: LLMResponse{
				Text: "Here's the XML response:\n```xml\n<person><name>John Doe</name><age>30</age><is_admin>true</is_admin></person>\n```\nEnd of response.",
			},
			target: &TestPerson{},
			expected: &TestPerson{
				Name:    "John Doe",
				Age:     30,
				IsAdmin: true,
			},
		},
		{
			name: "invalid XML",
			response: LLMResponse{
				Text: `<person><name>John Doe</name><age>invalid</age></person>`,
			},
			target:      &TestPerson{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewXMLExtractor(tt.target)
			result, err := extractor.Extract(tt.response)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTagExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		response    LLMResponse
		tag         string
		expected    string
		expectError bool
	}{
		{
			name: "successful extraction",
			response: LLMResponse{
				Text: "Here's the result: <answer>42 is the answer</answer>",
			},
			tag:      "answer",
			expected: "42 is the answer",
		},
		{
			name: "multiline content",
			response: LLMResponse{
				Text: "<code>\nfunction hello() {\n  console.log('Hello');\n}\n</code>",
			},
			tag:      "code",
			expected: "function hello() {\n  console.log('Hello');\n}",
		},
		{
			name: "tag not found",
			response: LLMResponse{
				Text: "No tags here",
			},
			tag:         "result",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewTagExtractor(tt.tag)
			result, err := extractor.Extract(tt.response)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Example usage
func ExampleJSONExtractor() {
	// Define a struct to hold the response data
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	// Create a response with JSON data
	response := LLMResponse{
		Text: `{"name": "John Doe", "age": 30}`,
	}

	// Create an extractor with a target struct
	var person Person
	extractor := NewJSONExtractor(&person)

	// Extract the data
	result, err := extractor.Extract(response)
	if err != nil {
		panic(err)
	}

	// Use the extracted data
	extracted := result.(*Person)
	fmt.Println(extracted.Name) // Output: John Doe
}

func ExampleTagExtractor() {
	// Create a response with tagged content
	response := LLMResponse{
		Text: "The answer is: <result>42</result>",
	}

	// Create a tag extractor
	extractor := NewTagExtractor("result")

	// Extract the content
	result, err := extractor.Extract(response)
	if err != nil {
		panic(err)
	}

	// Use the extracted content
	fmt.Println(result.(string)) // Output: 42
}

func TestTableExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		response    LLMResponse
		format      string
		expected    *Table
		expectError bool
	}{
		{
			name: "markdown table with headers",
			response: LLMResponse{
				Text: `| Name  | Age | City    |
|-----------------------|
| John  | 30  | London  |
| Jane  | 25  | Paris   |
| Bob   | 35  | Berlin  |`,
			},
			format: "markdown",
			expected: &Table{
				Headers: []string{"Name", "Age", "City"},
				Rows: [][]string{
					{"John", "30", "London"},
					{"Jane", "25", "Paris"},
					{"Bob", "35", "Berlin"},
				},
				Format: "markdown",
			},
		},
		{
			name: "simple markdown table",
			response: LLMResponse{
				Text: `| Column1 | Column2 |
|----------|----------|
| Value1   | Value2   |`,
			},
			format: "markdown",
			expected: &Table{
				Headers: []string{"Column1", "Column2"},
				Rows: [][]string{
					{"Value1", "Value2"},
				},
				Format: "markdown",
			},
		},
		{
			name: "table with extra whitespace and text",
			response: LLMResponse{
				Text: `Here's the data:

| Name   | Score |
|--------|-------|
| Alice  | 95    |
| Bob    | 87    |

Additional notes below.`,
			},
			format: "markdown",
			expected: &Table{
				Headers: []string{"Name", "Score"},
				Rows: [][]string{
					{"Alice", "95"},
					{"Bob", "87"},
				},
				Format: "markdown",
			},
		},
		{
			name: "table with empty cells",
			response: LLMResponse{
				Text: `| Name  | Age | Notes    |
|--------|-----|----------|
| John   |     | Active   |
| Jane   | 25  |          |`,
			},
			format: "markdown",
			expected: &Table{
				Headers: []string{"Name", "Age", "Notes"},
				Rows: [][]string{
					{"John", "", "Active"},
					{"Jane", "25", ""},
				},
				Format: "markdown",
			},
		},
		{
			name: "no table in response",
			response: LLMResponse{
				Text: "This is just regular text with no table.",
			},
			format:      "markdown",
			expectError: true,
		},
		{
			name: "malformed table",
			response: LLMResponse{
				Text: `| Column1 | Column2
Value1 | Value2`,
			},
			format:      "markdown",
			expectError: true,
		},
		{
			name: "table with numeric data",
			response: LLMResponse{
				Text: `| Item     | Quantity | Price |
|----------|-----------|--------|
| Apple    | 5         | 2.99   |
| Orange   | 3         | 1.99   |
| Banana   | 4         | 0.99   |`,
			},
			format: "markdown",
			expected: &Table{
				Headers: []string{"Item", "Quantity", "Price"},
				Rows: [][]string{
					{"Apple", "5", "2.99"},
					{"Orange", "3", "1.99"},
					{"Banana", "4", "0.99"},
				},
				Format: "markdown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewTableExtractor(tt.format)
			result, err := extractor.Extract(tt.response)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			table := result.(*Table)
			assert.Equal(t, tt.expected.Headers, table.Headers)
			assert.Equal(t, tt.expected.Rows, table.Rows)
			assert.Equal(t, tt.expected.Format, table.Format)

			// Additional validation
			if len(table.Headers) > 0 {
				// Verify all rows have the same number of columns as headers
				for i, row := range table.Rows {
					assert.Equal(t, len(table.Headers), len(row),
						"Row %d has incorrect number of columns", i)
				}
			}
		})
	}
}

func ExampleTableExtractor() {
	// Create a response containing a markdown table
	response := LLMResponse{
		Text: `| Name | Age |
|------|-----|
| John | 30  |
| Jane | 25  |`,
	}

	// Create a table extractor
	extractor := NewTableExtractor("markdown")

	// Extract the table
	result, err := extractor.Extract(response)
	if err != nil {
		panic(err)
	}

	// Use the extracted table
	table := result.(*Table)
	fmt.Printf("Headers: %v\n", table.Headers)
	fmt.Printf("First row: %v\n", table.Rows[0])
	// Output:
	// Headers: [Name Age]
	// First row: [John 30]
}
