package main

import (
	"github.com/shaharia-lab/goai/mcp"
)

func main() {
	client := mcp.NewSSEClient(mcp.SSEClientConfig{
		URL: "http://localhost:8080/events",
	})

	done := make(chan struct{})
	go func() {
		client.Start()
		close(done)
	}()

	<-done
}
