package main

import (
	"context"
	"encoding/json"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"os"
)

/*func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	flag.Parse()

	server := mcp.NewSSEServer(mcp.NewServerBuilder(
		mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
	))
	server.SetAddress(*addr)

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}*/

func main() {
	tool := []mcp.Tool{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a given location.",
			InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {
							"type": "string",
							"description": "The city and state, e.g. San Francisco, CA"
						}
					},
					"required": ["location"]
				}`),
		},
	}
	server := mcp.NewStdIOServer(
		mcp.NewServerBuilder(
			mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
			mcp.UseTools(tool...),
		),
		os.Stdin,
		os.Stdout,
	)
	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
