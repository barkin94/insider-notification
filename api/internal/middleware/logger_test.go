package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/barkin/insider-notification/api/internal/middleware"
)

func TestLogger_fields(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	handler := middleware.Logger()(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("parse log output: %v", err)
	}

	for _, field := range []string{"time", "level", "msg", "method", "path", "status", "latency_ms"} {
		if _, ok := entry[field]; !ok {
			t.Errorf("missing log field: %s", field)
		}
	}
}
