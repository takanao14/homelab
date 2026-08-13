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

	// Shared Proxmox inventory for the throttling panel.
	proxmoxHosts, err := loadProxmoxHostRegex()
	if err != nil {
		return nil, err
	}
	lxcGuests, err := loadLxcGuestRegex()
	if err != nil {
		return nil, err
	}
	// Resolve visible nodenames to hidden scrape instances.
	// joinNodename adds display names to query results.
	const (
		instFilter = `instance=~"$instance"`
		// Deduplicate hosts scraped by multiple jobs.
		joinNodename = `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info)`
		// Normalize load by logical CPU count; 1.0 means saturated.
		normByCPU = `/ on(instance) group_left() count by (instance) (node_cpu_seconds_total{mode="idle", ` + instFilter + `})`
		// Exclude pseudo and boot filesystems.
		fsFilter = `fstype=~"ext[234]|xfs|btrfs|zfs|vfat",mountpoint!~"/var/lib/docker/.*|/boot/efi|/boot/firmware"`
		// Show ZFS only for hosts with an imported pool. Gate live size and limits
		// together because module-only hosts expose a tiny ARC with GiB-scale bounds.
		// Keeping the collector enabled lets newly imported pools appear automatically.
		zfsActive = ` and on(instance) (node_zfs_arc_size{` + instFilter + `} > 1024 * 1024)`
	)

	// Exclude LXC guests where /proc reports host IO pressure and boot time.
	// CPU/memory PSI remains container-specific through lxcfs.
	ownKernel := instFilter + `, instance!~"` + lxcGuests + `"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// Dense I/O panels sort tooltips by value and cap their height. Pinned
	// tooltips remain scrollable; HideZeros removes idle devices.
	denseTooltip := func() *common.VizTooltipOptionsBuilder {
		return common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti).
			Sort(common.SortOrderDescending).
			MaxHeight(400).
			HideZeros(true)
	}

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// Summary conditions warn from one; only an unreachable node is critical.
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
		// External bare-metal and VM/LXC targets; Kubernetes nodes use another job.
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
		// Resolve selected nodenames to instances; multi-select produces a regex.
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).
		// Summary counts actionable states instead of enumerating every node.
		// Lifecycle and pressure form balanced lines. Filesystem usage remains even
		// with a zero history because persistent growth differs from transient load.
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
					Expr(`count((time() - node_boot_time_seconds{` + ownKernel + `}) < 3600) or vector(0)`).
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

		// Current per-node values pair with trends in later rows. Sort capacity
		// gauges so visible bars show the most constrained resources.
		// One day of node_uname_info history preserves names while targets are down.
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
					// Deduplicate stale kernel-release series in the lookback.
					Expr(`up{job="scrapeConfig/monitoring/node-exporter-external", ` + instFilter + `} * on(instance) group_left(nodename) max by (instance, nodename) (last_over_time(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}[1d]))`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				// Extra height keeps nineteen uptime values legible.
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
					Expr(`(time() - node_boot_time_seconds{` + ownKernel + `}) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(0),
		).
		// Keep filesystem level and trend together in the Disk row; avoid a
		// duplicate full-width Summary gauge.
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("CPU Usage").
				Datasource(ds).
				Span(8).Height(10).
				Unit("percent").
				Min(0).
				Max(100).
				// Horizontal bars preserve labels in narrow panels.
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
				// Show current load only; the CPU row owns the trend.
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
		// PSI `some` is partial stall time; `full` is total stall time.
		// Linux exposes no CPU `full` series.
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
					Expr(`rate(node_pressure_io_waiting_seconds_total{`+ownKernel+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} some"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Full").
					Expr(`rate(node_pressure_io_stalled_seconds_total{`+ownKernel+`}[$__rate_interval]) * 100 `+joinNodename).
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
					// Restrict package throttles to hypervisors because LXC guests inherit
					// the host cpu collector series.
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
				// Draw configured ARC bounds dashed around the live size.
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
