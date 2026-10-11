# region: makefile
# make <target> runs the commands under that target. .PHONY lists the
# targets that name commands, not files that make builds.
.PHONY: build test run run-api run-scheduler run-alerter up down logs web e2e db psql backup restore-check

# Arguments for make run. Override them: make run ARGS="-workers 20"
ARGS ?= -workers 10

# region: database
# Read POSTGRES_PASSWORD and POSTGRES_PORT from .env. Every command below
# gets DATABASE_URL: the Postgres from compose.yaml, seen from your machine.
-include .env
POSTGRES_PORT ?= 5432
DATABASE_URL ?= postgres://monitor:$(POSTGRES_PASSWORD)@127.0.0.1:$(POSTGRES_PORT)/monitor?sslmode=disable
export DATABASE_URL
# make run-alerter posts to the webhook in .env.
export ALERT_WEBHOOK_URL
# endregion: database

# Build every program under cmd/ into bin/, with the flags the Dockerfile uses.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ ./cmd/...

# Test the Go code against Postgres, then the web code.
test: web/node_modules db
	go vet ./...
	go test -race ./...
	npm --prefix web test

# Run one program on your machine, without Docker, against the Postgres
# in Docker.
run: db
	go run ./cmd/worker $(ARGS)

run-api: db
	go run ./cmd/api -addr 127.0.0.1:8080

run-scheduler: db
	go run ./cmd/scheduler

run-alerter: db
	go run ./cmd/alerter

# Build the images and start every service in the background.
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

# region: e2e
# Week 10: run the Playwright tests of web/. Playwright starts the Vite
# dev server itself.
e2e: web/node_modules
	npm --prefix web run test:e2e
# endregion: e2e

# Install the web packages when web/node_modules is missing or older than
# web/package-lock.json.
web/node_modules: web/package-lock.json
	npm --prefix web ci

# region: dbtargets
# Start only postgres, and wait until its healthcheck passes.
db:
	docker compose up --detach --wait postgres

# Open a psql shell on the monitor database.
psql:
	docker compose exec postgres psql -U monitor -d monitor

# Dump the whole database into backups/, in the pg_dump custom format.
backup:
	mkdir -p backups
	docker compose exec -T postgres pg_dump -U monitor -d monitor --format=custom \
		> backups/monitor-$$(date +%Y%m%d-%H%M%S).dump
	ls -l backups

# Restore the newest backup into a new database, count its rows, drop it.
restore-check:
	docker compose exec -T postgres dropdb -U monitor --if-exists monitor_restore
	docker compose exec -T postgres createdb -U monitor monitor_restore
	docker compose exec -T postgres pg_restore -U monitor -d monitor_restore \
		--no-owner --exit-on-error < $$(ls -t backups/*.dump | head -n 1)
	docker compose exec -T postgres psql -U monitor -d monitor_restore -c \
		"select (select count(*) from monitors) as monitors, (select count(*) from check_results) as results, (select max(version) from schema_migrations) as version"
	docker compose exec -T postgres dropdb -U monitor monitor_restore
# endregion: dbtargets
# endregion: makefile
