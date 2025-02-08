package mcp

import (
	"encoding/json"
)

// Message represents a JSON-RPC 2.0 message
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Err     *Error          `json:"error,omitempty"`
}

func (m Message) Error() string {
	//TODO implement me
	panic("implement me")
}

// Error represents a JSON-RPC 2.0 error object
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// NewRequest creates a new request message
func NewRequest(id interface{}, method string, params interface{}) (*Message, error) {
	var paramsJSON json.RawMessage
	if params != nil {
		bytes, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsJSON = bytes
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}, nil
}

// NewResponse creates a new response message
func NewResponse(id interface{}, result interface{}) *Message {
	return &Message{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates a new error response message
func NewErrorResponse(id interface{}, code int, message string, data interface{}) *Message {
	return &Message{
		JSONRPC: "2.0",
		ID:      id,
		Err: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// Add this to message.go
type MCPError struct {
	*Message
}

func (e *MCPError) Error() string {
	if e.Message != nil && e.Message.Err != nil {
		return e.Message.Err.Message
	}
	return "unknown error"
}

// Helper function to create error
func NewMCPError(code int, message string, data interface{}) error {
	return &MCPError{
		Message: NewErrorResponse(nil, code, message, data),
	}
}
