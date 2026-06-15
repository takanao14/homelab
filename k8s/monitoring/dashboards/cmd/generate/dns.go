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

	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
	legend := common.NewVizLegendOptionsBuilder().
		ShowLegend(true).
		DisplayMode(common.LegendDisplayModeList).
		Placement(common.LegendPlacementBottom)

	// zeroLine draws a solid reference line at y=0 for bidirectional rate panels.
	zeroLineThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "transparent"},
			{Value: float64Ptr(0), Color: "white"},
		})
	zeroLineStyle := common.NewGraphThresholdsStyleConfigBuilder().
		Mode(common.GraphThresholdsStyleModeLine)

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

	issueThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "red"},
		})

	servfailThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(0.001), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("DNS Overview").
		Uid("dns-overview").
		Tags([]string{"dns", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithRow(dashboard.NewRowBuilder("dnsdist")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist QPS").
				Datasource(ds).
				Span(8).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_queries{` + dnsdist + `}[5m]))`).
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
					Expr(`sum(rate(dnsdist_cache_hits{` + dnsdist + `}[5m])) / clamp_min(sum(rate(dnsdist_cache_hits{` + dnsdist + `}[5m]) + rate(dnsdist_cache_misses{` + dnsdist + `}[5m])), 1) * 100`).
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
		WithRow(dashboard.NewRowBuilder("pdns-auth")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("pdns-auth QPS").
				Datasource(ds).
				Span(12).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_udp_queries{` + pdns + `}[5m]))`).
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
					Expr(mapDNS(`rate(dnsdist_queries{`+dnsdist+`}[5m])`)).
					LegendFormat("{{server}} Queries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Responses").
					Expr(mapDNS(`rate(dnsdist_responses{`+dnsdist+`}[5m])`)).
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
					Expr(`sum(rate(dnsdist_frontend_noerror{`+dnsdist+`}[5m]))`).
					LegendFormat("NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_nxdomain{`+dnsdist+`}[5m]))`).
					LegendFormat("NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_servfail{`+dnsdist+`}[5m]))`).
					LegendFormat("SERVFAIL"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_frontend_refused{`+dnsdist+`}[5m]))`).
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
					Expr(mapDNS(`rate(dnsdist_cache_hits{` + dnsdist + `}[5m]) / clamp_min(rate(dnsdist_cache_hits{` + dnsdist + `}[5m]) + rate(dnsdist_cache_misses{` + dnsdist + `}[5m]), 1) * 100`)).
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
					Expr(mapDNS(`rate(dnsdist_acl_drops{` + dnsdist + `}[5m])`)).
					LegendFormat("{{server}} ACL Drop"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_rule_drops{` + dnsdist + `}[5m])`)).
					LegendFormat("{{server}} Rule Drop"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_dynamic_blocked{` + dnsdist + `}[5m])`)).
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
					Expr(mapDNS(`rate(dnsdist_queries{` + dnsdist + `}[5m]) - rate(dnsdist_responses{` + dnsdist + `}[5m])`)).
					LegendFormat("{{server}}"),
				),
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
					Expr(mapDNS(`rate(pdns_auth_udp_queries{` + pdns + `}[5m])`)).
					LegendFormat("{{server}} UDP"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(pdns_auth_tcp_queries{` + pdns + `}[5m])`)).
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
					Expr(`sum(rate(pdns_auth_noerror_packets{`+pdns+`}[5m]))`).
					LegendFormat("NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_nxdomain_packets{`+pdns+`}[5m]))`).
					LegendFormat("NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_servfail_packets{`+pdns+`}[5m]))`).
					LegendFormat("SERVFAIL"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_refused_packets{`+pdns+`}[5m]))`).
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
					Expr(`sum(rate(coredns_dns_requests_total{` + coredns + `}[5m]))`).
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
					Expr(`sum(rate(coredns_dns_responses_total{` + coredns + `,rcode="SERVFAIL"}[5m]))`).
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
					Expr(`histogram_quantile(0.99, sum by (le) (rate(coredns_dns_request_duration_seconds_bucket{` + coredns + `}[5m])))`).
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
					Expr(`sum by (cluster) (rate(coredns_dns_requests_total{` + coredns + `}[5m]))`).
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
					Expr(`sum by (cluster) (rate(coredns_dns_responses_total{` + coredns + `,rcode="SERVFAIL"}[5m]))`).
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
					Expr(`100 * sum by (cluster) (rate(coredns_cache_hits_total{` + coredns + `}[5m])) / clamp_min(sum by (cluster) (rate(coredns_cache_requests_total{` + coredns + `}[5m])), 1e-9)`).
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
					Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{` + coredns + `}[5m])))`).
					LegendFormat("{{cluster}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
