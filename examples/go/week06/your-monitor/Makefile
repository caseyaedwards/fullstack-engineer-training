# region: makefile
# make <target> runs the commands under that target. .PHONY lists the
# targets that name commands, not files that make builds.
.PHONY: build test run run-api up down logs web

# Arguments for make run. Override them: make run ARGS="-file urls.txt -workers 20"
ARGS ?= -file urls.txt -interval 30s

# Build every program under cmd/ into bin/, with the flags the Dockerfile uses.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ ./cmd/...

# Test the Go code, then the web code.
test: web/node_modules
	go vet ./...
	go test -race ./...
	npm --prefix web test

# Run the worker or the api on your machine, without Docker.
run:
	go run ./cmd/worker $(ARGS)

run-api:
	go run ./cmd/api -addr 127.0.0.1:8080

# Build the images and start postgres, api and worker in the background.
up:
	docker compose up --build --detach

# Stop and remove the containers. The pgdata volume, and your data, stay.
down:
	docker compose down

logs:
	docker compose logs --follow

# Start the Vite dev server for web/.
web: web/node_modules
	npm --prefix web run dev

# Install the web packages when web/node_modules is missing or older than
# web/package-lock.json.
web/node_modules: web/package-lock.json
	npm --prefix web ci
# endregion: makefile
