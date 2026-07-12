package anth2oai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// roundTripper is the L2 http.RoundTripper. It accepts OpenAI Chat Completions
// requests, translates them to Anthropic Messages requests, calls the upstream,
// and translates the response back — for both non-streaming and streaming.
type roundTripper struct {
	cfg  *config
	next http.RoundTripper
	now  func() int64
}

// NewRoundTripper builds the L2 transport. Plug it into any *http.Client or the
// OpenAI SDK via option.WithHTTPClient.
func NewRoundTripper(baseURL, apiKey string, opts ...Option) http.RoundTripper {
	return newRoundTripper(newConfig(baseURL, apiKey, opts...))
}

// newRoundTripper builds the transport from an already-resolved config, so
// callers that already hold a config (e.g. NewHandler) don't rebuild it.
func newRoundTripper(cfg *config) *roundTripper {
	return &roundTripper{
		cfg:  cfg,
		next: cfg.nextTransport(),
		now:  func() int64 { return time.Now().Unix() },
	}
}

// RoundTrip implements http.RoundTripper.
func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(strings.TrimSuffix(req.URL.Path, "/"), "chat/completions") {
		return errorResponse(req, http.StatusNotFound,
			newErrorBody("anth2oai only proxies POST /v1/chat/completions", "not_found_error")), nil
	}

	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("anth2oai: reading request body: %w", err)
		}
		body = b
	}

	var oaiReq oaiRequest
	if err := json.Unmarshal(body, &oaiReq); err != nil {
		return errorResponse(req, http.StatusBadRequest,
			newErrorBody("invalid OpenAI request body: "+err.Error(), "invalid_request_error")), nil
	}

	anthReq, err := translateRequest(&oaiReq, rt.cfg.defaultMaxTokens)
	if err != nil {
		return errorResponse(req, http.StatusBadRequest,
			newErrorBody(err.Error(), "invalid_request_error")), nil
	}

	anthBody, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("anth2oai: marshaling upstream request: %w", err)
	}

	upstreamReq, err := rt.buildUpstreamRequest(req, anthBody, oaiReq.Stream)
	if err != nil {
		return nil, err
	}

	rt.cfg.logger.WithFields(map[string]interface{}{
		"model":  anthReq.Model,
		"stream": anthReq.Stream,
		"url":    upstreamReq.URL.String(),
	}).Debug("anth2oai: forwarding to Anthropic upstream")

	resp, err := rt.next.RoundTrip(upstreamReq)
	if err != nil {
		return nil, fmt.Errorf("anth2oai: upstream request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return rt.errorFromUpstream(req, resp), nil
	}

	if oaiReq.Stream {
		return rt.streamResponse(req, resp, oaiReq.StreamOptions), nil
	}
	return rt.nonStreamResponse(req, resp)
}

// buildUpstreamRequest constructs the Anthropic /v1/messages request.
func (rt *roundTripper) buildUpstreamRequest(orig *http.Request, body []byte, stream bool) (*http.Request, error) {
	url := strings.TrimRight(rt.cfg.baseURL, "/") + "/v1/messages"
	up, err := http.NewRequestWithContext(orig.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anth2oai: building upstream request: %w", err)
	}
	up.Header.Set("Content-Type", "application/json")
	up.Header.Set("anthropic-version", rt.cfg.anthropicVersion)
	if rt.cfg.authKind == AuthXAPIKey {
		up.Header.Set("x-api-key", rt.cfg.apiKey)
	} else {
		up.Header.Set("Authorization", "Bearer "+rt.cfg.apiKey)
	}
	if stream {
		up.Header.Set("Accept", "text/event-stream")
	} else {
		up.Header.Set("Accept", "application/json")
	}
	up.ContentLength = int64(len(body))
	return up, nil
}

// errorFromUpstream reads an upstream error body and maps it to OpenAI shape.
func (rt *roundTripper) errorFromUpstream(req *http.Request, resp *http.Response) *http.Response {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return errorResponse(req, resp.StatusCode, mapAnthropicError(resp.StatusCode, body))
}

// nonStreamResponse parses the Anthropic Message and returns an OpenAI body.
func (rt *roundTripper) nonStreamResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return errorResponse(req, http.StatusBadGateway,
			newErrorBody("reading upstream response: "+err.Error(), "api_error")), nil
	}
	var anthResp anthResponse
	if err := json.Unmarshal(raw, &anthResp); err != nil {
		return errorResponse(req, http.StatusBadGateway,
			newErrorBody("malformed upstream response: "+err.Error(), "api_error")), nil
	}
	oaiResp := translateResponse(&anthResp, rt.now())
	out, err := json.Marshal(oaiResp)
	if err != nil {
		return nil, fmt.Errorf("anth2oai: marshaling response: %w", err)
	}
	return jsonResponse(req, http.StatusOK, out), nil
}

// streamResponse returns a response whose body lazily transforms the upstream
// Anthropic SSE into OpenAI chunk SSE via an io.Pipe + goroutine, preserving
// first-token latency.
func (rt *roundTripper) streamResponse(req *http.Request, resp *http.Response, so *oaiStreamOptions) *http.Response {
	includeUsage := so != nil && so.IncludeUsage
	pr, pw := io.Pipe()

	go rt.pumpStream(resp, pw, includeUsage)

	h := make(http.Header)
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     h,
		Body:       pr,
		Request:    req,
	}
}

// pumpStream reads the Anthropic SSE, transforms each event, and writes OpenAI
// SSE frames to pw. It always closes pw.
func (rt *roundTripper) pumpStream(resp *http.Response, pw *io.PipeWriter, includeUsage bool) {
	defer resp.Body.Close()

	transformer := newStreamTransformer(rt.now(), includeUsage)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var data strings.Builder
	dispatch := func() error {
		if data.Len() == 0 {
			return nil
		}
		payload := data.String()
		data.Reset()
		var ev anthStreamEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			rt.cfg.logger.WithFields(map[string]interface{}{
				"error": err.Error(),
			}).Warn("anth2oai: skipping malformed upstream SSE event")
			return nil
		}
		frames, done, herr := transformer.handle(&ev)
		for _, f := range frames {
			if _, werr := pw.Write(f); werr != nil {
				return werr
			}
		}
		if herr != nil {
			return herr
		}
		if done {
			return io.EOF
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				rt.finishStream(pw, err)
				return
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
		// event:, id:, retry:, and comments (":") are ignored.
	}

	// Flush a trailing event with no terminating blank line.
	if err := dispatch(); err != nil {
		rt.finishStream(pw, err)
		return
	}
	if err := scanner.Err(); err != nil {
		rt.finishStream(pw, err)
		return
	}
	rt.finishStream(pw, nil)
}

// finishStream closes the pipe. On a real transport error (not the io.EOF used
// to signal a clean [DONE]) it makes sure the client sees a terminator.
func (rt *roundTripper) finishStream(pw *io.PipeWriter, err error) {
	switch {
	case err == nil || err == io.EOF:
		_ = pw.Close()
	default:
		rt.cfg.logger.WithFields(map[string]interface{}{
			"error": err.Error(),
		}).Error("anth2oai: streaming aborted")
		// Best-effort terminator so an OpenAI SSE reader unblocks.
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
		_ = pw.Close()
	}
}
