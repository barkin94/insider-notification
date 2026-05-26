package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/barkin/insider-notification/shared/httpclient"
)

// DeliveryResult holds the outcome of a single delivery attempt.
type DeliveryResult struct {
	Success       bool
	Retryable     bool
	StatusCode    int
	LatencyMS     int64
	ProviderMsgID string
	ErrorMessage  string
}

// DeliveryClient delivers a notification to the webhook provider.
type DeliveryClient interface {
	Send(ctx context.Context, to, channel, content string) (DeliveryResult, error)
}

type webhookClient struct {
	http *httpclient.Client
}

// NewDeliveryClient returns a DeliveryClient that POSTs to webhookURL with the given timeout.
func NewDeliveryClient(webhookURL string, timeout time.Duration) DeliveryClient {
	return &webhookClient{
		http: httpclient.New(webhookURL, httpclient.WithTimeout(timeout)),
	}
}

type deliveryRequestBody struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

func (c *webhookClient) Send(ctx context.Context, to, channel, content string) (DeliveryResult, error) {
	start := time.Now()
	resp, err := c.http.Request(ctx, http.MethodPost, "", deliveryRequestBody{
		To:      to,
		Channel: channel,
		Content: content,
	})
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return DeliveryResult{Retryable: true, LatencyMS: latency, ErrorMessage: err.Error()}, nil
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	result := DeliveryResult{StatusCode: code, LatencyMS: latency}

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
		"to", to,
		"channel", channel,
		"status", code,
		"latency_ms", latency,
		"success", result.Success,
		"retryable", result.Retryable,
	)
	return result, nil
}
