package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func buildDnsOverview() (*dashboardv2.Dashboard, error) {
	const (
		dnsdist              = `job="scrapeConfig/monitoring/dnsdist-external"`
		resolver             = `job="scrapeConfig/monitoring/node-exporter-external",service="knot-resolver"`
		pdns                 = `job="scrapeConfig/monitoring/pdns-auth-external"`
		coredns              = `job="coredns"`
		extdns               = `job="external-dns"`
		authoritativeDNSJobs = `job=~"scrapeConfig/monitoring/dns-authoritative-(udp|tcp)"`
	)
	ds := dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("$datasource")
	tooltip := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
	legend := common.NewVizLegendOptionsBuilder().ShowLegend(true).DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom)
	measurementThresholds := func() *dashboardv2.ThresholdsConfigBuilder {
		return dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "blue"}})
	}
	issueThresholds := func() *dashboardv2.ThresholdsConfigBuilder {
		return dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "green"}, {Value: new(float64(1)), Color: "red"}})
	}
	d := dashboardv2.NewDashboardBuilder("DNS Overview V2 PoC").
		Tags([]string{"dns", "infrastructure", "poc"}).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("30s")).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("datasource").Label("Datasource").PluginId("prometheus"))

	d.Panel("panel-1", dashboardv2.NewPanelBuilder().Id(1).Title("dnsdist QPS").
		Visualization(stat.NewVisualizationV2Builder().Unit("reqps").Decimals(1).Thresholds(measurementThresholds()).ColorMode(common.BigValueColorModeValue).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval]))`, "QPS", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-2", dashboardv2.NewPanelBuilder().Id(2).Title("dnsdist Avg Latency").
		Visualization(stat.NewVisualizationV2Builder().Unit("µs").Decimals(1).Thresholds(dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: new(float64(0)), Color: "green"}, {Value: new(float64(50000)), Color: "yellow"}, {Value: new(float64(150000)), Color: "red"}})).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `avg(dnsdist_latency_avg100{`+dnsdist+`})`, "Avg Latency", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-3", dashboardv2.NewPanelBuilder().Id(3).Title("dnsdist Query/Response Rate").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("reqps").Thresholds(dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "transparent"}, {Value: new(float64(0)), Color: "white"}})).Tooltip(tooltip).Legend(legend).ThresholdsStyle(common.NewGraphThresholdsStyleConfigBuilder().Mode(common.GraphThresholdsStyleModeLine)).OverrideByQuery("Responses", []dashboardv2.DynamicConfigValue{{Id: "custom.transform", Value: "negative-Y"}})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("Queries", ds, `rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval])`, "{{instance}} Queries", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("Responses", ds, `rate(dnsdist_responses{`+dnsdist+`}[$__rate_interval])`, "{{instance}} Responses", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-4", dashboardv2.NewPanelBuilder().Id(4).Title("dnsdist Response Codes").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("reqps").Tooltip(tooltip).Legend(legend).Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).FillOpacity(10).OverrideByName("NOERROR", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "green", "mode": "fixed"}}}).OverrideByName("NXDOMAIN", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "yellow", "mode": "fixed"}}}).OverrideByName("SERVFAIL", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "red", "mode": "fixed"}}}).OverrideByName("REFUSED", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "orange", "mode": "fixed"}}})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(rate(dnsdist_frontend_noerror{`+dnsdist+`}[$__rate_interval]))`, "NOERROR", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("B", ds, `sum(rate(dnsdist_frontend_nxdomain{`+dnsdist+`}[$__rate_interval]))`, "NXDOMAIN", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("C", ds, `sum(rate(dnsdist_frontend_servfail{`+dnsdist+`}[$__rate_interval]))`, "SERVFAIL", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("D", ds, `sum(rate(dnsdist_frontend_refused{`+dnsdist+`}[$__rate_interval]))`, "REFUSED", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-5", dashboardv2.NewPanelBuilder().Id(5).Title("Resolver Cache Hit Rate").
		Visualization(stat.NewVisualizationV2Builder().Unit("percent").Decimals(1).Thresholds(dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "red"}, {Value: new(float64(60)), Color: "yellow"}, {Value: new(float64(90)), Color: "green"}})).ColorMode(common.BigValueColorModeValue).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `100 * sum(rate(resolver_answer_cached_total{`+resolver+`}[$__rate_interval])) / sum(rate(resolver_answer_total{`+resolver+`}[$__rate_interval]))`, "Hit rate", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-6", dashboardv2.NewPanelBuilder().Id(6).Title("Resolver Metrics Age").Description("Worst age of the node_exporter textfile, which the collector rewrites every 15s. Healthy readings run up to about 46s because the age carries the Prometheus scrape interval on top of the write interval; red matches the KnotResolverMetricsStale alert.").
		Visualization(stat.NewVisualizationV2Builder().Unit("s").Decimals(0).Thresholds(dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "green"}, {Value: new(float64(60)), Color: "red"}})).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `max(time() - node_textfile_mtime_seconds{`+resolver+`,file=~".*knot_resolver\\.prom"})`, "Age", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-7", dashboardv2.NewPanelBuilder().Id(7).Title("Resolver Query Rate").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("reqps").Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum by (instance) (rate(resolver_request_total{`+resolver+`}[$__rate_interval]))`, "{{instance}}", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-8", dashboardv2.NewPanelBuilder().Id(8).Title("Resolver Response Latency").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("s").Thresholds(dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: new(float64(0)), Color: "green"}, {Value: new(float64(0.05)), Color: "yellow"}, {Value: new(float64(0.15)), Color: "red"}})).Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `histogram_quantile(0.50, sum by (instance, le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`, "{{instance}} p50", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("B", ds, `histogram_quantile(0.95, sum by (instance, le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`, "{{instance}} p95", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("C", ds, `histogram_quantile(0.99, sum by (instance, le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`, "{{instance}} p99", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-9", dashboardv2.NewPanelBuilder().Id(9).Title("pdns-auth QPS").Description("Authoritative queries per second across both transports.").
		Visualization(stat.NewVisualizationV2Builder().Unit("reqps").Decimals(1).Thresholds(measurementThresholds()).ColorMode(common.BigValueColorModeValue).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(rate(pdns_auth_udp_queries{`+pdns+`}[$__rate_interval])) + sum(rate(pdns_auth_tcp_queries{`+pdns+`}[$__rate_interval]))`, "QPS", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-10", dashboardv2.NewPanelBuilder().Id(10).Title("Failed Authoritative DNS Probes").Description("Current failed direct root NS probes across both resolvers and transports.").
		Visualization(stat.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(issueThresholds()).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(probe_success{`+authoritativeDNSJobs+`} == 0) or vector(0)`, "Failed", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-11", dashboardv2.NewPanelBuilder().Id(11).Title("pdns-auth Response Codes").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("reqps").Tooltip(tooltip).Legend(legend).Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).FillOpacity(10).OverrideByName("NOERROR", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "green", "mode": "fixed"}}}).OverrideByName("NXDOMAIN", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "yellow", "mode": "fixed"}}}).OverrideByName("SERVFAIL", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "red", "mode": "fixed"}}}).OverrideByName("REFUSED", []dashboardv2.DynamicConfigValue{{Id: "color", Value: map[string]any{"fixedColor": "orange", "mode": "fixed"}}})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(rate(pdns_auth_noerror_packets{`+pdns+`}[$__rate_interval]))`, "NOERROR", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("B", ds, `sum(rate(pdns_auth_nxdomain_packets{`+pdns+`}[$__rate_interval]))`, "NXDOMAIN", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("C", ds, `sum(rate(pdns_auth_servfail_packets{`+pdns+`}[$__rate_interval]))`, "SERVFAIL", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("D", ds, `sum(rate(pdns_auth_refused_packets{`+pdns+`}[$__rate_interval]))`, "REFUSED", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-12", dashboardv2.NewPanelBuilder().Id(12).Title("pdns-auth Latency").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("µs").Thresholds(dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: new(float64(0)), Color: "green"}, {Value: new(float64(50000)), Color: "yellow"}, {Value: new(float64(150000)), Color: "red"}})).Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `pdns_auth_latency{`+pdns+`}`, "{{instance}}", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-13", dashboardv2.NewPanelBuilder().Id(13).Title("CoreDNS QPS").
		Visualization(stat.NewVisualizationV2Builder().Unit("reqps").Decimals(1).Thresholds(measurementThresholds()).ColorMode(common.BigValueColorModeValue).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(rate(coredns_dns_requests_total{`+coredns+`}[$__rate_interval]))`, "QPS", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-14", dashboardv2.NewPanelBuilder().Id(14).Title("CoreDNS Targets Down").
		Visualization(stat.NewVisualizationV2Builder().Unit("short").Thresholds(issueThresholds()).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum(up{`+coredns+`} == 0) or vector(0)`, "Down", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-15", dashboardv2.NewPanelBuilder().Id(15).Title("CoreDNS SERVFAIL Rate").Description("SERVFAIL responses per second from in-cluster CoreDNS, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("reqps").Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum by (cluster) (rate(coredns_dns_responses_total{`+coredns+`,rcode="SERVFAIL"}[$__rate_interval]))`, "{{cluster}}", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-16", dashboardv2.NewPanelBuilder().Id(16).Title("CoreDNS Request Latency p99").Description("99th percentile CoreDNS request duration, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("s").Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{`+coredns+`}[$__rate_interval])))`, "{{cluster}}", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-17", dashboardv2.NewPanelBuilder().Id(17).Title("external-dns Registry Errors (1h)").Description("Errors talking to the DNS provider (registry) in the last hour, all clusters.").
		Visualization(stat.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(issueThresholds()).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `ceil(sum(increase(external_dns_registry_errors_total{`+extdns+`}[1h]))) or vector(0)`, "Errors", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-18", dashboardv2.NewPanelBuilder().Id(18).Title("external-dns Source Errors (1h)").Description("Errors reading route sources (HTTPRoute/Service/...) in the last hour, all clusters.").
		Visualization(stat.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(issueThresholds()).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `ceil(sum(increase(external_dns_source_errors_total{`+extdns+`}[1h]))) or vector(0)`, "Errors", true, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-19", dashboardv2.NewPanelBuilder().Id(19).Title("external-dns Sync Age").Description("Seconds since the last provider sync and source reconcile, worst pod per cluster. A steadily climbing line means external-dns has stopped syncing.").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("s").Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `max by (cluster) (time() - external_dns_controller_last_sync_timestamp_seconds{`+extdns+`})`, "{{cluster}} sync", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("B", ds, `max by (cluster) (time() - external_dns_controller_last_reconcile_timestamp_seconds{`+extdns+`})`, "{{cluster}} reconcile", false, prometheus.PromQueryFormatTimeSeries))))

	d.Panel("panel-20", dashboardv2.NewPanelBuilder().Id(20).Title("external-dns Records").Description("Records desired by sources vs records present in the registry, grouped by cluster. The two lines should sit on top of each other; a persistent gap means records are failing to sync. Zone apex records (NS, SOA) are excluded because no source can produce them.").
		Visualization(timeseries.NewVisualizationV2Builder().Unit("short").Tooltip(tooltip).Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(promTarget("A", ds, `sum by (cluster) (external_dns_source_records{`+extdns+`})`, "{{cluster}} source", false, prometheus.PromQueryFormatTimeSeries)).
			Target(promTarget("B", ds, `sum by (cluster) (external_dns_registry_records{`+extdns+`,record_type!~"ns|soa"})`, "{{cluster}} registry", false, prometheus.PromQueryFormatTimeSeries))))

	d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("dnsdist").GridLayout(dashboardv2.NewGridBuilder().
			Item(dashboardv2.GridItem("panel-1").X(0).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-2").X(12).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-3").X(0).Y(4).Width(12).Height(8)).
			Item(dashboardv2.GridItem("panel-4").X(12).Y(4).Width(12).Height(8)))).
		Row(dashboardv2.NewRowBuilder().Title("Resolvers").GridLayout(dashboardv2.NewGridBuilder().
			Item(dashboardv2.GridItem("panel-5").X(0).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-6").X(12).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-7").X(0).Y(4).Width(12).Height(8)).
			Item(dashboardv2.GridItem("panel-8").X(12).Y(4).Width(12).Height(8)))).
		Row(dashboardv2.NewRowBuilder().Title("Authoritative DNS").GridLayout(dashboardv2.NewGridBuilder().
			Item(dashboardv2.GridItem("panel-9").X(0).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-10").X(12).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-11").X(0).Y(4).Width(12).Height(8)).
			Item(dashboardv2.GridItem("panel-12").X(12).Y(4).Width(12).Height(8)))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS").GridLayout(dashboardv2.NewGridBuilder().
			Item(dashboardv2.GridItem("panel-13").X(0).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-14").X(12).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-15").X(0).Y(4).Width(12).Height(8)).
			Item(dashboardv2.GridItem("panel-16").X(12).Y(4).Width(12).Height(8)))).
		Row(dashboardv2.NewRowBuilder().Title("external-dns").GridLayout(dashboardv2.NewGridBuilder().
			Item(dashboardv2.GridItem("panel-17").X(0).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-18").X(12).Y(0).Width(12).Height(4)).
			Item(dashboardv2.GridItem("panel-19").X(0).Y(4).Width(12).Height(8)).
			Item(dashboardv2.GridItem("panel-20").X(12).Y(4).Width(12).Height(8)))))
	result, err := d.Build()
	return &result, err
}
