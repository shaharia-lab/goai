package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// StdIOServer is the MCP server implementation using standard input/output.
type StdIOServer struct {
	*BaseServer // Embed the common server
	in          io.Reader
	out         io.Writer
}

// NewStdIOServer creates a new StdIOServer.
func NewStdIOServer(baseServer *BaseServer, in io.Reader, out io.Writer) *StdIOServer {
	s := &StdIOServer{
		BaseServer: baseServer,
		in:         in,
		out:        out,
	}

	// Set the concrete send methods for StdIOServer.
	s.sendResp = s.sendResponse
	s.sendErr = s.sendError
	s.sendNoti = s.sendNotification

	return s
}

// sendResponse sends a JSON-RPC response (StdIO implementation).
func (s *StdIOServer) sendResponse(clientID string, id *json.RawMessage, result interface{}, err *Error) {
	response := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	jsonResponse, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		s.logger.Printf("Error marshalling response: %v", marshalErr)
		s.sendError(clientID, id, -32603, "Internal error: failed to marshal response", nil)
		return
	}

	jsonResponse = append(jsonResponse, '\n')
	_, writeErr := s.out.Write(jsonResponse) // s.out is the io.Writer
	if writeErr != nil {
		s.logger.Printf("Error writing response to stdout: %v", writeErr)
	}
}

// sendError sends a JSON-RPC error response (StdIO implementation).
func (s *StdIOServer) sendError(clientID string, id *json.RawMessage, code int, message string, data interface{}) {
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
		s.logger.Printf("Error marshaling error response: %v", err)
		return
	}
	jsonErrorResponse = append(jsonErrorResponse, '\n')
	_, writeErr := s.out.Write(jsonErrorResponse) // s.out is the io.Writer
	if writeErr != nil {
		s.logger.Printf("Failed to write error response: %v", writeErr)
	}
}

// sendNotification sends a JSON-RPC notification (StdIO implementation).
func (s *StdIOServer) sendNotification(clientID string, method string, params interface{}) {
	notification := Notification{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			s.logger.Printf("Error marshaling notification parameters: %v", err)
			return
		}
		notification.Params = json.RawMessage(paramsBytes)
	}

	jsonNotification, err := json.Marshal(notification)
	if err != nil {
		s.logger.Printf("Error marshaling notification message: %v", err)
		return
	}
	jsonNotification = append(jsonNotification, '\n')
	_, writeErr := s.out.Write(jsonNotification) // s.out is the io.Writer
	if writeErr != nil {
		s.logger.Printf("Failed to write notification message: %v", writeErr)
	}
}

// Run starts the StdIOServer, reading and processing messages from stdin.
func (s *StdIOServer) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	initialized := false
	done := make(chan error, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			default:
				if !scanner.Scan() {
					if err := scanner.Err(); err != nil && err != io.EOF {
						done <- fmt.Errorf("scanner error: %w", err)
					} else {
						done <- nil
					}
					return
				}

				line := scanner.Text()
				s.logger.Printf("Received raw input: %s", line)

				var raw json.RawMessage
				if err := json.Unmarshal([]byte(line), &raw); err != nil {
					s.sendError("", nil, -32700, "Parse error", nil) // No client ID or request ID for parse errors.
					continue
				}

				var request Request
				if err := json.Unmarshal(raw, &request); err == nil && request.Method != "" && request.ID != nil {
					if request.Method != "initialize" && !initialized {
						s.sendError("", request.ID, -32000, "Server not initialized", nil) // Empty clientID
						continue
					}
					s.handleRequest(ctx, "", &request) // StdIO has no persistent client ID, so we pass an empty string.
					continue
				}

				var notification Notification
				if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
					if notification.Method == "notifications/initialized" {
						initialized = true
					} else if !initialized {
						s.logger.Printf("Received notification before 'initialized': %s", notification.Method)
						continue
					}
					s.handleNotification(nil, "", &notification) // Empty clientID.
					continue
				}

				s.sendError("", nil, -32600, "Invalid Request", nil) // No client ID or request ID.
			}
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Println("Context cancelled, shutting down server...")
		return ctx.Err()
	case err := <-done:
		s.logger.Println("StdIOServer shutting down.")
		return err
	}
}
