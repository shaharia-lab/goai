package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	baseRetryDelay = 5 * time.Second
	maxRetryDelay  = 30 * time.Second
	maxMissedPings = 3
	//pingInterval   = 30 * time.Second
)

type ConnectionState int

const (
	Disconnected ConnectionState = iota
	Connecting
	Connected
)

type VeryFirstEvent struct {
	Event           string
	MessageEndpoint string
	ClientID        string
}

type ServerCapabilities struct {
	ProtocolVersion string
	Capabilities    map[string]interface{}
}

type SSEClient struct {
	url                string
	messageEndpoint    string
	serverCapabilities ServerCapabilities
	lastPingTime       time.Time
	missedPings        int
	retryDelay         time.Duration
	stopChan           chan struct{}
	reconnectChan      chan struct{}
	mu                 sync.Mutex
	retryAttempt       int
	state              ConnectionState
	connected          chan struct{}
	clientID           string
	initialized        bool
}

type SSEClientConfig struct {
	URL           string
	RetryDelay    time.Duration
	StopChan      chan struct{}
	ReconnectChan chan struct{}
	MaxRetries    int
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID int `json:"id"`
}

func NewSSEClient(config SSEClientConfig) *SSEClient {
	if config.RetryDelay == 0 {
		config.RetryDelay = baseRetryDelay
	}
	if config.StopChan == nil {
		config.StopChan = make(chan struct{})
	}
	if config.ReconnectChan == nil {
		config.ReconnectChan = make(chan struct{})
	}
	return &SSEClient{
		url:           config.URL,
		retryDelay:    config.RetryDelay,
		stopChan:      config.StopChan,
		reconnectChan: config.ReconnectChan,
		state:         Disconnected,
		connected:     make(chan struct{}),
	}
}

func (c *SSEClient) Connect() error {
	log.Printf("Initiating connection to %s", c.url)
	c.setState(Connecting)

	// Create channels for initialization and errors
	initDone := make(chan struct{})
	errCh := make(chan error, 1)

	// Start the connection in a goroutine
	go func() {
		// Make the initial connection
		resp, err := http.Get(c.url)
		if err != nil {
			errCh <- fmt.Errorf("failed to connect to events endpoint: %v", err)
			return
		}

		go c.monitorPings()

		// Start reading the SSE stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-c.stopChan:
				resp.Body.Close()
				return
			default:
				line := scanner.Text()
				if strings.TrimSpace(line) == "" {
					continue
				}
				log.Printf("Received SSE event: %s", line)

				if strings.HasPrefix(line, "event:") {
					event := strings.TrimSpace(line[len("event:"):])
					if event == "endpoint" {
						continue
					}
				} else if strings.HasPrefix(line, "data:") {
					data := strings.TrimSpace(line[len("data:"):])

					// Handle message endpoint
					if !strings.HasPrefix(data, "{") {
						c.messageEndpoint = data
						c.extractClientID(data)

						// Send initialize request
						if err := c.sendInitializeRequest(); err != nil {
							errCh <- fmt.Errorf("initialize request failed: %v", err)
							continue
						}
						continue
					}

					// Handle JSON responses
					var jsonRPCResp jsonRPCResponse
					if err := json.Unmarshal([]byte(data), &jsonRPCResp); err == nil {
						// Handle initialize response
						if jsonRPCResp.ID == 1 && !c.initialized {
							var initResponse struct {
								ProtocolVersion string                 `json:"protocolVersion"`
								Capabilities    map[string]interface{} `json:"capabilities"`
							}
							if err := json.Unmarshal(jsonRPCResp.Result, &initResponse); err == nil {
								c.serverCapabilities = ServerCapabilities{
									ProtocolVersion: initResponse.ProtocolVersion,
									Capabilities:    initResponse.Capabilities,
								}

								// Send initialized notification
								if err := c.sendInitializedNotification(); err != nil {
									errCh <- fmt.Errorf("initialized notification failed: %v", err)
									continue
								}

								c.initialized = true
								c.setState(Connected)
								log.Println("Connection established successfully")
								close(initDone)
							}
						}
					}
				} else if line == ":ping" {
					log.Println("Received ping from server")
					c.resetPingStatus()
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				errCh <- fmt.Errorf("SSE stream error: %v", err)
			}
		}
	}()

	// Wait for either initialization completion or timeout
	select {
	case err := <-errCh:
		return err
	case <-initDone:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("connection timeout after 30 seconds")
	}
}

func (c *SSEClient) establishConnection() error {
	log.Println("Step 1: Connecting to /events endpoint...")
	resp, err := http.Get(c.url)
	if err != nil {
		return fmt.Errorf("failed to connect to events endpoint: %v", err)
	}

	go c.monitorPings()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		log.Printf("Received SSE event: %s", line)

		if strings.HasPrefix(line, "event:") {
			event := strings.TrimSpace(line[len("event:"):])
			if event == "message" {
				continue
			}
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(line[len("data:"):])

			// Handle message endpoint
			if !strings.HasPrefix(data, "{") {
				c.messageEndpoint = data
				c.extractClientID(data)

				// Send initialize request
				if err := c.sendInitializeRequest(); err != nil {
					resp.Body.Close()
					return fmt.Errorf("initialize request failed: %v", err)
				}
				continue
			}

			// Handle JSON responses
			var jsonRPCResp jsonRPCResponse
			if err := json.Unmarshal([]byte(data), &jsonRPCResp); err == nil {
				// Handle initialize response
				if jsonRPCResp.ID == 1 && !c.initialized {
					var initResponse struct {
						ProtocolVersion string                 `json:"protocolVersion"`
						Capabilities    map[string]interface{} `json:"capabilities"`
					}
					if err := json.Unmarshal(jsonRPCResp.Result, &initResponse); err == nil {
						c.serverCapabilities = ServerCapabilities{
							ProtocolVersion: initResponse.ProtocolVersion,
							Capabilities:    initResponse.Capabilities,
						}

						// Send initialized notification
						if err := c.sendInitializedNotification(); err != nil {
							resp.Body.Close()
							return fmt.Errorf("initialized notification failed: %v", err)
						}

						c.initialized = true
						c.setState(Connected)
						log.Println("Connection established successfully")
					}
				}
			}
		} else if line == ":ping" {
			log.Println("Received ping from server")
			c.resetPingStatus()
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE stream error: %v", err)
	}
	return fmt.Errorf("SSE stream ended")
}

func (c *SSEClient) sendInitializeRequest() error {
	// For initialization requests, we don't check connection state
	endpoint := c.messageEndpoint
	if endpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	payload := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]bool{
					"listChanged": true,
				},
				"sampling": map[string]interface{}{},
			},
			"clientInfo": map[string]string{
				"name":    "ExampleClient",
				"version": "1.0.0",
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	log.Printf("Sending request to %s with payload: %s", endpoint, string(jsonData))

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Response status: %s", resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	if len(body) > 0 {
		log.Printf("Response body: %s", string(body))
	}

	return nil
}

func (c *SSEClient) sendInitializedNotification() error {
	// For initialization notifications, we don't check connection state
	endpoint := c.messageEndpoint
	if endpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	payload := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	log.Printf("Sending request to %s with payload: %s", endpoint, string(jsonData))

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Response status: %s", resp.Status)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status: %s", resp.Status)
	}

	return nil
}

func (c *SSEClient) ListTools() ([]Tool, error) {
	idJson := json.RawMessage(`"1"`)

	request := Request{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      &idJson,
		Params:  json.RawMessage(`{"cursor": ""}`),
	}

	response, err := c.sendRequest(request)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %v", err)
	}

	var jsonRPCResp jsonRPCResponse
	if err := json.Unmarshal(response, &jsonRPCResp); err != nil {
		return nil, fmt.Errorf("failed to parse tools response: %v", err)
	}

	if jsonRPCResp.Error != nil {
		return nil, fmt.Errorf("server error: %s (code: %d)",
			jsonRPCResp.Error.Message, jsonRPCResp.Error.Code)
	}

	var tools []Tool
	if err := json.Unmarshal(jsonRPCResp.Result, &tools); err != nil {
		return nil, fmt.Errorf("failed to parse tools data: %v", err)
	}

	log.Printf("Retrieved %d tools from server", len(tools))
	return tools, nil
}

func (c *SSEClient) sendRequest(payload interface{}) ([]byte, error) {
	c.mu.Lock()
	if c.state != Connected {
		c.mu.Unlock()
		return nil, fmt.Errorf("client not connected")
	}
	endpoint := c.messageEndpoint
	c.mu.Unlock()

	if endpoint == "" {
		return nil, fmt.Errorf("no message endpoint available")
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %v", err)
	}

	log.Printf("Sending request to %s with payload: %s", endpoint, string(jsonData))

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	defer resp.Body.Close()

	log.Printf("Response status: %s", resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if len(body) > 0 {
		log.Printf("Response body: %s", string(body))
	}

	return body, nil
}

func (c *SSEClient) setState(state ConnectionState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != state {
		c.state = state
		if state == Connected {
			select {
			case c.connected <- struct{}{}:
			default:
			}
		}
	}
}

func (c *SSEClient) extractClientID(endpoint string) {
	parts := strings.Split(endpoint, "clientID=")
	if len(parts) > 1 {
		c.clientID = parts[1]
		log.Printf("Extracted client ID: %s", c.clientID)
	}
}

func (c *SSEClient) monitorPings() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkPingStatus()
		}
	}
}

func (c *SSEClient) checkPingStatus() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastPingTime) > pingInterval {
		c.missedPings++
		log.Printf("Missed ping #%d", c.missedPings)

		if c.missedPings >= maxMissedPings {
			log.Printf("Maximum missed pings (%d) reached, triggering reconnection", maxMissedPings)
			c.triggerReconnect()
		}
	}
}

func (c *SSEClient) resetPingStatus() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPingTime = time.Now()
	c.missedPings = 0
}

func (c *SSEClient) triggerReconnect() {
	select {
	case c.reconnectChan <- struct{}{}:
		delay := c.calculateBackoff()
		log.Printf("Connection lost. Attempting to reconnect in %v (attempt %d)", delay, c.retryAttempt)

		select {
		case <-time.After(delay):
			c.retryAttempt++
		case <-c.stopChan:
			return
		}
	default:
	}
}

func (c *SSEClient) Stop() {
	close(c.stopChan)
}

func (c *SSEClient) calculateBackoff() time.Duration {
	delay := baseRetryDelay * time.Duration(1<<uint(c.retryAttempt))

	if delay > maxRetryDelay {
		delay = maxRetryDelay
		c.retryAttempt = 0
		log.Printf("Maximum retry delay reached, resetting backoff")
	}

	return delay
}
