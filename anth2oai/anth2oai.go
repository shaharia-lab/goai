package anth2oai

import (
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// New returns an *openai.Client that speaks the OpenAI Chat Completions API but
// is wired, through the L2 transport, to an Anthropic Messages endpoint at
// baseURL. Callers then use the normal OpenAI SDK surface unchanged:
//
//	client := anth2oai.New("https://api.z.ai/api/anthropic", token)
//	resp, err := client.Chat.Completions.New(ctx, params)   // non-streaming
//	stream   := client.Chat.Completions.NewStreaming(ctx, params) // streaming
func New(baseURL, apiKey string, opts ...Option) *openai.Client {
	rt := NewRoundTripper(baseURL, apiKey, opts...)
	return openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
		option.WithHTTPClient(&http.Client{Transport: rt}),
	)
}
