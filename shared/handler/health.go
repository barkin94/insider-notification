package handler

import (
	"context"
	"fmt"
	"net/http"
)

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// ReadinessChecker is a named health probe used by the /readiness endpoint.
type ReadinessChecker struct {
	Name  string
	Check func(ctx context.Context) error
}

// livenessCheck godoc
// @Summary     Liveness check
// @Tags        system
// @Produce     json
// @Success     200 {object} healthResponse
// @Router      /liveness [get]
func livenessCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}

// readinessCheck godoc
// @Summary     Readiness check
// @Tags        system
// @Produce     json
// @Success     200 {object} healthResponse
// @Failure     503 {object} healthResponse
// @Router      /readiness [get]
func readinessCheck(checkers []ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(checkers) == 0 {
			WriteJSON(w, http.StatusOK, healthResponse{Status: "ok"})
			return
		}

		checks := make(map[string]string, len(checkers))
		degraded := false

		for _, c := range checkers {
			if err := c.Check(r.Context()); err != nil {
				checks[c.Name] = fmt.Sprintf("error: %s", err)
				degraded = true
			} else {
				checks[c.Name] = "ok"
			}
		}

		if degraded {
			WriteJSON(w, http.StatusServiceUnavailable, healthResponse{
				Status: "degraded",
				Checks: checks,
			})
			return
		}
		WriteJSON(w, http.StatusOK, healthResponse{
			Status: "ok",
			Checks: checks,
		})
	}
}
