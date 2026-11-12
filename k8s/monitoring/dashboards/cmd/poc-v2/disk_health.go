package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	diskHealthPanelUnhealthyDisks                       = 1
	diskHealthPanelDisksMonitored                       = 2
	diskHealthPanelSSDsWorn10Life                       = 3
	diskHealthPanelDisksWithFailurePrecursors           = 4
	diskHealthPanelSMARTHealth                          = 5
	diskHealthPanelReallocatedSectorsSATA               = 6
	diskHealthPanelCurrentPendingSectorsSATA            = 7
	diskHealthPanelOfflineUncorrectableSATA             = 8
	diskHealthPanelReportedUncorrectableSATA            = 9
	diskHealthPanelUDMACRCErrorsSATA                    = 10
	diskHealthPanelPendingReallocatedSectorTrendSATA    = 11
	diskHealthPanelMediaErrorLogEntriesNVMe             = 12
	diskHealthPanelSSDLifeRemainingSATA                 = 13
	diskHealthPanelEnduranceUsedNVMe                    = 14
	diskHealthPanelAvailableSpareNVMe                   = 15
	diskHealthPanelPowerOnHours                         = 16
	diskHealthPanelCriticalWarning                      = 17
	diskHealthPanelTotalDataWrittenTBW                  = 18
	diskHealthPanelTotalDataRead                        = 19
	diskHealthPanelWriteReadThroughput                  = 20
	diskHealthPanelSmartctlExitStatus                   = 21
	diskHealthPanelFailurePrecursors                    = 22
	diskHealthPanelUDMACRCErrorsTrueNAS                 = 23
	diskHealthPanelPowerOnHoursTrueNAS                  = 24
	diskHealthPanelPendingReallocatedSectorTrendTrueNAS = 25
	diskHealthPanelDiskTemperature                      = 26
)

// buildDiskHealth covers physical disks from smartmon, nvme_exporter, and
// TrueNAS smartctl_exporter. SATA attributes require type="sat" because
// smartmon's type="scsi" entries are virtual disks with false health failures.
// NVMe health comes from nvme_* metrics; TrueNAS is statically scoped because
// it is outside the node-exporter variable inventory.
func buildDiskHealth() (*dashboardv2.Dashboard, error) {
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
	crcThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})
	// Summary conditions warn from one; only unhealthy disks are critical.
	noticeThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
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

	d := dashboardv2.NewDashboardBuilder("Disk Health (S.M.A.R.T.)").
		Tags([]string{"disk", "smart", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-30d").To("now").AutoRefresh("5m")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		// Reuse node-overview's nodename-to-instance variables.
		QueryVariable(dashboardv2.NewQueryVariableBuilder("node").
			Label("Node").
			Query(promVariableQuery(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}, nodename)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true)).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("instance").
			Query(promVariableQuery(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename=~"$node"}, instance)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Multi(true).
			IncludeAll(true).
			Hide(dashboardv2.VariableHideHideVariable))

	return newLayout(d).
		Row("Summary", statGrid(4)).
		// Roll up SATA, NVMe, and TrueNAS health using each source's authoritative
		// signal. Keep TrueNAS outside the node filter and preserve zero values.
		Panel(diskHealthPanelUnhealthyDisks, dashboardv2.NewPanelBuilder().
			Title("Unhealthy Disks").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(issueThresholds())).
			Data(queries(promQuery().
				Expr(`(sum(smartmon_device_smart_healthy{type="sat",`+instFilter+`} == bool 0) or vector(0)) + (sum(nvme_critical_warning{`+instFilter+`} > bool 0) or vector(0)) + (sum(smartctl_device_smart_status{`+truenasFilter+`} == bool 0) or vector(0))`).
				Instant(true).Range(false).
				LegendFormat("Unhealthy")))).
		Panel(diskHealthPanelDisksMonitored, dashboardv2.NewPanelBuilder().
			Title("Disks Monitored").
			Description("Number of disks whose health is evaluated, i.e. exactly the tiles in the SMART Health panel below.").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "blue"},
						}))).
			Data(queries( // Count exactly the sources displayed by SMART Health, excluding virtual
				// SCSI disks and including NVMe.
				promQuery().
					Expr(`(count(smartmon_device_smart_healthy{type="sat",`+instFilter+`}) or vector(0)) + (count(nvme_critical_warning{`+instFilter+`}) or vector(0)) + (count(smartctl_device_smart_status{`+truenasFilter+`}) or vector(0))`).
					Instant(true).Range(false).
					LegendFormat("Disks")))).
		Panel(diskHealthPanelSSDsWorn10Life, dashboardv2.NewPanelBuilder().
			Title("SSDs Worn (<10% life)").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(issueThresholds())).
			Data(queries(promQuery().
				// Preserve zero when selected disks expose no wear attribute.
				Expr(`sum((`+
					`smartmon_wear_leveling_count_value{`+instFilter+`}`+
					` or smartmon_media_wearout_indicator_value{`+instFilter+`}`+
					` or smartmon_ssd_life_left_value{`+instFilter+`}`+
					` or smartmon_percent_lifetime_remain_value{`+instFilter+`}`+
					`) < bool 10) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Worn")))).
		// Count disks with any failure precursor once across all SATA attributes.
		// Standing damage warns; growth is alerted separately.
		Panel(diskHealthPanelDisksWithFailurePrecursors, dashboardv2.NewPanelBuilder().
			Title("Disks with Failure Precursors").
			Description("Disks reporting any reallocated, pending, offline-uncorrectable or "+
				"reported-uncorrectable sectors. A nonzero count is not an outage: it means "+
				"those disks have remapped or failed to read sectors at some point. Growth is "+
				"what matters, and is alerted separately.").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(noticeThresholds)).
			Data(queries(promQuery().
				Expr(`(count(max by (instance, disk) (`+
					`smartmon_reallocated_sector_ct_raw_value{`+instFilter+`}`+
					` or smartmon_current_pending_sector_raw_value{`+instFilter+`}`+
					` or smartmon_offline_uncorrectable_raw_value{`+instFilter+`}`+
					` or smartmon_reported_uncorrect_raw_value{`+instFilter+`}`+
					`) > 0) or vector(0))`+
					` + (count(max by (instance, device) (`+
					`smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",`+
					`attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector|Offline_Uncorrectable|Reported_Uncorrect"}`+
					`) > 0) or vector(0))`).
				Instant(true).Range(false).
				LegendFormat("Precursors")))).
		// SATA uses overall SMART health; NVMe uses critical_warning because the
		// ATA-centric smartmon collector does not provide NVMe health.
		Row("SMART Health", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeTall)).
		Panel(diskHealthPanelSMARTHealth, dashboardv2.NewPanelBuilder().
			Title("SMART Health").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(1)), Color: "green"},
					})).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("FAIL"), Color: new("red")},
							"1": {Text: new("OK"), Color: new("green")},
						},
					}},
					})).
			Data(queries( // Evaluate current health to avoid historical device/model cross-products.
				// Omit models so per-disk tiles remain readable.
				promQuery().
					Expr(`smartmon_device_smart_healthy{type="sat",`+instFilter+`} `+joinNodename).
					Instant(true).Range(false).
					LegendFormat("{{nodename}} {{disk}} (SATA)"),
				promQuery().
					Expr(`(nvme_critical_warning{`+instFilter+`} == bool 0) `+joinNodename).
					Instant(true).Range(false).
					LegendFormat("{{nodename}} {{device}} (NVMe)"),
				promQuery().
					Expr(`smartctl_device_smart_status{`+truenasFilter+`}`).
					Instant(true).Range(false).
					LegendFormat("{{instance}} {{device}} (SATA)")))).
		// Group SATA and NVMe failure precursors by operational question rather
		// than technology. TrueNAS remains separate because it ignores $node.
		Row("Failure Precursors", chartGrid()).
		Panel(diskHealthPanelReallocatedSectorsSATA, dashboardv2.NewPanelBuilder().
			Title("Reallocated Sectors (SATA)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds)).
			Data(queries(promQuery().
				Expr(`sort_desc(smartmon_reallocated_sector_ct_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))).
		Panel(diskHealthPanelCurrentPendingSectorsSATA, dashboardv2.NewPanelBuilder().
			Title("Current Pending Sectors (SATA)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds)).
			Data(queries(promQuery().
				Expr(`sort_desc(smartmon_current_pending_sector_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))).
		Row("Offline Uncorrectable (SATA)", chartGrid()).
		Panel(diskHealthPanelOfflineUncorrectableSATA, dashboardv2.NewPanelBuilder().
			Title("Offline Uncorrectable (SATA)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds)).
			Data(queries(promQuery().
				Expr(`sort_desc(smartmon_offline_uncorrectable_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))).
		Panel(diskHealthPanelReportedUncorrectableSATA, dashboardv2.NewPanelBuilder().
			Title("Reported Uncorrectable (SATA)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(precursorThresholds)).
			Data(queries(promQuery().
				Expr(`sort_desc(smartmon_reported_uncorrect_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))).
		Row("UDMA CRC Errors (SATA)", chartGrid()).
		Panel(diskHealthPanelUDMACRCErrorsSATA, dashboardv2.NewPanelBuilder().
			Title("UDMA CRC Errors (SATA)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(crcThresholds)).
			Data(queries(promQuery().
				Expr(`sort_desc(smartmon_udma_crc_error_count_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))).
		Row("Pending / Reallocated Sector Trend (SATA)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// Trend view so a precursor stepping off zero is visible historically.
		Panel(diskHealthPanelPendingReallocatedSectorTrendSATA, dashboardv2.NewPanelBuilder().
			Title("Pending / Reallocated Sector Trend (SATA)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`smartmon_reallocated_sector_ct_raw_value{`+instFilter+`} `+joinNodename).
				LegendFormat("{{nodename}} {{disk}} reallocated"),
				promQuery().
					Expr(`smartmon_current_pending_sector_raw_value{`+instFilter+`} `+joinNodename).
					LegendFormat("{{nodename}} {{disk}} pending")))).
		Row("Media & Error-Log Entries (NVMe)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// NVMe exposes media errors and error-log entries rather than SATA-style
		// sector attributes. Key history by serial across device-name swaps.
		Panel(diskHealthPanelMediaErrorLogEntriesNVMe, dashboardv2.NewPanelBuilder().
			Title("Media & Error-Log Entries (NVMe)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byNvmeDisk(`nvme_media_errors_total{`+instFilter+`}`)).
				LegendFormat("{{nodename}} {{model}} media errors"),
				promQuery().
					Expr(byNvmeDisk(`nvme_num_err_log_entries_total{`+instFilter+`}`)).
					LegendFormat("{{nodename}} {{model}} err-log entries"),
				promQuery().
					Expr(byNvmeDisk(`nvme_unsafe_shutdowns_total{`+instFilter+`}`)).
					LegendFormat("{{nodename}} {{model}} unsafe shutdowns")))).
		// Place SATA remaining-life and NVMe used-life together, but keep separate
		// scales. Vendor SATA attributes normalize to 100=new.
		Row("Wear & Lifetime", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// Stat tiles force value and name text so the usually sparse, long disk
		// labels remain readable.
		Panel(diskHealthPanelSSDLifeRemainingSATA, dashboardv2.NewPanelBuilder().
			Title("SSD Life Remaining (SATA)").
			Description("Normalized SSD wear (100 = new). Sourced from whichever vendor "+
				"attribute a disk exposes (wear_leveling_count, media_wearout_indicator, "+
				"ssd_life_left, percent_lifetime_remain). SSDs that expose none, plus all "+
				"HDDs, do not appear; NVMe endurance is the panel to the right.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(10)), Color: "yellow"},
						{Value: new(float64(20)), Color: "green"},
					})).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`(`+
					`smartmon_wear_leveling_count_value{`+instFilter+`}`+
					` or smartmon_media_wearout_indicator_value{`+instFilter+`}`+
					` or smartmon_ssd_life_left_value{`+instFilter+`}`+
					` or smartmon_percent_lifetime_remain_value{`+instFilter+`}`+
					`) `+joinNodename+` `+joinModel).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))).
		Row("Endurance Used (NVMe)", chartGrid()).
		Panel(diskHealthPanelEnduranceUsedNVMe, dashboardv2.NewPanelBuilder().
			Title("Endurance Used (NVMe)").
			Description("The drive's own estimate of write endurance consumed. Counts up, "+
				"unlike the SATA panel to the left, which counts down.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(80)), Color: "yellow"},
						{Value: new(float64(100)), Color: "red"},
					})).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`(nvme_percentage_used_ratio{`+instFilter+`} * 100) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))).
		Panel(diskHealthPanelAvailableSpareNVMe, dashboardv2.NewPanelBuilder().
			Title("Available Spare (NVMe)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(10)), Color: "yellow"},
						{Value: new(float64(20)), Color: "green"},
					})).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`(nvme_available_spare_ratio{`+instFilter+`} * 100) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))).
		Row("Power On Hours", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// Combine SATA and NVMe power-on hours; TrueNAS remains separately scoped.
		// Auto grid orientation keeps the larger disk set readable.
		Panel(diskHealthPanelPowerOnHours, dashboardv2.NewPanelBuilder().
			Title("Power On Hours").
			Description("How long each disk has been powered on. A figure to plan "+
				"replacements against, not a health signal, so it carries no threshold "+
				"colour: nothing here is good or bad on its own.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(measurementThresholds()).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`smartmon_power_on_hours_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}"),
				promQuery().
					Expr(`nvme_power_on_hours_total{`+instFilter+`} `+joinNodename+` `+joinNvmeModel).
					Instant(true).Range(false).
					LegendFormat("{{nodename}} {{device}} {{model}}")))).
		// Remaining metrics are NVMe-specific. Data units use the specification's
		// approximate 512000-byte unit; critical_warning is a fault bitfield.
		Row("NVMe", chartGrid()).
		// critical_warning is a bitfield; any nonzero bit indicates a fault.
		Panel(diskHealthPanelCriticalWarning, dashboardv2.NewPanelBuilder().
			Title("Critical Warning").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Text(diskHealthLabelText()).
				Thresholds(issueThresholds()).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("OK"), Color: new("green")},
						},
					}},
				})).
			Data(queries(promQuery().
				Expr(`nvme_critical_warning{`+instFilter+`} `+joinNodename).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}}")))).
		Panel(diskHealthPanelTotalDataWrittenTBW, dashboardv2.NewPanelBuilder().
			Title("Total Data Written (TBW)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("bytes").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				Text(diskHealthLabelText()).
				Thresholds(measurementThresholds()).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`(nvme_data_units_written_total{`+instFilter+`} * 512000) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))).
		Panel(diskHealthPanelTotalDataRead, dashboardv2.NewPanelBuilder().
			Title("Total Data Read").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("bytes").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationAuto).
				Text(diskHealthLabelText()).
				Thresholds(measurementThresholds()).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`(nvme_data_units_read_total{`+instFilter+`} * 512000) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))).
		Row("Write / Read Throughput", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// rate() runs before serial relabeling, so a device-name swap can create one
		// reboot-time spike. Clamping would also hide legitimate bursts.
		Panel(diskHealthPanelWriteReadThroughput, dashboardv2.NewPanelBuilder().
			Title("Write / Read Throughput").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				OverrideByQuery("Read", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(queryWithRef("Write", promQuery().
				RefId("Write").
				Expr(byNvmeDisk(`(rate(nvme_data_units_written_total{`+instFilter+`}[$__rate_interval]) * 512000)`)).
				LegendFormat("{{nodename}} {{model}} Write")),
				queryWithRef("Read", promQuery().
					RefId("Read").
					Expr(byNvmeDisk(`(rate(nvme_data_units_read_total{`+instFilter+`}[$__rate_interval]) * 512000)`)).
					LegendFormat("{{nodename}} {{model}} Read"))))).
		// TrueNAS smartctl_exporter covers passed-through SATA disks outside $node.
		// Overall health is shared above; this row shows precursors and read status.
		Row("TrueNAS Disks", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// Nonzero status means failure or standby skip; skipped disks stay stale until wake-up.
		Panel(diskHealthPanelSmartctlExitStatus, dashboardv2.NewPanelBuilder().
			Title("smartctl Exit Status").
			Description("0 = disk read OK. Nonzero means smartctl failed or the disk "+
				"was skipped in standby; the series in this row then stop updating.").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				TextMode(common.BigValueTextModeValueAndName).
				Thresholds(issueThresholds()).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("OK"), Color: new("green")},
						},
					}},
				})).
			Data(queries(promQuery().
				Expr(`smartctl_device_smartctl_exit_status{`+truenasFilter+`}`).
				Instant(true).Range(false).
				LegendFormat("{{instance}} {{device}}")))).
		Row("Failure Precursors", chartGrid()).
		Panel(diskHealthPanelFailurePrecursors, dashboardv2.NewPanelBuilder().
			Title("Failure Precursors").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(issueThresholds())).
			Data(queries(promQuery().
				Expr(`sort_desc(smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector|Offline_Uncorrectable|Reported_Uncorrect"})`).
				Instant(true).Range(false).
				LegendFormat("{{device}} {{attribute_name}}")))).
		// CRC errors warn (yellow) not alert (red), same as the smartmon panel.
		Panel(diskHealthPanelUDMACRCErrorsTrueNAS, dashboardv2.NewPanelBuilder().
			Title("UDMA CRC Errors (TrueNAS)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Orientation(common.VizOrientationHorizontal).
				Text(diskHealthLabelText()).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(1)), Color: "yellow"},
					}))).
			Data(queries(promQuery().
				Expr(`sort_desc(smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",attribute_name="UDMA_CRC_Error_Count"})`).
				Instant(true).Range(false).
				LegendFormat("{{device}} {{attribute_name}}")))).
		Row("Power On Hours (TrueNAS)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(diskHealthPanelPowerOnHoursTrueNAS, dashboardv2.NewPanelBuilder().
			Title("Power On Hours (TrueNAS)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("h").
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeValue).
				Orientation(common.VizOrientationHorizontal).
				TextMode(common.BigValueTextModeValueAndName).
				Text(diskHealthLabelText()).
				Thresholds(measurementThresholds()).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`(smartctl_device_power_on_seconds{`+truenasFilter+`} / 3600) `+joinSmartctlModel).
				Instant(true).Range(false).
				LegendFormat("{{device}} {{model_name}}")))).
		Row("Pending / Reallocated Sector Trend (TrueNAS)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// Trend view so a precursor stepping off zero is visible historically.
		Panel(diskHealthPanelPendingReallocatedSectorTrendTrueNAS, dashboardv2.NewPanelBuilder().
			Title("Pending / Reallocated Sector Trend (TrueNAS)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector"}`).
				LegendFormat("{{device}} {{attribute_name}}")))).
		Row("Temperature", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(diskHealthPanelDiskTemperature, dashboardv2.NewPanelBuilder().
			Title("Disk Temperature").
			Visualization(timeseries.NewVisualizationV2Builder().
					Unit("celsius").
					Tooltip(tooltipAll).
					Legend(legend)).
			Data(queries( // SATA device names are stable here; avoid historical model joins.
				promQuery().
					Expr(`smartmon_temperature_celsius_raw_value{`+instFilter+`} `+joinNodename).
					LegendFormat("{{nodename}} {{disk}} (SATA)"),
				// NVMe history uses serial identity across device-name swaps.
				promQuery().
					Expr(byNvmeDisk(`nvme_temperature_celsius{`+instFilter+`}`)).
					LegendFormat("{{nodename}} {{model}} (NVMe)"),
				promQuery().
					Expr(`smartctl_device_temperature{`+truenasFilter+`,temperature_type="current"}`).
					LegendFormat("{{instance}} {{device}} (SATA)")))).
		Build()
}

func diskHealthLabelText() *common.VizTextDisplayOptionsBuilder {
	return common.NewVizTextDisplayOptionsBuilder().TitleSize(16)
}
