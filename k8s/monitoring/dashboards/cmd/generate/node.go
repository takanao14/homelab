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
	const (
		elementNodesDown             = "nodes-down"
		elementRebooted1h            = "rebooted-1h"
		elementNodesSaturated        = "nodes-saturated"
		elementNodesOver80Memory     = "nodes-over-80-memory"
		elementFilesystemsOver85     = "filesystems-over-85"
		elementNodeExporterStatus    = "node-exporter-status"
		elementUptime                = "uptime"
		elementCPUUsageCurrent       = "cpu-usage-current"
		elementMemoryUsageCurrent    = "memory-usage-current"
		elementFilesystemUsage       = "filesystem-usage"
		elementLoadAverage1mPerCPU   = "load-average-1m-per-cpu"
		elementCPUUsage              = "cpu-usage"
		elementLoadAveragePerCPU1m   = "load-average-per-cpu-1m"
		elementMemoryUsed            = "memory-used"
		elementMemoryUsage           = "memory-usage"
		elementCPUPressureSome       = "cpu-pressure-some"
		elementMemoryPressure        = "memory-pressure"
		elementIOPressure            = "io-pressure"
		elementTemperature           = "temperature"
		elementCPUThrottlingRate     = "cpu-throttling-rate"
		elementRPiPowerThermalStatus = "rpi-power-thermal-status"
		elementNetworkIO             = "network-i-o"
		elementDiskIO                = "disk-i-o"
		elementFilesystemUsageTrend  = "filesystem-usage-trend"
		elementZFSARCSize            = "zfs-arc-size"
	)

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

	d.Panel(elementNodesDown, dashboardv2.NewPanelBuilder().
		Id(nodePanelNodesDown).
		Title("Nodes Down").
		Description("Scrape targets not answering. Red from one, unlike the other tiles here: this is the only one that is a fault rather than a state.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds()).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(up{job="scrapeConfig/monitoring/node-exporter-external", `+instFilter+`} == 0) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("down")))))

	d.Panel(elementRebooted1h, dashboardv2.NewPanelBuilder().
		Id(nodePanelRebooted1h).
		Title("Rebooted (1h)").
		Description("Nodes that booted within the last hour. Not a fault, but it explains gaps and resets elsewhere on this dashboard; a maintenance window showed 10 at once.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(noticeThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count((time() - node_boot_time_seconds{`+ownKernel+`}) < 3600) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("rebooted")))))

	d.Panel(elementNodesSaturated, dashboardv2.NewPanelBuilder().
		Id(nodePanelNodesSaturated).
		Title("Nodes Saturated").
		Description("Nodes whose 1-minute load average exceeds their CPU count, meaning work was queuing. Same quantity as the Load Average panels below and on node/k8s-node/proxmox-otlp, so a figure here can be compared with any of them.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(noticeThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count((node_load1{`+instFilter+`} `+normByCPU+`) > 1) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("saturated")))))

	d.Panel(elementNodesOver80Memory, dashboardv2.NewPanelBuilder().
		Id(nodePanelNodesOver80Memory).
		Title("Nodes Over 80% Memory").
		Description("Counted at 80% because that is where capacityThresholds turns yellow everywhere else in this repo. 90% was tried and has not happened once in 30 days.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(noticeThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(((1 - node_memory_MemAvailable_bytes{`+instFilter+`} / node_memory_MemTotal_bytes{`+instFilter+`}) * 100) > 80) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("over 80%")))))

	d.Panel(elementFilesystemsOver85, dashboardv2.NewPanelBuilder().
		Id(nodePanelFilesystemsOver85).
		Title("Filesystems Over 85%").
		Description("Counts filesystems, not nodes: one host can have several, and on pve they share a ZFS pool. 85% is where NodeFilesystemSpaceFillingUp starts looking at the trend, so a non-zero count here is the level that alert needs before it will fire -- it is the earlier and weaker of the two signals, and does not by itself mean anything is alerting.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(noticeThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(((1 - node_filesystem_avail_bytes{`+instFilter+`,`+fsFilter+`} / node_filesystem_size_bytes{`+instFilter+`,`+fsFilter+`}) * 100) > 85) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("over 85%")))))

	// One day of node_uname_info history resolves names while targets are down.
	d.Panel(elementNodeExporterStatus, dashboardv2.NewPanelBuilder().
		Id(nodePanelNodeExporterStatus).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Deduplicate stale kernel-release series in the lookback.
				Expr(`up{job="scrapeConfig/monitoring/node-exporter-external", `+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (last_over_time(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external"}[1d]))`).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementUptime, dashboardv2.NewPanelBuilder().
		Id(nodePanelUptime).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(time() - node_boot_time_seconds{`+ownKernel+`}) `+joinNodename).
				LegendFormat("{{nodename}}")))))

	// Keep filesystem level and trend together in the Disk row; avoid a
	// duplicate full-width Summary gauge.
	d.Panel(elementCPUUsageCurrent, dashboardv2.NewPanelBuilder().
		Id(nodePanelCPUUsageCurrent).
		Title("CPU Usage (Current)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			// Horizontal bars preserve labels in narrow panels.
			Orientation(common.VizOrientationHorizontal).
			Thresholds(capacityThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+instFilter+`}[$__rate_interval]) `+joinNodename+`) * 100))`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}}")))))

	// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
	d.Panel(elementMemoryUsageCurrent, dashboardv2.NewPanelBuilder().
		Id(nodePanelMemoryUsageCurrent).
		Title("Memory Usage (Current)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			// Fix horizontal bars so node names remain readable in narrow panels.
			Orientation(common.VizOrientationHorizontal).
			Thresholds(capacityThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc((1 - node_memory_MemAvailable_bytes{`+instFilter+`} / node_memory_MemTotal_bytes{`+instFilter+`}) `+joinNodename+` * 100)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementFilesystemUsage, dashboardv2.NewPanelBuilder().
		Id(nodePanelFilesystemUsage).
		Title("Filesystem Usage").
		Description("Every filesystem in scope, fullest first. This is where the \"Filesystems Over 85%\" count in the Summary resolves to a name.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Orientation(common.VizOrientationHorizontal).
			Thresholds(capacityThresholds()).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc((1 - node_filesystem_avail_bytes{`+instFilter+`,`+fsFilter+`} / node_filesystem_size_bytes{`+instFilter+`,`+fsFilter+`}) * 100 `+joinNodename+`)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{mountpoint}}")))))

	d.Panel(elementLoadAverage1mPerCPU, dashboardv2.NewPanelBuilder().
		Id(nodePanelLoadAverage1mPerCPU).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(node_load1{`+instFilter+`} `+normByCPU+`) `+joinNodename).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementCPUUsage, dashboardv2.NewPanelBuilder().
		Id(nodePanelCPUUsage).
		Title("CPU Usage (%)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Thresholds(capacityThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+instFilter+`}[$__rate_interval]) `+joinNodename+`) * 100)`).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementLoadAveragePerCPU1m, dashboardv2.NewPanelBuilder().
		Id(nodePanelLoadAveragePerCPU1m).
		Title("Load Average per CPU (1m)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percentunit").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(node_load1{`+instFilter+`} `+normByCPU+`) `+joinNodename).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementMemoryUsed, dashboardv2.NewPanelBuilder().
		Id(nodePanelMemoryUsed).
		Title("Memory Used").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(node_memory_MemTotal_bytes{`+instFilter+`} - node_memory_MemAvailable_bytes{`+instFilter+`}) `+joinNodename).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementMemoryUsage, dashboardv2.NewPanelBuilder().
		Id(nodePanelMemoryUsage).
		Title("Memory Usage").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Thresholds(capacityThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(1 - node_memory_MemAvailable_bytes{`+instFilter+`} / node_memory_MemTotal_bytes{`+instFilter+`}) * 100 `+joinNodename).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementCPUPressureSome, dashboardv2.NewPanelBuilder().
		Id(nodePanelCPUPressureSome).
		Title("CPU Pressure (some)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(node_pressure_cpu_waiting_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementMemoryPressure, dashboardv2.NewPanelBuilder().
		Id(nodePanelMemoryPressure).
		Title("Memory Pressure").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(node_pressure_memory_waiting_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}} some"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(node_pressure_memory_stalled_seconds_total{`+instFilter+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}} full")))))

	d.Panel(elementIOPressure, dashboardv2.NewPanelBuilder().
		Id(nodePanelIOPressure).
		Title("IO Pressure").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(node_pressure_io_waiting_seconds_total{`+ownKernel+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}} some"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(node_pressure_io_stalled_seconds_total{`+ownKernel+`}[$__rate_interval]) * 100 `+joinNodename).
				LegendFormat("{{nodename}} full")))))

	d.Panel(elementTemperature, dashboardv2.NewPanelBuilder().
		Id(nodePanelTemperature).
		Title("Temperature").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("celsius").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// CPU temp: x86 Package (Intel), cpu-thermal (RPi), or k10temp Tctl (Ryzen, PCI device 0000:00:18.x)
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`((label_replace(node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal", `+instFilter+`}, "sensor", "$1", "type", "(.*)")) or (label_replace(node_hwmon_temp_celsius{chip=~".*_0000:00:18_.*", sensor="temp1", `+instFilter+`}, "sensor", "cpu", "", ""))) `+joinNodename).
				LegendFormat("{{nodename}} CPU {{sensor}}"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(nvme_temperature_celsius{`+instFilter+`}) `+joinNodename).
				LegendFormat("{{nodename}} NVMe {{device}}"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(smartmon_temperature_celsius_raw_value{`+instFilter+`}) `+joinNodename).
				LegendFormat("{{nodename}} Disk {{device}}")))))

	d.Panel(elementCPUThrottlingRate, dashboardv2.NewPanelBuilder().
		Id(nodePanelCPUThrottlingRate).
		Title("CPU Throttling Rate").
		Description("Package thermal throttling on the machines that own their CPUs. LXC guests are excluded: the counter reaches them from the shared kernel and describes the hypervisor, not the container. Only the Intel hypervisors report it -- pve is AMD and the Raspberry Pis are ARM, and neither exposes this counter.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Restrict package throttles to hypervisors because LXC guests inherit
				// the host cpu collector series.
				Expr(`(rate(node_cpu_package_throttles_total{`+instFilter+`, instance=~"`+proxmoxHosts+`"}[$__rate_interval])) `+joinNodename).
				LegendFormat("{{nodename}} Throttles")))))

	d.Panel(elementRPiPowerThermalStatus, dashboardv2.NewPanelBuilder().
		Id(nodePanelRPiPowerThermalStatus).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(rpi_throttled_thermal_throttling{`+instFilter+`}) `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} Thermal Throttled"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(rpi_throttled_occurred{`+instFilter+`}) `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} Thermal Throttled Since Boot"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(rpi_throttled_under_voltage{`+instFilter+`}) `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} Under Voltage"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(rpi_throttled_under_voltage_occurred{`+instFilter+`}) `+joinNodename).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} Under Voltage Since Boot")))))

	d.Panel(elementNetworkIO, dashboardv2.NewPanelBuilder().
		Id(nodePanelNetworkIO).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Keep physical NICs and vmbr (Proxmox bridges); exclude per-VM/LXC virtual interfaces.
				Expr(`rate(node_network_receive_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|podman.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*|vnets.*"}[$__rate_interval]) `+joinNodename).
				LegendFormat("{{nodename}} Rx {{device}}"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Tx").Query(promQuery().
				Expr(`rate(node_network_transmit_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|podman.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*|vnets.*"}[$__rate_interval]) `+joinNodename).
				LegendFormat("{{nodename}} Tx {{device}}")))))

	d.Panel(elementDiskIO, dashboardv2.NewPanelBuilder().
		Id(nodePanelDiskIO).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(node_disk_read_bytes_total{`+instFilter+`, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[$__rate_interval]) `+joinNodename).
				LegendFormat("{{nodename}} Read {{device}}"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Write").Query(promQuery().
				Expr(`rate(node_disk_written_bytes_total{`+instFilter+`, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[$__rate_interval]) `+joinNodename).
				LegendFormat("{{nodename}} Write {{device}}")))))

	d.Panel(elementFilesystemUsageTrend, dashboardv2.NewPanelBuilder().
		Id(nodePanelFilesystemUsageTrend).
		Title("Filesystem Usage Trend").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Thresholds(capacityThresholds()).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(1 - node_filesystem_avail_bytes{`+instFilter+`,`+fsFilter+`} / node_filesystem_size_bytes{`+instFilter+`,`+fsFilter+`}) * 100 `+joinNodename).
				LegendFormat("{{nodename}} {{mountpoint}}")))))

	d.Panel(elementZFSARCSize, dashboardv2.NewPanelBuilder().
		Id(nodePanelZFSARCSize).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			// Draw configured ARC bounds dashed around the live size.
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(node_zfs_arc_size{`+instFilter+`}`+zfsActive+`) `+joinNodename).
				LegendFormat("{{nodename}} ARC Size"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(node_zfs_arc_c_max{`+instFilter+`}`+zfsActive+`) `+joinNodename).
				LegendFormat("{{nodename}} ARC Max"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`(node_zfs_arc_c_min{`+instFilter+`}`+zfsActive+`) `+joinNodename).
				LegendFormat("{{nodename}} ARC Min")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		// Summary counts actionable states instead of enumerating every node.
		// Lifecycle and pressure form balanced lines. Filesystem usage remains even
		// with a zero history because persistent growth differs from transient load.
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementNodesDown).
			WithItem(elementRebooted1h).
			WithItem(elementNodesSaturated).
			WithItem(elementNodesOver80Memory).
			WithItem(elementFilesystemsOver85))).
		// Current per-node values pair with trends in later rows. Sort capacity
		// gauges so visible bars show the most constrained resources.
		Row(dashboardv2.NewRowBuilder().Title("Current State by Node").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementNodeExporterStatus).
			WithItem(elementUptime))).
		Row(dashboardv2.NewRowBuilder().Title("CPU Usage (Current)").AutoGridLayout(chartGrid().
			WithItem(elementCPUUsageCurrent).
			WithItem(elementMemoryUsageCurrent))).
		Row(dashboardv2.NewRowBuilder().Title("Filesystem Usage").AutoGridLayout(chartGrid().
			WithItem(elementFilesystemUsage))).
		Row(dashboardv2.NewRowBuilder().Title("Load Average (1m) per CPU").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementLoadAverage1mPerCPU))).
		Row(dashboardv2.NewRowBuilder().Title("CPU Usage (%)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCPUUsage))).
		Row(dashboardv2.NewRowBuilder().Title("Load Average per CPU (1m)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementLoadAveragePerCPU1m))).
		// Used = Total - Available (buffers/cache are included in Available).
		Row(dashboardv2.NewRowBuilder().Title("Memory").AutoGridLayout(chartGrid().
			WithItem(elementMemoryUsed).
			WithItem(elementMemoryUsage))).
		// PSI `some` is partial stall time; `full` is total stall time.
		// Linux exposes no CPU `full` series.
		Row(dashboardv2.NewRowBuilder().Title("CPU Pressure (some)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCPUPressureSome))).
		Row(dashboardv2.NewRowBuilder().Title("Memory Pressure").AutoGridLayout(chartGrid().
			WithItem(elementMemoryPressure).
			WithItem(elementIOPressure))).
		Row(dashboardv2.NewRowBuilder().Title("Temperature").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementTemperature))).
		Row(dashboardv2.NewRowBuilder().Title("CPU Throttling Rate").AutoGridLayout(chartGrid().
			WithItem(elementCPUThrottlingRate))).
		Row(dashboardv2.NewRowBuilder().Title("RPi Power & Thermal Status").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementRPiPowerThermalStatus))).
		// Exclude dm-*, loop*, and sr* to avoid double-counting or noise from virtual/optical devices.
		Row(dashboardv2.NewRowBuilder().Title("Network").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementNetworkIO))).
		Row(dashboardv2.NewRowBuilder().Title("Disk I/O").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDiskIO))).
		Row(dashboardv2.NewRowBuilder().Title("Filesystem Usage Trend").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementFilesystemUsageTrend))).
		Row(dashboardv2.NewRowBuilder().Title("ZFS").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementZFSARCSize)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
