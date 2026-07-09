CHART_DASHBOARDS_DIR := ../charts/dashboards/dashboards

.PHONY: generate
generate:
	go run ./cmd/generate/ $(CHART_DASHBOARDS_DIR)/

.PHONY: build
build:
	go build ./...

.PHONY: dev
dev: generate
	docker compose up -d
	@echo "Grafana: http://localhost:3000"

.PHONY: dev-stop
dev-stop:
	docker compose down
