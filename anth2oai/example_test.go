package anth2oai_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai/anth2oai"
)

// Example shows the core use: drive an Anthropic-format endpoint with the normal
// OpenAI SDK. A stub HTTP server stands in for the real upstream (e.g.
// https://api.z.ai/api/anthropic) so the example is self-contained and runnable.
func Example() {
	// Stand-in for the Anthropic upstream: replies with an Anthropic Message.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"glm-4.5-air",`+
			`"content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn",`+
			`"usage":{"input_tokens":8,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	// New returns a plain *openai.Client wired to the Anthropic endpoint.
	client := anth2oai.New(upstream.URL, "test-key")

	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("glm-4.5-air"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Reply with exactly: OK"),
		}),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Choices[0].Message.Content)
	// Output: OK
}

// ExampleNew is the idiomatic real-world call against a live GLM/Z.ai endpoint,
// with the upstream model id passed through verbatim.
func ExampleNew() {
	client := anth2oai.New("https://api.z.ai/api/anthropic", os.Getenv("ANTHROPIC_AUTH_TOKEN"))

	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("glm-4.5-air"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are terse."),
			openai.UserMessage("Reply with exactly: OK"),
		}),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Choices[0].Message.Content)
}

// ExampleNew_streaming streams tokens through the same OpenAI SDK surface.
func ExampleNew_streaming() {
	client := anth2oai.New("https://api.z.ai/api/anthropic", os.Getenv("ANTHROPIC_AUTH_TOKEN"))

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("glm-4.5-air"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Count from one to five."),
		}),
	})
	defer stream.Close()

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		log.Fatal(err)
	}
}

// ExampleNew_tools shows tool/function calling: the OpenAI tool definition is
// translated to an Anthropic tool, and tool_use results come back as tool_calls.
func ExampleNew_tools() {
	client := anth2oai.New("https://api.z.ai/api/anthropic", os.Getenv("ANTHROPIC_AUTH_TOKEN"))

	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("glm-4.5-air"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("What is the weather in Paris?"),
		}),
		Tools: openai.F([]openai.ChatCompletionToolParam{{
			Type: openai.F(openai.ChatCompletionToolTypeFunction),
			Function: openai.F(openai.FunctionDefinitionParam{
				Name:        openai.F("get_weather"),
				Description: openai.F("Get the current weather for a location"),
				Parameters: openai.F(openai.FunctionParameters{
					"type":       "object",
					"properties": map[string]any{"location": map[string]any{"type": "string"}},
					"required":   []string{"location"},
				}),
			}),
		}}),
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, call := range resp.Choices[0].Message.ToolCalls {
		fmt.Printf("%s(%s)\n", call.Function.Name, call.Function.Arguments)
	}
}

// ExampleNewRoundTripper plugs the translation into a bare *http.Client, without
// the OpenAI SDK. The client then speaks OpenAI Chat Completions to the upstream.
func ExampleNewRoundTripper() {
	rt := anth2oai.NewRoundTripper("https://api.z.ai/api/anthropic", os.Getenv("ANTHROPIC_AUTH_TOKEN"))
	httpClient := &http.Client{Transport: rt}

	// POST an OpenAI-shaped body to any path ending in /chat/completions.
	_ = httpClient
}

// ExampleNewHandler exposes the Anthropic endpoint as an OpenAI-compatible HTTP
// proxy, so non-Go clients can reach it.
func ExampleNewHandler() {
	handler := anth2oai.NewHandler("https://api.z.ai/api/anthropic", os.Getenv("ANTHROPIC_AUTH_TOKEN"))

	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// ExampleWithAuthHeader targets an endpoint that requires the native Anthropic
// "x-api-key" scheme instead of the default Bearer token, and raises the default
// max_tokens.
func ExampleWithAuthHeader() {
	client := anth2oai.New(
		"https://api.anthropic.com",
		os.Getenv("ANTHROPIC_API_KEY"),
		anth2oai.WithAuthHeader(anth2oai.AuthXAPIKey),
		anth2oai.WithDefaultMaxTokens(1024),
	)
	_ = client
}
