package anth2oai

import (
	"net/http"
	"strings"
)

// NewHandler builds an http.Handler (the L4 proxy) that accepts OpenAI Chat
// Completions requests on POST /v1/chat/completions and forwards them, through
// the L2 transport, to the Anthropic endpoint at baseURL — reusing all of the
// translation logic verbatim.
func NewHandler(baseURL, apiKey string, opts ...Option) http.Handler {
	cfg := newConfig(baseURL, apiKey, opts...)
	return &handler{
		rt:  NewRoundTripper(baseURL, apiKey, opts...),
		cfg: cfg,
	}
}

type handler struct {
	rt  http.RoundTripper
	cfg *config
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed,
			newErrorBody("method not allowed; use POST", "invalid_request_error"))
		return
	}
	if !isChatCompletionsPath(r.URL.Path) {
		writeJSON(w, http.StatusNotFound,
			newErrorBody("unknown route; POST /v1/chat/completions", "not_found_error"))
		return
	}
	defer func() {
		if r.Body != nil {
			_ = r.Body.Close()
		}
	}()

	// Route through the L2 transport. The transport builds its own upstream URL
	// from the configured baseURL, so only the path (checked here) matters.
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"http://anth2oai.local/v1/chat/completions", r.Body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError,
			newErrorBody("internal error: "+err.Error(), "api_error"))
		return
	}
	upReq.Header.Set("Content-Type", "application/json")

	resp, err := h.rt.RoundTrip(upReq)
	if err != nil {
		h.cfg.logger.WithFields(map[string]interface{}{"error": err.Error()}).
			Error("anth2oai: handler upstream error")
		writeJSON(w, http.StatusBadGateway,
			newErrorBody("upstream error: "+err.Error(), "api_error"))
		return
	}
	defer resp.Body.Close()

	copyResponse(w, resp)
}

// copyResponse streams resp to w, flushing after each write so SSE frames reach
// the client incrementally.
func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 16*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func isChatCompletionsPath(p string) bool {
	return strings.HasSuffix(strings.TrimRight(p, "/"), "chat/completions")
}
