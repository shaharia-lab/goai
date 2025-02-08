package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// --- Initialize Request/Response ---

type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"` // Keep arguments
	Messages    []PromptMessage  `json:"messages,omitempty"`  // For prompts/get
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// VERY IMPORTANT: At this stage, only 'text' type is supported
type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Add Image, Audio, Resource content types later.
}

type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"` // For pagination (optional, but good practice).
}

type GetPromptParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"` // Raw JSON for flexibility.
}

// --- Server Implementation ---

type MCPServer struct {
	protocolVersion    string
	clientCapabilities map[string]any
	logger             *log.Logger
	in                 io.Reader
	out                io.Writer
	ServerInfo         struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	capabilities map[string]any
	resources    map[string]Resource
	minLogLevel  LogLevel
	tools        map[string]Tool // Store available tools
	prompts      map[string]Prompt
}

func NewMCPServer(in io.Reader, out io.Writer) *MCPServer {
	logger := log.New(os.Stderr, "[MCP Server] ", log.LstdFlags|log.Lmsgprefix)

	s := &MCPServer{
		protocolVersion: "2024-11-05",
		logger:          logger,
		in:              in,
		out:             out,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "Resource-Server-Example",
			Version: "0.1.0",
		},
		capabilities: map[string]any{
			"resources": map[string]any{
				"listChanged": false,
				"subscribe":   false,
			},
			"logging": map[string]any{},
			"tools": map[string]any{ // Add tools capability
				"listChanged": false,
			},
			"prompts": map[string]any{"listChanged": false},
		},
		resources:   make(map[string]Resource),
		minLogLevel: LogLevelInfo,
		tools:       make(map[string]Tool), // Initialize tools map
		prompts:     make(map[string]Prompt),
	}

	s.addResource(Resource{
		URI:         "file:///example/resource1.txt",
		Name:        "Example Resource 1",
		Description: "This is the first example resource.",
		MimeType:    "text/plain",
		TextContent: "Content of resource 1.",
	})
	s.addResource(Resource{
		URI:         "file:///example/resource2.json",
		Name:        "Example Resource 2",
		Description: "This is the second example resource.",
		MimeType:    "application/json",
		TextContent: `{"key": "value"}`,
	})

	// Add a sample tool
	s.addTool(Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a given location.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "location": {
                    "type": "string",
                    "description": "The city and state, e.g. San Francisco, CA"
                }
            },
            "required": ["location"]
        }`),
	})

	s.addPrompt(Prompt{
		Name:        "code_review",
		Description: "Asks the LLM to analyze code quality and suggest improvements",
		Arguments: []PromptArgument{
			{Name: "code", Description: "The code to review", Required: true},
		},
		Messages: []PromptMessage{ //Added messages
			{
				Role: "user",
				Content: PromptContent{
					Type: "text",
					Text: "Please review the following code:\n{code}",
				},
			},
		},
	})

	s.addPrompt(Prompt{
		Name:        "summarize_text",
		Description: "Summarizes the given text",
		Arguments: []PromptArgument{
			{Name: "text", Description: "Text for summarize", Required: true},
		},
		Messages: []PromptMessage{ //Added messages
			{
				Role: "user",
				Content: PromptContent{
					Type: "text",
					Text: "Summarize the following text:\n{text}",
				},
			},
		},
	})

	return s
}

func (s *MCPServer) addResource(resource Resource) {
	s.resources[resource.URI] = resource
}
func (s *MCPServer) addTool(tool Tool) {
	s.tools[tool.Name] = tool
}

func (s *MCPServer) addPrompt(prompt Prompt) {
	s.prompts[prompt.Name] = prompt
}

func (s *MCPServer) sendResponse(id *json.RawMessage, result interface{}, err *Error) {
	response := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	jsonResponse, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		s.logger.Printf("Error marshalling response: %v", marshalErr)
		s.sendError(id, -32603, "Internal error: failed to marshal response", nil)
		return
	}

	jsonResponse = append(jsonResponse, '\n')
	_, writeErr := s.out.Write(jsonResponse)
	if writeErr != nil {
		s.logger.Printf("Error writing response to stdout: %v", writeErr)
	}
}

func (s *MCPServer) sendError(id *json.RawMessage, code int, message string, data interface{}) {
	errorResponse := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	jsonErrorResponse, err := json.Marshal(errorResponse)
	if err != nil {
		s.logger.Printf("Error marshaling error response: %v", err)
		return
	}
	jsonErrorResponse = append(jsonErrorResponse, '\n')
	_, writeErr := s.out.Write(jsonErrorResponse)
	if writeErr != nil {
		s.logger.Printf("Failed to write error response: %v", writeErr)
	}
}

func (s *MCPServer) sendNotification(method string, params interface{}) {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			s.logger.Printf("Error marshaling notification parameters: %v", err)
			return
		}
		notification.Params = json.RawMessage(paramsBytes)
	}

	jsonNotification, err := json.Marshal(notification)
	if err != nil {
		s.logger.Printf("Error marshaling notification message: %v", err)
		return
	}
	jsonNotification = append(jsonNotification, '\n')
	_, writeErr := s.out.Write(jsonNotification)
	if writeErr != nil {
		s.logger.Printf("Failed to write notification message: %v", writeErr)
	}
}

func (s *MCPServer) handleRequest(request *Request) {
	s.logger.Printf("Received request: method=%s, id=%v", request.Method, request.ID)

	switch request.Method {
	case "initialize":
		var params InitializeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(request.ID, -32602, "Invalid params", nil)
			return
		}

		if !strings.HasPrefix(params.ProtocolVersion, "2024-11") {
			s.sendError(request.ID, -32602, "Unsupported protocol version", map[string][]string{"supported": {"2024-11-05"}})
			return
		}

		s.clientCapabilities = params.Capabilities
		result := InitializeResult{
			ProtocolVersion: s.protocolVersion,
			Capabilities:    s.capabilities,
			ServerInfo:      s.ServerInfo,
		}
		s.sendResponse(request.ID, result, nil)

	case "ping":
		s.sendResponse(request.ID, map[string]interface{}{}, nil)

	case "resources/list":
		var resourceList []Resource
		for _, res := range s.resources {
			resourceList = append(resourceList, Resource{
				URI:         res.URI,
				Name:        res.Name,
				Description: res.Description,
				MimeType:    res.MimeType,
			})
		}
		result := ListResourcesResult{Resources: resourceList}
		s.sendResponse(request.ID, result, nil)

	case "resources/read":
		var params ReadResourceParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(request.ID, -32602, "Invalid params", nil)
			return
		}

		resource, ok := s.resources[params.URI]
		if !ok {
			s.sendError(request.ID, -32002, "Resource not found", map[string]string{"uri": params.URI})
			return
		}

		result := ReadResourceResult{
			Contents: []ResourceContent{{
				URI:      resource.URI,
				MimeType: resource.MimeType,
				Text:     resource.TextContent,
			}},
		}
		s.sendResponse(request.ID, result, nil)
	case "prompts/list":
		var promptList []Prompt
		for _, prompt := range s.prompts {
			// Create a copy *without* the Messages field for the list response.
			promptList = append(promptList, Prompt{
				Name:        prompt.Name,
				Description: prompt.Description,
				Arguments:   prompt.Arguments, // Include arguments
				// Messages is *not* included in the list response.
			})
		}
		result := ListPromptsResult{Prompts: promptList}
		s.sendResponse(request.ID, result, nil)
	case "prompts/get":
		var params GetPromptParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(request.ID, -32602, "Invalid params", nil)
			return
		}

		prompt, ok := s.prompts[params.Name]
		if !ok {
			s.sendError(request.ID, -32602, "Prompt not found", map[string]string{"prompt": params.Name})
			return
		}

		// For simplicity, this example doesn't process arguments.
		// A full implementation would substitute arguments into the prompt messages.

		result := Prompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Messages:    prompt.Messages, // Include messages in get response

		}
		s.sendResponse(request.ID, result, nil)

	case "logging/setLevel":
		var params SetLogLevelParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(request.ID, -32602, "Invalid Params", nil)
			return
		}
		if _, ok := logLevelSeverity[params.Level]; !ok {
			s.sendError(request.ID, -32602, "Invalid log level", nil)
			return
		}

		s.minLogLevel = params.Level
		s.sendResponse(request.ID, struct{}{}, nil)

	case "tools/list":
		var toolList []Tool
		for _, tool := range s.tools {
			toolList = append(toolList, tool) // Append the tool directly.
		}
		result := ListToolsResult{Tools: toolList}
		s.sendResponse(request.ID, result, nil)
	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(request.ID, -32602, "Invalid params", nil)
			return
		}
		tool, ok := s.tools[params.Name]
		if !ok {
			s.sendError(request.ID, -32602, "Unknown tool", map[string]string{"tool": params.Name})
			return
		}
		// VERY basic argument validation, just checking required fields.
		var input map[string]interface{}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			s.sendError(request.ID, -32602, "Invalid tool arguments", nil)
			return
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			s.sendError(request.ID, -32603, "Internal error: invalid tool schema", nil)
			return
		}

		required, hasRequired := schema["required"].([]interface{}) // Use type assertion
		if hasRequired {
			for _, reqField := range required {
				reqFieldName, ok := reqField.(string)
				if !ok {
					continue
				}
				if _, present := input[reqFieldName]; !present {
					s.sendError(request.ID, -32602, "Missing required argument", map[string]string{"argument": reqFieldName})
					return
				}
			}
		}

		// Very basic tool execution.
		result := CallToolResult{IsError: false}
		if params.Name == "get_weather" {
			location, _ := input["location"].(string) //  We checked presence above
			result.Content = []ToolResultContent{{
				Type: "text",
				Text: fmt.Sprintf("Weather in %s: Sunny, 72°F", location), //  Fake data
			}}

		}
		s.sendResponse(request.ID, result, nil)

	default:
		s.sendError(request.ID, -32601, "Method not found", nil)
	}
}

func (s *MCPServer) handleNotification(notification *Notification) {
	s.logger.Printf("Received notification: method=%s", notification.Method)
	switch notification.Method {
	case "notifications/initialized":
		// The server is now initialized.
	case "notifications/cancelled":
		var cancelParams struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if err := json.Unmarshal(notification.Params, &cancelParams); err == nil {
			s.logger.Printf("Cancellation requested for ID %s: %s", string(cancelParams.RequestID), cancelParams.Reason)
		}

	default:
		s.logger.Printf("Unhandled notification: %s", notification.Method) // Log but don't send error.
	}
}

func (s *MCPServer) LogMessage(level LogLevel, loggerName string, data interface{}) {
	if logLevelSeverity[level] > logLevelSeverity[s.minLogLevel] {
		return
	}

	params := LogMessageParams{
		Level:  level,
		Logger: loggerName,
		Data:   data,
	}
	s.sendNotification("notifications/message", params)
}

func (s *MCPServer) Run() {
	scanner := bufio.NewScanner(s.in)
	buffer := make([]byte, 0, 64*1024) //  buffer
	scanner.Buffer(buffer, 1024*1024)  //  buffer size

	initialized := false // track whether we've received 'initialized'

	for scanner.Scan() {
		line := scanner.Text()
		s.logger.Printf("Received raw input: %s", line) //  Log the raw JSON.

		var raw json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			s.sendError(nil, -32700, "Parse error", nil)
			continue
		}

		var request Request
		if err := json.Unmarshal(raw, &request); err == nil && request.Method != "" && request.ID != nil {
			// It's a request
			if request.Method != "initialize" && !initialized {
				s.sendError(request.ID, -32000, "Server not initialized", nil) // Or other appropriate error code.
				continue
			}
			s.handleRequest(&request)
			continue
		}

		var notification Notification
		if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
			// It's a notification
			if notification.Method == "notifications/initialized" {
				initialized = true
			} else if !initialized {
				// We shouldn't receive any other notifications before 'initialized'.  Log it, but don't send error.
				s.logger.Printf("Received notification before 'initialized': %s", notification.Method)
				continue
			}

			s.handleNotification(&notification)
			continue
		}

		// If we reach here, it's neither a valid request nor a notification.
		s.sendError(nil, -32600, "Invalid Request", nil)

	}

	if err := scanner.Err(); err != nil {
		if err != io.EOF { // Don't log EOF, that's how we exit gracefully
			s.logger.Printf("Error reading from stdin: %v", err)
		}
	}
	s.logger.Println("Server shutting down.")
}
