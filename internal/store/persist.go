package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type persistEntry struct {
	Service        string     `json:"service"`
	Method         string     `json:"method"`
	Path           string     `json:"path"`
	Env            string     `json:"env"`
	Active         bool       `json:"active"`
	DelayMs        int        `json:"delay_ms"`
	FaultType      string     `json:"fault_type"`
	FaultProb      int        `json:"fault_prob"`
	Recording      bool       `json:"recording"`
	Scenarios      []Scenario `json:"scenarios"`
	ActiveScenario string     `json:"active_scenario"`
}

func (s *Store) SaveSnapshot(path string) error {
	s.mu.RLock()
	entries := make([]persistEntry, 0, len(s.configs))
	for k, v := range s.configs {
		if v == nil {
			continue
		}
		entries = append(entries, persistEntry{
			Service:        k.Service,
			Method:         k.Method,
			Path:           k.Path,
			Env:            k.Env,
			Active:         v.Active,
			DelayMs:        v.DelayMs,
			FaultType:      v.FaultType,
			FaultProb:      v.FaultProb,
			Recording:      v.Recording,
			Scenarios:      v.Scenarios,
			ActiveScenario: v.ActiveScenario,
		})
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("persist: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("persist: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("persist: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("persist: rename: %w", err)
	}
	return nil
}

func (s *Store) LoadSnapshot(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("persist: read %s: %w", path, err)
	}
	var entries []persistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("persist: unmarshal: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		key := RouteKey{e.Service, e.Method, e.Path, e.Env}
		s.configs[key] = &MockConfig{
			Active:         e.Active,
			DelayMs:        e.DelayMs,
			FaultType:      e.FaultType,
			FaultProb:      e.FaultProb,
			Recording:      e.Recording,
			Scenarios:      e.Scenarios,
			ActiveScenario: e.ActiveScenario,
		}
	}
	fmt.Printf("[persist] loaded %d entries from %s\n", len(entries), path)
	return nil
}

func (s *Store) SaveUsers(path string) error {
	s.mu.RLock()
	users := make([]*UserRecord, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	s.mu.RUnlock()
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return fmt.Errorf("persist users: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("persist users: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("persist users: write: %w", err)
	}
	return os.Rename(tmp, path)
}

func (s *Store) LoadUsers(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("persist users: read %s: %w", path, err)
	}
	var users []*UserRecord
	if err := json.Unmarshal(data, &users); err != nil {
		return fmt.Errorf("persist users: unmarshal: %w", err)
	}
	s.mu.Lock()
	for _, u := range users {
		s.users[u.Email] = u
	}
	s.mu.Unlock()
	fmt.Printf("[persist] loaded %d users from %s\n", len(users), path)
	return nil
}
