# CardHub — Mock Gateway Testing Guide

Step-by-step instructions for testing CardHub flows in the mock gateway UI.

---

## Starting the Gateway

One `config.yaml` controls everything. The upstream URL is set via environment variable — defaults to `localhost` if unset.

| Command | Upstream (when mock is OFF) |
|---|---|
| `make run` | `http://localhost:8000` (local cardhub) |
| `make run-uat` | `https://cardhub-uat.popclub.co.in` |
| `make run-prod` | `https://cardhub.popclub.co.in` |

```bash
# local dev
make run

# point passthrough at UAT
make run-uat

# point passthrough at prod
make run-prod
```

> The upstream URL only matters when **Active for route** is **OFF**. When mock is ON, the gateway serves the stored response regardless of which upstream is configured.

Open **http://localhost:9003/mock-ui/** in your browser.

---

## UI Layout

```
┌─────────────────────────────────────────────────────────────────────────┐
│ ● Mock Gateway  [http://localhost:9003]  ● 2 services  [X-Mock-Token ▢] │
├──────────────────┬────────────────────────────────────────┬─────────────┤
│ Search routes…   │  Request  │  Console (0)               │             │
│ ● cardhub ▾  🔄  ├────────────────────────────────────────┤ Mock        │
│ All tags ▾       │  POST ▾   [URL]              [Send]    │ control     │
│ Spec: 66 routes  ├────────────────────────────────────────┤             │
│                  │  Headers  Params  Body  Mock response   │ Environment │
│ userlogin        │                                         │ uat | prod  │
│  ● POST /otp/…   │  (request editor area)                  │ api|ui|dev  │
│  ● POST /otp/…   │                                         │             │
│                  │──────────── RESPONSE ───────────────────│ Status code │
│ useronboarding   │  200 OK  12ms  418 B  mock-gateway      │             │
│  ● GET  /users   │  Body  Headers                          │ Latency     │
│  ● POST /cards   │  { … }                                  │             │
│                  │                                         │ Fault       │
│                  │                                         │ [Save]      │
└──────────────────┴────────────────────────────────────────┴─────────────┘
```

| Area | Purpose |
|---|---|
| **Top bar** | Connect to gateway, set your token, see role |
| **Left sidebar** | Browse all CardHub routes grouped by tag |
| **Center — Request** | Build and send requests |
| **Center — Console** | See all request logs |
| **Right panel** | Control mock behaviour per route |

---

## Connect and Authenticate

1. The **Gateway URL** field defaults to `http://localhost:9003` — change it if the gateway runs elsewhere
2. Enter your token in the **X-Mock-Token** field on the right of the top bar (skip in open mode):

   | Token | Role | Can do |
   |---|---|---|
   | `mock-admin-secret` | admin | Everything |
   | `mock-tester-secret` | tester | Edit routes, scenarios, state |
   | `mock-dev-secret` | viewer | Read-only (uat-dev / prod-dev only) |

3. Press **Enter** or click outside the field — the dot turns **green** and your role appears

---

## Environments

The right panel has **two rows of pills** that together define the active environment:

```
Row 1 — Tier:    [ uat ]  [ prod ]
Row 2 — Purpose: [ api ]  [ ui ]  [ dev ]
```

Selecting `uat` + `api` sends `X-Mock-Env: uat-api`.

### What tier and purpose mean

| Tier | Meaning |
|---|---|
| `uat` | Mock data reflecting UAT baseline responses |
| `prod` | Mock data reflecting prod-like responses |

| Purpose | Used by |
|---|---|
| `api` | Backend / API test suites |
| `ui` | Mobile app / frontend in dev mode |
| `dev` | Developers iterating locally |

**Each of the six combinations is fully independent** — mock body, status code, delay, and fault are stored separately per env key. Changing `prod-api` never affects `uat-ui`.

> **Tier ≠ upstream URL.** The tier pill controls which mock data set is active. The actual upstream (when mock is OFF) is set by the `CARDHUB_URL` env var at startup — see [Starting the Gateway](#starting-the-gateway).

### How to set the env in your app / test suite

```js
// Mobile app (frontend, UAT mock data)
headers: {
  'X-Mock-Enabled': 'true',
  'X-Mock-Env': 'uat-ui',
  'X-Mock-Service': 'cardhub'
}
```

```go
// Go API test suite
req.Header.Set("X-Mock-Enabled", "true")
req.Header.Set("X-Mock-Env", "uat-api")
req.Header.Set("X-Mock-Service", "cardhub")
```

---

## Select CardHub

Open the **service dropdown** in the sidebar and select `cardhub`.

- The spec badge shows **66 routes** when loaded
- Routes appear grouped by tag: `userlogin`, `useronboarding`, `card`, etc.
- **Green dot** next to a route = mock active for the selected env; **grey dot** = passthrough

Use the **Search** box to filter routes. Use the **tag filter** to show only one group.

---

## Flow 1 — OTP Login

### Send OTP

1. Click `POST /api/v1/otp/send` in the sidebar
2. The center pane fills in:
   - URL bar: `POST http://localhost:9003/api/v1/otp/send`
   - Body tab opens with the request body pre-filled from the swagger spec
3. In the **right panel**:
   - Toggle **Enable mock** → ON
   - Toggle **Active for route** → ON
   - Select tier: **uat**, purpose: **api**
4. Click the **Mock response** tab (top of center pane)
   - The body seeded from the swagger spec appears
   - Edit it to something realistic:
     ```json
     {
       "message": "OTP sent successfully",
       "mobile_number": "9876543210",
       "first_time_user": false,
       "is_campaign_active": false
     }
     ```
5. Click **Save** in the right panel

6. Click the **Body** tab, confirm the request body:
   ```json
   { "phone": "9876543210" }
   ```
7. Click **Send**

**Response pane shows:**
- Status badge: `200 OK`
- Latency in ms
- `mock-gateway` badge (confirms mock served it, not the real backend)
- The body you saved

---

### Verify OTP

1. Click `POST /api/v1/otp/verify`
2. Toggle **Active for route** → ON in the right panel
3. Edit the **Mock response** body:
   ```json
   {
     "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.mock",
     "message": "OTP verified",
     "mobile_number": "9876543210",
     "user_id": 42
   }
   ```
4. Click **Save**, then **Send**

> Switch to the **Console** tab after both calls — you'll see both requests logged with method, path, env (`uat-api`), status, and whether they were mocked or real.

---

## Flow 2 — Error Scenarios

### Simulate 429 Rate Limit

1. Click `POST /api/v1/otp/send`
2. In the right panel, set **Status code** to `429`
3. In the **Mock response** tab, replace the body with:
   ```json
   {
     "error": "rate_limit_exceeded",
     "message": "Too many OTP requests. Try again in 60 seconds.",
     "retry_after": 60
   }
   ```
4. Click **Save**, then **Send** — you get a **429** response

Switch back to `200` and save when done.

### Simulate 500 Server Error

1. Select the route
2. Set **Status code** to `500`
3. Set **Mock response** body:
   ```json
   { "error": "internal_server_error", "message": "Something went wrong" }
   ```
4. Click **Save**, then **Send**

### Simulate a broken upstream (fault injection)

1. Select any route that is active
2. In the right panel, open the **Fault injection** section:
   - **Fault type**: `Malformed JSON`
   - **Probability**: `100`
3. Click **Save**, then **Send** — response body is broken JSON, your app's JSON parser should handle it
4. Try **Fault type: Timeout** — the request hangs for 30 seconds, simulating a dead upstream

> Set probability to 50% to simulate a flaky upstream — half your requests fail, half succeed.

---

## Flow 3 — Slow Network / Loading States

1. Select `GET /api/v1/users`
2. Toggle **Active for route** ON
3. In the right panel, drag the **Latency** slider to `2000` (2 seconds)
4. Click **Save**, then **Send**
5. Watch the loading state in your frontend for 2 seconds before the response arrives

Use this to test:
- Skeleton screens and loading spinners
- Timeout handling in mobile apps
- Button debounce while a request is in flight

---

## Flow 4 — UAT vs Prod Mock Data

The same route stores independent mock data per env key. Use this to maintain separate response sets that match what each backend actually returns.

### Example: different user profiles per tier

1. Select `GET /api/v1/users`
2. Select tier **uat**, purpose **api** → set a body reflecting UAT test data → Save
3. Switch tier to **prod** (keep purpose **api**) → the mock response field clears (no prod data yet)
4. Set a body reflecting prod-like data → Save

Now `uat-api` and `prod-api` return different user profiles. Your API tests can target either gateway env without changing test code — just change `X-Mock-Env`.

### Example: different responses for UI vs API teams

For the same route on the same tier:

1. Select tier **uat**, purpose **api** → set a technically-correct minimal response → Save
2. Switch purpose to **ui** → set a response with realistic display names, profile photos, formatted addresses → Save
3. Switch purpose to **dev** → set a fast, stub response for local iteration → Save

The sidebar dots reflect active state for whichever `tier + purpose` combination is currently selected.

---

## Flow 5 — Record a Real Response

Use this to capture exactly what the real backend returns and replay it as a mock — no manual body writing.

1. Select a route, e.g. `POST /api/v1/otp/send`
2. Select tier **uat** (or **prod**), choose any purpose
3. Make sure **Active for route** is **OFF** — requests proxy to the real backend
4. In the **Headers** tab, add the real auth header:
   - Click **+ Add header**
   - Key: `Source-Api-Key`, Value: `<your-api-key>`
5. Click **Send** — the real response appears in the response pane
6. Copy the response body → paste into **Mock response** tab
7. Toggle **Active for route** → ON → Click **Save**

Now that env key replays the real response as a mock — no backend dependency.

---

## Flow 6 — User Onboarding

Test the full onboarding sequence: OTP → profile → address → KYC

### Step 1: Pick your env

Select tier **uat**, purpose **ui** (or whichever combination your frontend uses).

### Step 2: Activate all routes in sequence

For each route below, click it → toggle **Active** ON → edit body if needed → Save:

| Route | Mock body hint |
|---|---|
| `POST /api/v1/otp/send` | `{"message":"OTP sent","first_time_user":true}` |
| `POST /api/v1/otp/verify` | `{"token":"mock-jwt","user_id":1,"first_time_user":true}` |
| `GET /api/v1/users` | Profile with blank fields (new user) |
| `PUT /api/v1/users` | Updated profile echoed back |
| `POST /api/v1/addresses` | `{"id":1,"status":"saved"}` |
| `GET /api/v1/ekyc` | `{"ekyc_url":"https://sandbox.ekyc.example.com/session/abc"}` |

### Step 3: Add JWT to headers for authenticated routes

After OTP verify returns a token:
1. In the **Headers** tab, click **+ Add header**
2. Key: `Authorization`, Value: `Bearer mock-jwt`
3. This persists across route switches so you don't re-add it every time

### Step 4: Walk through the flow

Click each route in the sidebar and Send in order. The Console tab gives a full trace of the session — each row shows the env key (`uat-ui`) so you can confirm the right config was used.

---

## Flow 7 — Card Creation

1. Click `POST /api/v1/cards`
2. Toggle **Active** ON
3. The **Params** tab shows any path or query parameters — fill them in
4. The **Body** tab pre-fills the request schema from the spec
5. Edit the **Mock response** to a realistic card response:
   ```json
   {
     "card_id": "CARD-0042",
     "status": "pending",
     "masked_number": "XXXX-XXXX-XXXX-4242",
     "created_at": "2026-04-23T12:00:00Z"
   }
   ```
6. Save and Send

---

## Console Tab — Inspecting Traffic

Click **Console** in the top tabs.

| Column | What it shows |
|---|---|
| Method | GET / POST / etc |
| Service | Which service handled it |
| Path | The endpoint |
| Env | `uat-api` / `uat-ui` / `prod-api` / etc. |
| Status | HTTP status code |
| Type | **MOCK** (orange) or **REAL** (grey) |
| Time | Latency in ms |
| At | Timestamp |

**Click any row** — a detail pane opens at the bottom showing:
- Left: full request headers + body
- Right: full response headers + body

Use this to verify:
- The right env key was used (`uat-api`, `prod-ui`, etc.)
- Request body was sent correctly
- Response matches what your app expects

**Filter** by service using the dropdown. **Clear** removes all logs.

---

## Copy Headers for Postman or curl

1. Select a route and configure it in the right panel
2. Click **Copy headers** (next to Save)
3. This copies the active headers to clipboard, e.g.:
   ```
   X-Mock-Service: cardhub
   X-Mock-Enabled: true
   X-Mock-Env: uat-api
   ```

Paste these into Postman's Headers tab or a curl `-H` flag.

---

## Reload Spec

If you update the swagger spec (`docs/swagger.json`):

1. Copy the new file: `cp ../cardhub/docs/swagger.json specs/cardhub/swagger.json`
2. Click the **🔄** button next to the service name in the sidebar
3. Routes re-seed from the new spec; mock data you already saved per env key is preserved

---

## Common Issues

| Problem | Fix |
|---|---|
| Status dot is red | Gateway not running — `make run` (or `make run-uat` / `make run-prod`) |
| Role shows `dev mode` | No token set — enter one in the top-right field |
| Spec shows `no spec` | Run `cp docs/swagger.json specs/cardhub/` then click 🔄 |
| Request goes to real backend | Check **Active for route** is ON and **Enable mock** is ON |
| Passthrough hits wrong backend | Check which `make run-*` command was used — it sets the upstream URL |
| 401 Unauthorized | Route auth requires `Authorization` header — add it in Headers tab |
| Body is greyed out | You have viewer role — use `mock-tester-secret` token |
| Wrong mock data returned | Check tier + purpose pills match the env your app is sending |
