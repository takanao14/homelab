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
		elementUnhealthyDisks                       = "unhealthy-disks"
		elementDisksMonitored                       = "disks-monitored"
		elementSSDsWorn10Life                       = "ssds-worn-10-life"
		elementDisksWithFailurePrecursors           = "disks-with-failure-precursors"
		elementSMARTHealth                          = "smart-health"
		elementReallocatedSectorsSATA               = "reallocated-sectors-sata"
		elementCurrentPendingSectorsSATA            = "current-pending-sectors-sata"
		elementOfflineUncorrectableSATA             = "offline-uncorrectable-sata"
		elementReportedUncorrectableSATA            = "reported-uncorrectable-sata"
		elementUDMACRCErrorsSATA                    = "udma-crc-errors-sata"
		elementPendingReallocatedSectorTrendSATA    = "pending-reallocated-sector-trend-sata"
		elementMediaErrorLogEntriesNVMe             = "media-error-log-entries-nvme"
		elementSSDLifeRemainingSATA                 = "ssd-life-remaining-sata"
		elementEnduranceUsedNVMe                    = "endurance-used-nvme"
		elementAvailableSpareNVMe                   = "available-spare-nvme"
		elementPowerOnHours                         = "power-on-hours"
		elementCriticalWarning                      = "critical-warning"
		elementTotalDataWrittenTBW                  = "total-data-written-tbw"
		elementTotalDataRead                        = "total-data-read"
		elementWriteReadThroughput                  = "write-read-throughput"
		elementSmartctlExitStatus                   = "smartctl-exit-status"
		elementFailurePrecursors                    = "failure-precursors"
		elementUDMACRCErrorsTrueNAS                 = "udma-crc-errors-truenas"
		elementPowerOnHoursTrueNAS                  = "power-on-hours-truenas"
		elementPendingReallocatedSectorTrendTrueNAS = "pending-reallocated-sector-trend-truenas"
		elementDiskTemperature                      = "disk-temperature"
	)

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

		// Roll up SATA, NVMe, and TrueNAS health using each source's authoritative
		// signal. Keep TrueNAS outside the node filter and preserve zero values.
	d.Panel(elementUnhealthyDisks, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelUnhealthyDisks).
		Title("Unhealthy Disks").
		Visualization(stat.NewVisualizationV2Builder().
			GraphMode(common.BigValueGraphModeNone).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Thresholds(issueThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(sum(smartmon_device_smart_healthy{type="sat",`+instFilter+`} == bool 0) or vector(0)) + (sum(nvme_critical_warning{`+instFilter+`} > bool 0) or vector(0)) + (sum(smartctl_device_smart_status{`+truenasFilter+`} == bool 0) or vector(0))`).
				Instant(true).Range(false).
				LegendFormat("Unhealthy")))))

	d.Panel(elementDisksMonitored, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelDisksMonitored).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			// Count exactly the sources displayed by SMART Health, excluding virtual
			// SCSI disks and including NVMe.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(count(smartmon_device_smart_healthy{type="sat",`+instFilter+`}) or vector(0)) + (count(nvme_critical_warning{`+instFilter+`}) or vector(0)) + (count(smartctl_device_smart_status{`+truenasFilter+`}) or vector(0))`).
				Instant(true).Range(false).
				LegendFormat("Disks")))))

	d.Panel(elementSSDsWorn10Life, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelSSDsWorn10Life).
		Title("SSDs Worn (<10% life)").
		Visualization(stat.NewVisualizationV2Builder().
			GraphMode(common.BigValueGraphModeNone).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Thresholds(issueThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				// Preserve zero when selected disks expose no wear attribute.
				Expr(`sum((`+
					`smartmon_wear_leveling_count_value{`+instFilter+`}`+
					` or smartmon_media_wearout_indicator_value{`+instFilter+`}`+
					` or smartmon_ssd_life_left_value{`+instFilter+`}`+
					` or smartmon_percent_lifetime_remain_value{`+instFilter+`}`+
					`) < bool 10) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Worn")))))

	// Count disks with any failure precursor once across all SATA attributes.
	// Standing damage warns; growth is alerted separately.
	d.Panel(elementDisksWithFailurePrecursors, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelDisksWithFailurePrecursors).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
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
				LegendFormat("Precursors")))))

	d.Panel(elementSMARTHealth, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelSMARTHealth).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			// Evaluate current health to avoid historical device/model cross-products.
			// Omit models so per-disk tiles remain readable.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`smartmon_device_smart_healthy{type="sat",`+instFilter+`} `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} (SATA)"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`(nvme_critical_warning{`+instFilter+`} == bool 0) `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} (NVMe)"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`smartctl_device_smart_status{`+truenasFilter+`}`).
				Instant(true).Range(false).
				LegendFormat("{{instance}} {{device}} (SATA)")))))

	d.Panel(elementReallocatedSectorsSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelReallocatedSectorsSATA).
		Title("Reallocated Sectors (SATA)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Orientation(common.VizOrientationHorizontal).
			Text(diskHealthLabelText()).
			Thresholds(precursorThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartmon_reallocated_sector_ct_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))))

	d.Panel(elementCurrentPendingSectorsSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelCurrentPendingSectorsSATA).
		Title("Current Pending Sectors (SATA)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Orientation(common.VizOrientationHorizontal).
			Text(diskHealthLabelText()).
			Thresholds(precursorThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartmon_current_pending_sector_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))))

	d.Panel(elementOfflineUncorrectableSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelOfflineUncorrectableSATA).
		Title("Offline Uncorrectable (SATA)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Orientation(common.VizOrientationHorizontal).
			Text(diskHealthLabelText()).
			Thresholds(precursorThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartmon_offline_uncorrectable_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))))

	d.Panel(elementReportedUncorrectableSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelReportedUncorrectableSATA).
		Title("Reported Uncorrectable (SATA)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Orientation(common.VizOrientationHorizontal).
			Text(diskHealthLabelText()).
			Thresholds(precursorThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartmon_reported_uncorrect_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))))

	d.Panel(elementUDMACRCErrorsSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelUDMACRCErrorsSATA).
		Title("UDMA CRC Errors (SATA)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Orientation(common.VizOrientationHorizontal).
			Text(diskHealthLabelText()).
			Thresholds(crcThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartmon_udma_crc_error_count_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel+`)`).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))))

	// Trend view so a precursor stepping off zero is visible historically.
	d.Panel(elementPendingReallocatedSectorTrendSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelPendingReallocatedSectorTrendSATA).
		Title("Pending / Reallocated Sector Trend (SATA)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`smartmon_reallocated_sector_ct_raw_value{`+instFilter+`} `+joinNodename).
				LegendFormat("{{nodename}} {{disk}} reallocated"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`smartmon_current_pending_sector_raw_value{`+instFilter+`} `+joinNodename).
				LegendFormat("{{nodename}} {{disk}} pending")))))

	// NVMe exposes media errors and error-log entries rather than SATA-style
	// sector attributes. Key history by serial across device-name swaps.
	d.Panel(elementMediaErrorLogEntriesNVMe, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelMediaErrorLogEntriesNVMe).
		Title("Media & Error-Log Entries (NVMe)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(byNvmeDisk(`nvme_media_errors_total{`+instFilter+`}`)).
				LegendFormat("{{nodename}} {{model}} media errors"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(byNvmeDisk(`nvme_num_err_log_entries_total{`+instFilter+`}`)).
				LegendFormat("{{nodename}} {{model}} err-log entries"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(byNvmeDisk(`nvme_unsafe_shutdowns_total{`+instFilter+`}`)).
				LegendFormat("{{nodename}} {{model}} unsafe shutdowns")))))

	// Stat tiles force value and name text so the usually sparse, long disk
	// labels remain readable.
	d.Panel(elementSSDLifeRemainingSATA, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelSSDLifeRemainingSATA).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(`+
					`smartmon_wear_leveling_count_value{`+instFilter+`}`+
					` or smartmon_media_wearout_indicator_value{`+instFilter+`}`+
					` or smartmon_ssd_life_left_value{`+instFilter+`}`+
					` or smartmon_percent_lifetime_remain_value{`+instFilter+`}`+
					`) `+joinNodename+` `+joinModel).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}")))))

	d.Panel(elementEnduranceUsedNVMe, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelEnduranceUsedNVMe).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(nvme_percentage_used_ratio{`+instFilter+`} * 100) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))))

	d.Panel(elementAvailableSpareNVMe, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelAvailableSpareNVMe).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(nvme_available_spare_ratio{`+instFilter+`} * 100) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))))

	// Combine SATA and NVMe power-on hours; TrueNAS remains separately scoped.
	// Auto grid orientation keeps the larger disk set readable.
	d.Panel(elementPowerOnHours, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelPowerOnHours).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`smartmon_power_on_hours_raw_value{`+instFilter+`} `+joinNodename+` `+joinModel).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{disk}} {{device_model}}"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`nvme_power_on_hours_total{`+instFilter+`} `+joinNodename+` `+joinNvmeModel).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))))

	// critical_warning is a bitfield; any nonzero bit indicates a fault.
	d.Panel(elementCriticalWarning, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelCriticalWarning).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`nvme_critical_warning{`+instFilter+`} `+joinNodename).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}}")))))

	d.Panel(elementTotalDataWrittenTBW, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelTotalDataWrittenTBW).
		Title("Total Data Written (TBW)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("bytes").
			GraphMode(common.BigValueGraphModeNone).
			ColorMode(common.BigValueColorModeValue).
			Orientation(common.VizOrientationAuto).
			Text(diskHealthLabelText()).
			Thresholds(measurementThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(nvme_data_units_written_total{`+instFilter+`} * 512000) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))))

	d.Panel(elementTotalDataRead, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelTotalDataRead).
		Title("Total Data Read").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("bytes").
			GraphMode(common.BigValueGraphModeNone).
			ColorMode(common.BigValueColorModeValue).
			Orientation(common.VizOrientationAuto).
			Text(diskHealthLabelText()).
			Thresholds(measurementThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(nvme_data_units_read_total{`+instFilter+`} * 512000) `+joinNodename+` `+joinNvmeModel).Instant(true).Range(false).
				LegendFormat("{{nodename}} {{device}} {{model}}")))))

	// rate() runs before serial relabeling, so a device-name swap can create one
	// reboot-time spike. Clamping would also hide legitimate bursts.
	d.Panel(elementWriteReadThroughput, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelWriteReadThroughput).
		Title("Write / Read Throughput").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("Bps").
			Tooltip(tooltipAll).
			Legend(legend).
			OverrideByQuery("Read", []dashboardv2.DynamicConfigValue{
				{Id: "custom.transform", Value: "negative-Y"},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("Write").Query(promQuery().
				RefId("Write").
				Expr(byNvmeDisk(`(rate(nvme_data_units_written_total{`+instFilter+`}[$__rate_interval]) * 512000)`)).
				LegendFormat("{{nodename}} {{model}} Write"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Read").Query(promQuery().
				RefId("Read").
				Expr(byNvmeDisk(`(rate(nvme_data_units_read_total{`+instFilter+`}[$__rate_interval]) * 512000)`)).
				LegendFormat("{{nodename}} {{model}} Read")))))

	// Nonzero status means failure or standby skip; skipped disks stay stale until wake-up.
	d.Panel(elementSmartctlExitStatus, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelSmartctlExitStatus).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`smartctl_device_smartctl_exit_status{`+truenasFilter+`}`).
				Instant(true).Range(false).
				LegendFormat("{{instance}} {{device}}")))))

	d.Panel(elementFailurePrecursors, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelFailurePrecursors).
		Title("Failure Precursors").
		Visualization(bargauge.NewVisualizationV2Builder().
			Orientation(common.VizOrientationHorizontal).
			Text(diskHealthLabelText()).
			Thresholds(issueThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector|Offline_Uncorrectable|Reported_Uncorrect"})`).
				Instant(true).Range(false).
				LegendFormat("{{device}} {{attribute_name}}")))))

	// CRC errors warn (yellow) not alert (red), same as the smartmon panel.
	d.Panel(elementUDMACRCErrorsTrueNAS, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelUDMACRCErrorsTrueNAS).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",attribute_name="UDMA_CRC_Error_Count"})`).
				Instant(true).Range(false).
				LegendFormat("{{device}} {{attribute_name}}")))))

	d.Panel(elementPowerOnHoursTrueNAS, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelPowerOnHoursTrueNAS).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`(smartctl_device_power_on_seconds{`+truenasFilter+`} / 3600) `+joinSmartctlModel).
				Instant(true).Range(false).
				LegendFormat("{{device}} {{model_name}}")))))

	// Trend view so a precursor stepping off zero is visible historically.
	d.Panel(elementPendingReallocatedSectorTrendTrueNAS, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelPendingReallocatedSectorTrendTrueNAS).
		Title("Pending / Reallocated Sector Trend (TrueNAS)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`smartctl_device_attribute{`+truenasFilter+`,attribute_value_type="raw",attribute_name=~"Reallocated_Sector_Ct|Current_Pending_Sector"}`).
				LegendFormat("{{device}} {{attribute_name}}")))))

	d.Panel(elementDiskTemperature, dashboardv2.NewPanelBuilder().
		Id(diskHealthPanelDiskTemperature).
		Title("Disk Temperature").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("celsius").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// SATA device names are stable here; avoid historical model joins.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`smartmon_temperature_celsius_raw_value{`+instFilter+`} `+joinNodename).
				LegendFormat("{{nodename}} {{disk}} (SATA)"))).
			// NVMe history uses serial identity across device-name swaps.
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(byNvmeDisk(`nvme_temperature_celsius{`+instFilter+`}`)).
				LegendFormat("{{nodename}} {{model}} (NVMe)"))).
			Target(dashboardv2.NewTargetBuilder().RefId("C").Query(promQuery().
				Expr(`smartctl_device_temperature{`+truenasFilter+`,temperature_type="current"}`).
				LegendFormat("{{instance}} {{device}} (SATA)")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementUnhealthyDisks).
			WithItem(elementDisksMonitored).
			WithItem(elementSSDsWorn10Life).
			WithItem(elementDisksWithFailurePrecursors))).
		// SATA uses overall SMART health; NVMe uses critical_warning because the
		// ATA-centric smartmon collector does not provide NVMe health.
		Row(dashboardv2.NewRowBuilder().Title("SMART Health").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeTall).
			WithItem(elementSMARTHealth))).
		// Group SATA and NVMe failure precursors by operational question rather
		// than technology. TrueNAS remains separate because it ignores $node.
		Row(dashboardv2.NewRowBuilder().Title("Failure Precursors").AutoGridLayout(chartGrid().
			WithItem(elementReallocatedSectorsSATA).
			WithItem(elementCurrentPendingSectorsSATA))).
		Row(dashboardv2.NewRowBuilder().Title("Offline Uncorrectable (SATA)").AutoGridLayout(chartGrid().
			WithItem(elementOfflineUncorrectableSATA).
			WithItem(elementReportedUncorrectableSATA))).
		Row(dashboardv2.NewRowBuilder().Title("UDMA CRC Errors (SATA)").AutoGridLayout(chartGrid().
			WithItem(elementUDMACRCErrorsSATA))).
		Row(dashboardv2.NewRowBuilder().Title("Pending / Reallocated Sector Trend (SATA)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementPendingReallocatedSectorTrendSATA))).
		Row(dashboardv2.NewRowBuilder().Title("Media & Error-Log Entries (NVMe)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementMediaErrorLogEntriesNVMe))).
		// Place SATA remaining-life and NVMe used-life together, but keep separate
		// scales. Vendor SATA attributes normalize to 100=new.
		Row(dashboardv2.NewRowBuilder().Title("Wear & Lifetime").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementSSDLifeRemainingSATA))).
		Row(dashboardv2.NewRowBuilder().Title("Endurance Used (NVMe)").AutoGridLayout(chartGrid().
			WithItem(elementEnduranceUsedNVMe).
			WithItem(elementAvailableSpareNVMe))).
		Row(dashboardv2.NewRowBuilder().Title("Power On Hours").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementPowerOnHours))).
		// Remaining metrics are NVMe-specific. Data units use the specification's
		// approximate 512000-byte unit; critical_warning is a fault bitfield.
		Row(dashboardv2.NewRowBuilder().Title("NVMe").AutoGridLayout(chartGrid().
			WithItem(elementCriticalWarning).
			WithItem(elementTotalDataWrittenTBW).
			WithItem(elementTotalDataRead))).
		Row(dashboardv2.NewRowBuilder().Title("Write / Read Throughput").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementWriteReadThroughput))).
		// TrueNAS smartctl_exporter covers passed-through SATA disks outside $node.
		// Overall health is shared above; this row shows precursors and read status.
		Row(dashboardv2.NewRowBuilder().Title("TrueNAS Disks").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementSmartctlExitStatus))).
		Row(dashboardv2.NewRowBuilder().Title("Failure Precursors").AutoGridLayout(chartGrid().
			WithItem(elementFailurePrecursors).
			WithItem(elementUDMACRCErrorsTrueNAS))).
		Row(dashboardv2.NewRowBuilder().Title("Power On Hours (TrueNAS)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementPowerOnHoursTrueNAS))).
		Row(dashboardv2.NewRowBuilder().Title("Pending / Reallocated Sector Trend (TrueNAS)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementPendingReallocatedSectorTrendTrueNAS))).
		Row(dashboardv2.NewRowBuilder().Title("Temperature").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDiskTemperature)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}

func diskHealthLabelText() *common.VizTextDisplayOptionsBuilder {
	return common.NewVizTextDisplayOptionsBuilder().TitleSize(16)
}
