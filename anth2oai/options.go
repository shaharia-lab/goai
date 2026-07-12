package anth2oai

import (
	"net/http"

	"github.com/shaharia-lab/goai"
)

// AuthKind selects how the upstream credential is presented to the Anthropic endpoint.
type AuthKind int

const (
	// AuthBearer sends the credential as "Authorization: Bearer <apiKey>".
	// This is the default and is validated against GLM/Z.ai's Anthropic endpoint,
	// which Claude Code reaches via ANTHROPIC_AUTH_TOKEN as a bearer token.
	AuthBearer AuthKind = iota
	// AuthXAPIKey sends the credential as "x-api-key: <apiKey>", the native
	// Anthropic authentication scheme. Use it for providers that require it.
	AuthXAPIKey
)

const (
	defaultMaxTokens       int64  = 4096
	defaultAnthropicHeader string = "2023-06-01"
)

// config holds the resolved settings for a RoundTripper / client / handler.
type config struct {
	baseURL          string
	apiKey           string
	defaultMaxTokens int64
	anthropicVersion string
	authKind         AuthKind
	httpClient       *http.Client
	baseTransport    http.RoundTripper
	logger           goai.Logger
}

// Option customises a config using the functional-options pattern used across goai.
type Option func(*config)

func newConfig(baseURL, apiKey string, opts ...Option) *config {
	c := &config{
		baseURL:          baseURL,
		apiKey:           apiKey,
		defaultMaxTokens: defaultMaxTokens,
		anthropicVersion: defaultAnthropicHeader,
		authKind:         AuthBearer,
		logger:           goai.NewNullLogger(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.logger == nil {
		c.logger = goai.NewNullLogger()
	}
	if c.defaultMaxTokens <= 0 {
		c.defaultMaxTokens = defaultMaxTokens
	}
	if c.anthropicVersion == "" {
		c.anthropicVersion = defaultAnthropicHeader
	}
	// Surface misconfiguration at construction rather than deep inside a request.
	// The constructors return concrete types (not errors), so we warn rather than fail.
	if c.baseURL == "" {
		c.logger.Warn("anth2oai: empty baseURL; upstream requests will fail")
	}
	if c.apiKey == "" {
		c.logger.Warn("anth2oai: empty apiKey; the upstream will reject requests")
	}
	return c
}

// nextTransport resolves the http.RoundTripper used to reach the upstream.
func (c *config) nextTransport() http.RoundTripper {
	if c.baseTransport != nil {
		return c.baseTransport
	}
	if c.httpClient != nil && c.httpClient.Transport != nil {
		return c.httpClient.Transport
	}
	return http.DefaultTransport
}

// WithDefaultMaxTokens sets the max_tokens value applied when the OpenAI caller
// omits both max_tokens and max_completion_tokens. Anthropic requires max_tokens.
func WithDefaultMaxTokens(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.defaultMaxTokens = int64(n)
		}
	}
}

// WithAnthropicVersion overrides the "anthropic-version" header (default 2023-06-01).
func WithAnthropicVersion(v string) Option {
	return func(c *config) { c.anthropicVersion = v }
}

// WithAuthHeader selects the upstream authentication scheme (Bearer default, or x-api-key).
func WithAuthHeader(kind AuthKind) Option {
	return func(c *config) { c.authKind = kind }
}

// WithHTTPClient sets the underlying *http.Client whose Transport is used for
// upstream calls. Only its Transport is consumed; timeouts/redirect policy are
// governed by the client the caller drives (e.g. the OpenAI SDK client).
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.httpClient = client }
}

// WithLogger sets the goai.Logger. A null logger is used by default.
func WithLogger(l goai.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithBaseTransport sets the http.RoundTripper used to reach the upstream,
// bypassing WithHTTPClient. Intended for testing and composition (e.g. a stub
// transport returning canned Anthropic responses).
func WithBaseTransport(rt http.RoundTripper) Option {
	return func(c *config) { c.baseTransport = rt }
}
