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
//
// TrueNAS owns a passed-through SATA controller, so its disks never appear in
// any node-exporter smartmon scrape; a smartctl_exporter app inside the TrueNAS
// guest exposes smartctl_* series instead (per-disk `device` label, model via
// the smartctl_device info metric). TrueNAS is not part of the
// node-exporter-external job that feeds the $node/$instance variables, so its
// queries filter by job statically and ignore the node variable.
func buildDiskHealth() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		instFilter = `instance=~"$instance"`
		// truenasFilter selects the smartctl_exporter scrape (TrueNAS only).
		truenasFilter = `job="scrapeConfig/monitoring/smartctl-exporter-external"`
		// joinSmartctlModel copies the disk model from the smartctl_device info
		// metric onto smartctl_* series so legends identify physical disks.
		joinSmartctlModel = `* on(instance, device) group_left(model_name) smartctl_device`
		// joinNodename copies nodename onto smartmon series so legends show hostnames.
		// max by deduplicates node_uname_info if scraped by multiple jobs.
		joinNodename = `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info)`
		// joinModel / joinNvmeModel copy the device model onto legends so each disk
		// is identifiable beyond its /dev path. SATA models come from
		// smartmon_device_info (label device_model), NVMe from nvme_device_info (model).
		joinModel     = `* on(instance, disk) group_left(device_model) smartmon_device_info`
		joinNvmeModel = `* on(instance, device) group_left(model) nvme_device_info`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// Any nonzero count of reallocated/pending/uncorrectable sectors is a strong
	// failure precursor, so the threshold flips to red at 1.
	precursorThresholds := issueThresholds()
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
		Time("now-30d", "now").
		Refresh("5m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
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
		// Cross-type unhealthy rollup, sourced consistently with the SMART Health
		// panel: SATA from the smartctl health flag, NVMe from critical_warning,
		// TrueNAS from smartctl_exporter's smart_status. `or vector(0)` keeps each
		// side at 0 (not "no data") when a node filter selects only one disk type.
		// The TrueNAS term is intentionally outside the $instance filter (its
		// instance is never a variable option), so it is always counted.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unhealthy Disks").
				Datasource(ds).
				Span(6).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(issueThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(sum(smartmon_device_smart_healthy{type="sat",` + instFilter + `} == bool 0) or vector(0)) + (sum(nvme_critical_warning{` + instFilter + `} > bool 0) or vector(0)) + (sum(smartctl_device_smart_status{` + truenasFilter + `} == bool 0) or vector(0))`).
					Instant().
					LegendFormat("Unhealthy"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Disks Monitored").
				Datasource(ds).
				Span(6).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "blue"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(count(smartmon_device_smart_healthy{` + instFilter + `}) or vector(0)) + (count(smartctl_device_smart_status{` + truenasFilter + `}) or vector(0))`).
					Instant().
					LegendFormat("Disks"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("SSDs Worn (<10% life)").
				Datasource(ds).
				Span(6).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(issueThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum((` +
						`smartmon_wear_leveling_count_value{` + instFilter + `}` +
						` or smartmon_media_wearout_indicator_value{` + instFilter + `}` +
						` or smartmon_ssd_life_left_value{` + instFilter + `}` +
						` or smartmon_percent_lifetime_remain_value{` + instFilter + `}` +
						`) < bool 10)`).
					Instant().
					LegendFormat("Worn"),
				),
		).
		// NVMe counterpart to "SSDs Worn": critical_warning is a bitfield, so any
		// nonzero bit (>0) on any NVMe device is surfaced here for parity with the
		// SATA wear tile. The per-disk SMART Health panel below covers both types.
		WithPanel(
			stat.NewPanelBuilder().
				Title("NVMe Warnings").
				Datasource(ds).
				Span(6).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(issueThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(nvme_critical_warning{` + instFilter + `} > bool 0)`).
					Instant().
					LegendFormat("Warnings"),
				),
		).
		// Per-disk health. SATA uses the smartctl overall-health flag. NVMe is
		// sourced from nvme_critical_warning instead: the smartmon textfile script
		// is ATA-centric and reports smart_available=0/enabled=0 for NVMe, so the
		// nvme exporter's critical_warning byte is the authoritative health signal
		// (== 0 means no warning bits set, i.e. OK).
		WithPanel(
			stat.NewPanelBuilder().
				Title("SMART Health").
				Datasource(ds).
				Span(24).Height(6).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
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
				// Instant queries: evaluate current health only. A range query over a
				// long window (e.g. 30d) would surface stale label combinations — NVMe
				// device names (nvme0n1/nvme1n1) can swap across reboots, so joining
				// historical device_info produces a device×model cross product.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_device_smart_healthy{type="sat",` + instFilter + `} ` + joinNodename + ` ` + joinModel).
					Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}} (SATA)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_critical_warning{` + instFilter + `} == bool 0) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					Instant().
					LegendFormat("{{nodename}} {{device}} {{model}} (NVMe)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartctl_device_smart_status{` + truenasFilter + `} ` + joinSmartctlModel).
					Instant().
					LegendFormat("{{instance}} {{device}} {{model_name}} (SATA)"),
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
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_reallocated_sector_ct_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel + `)`).Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Current Pending Sectors").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_current_pending_sector_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel + `)`).Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Offline Uncorrectable").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_offline_uncorrectable_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel + `)`).Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Reported Uncorrectable").
				Datasource(ds).
				Span(12).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_reported_uncorrect_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel + `)`).Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("UDMA CRC Errors").
				Datasource(ds).
				Span(12).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(crcThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartmon_udma_crc_error_count_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel + `)`).Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
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
		// Vendor-normalized SSD wear indicators. Each vendor exposes a different
		// attribute, but all are normalized values where 100 = new and the number
		// decreases toward 0 with use, so they can be unioned with `or`. Only SSDs
		// that publish one of these appear here; HDDs have no wear concept and NVMe
		// is covered by nvme_percentage_used_ratio in the NVMe row below. Referencing
		// an attribute that no disk reports is harmless (it just yields no series).
		WithRow(dashboard.NewRowBuilder("Wear & Lifetime")).
		// Rendered as a stat (not bargauge): this metric usually has a single
		// reporting disk. TextMode value_and_name forces the device name to show
		// even for one series, which "auto" mode would otherwise hide. Each disk
		// appears as its own colored tile (red <10, yellow <20, green).
		WithPanel(
			stat.NewPanelBuilder().
				Title("SSD Life Remaining (vendor wear attr)").
				Description("Normalized SSD wear (100 = new). Sourced from whichever vendor " +
					"attribute a disk exposes (wear_leveling_count, media_wearout_indicator, " +
					"ssd_life_left, percent_lifetime_remain). SSDs that expose none, plus all " +
					"HDDs, do not appear; NVMe endurance is shown in the NVMe row.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: float64Ptr(10), Color: "yellow"},
						{Value: float64Ptr(20), Color: "green"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(` +
						`smartmon_wear_leveling_count_value{` + instFilter + `}` +
						` or smartmon_media_wearout_indicator_value{` + instFilter + `}` +
						` or smartmon_ssd_life_left_value{` + instFilter + `}` +
						` or smartmon_percent_lifetime_remain_value{` + instFilter + `}` +
						`) ` + joinNodename + ` ` + joinModel).
					Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
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
				Orientation(common.VizOrientationHorizontal).
				TextMode(common.BigValueTextModeValueAndName).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_power_on_hours_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel).
					Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				).Decimals(0),
		).
		// NVMe drives don't expose the SATA SMART attributes; instead the nvme-cli
		// exporter publishes nvme_* series keyed by `device` (e.g. nvme0n1).
		// "Data Units Written/Read" follow the NVMe spec unit of 1000 x 512 bytes,
		// so bytes = value * 512000 (a documented approximation, not exact host I/O).
		WithRow(dashboard.NewRowBuilder("NVMe")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Endurance Used (%)").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(80), Color: "yellow"},
						{Value: float64Ptr(100), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_percentage_used_ratio{` + instFilter + `} * 100) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Available Spare (%)").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: float64Ptr(10), Color: "yellow"},
						{Value: float64Ptr(20), Color: "green"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_available_spare_ratio{` + instFilter + `} * 100) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(0),
		).
		// critical_warning is a bitfield; any nonzero bit indicates a fault.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Critical Warning").
				Datasource(ds).
				Span(8).Height(8).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Text(diskHealthLabelText()).
				Thresholds(issueThresholds()).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: strPtr("OK"), Color: strPtr("green")},
						},
					}},
				}).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_critical_warning{` + instFilter + `} ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Data Written (TBW)").
				Datasource(ds).
				Span(8).Height(8).
				Unit("bytes").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				Text(diskHealthLabelText()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_data_units_written_total{` + instFilter + `} * 512000) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Data Read").
				Datasource(ds).
				Span(8).Height(8).
				Unit("bytes").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				Text(diskHealthLabelText()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_data_units_read_total{` + instFilter + `} * 512000) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power On Hours").
				Datasource(ds).
				Span(8).Height(8).
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				Text(diskHealthLabelText()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_power_on_hours_total{` + instFilter + `} ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(0),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Write / Read Throughput").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Write").
					Expr(`(rate(nvme_data_units_written_total{`+instFilter+`}[$__rate_interval]) * 512000) `+joinNodename).
					LegendFormat("{{nodename}} {{device}} Write"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Read").
					Expr(`(rate(nvme_data_units_read_total{`+instFilter+`}[$__rate_interval]) * 512000) `+joinNodename).
					LegendFormat("{{nodename}} {{device}} Read"),
				).
				OverrideByQuery("Read", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		// Media errors and error-log entries should stay flat; any growth is a fault.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Media & Error-Log Entries").
				Datasource(ds).
				Span(24).Height(8).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_media_errors_total{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{device}} media errors"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_num_err_log_entries_total{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{device}} err-log entries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_unsafe_shutdowns_total{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{device}} unsafe shutdowns"),
				),
		).
		// TrueNAS disks behind the passed-through SATA controller, read by
		// smartctl_exporter inside the guest. Overall health is covered by the
		// shared SMART Health panel above; this row holds the SATA failure
		// precursors and exporter diagnostics. attribute_value_type="raw"
		// mirrors the smartmon *_raw_value series used for the other nodes.
		WithRow(dashboard.NewRowBuilder("TrueNAS (smartctl_exporter)")).
		// Nonzero exit status means smartctl could not read a disk: either a
		// real failure or the disk was skipped in standby (powermode-check),
		// in which case the other panels in this row go stale until it wakes.
		WithPanel(
			stat.NewPanelBuilder().
				Title("smartctl Exit Status").
				Description("0 = disk read OK. Nonzero means smartctl failed or the disk " +
					"was skipped in standby; the series in this row then stop updating.").
				Datasource(ds).
				Span(24).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Thresholds(issueThresholds()).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: strPtr("OK"), Color: strPtr("green")},
						},
					}},
				}).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartctl_device_smartctl_exit_status{` + truenasFilter + `}`).
					Instant().
					LegendFormat("{{instance}} {{device}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Failure Precursors").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(issueThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartctl_device_attribute{` + truenasFilter + `,attribute_value_type="raw",attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector|Offline_Uncorrectable|Reported_Uncorrect"})`).
					Instant().
					LegendFormat("{{device}} {{attribute_name}}"),
				),
		).
		// CRC errors warn (yellow) not alert (red), same as the smartmon panel.
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("UDMA CRC Errors").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(1), Color: "yellow"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartctl_device_attribute{` + truenasFilter + `,attribute_value_type="raw",attribute_name="UDMA_CRC_Error_Count"})`).
					Instant().
					LegendFormat("{{device}} {{attribute_name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power On Hours").
				Datasource(ds).
				Span(8).Height(8).
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationHorizontal).
				TextMode(common.BigValueTextModeValueAndName).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(smartctl_device_power_on_seconds{` + truenasFilter + `} / 3600) ` + joinSmartctlModel).
					Instant().
					LegendFormat("{{device}} {{model_name}}"),
				).Decimals(0),
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
					Expr(`smartctl_device_attribute{` + truenasFilter + `,attribute_value_type="raw",attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector"}`).
					LegendFormat("{{device}} {{attribute_name}}"),
				),
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
				// Timeseries join nodename only (no model): a model join over a long
				// range cross-products with stale device_info, like the throughput and
				// media-error panels above.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_temperature_celsius_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}} (SATA)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_temperature_celsius{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{device}} (NVMe)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartctl_device_temperature{` + truenasFilter + `,temperature_type="current"}`).
					LegendFormat("{{instance}} {{device}} (SATA)"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

func diskHealthLabelText() *common.VizTextDisplayOptionsBuilder {
	return common.NewVizTextDisplayOptionsBuilder().TitleSize(16)
}
