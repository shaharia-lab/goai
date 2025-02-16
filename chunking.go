// Package goai provides utilities for text chunking and AI-powered text processing.
// It offers flexible interfaces for breaking down large texts into manageable chunks
// while preserving context and meaning.
package goai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ChunkingProvider defines the interface for services that can split text into chunks.
type ChunkingProvider interface {
	// Chunk splits the input text into coherent segments using the provided context.
	// Returns the array of text chunks or an error if the chunking fails.
	Chunk(ctx context.Context, text string) ([]string, error)
}

// ChunkingByLLMProvider implements ChunkingProvider using a language model to
// intelligently split text while preserving context and meaning.
type ChunkingByLLMProvider struct {
	llm *LLMRequest
}

// NewChunkingByLLMProvider creates a new ChunkingByLLMProvider with the specified LLM request client.
//
// Example usage:
//
//	llm := NewLLMRequest(config, provider)
//	chunker := NewChunkingByLLMProvider(llm)
//	chunks, err := chunker.Chunk(ctx, longText)
func NewChunkingByLLMProvider(llm *LLMRequest) *ChunkingByLLMProvider {
	return &ChunkingByLLMProvider{
		llm: llm,
	}
}

const chunkingPromptTemplate = `Your task is to divide the input text into coherent chunks for generating embedding vectors. The chunks should:
- Preserve complete sentences and logical units where possible
- Have natural breakpoints (e.g., paragraphs, sections)
- Be roughly similar in length
- Not exceed 512 tokens per chunk
- Maintain context and readability

Input text:
{{.Text}}

Instructions:
Return ONLY a JSON array of chunk positions in the following format, with no other explanatory text:
[[start_position, end_position], [start_position, end_position], ...]

Example format:
[[0, 500], [501, 1000], [1001, 1500]]`

// Offset represents a text chunk's starting and ending positions in the original text.
type Offset struct {
	Start int
	End   int
}

// parseOffsets converts the LLM response string into a slice of Offset structs.
// Returns an error if the response format is invalid or cannot be parsed.
func parseOffsets(response string) ([]Offset, error) {
	response = strings.TrimSpace(response)

	if !strings.HasPrefix(response, "[[") && !strings.HasPrefix(response, "[]") {
		return nil, fmt.Errorf("invalid response format: %s", response)
	}

	var rawOffsets [][]int
	err := json.Unmarshal([]byte(response), &rawOffsets)
	if err != nil {
		return nil, fmt.Errorf("failed to parse offsets: %w", err)
	}

	if len(rawOffsets) == 0 {
		return []Offset{}, nil
	}

	offsets := make([]Offset, len(rawOffsets))
	for i, raw := range rawOffsets {
		if len(raw) != 2 {
			return nil, fmt.Errorf("invalid offset pair at index %d", i)
		}
		offsets[i] = Offset{
			Start: raw[0],
			End:   raw[1],
		}
	}

	return offsets, nil
}

// Chunk splits the input text into coherent segments using the language model.
// It preserves sentence boundaries and semantic units while maintaining consistent chunk sizes.
// Returns an error if the chunking process fails at any stage.
//
// Example usage:
//
//	chunker := NewChunkingByLLMProvider(llm)
//	chunks, err := chunker.Chunk(ctx, "Long text to be split...")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for i, chunk := range chunks {
//	    fmt.Printf("Chunk %d: %s\n", i, chunk)
//	}
func (p *ChunkingByLLMProvider) Chunk(ctx context.Context, text string) ([]string, error) {
	if len(text) == 0 {
		return []string{}, nil
	}

	template := &LLMPromptTemplate{
		Template: chunkingPromptTemplate,
		Data: map[string]interface{}{
			"Text": text,
		},
	}

	promptText, err := template.Parse()
	if err != nil {
		return nil, fmt.Errorf("failed to parse prompt template: %w", err)
	}

	messages := []LLMMessage{
		{Role: UserRole, Text: promptText},
	}

	llmResponse, err := p.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chunks: %w", err)
	}

	offsets, err := parseOffsets(llmResponse.Text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chunks: %w", err)
	}

	chunks := make([]string, len(offsets))
	for i, offset := range offsets {
		if offset.Start >= len(text) || offset.End > len(text) || offset.Start > offset.End {
			return nil, fmt.Errorf("invalid offset range [%d, %d] for text length %d",
				offset.Start, offset.End, len(text))
		}
		chunks[i] = text[offset.Start:offset.End]
	}

	return chunks, nil
}
