package goai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/xeipuuv/gojsonschema"
)

const (
	ProtocolVersion   = "2024-11-05"
	defaultServerName = "goai-server"
	serverVersion     = "0.1.0"
)

// ServerConfig holds all configuration for BaseServer
type ServerConfig struct {
	logger           Logger
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
func UseLogger(logger Logger) ServerConfigOption {
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
	logger             Logger
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
		logger:          NewDefaultLogger(),
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
func (s *BaseServer) handleRequest(ctx context.Context, clientID string, request *Request) {
	s.logger.WithFields(map[string]interface{}{
		"clientID": clientID,
		"method":   request.Method,
		"id":       request.ID,
	}).Debug("Received request from client")

	switch request.Method {
	case "initialize":
		s.handleInitialize(ctx, clientID, request)
	case "ping":
		s.handlePing(clientID, request)
	case "resources/list":
		s.handleResourcesList(ctx, clientID, request)
	case "resources/read":
		s.handleResourcesRead(ctx, clientID, request)
	case "logging/setLevel":
		s.handleLoggingSetLevel(ctx, clientID, request)
	case "tools/list":
		s.handleToolsList(ctx, clientID, request)
	case "tools/call":
		s.handleToolsCall(ctx, clientID, request)
	case "prompts/list":
		s.handlePromptsList(ctx, clientID, request)
	case "prompts/get":
		s.handlePromptGet(ctx, clientID, request)

	default:
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"method":   request.Method,
			"id":       request.ID,
		}).Warn("Method not found. Unhandled request from client")

		s.sendErr(clientID, request.ID, -32601, "Method not found", nil)
	}
}

func (s *BaseServer) handleInitialize(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handleInitialize")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params InitializeParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithErr(err).Error("Failed to parse initialize params")
		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	if !strings.HasPrefix(params.ProtocolVersion, "2024-11") {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"version":  params.ProtocolVersion,
		}).Error("Unsupported protocol version")
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

func (s *BaseServer) handleResourcesList(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handleResourcesList")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params ListParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithErr(err).Error("Failed to parse list resources params")
		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	result := s.ListResources(ctx, params.Cursor, 0)
	span.SetAttributes(attribute.Int("num_resources", len(result.Resources)))

	s.sendResp(clientID, request.ID, result, nil)
}

// ListResources returns a list of all resources, with optional pagination.
func (s *BaseServer) ListResources(ctx context.Context, cursor string, limit int) ListResourcesResult {
	ctx, span := StartSpan(ctx, "BaseServer.ListResources")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	if limit <= 0 {
		limit = 50
	}

	span.SetAttributes(
		attribute.Int("limit", limit),
		attribute.String("cursor", cursor),
		attribute.Int("num_resources", len(s.resources)),
	)

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

func (s *BaseServer) handleResourcesRead(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handleResourcesRead")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params ReadResourceParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"params":   string(request.Params),
		}).WithErr(err).Error("Failed to parse read resource params")

		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	if _, exists := s.resources[params.URI]; !exists {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"uri":      params.URI,
		}).Error("Resource not found")

		s.sendErr(clientID, request.ID, -32002, "Resource not found",
			map[string]string{"uri": params.URI})
		return
	}

	result, err := s.ReadResource(ctx, params)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"uri":      params.URI,
		}).WithErr(err).Error("Failed to read resource")

		s.sendErr(clientID, request.ID, -32603, "Failed to read resource",
			map[string]string{"uri": params.URI})
		return
	}

	s.logger.WithFields(map[string]interface{}{
		"clientID": clientID,
		"request":  request.ID,
		"uri":      params.URI,
	}).Debug("Resource read successfully")

	s.sendResp(clientID, request.ID, result, nil)
}

// ReadResource implementation with proper error handling and URI validation
func (s *BaseServer) ReadResource(ctx context.Context, params ReadResourceParams) (ReadResourceResult, error) {
	ctx, span := StartSpan(ctx, "BaseServer.ReadResource")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	if !isValidURIScheme(params.URI) {
		return ReadResourceResult{}, fmt.Errorf("invalid URI scheme: %s", params.URI)
	}

	resource, exists := s.resources[params.URI]
	if !exists {
		s.logger.WithFields(map[string]interface{}{
			"uri": params.URI,
		}).Error("Resource not found")

		return ReadResourceResult{}, fmt.Errorf("resource not found: %s", params.URI)
	}

	content := ResourceContent{
		URI:      resource.URI,
		MimeType: resource.MimeType,
	}

	span.SetAttributes(
		attribute.String("uri", resource.URI),
		attribute.String("mime_type", resource.MimeType),
	)

	if strings.HasPrefix(resource.MimeType, "text/") {
		content.Text = resource.TextContent
	} else {
		content.Blob = base64.StdEncoding.EncodeToString([]byte(resource.TextContent))
	}

	return ReadResourceResult{
		Contents: []ResourceContent{content},
	}, nil
}

func (s *BaseServer) handleLoggingSetLevel(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handleLoggingSetLevel")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params SetLogLevelParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"params":   string(request.Params),
		}).WithErr(err).Error("Failed to parse set log level params")

		s.sendErr(clientID, request.ID, -32602, "Invalid Params", nil)
		return
	}
	if _, ok := logLevelSeverity[params.Level]; !ok {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"level":    params.Level,
		}).Error("Invalid log level")

		s.sendErr(clientID, request.ID, -32602, "Invalid log level", nil)
		return
	}

	s.minLogLevel = params.Level
	s.sendResp(clientID, request.ID, struct{}{}, nil)
}

func (s *BaseServer) handleToolsList(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handleToolsList")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params ListParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"params":   string(request.Params),
		}).WithErr(err).Error("Failed to parse list tools params")

		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	s.logger.WithFields(map[string]interface{}{
		"clientID": clientID,
		"request":  request.ID,
		"cursor":   params.Cursor,
		"limit":    100,
	}).Debug("Listing tools")

	span.SetAttributes(attribute.Int("limit", 100), attribute.String("cursor", params.Cursor))

	s.sendResp(clientID, request.ID, s.ListTools(ctx, params.Cursor, 100), nil)
}

func (s *BaseServer) handleToolsCall(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handleToolsCall")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params CallToolParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"params":   string(request.Params),
		}).WithErr(err).Error("Failed to parse call tool params")

		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	s.logger.WithFields(map[string]interface{}{
		"clientID": clientID,
		"request":  request.ID,
		"tool":     params.Name,
	}).Debug("Calling tool")

	span.SetAttributes(
		attribute.String("tool", params.Name),
	)

	result, err := s.CallTool(ctx, params)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"tool":     params.Name,
		}).WithErr(err).Error("Failed to call tool")

		s.sendErr(clientID, request.ID, -32602, err.Error(), nil)
		return
	}

	s.sendResp(clientID, request.ID, result, nil)
}

// ListPrompts returns a list of all available prompts, with optional pagination
func (s *BaseServer) ListPrompts(ctx context.Context, cursor string, limit int) ListPromptsResult {
	ctx, span := StartSpan(ctx, "BaseServer.ListPrompts")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	if limit <= 0 {
		limit = 50
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

	span.SetAttributes(
		attribute.Int("limit", limit),
		attribute.String("cursor", cursor),
		attribute.Int("num_prompts", len(pagePrompts)),
	)

	return ListPromptsResult{
		Prompts:    pagePrompts,
		NextCursor: nextCursor,
	}
}

func (s *BaseServer) handlePromptsList(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handlePromptsList")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params ListParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"params":   string(request.Params),
		}).WithErr(err).Error("Failed to parse list prompts params")

		s.sendErr(clientID, request.ID, -32700, "Failed to parse params", err)
		return
	}

	result := s.ListPrompts(ctx, params.Cursor, 100)
	span.SetAttributes(attribute.Int("num_prompts", len(result.Prompts)))

	s.sendResp(clientID, request.ID, result, nil)
}

func (s *BaseServer) handlePromptGet(ctx context.Context, clientID string, request *Request) {
	ctx, span := StartSpan(ctx, "BaseServer.handlePromptGet")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var params GetPromptParams
	if err = json.Unmarshal(request.Params, &params); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"params":   string(request.Params),
		}).WithErr(err).Error("Failed to parse get prompt params")

		s.sendErr(clientID, request.ID, -32602, "Invalid params", nil)
		return
	}

	prompt, exists := s.prompts[params.Name]
	if !exists {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"prompt":   params.Name,
		}).Error("Prompt not found")

		s.sendErr(clientID, request.ID, -32602, "Prompt not found",
			map[string]string{"prompt": params.Name})
		return
	}

	processedPrompt, err := processPrompt(prompt, params.Arguments)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"request":  request.ID,
			"prompt":   params.Name,
		}).WithErr(err).Error("Failed to process prompt")

		s.sendErr(clientID, request.ID, -32603, "Failed to process prompt", nil)
		return
	}

	promptResponse := PromptGetResponse{Description: processedPrompt.Description, Messages: processedPrompt.Messages}
	span.SetAttributes(
		attribute.String("prompt", params.Name),
		attribute.Int("total_messages", len(processedPrompt.Messages)),
	)

	s.logger.WithFields(map[string]interface{}{
		"clientID": clientID,
		"request":  request.ID,
		"prompt":   params.Name,
	}).Debug("Sending prompt response")

	s.sendResp(clientID, request.ID, promptResponse, nil)
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

// handleNotification handles incoming notifications.  Common to both server types.
func (s *BaseServer) handleNotification(ctx context.Context, clientID string, notification *Notification) {
	ctx, span := StartSpan(ctx, "BaseServer.handleNotification")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	//s.logger.Printf("Received notification from client %s: method=%s", clientID, notification.Method)
	s.logger.WithFields(map[string]interface{}{
		"clientID": clientID,
		"method":   notification.Method,
	}).Debug("Received notification from client")

	switch notification.Method {
	case "notifications/initialized":
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
		}).Debug("Client initialized")
	case "notifications/cancelled":
		var cancelParams struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if err = json.Unmarshal(notification.Params, &cancelParams); err == nil {
			s.logger.WithFields(map[string]interface{}{
				"clientID":  clientID,
				"requestID": cancelParams.RequestID,
				"reason":    cancelParams.Reason,
			}).Debug("Cancellation requested")
		}

	default:
		s.logger.WithFields(map[string]interface{}{
			"clientID": clientID,
			"method":   notification.Method,
		}).Warn("Unhandled notification from client")

	}
}

func (s *BaseServer) ListTools(ctx context.Context, cursor string, limit int) ListToolsResult {
	ctx, span := StartSpan(ctx, "BaseServer.ListTools")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

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

	span.SetAttributes(
		attribute.Int("limit", limit),
		attribute.String("cursor", cursor),
		attribute.Int("num_tools", len(pageTools)),
	)

	return ListToolsResult{
		Tools:      pageTools,
		NextCursor: nextCursor,
	}
}

func (s *BaseServer) CallTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	ctx, span := StartSpan(ctx, "BaseServer.CallTool")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	if _, exists := s.tools[params.Name]; !exists {
		s.logger.WithFields(map[string]interface{}{
			"tool": params.Name,
		}).Error("Tool not found")

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
			s.logger.WithErr(err).Error("Schema validation error")
			return CallToolResult{}, fmt.Errorf("validation error")
		}

		if !result.Valid() {
			var errorMessages []string
			for _, desc := range result.Errors() {
				errorMessages = append(errorMessages, desc.String())
			}

			s.logger.WithFields(map[string]interface{}{
				"tool":   params.Name,
				"errors": errorMessages,
			}).Error("Schema validation failed")

			return CallToolResult{
				IsError: true,
				Content: []ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Schema validation failed: %s", strings.Join(errorMessages, "; ")),
				}},
			}, nil
		}
	}

	result, err := s.tools[params.Name].Handler(ctx, params)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"tool": params.Name,
		}).WithErr(err).Error("Tool handler failed with an error")

		return CallToolResult{
			IsError: true,
			Content: []ToolResultContent{{
				Type: "text",
				Text: err.Error(),
			}},
		}, nil
	}

	span.SetAttributes(
		attribute.String("tool", params.Name),
		attribute.Int("contents_length", len(result.Content)),
	)

	s.logger.WithFields(map[string]interface{}{
		"tool": params.Name,
	}).Debug("Tool handler executed successfully")

	return result, nil
}
