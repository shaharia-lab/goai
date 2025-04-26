// Example:
//
// This example demonstrates how to set up a basic JSON-based server using
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
//
//		"log"
//		"os"
//	)
//
//	func main() {
//		// Define a custom tool for fetching weather.
//		weatherTool := Tool{
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
//			Handler: func(ctx context.Context, params CallToolParams) (CallToolResult, error) {
//				var input struct {
//					Location string `json:"location"`
//				}
//				if err := json.Unmarshal(params.Arguments, &input); err != nil {
//					return CallToolResult{}, err
//				}
//
//				return CallToolResult{
//					Content: []ToolResultContent{
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
//		resources := []Resource{
//			{
//				URI:		 "file:///tmp/hello_world.txt",
//				Name:		"Hello World",
//				Description: "A sample text document for testing",
//				MimeType:	"text/plain",
//				TextContent: "This is the content of the sample document.",
//			},
//		}
//
//		prompt := Prompt{
//			Name:		"code_review",
//			Description: "Performs a detailed code review and suggesting improvements",
//			Arguments: []PromptArgument{
//				{
//					Name:		"code",
//					Description: "Source code to be reviewed",
//					Required:	true,
//				},
//			},
//			Messages: []PromptMessage{
//				{
//					Role: "user",
//					Content: PromptContent{
//						Type: "text",
//						Text: "Please review this code: {{code}}",
//					},
//				},
//			},
//		}
//
//		baseServer, err := NewBaseServer(
//			UseLogger(NewNullLogger()),
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
//		server := NewStdIOServer(
//			baseServer,
//			os.Stdin,
//			os.Stdout,
//		)
//		ctx := context.Background()
//		if err := server.Run(ctx); err != nil {
//			panic(err)
//		}
//	}
package goai
