package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/proxy"
	"github.com/your-org/mock-gateway/internal/store"
)

// MockHandler is the main HTTP handler for all proxied requests.
// It reads X-Mock-Service, X-Mock-Enabled, X-Mock-Env headers to decide
// whether to return a mock response or proxy to the real service.
type MockHandler struct {
	cfg  *config.Config
	st   *store.Store
	logs *store.LogRing
}

func NewMockHandler(cfg *config.Config, st *store.Store, logs *store.LogRing) *MockHandler {
	return &MockHandler{cfg: cfg, st: st, logs: logs}
}

// responseRecorder captures status + body while still writing to real ResponseWriter.
type responseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *responseRecorder) WriteHeader(s int) {
	r.status = s
	r.ResponseWriter.WriteHeader(s)
}
func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (h *MockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Skip internal admin + UI paths
	if strings.HasPrefix(r.URL.Path, "/admin") ||
		strings.HasPrefix(r.URL.Path, "/mock-ui") {
		http.NotFound(w, r)
		return
	}

	// Read the three mock headers
	serviceName := r.Header.Get("X-Mock-Service")
	mockEnabled := r.Header.Get("X-Mock-Enabled")
	mockEnv     := r.Header.Get("X-Mock-Env")

	if serviceName == "" {
		http.Error(w, `{"error":"missing X-Mock-Service header"}`, http.StatusBadRequest)
		return
	}

	svc, ok := h.cfg.Service(serviceName)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown service: %s"}`, serviceName), http.StatusNotFound)
		return
	}

	// Read request body for logging (non-destructive)
	reqBody := readBody(r)

	rec := &responseRecorder{ResponseWriter: w, status: 200}

	// No mock headers → straight passthrough
	if mockEnabled != "true" || mockEnv == "" {
		proxy.Forward(rec, r, svc.URL, r.URL.Path)
		h.appendLog(serviceName, r, r.URL.Path, mockEnv, rec.status,
			false, reqBody, rec.body.String(), flattenHeaders(w.Header()), start)
		return
	}

	// Look up mock config
	cfg, found := h.st.Get(serviceName, r.Method, r.URL.Path, mockEnv)
	if !found {
		// configured but inactive → passthrough
		proxy.Forward(rec, r, svc.URL, r.URL.Path)
		h.appendLog(serviceName, r, r.URL.Path, mockEnv, rec.status,
			false, reqBody, rec.body.String(), flattenHeaders(w.Header()), start)
		return
	}

	// Delay simulation
	if cfg.DelayMs > 0 {
		time.Sleep(time.Duration(cfg.DelayMs) * time.Millisecond)
	}

	// Fault injection
	if cfg.FaultType != "" && cfg.FaultType != "none" && cfg.FaultProb > 0 {
		if mathrand.Intn(100) < cfg.FaultProb {
			injectFault(w, cfg.FaultType)
			faultBody := `{"error":"fault_injected","fault":"` + cfg.FaultType + `"}`
			h.appendLog(serviceName, r, r.URL.Path, mockEnv, 500,
				true, reqBody, faultBody,
				map[string]string{"X-Served-By": "mock-gateway"}, start)
			return
		}
	}

	// Write mock response
	for k, v := range cfg.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Served-By", "mock-gateway")
	w.Header().Set("X-Mock-Service", serviceName)
	w.Header().Set("X-Mock-Env", mockEnv)
	w.WriteHeader(cfg.StatusCode)
	w.Write(cfg.Body)

	resHdrs := map[string]string{
		"Content-Type":    "application/json",
		"X-Served-By":     "mock-gateway",
		"X-Mock-Service":  serviceName,
		"X-Mock-Env":      mockEnv,
	}
	h.appendLog(serviceName, r, r.URL.Path, mockEnv, cfg.StatusCode,
		true, reqBody, string(cfg.Body), resHdrs, start)

	fmt.Printf("[mock] %-6s %-12s %-40s env=%-4s status=%d elapsed=%s\n",
		r.Method, serviceName, r.URL.Path, mockEnv, cfg.StatusCode, time.Since(start))
}

func injectFault(w http.ResponseWriter, faultType string) {
	switch faultType {
	case "timeout":
		time.Sleep(30 * time.Second)
	case "empty":
		w.WriteHeader(http.StatusOK)
	case "malformed":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{bad json`))
	default:
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"fault_injected"}`))
	}
}

func (h *MockHandler) appendLog(service string, r *http.Request, path, env string,
	status int, mock bool, reqBody, resBody string,
	resHdrs map[string]string, start time.Time) {

	b := make([]byte, 8)
	rand.Read(b)

	h.logs.Append(store.LogEntry{
		ID:         hex.EncodeToString(b),
		Service:    service,
		Method:     r.Method,
		Path:       path,
		Env:        env,
		StatusCode: status,
		Mock:       mock,
		LatencyMs:  time.Since(start).Milliseconds(),
		ReqHeaders: flattenHeaders(r.Header),
		ReqBody:    reqBody,
		ResHeaders: resHdrs,
		ResBody:    resBody,
		Timestamp:  time.Now(),
	})
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func readBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(b))
	return string(b)
}

// ── Health check endpoint ────────────────────────────────────────────────────

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
