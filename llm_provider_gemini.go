package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/shaharia-lab/goai/mcp"
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
	client    *genai.Client
	modelName string
}

func NewGeminiProvider(ctx context.Context, apiKey, modelName string) (*GeminiProvider, error) {
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
		client:    client,
		modelName: modelName,
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

	// --- Apply configurations to the model *before* starting chat ---
	genaiConfig, err := mapLLMConfigToGenaiConfig(config)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to map request config: %w", err)
	}
	model.GenerationConfig = *genaiConfig // Apply GenerationConfig here

	var activeTools map[string]mcp.Tool
	var genaiTools []*genai.Tool

	if len(config.AllowedTools) > 0 {
		if config.toolsProvider == nil {
			return LLMResponse{}, errors.New("request config allows tools but provides no toolsProvider")
		}
		fetchedTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
		if err != nil {
			return LLMResponse{}, fmt.Errorf("failed to get tools from config.toolsProvider: %w", err)
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
	}
	model.Tools = genaiTools

	// --- Start Chat Session *after* configuring the model ---
	cs := model.StartChat()

	initialHistory, err := mapLLMMessagesToGenaiContent(messages)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to map initial messages: %w", err)
	}

	// Set history and initial parts for the first SendMessage call
	if len(initialHistory) > 1 {
		cs.History = initialHistory[:len(initialHistory)-1]
	}
	var initialParts []genai.Part
	if len(initialHistory) > 0 {
		initialParts = initialHistory[len(initialHistory)-1].Parts
	}
	if len(initialParts) == 0 {
		return LLMResponse{}, errors.New("cannot start LLM conversation with empty initial message parts")
	}

	var finalResponse *genai.GenerateContentResponse
	sendParts := initialParts
	var loopError error

	// --- Tool loop starts ---
	for turn := 0; turn < MaxToolTurns; turn++ {
		log.Printf("Gemini Request Turn %d: Sending %d parts\n", turn+1, len(sendParts))
		resp, err := cs.SendMessage(ctx, sendParts...) // SendMessage uses the session's model config
		if err != nil {
			loopError = fmt.Errorf("gemini SendMessage failed (turn %d): %w", turn+1, err)
			break
		}

		if len(resp.Candidates) == 0 {
			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				loopError = fmt.Errorf("request blocked by API (turn %d): %s", turn+1, resp.PromptFeedback.BlockReason.String())
			} else {
				loopError = fmt.Errorf("gemini API returned no candidates (turn %d)", turn+1)
			}
			break
		}
		candidate := resp.Candidates[0]

		if candidate.Content != nil {
			cs.History = append(cs.History, candidate.Content)
		} else {
			log.Printf("Warning: Candidate content is nil in API response (turn %d).", turn+1)
		}

		funcCalls := findFunctionCalls(candidate)
		if len(funcCalls) == 0 {
			if candidate.FinishReason != genai.FinishReasonStop && candidate.FinishReason != genai.FinishReasonMaxTokens {
				log.Printf("Warning: Gemini response finished with non-standard reason: %s", candidate.FinishReason)
				if candidate.FinishReason == genai.FinishReasonSafety {
					loopError = fmt.Errorf("gemini response stopped due to safety settings")
					break
				}
			}
			//log.Printf("%+v", candidate.FinishReason)
			log.Printf("%+v", candidate.Content.Parts[0])
			finalResponse = resp
			log.Printf("Gemini Response Turn %d: Received non-function-call response. Assuming final.", turn+1)
			break
		}

		log.Printf("Gemini Response Turn %d: Received %d function call(s).", turn+1, len(funcCalls))
		functionResponseParts := make([]genai.Part, 0, len(funcCalls))
		allToolExecutionsSuccessful := true

		for _, fc := range funcCalls {
			log.Printf("Executing tool: %s", fc.Name)
			toolToRun, exists := activeTools[fc.Name]
			if !exists || toolToRun.Handler == nil {
				log.Printf("Error: Unknown tool '%s' requested or handler missing.", fc.Name)
				toolResult := mcp.CallToolResult{
					IsError: true,
					Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Tool '%s' not found or not runnable.", fc.Name)}},
				}
				resultMap, mapErr := callResultToMap(toolResult)
				if mapErr != nil {
					log.Printf("Error marshaling error result for tool '%s': %v", fc.Name, mapErr)
					allToolExecutionsSuccessful = false
					continue
				}
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				allToolExecutionsSuccessful = false
				continue
			}

			argsJSON, marshalErr := json.Marshal(fc.Args)
			if marshalErr != nil {
				log.Printf("Error marshaling arguments for tool '%s': %v", fc.Name, marshalErr)
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Invalid arguments format for tool '%s': %v", fc.Name, marshalErr)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				allToolExecutionsSuccessful = false
				continue
			}
			callParams := mcp.CallToolParams{Name: fc.Name, Arguments: json.RawMessage(argsJSON)}

			toolResult, handlerErr := toolToRun.Handler(ctx, callParams)
			if handlerErr != nil {
				log.Printf("Error executing tool handler '%s': %v", fc.Name, handlerErr)
				if !toolResult.IsError {
					toolResult.IsError = true
					toolResult.Content = append(toolResult.Content, mcp.ToolResultContent{Type: "text", Text: fmt.Sprintf("Handler error: %v", handlerErr)})
				}
				allToolExecutionsSuccessful = false
			}

			resultMap, mapErr := callResultToMap(toolResult)
			if mapErr != nil {
				log.Printf("Error converting tool result to map for '%s': %v", fc.Name, mapErr)
				resultMap = map[string]any{"error": fmt.Sprintf("Failed to format tool result: %v", mapErr)}
				allToolExecutionsSuccessful = false
			}

			functionResponseParts = append(functionResponseParts, genai.FunctionResponse{
				Name:     fc.Name,
				Response: resultMap,
			})
		}

		if len(functionResponseParts) > 0 {
			cs.History = append(cs.History, &genai.Content{
				Role:  RoleFunction,
				Parts: functionResponseParts,
			})
			if !allToolExecutionsSuccessful {
				log.Printf("Warning: One or more tool executions failed or had errors during turn %d.", turn+1)
			}
			// Prepare for the next SendMessage call - send a dummy non-empty part
			sendParts = []genai.Part{genai.Text("")} // <-- FIX: Send empty text part instead of empty slice
		} else {
			log.Printf("Error: No function response parts could be generated for turn %d. Aborting loop.", turn+1)
			loopError = fmt.Errorf("failed to generate any function responses for turn %d", turn+1)
			break
		}

		if turn == MaxToolTurns-1 {
			log.Printf("Warning: Reached maximum tool turns (%d).", MaxToolTurns)
			loopError = fmt.Errorf("reached maximum tool turns (%d) without final text response", MaxToolTurns)
		}
	} // End tool calling loop

	// --- After Loop Processing ---
	if loopError != nil {
		return LLMResponse{}, loopError
	}
	if finalResponse == nil {
		return LLMResponse{}, errors.New("tool loop finished, but no final response was captured")
	}
	if finalResponse != nil && len(finalResponse.Candidates) > 0 {
		finalFuncCalls := findFunctionCalls(finalResponse.Candidates[0])
		if len(finalFuncCalls) > 0 {
			log.Printf("Error: Loop finished, but the captured 'finalResponse' still contained %d function call(s).", len(finalFuncCalls))
			return LLMResponse{}, fmt.Errorf("tool loop finished, but final response required further tool calls (max turns likely reached)")
		}
	} else if finalResponse == nil {
		return LLMResponse{}, errors.New("internal error: loop finished with nil finalResponse despite no loopError")
	}

	// --- Construct final LLMResponse ---
	llmResponse := LLMResponse{
		CompletionTime: time.Since(startTime).Seconds(),
	}
	if len(finalResponse.Candidates) > 0 {
		llmResponse.Text = extractTextFromParts(finalResponse.Candidates[0].Content.Parts)
	} else {
		log.Println("Warning: Final response captured but has no candidates.")
	}
	if finalResponse.UsageMetadata != nil {
		llmResponse.TotalInputToken = int(finalResponse.UsageMetadata.PromptTokenCount)
		llmResponse.TotalOutputToken = int(finalResponse.UsageMetadata.CandidatesTokenCount)
	} else {
		log.Println("Warning: UsageMetadata may not be available on the final SendMessage response.")
	}
	return llmResponse, nil
}

func (p *GeminiProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	model := p.client.GenerativeModel(p.modelName)

	cs := model.StartChat()

	initialHistory, err := mapLLMMessagesToGenaiContent(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to map initial messages: %w", err)
	}
	if len(initialHistory) > 1 {
		cs.History = initialHistory[:len(initialHistory)-1]
	}
	var initialParts []genai.Part
	if len(initialHistory) > 0 {
		initialParts = initialHistory[len(initialHistory)-1].Parts
	}
	if len(initialParts) == 0 {
		return nil, errors.New("cannot start LLM streaming conversation with empty initial message parts")
	}

	genaiConfig, err := mapLLMConfigToGenaiConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to map request config: %w", err)
	}
	model.GenerationConfig = *genaiConfig

	var activeTools map[string]mcp.Tool
	if len(config.AllowedTools) > 0 {
		fetchedTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
		if err != nil {
			return nil, fmt.Errorf("failed to get toolsProvider: %w", err)
		}
		activeTools = make(map[string]mcp.Tool, len(fetchedTools))
		genaiTools := make([]*genai.Tool, 0, len(fetchedTools))
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

	sendParts := initialParts
	for turn := 0; turn < MaxToolTurns; turn++ {
		log.Printf("Streaming Pre-flight Turn %d: Sending %d parts\n", turn+1, len(sendParts))
		resp, err := cs.SendMessage(ctx, sendParts...)
		if err != nil {
			return nil, fmt.Errorf("gemini pre-flight SendMessage failed (turn %d): %w", turn+1, err)
		}
		if len(resp.Candidates) == 0 {
			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				return nil, fmt.Errorf("request blocked by API during pre-flight (turn %d): %s", turn+1, resp.PromptFeedback.BlockReason.String())
			}
			return nil, fmt.Errorf("gemini pre-flight API returned no candidates (turn %d)", turn+1)
		}
		candidate := resp.Candidates[0]

		if candidate.Content != nil {
			cs.History = append(cs.History, candidate.Content)
		} else {
			log.Printf("Warning: Pre-flight candidate content is nil (turn %d).", turn+1)
		}

		funcCalls := findFunctionCalls(candidate)
		if len(funcCalls) == 0 {
			log.Printf("Streaming Pre-flight Turn %d: Tool resolution complete.", turn+1)

			break
		}

		log.Printf("Streaming Pre-flight Turn %d: Received %d function call(s).", turn+1, len(funcCalls))
		functionResponseParts := make([]genai.Part, 0, len(funcCalls))

		for _, fc := range funcCalls {
			toolToRun, exists := activeTools[fc.Name]
			if !exists || toolToRun.Handler == nil {
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Tool '%s' not found.", fc.Name)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				continue
			}
			argsJSON, err := json.Marshal(fc.Args)
			if err != nil {
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Invalid args for '%s': %v", fc.Name, err)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
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
			functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
		}

		if len(functionResponseParts) > 0 {
			cs.History = append(cs.History, &genai.Content{
				Role:  RoleFunction,
				Parts: functionResponseParts,
			})
			sendParts = []genai.Part{genai.Text("")}
		} else {
			log.Printf("Error: No function response parts could be generated during pre-flight turn %d. Aborting stream setup.", turn+1)
			break
		}

		if turn == MaxToolTurns-1 {
			log.Printf("Warning: Reached maximum tool turns (%d) during streaming pre-flight.", MaxToolTurns)
			break
		}
	}

	var partsForStream []genai.Part
	if len(cs.History) > 0 {

		partsForStream = cs.History[len(cs.History)-1].Parts
		log.Printf("Attempting to start stream based on the %d parts from the *last* history item (Role: %s). Context might be incomplete.", len(partsForStream), cs.History[len(cs.History)-1].Role)

		lastUserParts := findLastUserParts(cs.History)
		if len(lastUserParts) > 0 {
			log.Printf("Attempting to start stream based on the parts from the *last user message* in history.")
			partsForStream = lastUserParts
		} else if len(partsForStream) == 0 {
			return nil, fmt.Errorf("cannot determine appropriate parts to initiate streaming after tool loop")
		}

	} else {

		return nil, fmt.Errorf("history is empty, cannot initiate stream")
	}

	streamChan := make(chan StreamingLLMResponse, 1)
	log.Printf("Starting final streaming call with %d parts derived from history.", len(partsForStream))

	iter := model.GenerateContentStream(ctx, partsForStream...)

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

			if _, ok := lastContent.Parts[len(lastContent.Parts)-1].(genai.Text); ok {
				lastContent.Parts = append(lastContent.Parts, genai.Text(msg.Text))
				continue
			}
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
		log.Printf("Checking part: %+v (Type: %T)", part, part) // Keep logging type for confirmation

		// --- CHANGE THE TYPE ASSERTION HERE ---
		// Try asserting the value type first
		if fcValue, ok := part.(genai.FunctionCall); ok {
			log.Printf("Found function call VALUE: %+v", fcValue)
			// IMPORTANT: Need a pointer for the slice. Take the address.
			fcPointer := &fcValue
			calls = append(calls, fcPointer)
			// Optionally, also check for pointer just in case (though less likely based on logs)
		} else if fcPointer, ok := part.(*genai.FunctionCall); ok {
			log.Printf("Found function call POINTER: %+v", fcPointer)
			calls = append(calls, fcPointer)
		}
		// --- END CHANGE ---

	}
	log.Printf("findFunctionCalls returning %d calls", len(calls)) // Add log to see result
	return calls
}

func findLastUserParts(history []*genai.Content) []genai.Part {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == GeminiRoleUser {
			return history[i].Parts
		}
	}
	return nil
}

type IntermediateJSONSchema struct {
	Type        string                             `json:"type"`
	Description string                             `json:"description,omitempty"`
	Properties  map[string]*IntermediateJSONSchema `json:"properties,omitempty"`
	Required    []string                           `json:"required,omitempty"`
	Items       *IntermediateJSONSchema            `json:"items,omitempty"`
	Enum        []string                           `json:"enum,omitempty"`
}

func convertJSONSchemaToGenaiSchema(js *IntermediateJSONSchema) (*genai.Schema, error) {
	if js == nil {
		return nil, nil
	}

	gs := &genai.Schema{
		Description: js.Description,
		Required:    js.Required,
		Enum:        js.Enum,
	}

	switch js.Type {
	case "string":
		gs.Type = genai.TypeString
	case "number":
		gs.Type = genai.TypeNumber
	case "integer":
		gs.Type = genai.TypeInteger
	case "boolean":
		gs.Type = genai.TypeBoolean
	case "array":
		gs.Type = genai.TypeArray

		var err error
		gs.Items, err = convertJSONSchemaToGenaiSchema(js.Items)
		if err != nil {
			return nil, fmt.Errorf("failed converting array items: %w", err)
		}

		if gs.Items == nil && js.Items != nil {

			return nil, fmt.Errorf("array type specified but item schema conversion failed or resulted in nil")
		} else if js.Items == nil {

		}

	case "object":
		gs.Type = genai.TypeObject

		if len(js.Properties) > 0 {
			gs.Properties = make(map[string]*genai.Schema)
			for k, v := range js.Properties {
				var err error
				gs.Properties[k], err = convertJSONSchemaToGenaiSchema(v)
				if err != nil {
					return nil, fmt.Errorf("failed converting object property '%s': %w", k, err)
				}
			}
		}
	case "":

		gs.Type = genai.TypeUnspecified
	default:

		return nil, fmt.Errorf("unsupported JSON schema type string: '%s'", js.Type)
	}

	return gs, nil
}

func ConvertToGenaiTool(customTool mcp.Tool) (*genai.Tool, error) {
	var parametersSchema *genai.Schema

	if len(customTool.InputSchema) > 0 && string(customTool.InputSchema) != "null" {

		var intermediateSchema IntermediateJSONSchema
		err := json.Unmarshal(customTool.InputSchema, &intermediateSchema)
		if err != nil {

			return nil, fmt.Errorf("unmarshal raw JSON schema for tool '%s': %w", customTool.Name, err)
		}

		parametersSchema, err = convertJSONSchemaToGenaiSchema(&intermediateSchema)
		if err != nil {

			return nil, fmt.Errorf("convert JSON schema to genai schema for tool '%s': %w", customTool.Name, err)
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
