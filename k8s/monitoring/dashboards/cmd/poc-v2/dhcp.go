package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

type prometheusVariableQuery struct {
	datasource string
	query      string
}

func (q prometheusVariableQuery) Build() (dashboardv2.DataQueryKind, error) {
	return dashboardv2.DataQueryKind{
		Kind: "DataQuery", Group: "prometheus", Version: "v0",
		Datasource: &dashboardv2.Dashboardv2DataQueryKindDatasource{Name: &q.datasource},
		Spec: map[string]any{
			"qryType": 1,
			"query":   q.query,
			"refId":   "PrometheusVariableQueryEditor-VariableQuery",
		},
	}, nil
}

func buildDhcpLeases() (*dashboardv2.Dashboard, error) {
	promDS := dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("$datasource")
	lokiDS := dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("$loki_datasource")
	tooltip := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
	legend := common.NewVizLegendOptionsBuilder().ShowLegend(true).DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom)
	measurementThresholds := dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "blue"}})
	healthThresholds := dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "red"}, {Value: new(float64(1)), Color: "green"}})
	snapshotThresholds := dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "green"}, {Value: new(float64(1800)), Color: "yellow"}, {Value: new(float64(3600)), Color: "red"}})

	const (
		observer  = `job="scrapeConfig/monitoring/node-exporter-external"`
		leaseInfo = `dhcp_lease_observer_lease_info{` + observer + `,ip=~"$ip"}`
		baseJSON  = `{unit="dhcp-lease-observer.service"} | json | __error__=""`
	)

	d := dashboardv2.NewDashboardBuilder("DHCP Leases V2 PoC").
		Tags([]string{"dhcp", "dns", "infrastructure", "poc"}).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("60s")).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("datasource").Label("Datasource").PluginId("prometheus")).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("loki_datasource").Label("Loki Datasource").PluginId("loki")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("ip").Label("Client IP").Query(prometheusVariableQuery{"$datasource", `label_values(dhcp_lease_observer_lease_info, ip)`}).Definition(`label_values(dhcp_lease_observer_lease_info, ip)`).Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).Sort(dashboardv2.VariableSortAlphabeticalAsc).Multi(true).IncludeAll(true).AllValue(".+")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("device").Label("Device ID").Query(prometheusVariableQuery{"$datasource", `label_values(dhcp_lease_observer_lease_info, device_id)`}).Definition(`label_values(dhcp_lease_observer_lease_info, device_id)`).Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).Sort(dashboardv2.VariableSortAlphabeticalAsc).Multi(true).IncludeAll(true).AllValue(".+"))

	d.Panel("panel-2", dashboardv2.NewPanelBuilder().Id(2).Title("Collection Health").Description("One when the last poll read the router; zero when it fell back to the last good snapshot.").Visualization(stat.NewVisualizationV2Builder().Unit("short").Thresholds(healthThresholds).Orientation(common.VizOrientationAuto)).Data(promData(promDS, `dhcp_lease_observer_up{`+observer+`}`, "{{source_instance}}")))
	d.Panel("panel-3", dashboardv2.NewPanelBuilder().Id(3).Title("Bound Leases").Visualization(stat.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(measurementThresholds).Orientation(common.VizOrientationAuto)).Data(promData(promDS, `dhcp_lease_observer_leases{`+observer+`,state="bound"}`, "{{source_instance}}")))
	d.Panel("panel-4", dashboardv2.NewPanelBuilder().Id(4).Title("Snapshot Age").Description("Time since the last successful lease collection. Historical attribution degrades as this grows.").Visualization(stat.NewVisualizationV2Builder().Unit("s").Min(0).Thresholds(snapshotThresholds).Orientation(common.VizOrientationAuto)).Data(promData(promDS, `time() - dhcp_lease_observer_last_success_timestamp_seconds{`+observer+`}`, "{{source_instance}}")))
	d.Panel("panel-5", dashboardv2.NewPanelBuilder().Id(5).Title("Poll Duration").Visualization(stat.NewVisualizationV2Builder().Unit("s").Min(0).Decimals(2).Thresholds(measurementThresholds).Orientation(common.VizOrientationAuto)).Data(promData(promDS, `dhcp_lease_observer_collection_duration_seconds{`+observer+`}`, "{{source_instance}}")))
	d.Panel("panel-7", dashboardv2.NewPanelBuilder().Id(7).Title("Current Leases").Description("Last good snapshot of the router lease table. Device IDs are opaque HMACs; no MAC or hostname is collected.").Visualization(table.NewVisualizationV2Builder()).Data(dashboardv2.NewQueryGroupBuilder().Target(dashboardv2.NewTargetBuilder().RefId("A").Query(prometheus.NewQueryV2Builder().Datasource(promDS).Expr(leaseInfo).Instant(true).Range(false).Format(prometheus.PromQueryFormatTable))).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{"excludeByName": map[string]any{"Time": true, "Value": true, "__name__": true, "instance": true, "job": true, "service": true}, "indexByName": map[string]any{"ip": 0, "device_id": 1, "state": 2, "scope": 3, "source_instance": 4}, "renameByName": map[string]any{"ip": "IP", "device_id": "Device ID", "state": "State", "scope": "Scope", "source_instance": "Router"}})).Transformation(dashboardv2.NewTransformationBuilder().Group("sortBy").Options(map[string]any{"sort": []any{map[string]any{"field": "IP"}}}))))
	d.Panel("panel-8", dashboardv2.NewPanelBuilder().Id(8).Title("Bound Lease Count").Visualization(timeseries.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(measurementThresholds).Tooltip(tooltip).Legend(legend)).Data(promData(promDS, `dhcp_lease_observer_leases{`+observer+`,state="bound"}`, "{{source_instance}}")))
	d.Panel("panel-10", dashboardv2.NewPanelBuilder().Id(10).Title("IP Assignments by Device").Description("Each series is one IP a device held, so a DHCP move appears as one series ending and another starting.").Visualization(timeseries.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(measurementThresholds).Tooltip(tooltip).Legend(legend)).Data(promData(promDS, `dhcp_lease_observer_lease_info{`+observer+`,device_id=~"$device"}`, "{{device_id}} {{ip}}")))
	d.Panel("panel-12", dashboardv2.NewPanelBuilder().Id(12).Title("Lease Events by Type").Description("Transition events only; a poll that changes nothing stays silent.").Visualization(timeseries.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(measurementThresholds).Tooltip(tooltip).Legend(legend).DrawStyle(common.GraphDrawStyleBars).FillOpacity(100).GradientMode(common.GraphGradientModeHue).Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal))).Data(dashboardv2.NewQueryGroupBuilder().QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m")).Target(dashboardv2.NewTargetBuilder().RefId("A").Query(loki.NewQueryV2Builder().Datasource(lokiDS).Expr(`sum by (event) (count_over_time(`+baseJSON+` | event =~ "lease_.*" | ip =~ "$ip" [$__auto]))`).LegendFormat("{{event}}")))))
	d.Panel("panel-13", dashboardv2.NewPanelBuilder().Id(13).Title("Lease Events").Visualization(logs.NewVisualizationV2Builder().ShowTime(true).SortOrder(common.LogsSortOrderDescending).EnableLogDetails(true).ShowLogContextToggle(true).ShowControls(true).ShowFieldSelector(true)).Data(dashboardv2.NewQueryGroupBuilder().Target(dashboardv2.NewTargetBuilder().RefId("A").Query(loki.NewQueryV2Builder().Datasource(lokiDS).Expr(baseJSON+` | ip =~ "$ip" | device_id =~ "$device" | line_format "{{.event}} {{.ip}} {{.device_id}} {{.state}}"`).MaxLines(500)))))

	d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Collector Status").GridLayout(dashboardv2.NewGridBuilder().Item(dashboardv2.GridItem("panel-2").X(0).Y(0).Width(6).Height(4)).Item(dashboardv2.GridItem("panel-3").X(6).Y(0).Width(6).Height(4)).Item(dashboardv2.GridItem("panel-4").X(12).Y(0).Width(6).Height(4)).Item(dashboardv2.GridItem("panel-5").X(18).Y(0).Width(6).Height(4)))).
		Row(dashboardv2.NewRowBuilder().Title("Lease Inventory").GridLayout(dashboardv2.NewGridBuilder().Item(dashboardv2.GridItem("panel-7").X(0).Y(0).Width(12).Height(9)).Item(dashboardv2.GridItem("panel-8").X(12).Y(0).Width(12).Height(9)))).
		Row(dashboardv2.NewRowBuilder().Title("Device Drill-down").GridLayout(dashboardv2.NewGridBuilder().Item(dashboardv2.GridItem("panel-10").X(0).Y(0).Width(24).Height(8)))).
		Row(dashboardv2.NewRowBuilder().Title("Lease Events").GridLayout(dashboardv2.NewGridBuilder().Item(dashboardv2.GridItem("panel-12").X(0).Y(0).Width(24).Height(8)).Item(dashboardv2.GridItem("panel-13").X(0).Y(8).Width(24).Height(12)))))
	result, err := d.Build()
	return &result, err
}

func promData(ds *dashboardv2.Dashboardv2DataQueryKindDatasourceBuilder, expr, legend string) *dashboardv2.QueryGroupBuilder {
	return dashboardv2.NewQueryGroupBuilder().Target(dashboardv2.NewTargetBuilder().RefId("A").Query(prometheus.NewQueryV2Builder().Datasource(ds).Expr(expr).LegendFormat(legend)))
}
