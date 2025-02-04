// Package goai provides utilities for AI-powered text processing and response extraction.
package goai

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
)

// ResponseExtractor defines the interface for extracting structured data from LLM responses.
type ResponseExtractor interface {
	// Extract processes the LLM response and returns the extracted data.
	// The extracted data's type depends on the specific extractor implementation.
	Extract(response LLMResponse) (interface{}, error)
}

// JSONExtractor implements ResponseExtractor for JSON formatted responses.
type JSONExtractor struct {
	// Target is a pointer to the struct where JSON data should be unmarshaled.
	Target interface{}
}

// NewJSONExtractor creates a new JSONExtractor with the specified target struct.
func NewJSONExtractor(target interface{}) *JSONExtractor {
	return &JSONExtractor{Target: target}
}

// Extract implements ResponseExtractor.Extract for JSON data.
func (e *JSONExtractor) Extract(response LLMResponse) (interface{}, error) {
	// Try to find JSON content within markdown code blocks first
	jsonContent := extractFromCodeBlock(response.Text, "json")
	if jsonContent == "" {
		// If no code block found, use the entire response
		jsonContent = response.Text
	}

	if err := json.Unmarshal([]byte(jsonContent), e.Target); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return e.Target, nil
}

// XMLExtractor implements ResponseExtractor for XML formatted responses.
type XMLExtractor struct {
	// Target is a pointer to the struct where XML data should be unmarshaled.
	Target interface{}
}

// NewXMLExtractor creates a new XMLExtractor with the specified target struct.
func NewXMLExtractor(target interface{}) *XMLExtractor {
	return &XMLExtractor{Target: target}
}

// Extract implements ResponseExtractor.Extract for XML data.
func (e *XMLExtractor) Extract(response LLMResponse) (interface{}, error) {
	// Try to find XML content within markdown code blocks first
	xmlContent := extractFromCodeBlock(response.Text, "xml")
	if xmlContent == "" {
		// If no code block found, use the entire response
		xmlContent = response.Text
	}

	if err := xml.Unmarshal([]byte(xmlContent), e.Target); err != nil {
		return nil, fmt.Errorf("failed to unmarshal XML: %w", err)
	}
	return e.Target, nil
}

// TagExtractor implements ResponseExtractor for custom tag-based responses.
type TagExtractor struct {
	// Tag is the name of the tag to extract content from (e.g., "result", "code")
	Tag string
}

// NewTagExtractor creates a new TagExtractor for the specified tag.
func NewTagExtractor(tag string) *TagExtractor {
	return &TagExtractor{Tag: tag}
}

// Extract implements ResponseExtractor.Extract for tag-based content.
func (e *TagExtractor) Extract(response LLMResponse) (interface{}, error) {
	pattern := fmt.Sprintf(`<%s>(.*?)</%s>`, e.Tag, e.Tag)
	re, err := regexp.Compile("(?s)" + pattern) // (?s) makes dot match newlines
	if err != nil {
		return nil, fmt.Errorf("invalid tag pattern: %w", err)
	}

	matches := re.FindStringSubmatch(response.Text)
	if len(matches) < 2 {
		return nil, fmt.Errorf("tag <%s> not found in response", e.Tag)
	}

	return strings.TrimSpace(matches[1]), nil
}

// Helper function to extract content from markdown code blocks
func extractFromCodeBlock(text, language string) string {
	pattern := fmt.Sprintf("```%s\\n([\\s\\S]*?)```", language)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

// Table represents extracted tabular data from an LLM response.
// It includes headers, rows, and the format of the table (e.g., "markdown").
type Table struct {
	Headers []string   // Headers contains the column names of the table.
	Rows    [][]string // Rows contains the data rows of the table.
	Format  string     // Format specifies the format of the table (e.g., "markdown").
}

// NewTableExtractor creates a new TableExtractor for extracting tables in the specified format.
// The format parameter specifies the expected format of the table (e.g., "markdown").
func NewTableExtractor(format string) *TableExtractor {
	return &TableExtractor{
		Format: format,
	}
}

// TableExtractor implements the ResponseExtractor interface for extracting tabular data.
type TableExtractor struct {
	Format string // Format specifies the expected format of the table (e.g., "markdown").
}

// Extract processes the LLM response to extract a table in the specified format.
// It identifies the table headers and rows, and returns a Table struct containing the extracted data.
//
// Parameters:
//   - response: The LLMResponse containing the text to be processed.
//
// Returns:
//   - interface{}: A Table struct containing the extracted headers and rows.
//   - error: An error if the table is malformed or no valid table is found.
//
// Example Usage:
//
//	func main() {
//	    response := LLMResponse{
//	        Text: `
//	        | Name  | Age | Occupation |
//	        |-------|-----|------------|
//	        | Alice | 30  | Engineer   |
//	        | Bob   | 25  | Designer   |
//	        `,
//	    }
//
//	    extractor := NewTableExtractor("markdown")
//	    result, err := response.Extract(extractor)
//	    if err != nil {
//	        log.Fatalf("Failed to extract table: %v", err)
//	    }
//
//	    table := result.(*Table)
//	    fmt.Printf("Headers: %v\n", table.Headers)
//	    for _, row := range table.Rows {
//	        fmt.Printf("Row: %v\n", row)
//	    }
//	}
func (e *TableExtractor) Extract(response LLMResponse) (interface{}, error) {
	table := &Table{
		Format: e.Format,
	}

	lines := strings.Split(response.Text, "\n")
	var inTable bool
	var separator string
	var headerCount int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !inTable {
			inTable, headerCount = e.detectTableStart(line, table)
			if inTable {
				continue
			}
		}

		if inTable {
			if e.isSeparatorLine(line) {
				separator = line
				if !strings.Contains(line, "|") || !inTable {
					return nil, fmt.Errorf("malformed table: missing proper separator")
				}
				continue
			}

			if line != separator {
				if !strings.Contains(line, "|") {
					inTable = false
					continue
				}

				row := e.parseTableRow(line, headerCount)
				if row != nil {
					table.Rows = append(table.Rows, row)
				}
			}
		}
	}

	if len(table.Headers) == 0 || len(table.Rows) == 0 {
		return nil, fmt.Errorf("no valid table found in response")
	}

	return table, nil
}

// detectTableStart detects the start of a markdown table and extracts headers.
func (e *TableExtractor) detectTableStart(line string, table *Table) (bool, int) {
	if strings.Contains(line, "|") {
		cells := strings.Split(line, "|")
		if len(cells) > 2 {
			cells = cells[1 : len(cells)-1]
		}
		for _, cell := range cells {
			table.Headers = append(table.Headers, strings.TrimSpace(cell))
		}
		return true, len(table.Headers)
	}
	return false, 0
}

// isSeparatorLine checks if the line is a markdown table separator.
func (e *TableExtractor) isSeparatorLine(line string) bool {
	return strings.Contains(line, "|-")
}

// parseTableRow parses a row of the table and returns it as a slice of strings.
func (e *TableExtractor) parseTableRow(line string, headerCount int) []string {
	cells := strings.Split(line, "|")
	if len(cells) > 2 {
		cells = cells[1 : len(cells)-1]
	}
	row := make([]string, len(cells))
	for i, cell := range cells {
		row[i] = strings.TrimSpace(cell)
	}
	if len(row) == headerCount {
		return row
	}
	return nil
}
