package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shaharia-lab/goai"
)

func main() {
	logger := goai.NewDefaultLogger()

	tool := goai.Tool{
		Name:        "get_weather",
		Description: "Get weather for location",
		InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "location": {"type": "string"}
        },
        "required": ["location"]
    }`),
		Handler: func(ctx context.Context, params goai.CallToolParams) (goai.CallToolResult, error) {
			var input struct {
				Location string `json:"location"`
			}
			json.Unmarshal(params.Arguments, &input)
			return goai.CallToolResult{
				Content: []goai.ToolResultContent{{
					Type: "text",
					Text: fmt.Sprintf("Weather in %s: Sunny", input.Location),
				}},
			}, nil
		},
	}

	baseServer, err := goai.NewBaseServer(
		goai.UseLogger(logger),
	)
	if err != nil {
		panic(err)
	}
	baseServer.AddTools(tool)

	server := goai.NewSSEServer(baseServer)
	server.SetAddress(":8080")
	ctx := context.Background()
	server.Run(ctx)
}
