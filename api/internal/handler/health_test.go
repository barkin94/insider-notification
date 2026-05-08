package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/barkin/insider-notification/api/internal/handler"
)

func TestHealth_200(t *testing.T) {
	t.Skip("health integration tested in api-main; skipped here to avoid testcontainers in handler unit tests")
}

func TestHealth_503(t *testing.T) {
	t.Skip("health integration tested in api-main; skipped here to avoid testcontainers in handler unit tests")
}

func TestHealth_routeRegistered(t *testing.T) {
	r := handler.NewRouter(handler.Deps{
		Service: nil,
		DB:      nil,
		Redis:   nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	func() {
		defer func() { recover() }() //nolint:errcheck
		r.ServeHTTP(w, req)
	}()

	if w.Code == http.StatusNotFound {
		t.Error("GET /api/v1/health returned 404 — route not registered")
	}
}
