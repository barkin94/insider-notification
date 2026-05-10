package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Option func(*Client)

func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.http.Timeout = d
	}
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HTTP returns the underlying *http.Client (useful for testing).
func (c *Client) HTTP() *http.Client { return c.http }

// Request sends an HTTP request to baseURL+path with payload marshaled as JSON.
// Payload may be nil for requests with no body. Logs the request and response
// via slog, and returns a non-nil error on both network failures and
// unsuccessful status codes are left to the caller to interpret.
func (c *Client) Request(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	slog.InfoContext(ctx, "http request", "method", method, "url", url)

	resp, err := c.http.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "http request failed", "method", method, "url", url, "error", err)
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}

	slog.InfoContext(ctx, "http response", "method", method, "url", url, "status", resp.StatusCode)
	return resp, nil
}
