package mcp

import (
	"context"
	"encoding/json"
)

type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// InitializeResult represents the result of server initialization.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// ServerInfo represents server information.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params"`
}

// Response represents a JSON-RPC response message.
type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

// Error represents a JSON-RPC error object.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Notification represents a JSON-RPC notification message.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Messages    []PromptMessage  `json:"messages,omitempty"`
}

type PromptGetResponse struct {
	Description string          `json:"description"`
	Messages    []PromptMessage `json:"messages"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

type PromptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

type GetPromptParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ListResourcesResult represents the result of listing resources.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceParams represents parameters for reading a resource.
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// ReadResourceResult represents the result of reading a resource.
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// LogLevel represents the severity level of a log message.
// The levels follow standard syslog severity levels.
type LogLevel string

const (
	LogLevelDebug     LogLevel = "debug"
	LogLevelInfo      LogLevel = "info"
	LogLevelNotice    LogLevel = "notice"
	LogLevelWarning   LogLevel = "warning"
	LogLevelError     LogLevel = "error"
	LogLevelCritical  LogLevel = "critical"
	LogLevelAlert     LogLevel = "alert"
	LogLevelEmergency LogLevel = "emergency"
)

// logLevelSeverity maps LogLevel to their numeric severity values.
// Lower numbers indicate higher severity (0 is most severe).
var logLevelSeverity = map[LogLevel]int{
	LogLevelDebug:     7,
	LogLevelInfo:      6,
	LogLevelNotice:    5,
	LogLevelWarning:   4,
	LogLevelError:     3,
	LogLevelCritical:  2,
	LogLevelAlert:     1,
	LogLevelEmergency: 0,
}

// SetLogLevelParams represents the parameters for setting the log level.
type SetLogLevelParams struct {
	Level LogLevel `json:"level"`
}

// LogMessageParams represents the parameters for logging a message.
type LogMessageParams struct {
	Level  LogLevel    `json:"level"`
	Logger string      `json:"logger,omitempty"`
	Data   interface{} `json:"data"`
}

// Resource represents a content resource in the MCP system.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        int    `json:"size,omitempty"`
	TextContent string `json:"-"`
}

// ResourceContent represents the actual content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// Tool represents a callable tool in the MCP system.
type Tool struct {
	Name        string                                                                   `json:"name"`
	Description string                                                                   `json:"description"`
	InputSchema json.RawMessage                                                          `json:"inputSchema"`
	Handler     func(ctx context.Context, params CallToolParams) (CallToolResult, error) `json:"-"`
}

// ToolResultContent represents the content returned by a tool.
type ToolResultContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallToolParams represents parameters for calling a tool.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// CallToolResult represents the result of calling a tool.
type CallToolResult struct {
	Content []ToolResultContent `json:"content"`
	IsError bool                `json:"isError"`
}

// ListToolsResult represents the result of listing available tools.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type ListParams struct {
	Cursor string `json:"cursor"`
}

type ToolImplementation func(args json.RawMessage) (CallToolResult, error)

type ToolHandler interface {
	GetName() string
	GetDescription() string
	GetInputSchema() json.RawMessage
	Handler(params CallToolParams) (CallToolResult, error)
}
