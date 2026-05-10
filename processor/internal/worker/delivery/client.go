package delivery

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/barkin/insider-notification/shared/httpclient"
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

type webhookClient struct {
	http *httpclient.Client
}

// NewClient returns a delivery Client that POSTs to webhookURL with the given timeout.
func NewClient(webhookURL string, timeout time.Duration) Client {
	return &webhookClient{
		http: httpclient.New(webhookURL, httpclient.WithTimeout(timeout)),
	}
}

type requestBody struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Content   string `json:"content"`
}

func (c *webhookClient) Send(ctx context.Context, n *model.Notification) (Result, error) {
	start := time.Now()
	resp, err := c.http.Request(ctx, http.MethodPost, "", requestBody{
		ID:        n.ID.String(),
		Channel:   n.Channel,
		Recipient: n.Recipient,
		Content:   n.Content,
	})
	latency := time.Since(start).Milliseconds()

	if err != nil {
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
