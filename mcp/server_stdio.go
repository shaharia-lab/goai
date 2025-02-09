package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// StdIOServer represents the main Message Control Protocol server that handles communication
// and manages resources, tools, and prompts.
type StdIOServer struct {
	// ServerInfo contains basic information about the server
	protocolVersion    string
	clientCapabilities map[string]any
	logger             *log.Logger
	in                 io.Reader
	out                io.Writer
	ServerInfo         struct {
		// Name is the name of the server
		Name string `json:"name"`

		// Version is the version of the server
		Version string `json:"version"`
	}
	capabilities              map[string]any
	resources                 map[string]Resource
	minLogLevel               LogLevel
	tools                     map[string]Tool // Store available tools
	prompts                   map[string]Prompt
	supportsPromptListChanged bool
	supportsToolListChanged   bool
}

// NewStdIOServer creates and initializes a new MCP server instance with the specified input and output streams.
//
// Example:
//
//	server := NewStdIOServer(os.Stdin, os.Stdout)
//	// server is ready to use with default configuration and sample resources
func NewStdIOServer(in io.Reader, out io.Writer) *StdIOServer {
	logger := log.New(os.Stderr, "[MCP StdIOServer] ", log.LstdFlags|log.Lmsgprefix)

	s := &StdIOServer{
		protocolVersion: "2024-11-05",
		logger:          logger,
		in:              in,
		out:             out,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "Resource-StdIOServer-Example",
			Version: "0.1.0",
		},
		capabilities: map[string]any{
			"resources": map[string]any{
				"listChanged": true,
				"subscribe":   true,
			},
			"logging": map[string]any{},
			"tools": map[string]any{ // Add tools capability
				"listChanged": true,
			},
			"prompts": map[string]any{"listChanged": true},
		},
		resources:                 make(map[string]Resource),
		minLogLevel:               LogLevelInfo,
		tools:                     make(map[string]Tool), // Initialize tools map
		prompts:                   make(map[string]Prompt),
		supportsPromptListChanged: false,
		supportsToolListChanged:   false,
	}

	s.AddResource(Resource{
		URI:         "file:///example/resource1.txt",
		Name:        "Example Resource 1",
		Description: "This is the first example resource.",
		MimeType:    "text/plain",
		TextContent: "Content of resource 1.",
	})
	s.AddResource(Resource{
		URI:         "file:///example/resource2.json",
		Name:        "Example Resource 2",
		Description: "This is the second example resource.",
		MimeType:    "application/json",
		TextContent: `{"key": "value"}`,
	})

	// Add a sample tool
	s.AddTool(Tool{
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

	s.AddPrompt(Prompt{
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

	s.AddPrompt(Prompt{
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

// AddResource adds a new resource to the server's resource collection.
//
// Example:
//
//	server.AddResource(Resource{
//	    URI: "file:///example/doc.txt",
//	    Name: "Documentation",
//	    Description: "API documentation",
//	    MimeType: "text/plain",
//	    TextContent: "API documentation content",
//	})
func (s *StdIOServer) AddResource(resource Resource) {
	s.resources[resource.URI] = resource
}

// AddTool registers a new tool with the server's tool collection.
//
// Example:
//
//	server.AddTool(Tool{
//	    Name: "calculator",
//	    Description: "Performs basic arithmetic",
//	    InputSchema: json.RawMessage(`{
//	        "type": "object",
//	        "properties": {
//	            "operation": {"type": "string"},
//	            "operands": {"type": "array"}
//	        }
//	    }`),
//	})
func (s *StdIOServer) AddTool(tool Tool) {
	s.tools[tool.Name] = tool

	if s.supportsToolListChanged {
		s.SendToolListChangedNotification()
	}
}

// AddPrompt registers a new prompt template with the server.
//
// Example:
//
//	server.AddPrompt(Prompt{
//	    Name: "translate",
//	    Description: "Translates text to specified language",
//	    Arguments: []PromptArgument{
//	        {Name: "text", Description: "Text to translate", Required: true},
//	        {Name: "targetLang", Description: "Target language", Required: true},
//	    },
//	    Messages: []PromptMessage{
//	        {
//	            Role: "user",
//	            Content: PromptContent{
//	                Type: "text",
//	                Text: "Translate this text: {text} to {targetLang}",
//	            },
//	        },
//	    },
//	})
func (s *StdIOServer) AddPrompt(prompt Prompt) {
	s.prompts[prompt.Name] = prompt

	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}
}

// DeletePrompt removes a prompt template identified by its name from the server's collection.
// Returns an error if the prompt does not exist. Sends a notification if prompt list change support is enabled.
func (s *StdIOServer) DeletePrompt(name string) error {
	if _, exists := s.prompts[name]; !exists {
		return fmt.Errorf("prompt not found: %s", name)
	}
	delete(s.prompts, name)
	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}
	return nil
}

func (s *StdIOServer) sendResponse(id *json.RawMessage, result interface{}, err *Error) {
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

func (s *StdIOServer) sendError(id *json.RawMessage, code int, message string, data interface{}) {
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

func (s *StdIOServer) sendNotification(method string, params interface{}) {
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

func (s *StdIOServer) handleRequest(request *Request) {
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

		// Check and set if server supports prompt list changed.
		if promptCaps, ok := s.capabilities["prompts"].(map[string]any); ok {
			if listChanged, ok := promptCaps["listChanged"].(bool); ok && listChanged {
				s.supportsPromptListChanged = true
			}
		}

		// Check and set if server supports tool list changed
		if toolCaps, ok := s.capabilities["tools"].(map[string]any); ok {
			if listChanged, ok := toolCaps["listChanged"].(bool); ok && listChanged {
				s.supportsToolListChanged = true
			}
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

func (s *StdIOServer) handleNotification(notification *Notification) {
	s.logger.Printf("Received notification: method=%s", notification.Method)
	switch notification.Method {
	case "notifications/initialized":
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

// LogMessage sends a log message notification with the specified severity level, logger name, and associated data.
// The message is only logged if its severity level satisfies the server's minimum log level.
func (s *StdIOServer) LogMessage(level LogLevel, loggerName string, data interface{}) {
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

// SendPromptListChangedNotification sends a notification indicating the prompt list has been changed.
func (s *StdIOServer) SendPromptListChangedNotification() {
	s.sendNotification("notifications/prompts/list_changed", nil)
}

// SendToolListChangedNotification sends a notification to indicate that the tool list has been updated.
func (s *StdIOServer) SendToolListChangedNotification() {
	s.sendNotification("notifications/tools/list_changed", nil)
}

// Run starts the processing loop of the StdIOServer, handling incoming input, requests, and notifications.
func (s *StdIOServer) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	initialized := false
	done := make(chan error, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			default:
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil && err != io.EOF {
						done <- fmt.Errorf("scanner error: %w", err)
					} else {
						done <- nil
					}
					return
				}

				line := scanner.Text()
				s.logger.Printf("Received raw input: %s", line)

				var raw json.RawMessage
				if err := json.Unmarshal([]byte(line), &raw); err != nil {
					s.sendError(nil, -32700, "Parse error", nil)
					continue
				}

				var request Request
				if err := json.Unmarshal(raw, &request); err == nil && request.Method != "" && request.ID != nil {
					if request.Method != "initialize" && !initialized {
						s.sendError(request.ID, -32000, "StdIOServer not initialized", nil)
						continue
					}
					s.handleRequest(&request)
					continue
				}

				var notification Notification
				if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
					if notification.Method == "notifications/initialized" {
						initialized = true
					} else if !initialized {
						s.logger.Printf("Received notification before 'initialized': %s", notification.Method)
						continue
					}
					s.handleNotification(&notification)
					continue
				}

				s.sendError(nil, -32600, "Invalid Request", nil)
			}
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Println("Context cancelled, shutting down server...")
		return ctx.Err()
	case err := <-done:
		s.logger.Println("StdIOServer shutting down.")
		return err
	}
}
