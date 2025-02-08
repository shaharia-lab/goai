package mcp

type Transport interface {
	Start() error
	Stop() error
	HandleMessage(handler MessageHandler)
}
