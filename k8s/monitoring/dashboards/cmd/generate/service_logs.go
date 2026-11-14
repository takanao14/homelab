package main

import (
	"fmt"
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	serviceLogsPanelLogRate                         = 1
	serviceLogsPanelReportingVMs24h                 = 2
	serviceLogsPanelErrors1h                        = 3
	serviceLogsPanelWarnings1h                      = 4
	serviceLogsPanelLogsReceivedByVM24h             = 5
	serviceLogsPanelVMLogActivity                   = 6
	serviceLogsPanelVectorTargetsUp                 = 7
	serviceLogsPanelLokiSinkErrors15m               = 8
	serviceLogsPanelUnintentionalDiscards15m        = 9
	serviceLogsPanelLokiBufferEvents                = 10
	serviceLogsPanelJournaldToLokiEventRate         = 11
	serviceLogsPanelJournaldSourceBufferUtilization = 12
	serviceLogsPanelLogVolumeByHost                 = 13
	serviceLogsPanelLogVolumeByUnit                 = 14
	serviceLogsPanelErrorsByUnit                    = 15
	serviceLogsPanelWarningsByUnit                  = 16
	serviceLogsPanelServiceLogs                     = 17
)

// buildServiceLogs covers Vector-shipped journald JSON with host/unit labels.
// Follow dns_logs window and zero-baseline conventions. Use rates for volume
// and count_over_time bars for sparse errors and warnings.
func buildServiceLogs() (*dashboardv2.Dashboard, error) {
	const (
		elementLogRate                         = "log-rate"
		elementReportingVMs24h                 = "reporting-vms-24h"
		elementErrors1h                        = "errors-1h"
		elementWarnings1h                      = "warnings-1h"
		elementLogsReceivedByVM24h             = "logs-received-by-vm-24h"
		elementVMLogActivity                   = "vm-log-activity"
		elementVectorTargetsUp                 = "vector-targets-up"
		elementLokiSinkErrors15m               = "loki-sink-errors-15m"
		elementUnintentionalDiscards15m        = "unintentional-discards-15m"
		elementLokiBufferEvents                = "loki-buffer-events"
		elementJournaldToLokiEventRate         = "journald-to-loki-event-rate"
		elementJournaldSourceBufferUtilization = "journald-source-buffer-utilization"
		elementLogVolumeByHost                 = "log-volume-by-host"
		elementLogVolumeByUnit                 = "log-volume-by-unit"
		elementErrorsByUnit                    = "errors-by-unit"
		elementWarningsByUnit                  = "warnings-by-unit"
		elementServiceLogs                     = "service-logs"
	)

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

	warnThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})
	reportingThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(len(logShippingVMs) - 1)), Color: "yellow"},
			{Value: new(float64(len(logShippingVMs))), Color: "green"},
		})
	activityThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})
	activityMappings := []dashboardv2.ValueMapping{
		{ValueMap: &dashboardv2.ValueMap{
			Type: dashboardv2.MappingTypeValue,
			Options: map[string]dashboardv2.ValueMappingResult{
				"0": {Text: new("QUIET"), Color: new("red")},
				"1": {Text: new("RECEIVING"), Color: new("green")},
			},
		}},
	}

	reportingVMExpressions := make([]string, 0, len(logShippingVMs))
	logsByVM := dashboardv2.NewPanelBuilder().
		Title("Logs Received by VM (24h)").
		Description("A zero means Loki received no journald logs from that expected VM in the last 24 hours.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(activityThresholds).
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true)))
	vmActivity := dashboardv2.NewPanelBuilder().
		Title("VM Log Activity").
		Description("Receiving means at least one log arrived in the preceding 15 minutes. Quiet can be normal for low-volume VMs.").
		Visualization(statetimeline.NewVisualizationV2Builder().
			Thresholds(activityThresholds).
			Mappings(activityMappings).
			ShowValue(common.VisibilityModeNever).
			MergeValues(true).
			Tooltip(tooltipAll))
	logsByVMQueries := dashboardv2.NewQueryGroupBuilder()
	vmActivityQueries := dashboardv2.NewQueryGroupBuilder()
	for _, host := range logShippingVMs {
		selector := fmt.Sprintf(`{host=%q, unit=~".+"}`, host)
		last24h := `sum(count_over_time(` + selector + `[24h])) or vector(0)`
		reportingVMExpressions = append(reportingVMExpressions, `((`+last24h+`) > bool 0)`)
		logsByVMQueries.Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
			Expr(last24h).
			Instant(true).
			LegendFormat(host)))
		vmActivityQueries.Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
			Expr(`((sum(count_over_time(` + selector + `[15m])) or vector(0)) > bool 0)`).
			LegendFormat(host)))
	}
	reportingVMs := strings.Join(reportingVMExpressions, " + ")
	vectorJob := `job="scrapeConfig/monitoring/vector-external"`
	vectorTargetThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(len(logShippingVMs) - 1)), Color: "yellow"},
			{Value: new(float64(len(logShippingVMs))), Color: "green"},
		})

	d := dashboardv2.NewDashboardBuilder("Service Logs").
		Tags([]string{"logs", "infrastructure", "journald"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-3h").To("now").AutoRefresh("60s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(lokiDatasourceVariable()).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("prometheus").
			Label("Prometheus").
			PluginId("prometheus")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("host").
			Label("Host").
			// Scope host options to journald streams carrying unit labels.
			Query(lokiVariableQuery(`label_values({unit=~".+"}, host)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			// Use .+ for All so silent hosts remain in scope.
			AllValue(".+")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("unit").
			Label("Unit").
			Query(lokiVariableQuery(`label_values({host=~"$host"}, unit)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			AllValue(".+"))

	d.Panel(elementLogRate, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelLogRate).
		Title("Log Rate").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(rate(`+base+`[5m])) or vector(0)`).
				Instant(true).
				LegendFormat("logs/s")))))

	d.Panel(elementReportingVMs24h, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelReportingVMs24h).
		Title("Reporting VMs (24h)").
		Description("Expected Vector-enabled VMs with at least one journald log received by Loki in the last 24 hours.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Max(float64(len(logShippingVMs))).
			Thresholds(reportingThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(reportingVMs).
				Instant(true).
				LegendFormat("reporting")))))

	d.Panel(elementErrors1h, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelErrors1h).
		Title("Errors (1h)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Orientation(common.VizOrientationAuto).
			Thresholds(errorThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				// PRIORITY 0-3: emerg, alert, crit, err
				Expr(`sum(count_over_time(`+baseJSON+` | PRIORITY =~ "[0-3]" [1h])) or vector(0)`).
				Instant(true).
				LegendFormat("errors")))))

	d.Panel(elementWarnings1h, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelWarnings1h).
		Title("Warnings (1h)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Orientation(common.VizOrientationAuto).
			Thresholds(warnThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				// PRIORITY 4: warning
				Expr(`sum(count_over_time(`+baseJSON+` | PRIORITY = "4" [1h])) or vector(0)`).
				Instant(true).
				LegendFormat("warnings")))))

	d.Panel(elementLogsReceivedByVM24h, logsByVM.Data(logsByVMQueries).Id(serviceLogsPanelLogsReceivedByVM24h))

	d.Panel(elementVMLogActivity, vmActivity.Data(vmActivityQueries).Id(serviceLogsPanelVMLogActivity))

	d.Panel(elementVectorTargetsUp, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelVectorTargetsUp).
		Title("Vector Targets Up").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Max(float64(len(logShippingVMs))).
			Thresholds(vectorTargetThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`sum(up{`+vectorJob+`}) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("targets")))))

	d.Panel(elementLokiSinkErrors15m, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelLokiSinkErrors15m).
		Title("Loki Sink Errors (15m)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds()).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`sum(increase(vector_component_errors_total{`+vectorJob+`,component_id="loki"}[15m])) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("errors")))))

	d.Panel(elementUnintentionalDiscards15m, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelUnintentionalDiscards15m).
		Title("Unintentional Discards (15m)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds()).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`sum(increase(vector_component_discarded_events_total{`+vectorJob+`,intentional="false"}[15m])) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("discards")))))

	d.Panel(elementLokiBufferEvents, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelLokiBufferEvents).
		Title("Loki Buffer Events").
		Description("Current events queued in the in-memory Loki sink buffers. Observe the baseline before alerting.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`sum(vector_buffer_size_events{`+vectorJob+`,component_id="loki"}) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("events")))))

	d.Panel(elementJournaldToLokiEventRate, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelJournaldToLokiEventRate).
		Title("Journald to Loki Event Rate").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			Thresholds(measurementThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`sum by (instance) (rate(vector_component_received_events_total{`+vectorJob+`,component_id="journald"}[$__rate_interval]))`).
				LegendFormat("{{instance}} received"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`sum by (instance) (rate(vector_component_sent_events_total{`+vectorJob+`,component_id="loki"}[$__rate_interval]))`).
				LegendFormat("{{instance}} sent")))))

	d.Panel(elementJournaldSourceBufferUtilization, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelJournaldSourceBufferUtilization).
		Title("Journald Source Buffer Utilization").
		Description("Current source-buffer utilization by VM; observe the baseline before setting alert thresholds.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percentunit").
			Min(0).
			Max(100).
			Thresholds(measurementThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().Datasource(queryDatasource("prometheus")).
				Expr(`max by (instance) (vector_source_buffer_utilization_level{`+vectorJob+`,component_id="journald"}) * 100`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementLogVolumeByHost, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelLogVolumeByHost).
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
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum by (host) (rate(`+base+`[$__auto]))`+
					` or sum by (host) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{host}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementLogVolumeByUnit, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelLogVolumeByUnit).
		Title("Log Volume by Unit").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			FillOpacity(10).
			Tooltip(tooltipAll).
			Legend(legend).
			SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum by (unit) (rate(`+base+`[$__auto]))`+
					` or sum by (unit) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{unit}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementErrorsByUnit, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelErrorsByUnit).
		Title("Errors by Unit").
		Description("Empty is good. Across every journald host the estate produces a few dozen lines a day at PRIORITY 0-4 in total, so a flat set of zero bars is the normal reading.").
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
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				// Key zero on the full range so quiet units remain visible.
				Expr(`sum by (unit) (count_over_time(`+baseJSON+` | PRIORITY =~ "[0-3]" [$__auto]))`+
					` or sum by (unit) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{unit}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementWarningsByUnit, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelWarningsByUnit).
		Title("Warnings by Unit").
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
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum by (unit) (count_over_time(`+baseJSON+` | PRIORITY = "4" [$__auto]))`+
					` or sum by (unit) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{unit}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementServiceLogs, dashboardv2.NewPanelBuilder().
		Id(serviceLogsPanelServiceLogs).
		Title("Service Logs").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			SortOrder(common.LogsSortOrderDescending).
			EnableLogDetails(true).
			ShowLogContextToggle(true).
			ShowControls(true).
			ShowFieldSelector(true)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				// line_format persists host, unit, and message for unfiltered log views.
				Expr(baseJSON+` | line_format "{{.host}} [{{.unit}}] {{.message}}"`).
				MaxLines(500)))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementLogRate).
			WithItem(elementReportingVMs24h).
			WithItem(elementErrors1h).
			WithItem(elementWarnings1h))).
		Row(dashboardv2.NewRowBuilder().Title("Delivery Status").AutoGridLayout(chartGrid().
			WithItem(elementLogsReceivedByVM24h))).
		Row(dashboardv2.NewRowBuilder().Title("VM Log Activity").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementVMLogActivity))).
		Row(dashboardv2.NewRowBuilder().Title("Vector Pipeline").AutoGridLayout(statGrid(4).
			WithItem(elementVectorTargetsUp).
			WithItem(elementLokiSinkErrors15m).
			WithItem(elementUnintentionalDiscards15m).
			WithItem(elementLokiBufferEvents))).
		Row(dashboardv2.NewRowBuilder().Title("Journald to Loki Event Rate").AutoGridLayout(chartGrid().
			WithItem(elementJournaldToLokiEventRate).
			WithItem(elementJournaldSourceBufferUtilization))).
		Row(dashboardv2.NewRowBuilder().Title("Volume Trends").AutoGridLayout(chartGrid().
			WithItem(elementLogVolumeByHost).
			WithItem(elementLogVolumeByUnit))).
		Row(dashboardv2.NewRowBuilder().Title("Errors & Warnings").AutoGridLayout(chartGrid().
			WithItem(elementErrorsByUnit).
			WithItem(elementWarningsByUnit))).
		Row(dashboardv2.NewRowBuilder().Title("Logs").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementServiceLogs)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
