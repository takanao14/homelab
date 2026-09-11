package main

import (
	"fmt"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildServiceLogs covers Vector-shipped journald JSON with host/unit labels.
// Follow dns_logs window and zero-baseline conventions. Use rates for volume
// and count_over_time bars for sparse errors and warnings.
func buildServiceLogs() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	logShippingVMs, err := loadLogShippingVMs()
	if err != nil {
		return nil, err
	}

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
	reportingThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(len(logShippingVMs) - 1)), Color: "yellow"},
			{Value: new(float64(len(logShippingVMs))), Color: "green"},
		})
	activityThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})
	activityMappings := []dashboard.ValueMapping{
		{ValueMap: &dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				"0": {Text: new("QUIET"), Color: new("red")},
				"1": {Text: new("RECEIVING"), Color: new("green")},
			},
		}},
	}

	reportingVMExpressions := make([]string, 0, len(logShippingVMs))
	logsByVM := bargauge.NewPanelBuilder().
		Title("Logs Received by VM (24h)").
		Description("A zero means Loki received no journald logs from that expected VM in the last 24 hours.").
		Datasource(ds).
		Span(12).Height(8).
		Unit("short").
		Min(0).
		Thresholds(activityThresholds).
		Orientation(common.VizOrientationHorizontal).
		ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))
	vmActivity := statetimeline.NewPanelBuilder().
		Title("VM Log Activity").
		Description("Receiving means at least one log arrived in the preceding 15 minutes. Quiet can be normal for low-volume VMs.").
		Datasource(ds).
		Span(12).Height(8).
		Thresholds(activityThresholds).
		Mappings(activityMappings).
		ShowValue(common.VisibilityModeNever).
		MergeValues(true).
		Tooltip(tooltipAll)
	for _, host := range logShippingVMs {
		selector := fmt.Sprintf(`{host=%q, unit=~".+"}`, host)
		last24h := `sum(count_over_time(` + selector + `[24h])) or vector(0)`
		reportingVMExpressions = append(reportingVMExpressions, `((`+last24h+`) > bool 0)`)
		logsByVM.WithTarget(loki.NewDataqueryBuilder().
			Expr(last24h).
			Instant(true).
			LegendFormat(host),
		)
		vmActivity.WithTarget(loki.NewDataqueryBuilder().
			Expr(`((sum(count_over_time(` + selector + `[15m])) or vector(0)) > bool 0)`).
			LegendFormat(host),
		)
	}
	reportingVMs := strings.Join(reportingVMExpressions, " + ")

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
				Span(6).Height(4).
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
				Title("Reporting VMs (24h)").
				Description("Expected Vector-enabled VMs with at least one journald log received by Loki in the last 24 hours.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).Max(float64(len(logShippingVMs))).
				Thresholds(reportingThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(reportingVMs).
					Instant(true).
					LegendFormat("reporting"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Errors (1h)").
				Datasource(ds).
				Span(6).Height(4).
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
				Span(6).Height(4).
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
		WithRow(dashboard.NewRowBuilder("Delivery Status")).
		WithPanel(logsByVM).
		WithPanel(vmActivity).
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
