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
			if err := json.Unmarshal(params.Arguments, &input); err != nil {
				return goai.CallToolResult{}, err
			}
			return goai.CallToolResult{
				Content: []goai.ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Hello, %s!", input.Name),
				}},
			}, nil
		},
	}
	if err := baseServer.AddTools(tool); err != nil {
		panic(err)
	}

	// Create and run SSE server
	server := goai.NewSSEServer(baseServer)
	server.SetAddress(":8080")

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
