package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type healthResponse struct {
	Status  string            `json:"status"`
	Checks  map[string]string `json:"checks"`
	Version string            `json:"version,omitempty"`
}

func healthCheck(db *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := make(map[string]string, 2)
		degraded := false

		pgCtx, pgCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer pgCancel()
		if err := db.QueryRow(pgCtx, "SELECT 1").Scan(new(int)); err != nil {
			checks["postgresql"] = fmt.Sprintf("error: %s", err)
			degraded = true
		} else {
			checks["postgresql"] = "ok"
		}

		redisCtx, redisCancel := context.WithTimeout(r.Context(), time.Second)
		defer redisCancel()
		if err := rdb.Ping(redisCtx).Err(); err != nil {
			checks["redis"] = fmt.Sprintf("error: %s", err)
			degraded = true
		} else {
			checks["redis"] = "ok"
		}

		if degraded {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{
				Status: "degraded",
				Checks: checks,
			})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Checks:  checks,
			Version: "1.0.0",
		})
	}
}
