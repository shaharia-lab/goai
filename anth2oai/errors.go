package anth2oai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// oaiErrorEnvelope is the OpenAI error response shape: {"error": {...}}.
type oaiErrorEnvelope struct {
	Error oaiErrorBody `json:"error"`
}

type oaiErrorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Code    *string `json:"code"`
	Param   *string `json:"param"`
}

// mapAnthropicError translates an upstream Anthropic error body into OpenAI
// error JSON. It preserves the upstream message and type when present and falls
// back to the raw body as the message otherwise.
func mapAnthropicError(status int, body []byte) []byte {
	env := oaiErrorEnvelope{Error: oaiErrorBody{Type: defaultErrorType(status)}}

	var anthEnv anthErrorEnvelope
	if err := json.Unmarshal(body, &anthEnv); err == nil && anthEnv.Error != nil && anthEnv.Error.Message != "" {
		env.Error.Message = anthEnv.Error.Message
		if anthEnv.Error.Type != "" {
			env.Error.Type = anthEnv.Error.Type
		}
	} else if trimmed := bytes.TrimSpace(body); len(trimmed) > 0 {
		env.Error.Message = string(trimmed)
	} else {
		env.Error.Message = http.StatusText(status)
	}

	out, err := json.Marshal(env)
	if err != nil {
		return []byte(`{"error":{"message":"upstream error","type":"api_error"}}`)
	}
	return out
}

// defaultErrorType maps an HTTP status to an OpenAI-style error type.
func defaultErrorType(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	default:
		if status >= 500 {
			return "api_error"
		}
		return "invalid_request_error"
	}
}

// newErrorBody builds an OpenAI error JSON body from scratch.
func newErrorBody(message, errType string) []byte {
	out, err := json.Marshal(oaiErrorEnvelope{Error: oaiErrorBody{Message: message, Type: errType}})
	if err != nil {
		return []byte(`{"error":{"message":"internal error","type":"api_error"}}`)
	}
	return out
}

// errorResponse constructs an *http.Response carrying an OpenAI error body.
func errorResponse(req *http.Request, status int, body []byte) *http.Response {
	return jsonResponse(req, status, body)
}

// jsonResponse builds an *http.Response with a JSON body.
func jsonResponse(req *http.Request, status int, body []byte) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
