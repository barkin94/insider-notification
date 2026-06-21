package messaging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	watermill "github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"

	apipub "github.com/barkin94/insider-notification/api/public"
	db "github.com/barkin94/insider-notification/deliveryscheduler/internal/db"
	"github.com/barkin94/insider-notification/deliveryscheduler/internal/transport/messaging"
	stream "github.com/barkin94/insider-notification/shared/messaging"
)

type cancelMockRepo struct {
	deletedIDs []string
	deleteErr  error

	// satisfy full interface
	upsertErr error
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

func makeCancelResult(notifID string) (stream.Result[apipub.NotificationScheduleCancelledEvent], *watermill.Message) {
	msg := watermill.NewMessage(uuid.New().String(), nil)
	evt := apipub.NotificationScheduleCancelledEvent{NotificationID: notifID}
	return stream.Result[apipub.NotificationScheduleCancelledEvent]{
		Ctx:   context.Background(),
		Event: evt,
		Msg:   msg,
	}, msg
}

func runCancelConsumer(repo *cancelMockRepo, results ...stream.Result[apipub.NotificationScheduleCancelledEvent]) {
	ch := make(chan stream.Result[apipub.NotificationScheduleCancelledEvent], len(results))
	for _, r := range results {
		ch <- r
	}
	close(ch)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messaging.NewCancelConsumer(repo, ch).Run(ctx)
}

func waitAck(t *testing.T, msg *watermill.Message) {
	t.Helper()
	select {
	case <-msg.Acked():
	case <-msg.Nacked():
		t.Error("expected ack, got nack")
	case <-time.After(time.Second):
		t.Error("timeout waiting for ack")
	}
}

func waitNack(t *testing.T, msg *watermill.Message) {
	t.Helper()
	select {
	case <-msg.Nacked():
	case <-msg.Acked():
		t.Error("expected nack, got ack")
	case <-time.After(time.Second):
		t.Error("timeout waiting for nack")
	}
}

func TestCancelConsumer_acksAndDeletesOnSuccess(t *testing.T) {
	repo := &cancelMockRepo{}
	notifID := uuid.New().String()
	result, msg := makeCancelResult(notifID)

	runCancelConsumer(repo, result)

	waitAck(t, msg)

	if len(repo.deletedIDs) != 1 || repo.deletedIDs[0] != notifID {
		t.Errorf("deleted IDs = %v, want [%s]", repo.deletedIDs, notifID)
	}
}

func TestCancelConsumer_nacksOnDeleteError(t *testing.T) {
	repo := &cancelMockRepo{deleteErr: errors.New("db unavailable")}
	result, msg := makeCancelResult(uuid.New().String())

	runCancelConsumer(repo, result)

	waitNack(t, msg)
}

func TestCancelConsumer_processesMultipleMessages(t *testing.T) {
	repo := &cancelMockRepo{}
	ids := []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}

	results := make([]stream.Result[apipub.NotificationScheduleCancelledEvent], len(ids))
	msgs := make([]*watermill.Message, len(ids))
	for i, id := range ids {
		results[i], msgs[i] = makeCancelResult(id)
	}

	runCancelConsumer(repo, results...)

	for _, msg := range msgs {
		waitAck(t, msg)
	}

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
	ch := make(chan stream.Result[apipub.NotificationScheduleCancelledEvent])

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
