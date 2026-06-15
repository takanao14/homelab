package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildServiceLogs defines a generic journald service log dashboard backed by Loki.
// Logs are JSON-encoded journald entries shipped via vector with labels: host, unit.
// PRIORITY follows syslog convention: 0=emerg … 3=err, 4=warning, 5=notice, 6=info, 7=debug.
func buildServiceLogs() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		base        = `{host=~"$host", unit=~"$unit"}`
		baseJSON    = `{host=~"$host", unit=~"$unit"} | json | __error__=""`
		baseMessage = `{host=~"$host", unit=~"$unit"} | json | __error__="" | line_format "{{.message}}"`
	)

	errorThresholds := issueThresholds()

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "yellow"},
		})

	d, err := dashboard.NewDashboardBuilder("Service Logs").
		Uid("service-logs").
		Tags([]string{"logs", "infrastructure", "journald"}).
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
			dashboard.NewQueryVariableBuilder("unit").
				Label("Unit").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values({host=~"$host"}, unit)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).

		// Row 1: Summary
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(8).Height(4).
				Unit("cps").
				Min(0).
				Orientation(common.VizOrientationAuto).
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
				Orientation(common.VizOrientationAuto).
				Thresholds(errorThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					// PRIORITY 0-3: emerg, alert, crit, err
					Expr(`sum(count_over_time(` + baseJSON + ` | PRIORITY =~ "[0-3]" [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Warnings (1h)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				Thresholds(warnThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					// PRIORITY 4: warning
					Expr(`sum(count_over_time(` + baseJSON + ` | PRIORITY = "4" [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).

		// Row 2: Volume Trends
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
					Expr(`sum by (host) (rate(` + base + `[5m]))`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Unit").
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
					Expr(`sum by (unit) (rate(` + base + `[5m]))`).
					LegendFormat("{{unit}}"),
				),
		).

		// Row 3: Errors and Warnings
		WithRow(dashboard.NewRowBuilder("Errors and Warnings")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Error Rate by Unit").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (unit) (rate(` + baseJSON + ` | PRIORITY =~ "[0-3]" [5m])) or sum by (unit) (rate(` + base + `[5m])) * 0`).
					LegendFormat("{{unit}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warning Rate by Unit").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (unit) (rate(` + baseJSON + ` | PRIORITY = "4" [5m])) or sum by (unit) (rate(` + base + `[5m])) * 0`).
					LegendFormat("{{unit}}"),
				),
		).

		// Row 4: Log browser
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Service Logs").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				EnableLogDetails(true).
				SortOrder(common.LogsSortOrderDescending).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(baseMessage).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
