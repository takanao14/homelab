CHART_DASHBOARDS_DIR := ../charts/dashboards/dashboards

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

.PHONY: dev
dev: generate
	docker compose up -d
	@echo "Grafana: http://localhost:3000"

.PHONY: dev-stop
dev-stop:
	docker compose down
