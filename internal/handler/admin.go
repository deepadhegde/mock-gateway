package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/seed"
	"github.com/your-org/mock-gateway/internal/store"
)

var healthClient = &http.Client{Timeout: 3 * time.Second}

type AdminHandler struct {
	cfg  *config.Config
	st   *store.Store
	logs *store.LogRing
}

func NewAdminHandler(cfg *config.Config, st *store.Store, logs *store.LogRing) *AdminHandler {
	return &AdminHandler{cfg: cfg, st: st, logs: logs}
}

// ── Role-based access ─────────────────────────────────────────────────────────

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleTester Role = "tester"
	RoleViewer Role = "viewer"
	RoleNone   Role = ""
)

func (h *AdminHandler) resolveRole(r *http.Request) Role {
	if h.cfg.OpenMode() {
		return RoleAdmin
	}
	if token := r.Header.Get("X-Mock-Token"); token != "" {
		if u, ok := h.cfg.UserByTokenSafe(token); ok {
			return Role(u.Role)
		}
	}
	if gt := bearerToken(r); gt != "" {
		email, _, err := verifyGoogleToken(gt, h.cfg.Gateway.GoogleClientID)
		if err == nil {
			if u, ok := h.st.GetUserByEmail(email); ok {
				return Role(u.Role)
			}
		}
	}
	return RoleNone
}

func (h *AdminHandler) requireRole(w http.ResponseWriter, r *http.Request, minRole Role) (Role, bool) {
	role := h.resolveRole(r)
	if role == RoleNone {
		http.Error(w, `{"error":"unauthorized — provide X-Mock-Token header"}`, http.StatusUnauthorized)
		return role, false
	}
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
		up := false
		resp, err := healthClient.Head(svc.URL)
		if err == nil {
			resp.Body.Close()
			up = resp.StatusCode < 500
		}
		out = append(out, serviceInfo{svc.Name, svc.URL, up})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ── GET /admin/routes?service=cardhub ────────────────────────────────────────

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
	Service   string `json:"service"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Env       string `json:"env"`
	Active    bool   `json:"active"`
	DelayMs   int    `json:"delay_ms"`
	FaultType string `json:"fault_type"`
	FaultProb int    `json:"fault_prob"`
	Recording bool   `json:"recording"`
	// Optional scenario upsert — if Body is present, upserts the named scenario.
	Scenario   string            `json:"scenario"`
	StatusCode int               `json:"status_code"`
	Body       json.RawMessage   `json:"body"`
	Headers    map[string]string `json:"headers"`
}

func (h *AdminHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
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

	h.st.UpdateRuntime(req.Service, req.Method, req.Path, req.Env,
		req.Active, req.DelayMs, req.FaultProb, req.FaultType, req.Recording)

	if len(req.Body) > 0 {
		name := req.Scenario
		if name == "" {
			name = "default"
		}
		statusCode := req.StatusCode
		if statusCode == 0 {
			statusCode = 200
		}
		h.st.UpsertScenario(req.Service, req.Method, req.Path, req.Env, store.Scenario{
			Name:       name,
			StatusCode: statusCode,
			Body:       req.Body,
			Headers:    req.Headers,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── POST /admin/routes/reset?service=cardhub ─────────────────────────────────

func (h *AdminHandler) ResetRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	svcName := r.URL.Query().Get("service")
	if svcName == "" {
		seed.AllReset(h.cfg, h.st)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	svc, ok := h.cfg.Service(svcName)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown service: %s"}`, svcName), http.StatusNotFound)
		return
	}
	if err := seed.ServiceReset(*svc, h.st); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Scenarios ─────────────────────────────────────────────────────────────────

type scenarioReq struct {
	Service    string            `json:"service"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Env        string            `json:"env"`
	Name       string            `json:"name"`
	StatusCode int               `json:"status_code"`
	Body       json.RawMessage   `json:"body"`
	Headers    map[string]string `json:"headers"`
	Match      []store.MatchRule `json:"match"`
	StateSet   map[string]string `json:"state_set"`
	Webhook    *store.Webhook    `json:"webhook"`
}

// GET /admin/scenarios?service=X&method=GET&path=/foo&env=api
func (h *AdminHandler) ListScenarios(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
		return
	}
	q := r.URL.Query()
	cfg, ok := h.st.GetConfig(q.Get("service"), q.Get("method"), q.Get("path"), q.Get("env"))
	if !ok {
		http.Error(w, `{"error":"route not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_scenario": cfg.ActiveScenario,
		"scenarios":       cfg.Scenarios,
	})
}

// POST /admin/scenarios — add or update a scenario
func (h *AdminHandler) UpsertScenario(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	var req scenarioReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Service == "" || req.Name == "" {
		http.Error(w, `{"error":"service and name required"}`, http.StatusBadRequest)
		return
	}
	statusCode := req.StatusCode
	if statusCode == 0 {
		statusCode = 200
	}
	h.st.UpsertScenario(req.Service, req.Method, req.Path, req.Env, store.Scenario{
		Name:       req.Name,
		StatusCode: statusCode,
		Body:       req.Body,
		Headers:    req.Headers,
		Match:      req.Match,
		StateSet:   req.StateSet,
		Webhook:    req.Webhook,
	})
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /admin/scenarios?service=X&method=GET&path=/foo&env=api&name=error
func (h *AdminHandler) DeleteScenario(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	q := r.URL.Query()
	if !h.st.DeleteScenario(q.Get("service"), q.Get("method"), q.Get("path"), q.Get("env"), q.Get("name")) {
		http.Error(w, `{"error":"scenario not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /admin/scenarios/active
func (h *AdminHandler) SetActiveScenario(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	var req struct {
		Service string `json:"service"`
		Method  string `json:"method"`
		Path    string `json:"path"`
		Env     string `json:"env"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"service, method, path, env, name required"}`, http.StatusBadRequest)
		return
	}
	if !h.st.SetActiveScenario(req.Service, req.Method, req.Path, req.Env, req.Name) {
		http.Error(w, `{"error":"route not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── State ─────────────────────────────────────────────────────────────────────

// GET /admin/state
func (h *AdminHandler) GetState(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.st.GetState())
}

// PUT /admin/state — merge key-value pairs into state
func (h *AdminHandler) SetState(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	var kv map[string]string
	if err := json.NewDecoder(r.Body).Decode(&kv); err != nil {
		http.Error(w, `{"error":"invalid JSON — expected object with string values"}`, http.StatusBadRequest)
		return
	}
	h.st.SetState(kv)
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /admin/state — clear all state
func (h *AdminHandler) ClearState(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	h.st.ClearState()
	w.WriteHeader(http.StatusNoContent)
}

// ── GET /admin/specs ──────────────────────────────────────────────────────────

func (h *AdminHandler) GetSpec(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
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
	data, err := os.ReadFile(svc.Spec)
	if err != nil {
		http.Error(w, `{"error":"spec file not found — run swag init and copy to specs/"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// ── POST /admin/specs ─────────────────────────────────────────────────────────

func (h *AdminHandler) UploadSpec(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
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
	var check interface{}
	if err := json.Unmarshal(data, &check); err != nil {
		http.Error(w, `{"error":"invalid JSON in spec"}`, http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(svc.Spec, data, 0600); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"write spec: %v"}`, err), http.StatusInternalServerError)
		return
	}
	if err := seed.ServiceReset(*svc, h.st); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"seed failed: %v"}`, err), http.StatusInternalServerError)
		return
	}
	fmt.Printf("[admin] spec uploaded and re-seeded for %s\n", svcName)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "seeded", "service": svcName})
}

// ── GET /admin/logs ───────────────────────────────────────────────────────────

func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleViewer); !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.logs.All(r.URL.Query().Get("service")))
}

// ── DELETE /admin/logs ────────────────────────────────────────────────────────

func (h *AdminHandler) ClearLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleTester); !ok {
		return
	}
	h.logs.Clear(r.URL.Query().Get("service"))
	w.WriteHeader(http.StatusNoContent)
}

// ── User management (admin only) ──────────────────────────────────────────────

type userReq struct {
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Role  string   `json:"role"`
	Envs  []string `json:"envs"`
}

func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleAdmin); !ok {
		return
	}
	var req userReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email, role and envs required"}`, http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "viewer"
	}
	if len(req.Envs) == 0 {
		req.Envs = []string{"dev"}
	}
	u := h.st.AddUser(req.Email, req.Name, req.Role, req.Envs)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(u)
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleAdmin); !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.st.AllUsers())
}

func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleAdmin); !ok {
		return
	}
	var req userReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}
	if !h.st.UpdateUser(req.Email, req.Role, req.Envs) {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireRole(w, r, RoleAdmin); !ok {
		return
	}
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, `{"error":"email query param required"}`, http.StatusBadRequest)
		return
	}
	if !h.st.RemoveUser(email) {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
