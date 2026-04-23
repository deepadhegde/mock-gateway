# mock-gateway

A self-hosted mock server and transparent reverse proxy for all your backend services.  
Zero changes required to existing services — just point your app or test suite at the gateway instead of the real backend.

---

## Table of contents

1. [How it works](#how-it-works)
2. [Local setup](#local-setup)
3. [Adding a new service](#adding-a-new-service)
4. [Features](#features)
5. [Using the developer UI](#using-the-developer-ui)
   - [UI layout](#ui-layout)
   - [Connect to gateway](#connect-to-gateway)
   - [Browse and select a route](#browse-and-select-a-route)
   - [Environments — tier × purpose](#environments--tier--purpose)
   - [Sending a request](#sending-a-request)
   - [Enabling mock vs proxy](#enabling-mock-vs-proxy)
   - [Editing the mock response](#editing-the-mock-response)
   - [Saving changes](#saving-changes)
   - [Console — inspecting traffic](#console--inspecting-traffic)
6. [Headers reference](#headers-reference)
7. [Mock vs proxy — decision flow](#mock-vs-proxy--decision-flow)
8. [Advanced features](#advanced-features)
   - [Multiple scenarios](#multiple-scenarios)
   - [Delay / latency simulation](#delay--latency-simulation)
   - [Fault injection](#fault-injection)
   - [Response recording](#response-recording)
   - [State management](#state-management)
   - [Webhooks](#webhooks)
9. [Admin API](#admin-api)
10. [Access control](#access-control)
11. [CI/CD — auto-push spec on deploy](#cicd--auto-push-spec-on-deploy)
12. [Docker deployment](#docker-deployment)
13. [Troubleshooting](#troubleshooting)

---

## How it works

Every request carries three headers. The gateway reads them and either returns a stored mock response or transparently proxies to the real service:

```
X-Mock-Service:  cardhub        # which service to target
X-Mock-Enabled:  true           # set to "true" to use mock; omit to proxy
X-Mock-Env:      uat-api        # which environment's stored response to serve
```

**When `X-Mock-Enabled` is omitted or not `true`**, the gateway is a transparent reverse proxy — it forwards the request to the upstream URL defined in `config.yaml` and returns the real response. No mock data is touched.

**When `X-Mock-Enabled: true`**, the gateway looks up the stored mock config for that `service + method + path + env` combination and serves it directly without hitting the real backend.

---

## Local setup

### Prerequisites

- Go 1.21+
- A Swagger/OpenAPI 2.0 spec file (swagger.json) for each service you want to mock

### Steps

```bash
# 1. Clone the repo
git clone <repo-url> mock-gateway
cd mock-gateway

# 2. Install dependencies
go mod tidy

# 3. Copy your service's swagger spec
mkdir -p specs/cardhub
cp ~/cardhub/docs/swagger.json specs/cardhub/swagger.json

# 4. (Optional) set upstream URLs via env vars — defaults to localhost if unset
export CARDHUB_URL=http://localhost:8000

# 5. Run the gateway
make run

# 6. Open the developer UI
open http://localhost:9003/mock-ui/
```

The gateway starts on port **9003** by default (set in `config.yaml`).

On first start, mock data is seeded from the swagger spec automatically — one `default` scenario per route per environment combination.

### Run targets

| Command | Upstream URL (when mock is OFF) |
|---|---|
| `make run` | `http://localhost:8000` (local dev) |
| `make run-uat` | `https://cardhub-uat.popclub.co.in` |
| `make run-prod` | `https://cardhub.popclub.co.in` |

The upstream only matters when **Active for route** is OFF. When mock is ON the gateway never contacts the upstream.

---

## Adding a new service

### Step 1 — Add it to `config.yaml`

```yaml
services:
  - name: cardhub
    url: ${CARDHUB_URL:-http://localhost:8000}
    spec: specs/cardhub/swagger.json

  - name: my-new-service                            # ← add this block
    url: ${MY_SERVICE_URL:-http://localhost:8082}
    spec: specs/my-new-service/swagger.json
```

The `${VAR:-default}` syntax reads from an environment variable and falls back to the default if the variable is unset.

### Step 2 — Copy the swagger spec

```bash
mkdir -p specs/my-new-service
cp ~/my-new-service/docs/swagger.json specs/my-new-service/swagger.json
```

### Step 3 — Restart the gateway

```bash
make run
```

The gateway seeds all routes from the spec automatically on startup. Existing mock data is never overwritten — only new routes are added.

### Step 4 — Add run targets for UAT / prod (optional)

In `Makefile`:

```makefile
run-uat:
    CARDHUB_URL=https://cardhub-uat.popclub.co.in \
    MY_SERVICE_URL=https://my-service-uat.popclub.co.in \
    go run ./cmd/main.go
```

### Step 5 — (CI/CD) Auto-push spec on deploy

See [CI/CD — auto-push spec on deploy](#cicd--auto-push-spec-on-deploy).

---

## Features

| Feature | Description |
|---|---|
| **Transparent proxy** | Forwards requests to the real upstream when mock is OFF |
| **Per-route mock toggle** | Enable or disable mock independently for each route and env |
| **Six independent environments** | `uat-api`, `uat-ui`, `uat-dev`, `prod-api`, `prod-ui`, `prod-dev` — each stores a fully independent mock config |
| **Multiple scenarios** | Each route+env can have multiple named response variants; switch between them instantly |
| **Conditional scenario matching** | Scenarios can match on query params, request headers, body fields, or state keys |
| **Latency simulation** | Configurable per-route delay in milliseconds |
| **Fault injection** | Inject timeouts, empty responses, or malformed JSON at a configurable probability |
| **Response recording** | Capture a real backend response and replay it as a mock |
| **State machine** | Shared key-value state that scenarios can read and write — models multi-step flows |
| **Template variables** | Dynamic values in response bodies: `{{uuid}}`, `{{now}}`, `{{state.key}}`, etc. |
| **Webhooks** | Fire an async HTTP call after serving a mock response |
| **Persistent storage** | Mock config, scenarios, and state survive restarts via JSON snapshots |
| **Request log** | Ring buffer of the last 200 requests with full request/response detail |
| **Swagger seeding** | Automatically builds mock stubs from your swagger spec on first run |
| **Hot spec reload** | Upload a new swagger.json via API or UI — routes re-seed without restart |
| **Role-based access** | admin / tester / viewer roles with token auth (or open mode for local dev) |
| **Multi-service** | Run as a single gateway for all your backend services simultaneously |
| **Developer UI** | Browser-based UI to browse routes, edit responses, send requests, and inspect logs |

---

## Using the developer UI

Open **http://localhost:9003/mock-ui/** in your browser.

### UI layout

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│  ● mock-gateway  [http://localhost:9003 ▢]  ● 2 services  [X-Mock-Token ▢]  ☀/☽ │
├─────────────────────┬──────────────────────────────────────┬─────────────────────┤
│  🔍 Search routes…  │  REQUEST         │  CONSOLE (0)       │  Mock control       │
│                     │                  │                     │  ☐ Enable mock      │
│  [cardhub ▾]  [↺]  ├──────────────────┼─────────────────────┤  ☐ Active for route │
│  [All tags ▾]       │  POST ▾  [URL]   [Send]                │─────────────────────│
│  Spec: 66 routes    ├──────────────────────────────────────  │  Environment        │
│                     │  Headers│Params│Body│Mock response      │  [uat] [prod]       │
│  ▾ userlogin        │                                        │  [api] [ui] [dev]   │
│    ● POST /otp/send │  (request editor)                      │─────────────────────│
│    ○ POST /otp/verify│                                       │  Status code        │
│                     │ ─────────────── RESPONSE ──────────── │  [200        ▢]     │
│  ▾ useronboarding   │  200 OK  ·  12ms  ·  418B  mock-gw    │  Latency (ms)       │
│    ○ GET  /users    │  Body│Headers                         │  [0          ▢]     │
│    ○ POST /cards    │  { … response body … }                │─────────────────────│
│                     │                                        │  Fault injection    │
│                     │                                        │  [none       ▾]     │
│                     │                                        │  Prob [0     ▢]     │
│                     │                                        │─────────────────────│
│                     │                                        │  [Copy headers]     │
│                     │                                        │  [Save]             │
└─────────────────────┴────────────────────────────────────────┴─────────────────────┘
```

| Area | Purpose |
|---|---|
| **Top bar** | Connect to gateway URL, enter token, toggle light/dark mode |
| **Left sidebar** | Browse routes grouped by swagger tag; filter by name or tag |
| **Center top** | Switch between Request builder and Console (traffic log) |
| **Center main** | Build requests (headers, params, body, mock response editor) |
| **Center bottom** | Response pane — status, latency, body, headers |
| **Right panel** | Control mock behaviour, select environment, save config |

---

### Connect to gateway

1. The Gateway URL defaults to `http://localhost:9003` — change it if your gateway runs elsewhere
2. Press **Enter** — the dot in the top bar turns green when connected, red when unreachable
3. In open mode (no users in `config.yaml`) the role shows **dev mode** — full access, no token needed

If you have access control enabled, enter your token in the `X-Mock-Token` field:

| Token | Role | Permissions |
|---|---|---|
| admin token | admin | Everything including user management |
| tester token | tester | Toggle mock, edit responses, scenarios, state |
| viewer token | viewer | Read-only — view routes, logs, state |

---

### Browse and select a route

1. Use the **service dropdown** in the sidebar to select a service (e.g. `cardhub`)
2. The sidebar loads all routes from the spec, grouped by swagger tag
3. Use the **search box** to filter by path or method
4. Use the **tag dropdown** to show only one group
5. **Click a route** — the center pane fills in the URL, default headers, params, and request body from the spec

Route status dots:
- **Green (●)** — mock is active for the currently selected env
- **Grey (○)** — mock is off; requests will be proxied to the real upstream

---

### Environments — tier × purpose

The right panel has two rows of pills:

```
Tier:    [ uat ]  [ prod ]
Purpose: [ api ]  [ ui  ]  [ dev ]
```

Selecting a tier and purpose sets `X-Mock-Env` automatically.

| Tier | When to use |
|---|---|
| `uat` | Mock data that matches your UAT baseline — use with UAT test suites |
| `prod` | Mock data that matches prod-like responses — use for prod smoke tests |

| Purpose | Who uses it |
|---|---|
| `api` | Backend / API test suites (pytest, Go test, etc.) |
| `ui` | Mobile app or frontend in dev mode |
| `dev` | Developers iterating locally |

**All six combinations are fully independent.** Changing `prod-api` never affects `uat-ui`. Each env key stores its own: mock toggle, response body, status code, delay, fault config, and scenarios.

> **The tier pill does NOT change the upstream URL.** The upstream URL is set at startup via the `CARDHUB_URL` env var (or equivalent). Use `make run-uat` / `make run-prod` to control which backend passthrough hits.

#### How to set env in your app or test suite

```http
# Any HTTP request through the gateway
X-Mock-Service:  cardhub
X-Mock-Enabled:  true
X-Mock-Env:      uat-api
```

```python
# pytest / requests
MOCK_HEADERS = {
    "X-Mock-Service": "cardhub",
    "X-Mock-Enabled": "true",
    "X-Mock-Env":     "uat-api",
}
resp = requests.post("http://localhost:9003/api/v1/otp/send",
                     json={"phone": "9876543210"}, headers=MOCK_HEADERS)
```

```js
// Mobile app / React Native
const mockHeaders = {
  'X-Mock-Service': 'cardhub',
  'X-Mock-Enabled': 'true',
  'X-Mock-Env': 'uat-ui',
};
```

---

### Sending a request

1. Click a route in the sidebar
2. The **URL bar** pre-fills with the gateway base URL + path
3. Fill in any **path parameters** in the Params tab (e.g. `/api/v1/cards/{card_id}`)
4. Add or edit **request headers** in the Headers tab
5. Edit the **request body** in the Body tab (pre-filled from the spec schema)
6. Click **Send**

The response appears in the bottom pane:
- Status badge (`200 OK`, `404 Not Found`, etc.)
- Latency in ms
- Response size
- `mock-gateway` badge — confirms mock served it; `<service-name>` means it was proxied
- Response body (syntax highlighted JSON)
- Response headers tab

**Request state is saved per route + env.** Switching to a different route and back restores your last headers and body for that combination.

---

### Enabling mock vs proxy

This is controlled by **two independent toggles** in the right panel:

```
☑ Enable mock        ← sends X-Mock-Enabled: true on all requests from the UI
☑ Active for route   ← tells the gateway to actually serve mock for this route+env
```

| Enable mock | Active for route | What happens |
|---|---|---|
| OFF | any | Request proxies to real upstream — mock headers are NOT sent |
| ON | OFF | Mock headers sent, but gateway proxies because this route is not active |
| ON | ON | Gateway serves stored mock response — real backend never contacted |

**Typical workflow:**

1. Toggle **Enable mock** ON — this is a global switch for the UI session
2. For each route you want to mock: toggle **Active for route** ON → set response body → Save
3. Routes with **Active** OFF fall through to the real backend automatically
4. To bypass all mocking temporarily: toggle **Enable mock** OFF

---

### Editing the mock response

1. Click the **Mock response** tab in the center pane
2. The current stored body appears — pre-filled from the swagger spec on first load
3. Edit the JSON directly
4. Click **Format** to auto-indent
5. Change **Status code** in the right panel (default 200)
6. Click **Save**

Example — simulating a 429 rate limit:

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests. Retry after 60 seconds.",
  "retry_after": 60
}
```

Set Status code to `429`, click Save. All requests to that route+env now return 429.

---

### Saving changes

Click **Save** in the bottom-right of the right panel. This:

1. Saves the mock response body, status code, and headers
2. Saves the active toggle, delay, fault config
3. Persists immediately to `data/store.json` — survives restarts

A `● Unsaved changes` indicator appears whenever you modify anything. It clears after Save.

---

### Console — inspecting traffic

Click the **Console** tab at the top of the center pane.

```
METHOD  SERVICE   PATH                     ENV       STATUS  TYPE   LATENCY
POST    cardhub   /api/v1/otp/send         uat-api   200     MOCK   4ms
GET     cardhub   /api/v1/users            uat-ui    200     REAL   231ms
POST    cardhub   /api/v1/otp/verify       uat-api   200     MOCK   2ms
```

- **MOCK** (orange) — served from stored mock data
- **REAL** (grey) — proxied to real upstream

Click any row to open the detail pane:
- Left side: full request headers + body
- Right side: full response headers + body

Use the service dropdown to filter by service. Click **Clear** to empty the log.

---

## Headers reference

| Header | Where to set | Values | Required |
|---|---|---|---|
| `X-Mock-Service` | Your app / test / UI | Service name from `config.yaml` (e.g. `cardhub`) | Always (when multiple services; auto-set if only one) |
| `X-Mock-Enabled` | Your app / test / UI | `true` | Only when you want mock — omit for proxy-only |
| `X-Mock-Env` | Your app / test / UI | `uat-api`, `uat-ui`, `uat-dev`, `prod-api`, `prod-ui`, `prod-dev` | Required when `X-Mock-Enabled: true` |
| `X-Mock-Token` | Admin UI / CI | Token value from `config.yaml` | Only when access control is enabled |
| `Authorization` | Your app / test | `Bearer <jwt>` | Only for routes that require auth on the real backend |

Headers set in the UI's **Headers tab** are sent along with every request for that route (and persisted per route+env). Use this to keep your JWT or API key without re-entering it.

---

## Mock vs proxy — decision flow

```
Incoming request
      │
      ▼
 X-Mock-Enabled == "true"?
      │
   No ──────────────────────────────────────────► Proxy to upstream URL
      │
   Yes
      │
      ▼
 X-Mock-Env set?  AND  route config exists?
      │
   No ──────────────────────────────────────────► Proxy to upstream URL
      │
   Yes
      │
      ▼
 cfg.Active == true?
      │
   No ──────────────────────────────────────────► Proxy (optionally recording real response)
      │
   Yes
      │
      ▼
 Fault injection fires? (cfg.FaultProb > 0, rand check)
      │
   Yes ─────────────────────────────────────────► Return injected fault response
      │
   No
      │
      ▼
 Select scenario:
   1. First scenario whose match rules all pass
   2. Named active scenario (cfg.ActiveScenario)
   3. First scenario (default)
      │
      ▼
 Apply template variables ({{uuid}}, {{now}}, {{state.key}}, …)
      │
      ▼
 Write response → log entry → fire webhook (async) → update state
```

---

## Advanced features

### Multiple scenarios

Each route+env can store multiple named response variants. This lets you switch between success, error, and edge-case responses without editing the body each time.

**From the Admin API:**

```bash
# Add an "error" scenario
curl -X POST http://localhost:9003/admin/scenarios \
  -H "Content-Type: application/json" \
  -d '{
    "service": "cardhub",
    "method": "POST",
    "path": "/api/v1/otp/send",
    "env": "uat-api",
    "name": "error",
    "status_code": 500,
    "body": {"error": "internal_server_error"}
  }'

# Switch to the error scenario
curl -X PUT http://localhost:9003/admin/scenarios/active \
  -H "Content-Type: application/json" \
  -d '{
    "service": "cardhub",
    "method": "POST",
    "path": "/api/v1/otp/send",
    "env": "uat-api",
    "name": "error"
  }'
```

**Conditional matching** — a scenario fires automatically when its rules match:

```json
{
  "name": "wrong_otp",
  "status_code": 400,
  "body": {"error": "invalid_otp"},
  "match": [
    { "source": "body", "key": "otp", "value": "0000" }
  ]
}
```

Match sources: `body` (dot-notation JSON path), `query`, `header`, `state`.

---

### Delay / latency simulation

Set **Latency (ms)** in the right panel and Save. Every request to that route+env sleeps for that many milliseconds before responding — regardless of mock or proxy mode.

Use this to test:
- Loading spinners and skeleton screens
- Request timeout handling in your app
- Button debounce while a request is in flight

---

### Fault injection

Fault injection lets you simulate broken or slow upstream behaviour without touching the real backend — useful for testing how your app handles failures.

#### The four fault types

| Fault type | Status code | What the client receives | What it tests |
|---|---|---|---|
| `none` | — | Normal mock response (default) | — |
| `timeout` | none | No response for 30 seconds, then connection closes | App timeout handling, loading state, retry logic |
| `empty` | `200` | Completely empty body | Null / empty body handling |
| `malformed` | `200` | Body is `{bad json` (invalid JSON) | JSON parse error handling |
| `error` | `500` | `{"error":"fault_injected"}` | Generic server error handling |

> `timeout`, `empty`, and `malformed` all return `200` intentionally — a backend that *appears* to succeed but sends garbage is harder for apps to handle than a straightforward `500`.

#### Probability

The **Prob** field (0–100) controls how often the fault fires:

- `100` → every request gets the fault
- `50` → roughly half fail, half return the normal mock response
- `0` → fault disabled (default)

Set to 50 to simulate a **flaky upstream** — your app should handle intermittent failures gracefully.

#### How to use it in the UI

1. Click a route in the sidebar (e.g. `POST /api/v1/otp/send`)
2. Make sure **Active for route** is ON
3. In the right panel, find **Fault injection**
4. Pick a fault type from the dropdown — e.g. `Malformed JSON`
5. Set **Prob** to `100`
6. Click **Save**
7. Click **Send** — the response pane shows the broken response instead of your mock body

To turn it off: set **Prob** back to `0` (or fault type back to `none`) and Save.

#### Example — testing OTP endpoint failure

You want to verify your app shows a proper error when the OTP endpoint is completely down:

1. Select `POST /api/v1/otp/send`
2. Fault type: `error`, Prob: `100`
3. Save → Send → your app should show an error screen, not crash

Then set Prob to `50` to simulate a flaky service — your app should retry automatically.

---

### Response recording

Record exactly what the real backend returns and replay it as a mock.

1. Make sure **Active for route** is OFF (so requests proxy to real backend)
2. In the right panel, enable **Recording** and Save
3. Send the request — the real response is captured and saved as a `recorded` scenario
4. Toggle **Active for route** ON → click Save
5. Subsequent requests now serve the recorded response without hitting the backend

---

### State management

The gateway maintains a shared key-value store (the "state") that persists across requests. Scenarios can read from it (via match rules) and write to it (via `state_set`).

**Example — model a two-step flow:**

Step 1 scenario writes state after responding:
```json
{
  "name": "otp_sent",
  "body": {"message": "OTP sent"},
  "state_set": { "otp_sent": "true" }
}
```

Step 2 scenario only fires when state confirms step 1 completed:
```json
{
  "name": "verify_with_otp_sent",
  "body": {"token": "mock-jwt"},
  "match": [{ "source": "state", "key": "otp_sent", "value": "true" }]
}
```

**Manage state via API:**

```bash
# View current state
curl http://localhost:9003/admin/state

# Set state keys
curl -X PUT http://localhost:9003/admin/state \
  -H "Content-Type: application/json" \
  -d '{"user_id": "42", "kyc_status": "verified"}'

# Clear all state
curl -X DELETE http://localhost:9003/admin/state
```

**Template variables** let you inject state values directly into response bodies:

```json
{
  "user_id": "{{state.user_id}}",
  "request_id": "{{uuid}}",
  "timestamp": "{{now}}"
}
```

Supported: `{{uuid}}`, `{{now}}` (RFC3339), `{{now_unix}}`, `{{method}}`, `{{path}}`, `{{query.<name>}}`, `{{state.<key>}}`.

---

### Webhooks

Fire an async HTTP call after the mock response is sent — useful for testing webhook consumers.

```json
{
  "name": "default",
  "body": {"status": "ok"},
  "webhook": {
    "url": "http://localhost:8080/webhooks/payment",
    "method": "POST",
    "delay_ms": 500,
    "body": {"event": "payment.completed", "amount": 100},
    "headers": {"X-Webhook-Secret": "secret"}
  }
}
```

The webhook fires after the response is written. The `delay_ms` lets you simulate realistic async delivery timing.

---

## Admin API

All endpoints are under `/admin`. In open mode (no users in config) they require no auth.

```
GET    /admin/me                              current role
GET    /health                                gateway health check

GET    /admin/services                        list services + reachability status
GET    /admin/specs?service=X                 download current swagger spec
POST   /admin/specs?service=X                 upload new swagger spec (re-seeds routes)

GET    /admin/routes?service=X                list all routes for a service
PUT    /admin/routes                          update route mock config
POST   /admin/routes/reset                    re-seed all services from spec
POST   /admin/routes/reset?service=X          re-seed one service from spec

GET    /admin/scenarios?service=X&method=M&path=P&env=E   list scenarios for a route
POST   /admin/scenarios                       add or update a scenario
DELETE /admin/scenarios?service=X&...&name=N  delete a scenario
PUT    /admin/scenarios/active                set the active (fallback) scenario

GET    /admin/state                           get shared state
PUT    /admin/state                           merge key-value pairs into state
DELETE /admin/state                           clear all state

GET    /admin/logs?service=X                  request log (omit service for all)
DELETE /admin/logs?service=X                  clear request log

GET    /admin/users                           list dynamic users (admin only)
POST   /admin/users                           create dynamic user (admin only)
PATCH  /admin/users                           update dynamic user (admin only)
DELETE /admin/users?email=X                   delete dynamic user (admin only)
```

---

## Access control

By default `config.yaml` has no users — the gateway runs in **open mode**: all admin endpoints are accessible, all mock requests are served regardless of which env is requested.

To enable token-based access, add users in `config.yaml`:

```yaml
users:
  - name: "admin"
    token: ${MOCK_ADMIN_TOKEN:-mock-admin-secret}
    role: "admin"
    envs: ["uat-api", "uat-ui", "uat-dev", "prod-api", "prod-ui", "prod-dev"]

  - name: "tester"
    token: ${MOCK_TESTER_TOKEN:-mock-tester-secret}
    role: "tester"
    envs: ["uat-api", "uat-ui", "uat-dev", "prod-api", "prod-ui", "prod-dev"]

  - name: "viewer"
    token: ${MOCK_VIEWER_TOKEN:-mock-dev-secret}
    role: "viewer"
    envs: ["uat-dev", "prod-dev"]
```

Pass the token as `X-Mock-Token: <token>` on every admin API call.

| Role | Can do |
|---|---|
| `admin` | Everything — routes, scenarios, state, users, specs |
| `tester` | Edit routes, scenarios, state; upload specs; clear logs |
| `viewer` | Read-only — list routes, view logs, view state |

---

## CI/CD — auto-push spec on deploy

### How it triggers

The spec push is the **last step** of cardhub's existing CI pipeline. It fires automatically whenever you push to `uat` or `main`:

```
git push → uat or main
        │
        ▼
1. Build Docker image + push to ECR
2. Update deploy.yml in CICD repo  (Kubernetes rollout)
3. Push swagger spec to mock-gateway  ← this step
        │
        curl -X POST $MOCK_GATEWAY_URL/admin/specs?service=cardhub \
             --data-binary "@docs/swagger.json"
        │
        ▼
   Gateway saves the new spec + re-seeds all routes
   (new endpoints get a default stub; existing mock configs preserved)
```

The step uses `continue-on-error: true` so a failed or unreachable gateway never blocks a production deploy.

### Setup required

**1. Add GitHub secrets** — cardhub repo → Settings → Secrets → Actions:

| Secret | Value |
|---|---|
| `MOCK_GATEWAY_URL` | Public URL where mock-gateway is reachable (e.g. `http://mock-gateway.internal:9003`) |
| `MOCK_ADMIN_TOKEN` | Admin token from `config.yaml` — leave blank if running in open mode |

**2. Mock-gateway must be network-reachable from the CI runner.**  
Your runner is self-hosted on EKS. If mock-gateway is deployed in the same cluster or VPC, it will reach it. If it is running locally on your laptop, the CI runner cannot reach it — the step will warn but not fail the deploy.

Once mock-gateway is deployed to a persistent host (EC2, ECS, or inside the cluster), set `MOCK_GATEWAY_URL` to that address and the sync becomes fully automatic on every push.

### The workflow step (already in `build.yaml`)

```yaml
- name: Push swagger spec to mock-gateway
  env:
    MOCK_GATEWAY_URL: ${{ secrets.MOCK_GATEWAY_URL }}
    MOCK_ADMIN_TOKEN: ${{ secrets.MOCK_ADMIN_TOKEN }}
  run: |
    echo "Pushing swagger spec to mock-gateway (branch: ${GITHUB_REF##*/})"
    HTTP_STATUS=$(curl -s -o /tmp/mock-resp.json -w "%{http_code}" \
      -X POST "${MOCK_GATEWAY_URL}/admin/specs?service=cardhub" \
      -H "Content-Type: application/json" \
      -H "X-Mock-Token: ${MOCK_ADMIN_TOKEN}" \
      --data-binary "@docs/swagger.json")
    cat /tmp/mock-resp.json
    if [ "$HTTP_STATUS" != "200" ]; then
      echo "Warning: mock-gateway spec update returned HTTP $HTTP_STATUS — continuing"
    else
      echo "Mock-gateway spec updated and re-seeded"
    fi
  continue-on-error: true
```

---

## Docker deployment

```bash
# Build and start
make docker-up

# View logs
make docker-logs

# Stop
make docker-down

# Rebuild after code change
make docker-rebuild
```

The `docker-compose.yml` uses a named volume for `data/` so mock state and user records persist across container restarts. The `specs/` directory is bind-mounted so you can drop new spec files in without rebuilding the image.

Required environment variables for Docker:

```bash
export CARDHUB_URL=https://cardhub-uat.popclub.co.in
# Optional — only needed if access control is enabled in config.yaml:
export MOCK_ADMIN_TOKEN=your-admin-secret
export MOCK_TESTER_TOKEN=your-tester-secret
export MOCK_VIEWER_TOKEN=your-viewer-secret
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Top-bar dot is red | Gateway not running | `make run` |
| `no routes found` in sidebar | Spec not seeded or wrong service selected | Click ↺ next to service name; check `specs/cardhub/swagger.json` exists |
| `proxy: upstream error: EOF` | Real backend is down or unreachable | Expected when `Active for route` is OFF and upstream is not running — toggle mock ON |
| Response says `mock-gateway` but body is wrong | Wrong env selected | Check tier + purpose pills match `X-Mock-Env` your app sends |
| Request goes to real backend unexpectedly | `Active for route` is OFF for this env | Toggle it ON and Save |
| `{"error":"X-Mock-Service header required"}` | Multiple services, no service header | Add `X-Mock-Service: cardhub` in the Headers tab |
| `{"error":"unknown service: X"}` | Service name doesn't match `config.yaml` | Check the `name:` field in `config.yaml` — it's case-sensitive |
| State not updating between requests | Scenario `state_set` is empty | Verify the scenario that should write state has a `state_set` map |
| Spec upload fails with `seed failed` | Spec file path in `config.yaml` doesn't match | Ensure `spec:` path in `config.yaml` points to the right file |
| Build errors in VS Code (gopls) | VS Code is rooted at a different module | These are false positives — run `go build ./...` in `mock-gateway/` to verify |
