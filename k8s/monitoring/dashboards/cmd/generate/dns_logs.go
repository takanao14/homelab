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
//
// rate() windows use $__auto, the Loki counterpart of $__rate_interval, so the
// window tracks the zoom level. A fixed [5m] is fine at the 3h default, where
// the step is around 15s, but once the range is widened past roughly half a day
// the step overtakes the window and Loki only ever looks at 5 minutes out of
// each step -- the data in between is never read, and short spikes vanish.
// count_over_time in "Top Queried Domains (Last 5m)" keeps its fixed window on
// purpose: there the five minutes is the quantity the panel is named for.
//
// Two things follow from these series being counts of discrete events rather
// than samples of a counter, and every timeseries below applies both.
//
// A LogQL rate() over a window containing no matching line yields no sample at
// all, where the Prometheus equivalent would still report 0. Zoomed in to a
// two-second window, NOERROR had a point at all 61 steps while NXDOMAIN had 28:
// the missing 33 are absences, not zeroes. Grafana then draws the line straight
// across them and, worse for the stacked panels, stacks series whose timestamps
// do not line up. Each by-label query therefore adds
// `or sum by (...) (count_over_time(...[$__range])) * 0`, which enumerates every
// label value present anywhere in the view and pins it to zero, so a series that
// falls silent reads as zero instead of vanishing. The same shape was already
// used for SERVFAIL by host; it is now consistent and keyed on $__range rather
// than the current window, which is itself empty when a host goes quiet.
//
// Interval("1m") floors $__auto. Left alone it follows the zoom down to about a
// second, where roughly three queries per second quantises the rate into a
// staircase of 0, 0.5 and 1 that says more about the window than the traffic.
func buildDnsLogs() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()

	const (
		baseJSON     = `{job="dns", host=~"$host"} | json | __error__=""`
		queryJSON    = `{job="dns", host=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_QUERY"`
		responseJSON = `{job="dns", host=~"$host"} | json | __error__="" | dnstap_operation="CLIENT_RESPONSE"`
		nxdomainJSON = responseJSON + ` | dns_rcode="NXDOMAIN"`

		// Known-benign NXDOMAIN categories (LogQL regexes are fully anchored):
		// reverse lookups for unregistered PTRs, Windows WPAD probes, unicast
		// DNS-SD discovery, mDNS .local names leaking to the unicast resolver,
		// gRPC load-balancer SRV probes, and search-domain suffixing of external
		// names (k8s ndots:5 pods and DHCP clients appending home.butaco.net).
		//
		// mDNS and gRPC were added after they turned out to be the two largest
		// sources by volume while being in no category at all: over 3h .local
		// accounted for 2162 lookups and _grpclb._tcp for another 2231.
		nxNoiseArpa   = `.+\\.arpa`
		nxNoiseWpad   = `wpad\\..*`
		nxNoiseDnssd  = `.*\\._dns-sd\\._udp\\..*`
		nxNoiseLocal  = `(.+\\.)?local`
		nxNoiseGrpclb = `_grpclb\\._tcp\\..*`
		nxNoiseSuffix = `.+\\..+\\.home\\.butaco\\.net`

		// nxUnexpected is NXDOMAIN with every known-benign category removed;
		// what remains (typos, stale configs, suspicious lookups) is the signal.
		// It drives only the category breakdown: the panels that rank names show
		// everything, because an exclusion list nobody can see is a bad way to
		// decide what an operator is allowed to notice.
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
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + queryJSON + `[$__auto]))`).
					LegendFormat("queries/s"),
				),
		).
		// Two tiles that used to sit here are gone, both because they stated a
		// conclusion the data did not support.
		//
		// "Policy Block Rate" filtered on dnstap.policy-action, which looks like
		// a decision and is not one. dnstap.proto is proto2, where an unset
		// optional enum reads back as its first declared value, and
		// Policy.Action declares NXDOMAIN first; dnsdist runs no RPZ so it never
		// populates the Policy message, and dnscollector reads GetAction()
		// without checking presence. Every line therefore carries
		// policy-action="NXDOMAIN", the filter matched 100% of queries, and the
		// tile reprinted the query rate -- 2.12/s against a query rate of
		// 2.12/s. The sibling fields prove the mechanism: policy-match, also an
		// enum, is uniformly "QNAME", its own first value, while the string
		// fields (policy-type, -rule, -value) are empty. Bring this back only
		// with a real policy, keyed on policy-rule, which stays empty until a
		// rule actually matches.
		//
		// "Unexpected NXDOMAIN Rate" reduced a judgement to one number, which
		// needs the exclusion list to be complete -- and it was not: mDNS and
		// gRPC discovery, the two largest sources by volume, were in no category
		// at all and were being counted as unexpected. That list is defensible
		// as a *breakdown*, where a miscategorised name lands in the wrong
		// colour and stays on screen, but not as a single figure asserting how
		// much is wrong. NXDOMAIN by Category carries that judgement now, and
		// Response Code Distribution the total.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unique Clients").
				Datasource(ds).
				Span(12).Height(4).
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
				// The series stack, so the categories have to partition NXDOMAIN
				// rather than merely describe it. Precedence runs most specific
				// first, and each target subtracts the categories above it that
				// it can actually overlap with. Only real overlaps are excluded:
				// search-domain suffixing is the broad one, and it genuinely
				// collides with the others -- over 3h, 1558 names matched both
				// _grpclb._tcp and the suffix pattern.
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
					// Unfiltered on purpose. This panel used to subtract the
					// nxUnexpected noise list, which turned out to be wrong in
					// both directions: over 3h it still ranked mDNS .local
					// lookups first and second (1045 and 1043) and ArgoCD's
					// _grpclb._tcp SRV probes third (673), because the list knows
					// neither category -- while the suffix pattern hid
					// openbao.home.butaco.net.home.butaco.net (197), a genuine
					// double-suffixing misconfiguration, which is exactly what
					// the panel was supposed to surface. A hardcoded exclusion
					// list ages against the environment, and being invisible in
					// the UI, nobody can tell what it swallowed.
					//
					// dns_qname != "" stays: it drops records with no name at
					// all, which would render as an unlabelled bar, and matched
					// nothing over 3h and 52k lines. It hides no domain.
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
					// line_format is what picks the displayed fields: the logs
					// panel has no displayedFields option in the schema, and
					// ShowFieldSelector only lets a viewer choose fields for the
					// session, with nothing persisted back to the dashboard.
					//
					// policy={{.dnstap_policy_action}} used to be part of this
					// line and is gone for the reason given above the Summary
					// row: the field is a proto2 default, not a decision, so
					// every line read policy=NXDOMAIN -- including the NOERROR
					// ones, where it flatly contradicts the rcode beside it.
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
