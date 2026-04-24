Add missing Swagger/OpenAPI annotations to every HTTP handler in this Go repository that doesn't have one yet, then regenerate the swagger docs.

## 1. Discover the repo layout

Run these commands to understand the project before touching any file:

```bash
# Find router/route registration files
grep -rl "HandleFunc\|\.GET\|\.POST\|\.PUT\|\.PATCH\|\.DELETE" --include="*.go" . | grep -v vendor | grep -v _test

# Find handler function files
grep -rl "http\.ResponseWriter.*\*http\.Request" --include="*.go" . | grep -v vendor | grep -v _test

# Check existing annotation style (if any)
grep -rn "@Summary\|@Router\|@Tags" --include="*.go" . | grep -v vendor | head -20

# Read go.mod to get the module path
head -3 go.mod
```

Identify: the router file(s), handler directories, HTTP framework in use (gorilla/mux, chi, gin, echo, fiber, or net/http), and any existing annotation patterns to match.

## 2. Extract every registered route

Read the router file(s) and collect each active route — skip commented-out lines. For each route record:

- **Path** — e.g. `/api/v1/users/{id}`
- **Method** — GET / POST / PUT / PATCH / DELETE
- **Handler function name** — the last identifier, e.g. `GetUserByID` from `r.Users.GetUserByID`
- **Auth middleware** — any wrapper like `auth.VerifyJWT(...)`, `middleware.Auth(...)`, etc.
- **Path params** — `{id}` or `:id` segments

Framework patterns:
| Framework | Route syntax |
|---|---|
| gorilla/mux | `router.HandleFunc("/path", handler).Methods("GET")` |
| chi | `r.Get("/path", handler)` |
| gin | `r.GET("/path", handler)` |
| echo | `e.GET("/path", handler)` |
| net/http | `mux.HandleFunc("/path", handler)` |

## 3. Check each handler for existing annotations

For every handler function from Step 2, find its source file:

```bash
grep -rn "func.*\bHandlerFuncName\b" --include="*.go" . | grep -v vendor | grep -v _test
```

Read the 20 lines above the `func` declaration. If `// @Summary` is already present — **skip it**. Only annotate handlers with no `@Summary`.

## 4. Write the annotation block

Add this godoc block immediately above the `func` line for each unannotated handler:

```go
// FuncName godoc
// @Summary     <short verb phrase>
// @Description <one sentence>
// @Tags        <tag>
// @Accept      json
// @Produce     json
// @Param       <name>  <in>  <type>  <required>  "<description>"
// @Success     200  {object}  <ResponseType>
// @Failure     400  {object}  <ErrorType>
// @Failure     401  {object}  <ErrorType>
// @Failure     500  {object}  <ErrorType>
// @Security    <SchemeName>
// @Router      /api/path [method]
```

**How to fill each field:**

| Field | Rule |
|---|---|
| `@Tags` | Derive from the URL segment or package name — e.g. `/api/v1/cards` → `cards` |
| `@Param` (path) | `mux.Vars(r)["id"]` / `c.Param("id")` → `@Param id path string true "..."` |
| `@Param` (query) | `r.URL.Query().Get("q")` → `@Param q query string false "..."` |
| `@Param` (body) | `json.Decode(&req)` / `c.ShouldBindJSON(&req)` → `@Param body body pkg.ReqType true "..."` using the actual struct |
| `@Success` | Use the actual response struct; slice → `[]Type`; message only → `map[string]string` |
| `@Failure` | Use the project's error response struct if one exists, else `map[string]string` |
| `@Security` | JWT middleware → `BearerAuth`; API-key middleware → `ApiKeyAuth`; no auth → omit the line |
| `@Router` | Exact path from the router + lowercase method: `[get]` `[post]` `[put]` `[patch]` `[delete]` |

## 5. Bootstrap swag if not already set up

```bash
grep "@title" main.go cmd/main.go 2>/dev/null
ls docs/docs.go 2>/dev/null
```

If swag is **not set up**, do this before regenerating:

**a) Global annotation** — add above `package main` in the entry point file:
```go
// @title           <Service Name> API
// @version         1.0
// @description     <one-line description>
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
```

**b) Docs import** — add to the import block (use actual module path from go.mod):
```go
_ "<module>/docs"
```

**c) Swagger UI route** — add to the router:
```go
import httpSwagger "github.com/swaggo/http-swagger"
router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
```

**d) Install dependencies:**
```bash
go get github.com/swaggo/swag
go get github.com/swaggo/http-swagger
go mod tidy
```

## 6. Regenerate and verify

```bash
which swag || go install github.com/swaggo/swag/cmd/swag@latest
swag init -g main.go --output docs --parseDependency --parseInternal
go build ./...
```

Fix any swag parse errors (common cause: wrong struct type name or missing import for a response type). Do not stop until `go build ./...` passes.

## 7. Report

When done, print a table:

| Status | Handler | File |
|---|---|---|
| ✓ annotated | FuncName | path/to/file.go |
| ↷ skipped | FuncName | path/to/file.go (already had @Summary) |
| ✗ not found | FuncName | — (registered in router but source not located) |

Also note if swag bootstrapping was performed.
