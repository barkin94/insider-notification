package service

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
type NtfnDeliveryClient interface {
	Send(ctx context.Context, to, channel, content string) (DeliveryResult, error)
}

type ntfnDeliveryClient struct {
	http *httpclient.Client
}

// NewDeliveryClient returns a DeliveryClient that POSTs to notificationProviderURL with the given timeout.
func NewNtfnDeliveryClient(notificationProviderURL string, timeout time.Duration) NtfnDeliveryClient {
	return &ntfnDeliveryClient{
		http: httpclient.New(notificationProviderURL, httpclient.WithTimeout(timeout)),
	}
}

type ntfnDeliveryRequestBody struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

func (c *ntfnDeliveryClient) Send(ctx context.Context, to, channel, content string) (DeliveryResult, error) {
	start := time.Now()
	resp, err := c.http.Request(ctx, http.MethodPost, "", ntfnDeliveryRequestBody{
		To:      to,
		Channel: channel,
		Content: content,
	})
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return DeliveryResult{Retryable: true, LatencyMS: latency, ErrorMessage: err.Error()}, nil
	}
	defer resp.Body.Close() //nolint:errcheck

	code := resp.StatusCode
	result := DeliveryResult{StatusCode: code, LatencyMS: latency}

	switch code {
	case http.StatusAccepted:
		result.Success = true
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
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
