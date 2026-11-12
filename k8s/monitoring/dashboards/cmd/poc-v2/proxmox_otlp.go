package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	proxmoxOtlpPanelRunningVMs              = 1
	proxmoxOtlpPanelRunningLXCs             = 2
	proxmoxOtlpPanelNodeCPUUsageCurrent     = 3
	proxmoxOtlpPanelNodeMemoryUsageCurrent  = 4
	proxmoxOtlpPanelStorageUsageCurrent     = 5
	proxmoxOtlpPanelNodeOSDiskUsage         = 6
	proxmoxOtlpPanelNodeCPUUsage            = 7
	proxmoxOtlpPanelNodeMemoryUsage         = 8
	proxmoxOtlpPanelNodeSwapUsage           = 9
	proxmoxOtlpPanelNodeNetworkIO           = 10
	proxmoxOtlpPanelNodeTemperature         = 11
	proxmoxOtlpPanelStorageUsage            = 12
	proxmoxOtlpPanelNodeLoadAveragePerCPU   = 13
	proxmoxOtlpPanelNodeZFSARCSize          = 14
	proxmoxOtlpPanelGuestUptime             = 15
	proxmoxOtlpPanelGuestCPUUsage           = 16
	proxmoxOtlpPanelGuestMemoryUsage        = 17
	proxmoxOtlpPanelGuestDiskIO             = 18
	proxmoxOtlpPanelGuestNetworkIO          = 19
	proxmoxOtlpPanelGuestCPUPressurePSISome = 20
	proxmoxOtlpPanelGuestIOPressurePSISome  = 21
)

// buildProxmoxOtlpOverview covers PVE 9 native OTLP node, guest, and storage
// metrics. OTLP emits running guests only and already carries usable labels.
func buildProxmoxOtlpOverview() (*dashboardv2.Dashboard, error) {
	const (
		job        = `job="proxmox-ve"`
		nodeFilter = `job="proxmox-ve", node=~"$node"`
		// Show ZFS only for nodes with an imported pool. Module-only hosts expose
		// tiny empty ARCs; metric-based gating lets new pools appear automatically.
		zfsActive = ` and (proxmox_node_memory_arcsize_bytes{` + nodeFilter + `} > 1024 * 1024)`
	)

	tooltipAll := defaultTooltip()
	tooltipSingle := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeSingle)
	legend := defaultLegend()

	// zeroLine draws a solid reference line at y=0 for bidirectional I/O panels.
	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	cpuThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(0.8)), Color: "yellow"},
			{Value: new(float64(0.9)), Color: "red"},
		})

	pctThresholds := capacityThresholds()

	d := dashboardv2.NewDashboardBuilder("Proxmox Overview (OTLP)").
		Tags([]string{"proxmox", "infrastructure", "otlp"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-1d").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("node").
			Label("Node").
			Query(promVariableQuery(`label_values(proxmox_node_cpustat_cpu_percent{` + job + `}, node)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	return newLayout(d).
		// Summary combines running guest counts and node resources.
		Row("Running Guests", statGrid(2)).
		Panel(proxmoxOtlpPanelRunningVMs, dashboardv2.NewPanelBuilder().
			Title("Running VMs").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count by (node) (proxmox_vm_cpu_percent{`+nodeFilter+`, type="qemu"}) or on(node) count by (node) (proxmox_node_cpustat_cpu_percent{`+nodeFilter+`}) * 0`).
				Instant(true).Range(false).
				LegendFormat("{{node}}")))).
		Panel(proxmoxOtlpPanelRunningLXCs, dashboardv2.NewPanelBuilder().
			Title("Running LXCs").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count by (node) (proxmox_vm_cpu_percent{`+nodeFilter+`, type="lxc"}) or on(node) count by (node) (proxmox_node_cpustat_cpu_percent{`+nodeFilter+`}) * 0`).
				Instant(true).Range(false).
				LegendFormat("{{node}}")))).
		Row("Node Resources", statGrid(3)).
		Panel(proxmoxOtlpPanelNodeCPUUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("Node CPU Usage (Current)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Max(1).
				Decimals(1).
				Thresholds(cpuThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`proxmox_node_cpustat_cpu_percent{`+nodeFilter+`}`).
				Instant(true).Range(false).
				LegendFormat("{{node}}")))).
		Panel(proxmoxOtlpPanelNodeMemoryUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("Node Memory Usage (Current)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`proxmox_node_memory_memused_bytes{`+nodeFilter+`} / proxmox_node_memory_memtotal_bytes{`+nodeFilter+`} * 100`).
				Instant(true).Range(false).
				LegendFormat("{{node}}")))).
		Panel(proxmoxOtlpPanelNodeOSDiskUsage, dashboardv2.NewPanelBuilder().
			// proxmox_node_blockstat_per_percent reflects the PVE OS root filesystem usage.
			// No mount-point label is exposed; one value is emitted per node.
			Title("Node OS Disk Usage").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`proxmox_node_blockstat_per_percent{`+nodeFilter+`}`).
				Instant(true).Range(false).
				LegendFormat("{{node}}")))).
		// Node: per-host resource trends
		Row("Storage", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(proxmoxOtlpPanelStorageUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("Storage Usage (Current)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`proxmox_storage_used_bytes{`+nodeFilter+`} / proxmox_storage_total_bytes{`+nodeFilter+`} * 100`).
				Instant(true).Range(false).
				LegendFormat("{{node}}/{{storage}}")))).
		Row("Node CPU & Memory", chartGrid()).
		Panel(proxmoxOtlpPanelNodeCPUUsage, dashboardv2.NewPanelBuilder().
			Title("Node CPU Usage (%)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Max(1).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`proxmox_node_cpustat_cpu_percent{`+nodeFilter+`}`).
				LegendFormat("{{node}}")))).
		Panel(proxmoxOtlpPanelNodeMemoryUsage, dashboardv2.NewPanelBuilder().
			Title("Node Memory Usage").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				OverrideByQuery("Total", []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				})).
			Data(queries(queryWithRef("Used", promQuery().
				RefId("Used").
				Expr(`proxmox_node_memory_memused_bytes{`+nodeFilter+`}`).
				LegendFormat("{{node}} Used")),
				queryWithRef("Total", promQuery().
					RefId("Total").
					Expr(`proxmox_node_memory_memtotal_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}} Total"))))).
		Row("Node Swap & Network", chartGrid()).
		Panel(proxmoxOtlpPanelNodeSwapUsage, dashboardv2.NewPanelBuilder().
			Title("Node Swap Usage").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				OverrideByQuery("Total", []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				})).
			Data(queries(queryWithRef("Used", promQuery().
				RefId("Used").
				Expr(`proxmox_node_memory_swapused_bytes{`+nodeFilter+`}`).
				LegendFormat("{{node}} Used")),
				queryWithRef("Total", promQuery().
					RefId("Total").
					Expr(`proxmox_node_memory_swaptotal_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}} Total"))))).
		Panel(proxmoxOtlpPanelNodeNetworkIO, dashboardv2.NewPanelBuilder().
			Title("Node Network I/O").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				OverrideByQuery("Tx", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(queryWithRef("Rx", promQuery().
				RefId("Rx").
				// Exclude loopback, guest, firewall, and unused nic0 devices. Keep nic1,
				// bridges, and SDN vnets; parallel NIC/bridge lines show traffic layers.
				Expr(`rate(proxmox_node_network_receive_bytes_total{`+nodeFilter+`, device!~"lo|tap.*|fwbr.*|fwpr.*|fwln.*|veth.*|nic0"}[$__rate_interval])`).
				LegendFormat("{{node}} {{device}} RX")),
				queryWithRef("Tx", promQuery().
					RefId("Tx").
					Expr(`rate(proxmox_node_network_transmit_bytes_total{`+nodeFilter+`, device!~"lo|tap.*|fwbr.*|fwpr.*|fwln.*|veth.*|nic0"}[$__rate_interval])`).
					LegendFormat("{{node}} {{device}} TX"))))).
		Row("Node Temperature & Storage", chartGrid()).
		Panel(proxmoxOtlpPanelNodeTemperature, dashboardv2.NewPanelBuilder().
			Title("Node Temperature").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("celsius").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Map OTLP node labels to nodenames before joining temperatures.
				Expr(`node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal"} * on(instance) group_left(nodename)
  (node_uname_info * on(nodename) group_left()
    label_replace(target_info{`+job+`, proxmox_node=~"$node"}, "nodename", "$1", "proxmox_node", "(.*)"))`).
				LegendFormat("{{nodename}} CPU {{type}}"),
				promQuery().
					// Ryzen: k10temp exposed as PCI device (0000:00:18.x), temp1=Tctl
					Expr(`node_hwmon_temp_celsius{chip=~".*_0000:00:18_.*", sensor="temp1"} * on(instance) group_left(nodename)
  (node_uname_info * on(nodename) group_left()
    label_replace(target_info{`+job+`, proxmox_node=~"$node"}, "nodename", "$1", "proxmox_node", "(.*)"))`).
					LegendFormat("{{nodename}} CPU Tctl"),
				promQuery().
					Expr(`smartmon_temperature_celsius_raw_value * on(instance) group_left(nodename)
  (node_uname_info * on(nodename) group_left()
    label_replace(target_info{`+job+`, proxmox_node=~"$node"}, "nodename", "$1", "proxmox_node", "(.*)"))`).
					LegendFormat("{{nodename}} Disk {{disk}}")))).
		Panel(proxmoxOtlpPanelStorageUsage, dashboardv2.NewPanelBuilder().
			Title("Storage Usage").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				OverrideByQuery("Total", []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				})).
			Data(queries(queryWithRef("Used", promQuery().
				RefId("Used").
				Expr(`proxmox_storage_used_bytes{`+nodeFilter+`}`).
				LegendFormat("{{node}}/{{storage}} Used")),
				queryWithRef("Total", promQuery().
					RefId("Total").
					Expr(`proxmox_storage_total_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}}/{{storage}} Total"))))).
		Row("Node Load & ARC", chartGrid()).
		Panel(proxmoxOtlpPanelNodeLoadAveragePerCPU, dashboardv2.NewPanelBuilder().
			Title("Node Load Average per CPU").
			Description("Run-queue length divided by the node's CPU count, so 100% is a saturated node whatever its size. The hosts here run from 6 to 16 CPUs, which is why the raw averages are not comparable between them.").
			Visualization(timeseries.NewVisualizationV2Builder().
				// Match normalized load percentage across node dashboards.
				Unit("percentunit").
				Min(0).
				Tooltip(tooltipSingle).
				Legend(legend).
				OverrideByQuery("Avg5", []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}).
				OverrideByQuery("Avg15", []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				})).
			Data(queries(queryWithRef("Avg1", promQuery().
				RefId("Avg1").
				Expr(`proxmox_node_cpustat_avg1_ratio{`+nodeFilter+`} / proxmox_node_cpustat_cpus_ratio{`+nodeFilter+`}`).
				LegendFormat("{{node}} 1m")),
				queryWithRef("Avg5", promQuery().
					RefId("Avg5").
					Expr(`proxmox_node_cpustat_avg5_ratio{`+nodeFilter+`} / proxmox_node_cpustat_cpus_ratio{`+nodeFilter+`}`).
					LegendFormat("{{node}} 5m")),
				queryWithRef("Avg15", promQuery().
					RefId("Avg15").
					Expr(`proxmox_node_cpustat_avg15_ratio{`+nodeFilter+`} / proxmox_node_cpustat_cpus_ratio{`+nodeFilter+`}`).
					LegendFormat("{{node}} 15m"))))).
		Panel(proxmoxOtlpPanelNodeZFSARCSize, dashboardv2.NewPanelBuilder().
			Title("Node ZFS ARC Size").
			Description("Only nodes with a pool imported appear; the others load the ZFS module but hold an empty ARC of about a kilobyte, which on a bytes axis beside a multi-gigabyte ARC is indistinguishable from zero while still occupying the legend. A node that gains a pool shows up here on its own.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`proxmox_node_memory_arcsize_bytes{`+nodeFilter+`}`+zfsActive).
				LegendFormat("{{node}} ARC")))).
		// Guest: per-VM/LXC resource trends
		Row("Guest", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(proxmoxOtlpPanelGuestUptime, dashboardv2.NewPanelBuilder().
			Title("Guest Uptime").
			Visualization(stat.NewVisualizationV2Builder().
				GraphMode(common.BigValueGraphModeNone).
				Unit("s").
				Min(0).
				Decimals(0).
				// Show 0 as red to highlight recently restarted guests.
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(300)), Color: "yellow"},
						{Value: new(float64(3600)), Color: "green"},
					}))).
			Data(queries(promQuery().
				Expr(`proxmox_vm_uptime_seconds{`+nodeFilter+`}`).
				Instant(true).Range(false).
				LegendFormat("{{name}}")))).
		Row("Guest CPU & Memory", chartGrid()).
		Panel(proxmoxOtlpPanelGuestCPUUsage, dashboardv2.NewPanelBuilder().
			Title("Guest CPU Usage (%)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Max(1).
				Tooltip(tooltipSingle).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`proxmox_vm_cpu_percent{`+nodeFilter+`}`).
				LegendFormat("{{name}} ({{type}})")))).
		Panel(proxmoxOtlpPanelGuestMemoryUsage, dashboardv2.NewPanelBuilder().
			Title("Guest Memory Usage").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipSingle).
				Legend(legend).
				OverrideByQuery("Total", []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				})).
			Data(queries(queryWithRef("Used", promQuery().
				RefId("Used").
				Expr(`proxmox_vm_mem_bytes{`+nodeFilter+`}`).
				LegendFormat("{{name}} Used")),
				queryWithRef("Total", promQuery().
					RefId("Total").
					Expr(`proxmox_vm_maxmem_bytes{`+nodeFilter+`}`).
					LegendFormat("{{name}} Total"))))).
		Row("Guest Disk & Network", chartGrid()).
		Panel(proxmoxOtlpPanelGuestDiskIO, dashboardv2.NewPanelBuilder().
			Title("Guest Disk I/O").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Tooltip(tooltipSingle).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				OverrideByQuery("Write", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(queryWithRef("Read", promQuery().
				RefId("Read").
				Expr(`rate(proxmox_vm_diskread_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
				LegendFormat("{{name}} Read")),
				queryWithRef("Write", promQuery().
					RefId("Write").
					Expr(`rate(proxmox_vm_diskwrite_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
					LegendFormat("{{name}} Write"))))).
		Panel(proxmoxOtlpPanelGuestNetworkIO, dashboardv2.NewPanelBuilder().
			Title("Guest Network I/O").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("Bps").
				Tooltip(tooltipSingle).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				OverrideByQuery("Tx", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(queryWithRef("Rx", promQuery().
				RefId("Rx").
				Expr(`rate(proxmox_vm_netin_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
				LegendFormat("{{name}} RX")),
				queryWithRef("Tx", promQuery().
					RefId("Tx").
					Expr(`rate(proxmox_vm_netout_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
					LegendFormat("{{name}} TX"))))).
		Row("Guest Pressure", chartGrid()).
		Panel(proxmoxOtlpPanelGuestCPUPressurePSISome, dashboardv2.NewPanelBuilder().
			Title("Guest CPU Pressure (PSI some)").
			Description("Percentage of time at least one task was stalled waiting for CPU. The axis grows to 100 only if a reading approaches it; on this fleet the busiest moment in 30 days was 2%, and a fixed 0-100 axis flattened all of that into the bottom pixel.").
			Visualization(timeseries.NewVisualizationV2Builder().
				// pressurecpusome_percent: % of time at least one task was stalled on CPU.
				Unit("percent").
				Min(0).
				// CPU pressure is normally low, so use a soft 100% ceiling; I/O pressure
				// can approach 100% and uses a hard bound.
				AxisSoftMax(100).
				Tooltip(tooltipSingle).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`proxmox_vm_pressurecpusome_percent{`+nodeFilter+`}`).
				LegendFormat("{{name}} ({{type}})")))).
		Panel(proxmoxOtlpPanelGuestIOPressurePSISome, dashboardv2.NewPanelBuilder().
			Title("Guest I/O Pressure (PSI some)").
			Visualization(timeseries.NewVisualizationV2Builder().
				// Despite its metric suffix, pressureiosome_ratio is emitted on a 0-100 percent scale.
				Unit("percent").
				Min(0).
				Max(100).
				Tooltip(tooltipSingle).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`proxmox_vm_pressureiosome_ratio{`+nodeFilter+`}`).
				LegendFormat("{{name}} ({{type}})")))).
		Build()
}
