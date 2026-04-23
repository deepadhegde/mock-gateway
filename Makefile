SERVICE ?= cardhub
PORT    ?= 9003

.PHONY: run build tidy sync-spec reset-spec test docker-build docker-up docker-down docker-logs docker-restart

## Run locally — upstream defaults to localhost
run:
	go run ./cmd/main.go

## Run pointing at UAT backend
run-uat:
	CARDHUB_URL=https://cardhub-uat.popclub.co.in \
	go run ./cmd/main.go

## Run pointing at prod backend
run-prod:
	CARDHUB_URL=https://cardhub.popclub.co.in \
	go run ./cmd/main.go

## Build binary
build:
	go build -o bin/mock-gateway ./cmd/main.go

## Tidy dependencies
tidy:
	go mod tidy

## Sync swagger spec from a service repo and re-seed
## Usage: make sync-spec SPEC=/path/to/cardhub/docs/swagger.json SERVICE=cardhub
sync-spec:
	@echo "Copying spec for $(SERVICE)..."
	cp $(SPEC) specs/$(SERVICE)/swagger.json
	curl -s -X POST "http://localhost:$(PORT)/admin/routes/reset?service=$(SERVICE)" && echo "Re-seeded $(SERVICE)"

## Upload spec via HTTP (gateway must be running)
## Usage: make upload-spec SPEC=/path/to/swagger.json SERVICE=cardhub
upload-spec:
	curl -s -X POST "http://localhost:$(PORT)/admin/specs?service=$(SERVICE)" \
	  -H "Content-Type: application/json" \
	  --data-binary "@$(SPEC)" | jq .

## Reset all services from local spec files
reset-spec:
	curl -s -X POST "http://localhost:$(PORT)/admin/routes/reset" && echo "All services re-seeded"

## Activate a route for testing
## Usage: make activate SERVICE=cardhub METHOD=GET PATH=/api/v1/cards ENV=api
activate:
	curl -s -X PUT http://localhost:$(PORT)/admin/routes \
	  -H "Content-Type: application/json" \
	  -d '{"service":"$(SERVICE)","method":"$(METHOD)","path":"$(PATH)","env":"$(ENV)","active":true,"delay_ms":0}' \
	  && echo "Activated $(METHOD) $(PATH) [$(ENV)]"

## Show request log
logs:
	curl -s "http://localhost:$(PORT)/admin/logs?service=$(SERVICE)" | jq .

## Clear logs
clear-logs:
	curl -s -X DELETE "http://localhost:$(PORT)/admin/logs?service=$(SERVICE)" && echo "Logs cleared"

## Quick health check
health:
	curl -s http://localhost:$(PORT)/health | jq .

## Test mock response (cardhub health endpoint)
test-mock:
	curl -s \
	  -H "X-Mock-Service: cardhub" \
	  -H "X-Mock-Enabled: true" \
	  -H "X-Mock-Env: api" \
	  http://localhost:$(PORT)/api/v1/health | jq .

## Test passthrough (no mock headers)
test-passthrough:
	curl -v \
	  -H "X-Mock-Service: cardhub" \
	  http://localhost:$(PORT)/api/v1/health

## ── Docker ───────────────────────────────────────────────────────────────────

## Build the Docker image
docker-build:
	docker compose build

## Start in background
docker-up:
	docker compose up -d

## Stop and remove containers
docker-down:
	docker compose down

## Tail logs
docker-logs:
	docker compose logs -f mock-gateway

## Restart (pick up config.yaml changes without rebuild)
docker-restart:
	docker compose restart mock-gateway

## Rebuild image and restart
docker-rebuild:
	docker compose up -d --build

## Open a shell in the running container
docker-shell:
	docker compose exec mock-gateway sh
