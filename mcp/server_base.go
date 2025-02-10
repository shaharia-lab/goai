package mcp

import (
	"encoding/json"
	"log"
	"strings"
)

const (
	ProtocolVersion   = "2024-11-05"
	defaultServerName = "goai-mcp-server"
	serverVersion     = "0.1.0"
)

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
	toolManager      *ToolManager
	promptManager    *PromptManager
	resourceManager  *ResourceManager
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
func UseResources(resourceManager *ResourceManager) ServerConfigOption {
	return func(c *ServerConfig) {
		c.resourceManager = resourceManager
	}
}

// UseTools sets initial tools
func UseTools(toolManager *ToolManager) ServerConfigOption {
	return func(c *ServerConfig) {
		c.toolManager = toolManager
	}
}

// UsePrompts sets initial prompts
func UsePrompts(promptManager *PromptManager) ServerConfigOption {
	return func(c *ServerConfig) {
		c.promptManager = promptManager
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
	capabilities    map[string]any
	resourceManager *ResourceManager
	minLogLevel     LogLevel
	toolManager     *ToolManager
	promptManager   *PromptManager

	supportsPromptListChanged bool
	supportsToolListChanged   bool

	// Abstract send methods.
	sendResp func(clientID string, id *json.RawMessage, result interface{}, err *Error)
	sendErr  func(clientID string, id *json.RawMessage, code int, message string, data interface{})
	sendNoti func(clientID string, method string, params interface{})
}

// NewBaseServer creates a new BaseServer instance with the given options
func NewBaseServer(opts ...ServerConfigOption) (*BaseServer, error) {
	// Get default configuration
	cfg := defaultConfig()

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	// Create server instance
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
		resourceManager:           cfg.resourceManager,
		minLogLevel:               cfg.minLogLevel,
		toolManager:               cfg.toolManager,
		promptManager:             cfg.promptManager,
		supportsPromptListChanged: false,
		supportsToolListChanged:   false,
		sendNoti:                  func(clientID string, method string, params interface{}) {},
	}

	// Initialize notifications if needed
	if tools := cfg.toolManager.ListTools("", 2); tools.Tools != nil && len(tools.Tools) > 0 {
		s.SendToolListChangedNotification()
	}

	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}

	return s, nil
}

func defaultConfig() *ServerConfig {
	tm, _ := NewToolManager([]ToolHandler{})
	pm, _ := NewPromptManager([]Prompt{})
	rm, _ := NewResourceManager([]Resource{})

	return &ServerConfig{
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
		toolManager:     tm,
		promptManager:   pm,
		resourceManager: rm,
	}
}

// SendPromptListChangedNotification sends a notification that the prompt list has changed.
func (s *BaseServer) SendPromptListChangedNotification() {
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

// handleRequest handles incoming requests.  Common to both server types.
func (s *BaseServer) handleRequest(clientID string, request *Request) {
	s.logger.Printf("Received request from client %s: method=%s, id=%v", clientID, request.Method, request.ID)

	switch request.Method {
	case "initialize":
		s.handleInitialize(clientID, request)
	case "ping":
		s.handlePing(clientID, request)
	case "resources/list":
		s.handleResourcesList(clientID, request)
	case "resources/read":
		s.handleResourcesRead(clientID, request)
	case "logging/setLevel":
		s.handleLoggingSetLevel(clientID, request)
	case "tools/list":
		s.handleToolsList(clientID, request)
	case "tools/call":
		s.handleToolsCall(clientID, request)
	case "prompts/list":
		s.handlePromptsList(clientID, request)
	case "prompts/get":
		s.handlePromptGet(clientID, request)

	default:
		s.sendErr(clientID, request.ID, -32601, "Method not found", nil)
	}
}

func (s *BaseServer) handleInitialize(clientID string, request *Request) {
	var params InitializeParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	if !strings.HasPrefix(params.ProtocolVersion, "2024-11") {
		s.sendErr(clientID, request.ID, -32602, "Unsupported protocol version",
			map[string][]string{"supported": {"2024-11-05"}})
		return
	}

	s.clientCapabilities = params.Capabilities
	result := InitializeResult{
		ProtocolVersion: s.protocolVersion,
		Capabilities:    s.capabilities,
		ServerInfo:      s.ServerInfo,
	}

	s.updateSupportedCapabilities()
	s.sendResp(clientID, request.ID, result, nil)
}

func (s *BaseServer) handlePing(clientID string, request *Request) {
	s.sendResp(clientID, request.ID, map[string]interface{}{}, nil)
}

func (s *BaseServer) handleResourcesList(clientID string, request *Request) {
	var params ListParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	result := s.resourceManager.ListResources(params.Cursor, 0)
	s.sendResp(clientID, request.ID, result, nil)
}

func (s *BaseServer) handleResourcesRead(clientID string, request *Request) {
	var params ReadResourceParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	_, err := s.resourceManager.GetResource(params.URI)
	if err != nil {
		s.sendErr(clientID, request.ID, -32002, "Resource not found",
			map[string]string{"uri": params.URI})
		return
	}

	result, err := s.resourceManager.ReadResource(params)
	if err != nil {
		s.sendErr(clientID, request.ID, -32603, "Failed to read resource",
			map[string]string{"uri": params.URI})
		return
	}

	s.sendResp(clientID, request.ID, result, nil)
}

func (s *BaseServer) handleLoggingSetLevel(clientID string, request *Request) {
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
}

func (s *BaseServer) handleToolsList(clientID string, request *Request) {
	var params ListParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	s.logger.Printf("Total tools: %+v", s.toolManager.ListTools(params.Cursor, 0))
	s.sendResp(clientID, request.ID, s.toolManager.ListTools(params.Cursor, 0), nil)
}

func (s *BaseServer) handleToolsCall(clientID string, request *Request) {
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
}

func (s *BaseServer) handlePromptsList(clientID string, request *Request) {
	var params ListParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	result := s.promptManager.ListPrompts(params.Cursor, 0)
	s.sendResp(clientID, request.ID, result, nil)
}

func (s *BaseServer) handlePromptGet(clientID string, request *Request) {
	var params GetPromptParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	prompt, err := s.promptManager.GetPrompt(params)
	if err != nil {
		s.sendErr(clientID, request.ID, -32602, "Prompt not found",
			map[string]string{"prompt": params.Name})
		return
	}

	s.sendResp(clientID, request.ID, prompt, nil)
}

// Helper functions
func (s *BaseServer) updateSupportedCapabilities() {
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
}

func (s *BaseServer) handlePromptRequest(clientID string, request *Request) {
	switch request.Method {
	case "prompts/list":
		var params ListParams

		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
			return
		}

		s.sendResp(clientID, request.ID, s.promptManager.ListPrompts(params.Cursor, 0), nil)

	case "prompt/get":
		var getParams GetPromptParams
		if err := json.Unmarshal(request.Params, &getParams); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		prompt, err := s.promptManager.GetPrompt(getParams)
		if err != nil {
			s.sendErr(clientID, request.ID, -32602, "Prompt not found", map[string]string{"prompt": prompt.Name})
			return
		}

		s.sendResp(clientID, request.ID, prompt, nil)

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
