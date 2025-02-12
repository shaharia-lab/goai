// Package mcp demonstrates the usage of the mcp library, including tools, prompts,
// resources, and server setup. It includes an example implementation of a "WeatherTool"
// to fetch weather data, and integrates the tool with a JSON-based API server.
//
// Example:
//
// This example demonstrates how to set up a basic JSON-based server using mcp,
// add a custom tool for weather information retrieval, display a resource, and add a prompt.
//
// The "WeatherTool" fetches the weather based on location input.
//
//	package main
//
//	import (
//		"context"
//		"encoding/json"
//		"fmt"
//		"github.com/shaharia-lab/goai/mcp"
//		"log"
//		"os"
//	)
//
//	func main() {
//		// Define a custom tool for fetching weather.
//		weatherTool := mcp.Tool{
//			Name:		"get_weather",
//			Description: "Get the current weather for a given location.",
//			InputSchema: json.RawMessage(`{
//						"type": "object",
//						"properties": {
//							"location": {
//								"type": "string",
//								"description": "The city and state, e.g. San Francisco, CA"
//							}
//						},
//						"required": ["location"]
//					}`),
//			Handler: func(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
//				var input struct {
//					Location string `json:"location"`
//				}
//				if err := json.Unmarshal(params.Arguments, &input); err != nil {
//					return mcp.CallToolResult{}, err
//				}
//
//				return mcp.CallToolResult{
//					Content: []mcp.ToolResultContent{
//						{
//							Type: "text",
//							Text: fmt.Sprintf("Weather in %s: Sunny, 72°F", input.Location),
//						},
//					},
//				}, nil
//			},
//		}
//
//		// Define resources, prompts, and configure the base server.
//		resources := []mcp.Resource{
//			{
//				URI:		 "file:///tmp/hello_world.txt",
//				Name:		"Hello World",
//				Description: "A sample text document for testing",
//				MimeType:	"text/plain",
//				TextContent: "This is the content of the sample document.",
//			},
//		}
//
//		prompt := mcp.Prompt{
//			Name:		"code_review",
//			Description: "Performs a detailed code review and suggesting improvements",
//			Arguments: []mcp.PromptArgument{
//				{
//					Name:		"code",
//					Description: "Source code to be reviewed",
//					Required:	true,
//				},
//			},
//			Messages: []mcp.PromptMessage{
//				{
//					Role: "user",
//					Content: mcp.PromptContent{
//						Type: "text",
//						Text: "Please review this code: {{code}}",
//					},
//				},
//			},
//		}
//
//		baseServer, err := mcp.NewBaseServer(
//			mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
//		)
//		if err != nil {
//			panic(err)
//		}
//
//		err = baseServer.AddTools(weatherTool)
//		if err != nil {
//			panic(err)
//		}
//
//		err = baseServer.AddPrompts(prompt)
//		if err != nil {
//			panic(err)
//		}
//
//		err = baseServer.AddResources(resources...)
//		if err != nil {
//			panic(err)
//		}
//
//		server := mcp.NewStdIOServer(
//			baseServer,
//			os.Stdin,
//			os.Stdout,
//		)
//		ctx := context.Background()
//		if err := server.Run(ctx); err != nil {
//			panic(err)
//		}
//	}
package mcp
