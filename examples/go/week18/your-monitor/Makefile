# region: makefile
# make <target> runs the commands under that target. .PHONY lists the
# targets that name commands, not files that make builds.
.PHONY: build test lint ci run run-api run-scheduler run-alerter up down logs check-obs check-slo web e2e db psql backup restore-check kind-up kind-load k8s-check kind-deploy kind-down

# Arguments for make run. Override them: make run ARGS="-workers 20"
ARGS ?= -workers 10 -debug-addr 127.0.0.1:6060 -pprof-addr 127.0.0.1:6061

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

# region: lint
# Run the linters that CI runs, with the settings in .golangci.yml.
lint:
	golangci-lint run

# Run every check that .github/workflows/ci.yml runs, in one go.
ci: lint test e2e
	npm --prefix web run build
# endregion: lint

# Run one program on your machine, without Docker, against the Postgres
# in Docker. Each sends spans to OTEL_EXPORTER_OTLP_ENDPOINT when you set
# it: http://127.0.0.1:4318 reaches the collector in Compose.
run: db
	go run ./cmd/worker $(ARGS)

run-api: db
	go run ./cmd/api -addr 127.0.0.1:8080 -metrics-addr 127.0.0.1:2112

run-scheduler: db
	go run ./cmd/scheduler -metrics-addr 127.0.0.1:2113

run-alerter: db
	go run ./cmd/alerter -metrics-addr 127.0.0.1:2114

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

# region: check-slo
# Check the SLO rules, run their unit tests, and check the Alertmanager
# configuration, with the tools inside the images Compose runs.
check-slo:
	docker compose run --rm --no-deps --entrypoint promtool prometheus check rules /etc/prometheus/rules/slo.rules.yml
	docker compose run --rm --no-deps --entrypoint promtool prometheus test rules /etc/prometheus/rules/slo.test.yml
	docker compose run --rm --no-deps --entrypoint amtool alertmanager check-config /etc/alertmanager/alertmanager.yml
# endregion: check-slo

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
# region: kind
# The kind cluster: Kubernetes 1.36 in one Docker container, as
# deploy/k8s/kind-config.yaml describes it. Each kubectl command names the
# context of this cluster, so none of them reaches another cluster.
KIND_CLUSTER := your-monitor
KUBECTL := kubectl --context kind-$(KIND_CLUSTER) --namespace your-monitor
IMAGES := api worker scheduler alerter web
KIND_ENV := deploy/k8s/overlays/kind/postgres.env

# Create the cluster, and make your-monitor the default namespace of its
# kubectl context.
kind-up:
	kind create cluster --config deploy/k8s/kind-config.yaml --wait 2m
	kubectl config set-context kind-$(KIND_CLUSTER) --namespace your-monitor

# Build the five images as make up does, named your-monitor-<service>, tag
# each one dev, and copy it into the node: the node has an image store of
# its own and never sees the images of your Docker.
kind-load:
	docker compose build $(IMAGES)
	for t in $(IMAGES); do \
	  docker tag your-monitor-$$t your-monitor-$$t:dev && \
	  kind load docker-image your-monitor-$$t:dev --name $(KIND_CLUSTER) || exit 1; \
	done

# A random Postgres password, made once. umask 077: only you can read it.
$(KIND_ENV):
	umask 077; printf 'POSTGRES_PASSWORD=%s\n' "$$(openssl rand -hex 16)" > $@

# Render the overlay and check every object against the schemas of
# Kubernetes 1.36, without a cluster.
k8s-check: $(KIND_ENV)
	kubectl kustomize deploy/k8s/overlays/kind | kubeconform -strict -kubernetes-version 1.36.0 -summary

# Send every object to the cluster, then wait for Postgres and each program.
kind-deploy: $(KIND_ENV)
	$(KUBECTL) apply -k deploy/k8s/overlays/kind
	$(KUBECTL) rollout status statefulset/postgres --timeout=5m
	for d in $(IMAGES); do $(KUBECTL) rollout status deployment/$$d --timeout=5m || exit 1; done

# Delete the cluster with everything in it, the Postgres volume included.
kind-down:
	kind delete cluster --name $(KIND_CLUSTER)
# endregion: kind
# endregion: makefile
