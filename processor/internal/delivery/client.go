package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/barkin/insider-notification/shared/model"
)

// Result holds the outcome of a single delivery attempt.
type Result struct {
	Success       bool
	Retryable     bool
	StatusCode    int
	LatencyMS     int64
	ProviderMsgID string
	ErrorMessage  string
}

// Client delivers a notification to the webhook provider.
type Client interface {
	Send(ctx context.Context, n *model.Notification) (Result, error)
}

type httpClient struct {
	http       *http.Client
	webhookURL string
}

// NewClient returns a delivery Client that POSTs to webhookURL with the given timeout.
// At the observability task, wrap with otelhttp.NewTransport to add trace spans.
func NewClient(webhookURL string, timeout time.Duration) Client {
	return &httpClient{
		http:       &http.Client{Timeout: timeout},
		webhookURL: webhookURL,
	}
}

type requestBody struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Content   string `json:"content"`
}

func (c *httpClient) Send(ctx context.Context, n *model.Notification) (Result, error) {
	body, err := json.Marshal(requestBody{
		ID:        n.ID.String(),
		Channel:   n.Channel,
		Recipient: n.Recipient,
		Content:   n.Content,
	})
	if err != nil {
		return Result{}, fmt.Errorf("marshal delivery body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.http.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		slog.ErrorContext(ctx, "delivery request failed",
			"notification_id", n.ID,
			"latency_ms", latency,
			"error", err,
		)
		return Result{Retryable: true, LatencyMS: latency, ErrorMessage: err.Error()}, nil
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	result := Result{StatusCode: code, LatencyMS: latency}

	switch {
	case code == http.StatusAccepted:
		result.Success = true
	case code == http.StatusBadRequest || code == http.StatusUnauthorized || code == http.StatusForbidden:
		result.ErrorMessage = fmt.Sprintf("non-retryable provider error: %d", code)
	default:
		result.Retryable = true
		result.ErrorMessage = fmt.Sprintf("retryable provider error: %d", code)
	}

	slog.InfoContext(ctx, "delivery response",
		"notification_id", n.ID,
		"status", code,
		"latency_ms", latency,
		"success", result.Success,
		"retryable", result.Retryable,
	)
	return result, nil
}
