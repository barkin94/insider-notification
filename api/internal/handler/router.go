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

// appRouter wraps chi.Router and automatically applies middleware.ErrorHandler
// on every AppHandler route, so call sites never need to wrap manually.
type appRouter struct{ chi.Router }

func (r *appRouter) Get(path string, h middleware.AppHandler) {
	r.Router.Get(path, middleware.ErrorHandler(h))
}

func (r *appRouter) Post(path string, h middleware.AppHandler) {
	r.Router.Post(path, middleware.ErrorHandler(h))
}

func (r *appRouter) Route(path string, fn func(*appRouter)) {
	r.Router.Route(path, func(sub chi.Router) {
		fn(&appRouter{sub})
	})
}

// NewRouter builds and returns the chi router with all routes registered.
func NewRouter(deps Deps) http.Handler {
	mux := chi.NewRouter()
	mux.Use(chiMiddleware.Recoverer)
	mux.Use(middleware.Logger())

	mux.Get("/api/v1/docs/*", httpSwagger.WrapHandler)
	mux.Get("/api/v1/health", healthCheck(deps.DB, deps.Redis))

	r := &appRouter{mux}
	r.Route("/api/v1/notifications", func(r *appRouter) {
		r.Post("/", createNotification(deps.Service))
		r.Get("/", listNotifications(deps.Service))
		r.Post("/batch", createBatch(deps.Service))
		r.Get("/{id}", getNotification(deps.Service))
		r.Post("/{id}/cancel", cancelNotification(deps.Service))
	})

	return otelhttp.NewHandler(mux, "api")
}
