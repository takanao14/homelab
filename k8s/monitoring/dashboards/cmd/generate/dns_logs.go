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
		elementQueryRate                = "query-rate"
		elementUniqueClients            = "unique-clients"
		elementQueryRateByType          = "query-rate-by-type"
		elementResponseCodeDistribution = "response-code-distribution"
		elementNXDOMAINByCategory       = "nxdomain-by-category"
		elementTopQueriedDomainsLast5m  = "top-queried-domains-last-5m"
		elementTopNXDOMAINTimeRange     = "top-nxdomain-time-range"
		elementQueryRateByHost          = "query-rate-by-host"
		elementNXDOMAINRateByHost       = "nxdomain-rate-by-host"
		elementSERVFAILRateByHost       = "servfail-rate-by-host"
		elementDNSQueryLogs             = "dns-query-logs"
	)

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

	d.Panel(elementQueryRate, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelQueryRate).
		Title("Query Rate").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Thresholds(measurementThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum(rate(`+queryJSON+`[$__auto]))`).
				LegendFormat("queries/s")))))

	// Omit Policy Block Rate because proto2 defaults label every query as
	// NXDOMAIN without an actual policy. Omit a single Unexpected NXDOMAIN
	// rate because category lists are incomplete; keep the visible breakdown.
	d.Panel(elementUniqueClients, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelUniqueClients).
		Title("Unique Clients").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// count(sum by ...) counts distinct IPs, not log lines.
				Expr(`count(sum by (network_query_ip) (count_over_time(`+queryJSON+` | network_query_ip != "" [$__range])))`).
				LegendFormat("clients")))))

	d.Panel(elementQueryRateByType, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelQueryRateByType).
		Title("Query Rate by Type").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (dns_qtype) (rate(`+queryJSON+`[$__auto]))`+
					` or sum by (dns_qtype) (count_over_time(`+queryJSON+`[$__range])) * 0`).
				LegendFormat("{{dns_qtype}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementResponseCodeDistribution, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelResponseCodeDistribution).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (dns_rcode) (rate(`+responseJSON+`[$__auto]))`+
					` or sum by (dns_rcode) (count_over_time(`+responseJSON+`[$__range])) * 0`).
				LegendFormat("{{dns_rcode}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementNXDOMAINByCategory, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelNXDOMAINByCategory).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			// Stacked categories must partition NXDOMAIN. Apply specific matches first
			// and subtract only real overlaps from broader suffix patterns.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseDnssd+`" [$__auto])) or vector(0)`).
				LegendFormat("dns-sd discovery"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(lokiQuery().
				Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseGrpclb+`" [$__auto])) or vector(0)`).
				LegendFormat("grpc lb discovery"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(lokiQuery().
				Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseLocal+`" [$__auto])) or vector(0)`).
				LegendFormat("mdns (.local)"))).
			Target(dashboardv2.NewTargetBuilder().RefId("D").Query(lokiQuery().
				Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseArpa+`" | dns_qname!~"`+nxNoiseDnssd+`" [$__auto])) or vector(0)`).
				LegendFormat("reverse lookup (PTR)"))).
			Target(dashboardv2.NewTargetBuilder().RefId("E").Query(lokiQuery().
				Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseWpad+`" [$__auto])) or vector(0)`).
				LegendFormat("wpad"))).
			Target(dashboardv2.NewTargetBuilder().RefId("F").Query(lokiQuery().
				Expr(`sum(rate(`+nxdomainJSON+` | dns_qname=~"`+nxNoiseSuffix+`"`+
					` | dns_qname!~"`+nxNoiseDnssd+`"`+
					` | dns_qname!~"`+nxNoiseGrpclb+`"`+
					` | dns_qname!~"`+nxNoiseWpad+`" [$__auto])) or vector(0)`).
				LegendFormat("search-domain suffix"))).
			Target(dashboardv2.NewTargetBuilder().RefId("G").Query(lokiQuery().
				Expr(`sum(rate(`+nxUnexpected+` [$__auto])) or vector(0)`).
				LegendFormat("unexpected"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementTopQueriedDomainsLast5m, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelTopQueriedDomainsLast5m).
		Title("Top Queried Domains (Last 5m)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("short").
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().
				Values(true).
				Limit(10))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(`+queryJSON+` | dns_qname != "" [5m]))))`).
				Instant(true).
				Range(false).
				LegendFormat("{{dns_qname}}")))))

	d.Panel(elementTopNXDOMAINTimeRange, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelTopNXDOMAINTimeRange).
		Title("Top NXDOMAIN (Time Range)").
		Description("Every NXDOMAIN name, unfiltered, over the full dashboard time range. The composition by category is in NXDOMAIN by Category above; this panel deliberately hides nothing.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("short").
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().
				Values(true).
				Limit(10))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// Keep rankings unfiltered: hardcoded noise lists hid real suffix errors
				// while missing newer benign categories. Drop only empty qnames.
				Expr(`sort_desc(topk(10, sum by (dns_qname) (count_over_time(`+nxdomainJSON+` | dns_qname != "" [$__range]))))`).
				Instant(true).
				Range(false).
				LegendFormat("{{dns_qname}}")))))

	d.Panel(elementQueryRateByHost, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelQueryRateByHost).
		Title("Query Rate by Host").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (identity) (rate(`+queryJSON+`[$__auto]))`+
					` or sum by (identity) (count_over_time(`+queryJSON+`[$__range])) * 0`).
				LegendFormat("{{identity}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementNXDOMAINRateByHost, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelNXDOMAINRateByHost).
		Title("NXDOMAIN Rate by Host").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (identity) (rate(`+responseJSON+` | dns_rcode="NXDOMAIN" [$__auto]))`+
					` or sum by (identity) (count_over_time(`+responseJSON+`[$__range])) * 0`).
				LegendFormat("{{identity}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementSERVFAILRateByHost, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelSERVFAILRateByHost).
		Title("SERVFAIL Rate by Host").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (identity) (rate(`+responseJSON+` | dns_rcode="SERVFAIL" [$__auto]))`+
					` or sum by (identity) (count_over_time(`+responseJSON+`[$__range])) * 0`).
				LegendFormat("{{identity}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementDNSQueryLogs, dashboardv2.NewPanelBuilder().
		Id(dnsLogsPanelDNSQueryLogs).
		Title("DNS Query Logs").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			SortOrder(common.LogsSortOrderDescending).
			EnableLogDetails(true).
			ShowLogContextToggle(true).
			ShowControls(true).
			ShowFieldSelector(true)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// line_format persists displayed fields. Exclude policy-action because its
				// proto2 default claims NXDOMAIN even for successful queries.
				Expr(baseJSON+` | line_format "{{.identity}} {{.dnstap_operation}} {{.network_query_ip}} -> {{.dns_qname}} {{.dns_qtype}} {{.dns_rcode}} latency={{.dnstap_latency_ms}}ms"`).
				MaxLines(500)))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(2).
			WithItem(elementQueryRate).
			WithItem(elementUniqueClients))).
		Row(dashboardv2.NewRowBuilder().Title("Query Trends").AutoGridLayout(chartGrid().
			WithItem(elementQueryRateByType).
			WithItem(elementResponseCodeDistribution))).
		Row(dashboardv2.NewRowBuilder().Title("NXDOMAIN by Category").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementNXDOMAINByCategory))).
		Row(dashboardv2.NewRowBuilder().Title("Top Domains").AutoGridLayout(chartGrid().
			WithItem(elementTopQueriedDomainsLast5m).
			WithItem(elementTopNXDOMAINTimeRange))).
		Row(dashboardv2.NewRowBuilder().Title("By Host").AutoGridLayout(chartGrid().
			WithItem(elementQueryRateByHost).
			WithItem(elementNXDOMAINRateByHost))).
		Row(dashboardv2.NewRowBuilder().Title("SERVFAIL Rate by Host").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementSERVFAILRateByHost))).
		Row(dashboardv2.NewRowBuilder().Title("Logs").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDNSQueryLogs)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
