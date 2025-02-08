package mcp

import "time"

type Transport interface {
	Start() error
	Stop() error
	HandleMessage(handler MessageHandler)
	SendMessage(msg Message) error
}

type TransportConfig struct {
	AuthManager      *AuthManager
	AllowedOrigins   []string
	MaxRequestSize   int64
	HandshakeTimeout time.Duration
}
