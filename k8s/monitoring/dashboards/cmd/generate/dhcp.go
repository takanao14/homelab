package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	dhcpPanelCollectionHealth      = 1
	dhcpPanelBoundLeases           = 2
	dhcpPanelSnapshotAge           = 3
	dhcpPanelPollDuration          = 4
	dhcpPanelCurrentLeases         = 5
	dhcpPanelBoundLeaseCount       = 6
	dhcpPanelIPAssignmentsByDevice = 7
	dhcpPanelLeaseEventsByType     = 8
	dhcpPanelLeaseEvents           = 9
)

// buildDhcpLeases correlates DNS activity with the IX2106 lease table. The
// collector exports opaque device IDs only, so vendor and MAC panels are not
// available here by design.
func buildDhcpLeases() (*dashboardv2.Dashboard, error) {
	const (
		elementCollectionHealth      = "collection-health"
		elementBoundLeases           = "bound-leases"
		elementSnapshotAge           = "snapshot-age"
		elementPollDuration          = "poll-duration"
		elementCurrentLeases         = "current-leases"
		elementBoundLeaseCount       = "bound-lease-count"
		elementIPAssignmentsByDevice = "ip-assignments-by-device"
		elementLeaseEventsByType     = "lease-events-by-type"
		elementLeaseEvents           = "lease-events"
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		observer  = `job="scrapeConfig/monitoring/node-exporter-external"`
		leaseInfo = `dhcp_lease_observer_lease_info{` + observer + `,ip=~"$ip"}`
		baseJSON  = `{unit="dhcp-lease-observer.service"} | json | __error__=""`
	)

	healthThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})

	// Match the DHCPLeaseObserverSnapshotStale alert, then a second window.
	snapshotAgeThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1800)), Color: "yellow"},
			{Value: new(float64(3600)), Color: "red"},
		})

	d := dashboardv2.NewDashboardBuilder("DHCP Leases").
		Tags([]string{"dhcp", "dns", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("60s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("loki_datasource").
			Label("Loki Datasource").
			PluginId("loki")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("ip").
			Label("Client IP").
			Query(promVariableQuery(`label_values(dhcp_lease_observer_lease_info, ip)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			// Use .+ for All so released IPs stay in scope for log filters.
			AllValue(".+")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("device").
			Label("Device ID").
			Query(promVariableQuery(`label_values(dhcp_lease_observer_lease_info, device_id)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			AllValue(".+"))

	d.Panel(elementCollectionHealth, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelCollectionHealth).
		Title("Collection Health").
		Description("One when the last poll read the router; zero when it fell back to the last good snapshot.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Thresholds(healthThresholds).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`dhcp_lease_observer_up{`+observer+`}`).
				LegendFormat("{{source_instance}}")))))

	d.Panel(elementBoundLeases, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelBoundLeases).
		Title("Bound Leases").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`dhcp_lease_observer_leases{`+observer+`,state="bound"}`).
				LegendFormat("{{source_instance}}")))))

	d.Panel(elementSnapshotAge, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelSnapshotAge).
		Title("Snapshot Age").
		Description("Time since the last successful lease collection. Historical attribution degrades as this grows.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Thresholds(snapshotAgeThresholds).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`time() - dhcp_lease_observer_last_success_timestamp_seconds{`+observer+`}`).
				LegendFormat("{{source_instance}}")))))

	d.Panel(elementPollDuration, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelPollDuration).
		Title("Poll Duration").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Decimals(2).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`dhcp_lease_observer_collection_duration_seconds{`+observer+`}`).
				LegendFormat("{{source_instance}}")))))

	d.Panel(elementCurrentLeases, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelCurrentLeases).
		Title("Current Leases").
		Description("Last good snapshot of the router lease table. Device IDs are opaque HMACs; no MAC or hostname is collected.").
		Visualization(table.NewVisualizationV2Builder()).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				// sort_by_label needs promql-experimental-functions, so the
				// table transformation orders the rows instead.
				Expr(leaseInfo).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable))).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
			"excludeByName": map[string]any{
				"Time":     true,
				"Value":    true,
				"__name__": true,
				"instance": true,
				"job":      true,
				"service":  true,
			},
			"indexByName": map[string]any{
				"ip":              0,
				"device_id":       1,
				"state":           2,
				"scope":           3,
				"source_instance": 4,
			},
			"renameByName": map[string]any{
				"ip":              "IP",
				"device_id":       "Device ID",
				"state":           "State",
				"scope":           "Scope",
				"source_instance": "Router",
			},
		})).Transformation(dashboardv2.NewTransformationBuilder().Group("sortBy").Options(map[string]any{
			"sort": []any{map[string]any{"field": "IP"}},
		}))))

	d.Panel(elementBoundLeaseCount, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelBoundLeaseCount).
		Title("Bound Lease Count").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`dhcp_lease_observer_leases{`+observer+`,state="bound"}`).
				LegendFormat("{{source_instance}}")))))

	d.Panel(elementIPAssignmentsByDevice, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelIPAssignmentsByDevice).
		Title("IP Assignments by Device").
		Description("Each series is one IP a device held, so a DHCP move appears as one series ending and another starting.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`dhcp_lease_observer_lease_info{`+observer+`,device_id=~"$device"}`).
				LegendFormat("{{device_id}} {{ip}}")))))

	d.Panel(elementLeaseEventsByType, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelLeaseEventsByType).
		Title("Lease Events by Type").
		Description("Transition events only; a poll that changes nothing stays silent.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Thresholds(measurementThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().Datasource(queryDatasource("loki_datasource")).
				Expr(`sum by (event) (count_over_time(`+baseJSON+
					` | event =~ "lease_.*" | ip =~ "$ip" [$__auto]))`).
				LegendFormat("{{event}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementLeaseEvents, dashboardv2.NewPanelBuilder().
		Id(dhcpPanelLeaseEvents).
		Title("Lease Events").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			SortOrder(common.LogsSortOrderDescending).
			EnableLogDetails(true).
			ShowLogContextToggle(true).
			ShowControls(true).
			ShowFieldSelector(true)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(lokiQuery().Datasource(queryDatasource("loki_datasource")).
				Expr(baseJSON+` | ip =~ "$ip" | device_id =~ "$device"`+
					` | line_format "{{.event}} {{.ip}} {{.device_id}} {{.state}}"`).
				MaxLines(500)))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Collector Status").AutoGridLayout(statGrid(4).
			WithItem(elementCollectionHealth).
			WithItem(elementBoundLeases).
			WithItem(elementSnapshotAge).
			WithItem(elementPollDuration))).
		Row(dashboardv2.NewRowBuilder().Title("Current Leases").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCurrentLeases))).
		Row(dashboardv2.NewRowBuilder().Title("Bound Lease Count").AutoGridLayout(chartGrid().
			WithItem(elementBoundLeaseCount))).
		Row(dashboardv2.NewRowBuilder().Title("Device Drill-down").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementIPAssignmentsByDevice))).
		Row(dashboardv2.NewRowBuilder().Title("Lease Events by Type").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementLeaseEventsByType))).
		Row(dashboardv2.NewRowBuilder().Title("Lease Events").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementLeaseEvents)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
