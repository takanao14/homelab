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
	nodePanelNodesDown             = 1
	nodePanelRebooted1h            = 2
	nodePanelNodesSaturated        = 3
	nodePanelNodesOver80Memory     = 4
	nodePanelFilesystemsOver85     = 5
	nodePanelNodeExporterStatus    = 6
	nodePanelUptime                = 7
	nodePanelCPUUsageCurrent       = 8
	nodePanelMemoryUsageCurrent    = 9
	nodePanelFilesystemUsage       = 10
	nodePanelLoadAverage1mPerCPU   = 11
	nodePanelCPUUsage              = 12
	nodePanelLoadAveragePerCPU1m   = 13
	nodePanelMemoryUsed            = 14
	nodePanelMemoryUsage           = 15
	nodePanelCPUPressureSome       = 16
	nodePanelMemoryPressure        = 17
	nodePanelIOPressure            = 18
	nodePanelTemperature           = 19
	nodePanelCPUThrottlingRate     = 20
	nodePanelRPiPowerThermalStatus = 21
	nodePanelNetworkIO             = 22
	nodePanelDiskIO                = 23
	nodePanelFilesystemUsageTrend  = 24
	nodePanelZFSARCSize            = 25
)

func buildNodeOverview() (*dashboardv2.Dashboard, error) {
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
	noticeThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	d := dashboardv2.NewDashboardBuilder("Node Overview").
		Tags([]string{"nodes", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-1d").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		// External bare-metal and VM/LXC targets; Kubernetes nodes use another job.
		QueryVariable(dashboardv2.NewQueryVariableBuilder("node").
			Label("Node").
			Query(promVariableQuery(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}, nodename)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true)).
		// Resolve selected nodenames to instances; multi-select produces a regex.
		QueryVariable(dashboardv2.NewQueryVariableBuilder("instance").
			Query(promVariableQuery(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename=~"$node"}, instance)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Multi(true).
			IncludeAll(true).
			Hide(dashboardv2.VariableHideHideVariable))

	return newLayout(d).
		// Summary counts actionable states instead of enumerating every node.
		// Lifecycle and pressure form balanced lines. Filesystem usage remains even
		// with a zero history because persistent growth differs from transient load.
		Row("Summary", statGrid(4)).
		Panel(nodePanelNodesDown, dashboardv2.NewPanelBuilder().
			Title("Nodes Down").
			Description("Scrape targets not answering. Red from one, unlike the other tiles here: this is the only one that is a fault rather than a state.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds()).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				Expr(`count(up{job="scrapeConfig/monitoring/node-exporter-external", `+instFilter+`} == 0) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("down")))).
		Panel(nodePanelRebooted1h, dashboardv2.NewPanelBuilder().
			Title("Rebooted (1h)").
			Description("Nodes that booted within the last hour. Not a fault, but it explains gaps and resets elsewhere on this dashboard; a maintenance window showed 10 at once.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				Expr(`count((time() - node_boot_time_seconds{`+ownKernel+`}) < 3600) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("rebooted")))).
		Panel(nodePanelNodesSaturated, dashboardv2.NewPanelBuilder().
			Title("Nodes Saturated").
			Description("Nodes whose 1-minute load average exceeds their CPU count, meaning work was queuing. Same quantity as the Load Average panels below and on node/k8s-node/proxmox-otlp, so a figure here can be compared with any of them.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				Expr(`count((node_load1{`+instFilter+`} `+normByCPU+`) > 1) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("saturated")))).
		Panel(nodePanelNodesOver80Memory, dashboardv2.NewPanelBuilder().
			Title("Nodes Over 80% Memory").
			Description("Counted at 80% because that is where capacityThresholds turns yellow everywhere else in this repo. 90% was tried and has not happened once in 30 days.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				Expr(`count(((1 - node_memory_MemAvailable_bytes{`+instFilter+`} / node_memory_MemTotal_bytes{`+instFilter+`}) * 100) > 80) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("over 80%")))).
		Panel(nodePanelFilesystemsOver85, dashboardv2.NewPanelBuilder().
			Title("Filesystems Over 85%").
			Description("Counts filesystems, not nodes: one host can have several, and on pve they share a ZFS pool. 85% is where NodeFilesystemSpaceFillingUp starts looking at the trend, so a non-zero count here is the level that alert needs before it will fire -- it is the earlier and weaker of the two signals, and does not by itself mean anything is alerting.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(noticeThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				Expr(`count(((1 - node_filesystem_avail_bytes{`+instFilter+`,`+fsFilter+`} / node_filesystem_size_bytes{`+instFilter+`,`+fsFilter+`}) * 100) > 85) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("over 85%")))).
		// Current per-node values pair with trends in later rows. Sort capacity
		// gauges so visible bars show the most constrained resources.
		Row("Current State by Node", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// One day of node_uname_info history resolves names while targets are down.
		Panel(nodePanelNodeExporterStatus, dashboardv2.NewPanelBuilder().
			Title("Node Exporter Status").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
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
							"0": {Text: new("DOWN"), Color: new("red")},
							"1": {Text: new("UP"), Color: new("green")},
						},
					}},
				})).
			Data(queries(promQuery().
				// Deduplicate stale kernel-release series in the lookback.
				Expr(`up{job="scrapeConfig/monitoring/node-exporter-external", `+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (last_over_time(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}[1d]))`).
				LegendFormat("{{nodename}}")))).
		Panel(nodePanelUptime, dashboardv2.NewPanelBuilder().
			Title("Uptime").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(3600)), Color: "yellow"},
						{Value: new(float64(86400)), Color: "green"},
					})).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`(time() - node_boot_time_seconds{`+ownKernel+`}) `+joinNodename).
				LegendFormat("{{nodename}}")))).
		Row("CPU Usage (Current)", chartGrid()).
		// Keep filesystem level and trend together in the Disk row; avoid a
		// duplicate full-width Summary gauge.
		Panel(nodePanelCPUUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("CPU Usage (Current)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				// Horizontal bars preserve labels in narrow panels.
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc(100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+instFilter+`}[$__rate_interval]) `+joinNodename+`) * 100))`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}}")))).
		// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
		Panel(nodePanelMemoryUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("Memory Usage (Current)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				// Fix horizontal bars so node names remain readable in narrow panels.
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc((1 - node_memory_MemAvailable_bytes{`+instFilter+`} / node_memory_MemTotal_bytes{`+instFilter+`}) `+joinNodename+` * 100)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}}")))).
		Row("Filesystem Usage", chartGrid()).
		Panel(nodePanelFilesystemUsage, dashboardv2.NewPanelBuilder().
			Title("Filesystem Usage").
			Description("Every filesystem in scope, fullest first. This is where the \"Filesystems Over 85%\" count in the Summary resolves to a name.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc((1 - node_filesystem_avail_bytes{`+instFilter+`,`+fsFilter+`} / node_filesystem_size_bytes{`+instFilter+`,`+fsFilter+`}) * 100 `+joinNodename+`)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{mountpoint}}")))).
		Row("Load Average (1m) per CPU", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelLoadAverage1mPerCPU, dashboardv2.NewPanelBuilder().
			Title("Load Average (1m) per CPU").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				// Show current load only; the CPU row owns the trend.
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(0.7)), Color: "yellow"},
						{Value: new(float64(1.0)), Color: "red"},
					})).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`(node_load1{`+instFilter+`} `+normByCPU+`) `+joinNodename).
				LegendFormat("{{nodename}}")))).
		Row("CPU Usage (%)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelCPUUsage, dashboardv2.NewPanelBuilder().
			Title("CPU Usage (%)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds()).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+instFilter+`}[$__rate_interval]) `+joinNodename+`) * 100)`).
				LegendFormat("{{nodename}}")))).
		Row("Load Average per CPU (1m)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelLoadAveragePerCPU1m, dashboardv2.NewPanelBuilder().
			Title("Load Average per CPU (1m)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`(node_load1{`+instFilter+`} `+normByCPU+`) `+joinNodename).
				LegendFormat("{{nodename}}")))).
		// Used = Total - Available (buffers/cache are included in Available).
		Row("Memory", chartGrid()).
		Panel(nodePanelMemoryUsed, dashboardv2.NewPanelBuilder().
			Title("Memory Used").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`(node_memory_MemTotal_bytes{`+instFilter+`} - node_memory_MemAvailable_bytes{`+instFilter+`}) `+joinNodename).
				LegendFormat("{{nodename}}")))).
		Panel(nodePanelMemoryUsage, dashboardv2.NewPanelBuilder().
			Title("Memory Usage").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds()).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`(1 - node_memory_MemAvailable_bytes{`+instFilter+`} / node_memory_MemTotal_bytes{`+instFilter+`}) * 100 `+joinNodename).
				LegendFormat("{{nodename}}")))).
		// PSI `some` is partial stall time; `full` is total stall time.
		// Linux exposes no CPU `full` series.
		Row("CPU Pressure (some)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelCPUPressureSome, dashboardv2.NewPanelBuilder().
			Title("CPU Pressure (some)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`rate(node_pressure_cpu_waiting_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}}")))).
		Row("Memory Pressure", chartGrid()).
		Panel(nodePanelMemoryPressure, dashboardv2.NewPanelBuilder().
			Title("Memory Pressure").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				})).
			Data(queries(queryWithRef("Some", promQuery().
				RefId("Some").
				Expr(`rate(node_pressure_memory_waiting_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}} some")),
				queryWithRef("Full", promQuery().
					RefId("Full").
					Expr(`rate(node_pressure_memory_stalled_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} full"))))).
		Panel(nodePanelIOPressure, dashboardv2.NewPanelBuilder().
			Title("IO Pressure").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				})).
			Data(queries(queryWithRef("Some", promQuery().
				RefId("Some").
				Expr(`rate(node_pressure_io_waiting_seconds_total{`+ownKernel+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}} some")),
				queryWithRef("Full", promQuery().
					RefId("Full").
					Expr(`rate(node_pressure_io_stalled_seconds_total{`+ownKernel+`}[$__rate_interval]) * 100 `+joinNodename).
					LegendFormat("{{nodename}} full"))))).
		Row("Temperature", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelTemperature, dashboardv2.NewPanelBuilder().
			Title("Temperature").
			Visualization(timeseries.NewVisualizationV2Builder().
					Unit("celsius").
					Tooltip(tooltipAll).
					Legend(legend)).
			Data(queries( // CPU temp: x86 Package (Intel), cpu-thermal (RPi), or k10temp Tctl (Ryzen, PCI device 0000:00:18.x)
				promQuery().
					Expr(`((label_replace(node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal", `+instFilter+`}, "sensor", "$1", "type", "(.*)")) or (label_replace(node_hwmon_temp_celsius{chip=~".*_0000:00:18_.*", sensor="temp1", `+instFilter+`}, "sensor", "cpu", "", ""))) `+joinNodename).
					LegendFormat("{{nodename}} CPU {{sensor}}"),
				promQuery().
					Expr(`(nvme_temperature_celsius{`+instFilter+`}) `+joinNodename).
					LegendFormat("{{nodename}} NVMe {{device}}"),
				promQuery().
					Expr(`(smartmon_temperature_celsius_raw_value{`+instFilter+`}) `+joinNodename).
					LegendFormat("{{nodename}} Disk {{device}}")))).
		Row("CPU Throttling Rate", chartGrid()).
		Panel(nodePanelCPUThrottlingRate, dashboardv2.NewPanelBuilder().
			Title("CPU Throttling Rate").
			Description("Package thermal throttling on the machines that own their CPUs. LXC guests are excluded: the counter reaches them from the shared kernel and describes the hypervisor, not the container. Only the Intel hypervisors report it -- pve is AMD and the Raspberry Pis are ARM, and neither exposes this counter.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Restrict package throttles to hypervisors because LXC guests inherit
				// the host cpu collector series.
				Expr(`(rate(node_cpu_package_throttles_total{`+instFilter+`, instance=~"`+proxmoxHosts+`"}[$__rate_interval])) `+joinNodename).
				LegendFormat("{{nodename}} Throttles")))).
		Row("RPi Power & Thermal Status", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelRPiPowerThermalStatus, dashboardv2.NewPanelBuilder().
			Title("RPi Power & Thermal Status").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Max(1).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				Text(common.NewVizTextDisplayOptionsBuilder().
					TitleSize(16).ValueSize(32)).
				Thresholds(issueThresholds()).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("OK"), Color: new("green")},
							"1": {Text: new("ISSUE"), Color: new("red")},
						},
					}},
				})).
			Data(queries(promQuery().
				Expr(`(rpi_throttled_thermal_throttling{`+instFilter+`}) `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} Thermal Throttled"),
				promQuery().
					Expr(`(rpi_throttled_occurred{`+instFilter+`}) `+joinNodename).
					Instant(true).Range(false).
					LegendFormat("{{nodename}} Thermal Throttled Since Boot"),
				promQuery().
					Expr(`(rpi_throttled_under_voltage{`+instFilter+`}) `+joinNodename).
					Instant(true).Range(false).
					LegendFormat("{{nodename}} Under Voltage"),
				promQuery().
					Expr(`(rpi_throttled_under_voltage_occurred{`+instFilter+`}) `+joinNodename).
					Instant(true).Range(false).
					LegendFormat("{{nodename}} Under Voltage Since Boot")))).
		// Exclude dm-*, loop*, and sr* to avoid double-counting or noise from virtual/optical devices.
		Row("Network", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelNetworkIO, dashboardv2.NewPanelBuilder().
			Title("Network I/O").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Tooltip(denseTooltip()).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				OverrideByQuery("Tx", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(queryWithRef("Rx", promQuery().
				RefId("Rx").
				// Keep physical NICs and vmbr (Proxmox bridges); exclude per-VM/LXC virtual interfaces.
				Expr(`rate(node_network_receive_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|podman.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*|vnets.*"}[$__rate_interval]) `+joinNodename).
				LegendFormat("{{nodename}} Rx {{device}}")),
				queryWithRef("Tx", promQuery().
					RefId("Tx").
					Expr(`rate(node_network_transmit_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|podman.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*|vnets.*"}[$__rate_interval]) `+joinNodename).
					LegendFormat("{{nodename}} Tx {{device}}"))))).
		Row("Disk I/O", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelDiskIO, dashboardv2.NewPanelBuilder().
			Title("Disk I/O").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Tooltip(denseTooltip()).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				OverrideByQuery("Write", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(promQuery().
				Expr(`rate(node_disk_read_bytes_total{`+instFilter+`, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[$__rate_interval]) `+joinNodename).
				LegendFormat("{{nodename}} Read {{device}}"),
				queryWithRef("Write", promQuery().
					RefId("Write").
					Expr(`rate(node_disk_written_bytes_total{`+instFilter+`, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[$__rate_interval]) `+joinNodename).
					LegendFormat("{{nodename}} Write {{device}}"))))).
		Row("Filesystem Usage Trend", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelFilesystemUsageTrend, dashboardv2.NewPanelBuilder().
			Title("Filesystem Usage Trend").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds()).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`(1 - node_filesystem_avail_bytes{`+instFilter+`,`+fsFilter+`} / node_filesystem_size_bytes{`+instFilter+`,`+fsFilter+`}) * 100 `+joinNodename).
				LegendFormat("{{nodename}} {{mountpoint}}")))).
		Row("ZFS", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(nodePanelZFSARCSize, dashboardv2.NewPanelBuilder().
			Title("ZFS ARC Size").
			Description("Only nodes with a pool imported appear; see zfsActive in the source for how they are told apart from the ones that merely have the module loaded. A node that gains a pool will show up here on its own, with no change needed to this panel.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* ARC (Max|Min)$"}, []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
					})).
			Data(queries( // Draw configured ARC bounds dashed around the live size.
				queryWithRef("A", promQuery().
					RefId("A").
					Expr(`(node_zfs_arc_size{`+instFilter+`}`+zfsActive+`) `+joinNodename).
					LegendFormat("{{nodename}} ARC Size")),
				queryWithRef("B", promQuery().
					RefId("B").
					Expr(`(node_zfs_arc_c_max{`+instFilter+`}`+zfsActive+`) `+joinNodename).
					LegendFormat("{{nodename}} ARC Max")),
				queryWithRef("C", promQuery().
					RefId("C").
					Expr(`(node_zfs_arc_c_min{`+instFilter+`}`+zfsActive+`) `+joinNodename).
					LegendFormat("{{nodename}} ARC Min"))))).
		Build()
}
