package main

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDnsOverview defines the DNS infrastructure dashboard.
//   - dist1/dist2: 192.168.10.231/232 (dnsdist)
//   - ns1: 192.168.10.233 (primary)
//   - ns2: 192.168.10.234 (secondary)
//   - ns3: 192.168.10.235 (secondary)
func buildDnsOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		dnsdist = `job="scrapeConfig/monitoring/dnsdist-external"`
		pdns    = `job="scrapeConfig/monitoring/pdns-auth-external"`
		coredns = `job="coredns"`
		extdns  = `job="external-dns"`
	)

	// mapDNS maps instance IPs to logical ns1/ns2 names for better readability.
	mapDNS := func(expr string) string {
		replacements := []struct {
			server   string
			instance string
		}{
			{"old-ns1", "192.168.10.242:.*"},
			{"old-ns2", "192.168.10.241:.*"},
			{"dist1", "192.168.10.231:.*"},
			{"dist2", "192.168.10.232:.*"},
			{"ns1", "192.168.10.233:.*"},
			{"ns2", "192.168.10.234:.*"},
			{"ns3", "192.168.10.235:.*"},
		}

		for _, r := range replacements {
			expr = fmt.Sprintf(`label_replace(%s, "server", "%s", "instance", "%s")`, expr, r.server, r.instance)
		}

		return expr
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// zeroLine draws a solid reference line at y=0 for bidirectional rate panels.
	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// Latency thresholds in microseconds (50ms = warning, 150ms = critical).
	latencyThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green", Value: float64Ptr(0)},
			{Color: "yellow", Value: float64Ptr(50000)},
			{Color: "red", Value: float64Ptr(150000)},
		})

	corednsLatencyThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green", Value: float64Ptr(0)},
			{Color: "yellow", Value: float64Ptr(0.05)},
			{Color: "red", Value: float64Ptr(0.15)},
		})

	issueThresholds := issueThresholds()

	servfailThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(0.001), Color: "red"},
		})

	// external-dns syncs every minute; warn once a sync is a few intervals
	// late, alert red when it has been stuck for 15 minutes.
	syncAgeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(300), Color: "yellow"},
			{Value: float64Ptr(900), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("DNS Overview").
		Uid("dns-overview").
		Tags([]string{"dns", "infrastructure"}).
		Timezone("browser").
		Time("now-30d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithRow(dashboard.NewRowBuilder("dnsdist")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist QPS").
				Datasource(ds).
				Span(8).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_queries{` + dnsdist + `}[$__rate_interval]))`).
					LegendFormat("QPS"),
				).Decimals(1),
		).
		// clamp_min prevents division by zero when there are no queries yet.
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist Cache Hit Rate").
				Datasource(ds).
				Span(8).Height(4).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_cache_hits{` + dnsdist + `}[$__rate_interval])) / clamp_min(sum(rate(dnsdist_cache_hits{` + dnsdist + `}[$__rate_interval]) + rate(dnsdist_cache_misses{` + dnsdist + `}[$__rate_interval])), 1) * 100`).
					LegendFormat("Cache Hit Rate"),
				).Decimals(2),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist Avg Latency").
				Datasource(ds).
				Span(8).Height(4).
				Unit("µs").
				Thresholds(latencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(dnsdist_latency_avg100{` + dnsdist + `})`).
					LegendFormat("Avg Latency"),
				).Decimals(1),
		).
		WithRow(dashboard.NewRowBuilder("dnsdist Metrics")).
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
					Expr(mapDNS(`rate(dnsdist_queries{`+dnsdist+`}[$__rate_interval])`)).
					LegendFormat("{{server}} Queries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Responses").
					Expr(mapDNS(`rate(dnsdist_responses{`+dnsdist+`}[$__rate_interval])`)).
					LegendFormat("{{server}} Responses"),
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
				Span(12).Height(8).
				Unit("µs").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(latencyThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`dnsdist_latency_avg100{`+dnsdist+`}`)).
					LegendFormat("{{server}} avg100"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`dnsdist_latency_avg1000{`+dnsdist+`}`)).
					LegendFormat("{{server}} avg1000"),
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
				Title("dnsdist Cache Hit Rate").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_cache_hits{` + dnsdist + `}[$__rate_interval]) / clamp_min(rate(dnsdist_cache_hits{` + dnsdist + `}[$__rate_interval]) + rate(dnsdist_cache_misses{` + dnsdist + `}[$__rate_interval]), 1) * 100`)).
					LegendFormat("{{server}}"),
				),
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
					Expr(mapDNS(`rate(dnsdist_acl_drops{` + dnsdist + `}[$__rate_interval])`)).
					LegendFormat("{{server}} ACL Drop"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_rule_drops{` + dnsdist + `}[$__rate_interval])`)).
					LegendFormat("{{server}} Rule Drop"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_dynamic_blocked{` + dnsdist + `}[$__rate_interval])`)).
					LegendFormat("{{server}} Dynamic Block"),
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
					Expr(mapDNS(`rate(dnsdist_queries{` + dnsdist + `}[$__rate_interval]) - rate(dnsdist_responses{` + dnsdist + `}[$__rate_interval])`)).
					LegendFormat("{{server}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("pdns-auth")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("pdns-auth QPS").
				Datasource(ds).
				Span(12).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_udp_queries{` + pdns + `}[$__rate_interval]))`).
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
		WithRow(dashboard.NewRowBuilder("pdns-auth Metrics")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(pdns_auth_udp_queries{` + pdns + `}[$__rate_interval])`)).
					LegendFormat("{{server}} UDP"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(pdns_auth_tcp_queries{` + pdns + `}[$__rate_interval])`)).
					LegendFormat("{{server}} TCP"),
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
					Expr(mapDNS(`pdns_auth_latency{` + pdns + `}`)).
					LegendFormat("{{server}}"),
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
					Expr(mapDNS(`pdns_auth_backend_latency{` + pdns + `}`)).
					LegendFormat("{{server}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("CoreDNS Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS QPS").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
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
		WithRow(dashboard.NewRowBuilder("CoreDNS Metrics")).
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
		WithRow(dashboard.NewRowBuilder("external-dns Metrics")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("external-dns Sync Age").
				Description("Seconds since the last provider sync and source reconcile, grouped by cluster. A steadily climbing line means external-dns has stopped syncing.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`time() - external_dns_controller_last_sync_timestamp_seconds{` + extdns + `}`).
					LegendFormat("{{cluster}} sync"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`time() - external_dns_controller_last_reconcile_timestamp_seconds{` + extdns + `}`).
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
				Description("Records desired by sources vs records present in the registry, grouped by cluster. A persistent gap means records are failing to sync.").
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
					Expr(`sum by (cluster) (external_dns_registry_records{` + extdns + `})`).
					LegendFormat("{{cluster}} registry"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
