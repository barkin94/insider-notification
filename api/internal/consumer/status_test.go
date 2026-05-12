package consumer_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"
	"time"

	watermill "github.com/ThreeDotsLabs/watermill/message"
	"github.com/barkin/insider-notification/api/internal/consumer"
	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

var testDB *bun.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(120*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Printf("terminate postgres container: %v", err)
		}
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	mig, err := migrate.New("file://../../../api/migrations", "pgx5://"+connStr[len("postgres://"):])
	if err != nil {
		log.Fatalf("create migrator: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("run migrations: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	testDB = bun.NewDB(sqldb, pgdialect.New())
	defer testDB.Close()

	os.Exit(m.Run())
}

func mustV7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}

func seedNotification(t *testing.T, id uuid.UUID, status string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	_, err := testDB.NewRaw(`
		INSERT INTO notifications
			(id, recipient, channel, content, priority, status, attempts, max_attempts, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "+1", "sms", "hello", "normal", status, 0, 4, `{}`, now, now,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
}

func makeResult(evt stream.NotificationDeliveryResultEvent) stream.Result[stream.NotificationDeliveryResultEvent] {
	msg := watermill.NewMessage(uuid.New().String(), nil)
	return stream.Result[stream.NotificationDeliveryResultEvent]{Ctx: context.Background(), Event: evt, Msg: msg}
}

func runConsumer(c *consumer.StatusConsumer, result stream.Result[stream.NotificationDeliveryResultEvent]) {
	ch := make(chan stream.Result[stream.NotificationDeliveryResultEvent], 1)
	ch <- result
	close(ch)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c.Run(ctx, ch)
}

func TestStatusConsumer_delivered(t *testing.T) {
	notifID := mustV7()
	seedNotification(t, notifID, model.StatusPending)

	notifRepo := db.NewNotificationRepository(testDB)
	attemptRepo := db.NewDeliveryAttemptRepository(testDB)
	c := consumer.NewStatusConsumer(notifRepo, attemptRepo)

	latency := 120
	code := 200
	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: notifID.String(),
		Status:         model.StatusDelivered,
		AttemptNumber:  1,
		HTTPStatusCode: code,
		LatencyMS:      latency,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	runConsumer(c, makeResult(evt))

	got, err := notifRepo.GetByID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.StatusDelivered {
		t.Errorf("status = %q, want %q", got.Status, model.StatusDelivered)
	}

	attempts, err := attemptRepo.ListByNotificationID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("ListAttempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts count = %d, want 1", len(attempts))
	}
	if attempts[0].AttemptNumber != 1 {
		t.Errorf("attempt_number = %d, want 1", attempts[0].AttemptNumber)
	}
	if attempts[0].Status != model.StatusDelivered {
		t.Errorf("attempt status = %q, want %q", attempts[0].Status, model.StatusDelivered)
	}
}

func TestStatusConsumer_failed(t *testing.T) {
	notifID := mustV7()
	seedNotification(t, notifID, model.StatusPending)

	notifRepo := db.NewNotificationRepository(testDB)
	attemptRepo := db.NewDeliveryAttemptRepository(testDB)
	c := consumer.NewStatusConsumer(notifRepo, attemptRepo)

	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: notifID.String(),
		Status:         model.StatusFailed,
		AttemptNumber:  4,
		ErrorMessage:   "provider timeout",
		LatencyMS:      500,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	runConsumer(c, makeResult(evt))

	got, err := notifRepo.GetByID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != model.StatusFailed {
		t.Errorf("status = %q, want %q", got.Status, model.StatusFailed)
	}
}

func TestStatusConsumer_idempotent(t *testing.T) {
	notifID := mustV7()
	seedNotification(t, notifID, model.StatusPending)

	notifRepo := db.NewNotificationRepository(testDB)
	attemptRepo := db.NewDeliveryAttemptRepository(testDB)
	c := consumer.NewStatusConsumer(notifRepo, attemptRepo)

	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: notifID.String(),
		Status:         model.StatusDelivered,
		AttemptNumber:  1,
		HTTPStatusCode: 200,
		LatencyMS:      80,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	runConsumer(c, makeResult(evt))
	runConsumer(c, makeResult(evt))

	attempts, err := attemptRepo.ListByNotificationID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("ListAttempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Errorf("attempts count = %d, want 1 (idempotent)", len(attempts))
	}
}
