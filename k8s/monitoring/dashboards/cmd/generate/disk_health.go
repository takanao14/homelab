package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDiskHealth renders physical disk S.M.A.R.T. health collected by the
// node-exporter textfile smartmon collector. smartmon_* series carry a `disk`
// (e.g. /dev/sda) and `type` (sat|nvme) label plus the node-exporter `instance`,
// which is joined to node_uname_info to resolve a human-readable nodename.
//
// Only SATA/ATA disks (type="sat") expose the per-attribute SMART counters
// (reallocated sectors, pending sectors, wear leveling, etc.). NVMe disks only
// report the overall smartmon_device_smart_healthy flag, so they appear in the
// health summary but not in the SATA-specific precursor panels.
func buildDiskHealth() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		instFilter = `instance=~"$instance"`
		// joinNodename copies nodename onto smartmon series so legends show hostnames.
		// max by deduplicates node_uname_info if scraped by multiple jobs.
		joinNodename = `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info)`
	)

	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
	legend := common.NewVizLegendOptionsBuilder().
		ShowLegend(true).
		DisplayMode(common.LegendDisplayModeList).
		Placement(common.LegendPlacementBottom)

	// Any nonzero count of reallocated/pending/uncorrectable sectors is a strong
	// failure precursor, so the threshold flips to red at 1.
	precursorThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "red"},
		})
	// CRC errors usually indicate a cabling/connection issue rather than imminent
	// media failure, so they warn (yellow) rather than alert (red).
	crcThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "yellow"},
		})

	d, err := dashboard.NewDashboardBuilder("Disk Health (S.M.A.R.T.)").
		Uid("disk-health").
		Tags([]string{"disk", "smart", "infrastructure"}).
		Timezone("browser").
		Time("now-7d", "now").
		Refresh("5m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		// Reuse the bare-metal node variable convention from node-overview:
		// expose $node (nodename) and resolve it to the hidden $instance (IP:port).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename!="gpuvm"}, nodename)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename!="gpuvm",nodename=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		// == bool 0 yields 1 per unhealthy disk; sum returns 0 (not "no data")
		// while at least one disk is reporting, so the panel stays meaningful.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unhealthy Disks").
				Datasource(ds).
				Span(8).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(1), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(smartmon_device_smart_healthy{` + instFilter + `} == bool 0)`).
					LegendFormat("Unhealthy"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Disks Monitored").
				Datasource(ds).
				Span(8).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "blue"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(smartmon_device_smart_healthy{` + instFilter + `})`).
					LegendFormat("Disks"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("SSDs Worn (<10% life)").
				Datasource(ds).
				Span(8).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(1), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(smartmon_wear_leveling_count_value{` + instFilter + `} < bool 10)`).
					LegendFormat("Worn"),
				),
		).
		// Per-disk health flag for both SATA and NVMe devices.
		WithPanel(
			stat.NewPanelBuilder().
				Title("SMART Health").
				Datasource(ds).
				Span(24).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: float64Ptr(1), Color: "green"},
					})).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: strPtr("FAIL"), Color: strPtr("red")},
							"1": {Text: strPtr("OK"), Color: strPtr("green")},
						},
					}},
				}).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_device_smart_healthy{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}} ({{type}})"),
				),
		).
		// SATA-only failure precursors. These should sit flat at 0; any step up is
		// the signal that the disk is starting to fail.
		WithRow(dashboard.NewRowBuilder("Failure Precursors (SATA)")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Reallocated Sectors").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_reallocated_sector_ct_raw_value{` + instFilter + `} ` + joinNodename + `)`).
					LegendFormat("{{nodename}} {{disk}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Current Pending Sectors").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_current_pending_sector_raw_value{` + instFilter + `} ` + joinNodename + `)`).
					LegendFormat("{{nodename}} {{disk}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Offline Uncorrectable").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_offline_uncorrectable_raw_value{` + instFilter + `} ` + joinNodename + `)`).
					LegendFormat("{{nodename}} {{disk}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Reported Uncorrectable").
				Datasource(ds).
				Span(12).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_reported_uncorrect_raw_value{` + instFilter + `} ` + joinNodename + `)`).
					LegendFormat("{{nodename}} {{disk}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("UDMA CRC Errors").
				Datasource(ds).
				Span(12).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(crcThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_udma_crc_error_count_raw_value{` + instFilter + `} ` + joinNodename + `)`).
					LegendFormat("{{nodename}} {{disk}}"),
				),
		).
		// Trend view so a precursor stepping off zero is visible historically.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Pending / Reallocated Sector Trend").
				Datasource(ds).
				Span(24).Height(8).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_reallocated_sector_ct_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}} reallocated"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_current_pending_sector_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}} pending"),
				),
		).
		// Wear leveling normalized value: 100 = new, decreasing toward 0 with use.
		WithRow(dashboard.NewRowBuilder("Wear & Lifetime")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("SSD Life Remaining (normalized)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: float64Ptr(10), Color: "yellow"},
						{Value: float64Ptr(20), Color: "green"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_wear_leveling_count_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}}"),
				).Decimals(0),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power On Hours").
				Datasource(ds).
				Span(12).Height(8).
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_power_on_hours_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}}"),
				).Decimals(0),
		).
		WithRow(dashboard.NewRowBuilder("Temperature")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Disk Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_temperature_celsius_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
