package handler_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/barkin/insider-notification/shared/handler"
)

func TestRequestLogger_fields(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	opts := handler.HandlerOpts{
		RegisterRoutesFunc: nil,
	}
	h := handler.NewHandler(opts)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/liveness", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

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
