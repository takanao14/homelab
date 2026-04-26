package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDnsOverview defines the DNS infrastructure dashboard.
// Two-server setup (192.168.10.241/242).
//   - ns1: 192.168.10.242 (primary)
//   - ns2: 192.168.10.241 (secondary)
func buildDnsOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		dnsdist = `job="scrapeConfig/monitoring/dnsdist-external"`
		pdns    = `job="scrapeConfig/monitoring/pdns-auth-external"`
	)

	// mapDNS maps instance IPs to logical ns1/ns2 names for better readability.
	mapDNS := func(expr string) string {
		return `label_replace(label_replace(` + expr + `, "server", "ns1", "instance", "192.168.10.242:.*"), "server", "ns2", "instance", "192.168.10.241:.*")`
	}

	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)

	// Latency thresholds in microseconds (50ms = warning, 150ms = critical).
	latencyThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Color: "green", Value: float64Ptr(0)},
			{Color: "yellow", Value: float64Ptr(50000)},
			{Color: "red", Value: float64Ptr(150000)},
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
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_queries{` + dnsdist + `}[5m])`)).
					LegendFormat("{{server}} Queries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`rate(dnsdist_responses{` + dnsdist + `}[5m])`)).
					LegendFormat("{{server}} Responses"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Response Codes").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
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
					{Id: "custom.lineStyle", Value: map[string]any{"dash": []int{4, 4}, "fill": "dash"}},
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
		WithRow(dashboard.NewRowBuilder("pdns-auth Metrics")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
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
				Thresholds(latencyThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(mapDNS(`pdns_auth_backend_latency{` + pdns + `}`)).
					LegendFormat("{{server}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
