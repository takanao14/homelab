package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildMonitoringOverview covers Prometheus and Loki self-monitoring.
// Discover jobs from build_info so release names remain flexible.
func buildMonitoringOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		promJob = `job="$prometheus_job"`
		lokiJob = `job="$loki_job"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	issueThresholds := issueThresholds()
	firingAlertThresholds := watchdogAwareFiringAlertThresholds()

	d, err := dashboard.NewDashboardBuilder("Monitoring Overview").
		Uid("monitoring-overview").
		Tags([]string{"monitoring", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("prometheus_job").
				Label("Prometheus Job").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(prometheus_build_info, job)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("loki_job").
				Label("Loki Job").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(loki_build_info, job)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc),
		).
		WithRow(dashboard.NewRowBuilder("Prometheus Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Scrape Targets Up").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(up == 1)`).
					LegendFormat("Up"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Scrape Targets Down").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(up == 0) or vector(0)`).
					LegendFormat("Down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Dropped Notifications (5m)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(prometheus_notifications_dropped_total{` + promJob + `}[5m]))) or vector(0)`).
					LegendFormat("Dropped"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("WAL Corruptions").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(prometheus_tsdb_wal_corruptions_total{` + promJob + `}) or vector(0)`).
					LegendFormat("Corruptions"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Config Reload Failures").
				Description("kube-prometheus-stack config-reloader sidecars (Prometheus/Alertmanager) that failed to reload their config on the last change.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(reloader_last_reload_successful == 0) or vector(0)`).
					LegendFormat("Failed"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Alerting")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Firing Alerts").
				Description("Current Prometheus alerts in the firing state. Watchdog-only firing is expected and stays green.").
				Datasource(ds).
				Span(4).Height(8).
				Unit("short").
				Min(0).
				Thresholds(firingAlertThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(ALERTS{alertstate="firing"}) or vector(0)`).
					LegendFormat("Firing"),
				),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("Firing Alert Details").
				Description("Current firing alerts and their Prometheus labels.").
				Datasource(ds).
				Span(20).Height(8).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ALERTS{alertstate="firing"}`).
					Instant().
					Format(prometheus.PromQueryFormatTable),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Alertmanager Notification Failures").
				Description("Failed Alertmanager notification requests grouped by integration and reason.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (integration, reason) (rate(alertmanager_notifications_failed_total[$__rate_interval])) > 0 or on() vector(0)`).
					LegendFormat("{{integration}} {{reason}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Alertmanager Notification Latency p99").
				Description("99th percentile Alertmanager notification latency grouped by integration.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(histogram_quantile(0.99, sum by (le, integration) (rate(alertmanager_notification_latency_seconds_bucket[$__rate_interval]))) and on (integration) sum by (integration) (rate(alertmanager_notifications_total[$__rate_interval])) > 0) or on() vector(0)`).
					LegendFormat("{{integration}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Loki Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Loki Active Streams").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(loki_ingester_memory_streams{` + lokiJob + `})`).
					LegendFormat("Streams"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Loki Chunk Utilization").
				Datasource(ds).
				Span(12).Height(4).
				Unit("percentunit").
				Min(0).
				Max(1).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(loki_ingester_chunk_utilization_sum{` + lokiJob + `}) / sum(loki_ingester_chunk_utilization_count{` + lokiJob + `})`).
					LegendFormat("Utilization"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Prometheus Performance & Storage")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Sample Ingestion Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Select float samples; native histograms need a separately labelled target.
					// Aggregate away instance: the pod IP changes on every reschedule, so a
					// long range would otherwise draw one broken series per past address.
					Expr(`sum by (cluster) (rate(prometheus_tsdb_head_samples_appended_total{` + promJob + `, type="float"}[$__rate_interval]))`).
					LegendFormat("{{cluster}} samples/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("TSDB Size").
				Description("Bytes on disk: compacted blocks, plus the head chunks not yet compacted into them.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// max, not sum: a single replica per cluster, so this only drops the
					// churning instance label without doubling the value during a handover.
					Expr(`max by (cluster) (prometheus_tsdb_storage_blocks_bytes{` + promJob + `})`).
					LegendFormat("{{cluster}} Blocks"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Use measured head storage bytes, not an assumed size per chunk.
					Expr(`max by (cluster) (prometheus_tsdb_head_chunks_storage_size_bytes{` + promJob + `})`).
					LegendFormat("{{cluster}} Head"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Active Series").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max by (cluster) (prometheus_tsdb_head_series{` + promJob + `})`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Query Duration p99").
				Description("A slice is drawn only while queries are passing through it; a gap means no traffic in that phase, not a slow one.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Guard quiet histogram slices so NaN quantiles do not appear broken.
					// Keep cluster in the grouping so the legend prefix resolves.
					Expr(`histogram_quantile(0.99, sum by (le, slice, cluster) (rate(prometheus_engine_query_duration_histogram_seconds_bucket{` + promJob + `}[$__rate_interval])))` +
						` and on (slice, cluster) sum by (slice, cluster) (rate(prometheus_engine_query_duration_histogram_seconds_count{` + promJob + `}[$__rate_interval])) > 0`).
					LegendFormat("{{cluster}} {{slice}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Slowest Scrape Targets").
				Datasource(ds).
				Span(24).Height(10).
				Unit("s").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(topk(10, avg by (job) (scrape_duration_seconds)))`).
					Instant().
					LegendFormat("{{job}}"),
				).
				Decimals(3),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Scrape Errors").
				Datasource(ds).
				Span(24).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_exceeded_sample_limit_total{` + promJob + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} sample limit exceeded"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_sample_duplicate_timestamp_total{` + promJob + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} duplicate timestamp"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_sample_out_of_order_total{` + promJob + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} out of order"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_sample_out_of_bounds_total{` + promJob + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} out of bounds"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Loki Ingestion & Requests")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Ingestion Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(loki_distributor_bytes_received_total{` + lokiJob + `}[$__rate_interval]))`).
					LegendFormat("bytes/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Chunk Stored Rate (compressed)").
				// Ingestion rate above is what Loki receives; this is what reaches disk.
				Description("Bytes written to storage after compression -- this, not the ingestion rate above, drives disk growth. Bursty over short intervals as chunks flush.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(loki_ingester_chunk_stored_bytes_total{` + lokiJob + `}[$__rate_interval]))`).
					LegendFormat("bytes/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Lines Ingested Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(loki_distributor_lines_received_total{` + lokiJob + `}[$__rate_interval]))`).
					LegendFormat("lines/s"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Loki Request Duration p99").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (le, route) (rate(loki_request_duration_seconds_bucket{` + lokiJob + `, route!~"ready|/grpc\\..*|/frontendv2pb\\..*|/logproto\\..*"}[$__rate_interval]))) and on (route) sum by (route) (rate(loki_request_duration_seconds_count{` + lokiJob + `, route!~"ready|/grpc\\..*|/frontendv2pb\\..*|/logproto\\..*"}[$__rate_interval])) > 0`).
					LegendFormat("{{route}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Loki Active Streams over Time").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
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
