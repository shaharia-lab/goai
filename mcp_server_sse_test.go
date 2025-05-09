package goai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewSSEServer(t *testing.T) {
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)

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
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)
	newAddress := ":9090"
	server.SetAddress(newAddress)

	if server.address != newAddress {
		t.Errorf("Expected address %s, got %s", newAddress, server.address)
	}
}

func TestHandleSSEConnection(t *testing.T) {
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)

	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	go server.handleSSEConnection(ctx, w, req)

	time.Sleep(100 * time.Millisecond)

	headers := w.Header()
	if headers.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type: text/event-stream, got %s", headers.Get("Content-Type"))
	}
	if headers.Get("Cache-Control") != "no-cache" {
		t.Errorf("Expected Cache-Control: no-cache, got %s", headers.Get("Cache-Control"))
	}

	server.clientsMutex.RLock()
	clientCount := len(server.clients)
	server.clientsMutex.RUnlock()
	if clientCount != 1 {
		t.Errorf("Expected 1 client, got %d", clientCount)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	server.clientsMutex.RLock()
	clientCount = len(server.clients)
	server.clientsMutex.RUnlock()
	if clientCount != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", clientCount)
	}
}

func TestHandleClientMessage(t *testing.T) {
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)
	clientID := "test-client"

	messageChan := make(chan []byte, 10)
	server.clientsMutex.Lock()
	server.clients[clientID] = messageChan
	server.clientsMutex.Unlock()

	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "ping",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/message?clientID="+clientID, bytes.NewBuffer(jsonBody))
	w := httptest.NewRecorder()

	server.handleClientMessage(context.Background(), w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/message", bytes.NewBuffer(jsonBody))
	w = httptest.NewRecorder()

	server.handleClientMessage(context.Background(), w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status BadRequest for missing clientID, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/message?clientID="+clientID, nil)
	w = httptest.NewRecorder()

	server.handleClientMessage(context.Background(), w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status MethodNotAllowed for GET request, got %d", w.Code)
	}
}

func TestBroadcastNotification(t *testing.T) {
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)

	client1Chan := make(chan []byte, 10)
	client2Chan := make(chan []byte, 10)

	server.clientsMutex.Lock()
	server.clients["client1"] = client1Chan
	server.clients["client2"] = client2Chan
	server.clientsMutex.Unlock()

	testMethod := "test/notification"
	testParams := map[string]string{"message": "test"}

	server.broadcastNotification(testMethod, testParams)

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

	if err := verifyNotification(client1Chan); err != nil {
		t.Errorf("Client 1 notification error: %v", err)
	}
	if err := verifyNotification(client2Chan); err != nil {
		t.Errorf("Client 2 notification error: %v", err)
	}
}

func TestSendMessageToClient(t *testing.T) {
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)
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

	server.sendMessageToClient("non-existent", testMessage)
}

func TestCORSHandling(t *testing.T) {
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)

	req := httptest.NewRequest("OPTIONS", "/events", nil)
	w := httptest.NewRecorder()

	server.handleHTTPRequest(context.Background(), w, req)

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
	baseServer, _ := NewBaseServer(UseLogger(NewNullLogger()))
	server := NewSSEServer(baseServer)
	server.SetAddress(":0")

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error)

	go func() {
		errChan <- server.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errChan:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(6 * time.Second): // Longer than shutdown timeout
		t.Error("Server shutdown timed out")
	}
}

func TestHandlePromptGetSSE(t *testing.T) {
	tests := []struct {
		name           string
		initFirst      bool
		input          string
		expectedID     string
		expectedError  *Error
		expectedResult map[string]interface{}
	}{
		{
			name:       "valid prompt get request",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {"name": "code_review", "arguments": {"language": "go", "code": "test code", "focus_areas": "test"}}}`,
			expectedID: "1",
			expectedResult: map[string]interface{}{
				"description": "Review code",
				"messages": []interface{}{
					map[string]interface{}{
						"role": "user",
						"content": map[string]interface{}{
							"type": "text",
							"text": "Please review this code:",
						},
					},
				},
			},
		},
		{
			name:       "request without initialization",
			initFirst:  false,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {"id": "test-prompt-1"}}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32000,
				Message: "Server not initialized",
			},
		},
		{
			name:      "malformed request",
			initFirst: false,
			input:     `{"jsonrpc": "2.0", "method": "prompts/get", "id": }`,
			expectedError: &Error{
				Code:    -32700,
				Message: "Error unmarshaling message",
			},
		},
		{
			name:       "get prompt with invalid name",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {"name": "nonexistent_prompt"}}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Prompt not found",
			},
		},
		{
			name:       "get prompt without name parameter",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": {}}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Prompt not found",
			},
		},
		{
			name:       "get prompt with null params",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": null}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Prompt not found",
			},
		},
		{
			name:       "get prompt with invalid params type",
			initFirst:  true,
			input:      `{"jsonrpc": "2.0", "method": "prompts/get", "id": 1, "params": "code_review"}`,
			expectedID: "1",
			expectedError: &Error{
				Code:    -32602,
				Message: "Invalid params",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create prompt manager with test prompt
			codeReviewPrompt := Prompt{
				Name:        "code_review",
				Description: "Review code",
				Messages: []PromptMessage{
					{
						Role: "user",
						Content: PromptContent{
							Type: "text",
							Text: "Please review this code:",
						},
					},
				},
				Arguments: []PromptArgument{
					{Name: "language", Required: true},
					{Name: "code", Required: true},
					{Name: "focus_areas", Required: true},
				},
			}

			baseServer, _ := NewBaseServer(
				UseLogger(NewNullLogger()),
			)
			baseServer.AddPrompts(codeReviewPrompt)
			server := NewSSEServer(baseServer)

			clientID := "test-client"
			messageChan := make(chan []byte, 10)
			server.clientsMutex.Lock()
			server.clients[clientID] = messageChan
			server.clientsMutex.Unlock()

			if tt.initFirst {
				initRequest := `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`
				req := httptest.NewRequest("POST", "/message?clientID="+clientID, bytes.NewBufferString(initRequest))
				w := httptest.NewRecorder()
				server.handleClientMessage(context.Background(), w, req)
				require.Equal(t, http.StatusOK, w.Code)

				initNotification := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
				req = httptest.NewRequest("POST", "/message?clientID="+clientID, bytes.NewBufferString(initNotification))
				w = httptest.NewRecorder()
				server.handleClientMessage(context.Background(), w, req)
				require.Equal(t, http.StatusOK, w.Code)

				for len(messageChan) > 0 {
					<-messageChan
				}
			}

			req := httptest.NewRequest("POST", "/message?clientID="+clientID, bytes.NewBufferString(tt.input))
			w := httptest.NewRecorder()
			server.handleClientMessage(context.Background(), w, req)
			require.Equal(t, http.StatusOK, w.Code)

			select {
			case responseBytes := <-messageChan:
				var response Response
				err := json.Unmarshal(responseBytes, &response)
				require.NoError(t, err)

				if tt.expectedError != nil {
					require.NotNil(t, response.Error)
					require.Equal(t, tt.expectedError.Code, response.Error.Code)
					require.Equal(t, tt.expectedError.Message, response.Error.Message)
				} else {
					require.Nil(t, response.Error)
					require.Equal(t, "2.0", response.JSONRPC)
					require.Equal(t, tt.expectedID, string(*response.ID))

					result, ok := response.Result.(map[string]interface{})
					require.True(t, ok)
					require.Equal(t, tt.expectedResult, result)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Timeout waiting for response")
			}

			server.clientsMutex.Lock()
			delete(server.clients, clientID)
			server.clientsMutex.Unlock()
			close(messageChan)
		})
	}
}

func TestSSEConnectionFlow(t *testing.T) {
	baseServer, err := NewBaseServer(
		UseLogger(NewNullLogger()),
		UseSSEServerPort(":0"),
	)
	require.NoError(t, err)

	server := NewSSEServer(baseServer)
	require.NotNil(t, server)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		wg.Wait()
		server.clientsMutex.Lock()
		for clientID, ch := range server.clients {
			close(ch)
			delete(server.clients, clientID)
		}
		server.clientsMutex.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := server.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Unexpected server error: %v", err)
		}
	}()

	t.Run("Complete Connection Flow", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events", nil)
		w := httptest.NewRecorder()
		clientCtx, clientCancel := context.WithCancel(context.Background())
		defer clientCancel()

		go server.handleSSEConnection(context.Background(), w, req.WithContext(clientCtx))
		time.Sleep(100 * time.Millisecond)

		headers := w.Header()
		require.Equal(t, "text/event-stream", headers.Get("Content-Type"))
		require.Equal(t, "no-cache", headers.Get("Cache-Control"))

		clientID := "test-client"
		server.clientsMutex.Lock()
		server.clients[clientID] = make(chan []byte, 10)
		server.clientsMutex.Unlock()

		initializePayload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "1",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}
		payloadBytes, err := json.Marshal(initializePayload)
		require.NoError(t, err)

		req = httptest.NewRequest("POST", "/message?clientID="+clientID, bytes.NewBuffer(payloadBytes))
		w = httptest.NewRecorder()

		server.handleClientMessage(context.Background(), w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var initResponse struct {
			JsonRPC string                 `json:"jsonrpc"`
			ID      string                 `json:"id"`
			Result  map[string]interface{} `json:"result"`
		}

		select {
		case msg := <-server.clients[clientID]:
			err := json.Unmarshal(msg, &initResponse)
			require.NoError(t, err)
			require.Equal(t, "2.0", initResponse.JsonRPC)
			require.Equal(t, "1", initResponse.ID)
			require.Contains(t, initResponse.Result, "serverInfo")
			require.Contains(t, initResponse.Result, "capabilities")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for initialize response")
		}
	})

	t.Run("Concurrent Connections", func(t *testing.T) {
		numClients := 5
		var wgClients sync.WaitGroup
		wgClients.Add(numClients)

		for i := 0; i < numClients; i++ {
			go func(clientNum int) {
				defer wgClients.Done()
				clientID := fmt.Sprintf("concurrent-client-%d", clientNum)

				server.clientsMutex.Lock()
				server.clients[clientID] = make(chan []byte, 10)
				server.clientsMutex.Unlock()

				req := httptest.NewRequest("GET", "/events", nil)
				w := httptest.NewRecorder()
				clientCtx, clientCancel := context.WithCancel(context.Background())
				defer clientCancel()

				go server.handleSSEConnection(context.Background(), w, req.WithContext(clientCtx))
				time.Sleep(50 * time.Millisecond)

				server.clientsMutex.RLock()
				_, exists := server.clients[clientID]
				server.clientsMutex.RUnlock()
				require.True(t, exists)
			}(i)
		}

		wgClients.Wait()
	})
}
