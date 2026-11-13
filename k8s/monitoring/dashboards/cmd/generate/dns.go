package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	dnsPanelDnsdistQPS                      = 1
	dnsPanelDnsdistAvgLatency               = 2
	dnsPanelDnsdistQueryResponseRate        = 3
	dnsPanelDnsdistResponseCodes            = 4
	dnsPanelDnsdistLatency                  = 5
	dnsPanelDnsdistDropRate                 = 6
	dnsPanelDnsdistUnansweredQueries        = 7
	dnsPanelResolverQPS                     = 8
	dnsPanelResolverCacheHitRateCurrent     = 9
	dnsPanelResolverLatencyP95              = 10
	dnsPanelResolverMetricsAge              = 11
	dnsPanelResolverQueryRate               = 12
	dnsPanelResolverCacheHitRate            = 13
	dnsPanelResolverResponseCodes           = 14
	dnsPanelResolverResponseLatency         = 15
	dnsPanelResolverCacheUsage              = 16
	dnsPanelResolverManagerMemory           = 17
	dnsPanelResolverMetricsFreshness        = 18
	dnsPanelAuthoritativeDNSProbeStatus     = 19
	dnsPanelAuthoritativeDNSProbeDuration   = 20
	dnsPanelAuthoritativeDNSAvailability    = 21
	dnsPanelFailedAuthoritativeDNSProbes    = 22
	dnsPanelDNSSECBogusAndValidationLogs    = 23
	dnsPanelPdnsAuthQPS                     = 24
	dnsPanelPdnsAuthAvgLatency              = 25
	dnsPanelPdnsAuthQueryRate               = 26
	dnsPanelPdnsAuthResponseCodes           = 27
	dnsPanelPdnsAuthLatency                 = 28
	dnsPanelPdnsAuthBackendLatency          = 29
	dnsPanelCoreDNSQPSCurrent               = 30
	dnsPanelCoreDNSTargetsDown              = 31
	dnsPanelCoreDNSSERVFAILRateCurrent      = 32
	dnsPanelCoreDNSRequestLatencyP99Current = 33
	dnsPanelCoreDNSQPS                      = 34
	dnsPanelCoreDNSSERVFAILRate             = 35
	dnsPanelCoreDNSCacheHitRate             = 36
	dnsPanelCoreDNSRequestLatencyP99        = 37
	dnsPanelExternalDnsRegistryErrors1h     = 38
	dnsPanelExternalDnsSourceErrors1h       = 39
	dnsPanelExternalDnsLastSyncAge          = 40
	dnsPanelExternalDnsSyncAge              = 41
	dnsPanelExternalDnsErrorRate            = 42
	dnsPanelExternalDnsRecords              = 43
)

// buildDnsOverview covers DNS infrastructure. Scrape configs relabel
// dnsdist and pdns-auth instances to hostnames.
func buildDnsOverview() (*dashboardv2.Dashboard, error) {
	const (
		elementDnsdistQPS                      = "dnsdist-qps"
		elementDnsdistAvgLatency               = "dnsdist-avg-latency"
		elementDnsdistQueryResponseRate        = "dnsdist-query-response-rate"
		elementDnsdistResponseCodes            = "dnsdist-response-codes"
		elementDnsdistLatency                  = "dnsdist-latency"
		elementDnsdistDropRate                 = "dnsdist-drop-rate"
		elementDnsdistUnansweredQueries        = "dnsdist-unanswered-queries"
		elementResolverQPS                     = "resolver-qps"
		elementResolverCacheHitRateCurrent     = "resolver-cache-hit-rate-current"
		elementResolverLatencyP95              = "resolver-latency-p95"
		elementResolverMetricsAge              = "resolver-metrics-age"
		elementResolverQueryRate               = "resolver-query-rate"
		elementResolverCacheHitRate            = "resolver-cache-hit-rate"
		elementResolverResponseCodes           = "resolver-response-codes"
		elementResolverResponseLatency         = "resolver-response-latency"
		elementResolverCacheUsage              = "resolver-cache-usage"
		elementResolverManagerMemory           = "resolver-manager-memory"
		elementResolverMetricsFreshness        = "resolver-metrics-freshness"
		elementAuthoritativeDNSProbeStatus     = "authoritative-dns-probe-status"
		elementAuthoritativeDNSProbeDuration   = "authoritative-dns-probe-duration"
		elementAuthoritativeDNSAvailability    = "authoritative-dns-availability"
		elementFailedAuthoritativeDNSProbes    = "failed-authoritative-dns-probes"
		elementDNSSECBogusAndValidationLogs    = "dnssec-bogus-and-validation-logs"
		elementPdnsAuthQPS                     = "pdns-auth-qps"
		elementPdnsAuthAvgLatency              = "pdns-auth-avg-latency"
		elementPdnsAuthQueryRate               = "pdns-auth-query-rate"
		elementPdnsAuthResponseCodes           = "pdns-auth-response-codes"
		elementPdnsAuthLatency                 = "pdns-auth-latency"
		elementPdnsAuthBackendLatency          = "pdns-auth-backend-latency"
		elementCoreDNSQPSCurrent               = "coredns-qps-current"
		elementCoreDNSTargetsDown              = "coredns-targets-down"
		elementCoreDNSSERVFAILRateCurrent      = "coredns-servfail-rate-current"
		elementCoreDNSRequestLatencyP99Current = "coredns-request-latency-p99-current"
		elementCoreDNSQPS                      = "coredns-qps"
		elementCoreDNSSERVFAILRate             = "coredns-servfail-rate"
		elementCoreDNSCacheHitRate             = "coredns-cache-hit-rate"
		elementCoreDNSRequestLatencyP99        = "coredns-request-latency-p99"
		elementExternalDnsRegistryErrors1h     = "external-dns-registry-errors-1h"
		elementExternalDnsSourceErrors1h       = "external-dns-source-errors-1h"
		elementExternalDnsLastSyncAge          = "external-dns-last-sync-age"
		elementExternalDnsSyncAge              = "external-dns-sync-age"
		elementExternalDnsErrorRate            = "external-dns-error-rate"
		elementExternalDnsRecords              = "external-dns-records"
	)

	const (
		dnsdist                = `job="scrapeConfig/monitoring/dnsdist-external"`
		resolver               = `job="scrapeConfig/monitoring/node-exporter-external",service="knot-resolver"`
		pdns                   = `job="scrapeConfig/monitoring/pdns-auth-external"`
		coredns                = `job="coredns"`
		extdns                 = `job="external-dns"`
		authoritativeDNSJobs   = `job=~"scrapeConfig/monitoring/dns-authoritative-(udp|tcp)"`
		resolverValidationLogs = `{job="dns-resolver", unit="knot-resolver.service"} | json | __error__="" |~ "(?i)(bogus|dnssec|validation)" | line_format "{{.message}}"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// zeroLine draws a solid reference line at y=0 for bidirectional rate panels.
	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// Latency thresholds in microseconds (50ms = warning, 150ms = critical).
	latencyThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Color: "green", Value: new(float64(0))},
			{Color: "yellow", Value: new(float64(50000))},
			{Color: "red", Value: new(float64(150000))},
		})

	corednsLatencyThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Color: "green", Value: new(float64(0))},
			{Color: "yellow", Value: new(0.05)},
			{Color: "red", Value: new(0.15)},
		})

	issueThresholds := issueThresholds()

	probeThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})

	availabilityThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(99)), Color: "yellow"},
			{Value: new(float64(99.9)), Color: "green"},
		})

	probeValueMappings := []dashboardv2.ValueMapping{
		{ValueMap: &dashboardv2.ValueMap{
			Type: dashboardv2.MappingTypeValue,
			Options: map[string]dashboardv2.ValueMappingResult{
				"0": {Text: new("DOWN"), Color: new("red")},
				"1": {Text: new("UP"), Color: new("green")},
			},
		}},
	}

	// Match resolver alert ratios: healthy >=90%, critical <60%.
	resolverCacheHitRateThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(60)), Color: "yellow"},
			{Value: new(float64(90)), Color: "green"},
		})

	servfailThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(0.001), Color: "red"},
		})

	// Warn after several missed one-minute syncs; alert after 15 minutes.
	syncAgeThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(300)), Color: "yellow"},
			{Value: new(float64(900)), Color: "red"},
		})

	d := dashboardv2.NewDashboardBuilder("DNS Overview").
		Tags([]string{"dns", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("loki_datasource").
			Label("Loki Datasource").
			PluginId("loki"))

	d.Panel(elementDnsdistQPS, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistQPS).
		Title("dnsdist QPS").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("reqps").
			Thresholds(measurementThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval]))`).
				LegendFormat("QPS")))))

	d.Panel(elementDnsdistAvgLatency, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistAvgLatency).
		Title("dnsdist Avg Latency").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("µs").
			Thresholds(latencyThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg(dnsdist_latency_avg100{`+dnsdist+`})`).
				LegendFormat("Avg Latency")))))

	d.Panel(elementDnsdistQueryResponseRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistQueryResponseRate).
		Title("dnsdist Query/Response Rate").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(zeroLineThresholds).
			ThresholdsStyle(zeroLineStyle).
			OverrideByQuery("Responses", []dashboardv2.DynamicConfigValue{
				{Id: "custom.transform", Value: "negative-Y"},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("Queries").Query(promQuery().
				RefId("Queries").
				Expr(`rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval])`).
				LegendFormat("{{instance}} Queries"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Responses").Query(promQuery().
				RefId("Responses").
				Expr(`rate(dnsdist_responses{`+dnsdist+`}[$__rate_interval])`).
				LegendFormat("{{instance}} Responses")))))

	d.Panel(elementDnsdistResponseCodes, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistResponseCodes).
		Title("dnsdist Response Codes").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			// Semantic coloring: OK=Green, Warning=Yellow, Error=Red, Refused=Orange
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "NOERROR"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "NXDOMAIN"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "SERVFAIL"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "REFUSED"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(dnsdist_frontend_noerror{`+dnsdist+`}[$__rate_interval]))`).
				LegendFormat("NOERROR"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`sum(rate(dnsdist_frontend_nxdomain{`+dnsdist+`}[$__rate_interval]))`).
				LegendFormat("NXDOMAIN"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`sum(rate(dnsdist_frontend_servfail{`+dnsdist+`}[$__rate_interval]))`).
				LegendFormat("SERVFAIL"))).
			Target(dashboardv2.NewTargetBuilder().RefId("D").Query(promQuery().
				Expr(`sum(rate(dnsdist_frontend_refused{`+dnsdist+`}[$__rate_interval]))`).
				LegendFormat("REFUSED")))))

	d.Panel(elementDnsdistLatency, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistLatency).
		Title("dnsdist Latency").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("µs").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(latencyThresholds).
			// Visual differentiation: emphasize avg100 (short-term) and de-emphasize avg1000 (long-term trend).
			Override(dashboardv2.MatcherConfig{
				Id:      "byRegexp",
				Options: ".*avg1000.*",
			}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				{Id: "drawStyle", Value: "line"},
				{Id: "fillOpacity", Value: 10},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`dnsdist_latency_avg100{`+dnsdist+`}`).
				LegendFormat("{{instance}} avg100"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`dnsdist_latency_avg1000{`+dnsdist+`}`).
				LegendFormat("{{instance}} avg1000")))))

	d.Panel(elementDnsdistDropRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistDropRate).
		Title("dnsdist Drop Rate").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`rate(dnsdist_acl_drops{`+dnsdist+`}[$__rate_interval])`).
				LegendFormat("{{instance}} ACL Drop"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`rate(dnsdist_rule_drops{`+dnsdist+`}[$__rate_interval])`).
				LegendFormat("{{instance}} Rule Drop"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`rate(dnsdist_dynamic_blocked{`+dnsdist+`}[$__rate_interval])`).
				LegendFormat("{{instance}} Dynamic Block")))))

	d.Panel(elementDnsdistUnansweredQueries, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDnsdistUnansweredQueries).
		Title("dnsdist Unanswered Queries").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval]) - rate(dnsdist_responses{`+dnsdist+`}[$__rate_interval])`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementResolverQPS, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverQPS).
		Title("Resolver QPS").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("reqps").
			Thresholds(measurementThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(resolver_request_total{`+resolver+`}[$__rate_interval]))`).
				Instant(true).Range(false).
				LegendFormat("QPS")))))

	d.Panel(elementResolverCacheHitRateCurrent, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverCacheHitRateCurrent).
		Title("Resolver Cache Hit Rate (Current)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Thresholds(resolverCacheHitRateThresholds).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				// Preserve a gap for undefined 0/0 cache ratios; clamping would report a
				// false 0% failure during idle windows.
				Expr(`100 * sum(rate(resolver_answer_cached_total{`+resolver+`}[$__rate_interval])) / sum(rate(resolver_answer_total{`+resolver+`}[$__rate_interval]))`).
				Instant(true).Range(false).
				LegendFormat("Hit rate")))))

	d.Panel(elementResolverLatencyP95, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverLatencyP95).
		Title("Resolver Latency p95").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Thresholds(corednsLatencyThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`histogram_quantile(0.95, sum by (le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`).
				Instant(true).Range(false).
				LegendFormat("p95")))))

	d.Panel(elementResolverMetricsAge, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverMetricsAge).
		Title("Resolver Metrics Age").
		Description("Worst age of the node_exporter textfile, which the collector rewrites every 15s. Healthy readings run up to about 46s because the age carries the Prometheus scrape interval on top of the write interval; red matches the KnotResolverMetricsStale alert.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			ColorMode(common.BigValueColorModeBackground).
			// Match resolverAlerts.metricsMaxAgeSeconds. Healthy write and scrape
			// intervals already consume most of the margin, so omit an amber band.
			Thresholds(dashboardv2.NewThresholdsConfigBuilder().
				Mode(dashboardv2.ThresholdsModeAbsolute).
				Steps([]dashboardv2.Threshold{
					{Value: nil, Color: "green"},
					{Value: new(float64(60)), Color: "red"},
				})).
			Decimals(0)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`max(time() - node_textfile_mtime_seconds{`+resolver+`,file=~".*knot_resolver\\.prom"})`).
				Instant(true).Range(false).
				LegendFormat("Age")))))

	d.Panel(elementResolverQueryRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverQueryRate).
		Title("Resolver Query Rate").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (instance) (rate(resolver_request_total{`+resolver+`}[$__rate_interval]))`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementResolverCacheHitRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverCacheHitRate).
		Title("Resolver Cache Hit Rate").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Tooltip(tooltipAll).
			Legend(legend).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				// Preserve gaps for undefined cache ratios.
				Expr(`100 * sum by (instance) (rate(resolver_answer_cached_total{`+resolver+`}[$__rate_interval])) / sum by (instance) (rate(resolver_answer_total{`+resolver+`}[$__rate_interval]))`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementResolverResponseCodes, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverResponseCodes).
		Title("Resolver Response Codes").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(resolver_answer_rcode_noerror_total{`+resolver+`}[$__rate_interval]))`).
				LegendFormat("NOERROR"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`sum(rate(resolver_answer_rcode_nodata_total{`+resolver+`}[$__rate_interval]))`).
				LegendFormat("NODATA"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`sum(rate(resolver_answer_rcode_nxdomain_total{`+resolver+`}[$__rate_interval]))`).
				LegendFormat("NXDOMAIN"))).
			Target(dashboardv2.NewTargetBuilder().RefId("D").Query(promQuery().
				Expr(`sum(rate(resolver_answer_rcode_servfail_total{`+resolver+`}[$__rate_interval]))`).
				LegendFormat("SERVFAIL")))))

	d.Panel(elementResolverResponseLatency, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverResponseLatency).
		Title("Resolver Response Latency").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(corednsLatencyThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`histogram_quantile(0.50, sum by (instance, le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`).
				LegendFormat("{{instance}} p50"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`histogram_quantile(0.95, sum by (instance, le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`).
				LegendFormat("{{instance}} p95"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (instance, le) (rate(resolver_response_latency_bucket{`+resolver+`}[$__rate_interval])))`).
				LegendFormat("{{instance}} p99")))))

	// Place cache occupancy beside memory because both show resource exhaustion.
	d.Panel(elementResolverCacheUsage, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverCacheUsage).
		Title("Resolver Cache Usage").
		Description("LMDB pages allocated against the configured map size. When the map fills, every stash fails and the cache is dropped, which collapses the hit rate and floods upstream with recursion until it refills -- see the 2026-08-10 addendum to ADR-0030. kresd publishes no cache metric, so this is read from the database's meta page by the textfile collector; data.mdb's own size is preallocated to the map size and cannot be used.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(dashboardv2.NewThresholdsConfigBuilder().
				Mode(dashboardv2.ThresholdsModeAbsolute).
				Steps([]dashboardv2.Threshold{
					{Value: nil, Color: "green"},
					{Value: new(float64(70)), Color: "yellow"},
					{Value: new(float64(85)), Color: "red"},
				})).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`100 * knot_resolver_cache_used_bytes{`+resolver+`} / knot_resolver_cache_size_bytes{`+resolver+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementResolverManagerMemory, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverManagerMemory).
		Title("Resolver Manager Memory").
		Description("Resident memory of the knot-resolver manager process only. The kresd workers that hold the cache and answer queries are not included -- the exporter attaches no instance_id to process_* series, so there is one reading per host regardless of worker count. Cache occupancy is in the panel beside this one.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`process_resident_memory_bytes{`+resolver+`}`).
				LegendFormat("{{instance}} RSS")))))

	d.Panel(elementResolverMetricsFreshness, dashboardv2.NewPanelBuilder().
		Id(dnsPanelResolverMetricsFreshness).
		Title("Resolver Metrics Freshness").
		Description("Per-resolver textfile age, which carries up to one scrape interval on top of the real age: the collector rewrites every 15s, Prometheus scrapes every 30s, and the reading climbs between scrapes. Healthy values sweep roughly 11-45s. Zoomed in the shape is a rising sawtooth; at the default range the step equals the scrape interval and the same signal aliases into a slow drift with occasional jumps. Neither shape is a fault -- only a line that climbs past 60s and keeps going, which means the collector stopped.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`time() - node_textfile_mtime_seconds{`+resolver+`,file=~".*knot_resolver\\.prom"}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementAuthoritativeDNSProbeStatus, dashboardv2.NewPanelBuilder().
		Id(dnsPanelAuthoritativeDNSProbeStatus).
		Title("Authoritative DNS Probe Status").
		Description("Direct non-recursive root NS probes from each resolver. A UDP-only failure indicates a transport-specific path problem.").
		Visualization(statetimeline.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			ShowValue(common.VisibilityModeNever).
			MergeValues(true).
			Tooltip(tooltipAll)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_success{`+authoritativeDNSJobs+`}`).
				LegendFormat("{{probe_source}} {{transport}} {{dns_server}}")))))

	d.Panel(elementAuthoritativeDNSProbeDuration, dashboardv2.NewPanelBuilder().
		Id(dnsPanelAuthoritativeDNSProbeDuration).
		Title("Authoritative DNS Probe Duration").
		Description("End-to-end duration of direct root NS probes, split by resolver, transport, and root server.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_duration_seconds{`+authoritativeDNSJobs+`}`).
				LegendFormat("{{probe_source}} {{transport}} {{dns_server}}")))))

	d.Panel(elementAuthoritativeDNSAvailability, dashboardv2.NewPanelBuilder().
		Id(dnsPanelAuthoritativeDNSAvailability).
		Title("Authoritative DNS Availability").
		Description("Successful direct root NS probes over the selected dashboard time range.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Thresholds(availabilityThresholds).
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg_over_time(probe_success{`+authoritativeDNSJobs+`}[$__range]) * 100`).
				Instant(true).Range(false).
				LegendFormat("{{probe_source}} {{transport}} {{dns_server}}")))))

	d.Panel(elementFailedAuthoritativeDNSProbes, dashboardv2.NewPanelBuilder().
		Id(dnsPanelFailedAuthoritativeDNSProbes).
		Title("Failed Authoritative DNS Probes").
		Description("Current failed direct root NS probes across both resolvers and transports.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(probe_success{`+authoritativeDNSJobs+`} == 0) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Failed")))))

	d.Panel(elementDNSSECBogusAndValidationLogs, dashboardv2.NewPanelBuilder().
		Id(dnsPanelDNSSECBogusAndValidationLogs).
		Title("DNSSEC Bogus and Validation Logs").
		Description("Knot Resolver journald entries shipped by Vector from resolver1 and resolver2.").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			EnableLogDetails(true).
			SortOrder(common.LogsSortOrderDescending)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().Datasource(queryDatasource("loki_datasource")).
				Expr(resolverValidationLogs).
				MaxLines(500)))))

	d.Panel(elementPdnsAuthQPS, dashboardv2.NewPanelBuilder().
		Id(dnsPanelPdnsAuthQPS).
		Title("pdns-auth QPS").
		Description("Authoritative queries per second across both transports.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("reqps").
			Thresholds(measurementThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				// Count UDP and TCP consistently with the Query Rate panel; TCP includes
				// large responses and zone transfers.
				Expr(`sum(rate(pdns_auth_udp_queries{`+pdns+`}[$__rate_interval]))`+
					` + sum(rate(pdns_auth_tcp_queries{`+pdns+`}[$__rate_interval]))`).
				LegendFormat("QPS")))))

	d.Panel(elementPdnsAuthAvgLatency, dashboardv2.NewPanelBuilder().
		Id(dnsPanelPdnsAuthAvgLatency).
		Title("pdns-auth Avg Latency").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("µs").
			Thresholds(latencyThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg(pdns_auth_latency{`+pdns+`})`).
				LegendFormat("Avg Latency")))))

	d.Panel(elementPdnsAuthQueryRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelPdnsAuthQueryRate).
		Title("pdns-auth Query Rate").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`rate(pdns_auth_udp_queries{`+pdns+`}[$__rate_interval])`).
				LegendFormat("{{instance}} UDP"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`rate(pdns_auth_tcp_queries{`+pdns+`}[$__rate_interval])`).
				LegendFormat("{{instance}} TCP")))))

	d.Panel(elementPdnsAuthResponseCodes, dashboardv2.NewPanelBuilder().
		Id(dnsPanelPdnsAuthResponseCodes).
		Title("pdns-auth Response Codes").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			// Semantic coloring: OK=Green, Warning=Yellow, Error=Red, Refused=Orange
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "NOERROR"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "NXDOMAIN"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "SERVFAIL"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byName", Options: "REFUSED"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(pdns_auth_noerror_packets{`+pdns+`}[$__rate_interval]))`).
				LegendFormat("NOERROR"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`sum(rate(pdns_auth_nxdomain_packets{`+pdns+`}[$__rate_interval]))`).
				LegendFormat("NXDOMAIN"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`sum(rate(pdns_auth_servfail_packets{`+pdns+`}[$__rate_interval]))`).
				LegendFormat("SERVFAIL"))).
			Target(dashboardv2.NewTargetBuilder().RefId("D").Query(promQuery().
				Expr(`sum(rate(pdns_auth_refused_packets{`+pdns+`}[$__rate_interval]))`).
				LegendFormat("REFUSED")))))

	d.Panel(elementPdnsAuthLatency, dashboardv2.NewPanelBuilder().
		Id(dnsPanelPdnsAuthLatency).
		Title("pdns-auth Latency").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("µs").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(latencyThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`pdns_auth_latency{`+pdns+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementPdnsAuthBackendLatency, dashboardv2.NewPanelBuilder().
		Id(dnsPanelPdnsAuthBackendLatency).
		Title("pdns-auth Backend Latency").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("µs").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(latencyThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`pdns_auth_backend_latency{`+pdns+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementCoreDNSQPSCurrent, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSQPSCurrent).
		Title("CoreDNS QPS (Current)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("reqps").
			Thresholds(measurementThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(coredns_dns_requests_total{`+coredns+`}[$__rate_interval]))`).
				Instant(true).Range(false).
				LegendFormat("QPS")))))

	d.Panel(elementCoreDNSTargetsDown, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSTargetsDown).
		Title("CoreDNS Targets Down").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(up{`+coredns+`} == 0) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Down")))))

	d.Panel(elementCoreDNSSERVFAILRateCurrent, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSSERVFAILRateCurrent).
		Title("CoreDNS SERVFAIL Rate (Current)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("reqps").
			Thresholds(servfailThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Decimals(2)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(rate(coredns_dns_responses_total{`+coredns+`,rcode="SERVFAIL"}[$__rate_interval]))`).
				Instant(true).Range(false).
				LegendFormat("SERVFAIL")))))

	d.Panel(elementCoreDNSRequestLatencyP99Current, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSRequestLatencyP99Current).
		Title("CoreDNS Request Latency p99 (Current)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Thresholds(corednsLatencyThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (le) (rate(coredns_dns_request_duration_seconds_bucket{`+coredns+`}[$__rate_interval])))`).
				Instant(true).Range(false).
				LegendFormat("p99")))))

	d.Panel(elementCoreDNSQPS, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSQPS).
		Title("CoreDNS QPS").
		Description("CoreDNS requests per second, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster) (rate(coredns_dns_requests_total{`+coredns+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementCoreDNSSERVFAILRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSSERVFAILRate).
		Title("CoreDNS SERVFAIL Rate").
		Description("SERVFAIL responses per second from in-cluster CoreDNS, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster) (rate(coredns_dns_responses_total{`+coredns+`,rcode="SERVFAIL"}[$__rate_interval]))`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementCoreDNSCacheHitRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSCacheHitRate).
		Title("CoreDNS Cache Hit Rate").
		Description("CoreDNS cache hit percentage, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Tooltip(tooltipAll).
			Legend(legend).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`100 * sum by (cluster) (rate(coredns_cache_hits_total{`+coredns+`}[$__rate_interval])) / clamp_min(sum by (cluster) (rate(coredns_cache_requests_total{`+coredns+`}[$__rate_interval])), 1e-9)`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementCoreDNSRequestLatencyP99, dashboardv2.NewPanelBuilder().
		Id(dnsPanelCoreDNSRequestLatencyP99).
		Title("CoreDNS Request Latency p99").
		Description("99th percentile CoreDNS request duration, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{`+coredns+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementExternalDnsRegistryErrors1h, dashboardv2.NewPanelBuilder().
		Id(dnsPanelExternalDnsRegistryErrors1h).
		Title("external-dns Registry Errors (1h)").
		Description("Errors talking to the DNS provider (registry) in the last hour, all clusters.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`ceil(sum(increase(external_dns_registry_errors_total{`+extdns+`}[1h]))) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Errors")))))

	d.Panel(elementExternalDnsSourceErrors1h, dashboardv2.NewPanelBuilder().
		Id(dnsPanelExternalDnsSourceErrors1h).
		Title("external-dns Source Errors (1h)").
		Description("Errors reading route sources (HTTPRoute/Service/...) in the last hour, all clusters.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`ceil(sum(increase(external_dns_source_errors_total{`+extdns+`}[1h]))) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Errors")))))

	d.Panel(elementExternalDnsLastSyncAge, dashboardv2.NewPanelBuilder().
		Id(dnsPanelExternalDnsLastSyncAge).
		Title("external-dns Last Sync Age").
		Description("Time since the last successful full sync to the DNS provider, worst cluster.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Thresholds(syncAgeThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Decimals(0)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`max(time() - external_dns_controller_last_sync_timestamp_seconds{`+extdns+`})`).
				Instant(true).Range(false).
				LegendFormat("Age")))))

	d.Panel(elementExternalDnsSyncAge, dashboardv2.NewPanelBuilder().
		Id(dnsPanelExternalDnsSyncAge).
		Title("external-dns Sync Age").
		Description("Seconds since the last provider sync and source reconcile, worst pod per cluster. A steadily climbing line means external-dns has stopped syncing.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// Aggregate pod and instance churn to the cluster label shown in the legend.
			// max reports the stalest replica, matching the summary tile.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`max by (cluster) (time() - external_dns_controller_last_sync_timestamp_seconds{`+extdns+`})`).
				LegendFormat("{{cluster}} sync"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`max by (cluster) (time() - external_dns_controller_last_reconcile_timestamp_seconds{`+extdns+`})`).
				LegendFormat("{{cluster}} reconcile")))))

	d.Panel(elementExternalDnsErrorRate, dashboardv2.NewPanelBuilder().
		Id(dnsPanelExternalDnsErrorRate).
		Title("external-dns Error Rate").
		Description("Registry (provider) and source errors per second, grouped by cluster.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster) (rate(external_dns_registry_errors_total{`+extdns+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} registry"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`sum by (cluster) (rate(external_dns_source_errors_total{`+extdns+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} source")))))

	d.Panel(elementExternalDnsRecords, dashboardv2.NewPanelBuilder().
		Id(dnsPanelExternalDnsRecords).
		Title("external-dns Records").
		Description("Records desired by sources vs records present in the registry, grouped by cluster. The two lines should sit on top of each other; a persistent gap means records are failing to sync. Zone apex records (NS, SOA) are excluded because no source can produce them.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster) (external_dns_source_records{`+extdns+`})`).
				LegendFormat("{{cluster}} source"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				// Exclude apex NS/SOA records that no external-dns source can produce;
				// retain all other record types for future sources.
				Expr(`sum by (cluster) (external_dns_registry_records{`+extdns+`,record_type!~"ns|soa"})`).
				LegendFormat("{{cluster}} registry")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("dnsdist Summary").AutoGridLayout(statGrid(2).
			WithItem(elementDnsdistQPS).
			WithItem(elementDnsdistAvgLatency))).
		Row(dashboardv2.NewRowBuilder().Title("dnsdist Query/Response Rate").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDnsdistQueryResponseRate))).
		Row(dashboardv2.NewRowBuilder().Title("dnsdist Response Codes").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDnsdistResponseCodes))).
		Row(dashboardv2.NewRowBuilder().Title("dnsdist Latency").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDnsdistLatency))).
		Row(dashboardv2.NewRowBuilder().Title("dnsdist Drop Rate").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDnsdistDropRate))).
		Row(dashboardv2.NewRowBuilder().Title("dnsdist Unanswered Queries").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDnsdistUnansweredQueries))).
		Row(dashboardv2.NewRowBuilder().Title("Knot Resolver Summary").AutoGridLayout(statGrid(4).
			WithItem(elementResolverQPS).
			WithItem(elementResolverCacheHitRateCurrent).
			WithItem(elementResolverLatencyP95).
			WithItem(elementResolverMetricsAge))).
		Row(dashboardv2.NewRowBuilder().Title("Knot Resolver Traffic & Performance").AutoGridLayout(chartGrid().
			WithItem(elementResolverQueryRate).
			WithItem(elementResolverCacheHitRate))).
		Row(dashboardv2.NewRowBuilder().Title("Resolver Response Codes").AutoGridLayout(chartGrid().
			WithItem(elementResolverResponseCodes).
			WithItem(elementResolverResponseLatency))).
		Row(dashboardv2.NewRowBuilder().Title("Resolver Cache Usage").AutoGridLayout(chartGrid().
			WithItem(elementResolverCacheUsage).
			WithItem(elementResolverManagerMemory))).
		Row(dashboardv2.NewRowBuilder().Title("Resolver Metrics Freshness").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementResolverMetricsFreshness))).
		Row(dashboardv2.NewRowBuilder().Title("Authoritative DNS Probe Status").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementAuthoritativeDNSProbeStatus))).
		Row(dashboardv2.NewRowBuilder().Title("Authoritative DNS Probe Duration").AutoGridLayout(chartGrid().
			WithItem(elementAuthoritativeDNSProbeDuration).
			WithItem(elementAuthoritativeDNSAvailability))).
		Row(dashboardv2.NewRowBuilder().Title("Failed Authoritative DNS Probes").AutoGridLayout(statGrid(1).
			WithItem(elementFailedAuthoritativeDNSProbes))).
		Row(dashboardv2.NewRowBuilder().Title("Knot Resolver DNSSEC Validation Logs").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDNSSECBogusAndValidationLogs))).
		Row(dashboardv2.NewRowBuilder().Title("pdns-auth Summary").AutoGridLayout(statGrid(2).
			WithItem(elementPdnsAuthQPS).
			WithItem(elementPdnsAuthAvgLatency))).
		Row(dashboardv2.NewRowBuilder().Title("pdns-auth Query Rate").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementPdnsAuthQueryRate))).
		Row(dashboardv2.NewRowBuilder().Title("pdns-auth Response Codes").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementPdnsAuthResponseCodes))).
		Row(dashboardv2.NewRowBuilder().Title("pdns-auth Latency").AutoGridLayout(chartGrid().
			WithItem(elementPdnsAuthLatency).
			WithItem(elementPdnsAuthBackendLatency))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS Summary").AutoGridLayout(statGrid(4).
			WithItem(elementCoreDNSQPSCurrent).
			WithItem(elementCoreDNSTargetsDown).
			WithItem(elementCoreDNSSERVFAILRateCurrent).
			WithItem(elementCoreDNSRequestLatencyP99Current))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS QPS").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCoreDNSQPS))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS SERVFAIL Rate").AutoGridLayout(chartGrid().
			WithItem(elementCoreDNSSERVFAILRate).
			WithItem(elementCoreDNSCacheHitRate))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS Request Latency p99").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCoreDNSRequestLatencyP99))).
		Row(dashboardv2.NewRowBuilder().Title("external-dns Summary").AutoGridLayout(statGrid(3).
			WithItem(elementExternalDnsRegistryErrors1h).
			WithItem(elementExternalDnsSourceErrors1h).
			WithItem(elementExternalDnsLastSyncAge))).
		Row(dashboardv2.NewRowBuilder().Title("external-dns Sync & Records").AutoGridLayout(chartGrid().
			WithItem(elementExternalDnsSyncAge).
			WithItem(elementExternalDnsErrorRate))).
		Row(dashboardv2.NewRowBuilder().Title("external-dns Records").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementExternalDnsRecords)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
