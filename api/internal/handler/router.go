package handler

import (
	"net/http"

	_ "github.com/barkin/insider-notification/api/docs"
	"github.com/barkin/insider-notification/api/internal/middleware"
	"github.com/barkin/insider-notification/api/internal/service"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	httpSwagger "github.com/swaggo/http-swagger"
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
	r := chi.NewRouter()

	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.Logger())

	r.Get("/api/v1/docs/*", httpSwagger.WrapHandler)
	r.Get("/api/v1/health", healthCheck(deps.DB, deps.Redis))

	r.Route("/api/v1/notifications", func(r chi.Router) {
		r.Post("/", createNotification(deps.Service))
		r.Get("/", listNotifications(deps.Service))
		r.Post("/batch", createBatch(deps.Service))
		r.Get("/{id}", getNotification(deps.Service))
		r.Post("/{id}/cancel", cancelNotification(deps.Service))
	})

	return otelhttp.NewHandler(r, "api")
}
