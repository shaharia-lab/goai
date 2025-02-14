package main

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
	baseURL        = "http://localhost:8080/events"
	pingInterval   = 30 * time.Second
	baseRetryDelay = 5 * time.Second
	maxRetryDelay  = 30 * time.Second
	maxMissedPings = 3
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
}

type SSEClientConfig struct {
	URL           string
	RetryDelay    time.Duration
	StopChan      chan struct{}
	ReconnectChan chan struct{}
}

func NewSSEClient(config SSEClientConfig) *SSEClient {
	return &SSEClient{
		url:           config.URL,
		retryDelay:    config.RetryDelay,
		stopChan:      config.StopChan,
		reconnectChan: config.ReconnectChan,
	}
}

func (c *SSEClient) Start() {
	log.Printf("Starting SSE client, connecting to %s", c.url)

	go c.monitorPings()

	for {
		select {
		case <-c.stopChan:
			log.Println("Stopping SSE client")
			return
		default:
			if err := c.establishConnection(); err != nil {
				log.Printf("Connection error: %v", err)

				delay := c.calculateBackoff()
				log.Printf("Waiting %v before reconnection attempt %d", delay, c.retryAttempt)

				select {
				case <-time.After(delay):
					c.retryAttempt++
				case <-c.stopChan:
					return
				}
			} else {
				c.retryAttempt = 0
			}
		}
	}
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

func (c *SSEClient) establishConnection() error {
	log.Println("Step 1: Connecting to /events endpoint...")
	resp, err := http.Get(c.url)
	if err != nil {
		return fmt.Errorf("failed to connect to events endpoint: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var firstEvent VeryFirstEvent

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			firstEvent.Event = strings.TrimSpace(line[len("event:"):])
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(line[len("data:"):])
			if !strings.HasPrefix(data, "{") {
				firstEvent.MessageEndpoint = data
				break
			}
		}
	}

	if firstEvent.MessageEndpoint == "" {
		return fmt.Errorf("failed to receive message endpoint")
	}

	log.Printf("Step 2: Sending initialize request to message endpoint: %s", firstEvent.MessageEndpoint)
	if err := c.sendInitializeRequest(firstEvent.MessageEndpoint); err != nil {
		return fmt.Errorf("initialize request failed: %v", err)
	}

	log.Println("Step 3: Sending initialized notification...")
	if err := c.sendInitializedNotification(firstEvent.MessageEndpoint); err != nil {
		return fmt.Errorf("initialized notification failed: %v", err)
	}

	log.Println("Connection established successfully")
	c.messageEndpoint = firstEvent.MessageEndpoint
	c.resetPingStatus()

	return c.monitorSSEStream(resp.Body)
}

func (c *SSEClient) sendInitializeRequest(endpoint string) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
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

	response, err := c.sendRequest(endpoint, payload)
	if err != nil {
		return fmt.Errorf("failed to send initialize request: %v", err)
	}

	log.Printf("Raw initialize response: %s", string(response))

	if len(response) > 0 {
		var initResponse struct {
			Result struct {
				ProtocolVersion string                 `json:"protocolVersion"`
				Capabilities    map[string]interface{} `json:"capabilities"`
			} `json:"result"`
		}

		if err := json.Unmarshal(response, &initResponse); err != nil {
			return fmt.Errorf("failed to parse initialize response: %v", err)
		}

		if initResponse.Result.ProtocolVersion == "" {
			log.Println("Warning: Server did not return a protocol version")
		}
		if initResponse.Result.Capabilities == nil {
			log.Println("Warning: Server did not return any capabilities")
		}

		c.serverCapabilities = ServerCapabilities{
			ProtocolVersion: initResponse.Result.ProtocolVersion,
			Capabilities:    initResponse.Result.Capabilities,
		}

		log.Printf("Server Protocol Version: %s", c.serverCapabilities.ProtocolVersion)
		log.Printf("Server Capabilities: %+v", c.serverCapabilities.Capabilities)
	} else {
		log.Println("Warning: Received empty response from initialize request")
	}

	return nil
}

func (c *SSEClient) sendInitializedNotification(endpoint string) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	_, err := c.sendRequest(endpoint, payload)
	return err
}

func (c *SSEClient) sendRequest(endpoint string, payload interface{}) ([]byte, error) {
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

func (c *SSEClient) monitorSSEStream(body interface{}) error {
	scanner := bufio.NewScanner(body.(interface{ Read([]byte) (int, error) }))
	for scanner.Scan() {
		line := scanner.Text()
		if line == ":ping" {
			log.Println("Received ping from server")
			c.resetPingStatus()
		}
	}
	return scanner.Err()
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

func (c *SSEClient) resetPingStatus() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPingTime = time.Now()
	c.missedPings = 0
}

func (c *SSEClient) Stop() {
	close(c.stopChan)
}

func main() {
	client := NewSSEClient(SSEClientConfig{
		URL:           baseURL,
		RetryDelay:    baseRetryDelay,
		StopChan:      make(chan struct{}),
		ReconnectChan: make(chan struct{}),
	})

	done := make(chan struct{})
	go func() {
		client.Start()
		close(done)
	}()

	<-done
}
