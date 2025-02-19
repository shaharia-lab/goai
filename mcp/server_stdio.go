package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai/observability"
	"go.opentelemetry.io/otel/codes"
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
		s.logger.WithErr(marshalErr).Error("Failed to marshal response")
		s.sendError(clientID, id, -32603, "Internal error: failed to marshal response", nil)
		return
	}

	jsonResponse = append(jsonResponse, '\n')
	_, writeErr := s.out.Write(jsonResponse)
	if writeErr != nil {
		s.logger.WithErr(writeErr).Error("Failed to write response")
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
		s.logger.WithErr(err).Error("Failed to marshal error response")
		return
	}
	jsonErrorResponse = append(jsonErrorResponse, '\n')
	_, writeErr := s.out.Write(jsonErrorResponse)
	if writeErr != nil {
		s.logger.WithErr(writeErr).Error("Failed to write error response")
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
			s.logger.WithErr(err).Error("Failed to marshal notification parameters")
			return
		}
		notification.Params = paramsBytes
	}

	jsonNotification, err := json.Marshal(notification)
	if err != nil {
		s.logger.WithErr(err).Error("Failed to marshal notification message")
		return
	}
	jsonNotification = append(jsonNotification, '\n')
	_, writeErr := s.out.Write(jsonNotification) // s.out is the io.Writer
	if writeErr != nil {
		s.logger.WithErr(writeErr).Error("Failed to write notification message")
	}
}

// Run starts the StdIOServer, reading and processing messages from stdin.
func (s *StdIOServer) Run(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx, "StdIOServer.Run")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

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

				var raw json.RawMessage
				if err := json.Unmarshal([]byte(line), &raw); err != nil {
					s.logger.WithErr(err).Error("Failed to unmarshal message")
					s.sendError("", nil, -32700, "Failed to unmarshal message", nil)
					continue
				}

				var request Request
				if err = json.Unmarshal(raw, &request); err == nil && request.Method != "" && request.ID != nil {
					if request.Method != "initialize" && !initialized {
						s.logger.Error("Received request before 'initialize'")
						s.sendError("", request.ID, -32000, "Server not initialized", nil) // Empty clientID
						continue
					}

					s.logger.Debug("Received request. Handling...")
					s.handleRequest(ctx, "", &request) // StdIO has no persistent client ID, so we pass an empty string.
					continue
				}

				var notification Notification
				if err := json.Unmarshal(raw, &notification); err == nil && notification.Method != "" {
					if notification.Method == "notifications/initialized" {
						initialized = true
					} else if !initialized {
						s.logger.Debug("Received notification before 'initialized'")
						continue
					}
					s.handleNotification(ctx, "", &notification) // Empty clientID.
					continue
				}

				s.sendError("", nil, -32600, "Invalid Request", nil) // No client ID or request ID.
			}
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Debug("Context cancelled, StdIOServer shutting down....")
		return ctx.Err()
	case err = <-done:
		s.logger.WithErr(err).Debug("StdIOServer shutting down.")
		return err
	}
}
