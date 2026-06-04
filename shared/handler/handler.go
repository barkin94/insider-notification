package handler

import (
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/barkin/insider-notification/shared/middleware"
)

// AppRouter wraps chi.Router and automatically applies errHandler on every AppHandler route.
type AppRouter struct {
	chi.Router
}

func (r *AppRouter) Get(path string, h AppHandler) {
	r.Router.Get(path, errHandler(h))
}

func (r *AppRouter) Post(path string, h AppHandler) {
	r.Router.Post(path, errHandler(h))
}

func (r *AppRouter) Route(path string, fn func(*AppRouter)) {
	r.Router.Route(path, func(sub chi.Router) {
		fn(&AppRouter{sub})
	})
}

// NewRouter builds the base chi router with standard middleware, Swagger UI,
// liveness (/api/v1/liveness) and readiness (/api/v1/readiness) endpoints.
func NewRouter(readinessChecks []ReadinessChecker) *AppRouter {
	mux := chi.NewRouter()
	mux.Use(chiMiddleware.Recoverer)
	mux.Use(middleware.Logger())

	mux.Get("/api/v1/docs/*", httpSwagger.WrapHandler)
	mux.Get("/api/v1/liveness", livenessCheck())
	mux.Get("/api/v1/readiness", readinessCheck(readinessChecks))

	return &AppRouter{mux}
}
