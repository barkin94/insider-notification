package handler

import (
	_ "github.com/barkin/insider-notification/api/docs"
	"github.com/barkin/insider-notification/shared/middleware"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
)

// AppRouter wraps chi.Router and automatically applies errHandler (with the
// configured ErrorMap) on every AppHandler route.
type AppRouter struct {
	chi.Router
	em ErrorMap
}

func (r *AppRouter) Get(path string, h AppHandler) {
	r.Router.Get(path, errHandler(h, r.em))
}

func (r *AppRouter) Post(path string, h AppHandler) {
	r.Router.Post(path, errHandler(h, r.em))
}

func (r *AppRouter) Route(path string, fn func(*AppRouter)) {
	r.Router.Route(path, func(sub chi.Router) {
		fn(&AppRouter{sub, r.em})
	})
}

// NewRouter builds the base chi router with standard middleware, Swagger UI,
// liveness (/api/v1/liveness) and readiness (/api/v1/readiness) endpoints.
//
// readinessChecks: named probes run on /readiness (nil → always 200).
// errorMap: sentinel errors mapped to HTTP responses (nil → disabled).
func NewRouter(readinessChecks []ReadinessChecker, errorMap ErrorMap) *AppRouter {
	mux := chi.NewRouter()
	mux.Use(chiMiddleware.Recoverer)
	mux.Use(middleware.Logger())

	mux.Get("/api/v1/docs/*", httpSwagger.WrapHandler)
	mux.Get("/api/v1/liveness", livenessCheck())
	mux.Get("/api/v1/readiness", readinessCheck(readinessChecks))

	return &AppRouter{mux, errorMap}
}
