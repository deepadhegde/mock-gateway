package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/seed"
	"github.com/your-org/mock-gateway/internal/store"
)

// AdminHandler handles all /admin/* endpoints.
type AdminHandler struct {
	cfg  *config.Config
	st   *store.Store
	logs *store.LogRing
}

func NewAdminHandler(cfg *config.Config, st *store.Store, logs *store.LogRing) *AdminHandler {
	return &AdminHandler{cfg: cfg, st: st, logs: logs}
}

// ── Role-based access ────────────────────────────────────────────────────────

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleTester Role = "tester"
	RoleViewer Role = "viewer"
	RoleNone   Role = ""
)

func (h *AdminHandler) resolveRole(r *http.Request) Role {
	token := r.Header.Get("X-Mock-Token")

	adminTok  := h.cfg.Roles.AdminToken
	testerTok := h.cfg.Roles.TesterToken
	viewerTok := h.cfg.Roles.ViewerToken

	// No tokens configured = open/dev mode, everyone is admin
	if adminTok == "" && testerTok == "" && viewerTok == "" {
		return RoleAdmin
	}

	switch {
	case token != "" && token == adminTok:
		return RoleAdmin
	case token != "" && token == testerTok:
		return RoleTester
	case token != "" && token == viewerTok:
		return RoleViewer
	default:
		return RoleNone
	}
}

func (h *AdminHandler) requireRole(w http.ResponseWriter, r *http.Request, minRole Role) (Role, bool) {
	role := h.resolveRole(r)
	if role == RoleNone {
		http.Error(w, `{"error":"unauthorized — provide X-Mock-Token header"}`, http.StatusUnauthorized)
		return role, false
	}
	// role hierarchy: admin > tester > viewer
	if minRole == RoleAdmin && role != RoleAdmin {
		http.Error(w, `{"error":"forbidden — admin role required"}`, http.StatusForbidden)
		return role, false
	}
	if minRole == RoleTester && role == RoleViewer {
		http.Error(w, `{"error":"forbidden — tester role required"}`, http.StatusForbidden)
		return role, false
	}
	return role, true
}

// ── GET /admin/me ─────────────────────────────────────────────────────────────

func (h *AdminHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	role := h.resolveRole(r)
	if role == RoleNone {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"role": string(role)})
}

// ── GET /admin/services ───────────────────────────────────────────────────────

type serviceInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Up   bool   `json:"up"`
}

func (h *AdminHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
		return
	}
	out := make([]serviceInfo, 0, len(h.cfg.Services))
	for _, svc := range h.cfg.Services {
		// quick health check — HEAD to service URL
		up := false
		resp, err := http.Head(svc.URL)
		if err == nil {
			resp.Body.Close()
			up = true
		}
		out = append(out, serviceInfo{svc.Name, svc.URL, up})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ── GET /admin/routes?service=cardhub ─────────────────────────────────────────

func (h *AdminHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
		return
	}
	svcName := r.URL.Query().Get("service")
	if svcName == "" {
		http.Error(w, `{"error":"service query param required"}`, http.StatusBadRequest)
		return
	}
	if _, ok := h.cfg.Service(svcName); !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown service: %s"}`, svcName), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.st.AllForService(svcName))
}

// ── PUT /admin/routes ─────────────────────────────────────────────────────────

type updateReq struct {
	Service    string          `json:"service"`
	Method     string          `json:"method"`
	Path       string          `json:"path"`
	Env        string          `json:"env"`
	Active     bool            `json:"active"`
	DelayMs    int             `json:"delay_ms"`
	FaultType  string          `json:"fault_type"`
	FaultProb  int             `json:"fault_prob"`
	// admin only:
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
	Headers    map[string]string `json:"headers"`
}

func (h *AdminHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	role, ok := h.requireRole(w, r, RoleTester)
	if !ok {
		return
	}
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Service == "" {
		http.Error(w, `{"error":"service field required"}`, http.StatusBadRequest)
		return
	}

	if role == RoleAdmin && req.Body != nil {
		h.st.UpdateFull(req.Service, req.Method, req.Path, req.Env, &store.MockConfig{
			Active:     req.Active,
			StatusCode: req.StatusCode,
			Body:       req.Body,
			Headers:    req.Headers,
			DelayMs:    req.DelayMs,
			FaultType:  req.FaultType,
			FaultProb:  req.FaultProb,
		})
	} else {
		h.st.UpdateRuntime(req.Service, req.Method, req.Path, req.Env,
			req.Active, req.DelayMs, req.FaultProb, req.FaultType)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── POST /admin/routes/reset?service=cardhub ──────────────────────────────────

func (h *AdminHandler) ResetRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleAdmin); !ok {
		return
	}
	svcName := r.URL.Query().Get("service")
	if svcName == "" {
		// reset all services
		seed.All(h.cfg, h.st)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	svc, ok := h.cfg.Service(svcName)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown service: %s"}`, svcName), http.StatusNotFound)
		return
	}
	if err := seed.Service(*svc, h.st); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── POST /admin/specs/:service — upload a new swagger.json ───────────────────

func (h *AdminHandler) UploadSpec(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleAdmin); !ok {
		return
	}
	svcName := r.URL.Query().Get("service")
	if svcName == "" {
		http.Error(w, `{"error":"service query param required"}`, http.StatusBadRequest)
		return
	}
	svc, ok := h.cfg.Service(svcName)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown service: %s"}`, svcName), http.StatusNotFound)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}
	// Validate it's valid JSON
	var check interface{}
	if err := json.Unmarshal(data, &check); err != nil {
		http.Error(w, `{"error":"invalid JSON in spec"}`, http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(svc.Spec, data, 0644); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"write spec: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if err := seed.Service(*svc, h.st); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"seed failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	fmt.Printf("[admin] spec uploaded and re-seeded for %s\n", svcName)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "seeded", "service": svcName})
}

// ── GET /admin/logs?service=cardhub ──────────────────────────────────────────

func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
		return
	}
	svcFilter := r.URL.Query().Get("service")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.logs.All(svcFilter))
}

// ── DELETE /admin/logs?service=cardhub ───────────────────────────────────────

func (h *AdminHandler) ClearLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	svcFilter := r.URL.Query().Get("service")
	h.logs.Clear(svcFilter)
	w.WriteHeader(http.StatusNoContent)
}
