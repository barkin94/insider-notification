package messaging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	natsio "github.com/nats-io/nats.go"

	apipub "github.com/barkin94/insider-notification/api/public"
	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	"github.com/barkin94/insider-notification/deliveryscheduler/internal/transport/messaging"
	natsmsg "github.com/barkin94/insider-notification/shared/messaging/nats"
)

type cancelMockRepo struct {
	deletedIDs []string
	deleteErr  error
	upsertErr  error
}

func (m *cancelMockRepo) UpsertAll(_ context.Context, _ []*db.ScheduledNotification) error {
	return m.upsertErr
}

func (m *cancelMockRepo) DeleteByScheduledAtBeforeReturning(_ context.Context, _ time.Time, _ int) ([]*db.ScheduledNotification, error) {
	return nil, nil
}

func (m *cancelMockRepo) DeleteByNotificationID(_ context.Context, id string) error {
	m.deletedIDs = append(m.deletedIDs, id)
	return m.deleteErr
}

func makeCancelResult(notifID string) natsmsg.Result[apipub.NotificationScheduleCancelledEvent] {
	return natsmsg.Result[apipub.NotificationScheduleCancelledEvent]{
		Ctx:           context.Background(),
		Event:         apipub.NotificationScheduleCancelledEvent{NotificationID: notifID},
		Msg:           &natsio.Msg{},
		AttemptNumber: 1,
	}
}

func runCancelConsumer(repo *cancelMockRepo, results ...natsmsg.Result[apipub.NotificationScheduleCancelledEvent]) {
	ch := make(chan natsmsg.Result[apipub.NotificationScheduleCancelledEvent], len(results))
	for _, r := range results {
		ch <- r
	}
	close(ch)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messaging.NewCancelConsumer(repo, ch).Run(ctx)
}

func TestCancelConsumer_deletesOnSuccess(t *testing.T) {
	repo := &cancelMockRepo{}
	notifID := uuid.New().String()

	runCancelConsumer(repo, makeCancelResult(notifID))

	if len(repo.deletedIDs) != 1 || repo.deletedIDs[0] != notifID {
		t.Errorf("deleted IDs = %v, want [%s]", repo.deletedIDs, notifID)
	}
}

func TestCancelConsumer_skipsDeleteOnError(t *testing.T) {
	repo := &cancelMockRepo{deleteErr: errors.New("db unavailable")}

	runCancelConsumer(repo, makeCancelResult(uuid.New().String()))

	// repo.DeleteByNotificationID was called but returned an error; consumer nacked and moved on
	if len(repo.deletedIDs) != 1 {
		t.Errorf("expected DeleteByNotificationID to be called once, got %d", len(repo.deletedIDs))
	}
}

func TestCancelConsumer_processesMultipleMessages(t *testing.T) {
	repo := &cancelMockRepo{}
	ids := []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}

	results := make([]natsmsg.Result[apipub.NotificationScheduleCancelledEvent], len(ids))
	for i, id := range ids {
		results[i] = makeCancelResult(id)
	}
	runCancelConsumer(repo, results...)

	if len(repo.deletedIDs) != len(ids) {
		t.Fatalf("deleted %d IDs, want %d", len(repo.deletedIDs), len(ids))
	}
	for i, id := range ids {
		if repo.deletedIDs[i] != id {
			t.Errorf("deletedIDs[%d] = %s, want %s", i, repo.deletedIDs[i], id)
		}
	}
}

func TestCancelConsumer_stopsOnContextCancel(t *testing.T) {
	repo := &cancelMockRepo{}
	ch := make(chan natsmsg.Result[apipub.NotificationScheduleCancelledEvent])

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		messaging.NewCancelConsumer(repo, ch).Run(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("consumer did not stop after context cancellation")
	}
}
