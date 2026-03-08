package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "events/docs"
	"events/internal/handler"
	"events/internal/middleware"
)

func NewRouter(
	eventHandler *handler.EventHandler,
	metricsHandler *handler.MetricsHandler,
	healthHandler *handler.HealthHandler,
	logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.Logging(logger))
	r.Use(middleware.MaxBody(10 << 20)) // 10 MB

	r.Get("/health/live", healthHandler.Liveness)
	r.Get("/health/ready", healthHandler.Readiness)

	r.Post("/events", eventHandler.PostEvent)
	r.Post("/events/bulk", eventHandler.PostBulkEvents)

	r.Get("/metrics", metricsHandler.GetMetrics)

	r.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusFound)
	})
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return r
}
