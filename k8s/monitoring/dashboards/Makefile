CHART_DASHBOARDS_DIR := ../charts/dashboards/dashboards

# Prefer real Docker Compose, then podman-compose. Exclude the podman-docker shim
# and delegated `podman compose` providers; set COMPOSE to use one explicitly.
# Recursive expansion limits probing to dev/dev-stop.
# Override with: make dev COMPOSE=podman-compose
COMPOSE ?= $(shell \
	if command -v docker >/dev/null 2>&1 \
		&& ! docker --version 2>/dev/null | grep -qi podman \
		&& docker compose version >/dev/null 2>&1; then echo docker compose; \
	elif command -v podman-compose >/dev/null 2>&1; then echo podman-compose; \
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
		echo "No usable compose implementation found."; \
		echo "On Linux, run ./setup-linux.sh to install podman-compose."; \
		echo "Note: docker-compose v1 and the podman-docker shim are ignored on purpose."; \
		exit 1; \
	} >&2

.PHONY: dev
dev: compose-check generate
	$(COMPOSE) up -d
	@echo "Grafana: http://localhost:3000"

.PHONY: dev-stop
dev-stop: compose-check
	$(COMPOSE) down
