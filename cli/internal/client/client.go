package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/colony-2/colony2/cli/internal/config"
	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

// New returns an OpenAPI client configured with auth and timeout.
func New(cfg config.Config) (openapi.ClientWithResponsesInterface, error) {
	if cfg.APIURL == "" {
		return nil, fmt.Errorf("api url is required")
	}
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}
	if cfg.Trace {
		httpClient.Transport = traceTransport{next: http.DefaultTransport}
	}

	opts := []openapi.ClientOption{
		openapi.WithHTTPClient(httpClient),
		openapi.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			if cfg.Token != "" {
				req.Header.Set("Authorization", "Bearer "+cfg.Token)
			}
			return nil
		}),
	}

	cl, err := openapi.NewClientWithResponses(cfg.APIURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return cl, nil
}

// Context applies a timeout to the command context.
func Context(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

type traceTransport struct {
	next http.RoundTripper
}

func (t traceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.next == nil {
		t.next = http.DefaultTransport
	}
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[trace] %s %s error: %v\n", req.Method, req.URL, err)
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[trace] %s %s -> %d (%s)\n", req.Method, req.URL, resp.StatusCode, time.Since(start).Truncate(time.Millisecond))
	return resp, nil
}
