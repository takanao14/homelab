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

// buildDnsLogs covers dnscollector/dnsdist JSON logs in Loki.
// Use $__auto with a one-minute floor for zoom-dependent windows. Add zero
// baselines from unfiltered selectors because quiet LogQL series disappear.
// Fixed windows remain only where the panel explicitly reports that period.
func buildDnsLogs() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()

	const (
		baseJSON     = `{job="dns", host=~"$host"} | json | __error__=""`
		queryJSON    = `{job="dns", host=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_QUERY"`
		responseJSON = `{job="dns", host=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_RESPONSE"`
		nxdomainJSON = responseJSON + ` | dns_rcode="NXDOMAIN"`

		// Categorize common PTR, WPAD, DNS-SD, mDNS, gRPC, and search-suffix
		// NXDOMAIN noise. LogQL regexes are fully anchored.
		nxNoiseArpa   = `.+\\.arpa`
		nxNoiseWpad   = `wpad\\..*`
		nxNoiseDnssd  = `.*\\._dns-sd\\._udp\\..*`
		nxNoiseLocal  = `(.+\\.)?local`
		nxNoiseGrpclb = `_grpclb\\._tcp\\..*`
		nxNoiseSuffix = `.+\\..+\\.home\\.butaco\\.net`

		// nxUnexpected drives only the visible category breakdown. Ranking panels
		// remain unfiltered so an incomplete exclusion list cannot hide names.
		nxUnexpected = nxdomainJSON +
			` | dns_qname!~"` + nxNoiseArpa + `"` +
			` | dns_qname!~"` + nxNoiseWpad + `"` +
			` | dns_qname!~"` + nxNoiseDnssd + `"` +
			` | dns_qname!~"` + nxNoiseLocal + `"` +
			` | dns_qname!~"` + nxNoiseGrpclb + `"` +
			` | dns_qname!~"` + nxNoiseSuffix + `"`
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
				Query(dashboard.StringOrMap{String: new(`label_values({job="dns"}, host)`)}).
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
				Span(12).Height(4).
				Unit("reqps").
				Min(0).
				Thresholds(measurementThresholds()).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + queryJSON + `[$__auto]))`).
					LegendFormat("queries/s"),
				),
		).
		// Omit Policy Block Rate because proto2 defaults label every query as
		// NXDOMAIN without an actual policy. Omit a single Unexpected NXDOMAIN
		// rate because category lists are incomplete; keep the visible breakdown.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unique Clients").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
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
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (dns_qtype) (rate(` + queryJSON + `[$__auto]))` +
						` or sum by (dns_qtype) (count_over_time(` + queryJSON + `[$__range])) * 0`).
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
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (dns_rcode) (rate(`+responseJSON+`[$__auto]))`+
						` or sum by (dns_rcode) (count_over_time(`+responseJSON+`[$__range])) * 0`).
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
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("NXDOMAIN by Category").
				Description("Known-benign noise categories vs. unexpected NXDOMAIN. Only the unexpected series is worth investigating.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Min(0).
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				// Stacked categories must partition NXDOMAIN. Apply specific matches first
				// and subtract only real overlaps from broader suffix patterns.
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseDnssd+`" [$__auto])) or vector(0)`).
					LegendFormat("dns-sd discovery"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseGrpclb+`" [$__auto])) or vector(0)`).
					LegendFormat("grpc lb discovery"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseLocal+`" [$__auto])) or vector(0)`).
					LegendFormat("mdns (.local)"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseArpa+`" | dns_qname!~"`+nxNoiseDnssd+`" [$__auto])) or vector(0)`).
					LegendFormat("reverse lookup (PTR)"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseWpad+`" [$__auto])) or vector(0)`).
					LegendFormat("wpad"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseSuffix+`"`+
						` | dns_qname!~"`+nxNoiseDnssd+`"`+
						` | dns_qname!~"`+nxNoiseGrpclb+`"`+
						` | dns_qname!~"`+nxNoiseWpad+`" [$__auto])) or vector(0)`).
					LegendFormat("search-domain suffix"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(`+nxUnexpected+` [$__auto])) or vector(0)`).
					LegendFormat("unexpected"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "unexpected"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
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
				Title("Top NXDOMAIN (Time Range)").
				Description("Every NXDOMAIN name, unfiltered, over the full dashboard time range. The composition by category is in NXDOMAIN by Category above; this panel deliberately hides nothing.").
				Datasource(ds).
				Span(12).Height(10).
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().
					Values(true).
					Limit(10)).
				WithTarget(loki.NewDataqueryBuilder().
					// Keep rankings unfiltered: hardcoded noise lists hid real suffix errors
					// while missing newer benign categories. Drop only empty qnames.
					Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(` + nxdomainJSON + ` | dns_qname != "" [$__range]))))`).
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
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + queryJSON + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + queryJSON + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("NXDOMAIN Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + responseJSON + ` | dns_rcode="NXDOMAIN" [$__auto]))` +
						` or sum by (host) (count_over_time(` + responseJSON + `[$__range])) * 0`).
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
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + responseJSON + ` | dns_rcode="SERVFAIL" [$__auto]))` +
						` or sum by (host) (count_over_time(` + responseJSON + `[$__range])) * 0`).
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
					// line_format persists displayed fields. Exclude policy-action because its
					// proto2 default claims NXDOMAIN even for successful queries.
					Expr(baseJSON + ` | line_format "{{.host}} {{.dnstap_operation}} {{.network_query_ip}} -> {{.dns_qname}} {{.dns_qtype}} {{.dns_rcode}} latency={{.dnstap_latency_ms}}ms"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
