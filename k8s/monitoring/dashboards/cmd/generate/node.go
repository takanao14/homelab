package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func buildNodeOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// The hypervisor list, reused from the same inventory proxmox_logs.go reads.
	// Only needed by the throttling panel; see the comment there.
	proxmoxHosts, err := loadProxmoxHostRegex()
	if err != nil {
		return nil, err
	}

	// Two-stage variable resolution: node_* metrics carry instance (IP:port) but
	// display names come from node_uname_info which has nodename. We expose $node
	// (nodename) in the UI and hide $instance (IP:port) resolved from it.
	// joinNodename copies nodename onto query results so legends show hostnames.
	const (
		instFilter = `instance=~"$instance"`
		// max by deduplicates node_uname_info if the same instance is scraped by multiple jobs.
		joinNodename = `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info)`
		// normByCPU divides by the number of logical CPUs so load values are expressed
		// as a fraction of total capacity (1.0 = fully loaded, >1.0 = overloaded).
		normByCPU = `/ on(instance) group_left() count by (instance) (node_cpu_seconds_total{mode="idle", ` + instFilter + `})`
		// fsFilter excludes pseudo/boot filesystems that don't need capacity monitoring.
		fsFilter = `fstype=~"ext[234]|xfs|btrfs|zfs|vfat",mountpoint!~"/var/lib/docker/.*|/boot/efi|/boot/firmware"`
		// zfsActive keeps only nodes with a pool actually imported.
		//
		// The Proxmox hosts all load the ZFS module and only pve has a pool, so
		// node1-5 publish an ARC holding a few header structs. It does not scale
		// with the machine -- 2880 bytes on a 31 GiB node, 1440 on a 1 GiB one --
		// against 6.3 GiB of real ARC on pve. A megabyte sits 350x above the empty
		// case and six thousand times below a working one, so the boundary is not
		// close to anything.
		//
		// Gating on size alone would not have been enough. arc_c_max and arc_c_min
		// are GiB-scale even with no pool -- 0.75 to 6.3 GiB, comparable to pve's
		// actual ARC -- so the limit lines are gated on the same condition rather
		// than left to be filtered by their own magnitude.
		//
		// The collector stays enabled on those hosts deliberately. The readings are
		// genuinely theirs, unlike the LXC guests where /proc/spl/kstat/zfs is the
		// hypervisor's and each guest was republishing its host's ARC under its own
		// name (see group_vars/node_exporter_lxc.yaml, where the collector is now
		// switched off for exactly that reason). Any of these hosts could gain a
		// pool, and when one does it crosses this threshold and appears here on its
		// own -- which is why the filter belongs in the query rather than in a list
		// of hostnames anywhere.
		zfsActive = ` and on(instance) (node_zfs_arc_size{` + instFilter + `} > 1024 * 1024)`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// denseTooltip is defaultTooltip for the panels that carry more series than a
	// screen can hold. Disk I/O draws 54 -- 27 devices, read and write -- and
	// Network I/O draws 66, so the shared multi-series tooltip listed every one of
	// them and ran off the bottom of the window, which meant the series under the
	// cursor could be the one you could not see.
	//
	// Sorting descending is the part that does the work: with the busiest series
	// at the top, the rows that fit are the rows worth reading.
	//
	// MaxHeight bounds the box at 400px, against a Grafana default of 600. The
	// remainder is not lost -- hovering and then clicking pins the tooltip in
	// place, and a pinned tooltip scrolls (confirmed on this Grafana, 13.1.3).
	// That interaction is worth knowing about, because a capped tooltip otherwise
	// looks like truncation.
	//
	// HideZeros drops devices idle at that instant. It earns less than it sounds:
	// measured, only 5 of the 54 disk series and 16 of the 66 network ones sit at
	// exactly zero, because nearly everything here carries some traffic. A row
	// reading 0 B/s is still never the row being looked for.
	//
	// A fresh builder per call: panels must not share one, and the tooltipAll
	// above is already shared by several.
	denseTooltip := func() *common.VizTooltipOptionsBuilder {
		return common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti).
			Sort(common.SortOrderDescending).
			MaxHeight(400).
			HideZeros(true)
	}

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// Yellow from one, for the Summary counters that report a condition worth
	// looking at rather than a fault. Only "Nodes Down" gets red: a node that
	// stopped answering is broken, whereas a saturated CPU, a full-ish memory or a
	// reboot are all states this fleet reaches in normal operation.
	noticeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	d, err := dashboard.NewDashboardBuilder("Node Overview").
		Uid("node-overview").
		Tags([]string{"nodes", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		// Bare-metal and LXC/VM guests scraped over the external scrapeConfig job;
		// the k8s cluster nodes have their own dashboard and their own job.
		//
		// A nodename!="gpuvm" term used to hang off each of these, added to hide
		// stale series from a period when that host was misconfigured. Nothing has
		// matched it for the whole retention window, and the host is now scraped as
		// gpuvm1 by the in-cluster job, which this job selector already excludes, so
		// the term could only ever have hidden something unrelated that happened to
		// be named gpuvm.
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
		// Hidden variable: resolves $node (nodename) to $instance (IP:port).
		// With Multi+IncludeAll, multiple selections produce a regex (a|b|c).
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).
		// Counts of how many things are in a state worth knowing about, and nothing
		// else. The row used to be five strips of one value per node -- status,
		// CPU, memory, load, uptime -- which at nineteen nodes came to 95 tiles and
		// about 600 pixels, so the answer to "is anything wrong" arrived only after
		// reading all of them, and the first screen held nothing else.
		// dashboards/README.md asks whether an operator can identify scope and
		// health without scrolling, and describes Summary as mixing health,
		// utilization and issue counts; every other dashboard here counts, and this
		// one enumerated. The strips are not gone, they are one row down, which is
		// where you go once a count is not zero.
		//
		// Two lines by meaning rather than by insertion order: node lifecycle
		// first at 12 each, then resource pressure at 8 each.
		//
		// Thresholds were checked against 30 days of history so that none of these
		// is a tile that can only ever read zero: nodes down peaked at 1, load per
		// CPU above 1.0 at 2, memory above 80% at 1, and reboots within the hour at
		// 10. Memory above 90% was tried and dropped -- it has not happened once,
		// and the 80% tile already covers the same resource.
		//
		// Filesystems are the exception to that rule and are counted anyway. The
		// fleet's fullest sits at 49.9% and has never crossed 70%, so by the test
		// above the tile would have been cut. It is here because the test is the
		// wrong one for this signal: CPU saturation, memory pressure and reboots
		// are transients that come back down by themselves, whereas a filesystem
		// only moves one way, and "has not happened yet" says nothing about whether
		// it will. A counter that sits at zero for months and then does not is
		// exactly what a summary counter is for.
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Nodes Down").
				Description("Scrape targets not answering. Red from one, unlike the other tiles here: this is the only one that is a fault rather than a state.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds()).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(up{job="scrapeConfig/monitoring/node-exporter-external", ` + instFilter + `} == 0) or vector(0)`).
					Instant().
					LegendFormat("down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Rebooted (1h)").
				Description("Nodes that booted within the last hour. Not a fault, but it explains gaps and resets elsewhere on this dashboard; a maintenance window showed 10 at once.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count((time() - node_boot_time_seconds{` + instFilter + `}) < 3600) or vector(0)`).
					Instant().
					LegendFormat("rebooted"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Nodes Saturated").
				Description("Nodes whose 1-minute load average exceeds their CPU count, meaning work was queuing. Same quantity as the Load Average panels below and on node/k8s-node/proxmox-otlp, so a figure here can be compared with any of them.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count((node_load1{` + instFilter + `} ` + normByCPU + `) > 1) or vector(0)`).
					Instant().
					LegendFormat("saturated"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Nodes Over 80% Memory").
				Description("Counted at 80% because that is where capacityThresholds turns yellow everywhere else in this repo. 90% was tried and has not happened once in 30 days.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(((1 - node_memory_MemAvailable_bytes{` + instFilter + `} / node_memory_MemTotal_bytes{` + instFilter + `}) * 100) > 80) or vector(0)`).
					Instant().
					LegendFormat("over 80%"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Filesystems Over 85%").
				Description("Counts filesystems, not nodes: one host can have several, and on pve they share a ZFS pool. 85% is where NodeFilesystemSpaceFillingUp starts looking at the trend, so a non-zero count here is the level that alert needs before it will fire -- it is the earlier and weaker of the two signals, and does not by itself mean anything is alerting.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(((1 - node_filesystem_avail_bytes{` + instFilter + `,` + fsFilter + `} / node_filesystem_size_bytes{` + instFilter + `,` + fsFilter + `}) * 100) > 85) or vector(0)`).
					Instant().
					LegendFormat("over 85%"),
				),
		).

		// The per-node strips the Summary used to carry. Current values, one tile
		// or bar per node; the rows below chart the same quantities over time.
		//
		// The pairing is the organising rule of this dashboard: a current-value
		// gauge here, its trend in the row named for the subject. CPU Usage pairs
		// with CPU Usage (%), Memory Usage with Memory Usage, Load Average per CPU
		// with Load Average per CPU (1m). Filesystem Usage was the one that did not
		// follow it -- both halves sat together down in the Disk row -- so the
		// Summary's "Filesystems Over 85%" count had nowhere to resolve to a name
		// without scrolling past everything else. Its trend stayed behind.
		//
		// All three capacity gauges sort_desc. Nineteen nodes, and twenty-seven
		// filesystems, do not fit the ten grid rows they are given, so the panel
		// scrolls and only the first few bars are visible without dragging. Sorted
		// by value the visible ones are the ones worth seeing; in label order they
		// were whichever hostnames happened to sort first. The sort wraps the whole
		// expression rather than an inner part of it: order does survive the
		// nodename join in practice -- checked against the live series -- but
		// nothing documents that it must, and there is no reason to depend on it.
		WithRow(dashboard.NewRowBuilder("Current State by Node")).
		// up{job=...} is always recorded by Prometheus for every configured scrape
		// target, returning 0 when the target is unreachable. Joining with
		// last_over_time(node_uname_info[1d]) resolves nodenames even while a node
		// is down (as long as it was seen within the last day).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node Exporter Status").
				Datasource(ds).
				Span(24).Height(4).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
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
							"0": {Text: new("DOWN"), Color: new("red")},
							"1": {Text: new("UP"), Color: new("green")},
						},
					}},
				}).
				WithTarget(prometheus.NewDataqueryBuilder().
					// max by dedupes node_uname_info when a kernel upgrade + reboot leaves
					// two series (differing only in "release") for the same instance
					// within the last_over_time lookback window.
					Expr(`up{job="scrapeConfig/monitoring/node-exporter-external", ` + instFilter + `} * on(instance) group_left(nodename) max by (instance, nodename) (last_over_time(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}[1d]))`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				// Height 6 rather than the 4 the neighbouring stats use. A stat panel
				// sizes its text to whatever box it is given, and nineteen values
				// across a full-width row leave each about 74px wide; at height 4
				// there was so little left after the node name that the figures came
				// out barely legible. Node Exporter Status keeps height 4 beside it
				// because "UP" survives being small in a way "3 week" does not.
				Span(24).Height(6).
				Unit("s").
				Min(0).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(3600)), Color: "yellow"},
						{Value: new(float64(86400)), Color: "green"},
					}),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(time() - node_boot_time_seconds{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(0),
		).
		// A "Filesystem Usage (Current)" bar gauge used to sit here, span 24. It ran
		// the same query as "Filesystem Usage" in the Disk row -- the only textual
		// difference was whether the nodename join fell inside or outside
		// sort_desc(), which does not change the result -- so the same bars were
		// drawn twice on one dashboard, and a third panel below charts the same
		// figure over time. The Disk row keeps the pair that answer different
		// questions, current level and trend; Summary is for reading at a glance
		// and a full-width duplicate was the largest thing on it.
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("CPU Usage").
				Datasource(ds).
				Span(8).Height(10).
				Unit("percent").
				Min(0).
				Max(100).
				// Horizontal in Grafana's naming, which draws each bar running left to
				// right with the node name beside it and stacks them down the panel.
				// Auto chose the other layout here: at span 8 the panel is wider than
				// tall, so it stood nineteen bars up side by side and had nowhere to put
				// the names. Matches Filesystem Usage next to it, so the three capacity
				// gauges on this line are read the same way.
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + instFilter + `}[$__rate_interval]) ` + joinNodename + `) * 100))`).
					Instant().
					LegendFormat("{{nodename}}"),
				).
				Decimals(1),
		).
		// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(8).Height(10).
				Unit("percent").
				Min(0).
				Max(100).
				// Horizontal in Grafana's naming, which draws each bar running left to
				// right with the node name beside it and stacks them down the panel.
				// Auto chose the other layout here: at span 8 the panel is wider than
				// tall, so it stood nineteen bars up side by side and had nowhere to put
				// the names. Matches Filesystem Usage next to it, so the three capacity
				// gauges on this line are read the same way.
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc((1 - node_memory_MemAvailable_bytes{` + instFilter + `} / node_memory_MemTotal_bytes{` + instFilter + `}) ` + joinNodename + ` * 100)`).
					Instant().
					LegendFormat("{{nodename}}"),
				).Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Filesystem Usage").
				Description("Every filesystem in scope, fullest first. This is where the \"Filesystems Over 85%\" count in the Summary resolves to a name.").
				Datasource(ds).
				Span(8).Height(10).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc((1 - node_filesystem_avail_bytes{` + instFilter + `,` + fsFilter + `} / node_filesystem_size_bytes{` + instFilter + `,` + fsFilter + `}) * 100 ` + joinNodename + `)`).
					Instant().
					LegendFormat("{{nodename}} {{mountpoint}}"),
				).
				Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Load Average (1m) per CPU").
				Datasource(ds).
				Span(24).Height(4).
				Unit("percentunit").
				Min(0).
				// No sparkline, matching Node Exporter Status and Uptime above it.
				// This row answers "what is each node doing right now"; the trend
				// belongs to "Load Average per CPU (1m)" in the CPU row, which has a
				// full panel and a legend for it. Squeezed behind nineteen values in
				// a strip four grid rows tall, the same curve was decoration over a
				// number that is the point of the panel.
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(0.7)), Color: "yellow"},
						{Value: new(float64(1.0)), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_load1{` + instFilter + `} ` + normByCPU + `) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(0),
		).
		WithRow(dashboard.NewRowBuilder("CPU")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds()).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + instFilter + `}[$__rate_interval]) ` + joinNodename + `) * 100)`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Load Average per CPU (1m)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_load1{` + instFilter + `} ` + normByCPU + `) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		// Used = Total - Available (buffers/cache are included in Available).
		WithRow(dashboard.NewRowBuilder("Memory")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Used").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_memory_MemTotal_bytes{` + instFilter + `} - node_memory_MemAvailable_bytes{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds()).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(1 - node_memory_MemAvailable_bytes{` + instFilter + `} / node_memory_MemTotal_bytes{` + instFilter + `}) * 100 ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		// PSI: fraction of time at least one task was stalled ("some") or all
		// tasks were stalled ("full") waiting on the resource. CPU has no "full"
		// series since the kernel doesn't track fully-stalled CPU time.
		WithRow(dashboard.NewRowBuilder("Pressure (PSI)")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Pressure (some)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_pressure_cpu_waiting_seconds_total{` + instFilter + `}[$__rate_interval]) * 100 ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Pressure").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Some").
					Expr(`rate(node_pressure_memory_waiting_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} some"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Full").
					Expr(`rate(node_pressure_memory_stalled_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} full"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("IO Pressure").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Some").
					Expr(`rate(node_pressure_io_waiting_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} some"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Full").
					Expr(`rate(node_pressure_io_stalled_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} full"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Temperature & Throttling")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				Tooltip(tooltipAll).
				Legend(legend).
				// CPU temp: x86 Package (Intel), cpu-thermal (RPi), or k10temp Tctl (Ryzen, PCI device 0000:00:18.x)
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`((label_replace(node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal", ` + instFilter + `}, "sensor", "$1", "type", "(.*)")) or (label_replace(node_hwmon_temp_celsius{chip=~".*_0000:00:18_.*", sensor="temp1", ` + instFilter + `}, "sensor", "cpu", "", ""))) ` + joinNodename).
					LegendFormat("{{nodename}} CPU {{sensor}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(nvme_temperature_celsius{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} NVMe {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(smartmon_temperature_celsius_raw_value{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} Disk {{device}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Throttling Rate").
				Description("Package thermal throttling on the machines that own their CPUs. LXC guests are excluded: the counter reaches them from the shared kernel and describes the hypervisor, not the container. Only the Intel hypervisors report it -- pve is AMD and the Raspberry Pis are ARM, and neither exposes this counter.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Restricted to the hypervisors. node_cpu_package_throttles_total
					// comes from the cpu collector, which cannot be turned off on the
					// LXC guests without losing their CPU usage as well (see
					// group_vars/node_exporter_lxc.yaml), so unlike thermal_zone and
					// hwmon it still leaks the host's reading into each container:
					// thirteen series were being drawn for five physical CPUs, and
					// ns1's "throttling" was node2's. The other temperature panels need
					// no such guard -- their collectors are disabled on the guests, and
					// the nvme and smartmon figures come from the textfile collector on
					// the physical host itself.
					Expr(`(rate(node_cpu_package_throttles_total{` + instFilter + `, instance=~"` + proxmoxHosts + `"}[$__rate_interval])) ` + joinNodename).
					LegendFormat("{{nodename}} Throttles"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("RPi Power & Thermal Status").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Max(1).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(issueThresholds()).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: new("OK"), Color: new("green")},
							"1": {Text: new("ISSUE"), Color: new("red")},
						},
					}},
				}).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_thermal_throttling{` + instFilter + `}) ` + joinNodename).
					Instant().
					LegendFormat("{{nodename}} Thermal Throttled"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_occurred{` + instFilter + `}) ` + joinNodename).
					Instant().
					LegendFormat("{{nodename}} Thermal Throttled Since Boot"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_under_voltage{` + instFilter + `}) ` + joinNodename).
					Instant().
					LegendFormat("{{nodename}} Under Voltage"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_under_voltage_occurred{` + instFilter + `}) ` + joinNodename).
					Instant().
					LegendFormat("{{nodename}} Under Voltage Since Boot"),
				),
		).
		// Exclude dm-*, loop*, and sr* to avoid double-counting or noise from virtual/optical devices.
		WithRow(dashboard.NewRowBuilder("Network")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(denseTooltip()).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Rx").
					// Keep physical NICs and vmbr (Proxmox bridges); exclude per-VM/LXC virtual interfaces.
					Expr(`rate(node_network_receive_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|podman.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*|vnets.*"}[$__rate_interval]) `+joinNodename).
					LegendFormat("{{nodename}} Rx {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Tx").
					Expr(`rate(node_network_transmit_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|podman.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*|vnets.*"}[$__rate_interval]) `+joinNodename).
					LegendFormat("{{nodename}} Tx {{device}}"),
				).
				OverrideByQuery("Tx", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Disk")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Disk I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(denseTooltip()).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_disk_read_bytes_total{`+instFilter+`, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[$__rate_interval]) `+joinNodename).
					LegendFormat("{{nodename}} Read {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Write").
					Expr(`rate(node_disk_written_bytes_total{`+instFilter+`, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[$__rate_interval]) `+joinNodename).
					LegendFormat("{{nodename}} Write {{device}}"),
				).
				OverrideByQuery("Write", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Filesystem Usage Trend").
				Datasource(ds).
				Span(24).Height(10).
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds()).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(1 - node_filesystem_avail_bytes{` + instFilter + `,` + fsFilter + `} / node_filesystem_size_bytes{` + instFilter + `,` + fsFilter + `}) * 100 ` + joinNodename).
					LegendFormat("{{nodename}} {{mountpoint}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("ZFS")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("ZFS ARC Size").
				Description("Only nodes with a pool imported appear; see zfsActive in the source for how they are told apart from the ones that merely have the module loaded. A node that gains a pool will show up here on its own, with no change needed to this panel.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				// Min and Max are the ARC's configured bounds, so they are drawn
				// dashed against the solid line of the size that moves between them.
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("A").
					Expr(`(node_zfs_arc_size{`+instFilter+`}`+zfsActive+`) `+joinNodename).
					LegendFormat("{{nodename}} ARC Size"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("B").
					Expr(`(node_zfs_arc_c_max{`+instFilter+`}`+zfsActive+`) `+joinNodename).
					LegendFormat("{{nodename}} ARC Max"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("C").
					Expr(`(node_zfs_arc_c_min{`+instFilter+`}`+zfsActive+`) `+joinNodename).
					LegendFormat("{{nodename}} ARC Min"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* ARC (Max|Min)$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
