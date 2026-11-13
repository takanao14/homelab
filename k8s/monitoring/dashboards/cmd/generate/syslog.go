package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	syslogPanelLogRate                 = 1
	syslogPanelErrors1h                = 2
	syslogPanelHostWarnings1h          = 3
	syslogPanelNetworkDeviceWarnings1h = 4
	syslogPanelParseErrors1h           = 5
	syslogPanelLogVolumeByHost         = 6
	syslogPanelLogVolumeBySeverity     = 7
	syslogPanelLogVolumeByApp          = 8
	syslogPanelErrorsByHost            = 9
	syslogPanelWarningsByHost          = 10
	syslogPanelSyslog                  = 11
)

// buildSyslog covers RFC 5424 network-device and Proxmox logs in Loki.
// Follow dns_logs window and baseline conventions. Fixed KPI windows remain
// literal; volume uses rates while sparse errors/warnings use event counts.
func buildSyslog() (*dashboardv2.Dashboard, error) {
	const (
		elementLogRate                 = "log-rate"
		elementErrors1h                = "errors-1h"
		elementHostWarnings1h          = "host-warnings-1h"
		elementNetworkDeviceWarnings1h = "network-device-warnings-1h"
		elementParseErrors1h           = "parse-errors-1h"
		elementLogVolumeByHost         = "log-volume-by-host"
		elementLogVolumeBySeverity     = "log-volume-by-severity"
		elementLogVolumeByApp          = "log-volume-by-app"
		elementErrorsByHost            = "errors-by-host"
		elementWarningsByHost          = "warnings-by-host"
		elementSyslog                  = "syslog"
	)

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

	warnThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	// Network-device warning volume has no evidence-based threshold; keep it neutral.
	neutralThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "text"},
		})

	d := dashboardv2.NewDashboardBuilder("Syslog").
		Tags([]string{"syslog", "network", "logs", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-3h").To("now").AutoRefresh("60s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(lokiDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("host").
			Label("Host").
			// Scope host options to streams carrying severity.
			Query(lokiVariableQuery(`label_values({severity=~".+"}, host)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			// Use .+ for All so infrequent hosts remain in scope.
			AllValue(".+")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("severity").
			Label("Severity").
			Query(lokiVariableQuery(`label_values({host=~"$host"}, severity)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			AllValue(".+")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("appname").
			Label("App").
			Query(lokiVariableQuery(`label_values({host=~"$host"}, appname)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			AllValue(".+"))

	d.Panel(elementLogRate, dashboardv2.NewPanelBuilder().
		Id(syslogPanelLogRate).
		Title("Log Rate").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			Thresholds(measurementThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum(rate(`+base+`[5m])) or vector(0)`).
				Instant(true).
				LegendFormat("logs/s")))))

	d.Panel(elementErrors1h, dashboardv2.NewPanelBuilder().
		Id(syslogPanelErrors1h).
		Title("Errors (1h)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum(count_over_time(`+errSel+` [1h])) or vector(0)`).
				Instant(true).
				LegendFormat("errors")))))

	d.Panel(elementHostWarnings1h, dashboardv2.NewPanelBuilder().
		Id(syslogPanelHostWarnings1h).
		Title("Host Warnings (1h)").
		Description("Linux hosts only: node1-5, pve, rpi3. Zero in most hours and one to three in the rest, almost all pvestatd, so yellow here is worth a look rather than the permanent state it used to be. The network appliances are counted separately below and the two tiles together are every warning-severity line.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(warnThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum(count_over_time(`+warnSelHost+` [1h])) or vector(0)`).
				Instant(true).
				LegendFormat("warnings")))))

	d.Panel(elementNetworkDeviceWarnings1h, dashboardv2.NewPanelBuilder().
		Id(syslogPanelNetworkDeviceWarnings1h).
		Title("Network Device Warnings (1h)").
		Description("bgw1, wifi-ap1 and lab-sw1, which log routine packet-level events at warning severity: ICMP redirect discards, TCP resets, DHCP leases not found. Watch the level, not the presence -- the steady state is around 78 an hour, so this tile is deliberately uncoloured.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(neutralThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum(count_over_time(`+warnSelDevice+` [1h])) or vector(0)`).
				Instant(true).
				LegendFormat("warnings")))))

	d.Panel(elementParseErrors1h, dashboardv2.NewPanelBuilder().
		Id(syslogPanelParseErrors1h).
		Title("Parse Errors (1h)").
		Description("Empty is good: syslog lines this dashboard could not parse as JSON. Zero over the last day.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// Restrict parse failures to syslog streams.
				Expr(`sum(count_over_time(`+baseAll+` | json | __error__!="" [1h])) or vector(0)`).
				Instant(true).
				LegendFormat("errors")))))

	d.Panel(elementLogVolumeByHost, dashboardv2.NewPanelBuilder().
		Id(syslogPanelLogVolumeByHost).
		Title("Log Volume by Host").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			FillOpacity(10).
			Tooltip(tooltipAll).
			Legend(legend).
			SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (host) (rate(`+base+`[$__auto]))`+
					` or sum by (host) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{host}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementLogVolumeBySeverity, dashboardv2.NewPanelBuilder().
		Id(syslogPanelLogVolumeBySeverity).
		Title("Log Volume by Severity").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			// Semantic coloring; covers both numeric and text severity values.
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: "/^(emerg|alert|crit|err|error|[0-3])$/"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: "/^(warning|warn|4)$/"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: "/^(notice|5)$/"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "blue"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: "/^(info|6)$/"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// Ignore the severity dropdown for the full breakdown.
				Expr(`sum by (severity) (rate(`+baseAll+`[$__auto]))`+
					` or sum by (severity) (count_over_time(`+baseAll+`[$__range])) * 0`).
				LegendFormat("{{severity}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementLogVolumeByApp, dashboardv2.NewPanelBuilder().
		Id(syslogPanelLogVolumeByApp).
		Title("Log Volume by App").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (appname) (rate(`+baseApp+`[$__auto]))`+
					` or sum by (appname) (count_over_time(`+baseApp+`[$__range])) * 0`).
				LegendFormat("{{appname}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementErrorsByHost, dashboardv2.NewPanelBuilder().
		Id(syslogPanelErrorsByHost).
		Title("Errors by Host").
		Description("Empty is good. The whole estate logs error severity on the order of ten lines a day, so a flat set of zero bars is the normal reading.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// Baseline hosts over the full range so quiet series remain visible.
				Expr(`sum by (host) (count_over_time(`+errSel+`[$__auto]))`+
					` or sum by (host) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{host}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementWarningsByHost, dashboardv2.NewPanelBuilder().
		Id(syslogPanelWarningsByHost).
		Title("Warnings by Host").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				Expr(`sum by (host) (count_over_time(`+warnSel+`[$__auto]))`+
					` or sum by (host) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{host}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementSyslog, dashboardv2.NewPanelBuilder().
		Id(syslogPanelSyslog).
		Title("Syslog").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			SortOrder(common.LogsSortOrderDescending).
			EnableLogDetails(true).
			ShowLogContextToggle(true).
			ShowControls(true).
			ShowFieldSelector(true)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().
				// line_format persists host, severity, app, and message for All views.
				Expr(baseApp+` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
				MaxLines(500)))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementLogRate).
			WithItem(elementErrors1h).
			WithItem(elementHostWarnings1h).
			WithItem(elementNetworkDeviceWarnings1h).
			WithItem(elementParseErrors1h))).
		Row(dashboardv2.NewRowBuilder().Title("Volume Trends").AutoGridLayout(chartGrid().
			WithItem(elementLogVolumeByHost).
			WithItem(elementLogVolumeBySeverity))).
		Row(dashboardv2.NewRowBuilder().Title("App Breakdown").AutoGridLayout(chartGrid().
			WithItem(elementLogVolumeByApp).
			WithItem(elementErrorsByHost))).
		Row(dashboardv2.NewRowBuilder().Title("Warnings by Host").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementWarningsByHost))).
		Row(dashboardv2.NewRowBuilder().Title("Logs").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementSyslog)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
