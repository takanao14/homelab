package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDnsOverview defines the DNS infrastructure dashboard.
// Two-server setup (192.168.10.241/242); no variables needed — {{instance}} distinguishes them.
//   - dnsdist  : DNS frontend / load balancer (port 8083)
//   - pdns-auth: authoritative DNS server (port 8081)
func buildDnsOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		dnsdist = `job="scrapeConfig/monitoring/dnsdist-external"`
		pdns    = `job="scrapeConfig/monitoring/pdns-auth-external"`
	)

	d, err := dashboard.NewDashboardBuilder("DNS Overview").
		Uid("dns-overview").
		Tags([]string{"dns", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist QPS").
				Datasource(ds).
				Span(6).Height(4).
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
				Span(6).Height(4).
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
				Span(6).Height(4).
				Unit("µs").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(dnsdist_latency_avg100{` + dnsdist + `})`).
					LegendFormat("Avg Latency"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("pdns-auth QPS").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_udp_queries{` + pdns + `}[5m]))`).
					LegendFormat("QPS"),
				).Decimals(1),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_queries{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Queries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_responses{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Responses"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Response Codes").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_frontend_noerror{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_frontend_nxdomain{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_frontend_servfail{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} SERVFAIL"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("µs").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dnsdist_latency_avg100{` + dnsdist + `}`).
					LegendFormat("{{instance}} avg100"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dnsdist_latency_avg1000{` + dnsdist + `}`).
					LegendFormat("{{instance}} avg1000"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Cache").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_cache_hits{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Hits"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_cache_misses{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Misses"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_udp_queries{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} UDP"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_tcp_queries{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} TCP"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Response Codes").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_noerror_packets{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_nxdomain_packets{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_servfail_packets{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} SERVFAIL"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("µs").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pdns_auth_latency{` + pdns + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
