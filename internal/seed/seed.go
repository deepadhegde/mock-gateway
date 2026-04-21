package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/store"
)

type swaggerDoc struct {
	Paths map[string]map[string]struct {
		Responses map[string]struct {
			Examples map[string]json.RawMessage `json:"examples"`
			Schema   *struct {
				Example json.RawMessage `json:"example"`
			} `json:"schema"`
		} `json:"responses"`
	} `json:"paths"`
}

var defaultBody = json.RawMessage(`{"status":"ok"}`)

// All seeds every service defined in config.
func All(cfg *config.Config, s *store.Store) {
	for _, svc := range cfg.Services {
		if err := Service(svc, s); err != nil {
			fmt.Printf("[seed] %s: %v\n", svc.Name, err)
		} else {
			fmt.Printf("[seed] %s: seeded from %s\n", svc.Name, svc.Spec)
		}
	}
}

// Service seeds one service from its swagger spec file.
func Service(svc config.Service, s *store.Store) error {
	data, err := os.ReadFile(svc.Spec)
	if err != nil {
		return fmt.Errorf("read spec: %w", err)
	}

	var doc swaggerDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse spec: %w", err)
	}

	count := 0
	for path, methods := range doc.Paths {
		for method, op := range methods {
			body := defaultBody

			if resp, ok := op.Responses["200"]; ok {
				for _, ex := range resp.Examples {
					body = ex
					break
				}
				if string(body) == string(defaultBody) &&
					resp.Schema != nil &&
					resp.Schema.Example != nil {
					body = resp.Schema.Example
				}
			}

			for _, env := range []string{"api", "ui", "dev"} {
				s.SeedRoute(svc.Name, strings.ToUpper(method), path, env, 200, body)
				count++
			}
		}
	}

	fmt.Printf("[seed] %s: %d route×env combinations seeded\n", svc.Name, count)
	return nil
}
