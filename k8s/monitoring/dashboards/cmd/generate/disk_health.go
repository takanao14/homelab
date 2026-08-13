package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildDiskHealth covers physical disks from smartmon, nvme_exporter, and
// TrueNAS smartctl_exporter. SATA attributes require type="sat" because
// smartmon's type="scsi" entries are virtual disks with false health failures.
// NVMe health comes from nvme_* metrics; TrueNAS is statically scoped because
// it is outside the node-exporter variable inventory.
func buildDiskHealth() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		instFilter = `instance=~"$instance"`
		// Select the TrueNAS smartctl_exporter scrape.
		truenasFilter = `job="scrapeConfig/monitoring/smartctl-exporter-external"`
		// Add TrueNAS disk models to legends.
		joinSmartctlModel = `* on(instance, device) group_left(model_name) smartctl_device`
		// Add nodenames and deduplicate multiply scraped hosts.
		joinNodename = `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info)`
		// Add SATA/NVMe models so legends identify physical disks.
		joinModel     = `* on(instance, disk) group_left(device_model) smartmon_device_info`
		joinNvmeModel = `* on(instance, device) group_left(model) nvme_device_info`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// Any damaged sector is a strong failure precursor.
	precursorThresholds := issueThresholds()
	// CRC errors warn because they usually indicate cabling, not media failure.
	crcThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})
	// Summary conditions warn from one; only unhealthy disks are critical.
	noticeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	// Key NVMe range series by serial because nvmeXnY names can swap on reboot.
	// Drop `device` to avoid historical device/model cross-products; keep model
	// for readable legends. Instant panels retain the current device name.
	byNvmeDisk := func(expr string) string {
		return `max by (nodename, model, serial) (` + expr +
			` * on(instance, device) group_left(model, serial) nvme_device_info ` +
			joinNodename + `)`
	}

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
		// Reuse node-overview's nodename-to-instance variables.
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}, nodename)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		// Roll up SATA, NVMe, and TrueNAS health using each source's authoritative
		// signal. Keep TrueNAS outside the node filter and preserve zero values.
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
				Description("Number of disks whose health is evaluated, i.e. exactly the tiles in the SMART Health panel below.").
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
				// Count exactly the sources displayed by SMART Health, excluding virtual
				// SCSI disks and including NVMe.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(count(smartmon_device_smart_healthy{type="sat",` + instFilter + `}) or vector(0)) + (count(nvme_critical_warning{` + instFilter + `}) or vector(0)) + (count(smartctl_device_smart_status{` + truenasFilter + `}) or vector(0))`).
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
					// Preserve zero when selected disks expose no wear attribute.
					Expr(`sum((` +
						`smartmon_wear_leveling_count_value{` + instFilter + `}` +
						` or smartmon_media_wearout_indicator_value{` + instFilter + `}` +
						` or smartmon_ssd_life_left_value{` + instFilter + `}` +
						` or smartmon_percent_lifetime_remain_value{` + instFilter + `}` +
						`) < bool 10) or vector(0)`).
					Instant().
					LegendFormat("Worn"),
				),
		).
		// Count disks with any failure precursor once across all SATA attributes.
		// Standing damage warns; growth is alerted separately.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Disks with Failure Precursors").
				Description("Disks reporting any reallocated, pending, offline-uncorrectable or " +
					"reported-uncorrectable sectors. A nonzero count is not an outage: it means " +
					"those disks have remapped or failed to read sectors at some point. Growth is " +
					"what matters, and is alerted separately.").
				Datasource(ds).
				Span(6).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(noticeThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(count(max by (instance, disk) (` +
						`smartmon_reallocated_sector_ct_raw_value{` + instFilter + `}` +
						` or smartmon_current_pending_sector_raw_value{` + instFilter + `}` +
						` or smartmon_offline_uncorrectable_raw_value{` + instFilter + `}` +
						` or smartmon_reported_uncorrect_raw_value{` + instFilter + `}` +
						`) > 0) or vector(0))` +
						` + (count(max by (instance, device) (` +
						`smartctl_device_attribute{` + truenasFilter + `,attribute_value_type="raw",` +
						`attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector|Offline_Uncorrectable|Reported_Uncorrect"}` +
						`) > 0) or vector(0))`).
					Instant().
					LegendFormat("Precursors"),
				),
		).
		// SATA uses overall SMART health; NVMe uses critical_warning because the
		// ATA-centric smartmon collector does not provide NVMe health.
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
						{Value: new(float64(1)), Color: "green"},
					})).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: new("FAIL"), Color: new("red")},
							"1": {Text: new("OK"), Color: new("green")},
						},
					}},
				}).
				// Evaluate current health to avoid historical device/model cross-products.
				// Omit models so per-disk tiles remain readable.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_device_smart_healthy{type="sat",` + instFilter + `} ` + joinNodename).
					Instant().
					LegendFormat("{{nodename}} {{disk}} (SATA)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_critical_warning{` + instFilter + `} == bool 0) ` + joinNodename).
					Instant().
					LegendFormat("{{nodename}} {{device}} (NVMe)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartctl_device_smart_status{` + truenasFilter + `}`).
					Instant().
					LegendFormat("{{instance}} {{device}} (SATA)"),
				),
		).
		// Group SATA and NVMe failure precursors by operational question rather
		// than technology. TrueNAS remains separate because it ignores $node.
		WithRow(dashboard.NewRowBuilder("Failure Precursors")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Reallocated Sectors (SATA)").
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
				Title("Current Pending Sectors (SATA)").
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
				Title("Offline Uncorrectable (SATA)").
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
				Title("Reported Uncorrectable (SATA)").
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
				Title("UDMA CRC Errors (SATA)").
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
				Title("Pending / Reallocated Sector Trend (SATA)").
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
		// NVMe exposes media errors and error-log entries rather than SATA-style
		// sector attributes. Key history by serial across device-name swaps.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Media & Error-Log Entries (NVMe)").
				Datasource(ds).
				Span(24).Height(8).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byNvmeDisk(`nvme_media_errors_total{` + instFilter + `}`)).
					LegendFormat("{{nodename}} {{model}} media errors"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byNvmeDisk(`nvme_num_err_log_entries_total{` + instFilter + `}`)).
					LegendFormat("{{nodename}} {{model}} err-log entries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byNvmeDisk(`nvme_unsafe_shutdowns_total{` + instFilter + `}`)).
					LegendFormat("{{nodename}} {{model}} unsafe shutdowns"),
				),
		).
		// Place SATA remaining-life and NVMe used-life together, but keep separate
		// scales. Vendor SATA attributes normalize to 100=new.
		WithRow(dashboard.NewRowBuilder("Wear & Lifetime")).
		// Stat tiles force value and name text so the usually sparse, long disk
		// labels remain readable.
		WithPanel(
			stat.NewPanelBuilder().
				Title("SSD Life Remaining (SATA)").
				Description("Normalized SSD wear (100 = new). Sourced from whichever vendor " +
					"attribute a disk exposes (wear_leveling_count, media_wearout_indicator, " +
					"ssd_life_left, percent_lifetime_remain). SSDs that expose none, plus all " +
					"HDDs, do not appear; NVMe endurance is the panel to the right.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(10)), Color: "yellow"},
						{Value: new(float64(20)), Color: "green"},
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
			bargauge.NewPanelBuilder().
				Title("Endurance Used (NVMe)").
				Description("The drive's own estimate of write endurance consumed. Counts up, " +
					"unlike the SATA panel to the left, which counts down.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(80)), Color: "yellow"},
						{Value: new(float64(100)), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_percentage_used_ratio{` + instFilter + `} * 100) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Available Spare (NVMe)").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(10)), Color: "yellow"},
						{Value: new(float64(20)), Color: "green"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_available_spare_ratio{` + instFilter + `} * 100) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(0),
		).
		// Combine SATA and NVMe power-on hours; TrueNAS remains separately scoped.
		// Auto grid orientation keeps the larger disk set readable.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power On Hours").
				Description("How long each disk has been powered on. A figure to plan " +
					"replacements against, not a health signal, so it carries no threshold " +
					"colour: nothing here is good or bad on its own.").
				Datasource(ds).
				Span(24).Height(10).
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_power_on_hours_raw_value{` + instFilter + `} ` + joinNodename + ` ` + joinModel).
					Instant().
					LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_power_on_hours_total{` + instFilter + `} ` + joinNodename + ` ` + joinNvmeModel).
					Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(0),
		).
		// Remaining metrics are NVMe-specific. Data units use the specification's
		// approximate 512000-byte unit; critical_warning is a fault bitfield.
		WithRow(dashboard.NewRowBuilder("NVMe")).
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
							"0": {Text: new("OK"), Color: new("green")},
						},
					}},
				}).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`nvme_critical_warning{` + instFilter + `} ` + joinNodename).Instant().
					LegendFormat("{{nodename}} {{device}}"),
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
				Thresholds(measurementThresholds()).
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
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_data_units_read_total{` + instFilter + `} * 512000) ` + joinNodename + ` ` + joinNvmeModel).Instant().
					LegendFormat("{{nodename}} {{device}} {{model}}"),
				).Decimals(1),
		).
		// rate() runs before serial relabeling, so a device-name swap can create one
		// reboot-time spike. Clamping would also hide legitimate bursts.
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
					Expr(byNvmeDisk(`(rate(nvme_data_units_written_total{`+instFilter+`}[$__rate_interval]) * 512000)`)).
					LegendFormat("{{nodename}} {{model}} Write"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Read").
					Expr(byNvmeDisk(`(rate(nvme_data_units_read_total{`+instFilter+`}[$__rate_interval]) * 512000)`)).
					LegendFormat("{{nodename}} {{model}} Read"),
				).
				OverrideByQuery("Read", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		// TrueNAS smartctl_exporter covers passed-through SATA disks outside $node.
		// Overall health is shared above; this row shows precursors and read status.
		// Nonzero exit status means failure or standby skip.
		WithRow(dashboard.NewRowBuilder("TrueNAS Disks")).
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
							"0": {Text: new("OK"), Color: new("green")},
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
				Title("UDMA CRC Errors (TrueNAS)").
				Datasource(ds).
				Span(8).Height(8).
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(1)), Color: "yellow"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(smartctl_device_attribute{` + truenasFilter + `,attribute_value_type="raw",attribute_name="UDMA_CRC_Error_Count"})`).
					Instant().
					LegendFormat("{{device}} {{attribute_name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power On Hours (TrueNAS)").
				Datasource(ds).
				Span(8).Height(8).
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationHorizontal).
				TextMode(common.BigValueTextModeValueAndName).
				Text(diskHealthLabelText()).
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(smartctl_device_power_on_seconds{` + truenasFilter + `} / 3600) ` + joinSmartctlModel).
					Instant().
					LegendFormat("{{device}} {{model_name}}"),
				).Decimals(0),
		).
		// Trend view so a precursor stepping off zero is visible historically.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Pending / Reallocated Sector Trend (TrueNAS)").
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
				// SATA device names are stable here; avoid historical model joins.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_temperature_celsius_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}} (SATA)"),
				).
				// NVMe history uses serial identity across device-name swaps.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byNvmeDisk(`nvme_temperature_celsius{` + instFilter + `}`)).
					LegendFormat("{{nodename}} {{model}} (NVMe)"),
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
