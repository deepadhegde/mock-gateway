package store

import (
	"encoding/json"
	"sync"
	"time"
)

type RouteKey struct {
	Service string
	Method  string
	Path    string
	Env     string
}

// MatchRule is one condition evaluated against an incoming request or state.
// All rules on a scenario must match for it to be selected.
type MatchRule struct {
	Source string `json:"source"` // "query", "body", "header", "state"
	Key    string `json:"key"`
	Value  string `json:"value"`
}

// Webhook fires an async HTTP call after the mock response is sent.
type Webhook struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Body    json.RawMessage   `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	DelayMs int               `json:"delay_ms,omitempty"`
}

// Scenario is one named response variant for a route+env.
type Scenario struct {
	Name       string            `json:"name"`
	StatusCode int               `json:"status_code"`
	Body       json.RawMessage   `json:"body"`
	Headers    map[string]string `json:"headers,omitempty"`
	Match      []MatchRule       `json:"match,omitempty"`     // all must match; empty = default/fallback
	StateSet   map[string]string `json:"state_set,omitempty"` // state to write after serving
	Webhook    *Webhook          `json:"webhook,omitempty"`
}

// MockConfig holds the full config for one route+env.
type MockConfig struct {
	Active         bool       `json:"active"`
	DelayMs        int        `json:"delay_ms"`
	FaultType      string     `json:"fault_type"`
	FaultProb      int        `json:"fault_prob"`
	Recording      bool       `json:"recording"`       // capture real responses as "recorded" scenario
	Scenarios      []Scenario `json:"scenarios"`
	ActiveScenario string     `json:"active_scenario"` // fallback scenario name (empty = first)
}

type RouteEntry struct {
	Service string      `json:"service"`
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Env     string      `json:"env"`
	Config  *MockConfig `json:"config"`
}

type UserRecord struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Envs      []string  `json:"envs"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu           sync.RWMutex
	configs      map[RouteKey]*MockConfig
	users        map[string]*UserRecord
	state        map[string]string
	snapshotPath string
	usersPath    string
}

var defaultBody = json.RawMessage(`{"status":"ok"}`)

func New() *Store {
	return &Store{
		configs: make(map[RouteKey]*MockConfig),
		users:   make(map[string]*UserRecord),
		state:   make(map[string]string),
	}
}

func NewWithSnapshot(routesPath, usersPath string) *Store {
	return &Store{
		configs:      make(map[RouteKey]*MockConfig),
		users:        make(map[string]*UserRecord),
		state:        make(map[string]string),
		snapshotPath: routesPath,
		usersPath:    usersPath,
	}
}

func (s *Store) autoSave() {
	if s.snapshotPath != "" {
		_ = s.SaveSnapshot(s.snapshotPath)
	}
}

func (s *Store) autoSaveUsers() {
	if s.usersPath != "" {
		_ = s.SaveUsers(s.usersPath)
	}
}

// ── State ─────────────────────────────────────────────────────────────────────

func (s *Store) GetState() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.state))
	for k, v := range s.state {
		out[k] = v
	}
	return out
}

func (s *Store) SetState(kv map[string]string) {
	s.mu.Lock()
	for k, v := range kv {
		s.state[k] = v
	}
	s.mu.Unlock()
}

func (s *Store) ClearState() {
	s.mu.Lock()
	s.state = make(map[string]string)
	s.mu.Unlock()
}

// ── Users ─────────────────────────────────────────────────────────────────────

func (s *Store) AddUser(email, name, role string, envs []string) *UserRecord {
	s.mu.Lock()
	u := &UserRecord{Email: email, Name: name, Role: role, Envs: envs, CreatedAt: time.Now()}
	s.users[email] = u
	s.mu.Unlock()
	s.autoSaveUsers()
	return u
}

func (s *Store) RemoveUser(email string) bool {
	s.mu.Lock()
	_, ok := s.users[email]
	delete(s.users, email)
	s.mu.Unlock()
	if ok {
		s.autoSaveUsers()
	}
	return ok
}

func (s *Store) GetUserByEmail(email string) (*UserRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[email]
	return u, ok
}

func (s *Store) UpdateUser(email, role string, envs []string) bool {
	s.mu.Lock()
	u, ok := s.users[email]
	if ok {
		u.Role = role
		u.Envs = envs
	}
	s.mu.Unlock()
	if ok {
		s.autoSaveUsers()
	}
	return ok
}

func (s *Store) AllUsers() []UserRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UserRecord, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, *u)
	}
	return out
}

// ── Scenario helpers ──────────────────────────────────────────────────────────

func scenarioIndex(scenarios []Scenario, name string) int {
	for i, sc := range scenarios {
		if sc.Name == name {
			return i
		}
	}
	return -1
}

// UpsertScenario adds or replaces a named scenario on a route+env.
func (s *Store) UpsertScenario(service, method, path, env string, sc Scenario) {
	if sc.Body == nil {
		sc.Body = defaultBody
	}
	s.mu.Lock()
	key := RouteKey{service, method, path, env}
	cfg := s.configs[key]
	if cfg == nil {
		cfg = &MockConfig{ActiveScenario: sc.Name, Scenarios: []Scenario{sc}}
		s.configs[key] = cfg
	} else if i := scenarioIndex(cfg.Scenarios, sc.Name); i >= 0 {
		cfg.Scenarios[i] = sc
	} else {
		cfg.Scenarios = append(cfg.Scenarios, sc)
	}
	s.mu.Unlock()
	s.autoSave()
}

// DeleteScenario removes a named scenario.
func (s *Store) DeleteScenario(service, method, path, env, name string) bool {
	s.mu.Lock()
	key := RouteKey{service, method, path, env}
	cfg := s.configs[key]
	if cfg == nil {
		s.mu.Unlock()
		return false
	}
	i := scenarioIndex(cfg.Scenarios, name)
	if i < 0 {
		s.mu.Unlock()
		return false
	}
	cfg.Scenarios = append(cfg.Scenarios[:i], cfg.Scenarios[i+1:]...)
	s.mu.Unlock()
	s.autoSave()
	return true
}

// SetActiveScenario sets which scenario is the fallback when no match rules fire.
func (s *Store) SetActiveScenario(service, method, path, env, name string) bool {
	s.mu.Lock()
	key := RouteKey{service, method, path, env}
	cfg := s.configs[key]
	if cfg == nil {
		s.mu.Unlock()
		return false
	}
	cfg.ActiveScenario = name
	s.mu.Unlock()
	s.autoSave()
	return true
}

// AddRecordedScenario saves a captured real response as scenario "recorded".
func (s *Store) AddRecordedScenario(service, method, path, env string, statusCode int, body json.RawMessage, headers map[string]string) {
	s.UpsertScenario(service, method, path, env, Scenario{
		Name:       "recorded",
		StatusCode: statusCode,
		Body:       body,
		Headers:    headers,
	})
}

// ── Route CRUD ────────────────────────────────────────────────────────────────

func (s *Store) SeedRoute(service, method, path, env string, statusCode int, body json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := RouteKey{service, method, path, env}
	if s.configs[key] != nil {
		return
	}
	if body == nil {
		body = defaultBody
	}
	s.configs[key] = &MockConfig{
		Active:         false,
		ActiveScenario: "default",
		Scenarios:      []Scenario{{Name: "default", StatusCode: statusCode, Body: body, Headers: map[string]string{}}},
	}
}

func (s *Store) ForceReseedRoute(service, method, path, env string, statusCode int, body json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := RouteKey{service, method, path, env}
	if body == nil {
		body = defaultBody
	}
	defaultSc := Scenario{Name: "default", StatusCode: statusCode, Body: body, Headers: map[string]string{}}
	existing := s.configs[key]
	if existing == nil {
		s.configs[key] = &MockConfig{Active: false, ActiveScenario: "default", Scenarios: []Scenario{defaultSc}}
		return
	}
	if i := scenarioIndex(existing.Scenarios, "default"); i >= 0 {
		existing.Scenarios[i] = defaultSc
	} else {
		existing.Scenarios = append([]Scenario{defaultSc}, existing.Scenarios...)
	}
}

func (s *Store) UpdateRuntime(service, method, path, env string, active bool, delayMs, faultProb int, faultType string, recording bool) {
	s.mu.Lock()
	key := RouteKey{service, method, path, env}
	cfg := s.configs[key]
	if cfg == nil {
		cfg = &MockConfig{
			ActiveScenario: "default",
			Scenarios:      []Scenario{{Name: "default", StatusCode: 200, Body: defaultBody}},
		}
		s.configs[key] = cfg
	}
	cfg.Active    = active
	cfg.DelayMs   = delayMs
	cfg.FaultType = faultType
	cfg.FaultProb = faultProb
	cfg.Recording = recording
	s.mu.Unlock()
	s.autoSave()
}

// GetConfig returns config regardless of active state (used for recording check).
func (s *Store) GetConfig(service, method, path, env string) (*MockConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.configs[RouteKey{service, method, path, env}]
	return cfg, ok && cfg != nil
}

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

// ── Log ring buffer ───────────────────────────────────────────────────────────

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

func NewLogRing(max int) *LogRing { return &LogRing{max: max} }

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
