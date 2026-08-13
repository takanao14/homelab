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
//
// Window and baseline conventions follow dns_logs.go; see the comment at the top
// of that file. Aggregation windows use $__auto so the window tracks the zoom
// rather than reading five minutes out of every step at wide ranges,
// Interval("1m") floors it, and every timeseries pins its series to zero because LogQL emits no
// sample at all for an empty window. The volume here is low enough that all
// three matter: across every journald host the estate produces a few dozen lines
// a day at PRIORITY 0-4, and most units log nothing for hours at a time.
//
// The error and warning panels plot count_over_time() as bars, while the volume
// panels keep rate() in counts per second; see the same split explained in
// syslog.go. At a few dozen events a day a per-second rate lands in thousandths
// and moves with the zoom, which is no way to read an error count.
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
				// Scoped to the streams that carry a unit label. The host label is
				// shared with the syslog and DNS query log streams, so the unscoped
				// label_values(host) offered 21 hosts of which only 11 ship journald:
				// picking any of the other 10 emptied the whole dashboard.
				Query(dashboard.StringOrMap{String: new(`label_values({unit=~".+"}, host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// All expands to ".+" rather than the values seen in the current
				// range, so a host that stayed silent through the view is still
				// covered. Every selector also constrains unit, which is what keeps
				// the syslog and DNS streams out.
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

		// Row 1: Summary
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

		// Row 2: Volume Trends
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

		// Row 3: Errors & Warnings
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
					// The baseline is keyed on $__range rather than the current window,
					// which is itself empty for a unit that has gone quiet.
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

		// Row 4: Log browser
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
					// line_format is what picks the displayed fields: the logs panel
					// has no displayedFields option in the schema, and
					// ShowFieldSelector only lets a viewer choose fields for the
					// session, with nothing persisted back to the dashboard. host and
					// unit are in front of the message because both variables default
					// to All, so the bare message left no way to tell which service on
					// which host produced a line -- and journald units repeat across
					// hosts, so the unit alone does not identify it either.
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
