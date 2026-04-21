package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/handler"
	"github.com/your-org/mock-gateway/internal/store"
)

func New(cfg *config.Config, st *store.Store, logs *store.LogRing) http.Handler {
	r := mux.NewRouter()

	// CORS middleware
	r.Use(corsMiddleware(cfg))

	// ── Admin API ──────────────────────────────────────────────────────────
	admin := handler.NewAdminHandler(cfg, st, logs)

	r.HandleFunc("/admin/me",           admin.GetMe).Methods(http.MethodGet)
	r.HandleFunc("/admin/services",     admin.ListServices).Methods(http.MethodGet)
	r.HandleFunc("/admin/routes",       admin.ListRoutes).Methods(http.MethodGet)
	r.HandleFunc("/admin/routes",       admin.UpdateRoute).Methods(http.MethodPut)
	r.HandleFunc("/admin/routes/reset", admin.ResetRoutes).Methods(http.MethodPost)
	r.HandleFunc("/admin/specs",        admin.UploadSpec).Methods(http.MethodPost)
	r.HandleFunc("/admin/logs",         admin.GetLogs).Methods(http.MethodGet)
	r.HandleFunc("/admin/logs",         admin.ClearLogs).Methods(http.MethodDelete)

	// ── Static developer UI ────────────────────────────────────────────────
	r.PathPrefix("/mock-ui/").Handler(
		http.StripPrefix("/mock-ui/", http.FileServer(http.Dir("ui/"))),
	)

	// ── Health ─────────────────────────────────────────────────────────────
	r.HandleFunc("/health", handler.HealthHandler).Methods(http.MethodGet)

	// ── All other requests → mock handler ─────────────────────────────────
	// Must be registered last so admin routes take priority.
	mock := handler.NewMockHandler(cfg, st, logs)
	r.PathPrefix("/").Handler(mock)

	return r
}

func corsMiddleware(cfg *config.Config) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers",
				"Content-Type, Authorization, "+
					"X-Mock-Service, X-Mock-Enabled, X-Mock-Env, X-Mock-Token, "+
					"Source-Api-Key, X-Request-Id")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
