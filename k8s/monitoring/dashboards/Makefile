CHART_DASHBOARDS_DIR := ../charts/dashboards/dashboards

# Compose implementation for the dev targets. Real Docker wins when present so
# existing setups are unchanged; Podman hosts fall back to podman-compose.
#
# `docker` is not proof of Docker: the podman-docker package installs a shim at
# /usr/bin/docker that forwards to podman, which in turn delegates compose to
# whatever external provider it finds -- often the obsolete docker-compose v1,
# which then fails against a Docker socket that does not exist. `docker compose
# version` succeeds in that setup, so the shim has to be ruled out by name.
#
# `podman compose` is deliberately not probed for the same reason: what it runs
# depends on which provider happens to be installed. Set COMPOSE explicitly to
# use it.
#
# COMPOSE is recursively expanded, so the probe only runs for dev/dev-stop and
# never slows down generate/check.
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
