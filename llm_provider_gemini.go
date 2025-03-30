package goai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/shaharia-lab/goai/mcp"
)

const (
	MaxToolTurns               = 5
	GeminiRoleUser  GeminiRole = "user"
	GeminiRoleModel GeminiRole = "model"
	RoleFunction    GeminiRole = "function"
)

type GeminiRole = string

type GeminiProvider struct {
	service GeminiModelService // Use the interface
	log     observability.Logger
}

// NewGeminiProvider to accept the service interface
func NewGeminiProvider(service GeminiModelService, log observability.Logger) (*GeminiProvider, error) {
	if service == nil {
		return nil, errors.New("GeminiModelService cannot be nil")
	}
	// API key/model checks are now responsibility of service creation
	return &GeminiProvider{
		service: service,
		log:     log,
	}, nil
}

func (p *GeminiProvider) Close() error {
	p.log.Info("Closing GeminiProvider (client closure depends on service implementation)")
	return nil
}

func (p *GeminiProvider) GetResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (LLMResponse, error) {
	startTime := time.Now()

	// --- Configure the injected service ---
	genaiConfig, err := mapLLMConfigToGenaiConfig(config)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to map request config: %w", err)
	}

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
	// Configure the service instance for this call
	err = p.service.ConfigureModel(genaiConfig, genaiTools)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to configure gemini model service: %w", err)
	}

	// --- Start Chat Session via Service ---
	initialHistory, err := p.mapLLMMessagesToGenaiContent(messages) // Use p.map... if it uses p.log
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to map initial messages: %w", err)
	}

	// Pass only the history needed to start the session service
	sessionService := p.service.StartChat(initialHistory)

	// The real session service handles history internally, so we don't need cs.History here.
	// We just need the parts for the first call.
	var initialParts []genai.Part
	if len(initialHistory) > 0 {
		// If StartChat only takes history, SendMessage needs the first turn's parts.
		// Let's assume StartChat consumed the history, and SendMessage sends the *next* turn.
		// This might need adjustment based on how the interfaces are best used.
		// Simpler: Assume StartChat takes all history, SendMessage sends ONLY new parts.
		initialParts = initialHistory[len(initialHistory)-1].Parts // Assuming last message is the one to send
		// If initialHistory had only 1 item, StartChat would have gotten it all.
		// If > 1, StartChat might need all but last, and SendMessage sends last. Let's adjust:
		var historyForSession []*genai.Content
		if len(initialHistory) > 1 {
			historyForSession = initialHistory[:len(initialHistory)-1]
			initialParts = initialHistory[len(initialHistory)-1].Parts
		} else if len(initialHistory) == 1 {
			historyForSession = []*genai.Content{} // Start with empty history
			initialParts = initialHistory[0].Parts
		} else {
			return LLMResponse{}, errors.New("cannot start LLM conversation with empty initial message")
		}
		sessionService = p.service.StartChat(historyForSession) // Pass adjusted history

	} // This history setup needs careful checking against interface intent
	if len(initialParts) == 0 { // Check initial parts again after adjustment
		return LLMResponse{}, errors.New("cannot determine initial message parts to send")
	}

	var finalResponse *genai.GenerateContentResponse
	sendParts := initialParts
	var loopError error

	// --- Tool loop using sessionService ---
	for turn := 0; turn < MaxToolTurns; turn++ {
		p.log.Infof("Gemini Request Turn %d: Sending %d parts via service\n", turn+1, len(sendParts))
		resp, err := sessionService.SendMessage(ctx, sendParts...) // Use the session service interface
		if err != nil {
			loopError = fmt.Errorf("gemini SendMessage service failed (turn %d): %w", turn+1, err)
			break
		}

		// Since the real session updates history internally, how do we get the model's response added for OUR logic?
		// The AppendHistory method on the interface is key here.
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			// We need to manually append the model's response to the session via the interface
			// to keep the mock and real behavior potentially aligned IF the provider needs access to full history later.
			// OR adjust the interface so SendMessage returns the updated history. Let's try AppendHistory.
			sessionService.AppendHistory(resp.Candidates[0].Content)
		} else {
			p.log.Warnf("Warning: Candidate content is nil in API response (turn %d). Cannot append to history.", turn+1)
			// What should happen here? Maybe continue without appending?
		}

		if len(resp.Candidates) == 0 { // Check candidates after trying to append history
			// ... (error handling for no candidates/block) ...
			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				loopError = fmt.Errorf("request blocked by API (turn %d): %s", turn+1, resp.PromptFeedback.BlockReason.String())
			} else {
				loopError = fmt.Errorf("gemini API returned no candidates (turn %d)", turn+1)
			}
			break
		}
		candidate := resp.Candidates[0] // Now safe to access candidate

		// Function call check uses candidate, no change here
		funcCalls := p.findFunctionCalls(candidate)
		if len(funcCalls) == 0 {
			// ... (final response handling, break) ...
			// Safety check, logging, etc.
			finalResponse = resp
			p.log.Infof("Gemini Response Turn %d: Received non-function-call response via service. Assuming final.", turn+1)
			break
		}

		p.log.Infof("Gemini Response Turn %d: Received %d function call(s) via service.", turn+1, len(funcCalls))
		functionResponseParts := make([]genai.Part, 0, len(funcCalls))
		allToolExecutionsSuccessful := true

		// Tool execution loop remains the same, uses activeTools map
		for _, fc := range funcCalls {
			// ... (tool execution logic - no changes needed here) ...
			p.log.Infof("Executing tool: %s", fc.Name)
			toolToRun, exists := activeTools[fc.Name]
			if !exists || toolToRun.Handler == nil {
				// ... handle missing tool ...
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Tool '%s' not found or not runnable.", fc.Name)}}}
				resultMap, mapErr := callResultToMap(toolResult)
				if mapErr != nil {
					allToolExecutionsSuccessful = false
					continue
				}
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				allToolExecutionsSuccessful = false
				continue
			}
			argsJSON, marshalErr := json.Marshal(fc.Args)
			if marshalErr != nil {
				// ... handle arg marshal error ...
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Invalid arguments format for tool '%s': %v", fc.Name, marshalErr)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				allToolExecutionsSuccessful = false
				continue
			}
			callParams := mcp.CallToolParams{Name: fc.Name, Arguments: json.RawMessage(argsJSON)}
			toolResult, handlerErr := toolToRun.Handler(ctx, callParams)
			if handlerErr != nil {
				// ... handle handler error ...
				if !toolResult.IsError {
					toolResult.IsError = true
					toolResult.Content = append(toolResult.Content, mcp.ToolResultContent{Type: "text", Text: fmt.Sprintf("Handler error: %v", handlerErr)})
				}
				allToolExecutionsSuccessful = false
			}
			resultMap, mapErr := callResultToMap(toolResult)
			if mapErr != nil {
				// ... handle result map error ...
				resultMap = map[string]any{"error": fmt.Sprintf("Failed to format tool result: %v", mapErr)}
				allToolExecutionsSuccessful = false
			}
			functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
		}

		if len(functionResponseParts) > 0 {
			// Create the content object for the function response turn
			functionResponseContent := &genai.Content{
				Role:  RoleFunction,
				Parts: functionResponseParts,
			}
			// Append this turn to the session via the interface
			sessionService.AppendHistory(functionResponseContent)

			if !allToolExecutionsSuccessful {
				p.log.Infof("Warning: One or more tool executions failed or had errors during turn %d.", turn+1)
			}
			sendParts = []genai.Part{genai.Text("")} // Send dummy part for next turn
		} else {
			p.log.Infof("Error: No function response parts could be generated for turn %d. Aborting loop.", turn+1)
			loopError = fmt.Errorf("failed to generate any function responses for turn %d", turn+1)
			break
		}

		if turn == MaxToolTurns-1 {
			p.log.Infof("Warning: Reached maximum tool turns (%d).", MaxToolTurns)
			loopError = fmt.Errorf("reached maximum tool turns (%d) without final text response", MaxToolTurns)
			// Break here, loopError will be checked after loop
			break
		}
	} // End tool calling loop

	// After loop processing remains the same, uses finalResponse and loopError
	// ... (check loopError, check finalResponse, construct LLMResponse) ...
	if loopError != nil {
		return LLMResponse{}, loopError
	}
	if finalResponse == nil {
		return LLMResponse{}, errors.New("tool loop finished, but no final response was captured")
	}
	// Sanity check finalResponse
	if len(finalResponse.Candidates) > 0 {
		finalFuncCalls := p.findFunctionCalls(finalResponse.Candidates[0])
		if len(finalFuncCalls) > 0 {
			p.log.Infof("Error: Loop finished, but the captured 'finalResponse' still contained %d function call(s).", len(finalFuncCalls))
			return LLMResponse{}, fmt.Errorf("tool loop finished, but final response required further tool calls (max turns likely reached)")
		}
	} else { // Should not happen if finalResponse is not nil, but check anyway
		return LLMResponse{}, errors.New("internal error: loop finished with nil candidates despite non-nil finalResponse")
	}

	llmResponse := LLMResponse{CompletionTime: time.Since(startTime).Seconds()}
	if len(finalResponse.Candidates) > 0 {
		llmResponse.Text = extractTextFromParts(finalResponse.Candidates[0].Content.Parts)
	}
	if finalResponse.UsageMetadata != nil {
		llmResponse.TotalInputToken = int(finalResponse.UsageMetadata.PromptTokenCount)
		llmResponse.TotalOutputToken = int(finalResponse.UsageMetadata.CandidatesTokenCount)
	} else {
		p.log.Info("Warning: UsageMetadata may not be available on the final SendMessage response.")
	}
	return llmResponse, nil
}

// Modified GetStreamingResponse
func (p *GeminiProvider) GetStreamingResponse(ctx context.Context, messages []LLMMessage, config LLMRequestConfig) (<-chan StreamingLLMResponse, error) {
	// --- Configure the injected service ---
	genaiConfig, err := mapLLMConfigToGenaiConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to map request config: %w", err)
	}
	var activeTools map[string]mcp.Tool
	var genaiTools []*genai.Tool
	if len(config.AllowedTools) > 0 {
		if config.toolsProvider == nil {
			return nil, errors.New("request config allows tools but provides no toolsProvider")
		}
		fetchedTools, err := config.toolsProvider.ListTools(ctx, config.AllowedTools)
		if err != nil {
			return nil, fmt.Errorf("failed to get tools from config.toolsProvider: %w", err)
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
	}
	err = p.service.ConfigureModel(genaiConfig, genaiTools)
	if err != nil {
		return nil, fmt.Errorf("failed to configure gemini model service: %w", err)
	}

	// --- Start Chat Session via Service for Pre-flight ---
	initialHistory, err := p.mapLLMMessagesToGenaiContent(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to map initial messages: %w", err)
	}

	var historyForSession []*genai.Content
	var initialParts []genai.Part
	if len(initialHistory) > 1 {
		historyForSession = initialHistory[:len(initialHistory)-1]
		initialParts = initialHistory[len(initialHistory)-1].Parts
	} else if len(initialHistory) == 1 {
		historyForSession = []*genai.Content{}
		initialParts = initialHistory[0].Parts
	} else {
		return nil, errors.New("cannot start LLM streaming conversation with empty initial message")
	}
	sessionService := p.service.StartChat(historyForSession) // Start session for pre-flight

	if len(initialParts) == 0 {
		return nil, errors.New("cannot determine initial message parts to send for streaming")
	}

	var finalSyncResponse *genai.GenerateContentResponse
	var loopError error
	sendParts := initialParts

	// Pre-flight loop using sessionService
	for turn := 0; turn < MaxToolTurns; turn++ {
		p.log.Infof("Streaming Pre-flight Turn %d: Sending %d parts via service\n", turn+1, len(sendParts))
		resp, err := sessionService.SendMessage(ctx, sendParts...) // Use session service
		if err != nil {
			loopError = fmt.Errorf("gemini pre-flight SendMessage failed (turn %d): %w", turn+1, err)
			break
		}

		// Append model response to session history via interface
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			sessionService.AppendHistory(resp.Candidates[0].Content)
		} else {
			p.log.Warnf("Warning: Pre-flight candidate content is nil (turn %d).", turn+1)
		}

		if len(resp.Candidates) == 0 {
			// ... (error handling for no candidates/block) ...
			if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
				loopError = fmt.Errorf("request blocked by API during pre-flight (turn %d): %s", turn+1, resp.PromptFeedback.BlockReason.String())
			} else {
				loopError = fmt.Errorf("gemini pre-flight API returned no candidates (turn %d)", turn+1)
			}
			break
		}
		candidate := resp.Candidates[0]

		funcCalls := p.findFunctionCalls(candidate)
		if len(funcCalls) == 0 {
			p.log.Infof("Streaming Pre-flight Turn %d: Tool resolution complete.", turn+1)
			finalSyncResponse = resp // Capture final sync response
			break                    // Exit pre-flight loop
		}

		p.log.Infof("Streaming Pre-flight Turn %d: Received %d function call(s) via service.", turn+1, len(funcCalls))
		functionResponseParts := make([]genai.Part, 0, len(funcCalls))
		allToolExecutionsSuccessful := true

		// Tool execution loop (same as GetResponse)
		for _, fc := range funcCalls {
			// ... (tool execution logic) ...
			toolToRun, exists := activeTools[fc.Name]
			if !exists || toolToRun.Handler == nil { /* ... handle ... */
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Tool '%s' not found.", fc.Name)}}}
				resultMap, mapErr := callResultToMap(toolResult)
				if mapErr != nil {
					allToolExecutionsSuccessful = false
					continue
				}
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				allToolExecutionsSuccessful = false
				continue
			}
			argsJSON, marshalErr := json.Marshal(fc.Args)
			if marshalErr != nil { /* ... handle ... */
				toolResult := mcp.CallToolResult{IsError: true, Content: []mcp.ToolResultContent{{Type: "text", Text: fmt.Sprintf("Invalid args for '%s': %v", fc.Name, marshalErr)}}}
				resultMap, _ := callResultToMap(toolResult)
				functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
				allToolExecutionsSuccessful = false
				continue
			}
			callParams := mcp.CallToolParams{Name: fc.Name, Arguments: json.RawMessage(argsJSON)}
			toolResult, handlerErr := toolToRun.Handler(ctx, callParams)
			if handlerErr != nil { /* ... handle ... */
				if !toolResult.IsError {
					toolResult.IsError = true
					toolResult.Content = append(toolResult.Content, mcp.ToolResultContent{Type: "text", Text: fmt.Sprintf("Handler error: %v", handlerErr)})
				}
				allToolExecutionsSuccessful = false
			}
			resultMap, mapErr := callResultToMap(toolResult)
			if mapErr != nil { /* ... handle ... */
				resultMap = map[string]any{"error": fmt.Sprintf("Failed to format tool result: %v", mapErr)}
				allToolExecutionsSuccessful = false
			}
			functionResponseParts = append(functionResponseParts, genai.FunctionResponse{Name: fc.Name, Response: resultMap})
		}

		if len(functionResponseParts) > 0 {
			// Append function response turn to session history via interface
			functionResponseContent := &genai.Content{Role: RoleFunction, Parts: functionResponseParts}
			sessionService.AppendHistory(functionResponseContent)
			if !allToolExecutionsSuccessful {
				p.log.Infof("Warning: One or more tool executions failed or had errors during pre-flight turn %d.", turn+1)
			}
			sendParts = []genai.Part{genai.Text("")} // Send dummy part for next turn
		} else {
			p.log.Infof("Error: No function response parts could be generated during pre-flight turn %d. Aborting stream setup.", turn+1)
			loopError = fmt.Errorf("failed to generate any function responses during pre-flight turn %d", turn+1)
			break
		}

		if turn == MaxToolTurns-1 {
			p.log.Infof("Warning: Reached maximum tool turns (%d) during streaming pre-flight.", MaxToolTurns)
			loopError = fmt.Errorf("reached maximum tool turns (%d) during streaming pre-flight", MaxToolTurns)
			break
		}
	} // End pre-flight loop

	// --- After Pre-flight Loop ---
	if loopError != nil {
		return nil, loopError
	}
	if finalSyncResponse == nil {
		return nil, errors.New("pre-flight tool loop finished, but no final response was captured")
	}

	// Now, we need to initiate the actual stream. The original problem remains:
	// GenerateContentStream expects ...Part, not history.
	// The "Simulated Streaming" approach is still the most viable with the current interface.
	finalContent := finalSyncResponse.Candidates[0].Content
	finalText := extractTextFromParts(finalContent.Parts)
	hasFunctionCall := false // Inline check instead of helper
	if finalContent != nil {
		for _, part := range finalContent.Parts {
			if _, ok := part.(genai.FunctionCall); ok {
				hasFunctionCall = true
				break
			}
			if _, ok := part.(*genai.FunctionCall); ok {
				hasFunctionCall = true
				break
			}
		}
	}
	if hasFunctionCall {
		return nil, fmt.Errorf("internal error: pre-flight loop captured a final response that still contained function calls")
	}

	// --- Simulate Streaming ---
	streamChan := make(chan StreamingLLMResponse, 1)
	go func() {
		defer close(streamChan)
		var outputTokens int = 0
		if finalSyncResponse != nil && finalSyncResponse.UsageMetadata != nil {
			outputTokens = int(finalSyncResponse.UsageMetadata.CandidatesTokenCount)
		} else {
			p.log.Infof("Warning: UsageMetadata missing on final synchronous response object.")
		}
		p.log.Infof("Simulating stream: Sending final pre-flight response text.")
		streamChan <- StreamingLLMResponse{Text: finalText, Done: true, Error: nil, TokenCount: outputTokens}
	}()

	return streamChan, nil
}

func (p *GeminiProvider) mapLLMMessagesToGenaiContent(messages []LLMMessage) ([]*genai.Content, error) {
	genaiMessages := make([]*genai.Content, 0, len(messages))
	for _, msg := range messages {
		var role GeminiRole
		switch msg.Role {
		case UserRole:
			role = GeminiRoleUser
		case AssistantRole:
			role = GeminiRoleModel
		case SystemRole:
			p.log.Info("Mapping LLMRoleSystem to Gemini GeminiRoleUser") // Use injected logger
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
		genaiMessages = append(genaiMessages, &genai.Content{Role: role, Parts: []genai.Part{genai.Text(msg.Text)}})
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

func (p *GeminiProvider) findFunctionCalls(candidate *genai.Candidate) []*genai.FunctionCall {
	calls := make([]*genai.FunctionCall, 0)
	if candidate == nil || candidate.Content == nil {
		return calls
	}
	for _, part := range candidate.Content.Parts {
		p.log.Infof("Checking part: %+v (Type: %T)", part, part) // Use injected logger
		if fcValue, ok := part.(genai.FunctionCall); ok {
			p.log.Infof("Found function call VALUE: %+v", fcValue) // Use injected logger
			fcPointer := &fcValue
			calls = append(calls, fcPointer)
		} else if fcPointer, ok := part.(*genai.FunctionCall); ok {
			p.log.Infof("Found function call POINTER: %+v", fcPointer) // Use injected logger
			calls = append(calls, fcPointer)
		}
	}
	p.log.Infof("findFunctionCalls returning %d calls", len(calls)) // Use injected logger
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
