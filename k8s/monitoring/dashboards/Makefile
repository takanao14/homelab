CHART_DASHBOARDS_DIR := ../charts/dashboards/dashboards

# Compose implementation for the dev targets. Docker wins when both are present
# so existing setups are unchanged; Podman hosts fall back to podman-compose.
# COMPOSE is recursively expanded, so the probe only runs for dev/dev-stop and
# never slows down generate/check.
# Override with: make dev COMPOSE=podman-compose
COMPOSE ?= $(shell \
	if docker compose version >/dev/null 2>&1; then echo docker compose; \
	elif command -v podman-compose >/dev/null 2>&1; then echo podman-compose; \
	elif podman compose version >/dev/null 2>&1; then echo podman compose; \
	fi)

.PHONY: generate
generate:
	go run ./cmd/generate/ $(CHART_DASHBOARDS_DIR)/

.PHONY: build
build:
	go build ./...

.PHONY: check
check:
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	go run ./cmd/generate/ "$$tmp" >/dev/null; \
	diff -ruN "$(CHART_DASHBOARDS_DIR)" "$$tmp"

.PHONY: compose-check
compose-check:
	@test -n "$(COMPOSE)" || { \
		echo "No compose implementation found."; \
		echo "Install Docker Compose, or run ./setup-linux.sh to set up podman-compose."; \
		exit 1; \
	} >&2

.PHONY: dev
dev: compose-check generate
	$(COMPOSE) up -d
	@echo "Grafana: http://localhost:3000"

.PHONY: dev-stop
dev-stop: compose-check
	$(COMPOSE) down
