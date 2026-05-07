package handler_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/barkin/insider-notification/api/internal/handler"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealth_200(t *testing.T) {
	// health_test uses a real DB+Redis — tested via integration in main;
	// here we verify the route is registered and responds to a valid connection.
	// Since we don't have testcontainers here, we verify the route exists via a nil-safe path.
	// Full integration covered by api-main task.
	t.Skip("health integration tested in api-main; skipped here to avoid testcontainers in handler unit tests")
}

func TestHealth_503(t *testing.T) {
	t.Skip("health integration tested in api-main; skipped here to avoid testcontainers in handler unit tests")
}

func TestHealth_routeRegistered(t *testing.T) {
	// Verify GET /api/v1/health is registered (will panic/500 with nil deps, not 404).
	r := handler.NewRouter(handler.Deps{
		Service: nil,
		Logger:  discardLogger(),
		DB:      nil,
		Redis:   nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	// We expect a non-404 (handler is registered; it will panic/recover to 500 with nil pool).
	func() {
		defer func() { recover() }() //nolint:errcheck
		r.ServeHTTP(w, req)
	}()

	if w.Code == http.StatusNotFound {
		t.Error("GET /api/v1/health returned 404 — route not registered")
	}
}
