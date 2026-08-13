package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildSyslog covers RFC 5424 network-device and Proxmox logs in Loki.
// Follow dns_logs window and baseline conventions. Fixed KPI windows remain
// literal; volume uses rates while sparse errors/warnings use event counts.
func buildSyslog() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		// Severity distinguishes syslog from other shared-host Loki streams.
		base = `{host=~"$host", severity=~"$severity"}`
		// baseAll supports severity-independent breakdowns.
		baseAll = `{host=~"$host", severity=~".+"}`
		// baseApp applies the selected application.
		baseApp = `{host=~"$host", severity=~"$severity"} | json | __error__="" | appname=~"$appname"`
		// Match numeric and text severities; KPI selectors ignore the dropdown.
		errSel  = `{host=~"$host", severity=~"emerg|alert|crit|err|error|[0-3]"}`
		warnSel = `{host=~"$host", severity=~"warning|warn|4"}`
		// Separate noisy network-appliance warnings from Linux warnings. Enumerate
		// appliances so unknown hosts fall into the actionable Linux side.
		networkDevices = `bgw1|wifi-ap1|lab-sw1`
		warnSelDevice  = `{host=~"$host", severity=~"warning|warn|4", host=~"` + networkDevices + `"}`
		warnSelHost    = `{host=~"$host", severity=~"warning|warn|4", host!~"` + networkDevices + `"}`
	)

	issueThresholds := issueThresholds()

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	// Network-device warning volume has no evidence-based threshold; keep it neutral.
	neutralThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "text"},
		})

	d, err := dashboard.NewDashboardBuilder("Syslog").
		Uid("syslog").
		Tags([]string{"syslog", "network", "logs", "infrastructure"}).
		Timezone("browser").
		Time("now-3h", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			lokiDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("host").
				Label("Host").
				Datasource(ds).
				// Scope host options to streams carrying severity.
				Query(dashboard.StringOrMap{String: new(`label_values({severity=~".+"}, host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// Use .+ for All so infrequent hosts remain in scope.
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("severity").
				Label("Severity").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$host"}, severity)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("appname").
				Label("App").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$host"}, appname)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				AllValue(".+"),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(8).Height(4).
				Unit("cps").
				Min(0).
				Thresholds(measurementThresholds()).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + base + `[5m])) or vector(0)`).
					Instant(true).
					LegendFormat("logs/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Errors (1h)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + errSel + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Host Warnings (1h)").
				Description("Linux hosts only: node1-5, pve, rpi3. Zero in most hours and one to three in the rest, almost all pvestatd, so yellow here is worth a look rather than the permanent state it used to be. The network appliances are counted separately below and the two tiles together are every warning-severity line.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(warnThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + warnSelHost + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Network Device Warnings (1h)").
				Description("bgw1, wifi-ap1 and lab-sw1, which log routine packet-level events at warning severity: ICMP redirect discards, TCP resets, DHCP leases not found. Watch the level, not the presence -- the steady state is around 78 an hour, so this tile is deliberately uncoloured.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(neutralThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + warnSelDevice + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Parse Errors (1h)").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Description("Empty is good: syslog lines this dashboard could not parse as JSON. Zero over the last day.").
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					// Restrict parse failures to syslog streams.
					Expr(`sum(count_over_time(` + baseAll + ` | json | __error__!="" [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Volume Trends")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + base + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Severity").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					// Ignore the severity dropdown for the full breakdown.
					Expr(`sum by (severity) (rate(`+baseAll+`[$__auto]))`+
						` or sum by (severity) (count_over_time(`+baseAll+`[$__range])) * 0`).
					LegendFormat("{{severity}}"),
				).
				// Semantic coloring; covers both numeric and text severity values.
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(emerg|alert|crit|err|error|[0-3])$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(warning|warn|4)$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(notice|5)$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "blue"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(info|6)$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}),
		).
		WithRow(dashboard.NewRowBuilder("App Breakdown")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by App").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate(` + baseApp + `[$__auto]))` +
						` or sum by (appname) (count_over_time(` + baseApp + `[$__range])) * 0`).
					LegendFormat("{{appname}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Errors by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				Description("Empty is good. The whole estate logs error severity on the order of ten lines a day, so a flat set of zero bars is the normal reading.").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					// Baseline hosts over the full range so quiet series remain visible.
					Expr(`sum by (host) (count_over_time(` + errSel + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warnings by Host").
				Datasource(ds).
				Span(24).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (count_over_time(` + warnSel + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Syslog").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				SortOrder(common.LogsSortOrderDescending).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				WithTarget(loki.NewDataqueryBuilder().
					// line_format persists host, severity, app, and message for All views.
					Expr(baseApp + ` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
