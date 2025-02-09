package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSSEServer(t *testing.T) {
	server := NewSSEServer()

	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.address != ":8080" {
		t.Errorf("Expected default address :8080, got %s", server.address)
	}
	if server.clients == nil {
		t.Fatal("Expected initialized clients map")
	}
}

func TestSetAddress(t *testing.T) {
	server := NewSSEServer()
	newAddress := ":9090"
	server.SetAddress(newAddress)

	if server.address != newAddress {
		t.Errorf("Expected address %s, got %s", newAddress, server.address)
	}
}

func TestHandleSSEConnection(t *testing.T) {
	server := NewSSEServer()

	// Create test request and response recorder
	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()

	// Create a context with cancellation
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	// Start handling SSE connection in a goroutine
	go server.handleSSEConnection(w, req)

	// Give it a moment to initialize
	time.Sleep(100 * time.Millisecond)

	// Check response headers
	headers := w.Header()
	if headers.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type: text/event-stream, got %s", headers.Get("Content-Type"))
	}
	if headers.Get("Cache-Control") != "no-cache" {
		t.Errorf("Expected Cache-Control: no-cache, got %s", headers.Get("Cache-Control"))
	}

	// Check if client was registered
	server.clientsMutex.RLock()
	clientCount := len(server.clients)
	server.clientsMutex.RUnlock()
	if clientCount != 1 {
		t.Errorf("Expected 1 client, got %d", clientCount)
	}

	// Cleanup
	cancel()
	time.Sleep(100 * time.Millisecond) // Give time for cleanup

	// Verify client was removed
	server.clientsMutex.RLock()
	clientCount = len(server.clients)
	server.clientsMutex.RUnlock()
	if clientCount != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", clientCount)
	}
}

func TestHandleClientMessage(t *testing.T) {
	server := NewSSEServer()
	clientID := "test-client"

	// Create a message channel for the test client
	messageChan := make(chan []byte, 10)
	server.clientsMutex.Lock()
	server.clients[clientID] = messageChan
	server.clientsMutex.Unlock()

	// Test valid request
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "ping",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/message?clientID="+clientID, bytes.NewBuffer(jsonBody))
	w := httptest.NewRecorder()

	server.handleClientMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	// Test missing clientID
	req = httptest.NewRequest("POST", "/message", bytes.NewBuffer(jsonBody))
	w = httptest.NewRecorder()

	server.handleClientMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status BadRequest for missing clientID, got %d", w.Code)
	}

	// Test invalid method
	req = httptest.NewRequest("GET", "/message?clientID="+clientID, nil)
	w = httptest.NewRecorder()

	server.handleClientMessage(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status MethodNotAllowed for GET request, got %d", w.Code)
	}
}

func TestBroadcastNotification(t *testing.T) {
	server := NewSSEServer()

	// Create multiple test clients
	client1Chan := make(chan []byte, 10)
	client2Chan := make(chan []byte, 10)

	server.clientsMutex.Lock()
	server.clients["client1"] = client1Chan
	server.clients["client2"] = client2Chan
	server.clientsMutex.Unlock()

	// Broadcast a test notification
	testMethod := "test/notification"
	testParams := map[string]string{"message": "test"}

	server.broadcastNotification(testMethod, testParams)

	// Helper function to verify notification
	verifyNotification := func(ch chan []byte) error {
		select {
		case msg := <-ch:
			var notification Notification
			if err := json.Unmarshal(msg, &notification); err != nil {
				return err
			}
			if notification.Method != testMethod {
				return fmt.Errorf("expected method %s, got %s", testMethod, notification.Method)
			}
			return nil
		case <-time.After(time.Second):
			return fmt.Errorf("timeout waiting for notification")
		}
	}

	// Verify both clients received the notification
	if err := verifyNotification(client1Chan); err != nil {
		t.Errorf("Client 1 notification error: %v", err)
	}
	if err := verifyNotification(client2Chan); err != nil {
		t.Errorf("Client 2 notification error: %v", err)
	}
}

func TestSendMessageToClient(t *testing.T) {
	server := NewSSEServer()
	clientID := "test-client"
	messageChan := make(chan []byte, 1)

	server.clientsMutex.Lock()
	server.clients[clientID] = messageChan
	server.clientsMutex.Unlock()

	testMessage := []byte("test message")
	server.sendMessageToClient(clientID, testMessage)

	select {
	case received := <-messageChan:
		if string(received) != string(testMessage) {
			t.Errorf("Expected message %s, got %s", testMessage, received)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for message")
	}

	// Test sending to non-existent client
	server.sendMessageToClient("non-existent", testMessage)
	// Should not panic and just log the error
}

func TestCORSHandling(t *testing.T) {
	server := NewSSEServer()

	// Test OPTIONS request
	req := httptest.NewRequest("OPTIONS", "/events", nil)
	w := httptest.NewRecorder()

	server.handleHTTPRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK for OPTIONS request, got %d", w.Code)
	}

	headers := w.Header()
	if headers.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Expected CORS Allow-Origin header")
	}
	if headers.Get("Access-Control-Allow-Methods") != "POST, GET, OPTIONS" {
		t.Error("Expected CORS Allow-Methods header")
	}
	if headers.Get("Access-Control-Allow-Headers") != "Content-Type, Authorization" {
		t.Error("Expected CORS Allow-Headers header")
	}
}

func TestServerShutdown(t *testing.T) {
	server := NewSSEServer()
	server.SetAddress(":0") // Use random available port

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error)

	// Start server in goroutine
	go func() {
		errChan <- server.Run(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	cancel()

	// Wait for shutdown with timeout
	select {
	case err := <-errChan:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(6 * time.Second): // Longer than shutdown timeout
		t.Error("Server shutdown timed out")
	}
}
