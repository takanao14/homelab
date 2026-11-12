package main

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
)

// Shared fragment factories return fresh builders to avoid mutable state aliasing.

// queryDatasource refers to a dashboard datasource variable.
func queryDatasource(name string) *dashboardv2.Dashboardv2DataQueryKindDatasourceBuilder {
	return dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("$" + name)
}

// promQuery starts a query against the selected Prometheus datasource.
func promQuery() *prometheus.QueryV2Builder {
	return prometheus.NewQueryV2Builder().Datasource(queryDatasource("datasource"))
}

// lokiQuery starts a query against the selected Loki datasource.
func lokiQuery() *loki.QueryV2Builder {
	return loki.NewQueryV2Builder().Datasource(queryDatasource("datasource"))
}

// queries assigns default refIds (A, B, ...); queryWithRef preserves explicit
// references used by field overrides.
func queries(qs ...cog.Builder[dashboardv2.DataQueryKind]) *dashboardv2.QueryGroupBuilder {
	group := dashboardv2.NewQueryGroupBuilder()
	for i, q := range qs {
		ref := refID(i)
		if named, ok := q.(namedPanelQuery); ok {
			ref = named.ref
		}
		group.Target(dashboardv2.NewTargetBuilder().RefId(ref).Query(q))
	}
	return group
}

type namedPanelQuery struct {
	cog.Builder[dashboardv2.DataQueryKind]
	ref string
}

func queryWithRef(ref string, query cog.Builder[dashboardv2.DataQueryKind]) namedPanelQuery {
	return namedPanelQuery{Builder: query, ref: ref}
}

func refID(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return fmt.Sprintf("%s%c", refID(i/26-1), rune('A'+i%26))
}

// statGrid lays out stat tiles in up to columns narrow, short cells.
func statGrid(columns float64) *dashboardv2.AutoGridBuilder {
	return dashboardv2.AutoGrid().
		MaxColumnCount(columns).
		ColumnWidthMode(dashboardv2.AutoGridLayoutSpecColumnWidthModeNarrow).
		RowHeightMode(dashboardv2.AutoGridLayoutSpecRowHeightModeShort)
}

// chartGrid lays out charts and bar gauges two per line.
func chartGrid() *dashboardv2.AutoGridBuilder {
	return dashboardv2.AutoGrid().
		MaxColumnCount(2).
		ColumnWidthMode(dashboardv2.AutoGridLayoutSpecColumnWidthModeStandard).
		RowHeightMode(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)
}

// wideGrid gives each panel the full width, for many-value stats, tables, and
// logs.
func wideGrid(height dashboardv2.AutoGridLayoutSpecRowHeightMode) *dashboardv2.AutoGridBuilder {
	return dashboardv2.AutoGrid().
		MaxColumnCount(1).
		ColumnWidthMode(dashboardv2.AutoGridLayoutSpecColumnWidthModeStandard).
		RowHeightMode(height)
}

// promDatasourceVariable returns the Prometheus datasource selector.
func promDatasourceVariable() *dashboardv2.DatasourceVariableBuilder {
	return dashboardv2.NewDatasourceVariableBuilder("datasource").
		Label("Datasource").
		PluginId("prometheus")
}

// lokiDatasourceVariable returns the Loki datasource selector.
func lokiDatasourceVariable() *dashboardv2.DatasourceVariableBuilder {
	return dashboardv2.NewDatasourceVariableBuilder("datasource").
		Label("Datasource").
		PluginId("loki")
}

// promVariableQuery is a Prometheus label_values-style variable query. The SDK
// has no builder for the Prometheus variable-query shape.
type promVariableQuery string

func (q promVariableQuery) Build() (dashboardv2.DataQueryKind, error) {
	return dashboardv2.DataQueryKind{
		Kind: "DataQuery", Group: "prometheus", Version: "v0",
		Datasource: &dashboardv2.Dashboardv2DataQueryKindDatasource{Name: new("$datasource")},
		Spec: map[string]any{
			"qryType": 1,
			"query":   string(q),
			"refId":   "PrometheusVariableQueryEditor-VariableQuery",
		},
	}, nil
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
func issueThresholds() *dashboardv2.ThresholdsConfigBuilder {
	return dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "red"},
		})
}

// watchdogAwareFiringAlertThresholds allows the expected Watchdog alert.
func watchdogAwareFiringAlertThresholds() *dashboardv2.ThresholdsConfigBuilder {
	return dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(2)), Color: "red"},
		})
}

// measurementThresholds prevents Grafana's implicit 80% threshold on neutral values.
func measurementThresholds() *dashboardv2.ThresholdsConfigBuilder {
	return dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "blue"},
		})
}

// capacityThresholds marks percentage utilization at 80% and 90%.
func capacityThresholds() *dashboardv2.ThresholdsConfigBuilder {
	return dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(80)), Color: "yellow"},
			{Value: new(float64(90)), Color: "red"},
		})
}

// timeToFullThresholds colors a "days until full" projection, where a lower
// value is worse: red below 30 days, yellow below 90, green above.
func timeToFullThresholds() *dashboardv2.ThresholdsConfigBuilder {
	return dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(30)), Color: "yellow"},
			{Value: new(float64(90)), Color: "green"},
		})
}

// zeroLineThresholds draws zero on bidirectional I/O panels.
func zeroLineThresholds() *dashboardv2.ThresholdsConfigBuilder {
	return dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "transparent"},
			{Value: new(float64(0)), Color: "white"},
		})
}

// zeroLineStyle renders the zero threshold as a line.
func zeroLineStyle() *common.GraphThresholdsStyleConfigBuilder {
	return common.NewGraphThresholdsStyleConfigBuilder().
		Mode(common.GraphThresholdsStyleModeLine)
}

// Loki's variable editor reads string queries through Grafana's legacy wrapper.
type lokiVariableQuery string

func (q lokiVariableQuery) Build() (dashboardv2.DataQueryKind, error) {
	return dashboardv2.DataQueryKind{
		Kind: "DataQuery", Group: "loki", Version: "v0",
		Datasource: &dashboardv2.Dashboardv2DataQueryKindDatasource{Name: new("$datasource")},
		Spec:       map[string]any{"__legacyStringValue": string(q)},
	}, nil
}
