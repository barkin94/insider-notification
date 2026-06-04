package model_test

import (
	"testing"

	"github.com/barkin/insider-notification/shared/model"
)

func TestChannelValues(t *testing.T) {
	cases := []struct {
		name string
		got  model.Channel
		want string
	}{
		{"SMS", model.ChannelSMS, "sms"},
		{"Email", model.ChannelEmail, "email"},
		{"Push", model.ChannelPush, "push"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Channel%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestPriorityValues(t *testing.T) {
	cases := []struct {
		name string
		got  model.Priority
		want string
	}{
		{"High", model.PriorityHigh, "high"},
		{"Normal", model.PriorityNormal, "normal"},
		{"Low", model.PriorityLow, "low"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Priority%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestStatusValues(t *testing.T) {
	cases := []struct {
		name string
		got  model.Status
		want string
	}{
		{"Pending", model.StatusPending, "pending"},
		{"Delivered", model.StatusDelivered, "delivered"},
		{"Failed", model.StatusFailed, "failed"},
		{"Cancelled", model.StatusCancelled, "cancelled"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Status%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
