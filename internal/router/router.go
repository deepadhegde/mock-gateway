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

	r.Use(corsMiddleware(cfg))

	admin := handler.NewAdminHandler(cfg, st, logs)

	// ── Routes ────────────────────────────────────────────────────────────────
	r.HandleFunc("/admin/me",             admin.GetMe).Methods(http.MethodGet)
	r.HandleFunc("/admin/services",       admin.ListServices).Methods(http.MethodGet)
	r.HandleFunc("/admin/routes",         admin.ListRoutes).Methods(http.MethodGet)
	r.HandleFunc("/admin/routes",         admin.UpdateRoute).Methods(http.MethodPut)
	r.HandleFunc("/admin/routes/reset",   admin.ResetRoutes).Methods(http.MethodPost)
	r.HandleFunc("/admin/specs",          admin.GetSpec).Methods(http.MethodGet)
	r.HandleFunc("/admin/specs",          admin.UploadSpec).Methods(http.MethodPost)
	r.HandleFunc("/admin/logs",           admin.GetLogs).Methods(http.MethodGet)
	r.HandleFunc("/admin/logs",           admin.ClearLogs).Methods(http.MethodDelete)
	r.HandleFunc("/admin/users",          admin.ListUsers).Methods(http.MethodGet)
	r.HandleFunc("/admin/users",          admin.CreateUser).Methods(http.MethodPost)
	r.HandleFunc("/admin/users",          admin.UpdateUser).Methods(http.MethodPatch)
	r.HandleFunc("/admin/users",          admin.DeleteUser).Methods(http.MethodDelete)

	// ── Scenarios ─────────────────────────────────────────────────────────────
	r.HandleFunc("/admin/scenarios/active", admin.SetActiveScenario).Methods(http.MethodPut)
	r.HandleFunc("/admin/scenarios",        admin.ListScenarios).Methods(http.MethodGet)
	r.HandleFunc("/admin/scenarios",        admin.UpsertScenario).Methods(http.MethodPost)
	r.HandleFunc("/admin/scenarios",        admin.DeleteScenario).Methods(http.MethodDelete)

	// ── State ─────────────────────────────────────────────────────────────────
	r.HandleFunc("/admin/state", admin.GetState).Methods(http.MethodGet)
	r.HandleFunc("/admin/state", admin.SetState).Methods(http.MethodPut)
	r.HandleFunc("/admin/state", admin.ClearState).Methods(http.MethodDelete)

	// ── Static developer UI ───────────────────────────────────────────────────
	r.PathPrefix("/mock-ui/").Handler(
		http.StripPrefix("/mock-ui/", http.FileServer(http.Dir("ui/"))),
	)

	// ── Health ────────────────────────────────────────────────────────────────
	r.HandleFunc("/health", handler.HealthHandler).Methods(http.MethodGet)

	// ── All other requests → mock handler (registered last) ───────────────────
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
					"Source-Api-Key, X-Request-Id, X-Platform")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
