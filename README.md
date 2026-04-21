# mock-gateway

Centralised mock gateway for all Go backend services. Zero changes to existing services.

## How it works

All requests carry three headers. The gateway reads them, returns a mock response or proxies to the real service.

```
X-Mock-Service:  cardhub          # which service
X-Mock-Enabled:  true             # activate mock
X-Mock-Env:      api | ui | dev   # which response body
```

Without `X-Mock-Enabled: true` the gateway is a transparent proxy.

## Quick start

```bash
# 1. install dependencies
go mod tidy

# 2. copy your swagger specs
cp ~/cardhub/docs/swagger.json specs/cardhub/swagger.json

# 3. run
make run

# 4. open developer UI
open http://localhost:9000/mock-ui/

# 5. test mock
make test-mock

# 6. test passthrough
make test-passthrough
```

## Adding a new service

Edit `config.yaml`:

```yaml
services:
  - name: my-new-service
    url: http://localhost:8082
    spec: specs/my-new-service/swagger.json
```

Copy its spec:

```bash
cp ~/my-new-service/docs/swagger.json specs/my-new-service/swagger.json
```

Restart the gateway. No code changes.

## Headers

| Header | Required | Values | Purpose |
|---|---|---|---|
| `X-Mock-Service` | Always | service name from config.yaml | Identifies which service |
| `X-Mock-Enabled` | For mock | `true` | Activates mock lookup |
| `X-Mock-Env` | For mock | `api`, `ui`, `dev` | Picks response body |
| `X-Mock-Token` | Optional | token from config.yaml | Role-based access |

## Environments

| Env | Use case | Response style |
|---|---|---|
| `api` | API test suites (pytest etc.) | Lean, contract-focused body |
| `ui` | Playwright / UI automation | Rich body with all display fields |
| `dev` | Local development / debugging | Debug body with `_debug` flags |

## Admin API

```
GET    /admin/services              list registered services + up/down status
GET    /admin/routes?service=X      list all routes for a service
PUT    /admin/routes                update mock config (body: {service, method, path, env, ...})
POST   /admin/routes/reset          re-seed all services from spec files
POST   /admin/routes/reset?service=X  re-seed one service
POST   /admin/specs?service=X       upload new swagger.json for a service
GET    /admin/logs?service=X        request log (omit service for all)
DELETE /admin/logs?service=X        clear log
GET    /admin/me                    current role
GET    /health                      gateway health
```

## Role-based access

Set tokens in `config.yaml` (leave empty for open/dev mode):

```yaml
roles:
  admin_token:  "gw-admin-secret"    # full edit including body
  tester_token: "gw-tester-secret"   # toggle/delay/fault only
  viewer_token: "gw-viewer-secret"   # read-only
```

Pass token as `X-Mock-Token` header.

## Sync spec from CI

In each service's CI pipeline after `swag init`:

```bash
curl -X POST "http://mock-gateway.internal:9000/admin/specs?service=cardhub" \
  -H "X-Mock-Token: $MOCK_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary "@docs/swagger.json"
```

## Test config migration

```python
# before — direct to service
BASE = "http://localhost:8080"

# after — through gateway (only BASE changes)
BASE = "http://localhost:9000"
MOCK_HEADERS = {
    "X-Mock-Service": "cardhub",
    "X-Mock-Enabled": "true",
    "X-Mock-Env":     "api",
}
resp = requests.get(f"{BASE}/api/v1/cards/card_abc", headers=MOCK_HEADERS)
```

```python
# Playwright
await page.set_extra_http_headers({
    "X-Mock-Service": "cardhub",
    "X-Mock-Enabled": "true",
    "X-Mock-Env":     "ui",
})
await page.goto("http://localhost:9000")
```

## Docker

```bash
docker build -t mock-gateway .
docker run -p 9000:9000 mock-gateway
```
