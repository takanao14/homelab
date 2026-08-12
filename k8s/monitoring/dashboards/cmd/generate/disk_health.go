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
// (reallocated sectors, pending sectors, wear leveling, etc.). NVMe disks do not
// appear in the smartmon_* series at all -- the only types emitted are sat and
// scsi -- so every NVMe panel here is sourced from the separate nvme_* exporter.
//
// The smartmon type="scsi" series are VM virtual disks. SMART is not really
// available through them, so the collector reports smart_healthy=0 for all of
// them; treating that as a failure would mean four permanently red disks. Every
// health panel therefore filters type="sat", and the disk counter must apply the
// same filter or it counts disks it never evaluates.
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
			{Value: new(float64(1)), Color: "yellow"},
		})
	// Yellow from one, for a Summary counter reporting a condition to plan around
	// rather than a fault. Red is reserved for Unhealthy Disks.
	noticeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	// byNvmeDisk re-keys an NVMe series from its kernel device name to the drive's
	// serial, for the range panels only.
	//
	// nvmeXnY is assigned in controller probe order and is not stable across
	// reboots. On node1, at the 2026-08-09 17:52 JST boot, the two drives swapped:
	// power_on_hours on nvme0n1 went 4159 -> 2313 while nvme1n1 went 2312 -> 4162.
	// Powered-on hours cannot decrease, so that is one name pointing at a different
	// disk, and the error-log counters crossed in the same scrape (0 -> 2349 and
	// 2346 -> 0). Plotted by device, one drive's history is drawn as two half-lines
	// with a cliff between them, which reads as a fault on a panel whose whole job
	// is to show faults.
	//
	// Dropping `device` from the output is the part that matters. Joining
	// nvme_device_info while keeping device does not help -- over a long range both
	// {device=nvme0n1, model=A} and {device=nvme0n1, model=B} exist and two disks
	// draw four half-empty lines, which is the cross product the instant panels
	// below were written to avoid. Grouping by serial collapses that back to one
	// line per physical drive; verified across the node1 swap, where the Samsung
	// reads a continuous 2346 -> 2349.
	//
	// model rides along so the legend stays readable -- a serial is not something
	// anyone recognises -- but serial, not model, is what holds identity, so two
	// identical models in one host would still be told apart.
	//
	// Not applied to the instant panels. Those evaluate now, when the device name
	// is correct and is the name to type into smartctl, so they keep {{device}}.
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
		// Reuse the bare-metal node variable convention from node-overview:
		// expose $node (nodename) and resolve it to the hidden $instance (IP:port).
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
				// One term per source, matching SMART Health and Unhealthy Disks
				// exactly, so the three tiles cannot disagree. Counting bare
				// smartmon_device_smart_healthy instead got this wrong twice
				// over: NVMe never appears in that metric at all (its types are
				// only sat and scsi), so six disks went missing, while the
				// type="scsi" virtual disks it did count are filtered out of
				// every health panel and so were never evaluated. The tile read
				// 12 against 15 health tiles.
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
					// or vector(0) for the same reason as Unhealthy Disks: only
					// two of the seven SATA disks publish a wear attribute at
					// all, so without it this tile reads "No data" on any node
					// whose disks are HDDs or expose no vendor wear attribute.
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
		// This slot used to hold "NVMe Warnings", which summed
		// nvme_critical_warning > 0 -- the exact second term of Unhealthy Disks
		// beside it. One of four Summary tiles restated part of another, while the
		// only nonzero health signal this fleet actually has sat two rows down in
		// the Failure Precursors bar gauges and could not be seen without
		// scrolling. NVMe is not losing coverage: a warning bit still shows in
		// Unhealthy Disks, in SMART Health per disk, and in the NVMe row's own
		// Critical Warning panel.
		//
		// max by (instance, disk) collapses the four attributes to one value per
		// disk, so a disk carrying damage in two of them still counts once. The
		// `or` chain is a vector union, not a max -- each attribute keeps its own
		// smart_id label, so all four survive into the max.
		//
		// Yellow, not red, and it reads 2 today: pve /dev/sdc and /dev/sdd are
		// Seagate ST2000DM001s carrying 24 reallocated sectors and 1, and 5,
		// reported-uncorrectables respectively. All of it has been flat for the
		// whole retention window, so this is a standing fact to plan replacements
		// around rather than an incident. DiskFailurePrecursorGrowing in
		// charts/prometheus alerts on the counts moving, which is the event.
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
				// Instant queries: evaluate current health only. A range query over a
				// long window (e.g. 30d) would surface stale label combinations — NVMe
				// device names (nvme0n1/nvme1n1) can swap across reboots, so joining
				// historical device_info produces a device×model cross product.
				//
				// Legends omit the device model (unlike other panels below): with one
				// tile per disk, the model string routinely overflows the tile width.
				// The model is still available per-disk in the Wear & Lifetime and
				// TrueNAS rows.
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
		// "Is any medium going bad?" for every disk the $node variable can select.
		// These should sit flat at 0; any step up is the signal that the disk is
		// starting to fail.
		//
		// The row was named "Failure Precursors (SATA)" and the NVMe answer to the
		// same question -- Media & Error-Log Entries -- sat at the bottom of the
		// NVMe row, roughly 1,700 pixels further down. Splitting one question
		// across the page by disk technology is the same mistake node-overview
		// made with Filesystem Usage. The row now asks the question and each panel
		// says which technology it can speak for, because the two sources report
		// genuinely different things: SATA counts remapped and unreadable sectors,
		// NVMe has no such attribute and reports media errors instead.
		//
		// TrueNAS answers this question too, with the identical attribute names,
		// and is deliberately not merged in: its instance is not a value of $node
		// (it runs smartctl_exporter as an app, not node-exporter), so its disks
		// would keep appearing here after filtering down to one node. It keeps its
		// own row for that reason -- a separate scope, not a separate technology.
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
		// The NVMe answer to this row's question. There is no NVMe equivalent of a
		// reallocated or pending sector: the controller remaps internally and
		// reports only the outcome, so media_errors (uncorrected data errors) and
		// the error-log entry count are what stand in for the bar gauges above.
		//
		// Keyed by serial (see byNvmeDisk): this is the panel the device-name swap
		// damaged most, because a renamed drive draws a cliff and a cliff here
		// reads as a fault. media_errors is 0 on all six drives; the error-log
		// count is not a fault on its own, since smartctl's own probing lands in
		// that log.
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
		// "How much life is left?" for every disk the $node variable can select.
		// SATA and NVMe measure it on opposite scales -- SATA counts down from 100
		// remaining, NVMe counts up to 100 used -- so they stay in separate panels
		// rather than being forced into one number that means two things. They are
		// on the same line because the question is the same; the NVMe pair used to
		// open the NVMe row instead, which meant answering "how worn is this fleet?"
		// took two rows.
		//
		// Vendor-normalized SATA wear: each vendor exposes a different attribute,
		// all normalized so 100 = new, so they can be unioned with `or`. Only SSDs
		// publishing one appear; HDDs have no wear concept. Referencing an
		// attribute no disk reports is harmless (it just yields no series).
		WithRow(dashboard.NewRowBuilder("Wear & Lifetime")).
		// Rendered as a stat (not bargauge): this metric usually has a single
		// reporting disk. TextMode value_and_name forces the device name to show
		// even for one series, which "auto" mode would otherwise hide. Each disk
		// appears as its own colored tile (red <10, yellow <20, green).
		//
		// Auto orientation, with both text sizes pinned. Left to size text itself,
		// Grafana gives the value the whole tile and shrinks the name to fit
		// whatever is left -- measured at roughly 70px against 10px, with the name
		// wrapping to two lines, because these names are long ("node3 /dev/sdb
		// Moment SSD 2.5" SSD 240GB"). Naming an explicit value size is what buys
		// the name its room back; the same 16/32 pairing is what SMART Health uses,
		// for the same reason.
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
		// Power-on hours unifies where wear does not: it is the same quantity from
		// both exporters, so the two targets share one panel rather than sitting a
		// row apart under the same title. TrueNAS keeps its own, in its own row,
		// for the scoping reason given above.
		//
		// Twelve tiles, not the six this panel held as SATA-only, so the layout had
		// to change with it. Horizontal stacks one full-width row per disk, which
		// meant twelve rows in the height that used to hold six -- about 20px each,
		// small enough to be unreadable. Doubling the width bought nothing, because
		// what horizontal consumes is height. Auto lays the tiles out as a grid
		// across the full span instead, and the explicit sizes stop Grafana
		// shrinking the names to fit the values.
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
		// What is left is NVMe-only because the metrics are NVMe-only, not because
		// the dashboard chose to group by disk technology: the SATA collector
		// exposes no I/O counters worth plotting (host_writes_32mib is published by
		// exactly one of the seven disks), and critical_warning is a bitfield with
		// no SATA analogue. "Data Units Written/Read" follow the NVMe spec unit of
		// 1000 x 512 bytes, so bytes = value * 512000 (a documented approximation,
		// not exact host I/O).
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
		// Keyed by serial like the panels above, but with one caveat the others do
		// not have: rate() is evaluated on the device-keyed series before anything
		// is relabelled, so at a swap it differences one drive's counter against
		// the other's. Measured on the node1 swap, that put a single sample at
		// 1.55 MB/s against a surrounding ~110 kB/s. The line identity is fixed,
		// the one-sample spike is not, and it cannot be -- a range selector cannot
		// be relabelled from the outside. Clamping it would hide real bursts, so it
		// is left visible; a spike at the instant of a reboot is what it looks like.
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
		// TrueNAS disks behind the passed-through SATA controller, read by
		// smartctl_exporter inside the guest. Overall health is covered by the
		// shared SMART Health panel above; this row holds the SATA failure
		// precursors and exporter diagnostics. attribute_value_type="raw"
		// mirrors the smartmon *_raw_value series used for the other nodes.
		//
		// The row is named for its subject, not its exporter -- which host these
		// disks are in is what distinguishes them from the rows above; that they
		// are read by smartctl_exporter is an implementation detail, and the
		// panels that need it say so in their own titles.
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
				// SATA joins nodename only. A model join here would cross-product with
				// stale device_info over a long range, and /dev/sdX has stayed put on
				// this fleet, so there is nothing to buy with the extra join.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_temperature_celsius_raw_value{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} {{disk}} (SATA)"),
				).
				// NVMe does need it, for the naming reason in byNvmeDisk: without it a
				// legend entry reading nvme0n1 means one drive early in the range and
				// another one later.
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
