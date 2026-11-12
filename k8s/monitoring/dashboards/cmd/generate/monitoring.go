package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	monitoringPanelScrapeTargetsUp                    = 1
	monitoringPanelScrapeTargetsDown                  = 2
	monitoringPanelDroppedNotifications5m             = 3
	monitoringPanelWALCorruptions                     = 4
	monitoringPanelConfigReloadFailures               = 5
	monitoringPanelFiringAlerts                       = 6
	monitoringPanelFiringAlertDetails                 = 7
	monitoringPanelAlertmanagerNotificationFailures   = 8
	monitoringPanelAlertmanagerNotificationLatencyP99 = 9
	monitoringPanelLokiActiveStreams                  = 10
	monitoringPanelLokiChunkUtilization               = 11
	monitoringPanelSampleIngestionRate                = 12
	monitoringPanelTSDBSize                           = 13
	monitoringPanelActiveSeries                       = 14
	monitoringPanelQueryDurationP99                   = 15
	monitoringPanelSlowestScrapeTargets               = 16
	monitoringPanelScrapeErrors                       = 17
	monitoringPanelLogIngestionRate                   = 18
	monitoringPanelChunkStoredRateCompressed          = 19
	monitoringPanelLogLinesIngestedRate               = 20
	monitoringPanelLokiRequestDurationP99             = 21
	monitoringPanelLokiActiveStreamsOverTime          = 22
)

// buildMonitoringOverview covers Prometheus and Loki self-monitoring.
// Discover jobs from build_info so release names remain flexible.
func buildMonitoringOverview() (*dashboardv2.Dashboard, error) {
	const (
		promJob = `job="$prometheus_job"`
		lokiJob = `job="$loki_job"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	issueThresholds := issueThresholds()
	firingAlertThresholds := watchdogAwareFiringAlertThresholds()

	d := dashboardv2.NewDashboardBuilder("Monitoring Overview").
		Tags([]string{"monitoring", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-1d").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("prometheus_job").
			Label("Prometheus Job").
			Query(promVariableQuery(`label_values(prometheus_build_info, job)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc)).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("loki_job").
			Label("Loki Job").
			Query(promVariableQuery(`label_values(loki_build_info, job)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc))

	return newLayout(d).
		Row("Prometheus Summary", statGrid(4)).
		Panel(monitoringPanelScrapeTargetsUp, dashboardv2.NewPanelBuilder().
			Title("Scrape Targets Up").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(up == 1)`).
				LegendFormat("Up")))).
		Panel(monitoringPanelScrapeTargetsDown, dashboardv2.NewPanelBuilder().
			Title("Scrape Targets Down").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(up == 0) or vector(0)`).
				LegendFormat("Down")))).
		Panel(monitoringPanelDroppedNotifications5m, dashboardv2.NewPanelBuilder().
			Title("Dropped Notifications (5m)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(prometheus_notifications_dropped_total{`+promJob+`}[5m]))) or vector(0)`).
				LegendFormat("Dropped")))).
		Panel(monitoringPanelWALCorruptions, dashboardv2.NewPanelBuilder().
			Title("WAL Corruptions").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(prometheus_tsdb_wal_corruptions_total{`+promJob+`}) or vector(0)`).
				LegendFormat("Corruptions")))).
		Panel(monitoringPanelConfigReloadFailures, dashboardv2.NewPanelBuilder().
			Title("Config Reload Failures").
			Description("kube-prometheus-stack config-reloader sidecars (Prometheus/Alertmanager) that failed to reload their config on the last change.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(reloader_last_reload_successful == 0) or vector(0)`).
				LegendFormat("Failed")))).
		Row("Alerting", statGrid(1)).
		Panel(monitoringPanelFiringAlerts, dashboardv2.NewPanelBuilder().
			Title("Firing Alerts").
			Description("Current Prometheus alerts in the firing state. Watchdog-only firing is expected and stays green.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(firingAlertThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(ALERTS{alertstate="firing"}) or vector(0)`).
				LegendFormat("Firing")))).
		Row("Firing Alert Details", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(monitoringPanelFiringAlertDetails, dashboardv2.NewPanelBuilder().
			Title("Firing Alert Details").
			Description("Current firing alerts and their Prometheus labels.").
			Visualization(table.NewVisualizationV2Builder()).
			Data(queries(promQuery().
				Expr(`ALERTS{alertstate="firing"}`).
				Instant(true).Range(false).
				Format(prometheus.PromQueryFormatTable)))).
		Row("Alertmanager Notification Failures", chartGrid()).
		Panel(monitoringPanelAlertmanagerNotificationFailures, dashboardv2.NewPanelBuilder().
			Title("Alertmanager Notification Failures").
			Description("Failed Alertmanager notification requests grouped by integration and reason.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (integration, reason) (rate(alertmanager_notifications_failed_total[$__rate_interval])) > 0 or on() vector(0)`).
				LegendFormat("{{integration}} {{reason}}")))).
		Panel(monitoringPanelAlertmanagerNotificationLatencyP99, dashboardv2.NewPanelBuilder().
			Title("Alertmanager Notification Latency p99").
			Description("99th percentile Alertmanager notification latency grouped by integration.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`(histogram_quantile(0.99, sum by (le, integration) (rate(alertmanager_notification_latency_seconds_bucket[$__rate_interval]))) and on (integration) sum by (integration) (rate(alertmanager_notifications_total[$__rate_interval])) > 0) or on() vector(0)`).
				LegendFormat("{{integration}}")))).
		Row("Loki Summary", statGrid(2)).
		Panel(monitoringPanelLokiActiveStreams, dashboardv2.NewPanelBuilder().
			Title("Loki Active Streams").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(loki_ingester_memory_streams{`+lokiJob+`})`).
				LegendFormat("Streams")))).
		Panel(monitoringPanelLokiChunkUtilization, dashboardv2.NewPanelBuilder().
			Title("Loki Chunk Utilization").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Max(1).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(loki_ingester_chunk_utilization_sum{`+lokiJob+`}) / sum(loki_ingester_chunk_utilization_count{`+lokiJob+`})`).
				LegendFormat("Utilization")))).
		Row("Prometheus Performance & Storage", chartGrid()).
		Panel(monitoringPanelSampleIngestionRate, dashboardv2.NewPanelBuilder().
			Title("Sample Ingestion Rate").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Select float samples; native histograms need a separately labelled target.
				// Aggregate away instance: the pod IP changes on every reschedule, so a
				// long range would otherwise draw one broken series per past address.
				Expr(`sum by (cluster) (rate(prometheus_tsdb_head_samples_appended_total{`+promJob+`, type="float"}[$__rate_interval]))`).
				LegendFormat("{{cluster}} samples/s")))).
		Panel(monitoringPanelTSDBSize, dashboardv2.NewPanelBuilder().
			Title("TSDB Size").
			Description("Bytes on disk: compacted blocks, plus the head chunks not yet compacted into them.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// max, not sum: a single replica per cluster, so this only drops the
				// churning instance label without doubling the value during a handover.
				Expr(`max by (cluster) (prometheus_tsdb_storage_blocks_bytes{`+promJob+`})`).
				LegendFormat("{{cluster}} Blocks"),
				promQuery().
					// Use measured head storage bytes, not an assumed size per chunk.
					Expr(`max by (cluster) (prometheus_tsdb_head_chunks_storage_size_bytes{`+promJob+`})`).
					LegendFormat("{{cluster}} Head")))).
		Row("Active Series", chartGrid()).
		Panel(monitoringPanelActiveSeries, dashboardv2.NewPanelBuilder().
			Title("Active Series").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`max by (cluster) (prometheus_tsdb_head_series{`+promJob+`})`).
				LegendFormat("{{cluster}}")))).
		Panel(monitoringPanelQueryDurationP99, dashboardv2.NewPanelBuilder().
			Title("Query Duration p99").
			Description("A slice is drawn only while queries are passing through it; a gap means no traffic in that phase, not a slow one.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Guard quiet histogram slices so NaN quantiles do not appear broken.
				// Keep cluster in the grouping so the legend prefix resolves.
				Expr(`histogram_quantile(0.99, sum by (le, slice, cluster) (rate(prometheus_engine_query_duration_histogram_seconds_bucket{`+promJob+`}[$__rate_interval])))`+
					` and on (slice, cluster) sum by (slice, cluster) (rate(prometheus_engine_query_duration_histogram_seconds_count{`+promJob+`}[$__rate_interval])) > 0`).
				LegendFormat("{{cluster}} {{slice}}")))).
		Row("Slowest Scrape Targets", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(monitoringPanelSlowestScrapeTargets, dashboardv2.NewPanelBuilder().
			Title("Slowest Scrape Targets").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Decimals(3)).
			Data(queries(promQuery().
				Expr(`sort_desc(topk(10, avg by (job) (scrape_duration_seconds)))`).
				Instant(true).Range(false).
				LegendFormat("{{job}}")))).
		Row("Scrape Errors", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(monitoringPanelScrapeErrors, dashboardv2.NewPanelBuilder().
			Title("Scrape Errors").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (rate(prometheus_target_scrapes_exceeded_sample_limit_total{`+promJob+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} sample limit exceeded"),
				promQuery().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_sample_duplicate_timestamp_total{`+promJob+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} duplicate timestamp"),
				promQuery().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_sample_out_of_order_total{`+promJob+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} out of order"),
				promQuery().
					Expr(`sum by (cluster) (rate(prometheus_target_scrapes_sample_out_of_bounds_total{`+promJob+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} out of bounds")))).
		Row("Loki Ingestion & Requests", chartGrid()).
		Panel(monitoringPanelLogIngestionRate, dashboardv2.NewPanelBuilder().
			Title("Log Ingestion Rate").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(rate(loki_distributor_bytes_received_total{`+lokiJob+`}[$__rate_interval]))`).
				LegendFormat("bytes/s")))).
		Panel(monitoringPanelChunkStoredRateCompressed, dashboardv2.NewPanelBuilder().
			Title("Chunk Stored Rate (compressed)").
			// Ingestion rate above is what Loki receives; this is what reaches disk.
			Description("Bytes written to storage after compression -- this, not the ingestion rate above, drives disk growth. Bursty over short intervals as chunks flush.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(rate(loki_ingester_chunk_stored_bytes_total{`+lokiJob+`}[$__rate_interval]))`).
				LegendFormat("bytes/s")))).
		Row("Log Lines Ingested Rate", chartGrid()).
		Panel(monitoringPanelLogLinesIngestedRate, dashboardv2.NewPanelBuilder().
			Title("Log Lines Ingested Rate").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(rate(loki_distributor_lines_received_total{`+lokiJob+`}[$__rate_interval]))`).
				LegendFormat("lines/s")))).
		Panel(monitoringPanelLokiRequestDurationP99, dashboardv2.NewPanelBuilder().
			Title("Loki Request Duration p99").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (le, route) (rate(loki_request_duration_seconds_bucket{`+lokiJob+`, route!~"ready|/grpc\\..*|/frontendv2pb\\..*|/logproto\\..*"}[$__rate_interval]))) and on (route) sum by (route) (rate(loki_request_duration_seconds_count{`+lokiJob+`, route!~"ready|/grpc\\..*|/frontendv2pb\\..*|/logproto\\..*"}[$__rate_interval])) > 0`).
				LegendFormat("{{route}}")))).
		Row("Loki Active Streams over Time", chartGrid()).
		Panel(monitoringPanelLokiActiveStreamsOverTime, dashboardv2.NewPanelBuilder().
			Title("Loki Active Streams over Time").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(loki_ingester_memory_streams{`+lokiJob+`})`).
				LegendFormat("Streams")))).
		Build()
}
