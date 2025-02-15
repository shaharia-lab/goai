package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/xeipuuv/gojsonschema"
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
	capabilities     Capabilities
	initialResources []Resource
	initialTools     []Tool
	initialPrompts   []Prompt
	prompts          *Prompt
	resources        []Resource
	sseServerPort    string
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

func UseCapabilities(capabilities Capabilities) ServerConfigOption {
	return func(c *ServerConfig) {
		c.capabilities = capabilities
	}
}

func UseSSEServerPort(port string) ServerConfigOption {
	return func(c *ServerConfig) {
		c.sseServerPort = port
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
	sseServerPort string
	capabilities  Capabilities
	minLogLevel   LogLevel
	tools         map[string]Tool
	prompts       map[string]Prompt
	resources     map[string]Resource

	supportsPromptListChanged bool
	supportsToolListChanged   bool

	sendResp func(clientID string, id *json.RawMessage, result interface{}, err *Error)
	sendErr  func(clientID string, id *json.RawMessage, code int, message string, data interface{})
	sendNoti func(clientID string, method string, params interface{})
}

// NewBaseServer creates a new BaseServer instance with the given options
func NewBaseServer(opts ...ServerConfigOption) (*BaseServer, error) {
	cfg := defaultConfig()

	for _, opt := range opts {
		opt(cfg)
	}

	s := &BaseServer{
		protocolVersion: cfg.protocolVersion,
		logger:          cfg.logger,
		ServerInfo: ServerInfo{
			Name:    cfg.serverName,
			Version: cfg.serverVersion,
		},
		capabilities:              cfg.capabilities,
		minLogLevel:               cfg.minLogLevel,
		supportsPromptListChanged: false,
		supportsToolListChanged:   false,
		sendNoti:                  func(clientID string, method string, params interface{}) {},
		tools:                     make(map[string]Tool),
		prompts:                   make(map[string]Prompt),
		resources:                 make(map[string]Resource),
		sseServerPort:             cfg.sseServerPort,
	}

	if len(s.tools) > 0 {
		s.SendToolListChangedNotification()
	}

	if s.supportsPromptListChanged {
		s.SendPromptListChangedNotification()
	}

	return s, nil
}

func (s *BaseServer) AddTools(tools ...Tool) error {
	for _, tool := range tools {
		if _, exists := s.tools[tool.Name]; exists {
			return fmt.Errorf("duplicate tool: %s", tool.Name)
		}

		err := validateToolV2(tool)
		if err != nil {
			return fmt.Errorf("invalid tool: %v", err)
		}

		s.tools[tool.Name] = tool
	}

	return nil
}

func (s *BaseServer) AddPrompts(prompts ...Prompt) error {
	for _, prompt := range prompts {
		if _, exists := s.prompts[prompt.Name]; exists {
			return fmt.Errorf("duplicate prompt: %s", prompt.Name)
		}

		err := validatePrompt(prompt)
		if err != nil {
			return fmt.Errorf("invalid prompt: %v", err)
		}

		s.prompts[prompt.Name] = prompt
	}

	return nil
}

func (s *BaseServer) AddResources(resources ...Resource) error {
	for _, resource := range resources {
		if _, exists := s.resources[resource.URI]; exists {
			return fmt.Errorf("duplicate resource: %s", resource.URI)
		}

		err := validateResource(resource)
		if err != nil {
			return fmt.Errorf("invalid resource: %v", err)
		}

		s.resources[resource.URI] = resource
	}

	return nil
}

func defaultConfig() *ServerConfig {
	return &ServerConfig{
		logger:          log.Default(),
		protocolVersion: ProtocolVersion,
		serverName:      defaultServerName,
		serverVersion:   serverVersion,
		sseServerPort:   ":8080",
		minLogLevel:     LogLevelInfo,
		capabilities: Capabilities{
			Resources: CapabilitiesResources{
				ListChanged: true,
				Subscribe:   true,
			},
			Logging: CapabilitiesLogging{},
			Tools: CapabilitiesTools{
				ListChanged: true,
			},
			Prompts: CapabilitiesPrompts{
				ListChanged: true,
			},
		},
	}
}

// SendPromptListChangedNotification sends a notification that the prompt list has changed.
func (s *BaseServer) SendPromptListChangedNotification() {
	s.sendNoti("", "notifications/prompts/list_changed", nil)
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
	s.sendNoti("", "notifications/message", params)
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

	result := s.ListResources(params.Cursor, 0)
	s.sendResp(clientID, request.ID, result, nil)
}

// ListResources returns a list of all resources, with optional pagination.
func (s *BaseServer) ListResources(cursor string, limit int) ListResourcesResult {
	if limit <= 0 {
		limit = 50
	}

	result := ListResourcesResult{
		Resources: []Resource{},
	}

	if len(s.resources) == 0 {
		return result
	}

	var uris []string
	for uri := range s.resources {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	startIdx := 0
	if cursor != "" {
		cursorFound := false
		for i, uri := range uris {
			if uri == cursor {
				startIdx = i + 1
				cursorFound = true
				break
			}
		}
		if !cursorFound {
			return result
		}
	}

	if startIdx >= len(uris) {
		return result
	}

	endIdx := startIdx + limit
	if endIdx > len(uris) {
		endIdx = len(uris)
	}

	for i := startIdx; i < endIdx; i++ {
		result.Resources = append(result.Resources, s.resources[uris[i]])
	}

	if endIdx < len(uris) {
		result.NextCursor = uris[endIdx-1]
	}

	return result
}

func (s *BaseServer) handleResourcesRead(clientID string, request *Request) {
	var params ReadResourceParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	if _, exists := s.resources[params.URI]; !exists {
		s.sendErr(clientID, request.ID, -32002, "Resource not found",
			map[string]string{"uri": params.URI})
		return
	}

	result, err := s.ReadResource(params)
	if err != nil {
		s.sendErr(clientID, request.ID, -32603, "Failed to read resource",
			map[string]string{"uri": params.URI})
		return
	}

	s.sendResp(clientID, request.ID, result, nil)
}

// ReadResource implementation with proper error handling and URI validation
func (s *BaseServer) ReadResource(params ReadResourceParams) (ReadResourceResult, error) {
	if !isValidURIScheme(params.URI) {
		return ReadResourceResult{}, fmt.Errorf("invalid URI scheme: %s", params.URI)
	}

	resource, exists := s.resources[params.URI]
	if !exists {
		return ReadResourceResult{}, fmt.Errorf("resource not found: %s", params.URI)
	}

	content := ResourceContent{
		URI:      resource.URI,
		MimeType: resource.MimeType,
	}

	if strings.HasPrefix(resource.MimeType, "text/") {
		content.Text = resource.TextContent
	} else {
		content.Blob = base64.StdEncoding.EncodeToString([]byte(resource.TextContent))
	}

	return ReadResourceResult{
		Contents: []ResourceContent{content},
	}, nil
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

	s.sendResp(clientID, request.ID, s.ListTools(params.Cursor, 1), nil)
}

func (s *BaseServer) handleToolsCall(clientID string, request *Request) {
	var params CallToolParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	result, err := s.CallTool(params)
	if err != nil {
		s.sendErr(clientID, request.ID, -32602, err.Error(), nil)
		return
	}

	s.sendResp(clientID, request.ID, result, nil)
}

// ListPrompts returns a list of all available prompts, with optional pagination
func (s *BaseServer) ListPrompts(cursor string, limit int) ListPromptsResult {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	// Get sorted list of prompt names
	var names []string
	for name := range s.prompts {
		names = append(names, name)
	}
	sort.Strings(names)

	// Find starting index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, name := range names {
			if name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Calculate end index
	endIdx := startIdx + limit
	if endIdx > len(names) {
		endIdx = len(names)
	}

	// Get the page of prompts
	pagePrompts := make([]Prompt, 0)
	for i := startIdx; i < endIdx; i++ {
		if prompt, exists := s.prompts[names[i]]; exists {
			pagePrompts = append(pagePrompts, prompt)
		}
	}

	// Set next cursor if there are more items
	var nextCursor string
	if endIdx < len(names) {
		nextCursor = names[endIdx]
	}

	return ListPromptsResult{
		Prompts:    pagePrompts,
		NextCursor: nextCursor,
	}
}

func (s *BaseServer) handlePromptsList(clientID string, request *Request) {
	var params ListParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	result := s.ListPrompts(params.Cursor, 0)
	s.sendResp(clientID, request.ID, result, nil)
}

func (s *BaseServer) handlePromptGet(clientID string, request *Request) {
	var params GetPromptParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	prompt, exists := s.prompts[params.Name]
	if !exists {
		s.sendErr(clientID, request.ID, -32602, "Prompt not found",
			map[string]string{"prompt": params.Name})
		return
	}

	processedPrompt, err := processPrompt(prompt, params.Arguments)
	if err != nil {
		s.sendErr(clientID, request.ID, -32603, "Failed to process prompt", nil)
		return
	}

	s.sendResp(clientID, request.ID, PromptGetResponse{Description: processedPrompt.Description, Messages: processedPrompt.Messages}, nil)
}

// Helper functions
func (s *BaseServer) updateSupportedCapabilities() {
	if s.capabilities.Prompts.ListChanged {
		s.supportsPromptListChanged = true
	}
	if s.capabilities.Tools.ListChanged {
		s.supportsToolListChanged = true
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

		s.sendResp(clientID, request.ID, s.ListPrompts(params.Cursor, 0), nil)

	case "prompt/get":
		var getParams GetPromptParams
		if err := json.Unmarshal(request.Params, &getParams); err != nil {
			s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
			return
		}

		prompt, exists := s.prompts[getParams.Name]
		if !exists {
			s.sendErr(clientID, request.ID, -32602, "Prompt not found", map[string]string{"prompt": getParams.Name})
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

func (s *BaseServer) ListTools(cursor string, limit int) ListToolsResult {
	if limit <= 0 {
		limit = 50
	}

	// Create a map for easier tool lookup
	toolMap := make(map[string]Tool)
	var names []string
	for _, t := range s.tools {
		names = append(names, t.Name)
		toolMap[t.Name] = Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	sort.Strings(names)

	startIdx := 0
	if cursor != "" {
		// Find the cursor position
		for i, name := range names {
			if name == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(names) {
		endIdx = len(names)
	}

	// Get the page of tools
	pageTools := make([]Tool, 0)
	for i := startIdx; i < endIdx; i++ {
		if tool, exists := toolMap[names[i]]; exists {
			pageTools = append(pageTools, tool)
		}
	}

	var nextCursor string
	if endIdx < len(names) {
		nextCursor = names[endIdx] // Use the next item's name as cursor
	}

	return ListToolsResult{
		Tools:      pageTools,
		NextCursor: nextCursor,
	}
}

func (s *BaseServer) CallTool(params CallToolParams) (CallToolResult, error) {
	if _, exists := s.tools[params.Name]; !exists {
		return CallToolResult{}, fmt.Errorf("tool metadata not found: %s", params.Name)
	}

	if s.tools[params.Name].InputSchema != nil && len(params.Arguments) > 0 {
		schemaLoader := gojsonschema.NewStringLoader(string(s.tools[params.Name].InputSchema))

		argsJSON, err := json.Marshal(params.Arguments)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal arguments: %v", err)
		}

		documentLoader := gojsonschema.NewStringLoader(string(argsJSON))

		result, err := gojsonschema.Validate(schemaLoader, documentLoader)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("validation error: %v", err)
		}

		if !result.Valid() {
			var errorMessages []string
			for _, desc := range result.Errors() {
				errorMessages = append(errorMessages, desc.String())
			}

			return CallToolResult{
				IsError: true,
				Content: []ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Schema validation failed: %s", strings.Join(errorMessages, "; ")),
				}},
			}, nil
		}
	}

	result, err := s.tools[params.Name].Handler(context.Background(), params)
	if err != nil {
		return CallToolResult{
			IsError: true,
			Content: []ToolResultContent{{
				Type: "text",
				Text: err.Error(),
			}},
		}, nil
	}

	return result, nil
}
