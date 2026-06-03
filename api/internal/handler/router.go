package handler

import (
	"context"
	"net/http"
	"time"

	_ "github.com/barkin/insider-notification/api/docs"
	"github.com/barkin/insider-notification/api/internal/db"
	"github.com/barkin/insider-notification/api/internal/service"
	sharedhandler "github.com/barkin/insider-notification/shared/handler"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Deps holds the dependencies required to build the HTTP router.
type Deps struct {
	Service service.NotificationService
	DB      *bun.DB
	Redis   *redis.Client
}

// NewRouter builds and returns the chi router with all routes registered.
func NewRouter(deps Deps) http.Handler {
	checkers := []sharedhandler.ReadinessChecker{
		{
			Name: "postgresql",
			Check: func(ctx context.Context) error {
				ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				return deps.DB.PingContext(ctx)
			},
		},
		{
			Name: "redis",
			Check: func(ctx context.Context) error {
				ctx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				return deps.Redis.Ping(ctx).Err()
			},
		},
	}

	errorMap := sharedhandler.ErrorMap{
		db.ErrNotFound:        {Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "resource not found"},
		db.ErrTransitionFailed: {Status: http.StatusConflict, Code: "INVALID_STATUS_TRANSITION"},
	}

	r := sharedhandler.NewRouter(checkers, errorMap)

	r.Route("/api/v1/notifications", func(r *sharedhandler.AppRouter) {
		r.Post("/", createNotification(deps.Service))
		r.Get("/", listNotifications(deps.Service))
		r.Post("/batch", createBatch(deps.Service))
		r.Get("/{id}", getNotification(deps.Service))
		r.Post("/{id}/cancel", cancelNotification(deps.Service))
	})

	return otelhttp.NewHandler(r, "api")
}
