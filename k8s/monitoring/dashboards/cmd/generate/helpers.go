package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

// Shared fragment factories return fresh builders to avoid mutable state aliasing.

// promDatasource binds panels to the selected Prometheus datasource.
func promDatasource() common.DataSourceRef {
	dsType := "prometheus"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

// lokiDatasource binds panels to the selected Loki datasource.
func lokiDatasource() common.DataSourceRef {
	dsType := "loki"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

// promDatasourceVariable returns the Prometheus datasource selector.
func promDatasourceVariable() *dashboard.DatasourceVariableBuilder {
	return dashboard.NewDatasourceVariableBuilder("datasource").
		Label("Datasource").
		Type("prometheus")
}

// lokiDatasourceVariable returns the Loki datasource selector.
func lokiDatasourceVariable() *dashboard.DatasourceVariableBuilder {
	return dashboard.NewDatasourceVariableBuilder("datasource").
		Label("Datasource").
		Type("loki")
}

// defaultTooltip returns the standard multi-series tooltip.
func defaultTooltip() *common.VizTooltipOptionsBuilder {
	return common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
}

// defaultLegend returns the standard list-style legend placed at the bottom.
func defaultLegend() *common.VizLegendOptionsBuilder {
	return common.NewVizLegendOptionsBuilder().
		ShowLegend(true).
		DisplayMode(common.LegendDisplayModeList).
		Placement(common.LegendPlacementBottom)
}

// issueThresholds marks any nonzero anomaly count red.
func issueThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "red"},
		})
}

// watchdogAwareFiringAlertThresholds allows the expected Watchdog alert.
func watchdogAwareFiringAlertThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(2)), Color: "red"},
		})
}

// measurementThresholds prevents Grafana's implicit 80% threshold on neutral values.
func measurementThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "blue"},
		})
}

// capacityThresholds marks percentage utilization at 80% and 90%.
func capacityThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(80)), Color: "yellow"},
			{Value: new(float64(90)), Color: "red"},
		})
}

// timeToFullThresholds colors a "days until full" projection, where a lower
// value is worse: red below 30 days, yellow below 90, green above.
func timeToFullThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(30)), Color: "yellow"},
			{Value: new(float64(90)), Color: "green"},
		})
}

// zeroLineThresholds draws zero on bidirectional I/O panels.
func zeroLineThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "transparent"},
			{Value: new(float64(0)), Color: "white"},
		})
}

// zeroLineStyle renders the zero threshold as a line.
func zeroLineStyle() *common.GraphThresholdsStyleConfigBuilder {
	return common.NewGraphThresholdsStyleConfigBuilder().
		Mode(common.GraphThresholdsStyleModeLine)
}
