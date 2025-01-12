package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ChunkingProvider interface {
	Chunk(ctx context.Context, text string) ([]string, error)
}

type ChunkingByLLMProvider struct {
	llm *LLMRequest
}

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

type Offset struct {
	Start int
	End   int
}

func parseOffsets(response string) ([]Offset, error) {
	// Clean up the response to ensure we have valid JSON
	response = strings.TrimSpace(response)

	// Convert string array format to valid JSON
	if !strings.HasPrefix(response, "[[") && !strings.HasPrefix(response, "[]") {
		return nil, fmt.Errorf("invalid response format: %s", response)
	}

	// Parse the array
	var rawOffsets [][]int
	err := json.Unmarshal([]byte(response), &rawOffsets)
	if err != nil {
		return nil, fmt.Errorf("failed to parse offsets: %w", err)
	}

	// Handle empty array case
	if len(rawOffsets) == 0 {
		return []Offset{}, nil
	}

	// Convert to Offset structs
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

func (p *ChunkingByLLMProvider) Chunk(ctx context.Context, text string) ([]string, error) {
	// Handle empty input text
	if len(text) == 0 {
		return []string{}, nil
	}

	// Create the prompt template
	template := &LLMPromptTemplate{
		Template: chunkingPromptTemplate,
		Data: map[string]interface{}{
			"Text": text,
		},
	}

	// Parse the template
	promptText, err := template.Parse()
	if err != nil {
		return nil, fmt.Errorf("failed to parse prompt template: %w", err)
	}

	messages := []LLMMessage{
		{Role: UserRole, Text: promptText},
	}

	// Get response from LLM
	llmResponse, err := p.llm.Generate(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chunks: %w", err)
	}

	// Parse the response into offsets
	offsets, err := parseOffsets(llmResponse.Text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chunks: %w", err)
	}

	// Generate the chunks from the text using the offsets
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
