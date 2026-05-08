package model_test

import (
	"testing"

	"github.com/barkin/insider-notification/shared/model"
)

func TestChannelValues(t *testing.T) {
	cases := map[string]string{
		"SMS":   model.ChannelSMS,
		"Email": model.ChannelEmail,
		"Push":  model.ChannelPush,
	}
	want := map[string]string{
		"SMS":   "sms",
		"Email": "email",
		"Push":  "push",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("Channel%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestPriorityValues(t *testing.T) {
	cases := map[string]string{
		"High":   model.PriorityHigh,
		"Normal": model.PriorityNormal,
		"Low":    model.PriorityLow,
	}
	want := map[string]string{
		"High":   "high",
		"Normal": "normal",
		"Low":    "low",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("Priority%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestStatusValues(t *testing.T) {
	cases := map[string]string{
		"Pending":    model.StatusPending,
		"Processing": model.StatusProcessing,
		"Delivered":  model.StatusDelivered,
		"Failed":     model.StatusFailed,
		"Cancelled":  model.StatusCancelled,
	}
	want := map[string]string{
		"Pending":    "pending",
		"Processing": "processing",
		"Delivered":  "delivered",
		"Failed":     "failed",
		"Cancelled":  "cancelled",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("Status%s = %q, want %q", name, got, want[name])
		}
	}
}
