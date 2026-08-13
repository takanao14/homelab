package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDnsOverview covers DNS infrastructure. Scrape configs relabel
// dnsdist and pdns-auth instances to hostnames.
func buildDnsOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()
	lokiType := "loki"
	lokiUID := "$loki_datasource"
	lokiDS := common.DataSourceRef{Type: &lokiType, Uid: &lokiUID}

	const (
		dnsdist                = `job="scrapeConfig/monitoring/dnsdist-external"`
		resolver               = `job="scrapeConfig/monitoring/node-exporter-external",service="knot-resolver"`
		pdns                   = `job="scrapeConfig/monitoring/pdns-auth-external"`
		coredns                = `job="coredns"`
		extdns                 = `job="external-dns"`
		resolverValidationLogs = `{job="dns-resolver", unit="knot-resolver.service"} | json | __error__="" |~ "(?i)(bogus|dnssec|validation)" | line_format "{{.message}}"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// zeroLine draws a solid reference line at y=0 for bidirectional rate panels.
	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// Latency thresholds in microseconds (50ms = warning, 150ms = critical).
	latencyThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green", Value: new(float64(0))},
			{Color: "yellow", Value: new(float64(50000))},
			{Color: "red", Value: new(float64(150000))},
		})

	corednsLatencyThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green", Value: new(float64(0))},
			{Color: "yellow", Value: new(0.05)},
			{Color: "red", Value: new(0.15)},
		})

	issueThresholds := issueThresholds()

	// Match resolver alert ratios: healthy >=90%, critical <60%.
	resolverCacheHitRateThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(60)), Color: "yellow"},
			{Value: new(float64(90)), Color: "green"},
		})

	servfailThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(0.001), Color: "red"},
		})

	// Warn after several missed one-minute syncs; alert after 15 minutes.
	syncAgeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(300)), Color: "yellow"},
			{Value: new(float64(900)), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("DNS Overview").
		Uid("dns-overview").
		Tags([]string{"dns", "infrastructure"}).
		Timezone("browser").
		// Six hours preserves QPS and latency spikes hidden by the old 30-day step.
		Time("now-6h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("loki_datasource").
				Label("Loki Datasource").
				Type("loki"),
		).
		WithRow(dashboard.NewRowBuilder("dnsdist Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist QPS").
				Datasource(ds).
				Span(12).Height(4).
				Unit("reqps").
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_queries{` + dnsdist + `}[$__rate_interval]))`).
					LegendFormat("QPS"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist Avg Latency").
				Datasource(ds).
				Span(12).Height(4).
				Unit("µs").
				Thresholds(latencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(dnsdist_latency_avg100{` + dnsdist + `})`).
					LegendFormat("Avg Latency"),
				).Decimals(1),
		).
		WithRow(dashboard.NewRowBuilder("dnsdist Traffic & Performance")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Query/Response Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Queries").
					Expr(`rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval])`).
					LegendFormat("{{instance}} Queries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Responses").
					Expr(`rate(dnsdist_responses{`+dnsdist+`}[$__rate_interval])`).
					LegendFormat("{{instance}} Responses"),
				).
				OverrideByQuery("Responses", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Response Codes").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_noerror{`+dnsdist+`}[$__rate_interval]))`).
					LegendFormat("NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_nxdomain{`+dnsdist+`}[$__rate_interval]))`).
					LegendFormat("NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_servfail{`+dnsdist+`}[$__rate_interval]))`).
					LegendFormat("SERVFAIL"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_refused{`+dnsdist+`}[$__rate_interval]))`).
					LegendFormat("REFUSED"),
				).
				// Semantic coloring: OK=Green, Warning=Yellow, Error=Red, Refused=Orange
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "NOERROR"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "NXDOMAIN"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "SERVFAIL"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "REFUSED"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Latency").
				Datasource(ds).
				Span(24).Height(8).
				Unit("µs").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(latencyThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dnsdist_latency_avg100{`+dnsdist+`}`).
					LegendFormat("{{instance}} avg100"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dnsdist_latency_avg1000{`+dnsdist+`}`).
					LegendFormat("{{instance}} avg1000"),
				).
				// Visual differentiation: emphasize avg100 (short-term) and de-emphasize avg1000 (long-term trend).
				WithOverride(dashboard.MatcherConfig{
					Id:      "byRegexp",
					Options: ".*avg1000.*",
				}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
					{Id: "drawStyle", Value: "line"},
					{Id: "fillOpacity", Value: 10},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Drop Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_acl_drops{` + dnsdist + `}[$__rate_interval])`).
					LegendFormat("{{instance}} ACL Drop"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_rule_drops{` + dnsdist + `}[$__rate_interval])`).
					LegendFormat("{{instance}} Rule Drop"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_dynamic_blocked{` + dnsdist + `}[$__rate_interval])`).
					LegendFormat("{{instance}} Dynamic Block"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Unanswered Queries").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_queries{` + dnsdist + `}[$__rate_interval]) - rate(dnsdist_responses{` + dnsdist + `}[$__rate_interval])`).
					LegendFormat("{{instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Knot Resolver Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Resolver QPS").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(resolver_request_total{` + resolver + `}[$__rate_interval]))`).
					Instant().
					LegendFormat("QPS"),
				).
				Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Resolver Cache Hit Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Thresholds(resolverCacheHitRateThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Preserve a gap for undefined 0/0 cache ratios; clamping would report a
					// false 0% failure during idle windows.
					Expr(`100 * sum(rate(resolver_answer_cached_total{` + resolver + `}[$__rate_interval])) / sum(rate(resolver_answer_total{` + resolver + `}[$__rate_interval]))`).
					Instant().
					LegendFormat("Hit rate"),
				).
				Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Resolver Latency p95").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				Thresholds(corednsLatencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.95, sum by (le) (rate(resolver_response_latency_bucket{` + resolver + `}[$__rate_interval])))`).
					Instant().
					LegendFormat("p95"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Resolver Metrics Age").
				Description("Worst age of the node_exporter textfile, which the collector rewrites every 15s. Healthy readings run up to about 46s because the age carries the Prometheus scrape interval on top of the write interval; red matches the KnotResolverMetricsStale alert.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				ColorMode(common.BigValueColorModeBackground).
				// Match resolverAlerts.metricsMaxAgeSeconds. Healthy write and scrape
				// intervals already consume most of the margin, so omit an amber band.
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(60)), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max(time() - node_textfile_mtime_seconds{` + resolver + `,file=~".*knot_resolver\\.prom"})`).
					Instant().
					LegendFormat("Age"),
				).
				Decimals(0),
		).
		WithRow(dashboard.NewRowBuilder("Knot Resolver Traffic & Performance")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Query Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(resolver_request_total{` + resolver + `}[$__rate_interval]))`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Cache Hit Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Preserve gaps for undefined cache ratios.
					Expr(`100 * sum by (instance) (rate(resolver_answer_cached_total{` + resolver + `}[$__rate_interval])) / sum by (instance) (rate(resolver_answer_total{` + resolver + `}[$__rate_interval]))`).
					LegendFormat("{{instance}}"),
				).
				Decimals(1),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Response Codes").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(resolver_answer_rcode_noerror_total{` + resolver + `}[$__rate_interval]))`).
					LegendFormat("NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(resolver_answer_rcode_nodata_total{` + resolver + `}[$__rate_interval]))`).
					LegendFormat("NODATA"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(resolver_answer_rcode_nxdomain_total{` + resolver + `}[$__rate_interval]))`).
					LegendFormat("NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(resolver_answer_rcode_servfail_total{` + resolver + `}[$__rate_interval]))`).
					LegendFormat("SERVFAIL"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Response Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(corednsLatencyThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.50, sum by (instance, le) (rate(resolver_response_latency_bucket{` + resolver + `}[$__rate_interval])))`).
					LegendFormat("{{instance}} p50"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.95, sum by (instance, le) (rate(resolver_response_latency_bucket{` + resolver + `}[$__rate_interval])))`).
					LegendFormat("{{instance}} p95"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (instance, le) (rate(resolver_response_latency_bucket{` + resolver + `}[$__rate_interval])))`).
					LegendFormat("{{instance}} p99"),
				),
		).
		// Place cache occupancy beside memory because both show resource exhaustion.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Cache Usage").
				Description("LMDB pages allocated against the configured map size. When the map fills, every stash fails and the cache is dropped, which collapses the hit rate and floods upstream with recursion until it refills -- see the 2026-08-10 addendum to ADR-0030. kresd publishes no cache metric, so this is read from the database's meta page by the textfile collector; data.mdb's own size is preallocated to the map size and cannot be used.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).Max(100).
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(70)), Color: "yellow"},
						{Value: new(float64(85)), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 * knot_resolver_cache_used_bytes{` + resolver + `} / knot_resolver_cache_size_bytes{` + resolver + `}`).
					LegendFormat("{{instance}}"),
				).Decimals(1),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Manager Memory").
				Description("Resident memory of the knot-resolver manager process only. The kresd workers that hold the cache and answer queries are not included -- the exporter attaches no instance_id to process_* series, so there is one reading per host regardless of worker count. Cache occupancy is in the panel beside this one.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`process_resident_memory_bytes{` + resolver + `}`).
					LegendFormat("{{instance}} RSS"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Resolver Metrics Freshness").
				Description("Per-resolver textfile age, which carries up to one scrape interval on top of the real age: the collector rewrites every 15s, Prometheus scrapes every 30s, and the reading climbs between scrapes. Healthy values sweep roughly 11-45s. Zoomed in the shape is a rising sawtooth; at the default range the step equals the scrape interval and the same signal aliases into a slow drift with occasional jumps. Neither shape is a fault -- only a line that climbs past 60s and keeps going, which means the collector stopped.").
				Datasource(ds).
				// Full width keeps the collector-age sawtooth readable.
				Span(24).Height(8).
				Unit("s").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`time() - node_textfile_mtime_seconds{` + resolver + `,file=~".*knot_resolver\\.prom"}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Knot Resolver DNSSEC Validation Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("DNSSEC Bogus and Validation Logs").
				Description("Knot Resolver journald entries shipped by Vector from resolver1 and resolver2.").
				Datasource(lokiDS).
				Span(24).Height(12).
				ShowTime(true).
				EnableLogDetails(true).
				SortOrder(common.LogsSortOrderDescending).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(resolverValidationLogs).
					MaxLines(500),
				),
		).
		WithRow(dashboard.NewRowBuilder("pdns-auth Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("pdns-auth QPS").
				Description("Authoritative queries per second across both transports.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("reqps").
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Count UDP and TCP consistently with the Query Rate panel; TCP includes
					// large responses and zone transfers.
					Expr(`sum(rate(pdns_auth_udp_queries{` + pdns + `}[$__rate_interval]))` +
						` + sum(rate(pdns_auth_tcp_queries{` + pdns + `}[$__rate_interval]))`).
					LegendFormat("QPS"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("pdns-auth Avg Latency").
				Datasource(ds).
				Span(12).Height(4).
				Unit("µs").
				Thresholds(latencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(pdns_auth_latency{` + pdns + `})`).
					LegendFormat("Avg Latency"),
				).Decimals(1),
		).
		WithRow(dashboard.NewRowBuilder("pdns-auth Traffic & Performance")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_udp_queries{` + pdns + `}[$__rate_interval])`).
					LegendFormat("{{instance}} UDP"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_tcp_queries{` + pdns + `}[$__rate_interval])`).
					LegendFormat("{{instance}} TCP"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Response Codes").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_noerror_packets{`+pdns+`}[$__rate_interval]))`).
					LegendFormat("NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_nxdomain_packets{`+pdns+`}[$__rate_interval]))`).
					LegendFormat("NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_servfail_packets{`+pdns+`}[$__rate_interval]))`).
					LegendFormat("SERVFAIL"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_refused_packets{`+pdns+`}[$__rate_interval]))`).
					LegendFormat("REFUSED"),
				).
				// Semantic coloring: OK=Green, Warning=Yellow, Error=Red, Refused=Orange
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "NOERROR"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "NXDOMAIN"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "SERVFAIL"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "REFUSED"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("µs").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(latencyThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pdns_auth_latency{` + pdns + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Backend Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("µs").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(latencyThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pdns_auth_backend_latency{` + pdns + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("CoreDNS Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS QPS").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(coredns_dns_requests_total{` + coredns + `}[$__rate_interval]))`).
					Instant().
					LegendFormat("QPS"),
				).
				Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS Targets Down").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(up{` + coredns + `} == 0) or vector(0)`).
					Instant().
					LegendFormat("Down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS SERVFAIL Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Thresholds(servfailThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(coredns_dns_responses_total{` + coredns + `,rcode="SERVFAIL"}[$__rate_interval]))`).
					Instant().
					LegendFormat("SERVFAIL"),
				).
				Decimals(2),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS Request Latency p99").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				Thresholds(corednsLatencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (le) (rate(coredns_dns_request_duration_seconds_bucket{` + coredns + `}[$__rate_interval])))`).
					Instant().
					LegendFormat("p99"),
				),
		).
		WithRow(dashboard.NewRowBuilder("CoreDNS Traffic & Performance")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CoreDNS QPS").
				Description("CoreDNS requests per second, grouped by cluster.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(coredns_dns_requests_total{` + coredns + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CoreDNS SERVFAIL Rate").
				Description("SERVFAIL responses per second from in-cluster CoreDNS, grouped by cluster.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(coredns_dns_responses_total{` + coredns + `,rcode="SERVFAIL"}[$__rate_interval]))`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CoreDNS Cache Hit Rate").
				Description("CoreDNS cache hit percentage, grouped by cluster.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 * sum by (cluster) (rate(coredns_cache_hits_total{` + coredns + `}[$__rate_interval])) / clamp_min(sum by (cluster) (rate(coredns_cache_requests_total{` + coredns + `}[$__rate_interval])), 1e-9)`).
					LegendFormat("{{cluster}}"),
				).
				Decimals(1),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CoreDNS Request Latency p99").
				Description("99th percentile CoreDNS request duration, grouped by cluster.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("s").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{` + coredns + `}[$__rate_interval])))`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("external-dns Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("external-dns Registry Errors (1h)").
				Description("Errors talking to the DNS provider (registry) in the last hour, all clusters.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(external_dns_registry_errors_total{` + extdns + `}[1h]))) or vector(0)`).
					Instant().
					LegendFormat("Errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("external-dns Source Errors (1h)").
				Description("Errors reading route sources (HTTPRoute/Service/...) in the last hour, all clusters.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(external_dns_source_errors_total{` + extdns + `}[1h]))) or vector(0)`).
					Instant().
					LegendFormat("Errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("external-dns Last Sync Age").
				Description("Time since the last successful full sync to the DNS provider, worst cluster.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("s").
				Thresholds(syncAgeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max(time() - external_dns_controller_last_sync_timestamp_seconds{` + extdns + `})`).
					Instant().
					LegendFormat("Age"),
				).Decimals(0),
		).
		WithRow(dashboard.NewRowBuilder("external-dns Sync & Records")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("external-dns Sync Age").
				Description("Seconds since the last provider sync and source reconcile, worst pod per cluster. A steadily climbing line means external-dns has stopped syncing.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Tooltip(tooltipAll).
				Legend(legend).
				// Aggregate pod and instance churn to the cluster label shown in the legend.
				// max reports the stalest replica, matching the summary tile.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max by (cluster) (time() - external_dns_controller_last_sync_timestamp_seconds{` + extdns + `})`).
					LegendFormat("{{cluster}} sync"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max by (cluster) (time() - external_dns_controller_last_reconcile_timestamp_seconds{` + extdns + `})`).
					LegendFormat("{{cluster}} reconcile"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("external-dns Error Rate").
				Description("Registry (provider) and source errors per second, grouped by cluster.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(external_dns_registry_errors_total{` + extdns + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} registry"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(external_dns_source_errors_total{` + extdns + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} source"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("external-dns Records").
				Description("Records desired by sources vs records present in the registry, grouped by cluster. The two lines should sit on top of each other; a persistent gap means records are failing to sync. Zone apex records (NS, SOA) are excluded because no source can produce them.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (external_dns_source_records{` + extdns + `})`).
					LegendFormat("{{cluster}} source"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Exclude apex NS/SOA records that no external-dns source can produce;
					// retain all other record types for future sources.
					Expr(`sum by (cluster) (external_dns_registry_records{` + extdns + `,record_type!~"ns|soa"})`).
					LegendFormat("{{cluster}} registry"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
