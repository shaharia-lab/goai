// Package ai provides a flexible interface for interacting with various Language Learning Models (LLMs).
package ai

import (
	"bytes"
	"text/template"
)

// LLMPromptTemplate provides functionality for creating dynamic prompts using templates.
// It supports Go's text/template syntax for variable substitution and logic.
type LLMPromptTemplate struct {
	// Template is the template string using Go's text/template syntax
	Template string

	// Data contains the values to be substituted in the template
	Data map[string]interface{}
}

// Parse processes the template with the provided data and returns the final prompt string.
// Returns an error if template parsing or execution fails.
func (p *LLMPromptTemplate) Parse() (string, error) {
	tmpl, err := template.New("prompt").Parse(p.Template)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p.Data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
