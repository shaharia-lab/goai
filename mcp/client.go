package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/shaharia-lab/goai/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	defaultBaseRetryDelay  = 5 * time.Second
	defaultMaxRetryDelay   = 30 * time.Second
	defaultMaxRetries      = 5
	defaultMaxMissedPings  = 2
	defaultClientName      = "mcp-client"
	defaultClientVersion   = "1.0.0"
	defaultMessageEndpoint = ""
	defaultRequestTimeout  = 30 * time.Second
)

type ConnectionState int

const (
	Disconnected ConnectionState = iota
	Connecting
	Connected
)

type SSEConfig struct {
	URL string
}

type StdIOConfig struct {
	Reader io.Reader
	Writer io.Writer
}

type ClientConfig struct {
	RetryDelay      time.Duration
	MaxRetries      int
	ClientName      string
	ClientVersion   string
	Logger          observability.Logger
	SSE             SSEConfig
	StdIO           StdIOConfig
	MessageEndpoint string
	RequestTimeout  time.Duration
}

type Transport interface {
	Connect(ctx context.Context, config ClientConfig) error
	SendMessage(ctx context.Context, message interface{}) error
	Close(ctx context.Context) error
	SetReceiveMessageCallback(callback func(message []byte))
}

type Client struct {
	transport        Transport
	config           ClientConfig
	logger           observability.Logger
	state            ConnectionState
	initialized      bool
	capabilities     Capabilities
	protocolVersion  string
	mu               sync.RWMutex
	responseHandlers map[string]chan *Response
	wg               sync.WaitGroup
}

func NewClient(transport Transport, config ClientConfig) *Client {
	if config.Logger == nil {
		config.Logger = observability.NewDefaultLogger()
	}

	if config.ClientName == "" {
		config.ClientName = defaultClientName
	}
	if config.ClientVersion == "" {
		config.ClientVersion = defaultClientVersion
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = defaultBaseRetryDelay
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = defaultRequestTimeout
	}

	config.Logger = config.Logger.WithFields(map[string]interface{}{
		"client_name":    config.ClientName,
		"client_version": config.ClientVersion,
		"server_url":     config.SSE.URL,
	})

	client := &Client{
		transport:        transport,
		config:           config,
		logger:           config.Logger,
		state:            Disconnected,
		capabilities:     Capabilities{},
		responseHandlers: make(map[string]chan *Response),
	}

	transport.SetReceiveMessageCallback(client.processReceivedMessage)
	return client
}

func (c *Client) registerResponseHandler(id string, ch chan *Response) {
	c.mu.Lock()
	c.responseHandlers[id] = ch
	c.mu.Unlock()
}

func (c *Client) removeResponseHandler(id string) {
	c.mu.Lock()
	delete(c.responseHandlers, id)
	c.mu.Unlock()
}

func (c *Client) Connect(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx, "Client.Connect")
	span.SetAttributes(
		attribute.String("client.name", c.config.ClientName),
		attribute.String("client.version", c.config.ClientVersion),
		attribute.String("server.url", c.config.SSE.URL),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	c.logger.Debug("Connecting to server...")

	c.mu.Lock()
	if c.state != Disconnected {
		c.mu.Unlock()
		c.logger.Error("Attempt to connect while already connected or in the process of connecting.")
		return fmt.Errorf("client is already connected or connecting")
	}
	c.state = Connecting
	c.mu.Unlock()

	if err = c.transport.Connect(ctx, c.config); err != nil {
		c.logger.WithErr(err).Error("Failed to connect to transport.")
		return fmt.Errorf("failed to connect to transport: %v", err)
	}

	if err = c.sendInitializeRequest(ctx); err != nil {
		c.logger.Error("Failed to send initialize request to server")
		return fmt.Errorf("sending initialize request failed: %v", err)
	}

	if err = c.sendInitializedNotification(ctx); err != nil {
		c.logger.WithErr(err).Error("Failed to send initialized notification to server")
		return fmt.Errorf("failed to send initialized notification: %v", err)
	}

	c.mu.Lock()
	c.state = Connected
	c.initialized = true
	c.logger.Debug("State changed to Connected and client initialized successfully.")
	c.mu.Unlock()

	c.logger.Info("Connected to server successfully")

	return nil
}

func (c *Client) processReceivedMessage(message []byte) {
	c.logger.WithFields(map[string]interface{}{"message_length": len(message)}).Debug("Processing received message")

	var response Response
	if err := json.Unmarshal(message, &response); err != nil {
		c.logger.WithErr(err).Error("Failed to parse response")
		return
	}

	if response.ID != nil {
		var idStr string
		if err := json.Unmarshal(*response.ID, &idStr); err != nil {
			c.logger.WithFields(map[string]interface{}{"request_id": *response.ID}).WithErr(err).Error("Failed to unmarshal ID")
			return
		}

		c.mu.RLock()
		ch, exists := c.responseHandlers[idStr]
		c.mu.RUnlock()

		if exists {
			select {
			case ch <- &response:
				c.logger.Debug("Response sent to handler")
			default:
				c.logger.WithFields(map[string]interface{}{"request_id": idStr}).Debug("Handler channel full")
			}
		} else {
			c.logger.WithFields(map[string]interface{}{"request_id": idStr}).Warn("No handler found for request ID")
		}
	}

	c.logger.Debug("Response ID is empty, ignoring")
}

func (c *Client) sendInitializeRequest(ctx context.Context) error {
	responseChan := make(chan *Response, 1)
	requestID := "init"
	rawID := json.RawMessage(`"init"`)

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)

	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]any{
			"tools": map[string]bool{
				"listChanged": true,
			},
			"prompts": map[string]bool{
				"listChanged": true,
			},
		},
		ClientInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    c.config.ClientName,
			Version: c.config.ClientVersion,
		},
	}

	request := Request{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      &rawID,
		Params:  nil,
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		c.wg.Done()
		c.logger.WithErr(err).Error("Failed to marshal initialize params")
		return fmt.Errorf("failed to marshal initialize params: %v", err)
	}
	request.Params = paramsBytes

	if err = c.transport.SendMessage(ctx, &request); err != nil {
		c.wg.Done()
		c.logger.WithErr(err).Error("Failed to send initialize request")
		return fmt.Errorf("failed to send initialize request: %v", err)
	}

	select {
	case genericResponse := <-responseChan:
		defer c.wg.Done()
		if genericResponse.Error != nil {
			c.logger.WithFields(map[string]interface{}{
				"error_message": genericResponse.Error.Message,
				"error_code":    genericResponse.Error.Code,
			}).Error("Server error during initialize request")
			return fmt.Errorf("server error: %s (code: %d)", genericResponse.Error.Message, genericResponse.Error.Code)
		}

		rawResult, err := json.Marshal(genericResponse.Result)
		if err != nil {
			c.logger.WithErr(err).Error("Failed to marshal result into raw message")
			c.logger.WithFields(map[string]interface{}{"result": genericResponse.Result}).Debug("Failed to marshal result into raw message")
			return fmt.Errorf("failed to marshal result into raw message")
		}

		var initResponse InitializeResponse
		if err := json.Unmarshal(rawResult, &initResponse); err != nil {
			c.logger.WithErr(err).Error("Failed to unmarshal raw message into initialize response")
			return fmt.Errorf("failed to unmarshal raw message into initialize response")
		}

		c.mu.Lock()
		c.capabilities = initResponse.Result.Capabilities
		c.protocolVersion = initResponse.Result.ProtocolVersion
		c.mu.Unlock()

		c.logger.Debug("Client received response for initialize request successfully")
		return nil

	case <-time.After(c.config.RequestTimeout):
		defer c.wg.Done()
		c.logger.WithFields(map[string]interface{}{"request_timeout": c.config.RequestTimeout.String()}).Error("Initialize request timeout")
		return fmt.Errorf("initialize request timeout")
	}
}

func (c *Client) sendInitializedNotification(ctx context.Context) error {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	c.logger.WithFields(map[string]interface{}{"method": "notifications/initialized"}).Debug("Sending initialized notification request to server...")
	return c.transport.SendMessage(ctx, &notification)
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	ctx, span := observability.StartSpan(ctx, "Client.ListTools")
	span.SetAttributes(
		attribute.String("client.name", c.config.ClientName),
		attribute.String("client.version", c.config.ClientVersion),
		attribute.String("server.url", c.config.SSE.URL),
		attribute.String("methods", "tools/list"),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		c.logger.Error("Request for tools/lists but client is not connected and initialized")
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	responseChan := make(chan *Response, 1)
	requestID := "tools-list"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)

	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      &rawID,
		Params:  json.RawMessage(`{"cursor":""}`),
	}

	if err = c.transport.SendMessage(ctx, &request); err != nil {
		c.wg.Done()
		c.logger.WithErr(err).Error("Failed to send tools/list request")
		return nil, fmt.Errorf("failed to send tools/list request: %v", err)
	}

	select {
	case response := <-responseChan:
		defer c.wg.Done()
		if response.Error != nil {
			c.logger.WithFields(map[string]interface{}{
				"error_message": response.Error.Message,
				"error_code":    response.Error.Code,
			}).Error("Server error during tools/list request")
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			c.logger.WithErr(err).Error("Failed to marshal result into raw message")
			return nil, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result ListToolsResult
		if err = json.Unmarshal(rawResult, &result); err != nil {
			c.logger.WithErr(err).Error("Failed to parse tools list")
			return nil, fmt.Errorf("failed to parse tools list: %v", err)
		}

		c.logger.WithFields(map[string]interface{}{
			"tools_count": len(result.Tools),
		}).Debug("Client received response for tools/list request successfully")

		span.SetAttributes(attribute.Int("tools.count", len(result.Tools)))

		return result.Tools, nil

	case <-time.After(c.config.RequestTimeout):
		defer c.wg.Done()
		c.logger.WithFields(map[string]interface{}{"request_timeout": c.config.RequestTimeout.String()}).Error("tools/list request timeout")
		return nil, fmt.Errorf("tools/list request timeout")
	}
}

func (c *Client) CallTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	ctx, span := observability.StartSpan(ctx, "Client.CallTool")
	span.SetAttributes(
		attribute.String("client.name", c.config.ClientName),
		attribute.String("client.version", c.config.ClientVersion),
		attribute.String("server.url", c.config.SSE.URL),
		attribute.String("tool.name", params.Name),
		attribute.String("methods", "tools/call"),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		c.logger.Error("Client is not connected and initialized")
		return CallToolResult{}, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	responseChan := make(chan *Response, 1)
	requestID := "tools-call"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)
	defer c.wg.Done()

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		c.logger.WithFields(map[string]interface{}{"tool_name": params.Name}).WithErr(err).Error("Failed to marshal CallToolParams")
		return CallToolResult{}, fmt.Errorf("failed to marshal CallToolParams")
	}

	span.SetAttributes(attribute.String("tool.params", string(paramsBytes)))

	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      &rawID,
		Params:  json.RawMessage(paramsBytes),
	}

	c.logger.WithFields(map[string]interface{}{
		"tool_name":   params.Name,
		"tool_params": paramsBytes,
	}).Debug("Sending tools/call request to server...")

	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	if err := c.transport.SendMessage(requestCtx, &request); err != nil {
		c.logger.WithFields(map[string]interface{}{"tool_name": params.Name}).WithErr(err).Error("Failed to send tools/call request")
		return CallToolResult{}, fmt.Errorf("failed to send tools/call request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			c.logger.WithFields(map[string]interface{}{
				"tool_name":     params.Name,
				"error_message": response.Error.Message,
				"error_code":    response.Error.Code,
			}).Error("Server error during tools/call request")

			return CallToolResult{}, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			c.logger.WithFields(map[string]interface{}{
				"tool_name": params.Name,
			}).WithErr(err).Error("Failed to marshal result into raw message")
			return CallToolResult{}, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result CallToolResult
		if err := json.Unmarshal(rawResult, &result); err != nil {
			c.logger.WithFields(map[string]interface{}{
				"tool_name": params.Name,
			}).WithErr(err).Error("Failed to parse tool result")

			return CallToolResult{}, fmt.Errorf("failed to parse tool result: %v", err)
		}
		c.logger.WithFields(map[string]interface{}{
			"tool_name": params.Name,
		}).Debug("Client received response for tools/call request successfully")

		return result, nil

	case <-requestCtx.Done():
		c.logger.WithFields(map[string]interface{}{
			"tool_name":       params.Name,
			"request_timeout": c.config.RequestTimeout.String(),
		}).Error("tools/call request timeout")

		return CallToolResult{}, fmt.Errorf("tools/call request timeout: %v", requestCtx.Err())
	}
}

func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	ctx, span := observability.StartSpan(ctx, "Client.ListPrompts")
	span.SetAttributes(
		attribute.String("client.name", c.config.ClientName),
		attribute.String("client.version", c.config.ClientVersion),
		attribute.String("server.url", c.config.SSE.URL),
		attribute.String("methods", "prompts/list"),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	responseChan := make(chan *Response, 1)
	requestID := "prompts-list"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)
	defer c.wg.Done()

	request := Request{
		JSONRPC: "2.0",
		Method:  "prompts/list",
		ID:      &rawID,
		Params:  json.RawMessage(`{"cursor":""}`),
	}

	// Create a timeout context for this request
	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	c.logger.Debug("Requesting prompts list...")
	if err = c.transport.SendMessage(requestCtx, &request); err != nil {
		c.logger.WithErr(err).Error("Failed to send prompts/list request")
		return nil, fmt.Errorf("failed to send prompts/list request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			c.logger.WithFields(map[string]interface{}{
				"error_message": response.Error.Message,
				"error_code":    response.Error.Code,
			}).Error("Server error during prompts/list request")
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			c.logger.WithErr(err).Error("Failed to marshal result into raw message")
			return nil, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result ListPromptsResult
		if err = json.Unmarshal(rawResult, &result); err != nil {
			c.logger.WithErr(err).Error("Failed to parse prompts list")
			return nil, fmt.Errorf("failed to parse prompts list: %v", err)
		}

		span.SetAttributes(attribute.Int("prompts.count", len(result.Prompts)))
		c.logger.WithFields(map[string]interface{}{
			"prompts_count": len(result.Prompts),
		}).Debug("Client received response for prompts/list request successfully")
		return result.Prompts, nil

	case <-requestCtx.Done():
		return nil, fmt.Errorf("prompts/list request timeout: %v", requestCtx.Err())
	}
}

func (c *Client) GetPrompt(ctx context.Context, params GetPromptParams) ([]PromptMessage, error) {
	ctx, span := observability.StartSpan(ctx, "Client.GetPrompt")
	span.SetAttributes(
		attribute.String("client.name", c.config.ClientName),
		attribute.String("client.version", c.config.ClientVersion),
		attribute.String("server.url", c.config.SSE.URL),
		attribute.String("methods", "prompts/get"),
		attribute.String("prompt.name", params.Name),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	c.mu.RLock()
	if c.state != Connected || !c.initialized {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is not connected and initialized")
	}
	c.mu.RUnlock()

	responseChan := make(chan *Response, 1)
	requestID := "prompts-get"
	rawID := json.RawMessage(fmt.Sprintf(`"%s"`, requestID))

	c.registerResponseHandler(requestID, responseChan)
	defer c.removeResponseHandler(requestID)

	c.wg.Add(1)
	defer c.wg.Done()

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		c.logger.WithFields(map[string]interface{}{
			"prompt_name": params.Name,
		}).WithErr(err).Error("Failed to marshal GetPromptParams")
		return nil, fmt.Errorf("failed to marshal GetPromptParams: %v", err)
	}

	request := Request{
		JSONRPC: "2.0",
		Method:  "prompts/get",
		ID:      &rawID,
		Params:  json.RawMessage(paramsBytes),
	}

	// Create a timeout context for this request
	requestCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	c.logger.WithFields(map[string]interface{}{
		"prompt_name":   params.Name,
		"prompt_params": paramsBytes,
	}).Debug("Requesting prompt details...")

	if err = c.transport.SendMessage(requestCtx, &request); err != nil {
		c.logger.WithFields(map[string]interface{}{
			"prompt_name":   params.Name,
			"prompt_params": paramsBytes,
		}).WithErr(err).Error("Failed to send prompts/get request")
		return nil, fmt.Errorf("failed to send prompts/get request: %v", err)
	}

	select {
	case response := <-responseChan:
		if response.Error != nil {
			c.logger.WithFields(map[string]interface{}{
				"prompt_name":   params.Name,
				"error_message": response.Error.Message,
				"error_code":    response.Error.Code,
			}).Error("Server error during prompts/get request")
			return nil, fmt.Errorf("server error: %s (code: %d)", response.Error.Message, response.Error.Code)
		}

		rawResult, err := json.Marshal(response.Result)
		if err != nil {
			c.logger.WithErr(err).Error("Failed to marshal result into raw message")
			return nil, fmt.Errorf("failed to marshal result into raw message: %v", err)
		}

		var result PromptGetResponse
		if err := json.Unmarshal(rawResult, &result); err != nil {
			c.logger.WithErr(err).Error("Failed to parse prompt details")
			return nil, fmt.Errorf("failed to parse prompt details: %v", err)
		}
		c.logger.WithFields(map[string]interface{}{
			"prompt_name":    params.Name,
			"messages_count": len(result.Messages),
		}).Debug("Client received response for prompts/get request successfully")

		return result.Messages, nil

	case <-requestCtx.Done():
		c.logger.WithFields(map[string]interface{}{
			"prompt_name":     params.Name,
			"request_timeout": c.config.RequestTimeout.String(),
		}).Error("prompts/get request timeout")

		return nil, fmt.Errorf("prompts/get request timeout: %v", requestCtx.Err())
	}
}

func (c *Client) Close(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx, "Client.Close")
	span.SetAttributes(
		attribute.String("client.name", c.config.ClientName),
		attribute.String("client.version", c.config.ClientVersion),
		attribute.String("server.url", c.config.SSE.URL),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	c.mu.Lock()
	if c.state == Disconnected {
		c.mu.Unlock()
		return nil
	}

	c.wg.Wait()

	if err := c.transport.Close(ctx); err != nil {
		c.mu.Unlock()
		return err
	}

	c.state = Disconnected
	c.initialized = false
	c.mu.Unlock()

	c.logger.Info("Client closed successfully")
	return nil
}

func (c *Client) GetState() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Client) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

func (c *Client) GetCapabilities() Capabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capabilities
}

func (c *Client) GetProtocolVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolVersion
}

func calculateBackoff(baseDelay time.Duration, attempt int) time.Duration {
	maxDelay := defaultMaxRetryDelay

	backoff := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}

	jitter := 0.1
	backoff = backoff * (1 + jitter*(2*rand.Float64()-1))

	return time.Duration(backoff)
}
