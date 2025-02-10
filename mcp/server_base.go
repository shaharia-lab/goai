package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

const (
	ProtocolVersion   = "2024-11-05"
	defaultServerName = "goai-mcp-server"
	serverVersion     = "0.1.0"
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
	sendNotification(clientID string, method string, params interface{})
}

// ServerConfig holds all configuration for BaseServer
type ServerConfig struct {
	logger           *log.Logger
	protocolVersion  string
	serverName       string
	serverVersion    string
	minLogLevel      LogLevel
	capabilities     map[string]any
	initialResources []Resource
	initialTools     []Tool
	initialPrompts   []Prompt
	toolManager      ToolManager
	prompts          map[string]Prompt
	resources        map[string]Resource
}

// ServerConfigOption is a function that modifies ServerConfig
type ServerConfigOption func(*ServerConfig)

// UseLogger sets a custom logger
func UseLogger(logger *log.Logger) ServerConfigOption {
	return func(c *ServerConfig) {
		c.logger = logger
	}
}

// UseServerInfo sets server name and version
func UseServerInfo(name, version string) ServerConfigOption {
	return func(c *ServerConfig) {
		c.serverName = name
		c.serverVersion = version
	}
}

// UseLogLevel sets minimum log level
func UseLogLevel(level LogLevel) ServerConfigOption {
	return func(c *ServerConfig) {
		c.minLogLevel = level
	}
}

// UseResources sets initial resources
func UseResources(resources ...Resource) ServerConfigOption {
	return func(c *ServerConfig) {
		for _, resource := range resources {
			c.resources[resource.URI] = resource
		}
	}
}

// UseToolManager sets initial tools
func UseToolManager(toolManager ToolManager) ServerConfigOption {
	return func(c *ServerConfig) {
		c.toolManager = toolManager
	}
}

// UsePrompts sets initial prompts
func UsePrompts(prompts ...Prompt) ServerConfigOption {
	return func(c *ServerConfig) {
		for _, prompt := range prompts {
			c.prompts[prompt.Name] = prompt
		}
	}
}

// BaseServer contains the common fields and methods for all MCP server implementations.
type BaseServer struct {
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
	toolManager               ToolManager
	prompts                   map[string]Prompt
	supportsPromptListChanged bool
	supportsToolListChanged   bool

	// Abstract send methods.
	sendResp func(clientID string, id *json.RawMessage, result interface{}, err *Error)
	sendErr  func(clientID string, id *json.RawMessage, code int, message string, data interface{})
	sendNoti func(clientID string, method string, params interface{})
}

// NewServerBuilder creates a new BaseServer instance Use the given options
func NewServerBuilder(opts ...ServerConfigOption) *BaseServer {
	// Default configuration
	cfg := &ServerConfig{
		logger:          log.Default(),
		protocolVersion: ProtocolVersion,
		serverName:      defaultServerName,
		serverVersion:   serverVersion,
		minLogLevel:     LogLevelInfo,
		capabilities: map[string]any{
			"resources": map[string]any{
				"listChanged": true,
				"subscribe":   true,
			},
			"logging": map[string]any{},
			"tools": map[string]any{
				"listChanged": true,
			},
			"prompts": map[string]any{
				"listChanged": true,
			},
		},
		toolManager: NewToolManager([]ToolHandler{}),
		prompts:     make(map[string]Prompt),
		resources:   make(map[string]Resource),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	s := &BaseServer{
		protocolVersion: cfg.protocolVersion,
		logger:          cfg.logger,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    cfg.serverName,
			Version: cfg.serverVersion,
		},
		capabilities:              cfg.capabilities,
		resources:                 cfg.resources,
		minLogLevel:               cfg.minLogLevel,
		toolManager:               cfg.toolManager,
		prompts:                   cfg.prompts,
		supportsPromptListChanged: false,
		supportsToolListChanged:   false,
		sendNoti:                  func(clientID string, method string, params interface{}) {},
	}

	if tools := cfg.toolManager.ListTools("", 2); tools.Tools != nil && len(tools.Tools) > 0 {
		s.SendToolListChangedNotification()
	}

	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}

	return s
}

// AddResource adds a new resource to the server.
func (s *BaseServer) AddResource(resource Resource) {
	s.resources[resource.URI] = resource
}

// AddPrompt adds a new prompt to the server.
func (s *BaseServer) AddPrompt(prompt Prompt) {
	s.prompts[prompt.Name] = prompt
	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}
}

// DeletePrompt removes a prompt from the server.
func (s *BaseServer) DeletePrompt(name string) error {
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
func (s *BaseServer) SendPromptListChangedNotification() {
	// The concrete server implementations will handle broadcasting this.
	s.sendNoti("", "notifications/prompts/list_changed", nil) // Empty clientID.  Common server doesn't know *who* to send to
}

// SendToolListChangedNotification sends a notification that the tool list has changed.
func (s *BaseServer) SendToolListChangedNotification() {
	s.sendNoti("", "notifications/tools/list_changed", nil)
}

// LogMessage logs a message.
func (s *BaseServer) LogMessage(level LogLevel, loggerName string, data interface{}) {
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
func (s *BaseServer) handleRequest(clientID string, request *Request) {
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
		var promptList = make([]Prompt, 0)
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
		var params ListToolsParams

		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
			return
		}

		log.Printf("Total tools: %+v", s.toolManager.ListTools(params.Cursor, 0))

		s.sendResp(clientID, request.ID, s.toolManager.ListTools(params.Cursor, 0), nil)

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		result, err := s.toolManager.CallTool(params)
		if err != nil {
			s.sendErr(clientID, request.ID, -32602, err.Error(), nil)
			return
		}

		s.sendResp(clientID, request.ID, result, nil)

	default:
		s.sendErr(clientID, request.ID, -32601, "Method not found", nil)
	}
}

// handleNotification handles incoming notifications.  Common to both server types.
func (s *BaseServer) handleNotification(clientID string, notification *Notification) {
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
