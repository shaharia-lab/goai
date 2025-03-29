package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	MaxToolTurns               = 5
	GeminiRoleUser  GeminiRole = "user"
	GeminiRoleModel GeminiRole = "model"
	RoleFunction    GeminiRole = "function"
)

type GeminiRole = string

type GeminiProvider struct {
	client        *genai.Client
	modelName     string
	toolsProvider ToolsProvider
}

func NewGeminiProvider(ctx context.Context, apiKey, modelName string, toolsProvider ToolsProvider) (*GeminiProvider, error) {
	if apiKey == "" {
		return nil, errors.New("API key cannot be empty")
	}
	if modelName == "" {
		return nil, errors.New("model name cannot be empty")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &GeminiProvider{
		client:        client,
		modelName:     modelName,
		toolsProvider: toolsProvider,
	}, nil
}

func (p *GeminiProvider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

func (p *GeminiProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()
	model := p.client.GenerativeModel(p.modelName)

	currentHistory, err := mapLLMMessagesToGenaiContent(messages)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to map initial messages: %w", err)
	}

	genaiConfig, err := mapLLMConfigToGenaiConfig(config)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to map request config: %w", err)
	}
	model.GenerationConfig = *genaiConfig

	var genaiTools []*genai.Tool
	var activeTools map[string]mcp.Tool
	if config.toolsProvider != nil && len(config.AllowedTools) > 0 {
		fetchedTools, err := p.toolsProvider.ListTools(ctx, config.AllowedTools)
		if err != nil {
			return LLMResponse{}, fmt.Errorf("failed to get toolsProvider from provider: %w", err)
		}
		activeTools = make(map[string]mcp.Tool, len(fetchedTools))
		genaiTools = make([]*genai.Tool, 0, len(fetchedTools))
		for _, customTool := range fetchedTools {
			gTool, err := ConvertToGenaiTool(customTool)
			if err != nil {
				return LLMResponse{}, fmt.Errorf("failed to convert tool '%s': %w", customTool.Name, err)
			}
			genaiTools = append(genaiTools, gTool)
			activeTools[customTool.Name] = customTool
		}
		if len(genaiTools) > 0 {
			model.Tools = genaiTools
		}
	}

	var finalResponse *genai.GenerateContentResponse
	for turn := 0; turn < MaxToolTurns; turn++ {

		log.Printf("Gemini Request Turn %d: History Len=%d\n", turn+1, len(currentHistory))
		resp, err := model.GenerateContent(ctx, currentHistory...)
		if err != nil {
			return LLMResponse{}, fmt.Errorf("gemini API call failed (turn %d): %w", turn+1, err)
		}

		if len(resp.Candidates) == 0 {
			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				return LLMResponse{}, fmt.Errorf("request blocked by API (turn %d): %s", turn+1, resp.PromptFeedback.BlockReason.String())
			}
			return LLMResponse{}, fmt.Errorf("gemini API returned no candidates (turn %d)", turn+1)
		}
		candidate := resp.Candidates[0]

		funcCalls := findFunctionCalls(candidate)
		if len(funcCalls) == 0 {
			if candidate.FinishReason != genai.FinishReasonStop && candidate.FinishReason != genai.FinishReasonMaxTokens {
				log.Printf("Warning: Gemini response finished with reason: %s", candidate.FinishReason)
				if candidate.FinishReason == genai.FinishReasonSafety {
					return LLMResponse{}, fmt.Errorf("gemini response stopped due to safety settings")
				}
			}
			finalResponse = resp
			log.Printf("Gemini Response Turn %d: Final text response received.", turn+1)
			break
		}

		log.Printf("Gemini Response Turn %d: Received %d function call(s).", turn+1, len(funcCalls))
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			// Add the LLM's turn which contains the function call request(s)
			currentHistory = append(currentHistory, candidate.Content)
		} else {
			log.Printf("Warning: Candidate content is nil or empty despite function calls being found.")
		}

		functionResponses := make([]genai.Part, 0, len(funcCalls))
		for _, fc := range funcCalls {
			log.Printf("Executing tool: %s", fc.Name)
			toolToRun, exists := activeTools[fc.Name]
			if !exists || toolToRun.Handler == nil {
				log.Printf("Error: Unknown tool '%s' requested or handler missing.", fc.Name)
				toolResult := mcp.CallToolResult{
					IsError: true,
					Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Tool '%s' not found or not runnable.", fc.Name)}},
				}
				resultMap, err := callResultToMap(toolResult)
				if err != nil {
					log.Printf("Error marshaling error result for tool '%s': %v", fc.Name, err)
					continue
				}
				functionResponses = append(functionResponses, genai.FunctionResponse{
					Name:     fc.Name,
					Response: resultMap,
				})
				continue
			}

			argsJSON, err := json.Marshal(fc.Args)
			if err != nil {
				log.Printf("Error marshaling arguments for tool '%s': %v", fc.Name, err)
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Invalid arguments format for tool '%s': %v", fc.Name, err)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponses = append(functionResponses, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				continue
			}
			callParams := mcp.CallToolParams{
				Name:      fc.Name,
				Arguments: json.RawMessage(argsJSON),
			}

			toolResult, err := toolToRun.Handler(ctx, callParams)
			if err != nil {
				log.Printf("Error executing tool handler '%s': %v", fc.Name, err)
				if !toolResult.IsError {
					toolResult.IsError = true
					toolResult.Content = append(toolResult.Content, mcp.ToolResultContent{Type: "text", Text: fmt.Sprintf("Handler error: %v", err)})
				}
			}

			resultMap, err := callResultToMap(toolResult)
			if err != nil {
				log.Printf("Error converting tool result to map for '%s': %v", fc.Name, err)
				resultMap = map[string]any{"error": fmt.Sprintf("Failed to format tool result: %v", err)}
			}

			functionResponses = append(functionResponses, genai.FunctionResponse{
				Name:     fc.Name,
				Response: resultMap,
			})
		}

		if len(functionResponses) > 0 {
			// Add the consolidated function results as a single turn in history
			currentHistory = append(currentHistory, &genai.Content{
				Role:  RoleFunction,
				Parts: functionResponses,
			})
		}

		if turn == MaxToolTurns-1 {
			log.Printf("Warning: Reached maximum tool turns (%d). Returning last response.", MaxToolTurns)
			finalResponse = resp
			break
		}
	}

	if finalResponse == nil {
		return LLMResponse{}, errors.New("no final response generated after tool loop")
	}

	llmResponse := LLMResponse{
		CompletionTime: time.Since(startTime).Seconds(),
	}

	if len(finalResponse.Candidates) > 0 {
		llmResponse.Text = extractTextFromParts(finalResponse.Candidates[0].Content.Parts)
	}

	if finalResponse.UsageMetadata != nil {
		llmResponse.TotalInputToken = int(finalResponse.UsageMetadata.PromptTokenCount)
		llmResponse.TotalOutputToken = int(finalResponse.UsageMetadata.CandidatesTokenCount)
	} else {
		log.Println("Warning: UsageMetadata missing in the final Gemini response.")
	}

	return llmResponse, nil
}

func (p *GeminiProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	model := p.client.GenerativeModel(p.modelName)

	currentHistory, err := mapLLMMessagesToGenaiContent(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to map initial messages: %w", err)
	}
	genaiConfig, err := mapLLMConfigToGenaiConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to map request config: %w", err)
	}
	model.GenerationConfig = *genaiConfig

	var genaiTools []*genai.Tool
	var activeTools map[string]mcp.Tool
	if config.toolsProvider != nil && len(config.AllowedTools) > 0 {
		fetchedTools, err := p.toolsProvider.ListTools(ctx, config.AllowedTools)
		if err != nil {
			return nil, fmt.Errorf("failed to get toolsProvider: %w", err)
		}
		activeTools = make(map[string]mcp.Tool, len(fetchedTools))
		genaiTools = make([]*genai.Tool, 0, len(fetchedTools))
		for _, customTool := range fetchedTools {
			gTool, err := ConvertToGenaiTool(customTool)
			if err != nil {
				return nil, fmt.Errorf("failed to convert tool '%s': %w", customTool.Name, err)
			}
			genaiTools = append(genaiTools, gTool)
			activeTools[customTool.Name] = customTool
		}
		if len(genaiTools) > 0 {
			model.Tools = genaiTools
		}
	}

	finalHistoryForStreaming := currentHistory
	for turn := 0; turn < MaxToolTurns; turn++ {
		log.Printf("Streaming Pre-flight Turn %d: History Len=%d\n", turn+1, len(finalHistoryForStreaming))
		resp, err := model.GenerateContent(ctx, finalHistoryForStreaming...)
		if err != nil {
			return nil, fmt.Errorf("gemini pre-flight API call failed (turn %d): %w", turn+1, err)
		}
		if len(resp.Candidates) == 0 {
			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				return nil, fmt.Errorf("request blocked by API during pre-flight (turn %d): %s", turn+1, resp.PromptFeedback.BlockReason.String())
			}
			return nil, fmt.Errorf("gemini pre-flight API returned no candidates (turn %d)", turn+1)
		}
		candidate := resp.Candidates[0]

		funcCalls := findFunctionCalls(candidate)
		if len(funcCalls) == 0 {
			log.Printf("Streaming Pre-flight Turn %d: Tool resolution complete.", turn+1)
			finalHistoryForStreaming = currentHistory
			break
		}

		log.Printf("Streaming Pre-flight Turn %d: Received %d function call(s).", turn+1, len(funcCalls))
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			currentHistory = append(currentHistory, candidate.Content)
		}

		functionResponses := make([]genai.Part, 0, len(funcCalls))
		for _, fc := range funcCalls {
			toolToRun, exists := activeTools[fc.Name]
			if !exists || toolToRun.Handler == nil {
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Tool '%s' not found.", fc.Name)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponses = append(functionResponses, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				continue
			}
			argsJSON, err := json.Marshal(fc.Args)
			if err != nil {
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Invalid args for '%s': %v", fc.Name, err)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponses = append(functionResponses, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				continue
			}
			callParams := mcp.CallToolParams{Name: fc.Name, Arguments: json.RawMessage(argsJSON)}
			toolResult, err := toolToRun.Handler(ctx, callParams)
			if err != nil {
				if !toolResult.IsError {
					toolResult.IsError = true
					toolResult.Content = append(toolResult.Content, mcp.ToolResultContent{Type: "text", Text: fmt.Sprintf("Handler error: %v", err)})
				}
			}
			resultMap, err := callResultToMap(toolResult)
			if err != nil {
				log.Printf("Error converting tool result to map for '%s' in streaming pre-flight: %v", fc.Name, err)
				resultMap = map[string]any{"error": fmt.Sprintf("Failed to format tool result: %v", err)}
			}
			functionResponses = append(functionResponses, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
		}
		if len(functionResponses) > 0 {
			currentHistory = append(currentHistory, &genai.Content{
				Role:  RoleFunction,
				Parts: functionResponses,
			})
		}

		finalHistoryForStreaming = currentHistory

		if turn == MaxToolTurns-1 {
			log.Printf("Warning: Reached maximum tool turns (%d) during streaming pre-flight.", MaxToolTurns)
			break
		}
	}

	streamChan := make(chan StreamingLLMResponse, 1)
	log.Printf("Starting final streaming call with history Len=%d", len(finalHistoryForStreaming))
	iter := model.GenerateContentStream(ctx, finalHistoryForStreaming...)

	go func() {
		defer close(streamChan)
		var cumulativeOutputTokens int = 0

		for {
			resp, err := iter.Next()
			if errors.Is(err, iterator.Done) {
				return
			}
			if err != nil {
				log.Printf("Error during Gemini stream: %v", err)
				streamChan <- StreamingLLMResponse{Error: fmt.Errorf("gemini stream error: %w", err), Done: true}
				return
			}

			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				streamChan <- StreamingLLMResponse{Error: fmt.Errorf("prompt blocked during stream: %s", resp.PromptFeedback.BlockReason.String()), Done: true}
				return
			}
			if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason == genai.FinishReasonSafety {
				streamChan <- StreamingLLMResponse{Error: fmt.Errorf("response stopped during stream due to safety: %s", resp.Candidates[0].FinishReason.String()), Done: true}
				return
			}

			chunkText := ""
			currentOutputTokens := 0
			if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
				chunkText = extractTextFromParts(resp.Candidates[0].Content.Parts)
			}
			if resp.UsageMetadata != nil {
				currentOutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
				cumulativeOutputTokens = currentOutputTokens
			}

			streamChan <- StreamingLLMResponse{
				Text:       chunkText,
				Done:       false,
				Error:      nil,
				TokenCount: cumulativeOutputTokens,
			}
		}
	}()

	return streamChan, nil
}

func mapLLMMessagesToGenaiContent(messages []LLMMessage) ([]*genai.Content, error) {
	genaiMessages := make([]*genai.Content, 0, len(messages))
	for _, msg := range messages {
		var role GeminiRole
		switch msg.Role {
		case UserRole:
			role = GeminiRoleUser
		case AssistantRole:
			role = GeminiRoleModel
		case SystemRole:
			log.Println("Mapping LLMRoleSystem to Gemini GeminiRoleUser")
			role = GeminiRoleUser
		default:
			return nil, fmt.Errorf("unsupported LLMMessageRole for initial mapping: %s", msg.Role)
		}

		if len(genaiMessages) > 0 && genaiMessages[len(genaiMessages)-1].Role == role {
			lastContent := genaiMessages[len(genaiMessages)-1]
			lastContent.Parts = append(lastContent.Parts, genai.Text(msg.Text))
			continue
		}

		genaiMessages = append(genaiMessages, &genai.Content{
			Role:  role,
			Parts: []genai.Part{genai.Text(msg.Text)},
		})
	}
	if len(genaiMessages) > 0 && genaiMessages[0].Role == GeminiRoleModel {
		return nil, errors.New("conversation history cannot start with an assistant/model message")
	}

	return genaiMessages, nil
}

func mapLLMConfigToGenaiConfig(config LLMRequestConfig) (*genai.GenerationConfig, error) {
	genaiConfig := &genai.GenerationConfig{}
	if config.MaxToken > 0 {
		if config.MaxToken > int64(^uint32(0)>>1) {
			return nil, fmt.Errorf("MaxToken %d exceeds int32 limit", config.MaxToken)
		}
		maxTokens := int32(config.MaxToken)
		genaiConfig.MaxOutputTokens = &maxTokens
	}
	if config.Temperature >= 0 {
		temp := float32(config.Temperature)
		genaiConfig.Temperature = &temp
	}
	if config.TopP > 0 {
		topP := float32(config.TopP)
		genaiConfig.TopP = &topP
	}
	if config.TopK > 0 {
		if config.TopK > int64(^uint32(0)>>1) {
			return nil, fmt.Errorf("TopK %d exceeds int32 limit", config.TopK)
		}
		topK := int32(config.TopK)
		genaiConfig.TopK = &topK
	}
	return genaiConfig, nil
}

func extractTextFromParts(parts []genai.Part) string {
	var textContent string
	for _, part := range parts {
		if text, ok := part.(genai.Text); ok {
			textContent += string(text)
		}
	}
	return textContent
}

func findFunctionCalls(candidate *genai.Candidate) []*genai.FunctionCall {
	calls := make([]*genai.FunctionCall, 0)
	if candidate == nil || candidate.Content == nil {
		return calls
	}
	for _, part := range candidate.Content.Parts {
		if fc, ok := part.(*genai.FunctionCall); ok {
			calls = append(calls, fc)
		}
	}
	return calls
}

func ConvertToGenaiTool(customTool mcp.Tool) (*genai.Tool, error) {
	var parametersSchema *genai.Schema
	if len(customTool.InputSchema) > 0 && string(customTool.InputSchema) != "null" {
		parametersSchema = &genai.Schema{}
		err := json.Unmarshal(customTool.InputSchema, parametersSchema)
		if err != nil {
			return nil, fmt.Errorf("unmarshal schema for tool '%s': %w", customTool.Name, err)
		}
	}
	funcDecl := &genai.FunctionDeclaration{
		Name:        customTool.Name,
		Description: customTool.Description,
		Parameters:  parametersSchema,
	}
	return &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{funcDecl}}, nil
}

func callResultToMap(result mcp.CallToolResult) (map[string]any, error) {
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CallToolResult: %w", err)
	}
	var resultMap map[string]any
	err = json.Unmarshal(jsonBytes, &resultMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal CallToolResult into map: %w", err)
	}
	return resultMap, nil
}
