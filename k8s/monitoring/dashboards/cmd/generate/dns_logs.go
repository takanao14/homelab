package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	dnsLogsPanelQueryRate                = 1
	dnsLogsPanelUniqueClients            = 2
	dnsLogsPanelQueryRateByType          = 3
	dnsLogsPanelResponseCodeDistribution = 4
	dnsLogsPanelNXDOMAINByCategory       = 5
	dnsLogsPanelTopQueriedDomainsLast5m  = 6
	dnsLogsPanelTopNXDOMAINTimeRange     = 7
	dnsLogsPanelQueryRateByHost          = 8
	dnsLogsPanelNXDOMAINRateByHost       = 9
	dnsLogsPanelSERVFAILRateByHost       = 10
	dnsLogsPanelDNSQueryLogs             = 11
)

// buildDnsLogs covers dnscollector/dnsdist JSON logs in Loki.
// Use $__auto with a one-minute floor for zoom-dependent windows. Add zero
// baselines from unfiltered selectors because quiet LogQL series disappear.
// Fixed windows remain only where the panel explicitly reports that period.
func buildDnsLogs() (*dashboardv2.Dashboard, error) {

	const (
		baseJSON     = `{job="dns", identity=~"$host"} | json | __error__=""`
		queryJSON    = `{job="dns", identity=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_QUERY"`
		responseJSON = `{job="dns", identity=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_RESPONSE"`
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

	d := dashboardv2.NewDashboardBuilder("DNS Query Logs").
		Tags([]string{"dns", "logs", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-3h").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(lokiDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("host").
			Label("Host").
			Query(lokiVariableQuery(`label_values({job="dns"}, identity)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	return newLayout(d).
		Row("Summary", statGrid(2)).
		Panel(dnsLogsPanelQueryRate, dashboardv2.NewPanelBuilder().
			Title("Query Rate").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Thresholds(measurementThresholds())).
			Data(queries(lokiQuery().
				Expr(`sum(rate(`+queryJSON+`[$__auto]))`).
				LegendFormat("queries/s")))).
		// Omit Policy Block Rate because proto2 defaults label every query as
		// NXDOMAIN without an actual policy. Omit a single Unexpected NXDOMAIN
		// rate because category lists are incomplete; keep the visible breakdown.
		Panel(dnsLogsPanelUniqueClients, dashboardv2.NewPanelBuilder().
			Title("Unique Clients").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds())).
			Data(queries(lokiQuery().
				// count(sum by ...) counts distinct IPs, not log lines.
				Expr(`count(sum by (network_query_ip) (count_over_time(`+queryJSON+` | network_query_ip != "" [$__range])))`).
				LegendFormat("clients")))).
		Row("Query Trends", chartGrid()).
		Panel(dnsLogsPanelQueryRateByType, dashboardv2.NewPanelBuilder().
			Title("Query Rate by Type").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
			Data(queries(lokiQuery().
				Expr(`sum by (dns_qtype) (rate(`+queryJSON+`[$__auto]))`+
					` or sum by (dns_qtype) (count_over_time(`+queryJSON+`[$__range])) * 0`).
				LegendFormat("{{dns_qtype}}")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")))).
		Panel(dnsLogsPanelResponseCodeDistribution, dashboardv2.NewPanelBuilder().
			Title("Response Code Distribution").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				// Semantic coloring consistent with DNS Overview dashboard.
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
			Data(queries(lokiQuery().
				Expr(`sum by (dns_rcode) (rate(`+responseJSON+`[$__auto]))`+
					` or sum by (dns_rcode) (count_over_time(`+responseJSON+`[$__range])) * 0`).
				LegendFormat("{{dns_rcode}}")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")))).
		Row("NXDOMAIN by Category", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(dnsLogsPanelNXDOMAINByCategory, dashboardv2.NewPanelBuilder().
			Title("NXDOMAIN by Category").
			Description("Known-benign noise categories vs. unexpected NXDOMAIN. Only the unexpected series is worth investigating.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Override(dashboardv2.MatcherConfig{Id: "byName", Options: "unexpected"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
					})).
			Data(queries( // Stacked categories must partition NXDOMAIN. Apply specific matches first
				// and subtract only real overlaps from broader suffix patterns.
				lokiQuery().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseDnssd+`" [$__auto])) or vector(0)`).
					LegendFormat("dns-sd discovery"),
				lokiQuery().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseGrpclb+`" [$__auto])) or vector(0)`).
					LegendFormat("grpc lb discovery"),
				lokiQuery().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseLocal+`" [$__auto])) or vector(0)`).
					LegendFormat("mdns (.local)"),
				lokiQuery().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseArpa+`" | dns_qname!~"`+nxNoiseDnssd+`" [$__auto])) or vector(0)`).
					LegendFormat("reverse lookup (PTR)"),
				lokiQuery().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseWpad+`" [$__auto])) or vector(0)`).
					LegendFormat("wpad"),
				lokiQuery().
					Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseSuffix+`"`+
						` | dns_qname!~"`+nxNoiseDnssd+`"`+
						` | dns_qname!~"`+nxNoiseGrpclb+`"`+
						` | dns_qname!~"`+nxNoiseWpad+`" [$__auto])) or vector(0)`).
					LegendFormat("search-domain suffix"),
				lokiQuery().
					Expr(`sum(rate(`+nxUnexpected+` [$__auto])) or vector(0)`).
					LegendFormat("unexpected")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")))).
		Row("Top Domains", chartGrid()).
		Panel(dnsLogsPanelTopQueriedDomainsLast5m, dashboardv2.NewPanelBuilder().
			Title("Top Queried Domains (Last 5m)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().
					Values(true).
					Limit(10))).
			Data(queries(lokiQuery().
				Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(`+queryJSON+` | dns_qname != "" [5m]))))`).
				Instant(true).
				Range(false).
				LegendFormat("{{dns_qname}}")))).
		Panel(dnsLogsPanelTopNXDOMAINTimeRange, dashboardv2.NewPanelBuilder().
			Title("Top NXDOMAIN (Time Range)").
			Description("Every NXDOMAIN name, unfiltered, over the full dashboard time range. The composition by category is in NXDOMAIN by Category above; this panel deliberately hides nothing.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().
					Values(true).
					Limit(10))).
			Data(queries(lokiQuery().
				// Keep rankings unfiltered: hardcoded noise lists hid real suffix errors
				// while missing newer benign categories. Drop only empty qnames.
				Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(`+nxdomainJSON+` | dns_qname != "" [$__range]))))`).
				Instant(true).
				Range(false).
				LegendFormat("{{dns_qname}}")))).
		Row("By Host", chartGrid()).
		Panel(dnsLogsPanelQueryRateByHost, dashboardv2.NewPanelBuilder().
			Title("Query Rate by Host").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(lokiQuery().
				Expr(`sum by (identity) (rate(`+queryJSON+`[$__auto]))`+
					` or sum by (identity) (count_over_time(`+queryJSON+`[$__range])) * 0`).
				LegendFormat("{{identity}}")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")))).
		Panel(dnsLogsPanelNXDOMAINRateByHost, dashboardv2.NewPanelBuilder().
			Title("NXDOMAIN Rate by Host").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(lokiQuery().
				Expr(`sum by (identity) (rate(`+responseJSON+` | dns_rcode="NXDOMAIN" [$__auto]))`+
					` or sum by (identity) (count_over_time(`+responseJSON+`[$__range])) * 0`).
				LegendFormat("{{identity}}")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")))).
		Row("SERVFAIL Rate by Host", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(dnsLogsPanelSERVFAILRateByHost, dashboardv2.NewPanelBuilder().
			Title("SERVFAIL Rate by Host").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(lokiQuery().
				Expr(`sum by (identity) (rate(`+responseJSON+` | dns_rcode="SERVFAIL" [$__auto]))`+
					` or sum by (identity) (count_over_time(`+responseJSON+`[$__range])) * 0`).
				LegendFormat("{{identity}}")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")))).
		Row("Logs", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(dnsLogsPanelDNSQueryLogs, dashboardv2.NewPanelBuilder().
			Title("DNS Query Logs").
			Visualization(logs.NewVisualizationV2Builder().
				ShowTime(true).
				SortOrder(common.LogsSortOrderDescending).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true)).
			Data(queries(lokiQuery().
				// line_format persists displayed fields. Exclude policy-action because its
				// proto2 default claims NXDOMAIN even for successful queries.
				Expr(baseJSON+` | line_format "{{.identity}} {{.dnstap_operation}} {{.network_query_ip}} -> {{.dns_qname}} {{.dns_qtype}} {{.dns_rcode}} latency={{.dnstap_latency_ms}}ms"`).
				MaxLines(500)))).
		Build()
}
