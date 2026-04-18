CHART_DASHBOARDS_DIR := ../charts/prometheus/dashboards

.PHONY: generate
generate:
	go run ./cmd/generate/ generated/

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

.PHONY: deploy
deploy: generate
	mkdir -p $(CHART_DASHBOARDS_DIR)
	cp generated/*.json $(CHART_DASHBOARDS_DIR)/
