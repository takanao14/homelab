package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildMonitoringOverview defines the self-monitoring dashboard for Prometheus and Loki.
// Job variables are discovered from build_info metrics so they adapt to any release name.
func buildMonitoringOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		promJob = `job=~"$prometheus_job"`
		lokiJob = `job=~"$loki_job"`
	)

	issueThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("Monitoring Overview").
		Uid("monitoring-overview").
		Tags([]string{"monitoring", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("prometheus_job").
				Label("Prometheus Job").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(prometheus_build_info, job)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("loki_job").
				Label("Loki Job").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(loki_build_info, job)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc),
		).

		// Row 1: Health stats
		WithPanel(
			stat.NewPanelBuilder().
				Title("Scrape Targets Up").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(up == 1)`).
					LegendFormat("Up"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Scrape Targets Down").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(up == 0) or vector(0)`).
					LegendFormat("Down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Dropped Notifications (5m)").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(prometheus_notifications_dropped_total{` + promJob + `}[5m])))`).
					LegendFormat("Dropped"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("WAL Corruptions").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(prometheus_tsdb_wal_corruptions_total{` + promJob + `}) or vector(0)`).
					LegendFormat("Corruptions"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Loki Active Streams").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					// Use job="loki" only to avoid double-counting from loki-headless.
					Expr(`sum(loki_ingester_memory_streams{job="loki"})`).
					LegendFormat("Streams"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Loki Chunk Utilization").
				Datasource(ds).
				Span(4).Height(4).
				Unit("percentunit").
				WithTarget(prometheus.NewDataqueryBuilder().
					// chunk_utilization is a histogram; derive average from sum/count.
					Expr(`sum(loki_ingester_chunk_utilization_sum{` + lokiJob + `}) / sum(loki_ingester_chunk_utilization_count{` + lokiJob + `})`).
					LegendFormat("Utilization"),
				),
		).

		// Row 2: Prometheus metrics
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Sample Ingestion Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(prometheus_tsdb_head_samples_appended_total{` + promJob + `}[5m])`).
					LegendFormat("samples/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("TSDB Size").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`prometheus_tsdb_storage_blocks_bytes{` + promJob + `}`).
					LegendFormat("Blocks"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`prometheus_tsdb_head_chunks{` + promJob + `} * 1024`).
					LegendFormat("Head (est.)"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Query Duration p99").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (le, slice) (rate(prometheus_engine_query_duration_histogram_seconds_bucket{` + promJob + `}[5m])))`).
					LegendFormat("{{slice}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Slowest Scrape Targets").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(topk(10, avg by (job) (scrape_duration_seconds)))`).
					LegendFormat("{{job}}"),
				).
				Decimals(3),
		).

		// Row 3: Loki metrics
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Ingestion Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(loki_distributor_bytes_received_total{` + lokiJob + `}[5m]))`).
					LegendFormat("bytes/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Lines Ingested Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(loki_distributor_lines_received_total{` + lokiJob + `}[5m]))`).
					LegendFormat("lines/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Loki Request Duration p99").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (le, route) (rate(loki_request_duration_seconds_bucket{` + lokiJob + `}[5m])))`).
					LegendFormat("{{route}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Loki Active Streams").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(loki_ingester_memory_streams{` + lokiJob + `})`).
					LegendFormat("Streams"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
