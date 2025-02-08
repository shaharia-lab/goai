package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
)

// Define the core JSON-RPC types.  We don't use a library to be absolutely
// explicit about adhering to the spec, and to avoid unnecessary dependencies.

type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params"`
}

type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// Initialize Request and Response Params

type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// --- Server Implementation ---

type MCPServer struct {
	protocolVersion    string
	clientCapabilities map[string]any
	logger             *log.Logger // Use Go's standard logger
	in                 io.Reader   // Input stream (stdin)
	out                io.Writer   // Output stream (stdout)
	ServerInfo         struct {    //  serverInfo, use an exported field
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	// Server Capabilities
	capabilities map[string]any
}

func NewMCPServer(in io.Reader, out io.Writer) *MCPServer {

	logger := log.New(os.Stderr, "[MCP Server] ", log.LstdFlags|log.Lmsgprefix) // Log to stderr

	s := &MCPServer{
		protocolVersion: "2024-11-05",
		logger:          logger,
		in:              in,
		out:             out,
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "Generic-MCP-Server", // Server Name
			Version: "0.1.0",              // Server version
		},
		capabilities: map[string]any{
			"logging": map[string]any{}, // Example:  Enable logging capability.
			// Add other server capabilities here, e.g., "resources", "prompts", "tools".
		},
	}

	return s
}

func (s *MCPServer) sendResponse(id *json.RawMessage, result interface{}, err *Error) {
	response := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	jsonResponse, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		s.logger.Printf("Error marshalling response: %v", marshalErr)
		//  Attempt to send an error response about the marshalling failure.
		s.sendError(id, -32603, "Internal error: failed to marshal response", nil)
		return
	}

	// Add newline delimiter.
	jsonResponse = append(jsonResponse, '\n')

	_, writeErr := s.out.Write(jsonResponse)
	if writeErr != nil {
		s.logger.Printf("Error writing response to stdout: %v", writeErr)
		// At this point, we can't really communicate back to the client.  Just log.
	}
}

// sendError sends a JSON-RPC error response.  Handles the boilerplate.
func (s *MCPServer) sendError(id *json.RawMessage, code int, message string, data interface{}) {
	errorResponse := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	jsonErrorResponse, err := json.Marshal(errorResponse)
	if err != nil {
		s.logger.Printf("Error marshaling error response: %v", err) //  Log, not much else we can do.
		return                                                      // Give up.
	}

	// Add newline delimiter.
	jsonErrorResponse = append(jsonErrorResponse, '\n')

	_, writeErr := s.out.Write(jsonErrorResponse)
	if writeErr != nil {
		s.logger.Printf("Failed to write error response: %v", writeErr) // Log write error
	}

}

func (s *MCPServer) handleRequest(request *Request) {
	s.logger.Printf("Received request: method=%s, id=%v", request.Method, request.ID)

	switch request.Method {
	case "initialize":
		var params InitializeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			s.sendError(request.ID, -32602, "Invalid params", nil)
			return
		}

		if !strings.HasPrefix(params.ProtocolVersion, "2024-11") { // Check for any "2024-11" version, not just -05
			s.sendError(request.ID, -32602, "Unsupported protocol version", map[string][]string{"supported": {"2024-11-05"}}) // Send supported versions.
			return
		}

		s.clientCapabilities = params.Capabilities
		result := InitializeResult{
			ProtocolVersion: s.protocolVersion,
			Capabilities:    s.capabilities,
			ServerInfo:      s.ServerInfo, // Use the exported field
		}

		s.sendResponse(request.ID, result, nil)
		//  The spec requires an "initialized" *notification* after the initialize response.
		//  We can't send it *here* because we might be handling other requests concurrently.
		//  The main loop will have to track whether the 'initialized' notification has been received.

	case "ping":
		// Respond with an empty result.
		s.sendResponse(request.ID, map[string]interface{}{}, nil)

	// Add other request handlers here (resources/list, resources/read, etc.)

	default:
		s.sendError(request.ID, -32601, "Method not found", nil)
	}
}

func (s *MCPServer) handleNotification(notification *Notification) {
	s.logger.Printf("Received notification: method=%s", notification.Method)
	switch notification.Method {
	case "notifications/initialized":
		// The server is now initialized. This is currently a noop but important for lifecycle management.
	case "notifications/cancelled":
		// Handle cancellation, likely need a way to map request IDs to running operations
		var cancelParams struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if err := json.Unmarshal(notification.Params, &cancelParams); err == nil {
			//  Here, you'd need a mechanism to find and cancel the operation
			//  associated with cancelParams.RequestID.  This might involve
			//  context.Context and goroutine management, which is beyond a simple example.
			s.logger.Printf("Cancellation requested for ID %s: %s", string(cancelParams.RequestID), cancelParams.Reason)
		}
	// ... handle other notifications ...

	default:
		s.logger.Printf("Unhandled notification: %s", notification.Method) // Log but don't send error.
	}
}

func (s *MCPServer) Run() {
	scanner := bufio.NewScanner(s.in)
	buffer := make([]byte, 0, 64*1024) //  buffer
	scanner.Buffer(buffer, 1024*1024)  //  buffer size

	initialized := false // track whether we've received 'initialized'

	for scanner.Scan() {
		line := scanner.Text()
		s.logger.Printf("Received raw input: %s", line) //  Log the raw JSON.

		var raw json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			s.sendError(nil, -32700, "Parse error", nil)
			continue
		}

		var request Request
		if err := json.Unmarshal(raw, &request); err == nil && request.Method != "" && request.ID != nil {
			// It's a request
			if request.Method != "initialize" && !initialized {
				s.sendError(request.ID, -32000, "Server not initialized", nil) // Or other appropriate error code.
				continue
			}
			s.handleRequest(&request)
			continue
		}

		var notification Notification
		if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
			// It's a notification
			if notification.Method == "notifications/initialized" {
				initialized = true
			} else if !initialized {
				// We shouldn't receive any other notifications before 'initialized'.  Log it, but don't send error.
				s.logger.Printf("Received notification before 'initialized': %s", notification.Method)
				continue
			}

			s.handleNotification(&notification)
			continue
		}

		// If we reach here, it's neither a valid request nor a notification.
		s.sendError(nil, -32600, "Invalid Request", nil)

	}

	if err := scanner.Err(); err != nil {
		if err != io.EOF { // Don't log EOF, that's how we exit gracefully
			s.logger.Printf("Error reading from stdin: %v", err)
		}
	}
	s.logger.Println("Server shutting down.")
}

func main() {
	server := NewMCPServer(os.Stdin, os.Stdout)
	server.Run()
}
