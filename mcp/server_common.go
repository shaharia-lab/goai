// /home/shaharia/Projects/goai/mcp/server_common.go
package mcp

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log"
	"strings"
)

// Server is the common interface for all MCP servers.
type Server interface {
	AddResource(resource Resource)
	AddTool(tool Tool)
	AddPrompt(prompt Prompt)
	DeletePrompt(name string) error
	LogMessage(level LogLevel, loggerName string, data interface{})
	SendPromptListChangedNotification()
	SendToolListChangedNotification()
	handleRequest(clientID string, request *Request)
	handleNotification(clientID string, notification *Notification)

	// Abstract methods for sending messages - to be implemented by specific server types.
	sendResponse(clientID string, id *json.RawMessage, result interface{}, err *Error)
	sendError(clientID string, id *json.RawMessage, code int, message string, data interface{})
	sendNotification(clientID string, method string, params interface{}) // clientID is needed, as both will send it, but SSE may send to many
}

// CommonServer contains the common fields and methods for all MCP server implementations.
type CommonServer struct {
	protocolVersion    string
	clientCapabilities map[string]any
	logger             *log.Logger
	ServerInfo         struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	capabilities              map[string]any
	resources                 map[string]Resource
	minLogLevel               LogLevel
	tools                     map[string]Tool
	prompts                   map[string]Prompt
	supportsPromptListChanged bool
	supportsToolListChanged   bool
	// The specific server implementations (StdIOServer, SSEServer) embed this and
	// provide the transport-specific parts.

	// New fields for managing clientID.  Needed for shared handleRequest
	generateClientID func() string
	//sendMessage   func(clientID string, message []byte) // NO.  Too generic, not all servers will send generic msgs

	// Abstract send methods.
	sendResp func(clientID string, id *json.RawMessage, result interface{}, err *Error)
	sendErr  func(clientID string, id *json.RawMessage, code int, message string, data interface{})
	sendNoti func(clientID string, method string, params interface{})
}

// NewCommonServer creates a new CommonServer instance, initialized with default values.
func NewCommonServer(logger *log.Logger) *CommonServer {
	s := &CommonServer{
		protocolVersion: "2024-11-05",
		logger:          logger,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "goai-server", // Default, can be overridden
			Version: "0.1.0",       // Default, can be overridden
		},
		capabilities: map[string]any{
			"resources": map[string]any{
				"listChanged": true,
				"subscribe":   true,
			},
			"logging": map[string]any{},
			"tools": map[string]any{
				"listChanged": true,
			},
			"prompts": map[string]any{"listChanged": true},
		},
		resources:                 make(map[string]Resource),
		minLogLevel:               LogLevelInfo,
		tools:                     make(map[string]Tool),
		prompts:                   make(map[string]Prompt),
		supportsPromptListChanged: false,
		supportsToolListChanged:   false,
		generateClientID:          func() string { return uuid.NewString() }, // Default generator.  SSE can use this.
	}

	return s
}

// AddResource adds a new resource to the server.
func (s *CommonServer) AddResource(resource Resource) {
	s.resources[resource.URI] = resource
}

// AddTool adds a new tool to the server.
func (s *CommonServer) AddTool(tool Tool) {
	s.tools[tool.Name] = tool
	if s.supportsToolListChanged {
		s.SendToolListChangedNotification()
	}
}

// AddPrompt adds a new prompt to the server.
func (s *CommonServer) AddPrompt(prompt Prompt) {
	s.prompts[prompt.Name] = prompt
	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}
}

// DeletePrompt removes a prompt from the server.
func (s *CommonServer) DeletePrompt(name string) error {
	if _, exists := s.prompts[name]; !exists {
		return fmt.Errorf("prompt not found: %s", name)
	}
	delete(s.prompts, name)
	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}
	return nil
}

// SendPromptListChangedNotification sends a notification that the prompt list has changed.
func (s *CommonServer) SendPromptListChangedNotification() {
	// The concrete server implementations will handle broadcasting this.
	s.sendNoti("", "notifications/prompts/list_changed", nil) // Empty clientID.  Common server doesn't know *who* to send to
}

// SendToolListChangedNotification sends a notification that the tool list has changed.
func (s *CommonServer) SendToolListChangedNotification() {
	s.sendNoti("", "notifications/tools/list_changed", nil)
}

// LogMessage logs a message.
func (s *CommonServer) LogMessage(level LogLevel, loggerName string, data interface{}) {
	if logLevelSeverity[level] > logLevelSeverity[s.minLogLevel] {
		return
	}

	params := LogMessageParams{
		Level:  level,
		Logger: loggerName,
		Data:   data,
	}
	s.sendNoti("", "notifications/message", params) // Empty client ID - it's a broadcast
}

// handleRequest handles incoming requests.  This is now common to both server types.
func (s *CommonServer) handleRequest(clientID string, request *Request) {
	s.logger.Printf("Received request from client %s: method=%s, id=%v", clientID, request.Method, request.ID)

	switch request.Method {
	case "initialize":
		var params InitializeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		if !strings.HasPrefix(params.ProtocolVersion, "2024-11") {
			s.sendErr(clientID, request.ID, -32602, "Unsupported protocol version", map[string][]string{"supported": {"2024-11-05"}})
			return
		}

		s.clientCapabilities = params.Capabilities
		result := InitializeResult{
			ProtocolVersion: s.protocolVersion,
			Capabilities:    s.capabilities,
			ServerInfo:      s.ServerInfo,
		}

		// Check and set if server supports prompt/tool list changed.
		if promptCaps, ok := s.capabilities["prompts"].(map[string]any); ok {
			if listChanged, ok := promptCaps["listChanged"].(bool); ok && listChanged {
				s.supportsPromptListChanged = true
			}
		}
		if toolCaps, ok := s.capabilities["tools"].(map[string]any); ok {
			if listChanged, ok := toolCaps["listChanged"].(bool); ok && listChanged {
				s.supportsToolListChanged = true
			}
		}

		s.sendResp(clientID, request.ID, result, nil)

	case "ping":
		s.sendResp(clientID, request.ID, map[string]interface{}{}, nil)

	case "resources/list":
		// Initialize the slice here:
		var resourceList []Resource = make([]Resource, 0)
		for _, res := range s.resources {
			resourceList = append(resourceList, Resource{
				URI:         res.URI,
				Name:        res.Name,
				Description: res.Description,
				MimeType:    res.MimeType,
			})
		}
		result := ListResourcesResult{Resources: resourceList}
		s.sendResp(clientID, request.ID, result, nil)

	case "resources/read":
		var params ReadResourceParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		resource, ok := s.resources[params.URI]
		if !ok {
			s.sendErr(clientID, request.ID, -32002, "Resource not found", map[string]string{"uri": params.URI})
			return
		}

		result := ReadResourceResult{
			Contents: []ResourceContent{{
				URI:      resource.URI,
				MimeType: resource.MimeType,
				Text:     resource.TextContent, // Assuming text content for now
			}},
		}
		s.sendResp(clientID, request.ID, result, nil)

	case "prompts/list":
		// Initialize the slice here:
		var promptList []Prompt = make([]Prompt, 0)
		for _, prompt := range s.prompts {
			promptList = append(promptList, Prompt{
				Name:        prompt.Name,
				Description: prompt.Description,
				Arguments:   prompt.Arguments,
			})
		}
		result := ListPromptsResult{Prompts: promptList}
		s.sendResp(clientID, request.ID, result, nil)

	case "prompts/get":
		var params GetPromptParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		prompt, ok := s.prompts[params.Name]
		if !ok {
			s.sendErr(clientID, request.ID, -32602, "Prompt not found", map[string]string{"prompt": params.Name})
			return
		}

		result := Prompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Messages:    prompt.Messages, // Include messages in get response
		}
		s.sendResp(clientID, request.ID, result, nil)

	case "logging/setLevel":
		var params SetLogLevelParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid Params", nil)
			return
		}
		if _, ok := logLevelSeverity[params.Level]; !ok {
			s.sendErr(clientID, request.ID, -32602, "Invalid log level", nil)
			return
		}

		s.minLogLevel = params.Level
		s.sendResp(clientID, request.ID, struct{}{}, nil)

	case "tools/list":
		// Initialize the slice here:
		var toolList []Tool = make([]Tool, 0)
		for _, tool := range s.tools {
			toolList = append(toolList, tool)
		}
		result := ListToolsResult{Tools: toolList}
		s.sendResp(clientID, request.ID, result, nil)

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}
		tool, ok := s.tools[params.Name]
		if !ok {
			s.sendErr(clientID, request.ID, -32602, "Unknown tool", map[string]string{"tool": params.Name})
			return
		}
		// VERY basic argument validation, just checking required fields.
		var input map[string]interface{}
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid tool arguments", nil)
			return
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			s.sendErr(clientID, request.ID, -32603, "Internal error: invalid tool schema", nil)
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
					s.sendErr(clientID, request.ID, -32602, "Missing required argument", map[string]string{"argument": reqFieldName})
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

		} else if params.Name == "greet" { //From main.go example
			name, _ := input["name"].(string)
			result.Content = []ToolResultContent{
				{Type: "text", Text: fmt.Sprintf("Hello, %s!", name)},
			}
		} else if params.Name == "another_tool" {
			value, _ := input["value"].(float64)
			result.Content = []ToolResultContent{{
				Type: "text",
				Text: fmt.Sprintf("The value is %f", value),
			}}

		}
		s.sendResp(clientID, request.ID, result, nil)

	default:
		s.sendErr(clientID, request.ID, -32601, "Method not found", nil)
	}
}

// handleNotification handles incoming notifications.  Common to both server types.
func (s *CommonServer) handleNotification(clientID string, notification *Notification) {
	s.logger.Printf("Received notification from client %s: method=%s", clientID, notification.Method)
	switch notification.Method {
	case "notifications/initialized": // The client confirms it's initialized.
		// We don't need to *do* anything here, but it's good to log.
		s.logger.Printf("Client %s initialized.", clientID)
	case "notifications/cancelled":
		var cancelParams struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if err := json.Unmarshal(notification.Params, &cancelParams); err == nil {
			s.logger.Printf("Cancellation requested for ID %s from client %s: %s", string(cancelParams.RequestID), clientID, cancelParams.Reason)
			// In a real implementation, you'd have a way to map request IDs to
			// ongoing operations and cancel them.  This is a placeholder.
		}

	default:
		s.logger.Printf("Unhandled notification from client %s: %s", clientID, notification.Method) // Log but don't send error.
	}
}
