package store

import (
	"encoding/json"
	"sync"
	"time"
)

// RouteKey uniquely identifies one mock config: service + method + path + env.
type RouteKey struct {
	Service string
	Method  string
	Path    string
	Env     string
}

// MockConfig holds the full config for one route+env.
// Body and StatusCode are seeded from the Swagger spec — read-only for
// tester role. Only admin role can overwrite them via UpdateFull.
type MockConfig struct {
	Active     bool              `json:"active"`
	StatusCode int               `json:"status_code"`
	Body       json.RawMessage   `json:"body"`
	Headers    map[string]string `json:"headers"`
	DelayMs    int               `json:"delay_ms"`
	FaultType  string            `json:"fault_type"`
	FaultProb  int               `json:"fault_prob"`
}

// RouteEntry is the JSON shape for the admin list endpoint.
type RouteEntry struct {
	Service string      `json:"service"`
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Env     string      `json:"env"`
	Config  *MockConfig `json:"config"`
}

// Store is the thread-safe in-memory mock config store.
type Store struct {
	mu      sync.RWMutex
	configs map[RouteKey]*MockConfig
}

var defaultBody = json.RawMessage(`{"status":"ok"}`)

func New() *Store {
	return &Store{configs: make(map[RouteKey]*MockConfig)}
}

// SeedRoute is called only by seed on startup or /admin/routes/reset.
// Preserves existing runtime config (active, delay, fault).
func (s *Store) SeedRoute(service, method, path, env string, statusCode int, body json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := RouteKey{service, method, path, env}
	existing := s.configs[key]
	if existing == nil {
		existing = &MockConfig{Active: false}
	}
	existing.StatusCode = statusCode
	if body != nil {
		existing.Body = body
	} else {
		existing.Body = defaultBody
	}
	existing.Headers = map[string]string{}
	s.configs[key] = existing
}

// UpdateRuntime is called by tester + admin roles.
// Only touches Active, DelayMs, FaultType, FaultProb. Body/StatusCode unchanged.
func (s *Store) UpdateRuntime(service, method, path, env string, active bool, delayMs, faultProb int, faultType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := RouteKey{service, method, path, env}
	cfg := s.configs[key]
	if cfg == nil {
		return
	}
	cfg.Active    = active
	cfg.DelayMs   = delayMs
	cfg.FaultType = faultType
	cfg.FaultProb = faultProb
}

// UpdateFull replaces entire config — admin role only.
func (s *Store) UpdateFull(service, method, path, env string, cfg *MockConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[RouteKey{service, method, path, env}] = cfg
}

// Get returns config if it exists and is active.
func (s *Store) Get(service, method, path, env string) (*MockConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[RouteKey{service, method, path, env}]
	return cfg, ok && cfg != nil && cfg.Active
}

// AllForService returns all entries for one service.
func (s *Store) AllForService(service string) []RouteEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []RouteEntry
	for k, v := range s.configs {
		if k.Service == service {
			out = append(out, RouteEntry{k.Service, k.Method, k.Path, k.Env, v})
		}
	}
	return out
}

// ── Log ring buffer ──────────────────────────────────────────────────────────

type LogEntry struct {
	ID         string            `json:"id"`
	Service    string            `json:"service"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Env        string            `json:"env"`
	StatusCode int               `json:"status_code"`
	Mock       bool              `json:"mock"`
	LatencyMs  int64             `json:"latency_ms"`
	ReqHeaders map[string]string `json:"req_headers"`
	ReqBody    string            `json:"req_body"`
	ResHeaders map[string]string `json:"res_headers"`
	ResBody    string            `json:"res_body"`
	Timestamp  time.Time         `json:"timestamp"`
}

type LogRing struct {
	mu      sync.RWMutex
	entries []LogEntry
	max     int
}

func NewLogRing(max int) *LogRing {
	return &LogRing{max: max}
}

func (l *LogRing) Append(e LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append([]LogEntry{e}, l.entries...)
	if len(l.entries) > l.max {
		l.entries = l.entries[:l.max]
	}
}

func (l *LogRing) All(service string) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if service == "" {
		return append([]LogEntry{}, l.entries...)
	}
	var out []LogEntry
	for _, e := range l.entries {
		if e.Service == service {
			out = append(out, e)
		}
	}
	return out
}

func (l *LogRing) Clear(service string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if service == "" {
		l.entries = nil
		return
	}
	var kept []LogEntry
	for _, e := range l.entries {
		if e.Service != service {
			kept = append(kept, e)
		}
	}
	l.entries = kept
}
