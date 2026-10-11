# region: makefile
# make <target> runs the commands under that target. .PHONY lists the
# targets that name commands, not files that make builds.
.PHONY: build test lint ci run run-api run-scheduler run-alerter up down logs web e2e check-obs

# Arguments for make run. Override them: make run ARGS="-workers 20"
ARGS ?= -workers 10 -debug-addr 127.0.0.1:6060

# Build every program under cmd/ into bin/, with the flags the Dockerfile uses.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ ./cmd/...

# Test the Go code, then the web code.
test: web/node_modules
	go vet ./...
	go test -race ./...
	npm --prefix web test

# region: lint
# Run the linters that CI runs, with the settings in .golangci.yml.
lint:
	golangci-lint run

# Run every check that .github/workflows/ci.yml runs, in one go.
ci: lint test e2e
	npm --prefix web run build
# endregion: lint

# Run one program on your machine, without Docker. Each reads DATABASE_URL
# from your shell, and sends spans to OTEL_EXPORTER_OTLP_ENDPOINT when you
# set it: http://127.0.0.1:4318 reaches the collector in Compose.
run:
	go run ./cmd/worker $(ARGS)

run-api:
	go run ./cmd/api -addr 127.0.0.1:8080 -metrics-addr 127.0.0.1:2112

run-scheduler:
	go run ./cmd/scheduler -metrics-addr 127.0.0.1:2113

run-alerter:
	go run ./cmd/alerter

# Build the images and start every service in the background.
up:
	docker compose up --build --detach

# Stop and remove the containers. The pgdata volume, and your data, stay.
down:
	docker compose down

logs:
	docker compose logs --follow

# region: check-obs
# Check the Prometheus and collector configuration with their own tools,
# from the same images Compose runs.
check-obs:
	docker compose run --rm --no-deps --entrypoint promtool prometheus check config /etc/prometheus/prometheus.yml
	docker compose run --rm --no-deps otel-collector validate --config=/etc/otelcol/config.yaml
# endregion: check-obs

# Start the Vite dev server for web/.
web: web/node_modules
	npm --prefix web run dev

# contract stub: week 10. Run the Playwright tests of web/.
e2e: web/node_modules
	npm --prefix web run test:e2e

# Install the web packages when web/node_modules is missing or older than
# web/package-lock.json.
web/node_modules: web/package-lock.json
	npm --prefix web ci
# endregion: makefile
