package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDnsLogs defines the DNS query log dashboard backed by Loki.
// Logs are JSON from dnscollector/dnsdist; field names use dots which LogQL normalizes to underscores.
func buildDnsLogs() (*dashboard.Dashboard, error) {
	dsType := "loki"
	dsUID := "$datasource"
	ds := common.DataSourceRef{Type: &dsType, Uid: &dsUID}

	const (
		base     = `{job="dns", host=~"$host"}`
		baseJSON = `{job="dns", host=~"$host"} | json | __error__=""`
	)

	d, err := dashboard.NewDashboardBuilder("DNS Query Logs").
		Uid("dns-logs").
		Tags([]string{"dns", "logs", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("loki"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("host").
				Label("Host").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		// Row 1: Summary stats
		WithPanel(
			stat.NewPanelBuilder().
				Title("Query Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + base + `[5m]))`).
					LegendFormat("queries/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("NXDOMAIN Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + baseJSON + ` | dns_rcode="NXDOMAIN" [5m]))`).
					LegendFormat("nxdomain/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Policy Block Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					// dnstap.policy-action reflects dnsdist policy decisions (NXDOMAIN, DROP, etc.)
					Expr(`sum(rate(` + baseJSON + ` | dnstap_policy__action!="" | dnstap_policy__action!="PASSTHRU" [5m]))`).
					LegendFormat("blocked/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unique Clients").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`count_over_time(` + baseJSON + ` | network_query_ip != "" [1h])`).
					LegendFormat("clients"),
				),
		).

		// Row 2: Query rate trends
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Query Rate by Type").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (dns_qtype) (rate(` + baseJSON + `[5m]))`).
					LegendFormat("{{dns_qtype}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Response Code Distribution").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (dns_rcode) (rate(` + baseJSON + `[5m]))`).
					LegendFormat("{{dns_rcode}}"),
				),
		).

		// Row 3: Query rate by host + NXDOMAIN breakdown
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Query Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + base + `[5m]))`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("NXDOMAIN Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + baseJSON + ` | dns_rcode="NXDOMAIN" [5m]))`).
					LegendFormat("{{host}}"),
				),
		).

		// Row 4: Log browser
		WithPanel(
			logs.NewPanelBuilder().
				Title("DNS Query Logs").
				Datasource(ds).
				Span(24).Height(12).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(baseJSON).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
