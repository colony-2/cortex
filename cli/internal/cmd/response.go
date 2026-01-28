package cmd

import (
	"fmt"
	"net/http"
)

// expectStatus ensures the HTTP status code is one of the allowed values; otherwise
// returns an error that includes method, URL, and a small excerpt of the response body.
func expectStatus(resp *http.Response, body []byte, ok ...int) error {
	status := 0
	method := ""
	url := ""
	if resp != nil {
		status = resp.StatusCode
		if resp.Request != nil {
			method = resp.Request.Method
			url = resp.Request.URL.String()
		}
	}
	for _, v := range ok {
		if status == v {
			return nil
		}
	}
	return fmt.Errorf("unexpected status %d %s %s: %s", status, method, url, previewBody(body))
}

// requirePayload ensures status is OK and payload is non-nil, otherwise returns a detailed error.
func requirePayload[T any](payload *T, resp *http.Response, body []byte, ok ...int) (*T, error) {
	if err := expectStatus(resp, body, ok...); err != nil {
		return nil, err
	}
	if payload == nil {
		method, url := "", ""
		if resp != nil && resp.Request != nil {
			method = resp.Request.Method
			url = resp.Request.URL.String()
		}
		return nil, fmt.Errorf("empty response body %s %s: %s", method, url, previewBody(body))
	}
	return payload, nil
}

// previewBody returns a trimmed string representation of a response body for error messages.
func previewBody(body []byte) string {
	if len(body) == 0 {
		return "no body"
	}
	const limit = 512
	if len(body) > limit {
		body = body[:limit]
	}
	return string(body)
}
