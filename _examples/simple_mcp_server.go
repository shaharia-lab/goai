package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai"
)

func main() {
	// Create base server
	baseServer, err := goai.NewBaseServer(
		goai.UseLogger(goai.NewDefaultLogger()),
	)
	if err != nil {
		panic(err)
	}

	// Add tool
	tool := goai.Tool{
		Name:        "greet",
		Description: "Greet user",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "name": {"type": "string"}
            },
            "required": ["name"]
        }`),
		Handler: func(ctx context.Context, params goai.CallToolParams) (goai.CallToolResult, error) {
			var input struct {
				Name string `json:"name"`
			}
			json.Unmarshal(params.Arguments, &input)
			return goai.CallToolResult{
				Content: []goai.ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Hello, %s!", input.Name),
				}},
			}, nil
		},
	}
	baseServer.AddTools(tool)

	// Create and run SSE server
	server := goai.NewSSEServer(baseServer)
	server.SetAddress(":8080")

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
