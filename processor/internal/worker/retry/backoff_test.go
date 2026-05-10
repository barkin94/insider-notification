package retry_test

import (
	"testing"
	"time"

	"github.com/barkin/insider-notification/processor/internal/worker/retry"
)

func TestDelay_attempt2(t *testing.T) {
	d := retry.Delay(2)
	if d < 60*time.Second || d > 72*time.Second {
		t.Errorf("attempt 2: got %v, want [60s, 72s]", d)
	}
}

func TestDelay_attempt3(t *testing.T) {
	d := retry.Delay(3)
	if d < 120*time.Second || d > 144*time.Second {
		t.Errorf("attempt 3: got %v, want [120s, 144s]", d)
	}
}

func TestDelay_attempt4(t *testing.T) {
	d := retry.Delay(4)
	if d < 240*time.Second || d > 288*time.Second {
		t.Errorf("attempt 4: got %v, want [240s, 288s]", d)
	}
}

func TestDelay_cap(t *testing.T) {
	d := retry.Delay(10)
	if d < 480*time.Second || d > 576*time.Second {
		t.Errorf("attempt 10 (capped): got %v, want [480s, 576s]", d)
	}
}

func TestDelay_jitter_nonNegative(t *testing.T) {
	for i := 0; i < 1000; i++ {
		d := retry.Delay(2)
		if d < 60*time.Second {
			t.Fatalf("jitter produced negative offset: got %v < 60s", d)
		}
	}
}

func TestDelay_jitter_bounded(t *testing.T) {
	for i := 0; i < 1000; i++ {
		d := retry.Delay(2)
		if d > 72*time.Second {
			t.Fatalf("jitter exceeded 20%% of base: got %v > 72s", d)
		}
	}
}
