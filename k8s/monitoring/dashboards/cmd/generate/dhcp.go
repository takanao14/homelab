package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDhcpLeases correlates DNS activity with the IX2106 lease table. The
// collector exports opaque device IDs only, so vendor and MAC panels are not
// available here by design.
func buildDhcpLeases() (*dashboard.Dashboard, error) {
	ds := promDatasource()
	lokiType := "loki"
	lokiUID := "$loki_datasource"
	lokiDS := common.DataSourceRef{Type: &lokiType, Uid: &lokiUID}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		observer  = `job="scrapeConfig/monitoring/node-exporter-external"`
		leaseInfo = `dhcp_lease_observer_lease_info{` + observer + `,ip=~"$ip"}`
		baseJSON  = `{unit="dhcp-lease-observer.service"} | json | __error__=""`
	)

	healthThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})

	// Match the DHCPLeaseObserverSnapshotStale alert, then a second window.
	snapshotAgeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1800)), Color: "yellow"},
			{Value: new(float64(3600)), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("DHCP Leases").
		Uid("dhcp-leases").
		Tags([]string{"dhcp", "dns", "infrastructure"}).
		Timezone("browser").
		Time("now-6h", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("loki_datasource").
				Label("Loki Datasource").
				Type("loki"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("ip").
				Label("Client IP").
				Datasource(ds).
				Query(dashboard.StringOrMap{
					String: new(`label_values(dhcp_lease_observer_lease_info, ip)`),
				}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// Use .+ for All so released IPs stay in scope for log filters.
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("device").
				Label("Device ID").
				Datasource(ds).
				Query(dashboard.StringOrMap{
					String: new(`label_values(dhcp_lease_observer_lease_info, device_id)`),
				}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				AllValue(".+"),
		).
		WithRow(dashboard.NewRowBuilder("Collector Status")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Collection Health").
				Description("One when the last poll read the router; zero when it fell back to the last good snapshot.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(healthThresholds).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dhcp_lease_observer_up{` + observer + `}`).
					LegendFormat("{{source_instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Bound Leases").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dhcp_lease_observer_leases{` + observer + `,state="bound"}`).
					LegendFormat("{{source_instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Snapshot Age").
				Description("Time since the last successful lease collection. Historical attribution degrades as this grows.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				Min(0).
				Thresholds(snapshotAgeThresholds).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`time() - dhcp_lease_observer_last_success_timestamp_seconds{` + observer + `}`).
					LegendFormat("{{source_instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Poll Duration").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				Min(0).
				Decimals(2).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dhcp_lease_observer_collection_duration_seconds{` + observer + `}`).
					LegendFormat("{{source_instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Lease Inventory")).
		WithPanel(
			table.NewPanelBuilder().
				Title("Current Leases").
				Description("Last good snapshot of the router lease table. Device IDs are opaque HMACs; no MAC or hostname is collected.").
				Datasource(ds).
				Span(12).Height(9).
				WithTarget(prometheus.NewDataqueryBuilder().
					// sort_by_label needs promql-experimental-functions, so the
					// table transformation orders the rows instead.
					Expr(leaseInfo).
					Instant().Format(prometheus.PromQueryFormatTable),
				).
				WithTransformation(dashboard.DataTransformerConfig{
					Id: "organize",
					Options: map[string]any{
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
					},
				}).
				WithTransformation(dashboard.DataTransformerConfig{
					Id: "sortBy",
					Options: map[string]any{
						"sort": []any{map[string]any{"field": "IP"}},
					},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Bound Lease Count").
				Datasource(ds).
				Span(12).Height(9).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dhcp_lease_observer_leases{` + observer + `,state="bound"}`).
					LegendFormat("{{source_instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Device Drill-down")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("IP Assignments by Device").
				Description("Each series is one IP a device held, so a DHCP move appears as one series ending and another starting.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dhcp_lease_observer_lease_info{` + observer + `,device_id=~"$device"}`).
					LegendFormat("{{device_id}} {{ip}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Lease Events")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Lease Events by Type").
				Description("Transition events only; a poll that changes nothing stays silent.").
				Datasource(lokiDS).
				Span(24).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Thresholds(measurementThresholds()).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (event) (count_over_time(` + baseJSON +
						` | event =~ "lease_.*" | ip =~ "$ip" [$__auto]))`).
					LegendFormat("{{event}}"),
				),
		).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Lease Events").
				Datasource(lokiDS).
				Span(24).Height(12).
				ShowTime(true).
				SortOrder(common.LogsSortOrderDescending).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(baseJSON + ` | ip =~ "$ip" | device_id =~ "$device"` +
						` | line_format "{{.event}} {{.ip}} {{.device_id}} {{.state}}"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
