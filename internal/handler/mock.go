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
	"strconv"
	"strings"
	"time"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/proxy"
	"github.com/your-org/mock-gateway/internal/store"
)

type MockHandler struct {
	cfg  *config.Config
	st   *store.Store
	logs *store.LogRing
}

func NewMockHandler(cfg *config.Config, st *store.Store, logs *store.LogRing) *MockHandler {
	return &MockHandler{cfg: cfg, st: st, logs: logs}
}

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

	if strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, "/mock-ui") {
		http.NotFound(w, r)
		return
	}

	serviceName := r.Header.Get("X-Mock-Service")
	mockEnabled := r.Header.Get("X-Mock-Enabled")
	mockEnv     := r.Header.Get("X-Mock-Env")

	if serviceName == "" {
		if len(h.cfg.Services) == 1 {
			serviceName = h.cfg.Services[0].Name
		} else {
			http.Error(w, `{"error":"X-Mock-Service header required when multiple services are configured"}`, http.StatusBadRequest)
			return
		}
	}

	svc, ok := h.cfg.Service(serviceName)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown service: %s"}`, serviceName), http.StatusNotFound)
		return
	}

	reqBody := readBody(r)
	rec := &responseRecorder{ResponseWriter: w, status: 200}

	// Look up config regardless of active state (needed for recording).
	var cfg *store.MockConfig
	var cfgExists bool
	if mockEnv != "" {
		cfg, cfgExists = h.st.GetConfig(serviceName, r.Method, r.URL.Path, mockEnv)
	}

	useMock := mockEnabled == "true" && mockEnv != "" && cfgExists && cfg.Active

	if useMock {
		// Delay
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

		// Select scenario
		state := h.st.GetState()
		sc := selectScenario(cfg, r, reqBody, state)
		if sc == nil {
			http.Error(w, `{"error":"no scenario configured"}`, http.StatusNotFound)
			return
		}

		// Apply template variables
		body := applyTemplate(sc.Body, r, state)

		// Write response
		for k, v := range sc.Headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Served-By", "mock-gateway")
		w.Header().Set("X-Mock-Service", serviceName)
		w.Header().Set("X-Mock-Env", mockEnv)
		w.Header().Set("X-Mock-Scenario", sc.Name)
		w.WriteHeader(sc.StatusCode)
		w.Write(body)

		resHdrs := map[string]string{
			"Content-Type":    "application/json",
			"X-Served-By":     "mock-gateway",
			"X-Mock-Service":  serviceName,
			"X-Mock-Env":      mockEnv,
			"X-Mock-Scenario": sc.Name,
		}
		h.appendLog(serviceName, r, r.URL.Path, mockEnv, sc.StatusCode,
			true, reqBody, string(body), resHdrs, start)

		fmt.Printf("[mock] %-6s %-12s %-40s env=%-4s scenario=%-12s status=%d elapsed=%s\n",
			r.Method, serviceName, r.URL.Path, mockEnv, sc.Name, sc.StatusCode, time.Since(start))

		// Apply state updates after responding
		if len(sc.StateSet) > 0 {
			h.st.SetState(sc.StateSet)
		}

		// Fire webhook asynchronously
		if sc.Webhook != nil {
			go fireWebhook(sc.Webhook)
		}

	} else {
		// Proxy to the real upstream URL defined in config.yaml (CARDHUB_URL / SERVICE_B_URL etc.)
		if cfgExists && cfg.Recording {
			proxy.Forward(rec, r, svc.URL, r.URL.Path)
			if rec.body.Len() > 0 {
				captured := make([]byte, rec.body.Len())
				copy(captured, rec.body.Bytes())
				h.st.AddRecordedScenario(serviceName, r.Method, r.URL.Path, mockEnv,
					rec.status, json.RawMessage(captured), flattenHeaders(w.Header()))
				fmt.Printf("[record] %-6s %-12s %-40s env=%s → saved as 'recorded' scenario\n",
					r.Method, serviceName, r.URL.Path, mockEnv)
			}
		} else {
			proxy.Forward(rec, r, svc.URL, r.URL.Path)
		}
		h.appendLog(serviceName, r, r.URL.Path, mockEnv, rec.status,
			false, reqBody, rec.body.String(), flattenHeaders(w.Header()), start)
	}
}

// selectScenario picks the best scenario:
// 1. first scenario whose match rules all satisfy
// 2. the named active scenario
// 3. first scenario
func selectScenario(cfg *store.MockConfig, r *http.Request, reqBody string, state map[string]string) *store.Scenario {
	if len(cfg.Scenarios) == 0 {
		return nil
	}
	for i := range cfg.Scenarios {
		sc := &cfg.Scenarios[i]
		if len(sc.Match) > 0 && matchesAll(sc.Match, r, reqBody, state) {
			return sc
		}
	}
	if cfg.ActiveScenario != "" {
		for i := range cfg.Scenarios {
			if cfg.Scenarios[i].Name == cfg.ActiveScenario {
				return &cfg.Scenarios[i]
			}
		}
	}
	return &cfg.Scenarios[0]
}

func matchesAll(rules []store.MatchRule, r *http.Request, reqBody string, state map[string]string) bool {
	for _, rule := range rules {
		var got string
		switch rule.Source {
		case "query":
			got = r.URL.Query().Get(rule.Key)
		case "header":
			got = r.Header.Get(rule.Key)
		case "body":
			got = jsonPathGet(reqBody, rule.Key)
		case "state":
			got = state[rule.Key]
		default:
			return false
		}
		if got != rule.Value {
			return false
		}
	}
	return true
}

// jsonPathGet extracts a value at a dot-notation path from a JSON string.
func jsonPathGet(body, path string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return ""
	}
	cur := data
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur, ok = m[p]
		if !ok {
			return ""
		}
	}
	return fmt.Sprintf("%v", cur)
}

// applyTemplate replaces {{variable}} placeholders in a response body.
// Supported: {{uuid}}, {{now}}, {{now_unix}}, {{method}}, {{path}},
//            {{query.<name>}}, {{state.<key>}}
func applyTemplate(body []byte, r *http.Request, state map[string]string) []byte {
	s := string(body)
	if !strings.Contains(s, "{{") {
		return body
	}
	s = strings.ReplaceAll(s, "{{uuid}}", newUUID())
	s = strings.ReplaceAll(s, "{{now}}", time.Now().UTC().Format(time.RFC3339))
	s = strings.ReplaceAll(s, "{{now_unix}}", strconv.FormatInt(time.Now().Unix(), 10))
	s = strings.ReplaceAll(s, "{{method}}", r.Method)
	s = strings.ReplaceAll(s, "{{path}}", r.URL.Path)
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			s = strings.ReplaceAll(s, "{{query."+k+"}}", vals[0])
		}
	}
	for k, v := range state {
		s = strings.ReplaceAll(s, "{{state."+k+"}}", v)
	}
	return []byte(s)
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func fireWebhook(wh *store.Webhook) {
	if wh.DelayMs > 0 {
		time.Sleep(time.Duration(wh.DelayMs) * time.Millisecond)
	}
	method := wh.Method
	if method == "" {
		method = "POST"
	}
	var bodyReader io.Reader
	if len(wh.Body) > 0 {
		bodyReader = bytes.NewReader(wh.Body)
	}
	req, err := http.NewRequest(method, wh.URL, bodyReader)
	if err != nil {
		return
	}
	for k, v := range wh.Headers {
		req.Header.Set(k, v)
	}
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	http.DefaultClient.Do(req) //nolint:errcheck
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

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

