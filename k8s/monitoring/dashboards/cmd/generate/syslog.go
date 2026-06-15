package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildSyslog defines the network device syslog dashboard backed by Loki.
// Logs are JSON-parsed syslog entries with fields: host, severity, appname, message.
func buildSyslog() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		// Syslog has no job label; filter by severity to exclude DNS query logs.
		base = `{host=~"$host", severity=~"$severity"}`
		// baseApp additionally filters by the $appname variable for app-scoped panels.
		baseApp = `{host=~"$host", severity=~"$severity"} | json | __error__="" | appname=~"$appname"`
		// Severity ships as either RFC5424 numeric (0=emerg … 3=err, 4=warning, 6=info)
		// or text depending on the device, so error/warning selectors match both forms.
		// These KPI selectors intentionally ignore $severity so the counts stay meaningful
		// regardless of the dropdown selection.
		errSel  = `{host=~"$host", severity=~"emerg|alert|crit|err|error|[0-3]"}`
		warnSel = `{host=~"$host", severity=~"warning|warn|4"}`
	)

	issueThresholds := issueThresholds()

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "yellow"},
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
				Query(dashboard.StringOrMap{String: strPtr(`label_values(host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("severity").
				Label("Severity").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(severity)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("appname").
				Label("App").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(appname)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).

		// Row 1: Summary stats
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("cps").
				Min(0).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + base + `[$__rate_interval])) or vector(0)`).
					Instant(true).
					LegendFormat("logs/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Errors (1h)").
				Datasource(ds).
				Span(6).Height(4).
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
				Title("Warnings (1h)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(warnThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + warnSel + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Parse Errors (1h)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time({host=~"$host"} | json | __error__!="" [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).

		// Row 2: Volume trends
		WithRow(dashboard.NewRowBuilder("Volume Trends")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + base + `[$__rate_interval]))`).
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
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					// Always break down across all severities, independent of the $severity filter.
					Expr(`sum by (severity) (rate({host=~"$host", severity=~".+"}[$__rate_interval]))`).
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

		// Row 3: App breakdown + errors/warnings by host
		WithRow(dashboard.NewRowBuilder("App Breakdown")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by App").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate(` + baseApp + `[$__rate_interval]))`).
					LegendFormat("{{appname}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Error Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					// "* 0" tail keeps every host series present even at zero error rate.
					Expr(`sum by (host) (rate(` + errSel + `[$__rate_interval])) or sum by (host) (rate(` + base + `[$__rate_interval])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warning Rate by Host").
				Datasource(ds).
				Span(24).Height(8).
				Unit("cps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + warnSel + `[$__rate_interval])) or sum by (host) (rate(` + base + `[$__rate_interval])) * 0`).
					LegendFormat("{{host}}"),
				),
		).

		// Row 4: Log browser
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Syslog").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				EnableLogDetails(true).
				SortOrder(common.LogsSortOrderDescending).
				WithTarget(loki.NewDataqueryBuilder().
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
