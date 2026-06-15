package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
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
	ds := lokiDatasource()

	const (
		baseJSON     = `{job="dns", host=~"$host"} | json | __error__=""`
		queryJSON    = `{job="dns", host=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_QUERY"`
		responseJSON = `{job="dns", host=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_RESPONSE"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	d, err := dashboard.NewDashboardBuilder("DNS Query Logs").
		Uid("dns-logs").
		Tags([]string{"dns", "logs", "infrastructure"}).
		Timezone("browser").
		Time("now-3h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			lokiDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("host").
				Label("Host").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values({job="dns"}, host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Query Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Min(0).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + queryJSON + `[5m]))`).
					LegendFormat("queries/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("NXDOMAIN Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Min(0).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + responseJSON + ` | dns_rcode="NXDOMAIN" [5m]))`).
					LegendFormat("nxdomain/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Policy Block Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Min(0).
				WithTarget(loki.NewDataqueryBuilder().
					// dnstap.policy-action reflects dnsdist policy decisions (NXDOMAIN, DROP, etc.)
					Expr(`sum(rate(` + queryJSON + ` | dnstap_policy_action!="" | dnstap_policy_action!="PASSTHRU" [5m])) or vector(0)`).
					LegendFormat("blocked/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unique Clients").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				WithTarget(loki.NewDataqueryBuilder().
					// count(sum by ...) counts distinct IPs, not log lines.
					Expr(`count(sum by (network_query_ip) (count_over_time(` + queryJSON + ` | network_query_ip != "" [$__range])))`).
					LegendFormat("clients"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Query Trends")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Query Rate by Type").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (dns_qtype) (rate(` + queryJSON + `[5m]))`).
					LegendFormat("{{dns_qtype}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Response Code Distribution").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (dns_rcode) (rate(`+responseJSON+`[5m]))`).
					LegendFormat("{{dns_rcode}}"),
				).
				// Semantic coloring consistent with DNS Overview dashboard.
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
		WithRow(dashboard.NewRowBuilder("Top Domains")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Top Queried Domains (Last 5m)").
				Datasource(ds).
				Span(12).Height(10).
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().
					Values(true).
					Limit(10)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(` + queryJSON + ` | dns_qname != "" [5m]))))`).
					Instant(true).
					Range(false).
					LegendFormat("{{dns_qname}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Top NXDOMAIN Queries (Last 5m)").
				Datasource(ds).
				Span(12).Height(10).
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().
					Values(true).
					Limit(10)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(` + responseJSON + ` | dns_qname != "" | dns_rcode="NXDOMAIN" [5m]))))`).
					Instant(true).
					Range(false).
					LegendFormat("{{dns_qname}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("By Host")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Query Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + queryJSON + `[5m]))`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("NXDOMAIN Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + responseJSON + ` | dns_rcode="NXDOMAIN" [5m]))`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("SERVFAIL Rate by Host").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + responseJSON + ` | dns_rcode="SERVFAIL" [5m])) or sum by (host) (rate(` + responseJSON + `[5m])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("DNS Query Logs").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				SortOrder(common.LogsSortOrderDescending).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(baseJSON + ` | line_format "{{.host}} {{.dnstap_operation}} {{.network_query_ip}} -> {{.dns_qname}} {{.dns_qtype}} {{.dns_rcode}} policy={{.dnstap_policy_action}} latency={{.dnstap_latency_ms}}ms"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
