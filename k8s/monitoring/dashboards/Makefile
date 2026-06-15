CHART_DASHBOARDS_DIR := ../charts/prometheus/dashboards

.PHONY: generate
generate:
	go run ./cmd/generate/ generated/

.PHONY: build
build:
	go build ./...

# Verify the committed chart JSON is in sync with the Go sources.
# Regenerates into a temp dir and diffs each generated file against the chart;
# fails on drift so a forgotten `make deploy` is caught in CI / pre-commit.
# Only generated dashboards are checked — hand-maintained JSON in the chart
# (e.g. proxmox-overview.json) is intentionally ignored.
.PHONY: check
check:
	@tmp=$$(mktemp -d); \
	go run ./cmd/generate/ "$$tmp" >/dev/null; \
	rc=0; \
	for f in "$$tmp"/*.json; do \
		name=$$(basename "$$f"); \
		if ! diff -u "$(CHART_DASHBOARDS_DIR)/$$name" "$$f" >/dev/null 2>&1; then \
			echo "ERROR: $$name differs from $(CHART_DASHBOARDS_DIR). Run 'make deploy'."; \
			diff -u "$(CHART_DASHBOARDS_DIR)/$$name" "$$f" || true; \
			rc=1; \
		fi; \
	done; \
	rm -rf "$$tmp"; \
	if [ $$rc -eq 0 ]; then echo "OK: generated dashboards are in sync."; fi; \
	exit $$rc

.PHONY: dev
dev: generate
	docker compose up -d
	@echo "Grafana: http://localhost:3000"

.PHONY: dev-stop
dev-stop:
	docker compose down

.PHONY: deploy
deploy: generate
	mkdir -p $(CHART_DASHBOARDS_DIR)
	cp generated/*.json $(CHART_DASHBOARDS_DIR)/
