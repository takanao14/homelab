CHART_DASHBOARDS_DIR := ../charts/prometheus/dashboards

.PHONY: generate
generate:
	go run ./cmd/generate/ generated/
	mkdir -p $(CHART_DASHBOARDS_DIR)
	cp generated/*.json $(CHART_DASHBOARDS_DIR)/

.PHONY: build
build:
	go build ./...
