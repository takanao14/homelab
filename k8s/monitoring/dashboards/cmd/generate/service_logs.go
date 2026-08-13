package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildServiceLogs covers Vector-shipped journald JSON with host/unit labels.
// Follow dns_logs window and zero-baseline conventions. Use rates for volume
// and count_over_time bars for sparse errors and warnings.
func buildServiceLogs() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		base     = `{host=~"$host", unit=~"$unit"}`
		baseJSON = `{host=~"$host", unit=~"$unit"} | json | __error__=""`
	)

	errorThresholds := issueThresholds()

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
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
				// Scope host options to journald streams carrying unit labels.
				Query(dashboard.StringOrMap{String: new(`label_values({unit=~".+"}, host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// Use .+ for All so silent hosts remain in scope.
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("unit").
				Label("Unit").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$host"}, unit)`)}).
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
				Title("Log Volume by Unit").
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
					Expr(`sum by (unit) (rate(` + base + `[$__auto]))` +
						` or sum by (unit) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{unit}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Errors & Warnings")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Errors by Unit").
				Description("Empty is good. Across every journald host the estate produces a few dozen lines a day at PRIORITY 0-4 in total, so a flat set of zero bars is the normal reading.").
				Datasource(ds).
				Span(12).Height(8).
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
					// Key zero on the full range so quiet units remain visible.
					Expr(`sum by (unit) (count_over_time(` + baseJSON + ` | PRIORITY =~ "[0-3]" [$__auto]))` +
						` or sum by (unit) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{unit}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warnings by Unit").
				Datasource(ds).
				Span(12).Height(8).
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
					Expr(`sum by (unit) (count_over_time(` + baseJSON + ` | PRIORITY = "4" [$__auto]))` +
						` or sum by (unit) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{unit}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Service Logs").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				SortOrder(common.LogsSortOrderDescending).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				WithTarget(loki.NewDataqueryBuilder().
					// line_format persists host, unit, and message for unfiltered log views.
					Expr(baseJSON + ` | line_format "{{.host}} [{{.unit}}] {{.message}}"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
