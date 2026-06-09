package messaging_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"
	"time"

	watermill "github.com/ThreeDotsLabs/watermill/message"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	repopostgres "github.com/barkin/insider-notification/api/internal/repository/postgres"
	"github.com/barkin/insider-notification/api/internal/service"
	"github.com/barkin/insider-notification/api/internal/transport/messaging"
	"github.com/barkin/insider-notification/shared/model"
	"github.com/barkin/insider-notification/shared/stream"
)

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }

var testDB *bun.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
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

	mig, err := migrate.New("file://../../../../api/migrations", "pgx5://"+connStr[len("postgres://"):])
	if err != nil {
		log.Fatalf("create migrator: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("run migrations: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	testDB = bun.NewDB(sqldb, pgdialect.New())
	defer testDB.Close() //nolint:errcheck

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
			(id, recipient, channel, content, priority, status, max_attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "+1", "sms", "hello", "normal", status, 4, now, now,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
}

func makeResult(evt stream.NotificationDeliveryResultEvent) stream.Result[stream.NotificationDeliveryResultEvent] {
	msg := watermill.NewMessage(uuid.New().String(), nil)
	return stream.Result[stream.NotificationDeliveryResultEvent]{Ctx: context.Background(), Event: evt, Msg: msg}
}

func makeSvc() service.NotificationService {
	return service.NewNotificationService(repopostgres.NewNotificationRepository(testDB), noopPublisher{})
}

func runConsumer(svc service.NotificationService, result stream.Result[stream.NotificationDeliveryResultEvent]) {
	ch := make(chan stream.Result[stream.NotificationDeliveryResultEvent], 1)
	ch <- result
	close(ch)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messaging.NewDeliveryResultConsumer(svc, ch).Run(ctx)
}

func TestDeliveryResultConsumer_delivered(t *testing.T) {
	notifID := mustV7()
	seedNotification(t, notifID, string(model.StatusPending))

	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: notifID.String(),
		Status:         string(model.StatusDelivered),
		AttemptNumber:  1,
		HTTPStatusCode: 200,
		LatencyMS:      120,
	}
	runConsumer(makeSvc(), makeResult(evt))

	got, err := repopostgres.NewNotificationRepository(testDB).GetByID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != string(model.StatusDelivered) {
		t.Errorf("status = %q, want %q", got.Status, string(model.StatusDelivered))
	}
}

func TestDeliveryResultConsumer_failed(t *testing.T) {
	notifID := mustV7()
	seedNotification(t, notifID, string(model.StatusPending))

	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: notifID.String(),
		Status:         string(model.StatusFailed),
		AttemptNumber:  4,
		ErrorMessage:   "provider timeout",
		LatencyMS:      500,
	}
	runConsumer(makeSvc(), makeResult(evt))

	got, err := repopostgres.NewNotificationRepository(testDB).GetByID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != string(model.StatusFailed) {
		t.Errorf("status = %q, want %q", got.Status, string(model.StatusFailed))
	}
}

func TestDeliveryResultConsumer_idempotent(t *testing.T) {
	notifID := mustV7()
	seedNotification(t, notifID, string(model.StatusPending))

	evt := stream.NotificationDeliveryResultEvent{
		NotificationID: notifID.String(),
		Status:         string(model.StatusDelivered),
		AttemptNumber:  1,
		HTTPStatusCode: 200,
		LatencyMS:      80,
	}

	svc := makeSvc()
	runConsumer(svc, makeResult(evt))
	runConsumer(svc, makeResult(evt)) // second run: domain rejects delivered→delivered, nacks

	got, err := repopostgres.NewNotificationRepository(testDB).GetByID(context.Background(), notifID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != string(model.StatusDelivered) {
		t.Errorf("status = %q, want %q", got.Status, string(model.StatusDelivered))
	}
}
