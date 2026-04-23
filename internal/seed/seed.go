package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/your-org/mock-gateway/internal/config"
	"github.com/your-org/mock-gateway/internal/store"
)

type swaggerSpec struct {
	Paths       map[string]map[string]swaggerOp `json:"paths"`
	Definitions map[string]swaggerSchema        `json:"definitions"`
}

type swaggerOp struct {
	Responses map[string]swaggerResponse `json:"responses"`
}

type swaggerResponse struct {
	Examples map[string]json.RawMessage `json:"examples"`
	Schema   *swaggerSchema             `json:"schema"`
}

type swaggerSchema struct {
	Ref                  string                     `json:"$ref"`
	Type                 string                     `json:"type"`
	Example              json.RawMessage            `json:"example"`
	Properties           map[string]*swaggerSchema  `json:"properties"`
	Items                *swaggerSchema             `json:"items"`
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
}

var defaultBody = json.RawMessage(`{"status":"ok"}`)

// resolveExample walks a schema and builds a synthetic JSON value.
// depth guards against circular $ref chains.
func resolveExample(schema *swaggerSchema, defs map[string]swaggerSchema, depth int) json.RawMessage {
	if depth > 8 {
		return json.RawMessage(`{}`)
	}

	if len(schema.Example) > 0 && string(schema.Example) != "null" {
		return schema.Example
	}

	if schema.Ref != "" {
		name := strings.TrimPrefix(schema.Ref, "#/definitions/")
		if def, ok := defs[name]; ok {
			return resolveExample(&def, defs, depth+1)
		}
		return defaultBody
	}

	switch schema.Type {
	case "array":
		if schema.Items != nil {
			item := resolveExample(schema.Items, defs, depth+1)
			return json.RawMessage("[" + string(item) + "]")
		}
		return json.RawMessage(`[]`)

	case "object", "":
		if len(schema.Properties) == 0 {
			return json.RawMessage(`{}`)
		}
		obj := make(map[string]interface{}, len(schema.Properties))
		for name, prop := range schema.Properties {
			raw := resolveExample(prop, defs, depth+1)
			var v interface{}
			json.Unmarshal(raw, &v) //nolint:errcheck
			obj[name] = v
		}
		b, _ := json.Marshal(obj)
		return b

	case "string":
		return json.RawMessage(`"string"`)
	case "integer":
		return json.RawMessage(`0`)
	case "number":
		return json.RawMessage(`0`)
	case "boolean":
		return json.RawMessage(`false`)
	}

	return defaultBody
}

// All seeds every service defined in config.
// Existing routes (e.g. loaded from snapshot) are left untouched.
func All(cfg *config.Config, s *store.Store) {
	for _, svc := range cfg.Services {
		if err := Service(svc, s); err != nil {
			fmt.Printf("[seed] %s: %v\n", svc.Name, err)
		} else {
			fmt.Printf("[seed] %s: seeded from %s\n", svc.Name, svc.Spec)
		}
	}
}

// AllReset re-seeds every service, overwriting spec fields (body, status code,
// headers) even for routes that already exist in the store.
func AllReset(cfg *config.Config, s *store.Store) {
	for _, svc := range cfg.Services {
		if err := ServiceReset(svc, s); err != nil {
			fmt.Printf("[seed] %s: %v\n", svc.Name, err)
		} else {
			fmt.Printf("[seed] %s: force-reseeded from %s\n", svc.Name, svc.Spec)
		}
	}
}

// Service fills in routes that do not yet exist in the store from a swagger
// spec file. Existing entries are not modified.
func Service(svc config.Service, s *store.Store) error {
	return seedService(svc, s, false)
}

// ServiceReset re-seeds one service, overwriting spec fields (body, status
// code, headers) for all existing routes as well as new ones.
func ServiceReset(svc config.Service, s *store.Store) error {
	return seedService(svc, s, true)
}

func seedService(svc config.Service, s *store.Store, force bool) error {
	data, err := os.ReadFile(svc.Spec)
	if err != nil {
		return fmt.Errorf("read spec: %w", err)
	}

	var doc swaggerSpec
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse spec: %w", err)
	}

	if doc.Definitions == nil {
		doc.Definitions = map[string]swaggerSchema{}
	}

	count := 0
	for path, methods := range doc.Paths {
		for method, op := range methods {
			body := extractBody(op, doc.Definitions)

			for _, env := range []string{"uat-api", "uat-ui", "uat-dev", "prod-api", "prod-ui", "prod-dev"} {
				if force {
					s.ForceReseedRoute(svc.Name, strings.ToUpper(method), path, env, 200, body)
				} else {
					s.SeedRoute(svc.Name, strings.ToUpper(method), path, env, 200, body)
				}
				count++
			}
		}
	}

	fmt.Printf("[seed] %s: %d route×env combinations seeded\n", svc.Name, count)
	return nil
}

// extractBody picks the best mock body for a 200 response in priority order:
// 1. explicit examples map
// 2. schema.example
// 3. resolved $ref or inline schema (synthetic)
// 4. default stub
func extractBody(op swaggerOp, defs map[string]swaggerSchema) json.RawMessage {
	resp, ok := op.Responses["200"]
	if !ok {
		return defaultBody
	}

	// explicit examples block
	for _, ex := range resp.Examples {
		return ex
	}

	if resp.Schema == nil {
		return defaultBody
	}

	// schema-level example
	if len(resp.Schema.Example) > 0 && string(resp.Schema.Example) != "null" {
		return resp.Schema.Example
	}

	// resolve from definitions
	result := resolveExample(resp.Schema, defs, 0)
	if string(result) == string(defaultBody) {
		return defaultBody
	}
	return result
}
