package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultPingTimeout = 2 * pingInterval
)

type SSETransport struct {
	client          *http.Client
	config          SSEConfig
	logger          observability.Logger
	stopChan        chan struct{}
	reconnectChan   chan struct{}
	mu              sync.RWMutex
	lastPingTime    time.Time
	missedPings     int
	messageEndpoint string
	receiveCallback func(message []byte)
	clientID        string
	state           ConnectionState
	connOnce        sync.Once
	closeOnce       sync.Once
}

func NewSSETransport(logger observability.Logger) *SSETransport {
	return &SSETransport{
		client:          &http.Client{},
		stopChan:        make(chan struct{}),
		reconnectChan:   make(chan struct{}, 1),
		logger:          logger,
		messageEndpoint: defaultMessageEndpoint,
		state:           Disconnected,
	}
}

func (t *SSETransport) SetReceiveMessageCallback(callback func(message []byte)) {
	t.receiveCallback = callback
}

func (t *SSETransport) Connect(ctx context.Context, config ClientConfig) error {
	ctx, span := observability.StartSpan(ctx, "SSETransport.Connect")
	span.SetAttributes(
		attribute.String("url", config.SSE.URL),
		attribute.String("client_name", config.ClientName),
		attribute.String("client_version", config.ClientVersion),
	)
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	t.config = config.SSE
	t.logger = config.Logger
	t.messageEndpoint = config.MessageEndpoint
	t.setState(Connecting)

	t.logger.Debug("Resetting connection state")
	t.connOnce = sync.Once{}

	t.logger.Debug("Starting connection manager")
	t.connOnce.Do(func() {
		go t.connectionManager(ctx, config)
	})

	// Wait for initial connection or failure
	select {
	case <-time.After(30 * time.Second):
		t.logger.WithFields(map[string]interface{}{
			"url":     config.SSE.URL,
			"timeout": 30 * time.Second,
		}).Debug("Timeout waiting for initial connection")
		return fmt.Errorf("timeout waiting for initial connection")
	case <-ctx.Done():
		t.logger.Debug("Context cancelled")
		return ctx.Err()
	case <-t.reconnectChan:
		t.logger.Debug("Connection established")
		return nil
	}
}

func (t *SSETransport) connectionManager(ctx context.Context, config ClientConfig) {
	retryCount := 0

	for {
		select {
		case <-t.stopChan:
			return
		default:
			if t.getState() != Connecting && t.getState() != Disconnected {
				// Wait for disconnect or reconnect signal
				select {
				case <-t.stopChan:
					return
				case <-t.reconnectChan:
					t.setState(Connecting)
				}
			}

			if err := t.establishConnection(ctx, config); err != nil {
				retryCount++
				if retryCount >= config.MaxRetries {
					t.logger.WithErr(err).Debug("Max retries reached. Connection attempt failed, Disconnecting...")
					t.setState(Disconnected)
					return
				}

				delay := calculateBackoff(config.RetryDelay, retryCount)
				t.logger.WithErr(err).WithFields(map[string]interface{}{
					"retry_count": retryCount,
					"delay":       delay,
				}).Debug("Connection attempt failed. Retrying...")

				select {
				case <-time.After(delay):
				case <-t.stopChan:
					return
				}
			} else {
				t.logger.Debug("Connection established")
				t.setState(Connected)
				retryCount = 0

				// Notify about successful connection
				select {
				case t.reconnectChan <- struct{}{}:
				default:
					t.logger.Debug("Reconnect channel is full. But it's OK")
				}
			}
		}
	}
}

func (t *SSETransport) establishConnection(ctx context.Context, config ClientConfig) error {
	t.logger.WithFields(map[string]interface{}{
		"url":            config.SSE.URL,
		"client_name":    config.ClientName,
		"client_version": config.ClientVersion,
	}).Debug("Establishing connection...")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Add Accept header for SSE
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.client.Do(req)
	if err != nil {
		t.logger.WithErr(err).Debug("Failed to connect to events endpoint")
		return fmt.Errorf("failed to connect to events endpoint")
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.logger.WithFields(map[string]interface{}{
			"status_code": resp.StatusCode,
			"url":         config.SSE.URL,
		}).Debug("Unexpected status code during establishing connection")

		return fmt.Errorf("unexpected status code")
	}

	messageEndpointChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		t.processEventStream(ctx, resp.Body, messageEndpointChan, errChan, config)
	}()

	select {
	case endpoint := <-messageEndpointChan:
		t.messageEndpoint = endpoint
		t.clientID = t.extractClientID(endpoint)
		t.logger.WithFields(map[string]interface{}{
			"message_endpoint": endpoint,
			"client_id":        t.clientID,
		}).Debug("Received message endpoint")

	case err := <-errChan:
		t.setState(Disconnected)
		t.logger.WithErr(err).Debug("failed to get message endpoint")
		return fmt.Errorf("failed to get message endpoint")
	case <-time.After(30 * time.Second):
		t.setState(Disconnected)
		t.logger.Debug("Timeout waiting for message endpoint")
		return fmt.Errorf("timeout waiting for message endpoint")
	case <-ctx.Done():
		t.logger.Debug("Context cancelled during establishing connection")
		return ctx.Err()
	}

	t.mu.Lock()
	t.lastPingTime = time.Now()
	t.missedPings = 0
	t.mu.Unlock()

	t.logger.Debug("Starting ping monitor")
	go t.monitorPings()

	return nil
}

func (t *SSETransport) processEventStream(ctx context.Context, reader io.Reader, messageEndpointChan chan string, errChan chan error, config ClientConfig) {
	t.logger.Debug("Event stream processing started")

	var endpointSent bool
	event := make(map[string]string)
	dataBuffer := new(bytes.Buffer)

	// Create a scanner with a custom split function
	scanner := bufio.NewScanner(reader)

	// Create a larger initial buffer
	const initialBufferSize = 32 * 1024 // 1MB
	buf := make([]byte, initialBufferSize)
	scanner.Buffer(buf, bufio.MaxScanTokenSize*100) // Increase max token size

	// Process line by line
	for scanner.Scan() {
		select {
		case <-t.stopChan:
			return
		default:
			line := scanner.Text()

			// Empty line marks the end of an event
			if line == "" {
				if len(event) > 0 {
					t.processEvent(event, messageEndpointChan, endpointSent, &endpointSent)
					// Clear for next event
					event = make(map[string]string)
					dataBuffer.Reset()
				}
				continue
			}

			// Handle ping event
			if line == ":ping" {
				t.handlePing()
				continue
			}

			// Parse event fields
			switch {
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimPrefix(line, "data:")
				if len(data) > 0 && data[0] == ' ' {
					data = data[1:] // Remove space if it exists
				}
				dataBuffer.WriteString(data)
				dataBuffer.WriteString("\n")
				event["data"] = dataBuffer.String()

			case strings.HasPrefix(line, "event:"):
				event["event"] = strings.TrimSpace(strings.TrimPrefix(line, "event:"))

			case strings.HasPrefix(line, "id:"):
				event["id"] = strings.TrimSpace(strings.TrimPrefix(line, "id:"))

			case strings.HasPrefix(line, "retry:"):
				event["retry"] = strings.TrimSpace(strings.TrimPrefix(line, "retry:"))
			}
		}
	}

	// Handle scanner errors
	if err := scanner.Err(); err != nil {
		t.logger.WithErr(err).Debug("Scanner error")
		errChan <- fmt.Errorf("scanner error: %v", err)
		select {
		case t.reconnectChan <- struct{}{}:
		default:
			t.logger.Debug("Reconnect channel is full. But it's OK")
		}

		return
	}

	// Handle EOF
	t.logger.Debug("Event stream closed unexpected due to possibly EOF")
	errChan <- fmt.Errorf("event stream closed unexpectedly")
	select {
	case t.reconnectChan <- struct{}{}:
	default:
		t.logger.Debug("Reconnect channel is full. But it's OK")
	}
}
func (t *SSETransport) processEvent(
	event map[string]string,
	messageEndpointChan chan string,
	endpointSent bool,
	endpointSentPtr *bool,
) {
	t.logger.WithFields(map[string]interface{}{
		"event": event["event"],
		"id":    event["id"],
		"retry": event["retry"],
	}).Debug("Received event")

	if data, ok := event["data"]; ok {
		// Remove last newline if exists
		data = strings.TrimSuffix(data, "\n")

		// Check if data is a URL for message endpoint
		if (strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://")) && !endpointSent {
			messageEndpointChan <- data
			*endpointSentPtr = true
			return
		}

		// If we have a callback and this isn't an endpoint URL, process the data
		t.logger.Debug("this isn't an endpoint URL, process the data")
		if t.receiveCallback != nil {
			t.receiveCallback([]byte(data))
		}
	}
}

func (t *SSETransport) SendMessage(ctx context.Context, message interface{}) error {
	ctx, span := observability.StartSpan(ctx, "SSETransport.SendMessage")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	t.mu.RLock()
	endpoint := t.messageEndpoint
	t.mu.RUnlock()

	if endpoint == "" {
		return fmt.Errorf("no message endpoint available")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	t.logger.WithFields(map[string]interface{}{
		"endpoint": endpoint,
	}).Debug("Sending message")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		t.logger.WithErr(err).Debug("Failed to create request")
		return fmt.Errorf("failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// Check if the error was due to context cancellation
		if ctx.Err() != nil {
			t.logger.Debug("Context cancelled during sending message")
			return ctx.Err()
		}

		t.logger.WithErr(err).Debug("Failed to send message")
		return fmt.Errorf("failed to send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {

			t.logger.WithFields(map[string]interface{}{
				"status_code": resp.StatusCode,
			}).Debug("Unexpected status code")
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (t *SSETransport) Close(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx, "SSETransport.Close")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	t.closeOnce.Do(func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		if t.state == Disconnected {
			return
		}

		t.logger.Debug("Closing SSE transport...")
		close(t.stopChan)
		t.state = Disconnected
	})

	return err
}

func (t *SSETransport) setState(s ConnectionState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = s
}

func (t *SSETransport) getState() ConnectionState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

func (t *SSETransport) handlePing() {
	t.mu.Lock()
	t.lastPingTime = time.Now()
	t.missedPings = 0
	t.mu.Unlock()
}

func (t *SSETransport) monitorPings() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopChan:
			return
		case <-ticker.C:
			if t.checkPingStatus() {
				// Signal reconnect
				select {
				case t.reconnectChan <- struct{}{}:
				default:
					// Channel is full, that's OK
				}
				return
			}
		}
	}
}

func (t *SSETransport) checkPingStatus() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Since(t.lastPingTime) > defaultPingTimeout {
		t.missedPings++
		t.logger.WithFields(map[string]interface{}{
			"total_missed_pings": t.missedPings,
		}).Warn("Missed ping")

		if t.missedPings >= defaultMaxMissedPings {
			t.logger.WithFields(map[string]interface{}{
				"total_missed_pings": t.missedPings,
			}).Warn("Connection lost")

			t.state = Disconnected
			return true
		}
	}

	return false
}

func (t *SSETransport) extractClientID(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		t.logger.WithFields(map[string]interface{}{
			"endpoint": endpoint,
		}).Debug("Failed to parse endpoint URL")

		return ""
	}

	values := u.Query()
	clientID := values.Get("clientID")
	t.logger.WithFields(map[string]interface{}{
		"client_id": clientID,
	}).Debug("Extracted client ID")

	return clientID
}
