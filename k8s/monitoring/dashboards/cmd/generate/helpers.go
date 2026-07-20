package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

// This file holds the shared "house style" helpers reused across dashboards.
// Reuse is kept to two levels (see README "Conventions"): string constants live
// inline in each builder (L0), and the fragment factories below (L1) return a
// fresh builder on every call so panels never share mutable state.

// promDatasource returns a datasource ref using "$datasource" as the UID so that
// all panels switch together when the user changes the datasource dropdown variable.
func promDatasource() common.DataSourceRef {
	dsType := "prometheus"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

// lokiDatasource is the Loki counterpart of promDatasource.
func lokiDatasource() common.DataSourceRef {
	dsType := "loki"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

// promDatasourceVariable returns the standard Prometheus datasource dropdown variable.
func promDatasourceVariable() *dashboard.DatasourceVariableBuilder {
	return dashboard.NewDatasourceVariableBuilder("datasource").
		Label("Datasource").
		Type("prometheus")
}

// lokiDatasourceVariable returns the standard Loki datasource dropdown variable.
func lokiDatasourceVariable() *dashboard.DatasourceVariableBuilder {
	return dashboard.NewDatasourceVariableBuilder("datasource").
		Label("Datasource").
		Type("loki")
}

// defaultTooltip returns a multi-series tooltip so all series values are visible on hover.
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

// issueThresholds returns green/red thresholds where any value >= 1 is red.
// Used for panels that count errors, degraded workloads, or other anomalies.
func issueThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "red"},
		})
}

// watchdogAwareFiringAlertThresholds keeps the normal Watchdog alert green while
// still marking any additional firing alert as an issue.
func watchdogAwareFiringAlertThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(2)), Color: "red"},
		})
}

// capacityThresholds returns the standard utilization thresholds for percent-based
// capacity panels: green below 80, yellow at 80, red at 90.
func capacityThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(80)), Color: "yellow"},
			{Value: new(float64(90)), Color: "red"},
		})
}

// zeroLineThresholds returns a transparent/white threshold pair used to draw a
// zero-reference line on bidirectional I/O panels (receive positive, transmit negative-Y).
func zeroLineThresholds() *dashboard.ThresholdsConfigBuilder {
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "transparent"},
			{Value: new(float64(0)), Color: "white"},
		})
}

// zeroLineStyle returns a threshold style that renders the zero-reference line
// as a solid line (not a shaded region).
func zeroLineStyle() *common.GraphThresholdsStyleConfigBuilder {
	return common.NewGraphThresholdsStyleConfigBuilder().
		Mode(common.GraphThresholdsStyleModeLine)
}
