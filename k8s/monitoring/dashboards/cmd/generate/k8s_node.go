package main

import (
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	k8sNodePanelCPUUsageCurrent         = 1
	k8sNodePanelMemoryUsageCurrent      = 2
	k8sNodePanelPodsRunning             = 3
	k8sNodePanelCPUCores                = 4
	k8sNodePanelMemoryTotal             = 5
	k8sNodePanelUptime                  = 6
	k8sNodePanelCPUUsage                = 7
	k8sNodePanelLoadAveragePerCPU       = 8
	k8sNodePanelMemoryUsage             = 9
	k8sNodePanelCPUPressureSome         = 10
	k8sNodePanelMemoryPressure          = 11
	k8sNodePanelIOPressure              = 12
	k8sNodePanelFilesystemUsage         = 13
	k8sNodePanelDiskSpaceUsed           = 14
	k8sNodePanelTimeUntilFilesystemFull = 15
	k8sNodePanelDiskIO                  = 16
	k8sNodePanelNetworkIO               = 17
)

func buildK8sNodeOverview() (*dashboardv2.Dashboard, error) {
	const (
		elementCPUUsageCurrent         = "cpu-usage-current"
		elementMemoryUsageCurrent      = "memory-usage-current"
		elementPodsRunning             = "pods-running"
		elementCPUCores                = "cpu-cores"
		elementMemoryTotal             = "memory-total"
		elementUptime                  = "uptime"
		elementCPUUsage                = "cpu-usage"
		elementLoadAveragePerCPU       = "load-average-per-cpu"
		elementMemoryUsage             = "memory-usage"
		elementCPUPressureSome         = "cpu-pressure-some"
		elementMemoryPressure          = "memory-pressure"
		elementIOPressure              = "io-pressure"
		elementFilesystemUsage         = "filesystem-usage"
		elementDiskSpaceUsed           = "disk-space-used"
		elementTimeUntilFilesystemFull = "time-until-filesystem-full"
		elementDiskIO                  = "disk-i-o"
		elementNetworkIO               = "network-i-o"
	)

	const (
		clusterFilter = `cluster=~"$cluster"`
		nodeFilter    = `nodename=~"$node"`
		// Match node-overview filesystem exclusions.
		fsFilter = `fstype=~"ext[234]|xfs|btrfs|zfs|vfat",mountpoint!~"/var/lib/docker/.*|/boot/efi|/boot/firmware"`
		// Exclude CNI and pod virtual devices so traffic is counted once at the
		// physical boundary. Cilium's lxc* veth names bypassed older filters.
		// Use exclusions so unexpected physical device names remain visible.
		netDeviceFilter = `device!~"lo|veth.*|docker.*|br-.*|lxc.*|cilium_.*"`
	)

	// Copy nodename onto results and deduplicate stale kernel-release series
	// before joining.
	joinNode := `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{` + clusterFilter + `, ` + nodeFilter + `})`

	// Normalize load by logical CPU count; node sizes differ and 1.0 always
	// means saturation.
	perCore := func(expr string) string {
		return expr + ` / on(instance) group_left() count by (instance) (node_cpu_seconds_total{mode="idle", ` + clusterFilter + `})`
	}

	// Drop the exporter's own identity.
	byNode := func(expr string, extraKeys ...string) string {
		keys := strings.Join(append([]string{"nodename"}, extraKeys...), ", ")
		return `max by (` + keys + `) (` + expr + `)`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// Draw saturation at 1.0 without pinning the axis; idle values need autoscaling.
	saturationThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "transparent"},
			{Value: new(float64(1)), Color: "red"},
		})

	d := dashboardv2.NewDashboardBuilder("K8s Node Overview").
		Tags([]string{"kubernetes", "nodes", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-1d").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			// Use node_uname_info as it is present on both k8s and external nodes.
			Query(promVariableQuery(`label_values(node_uname_info, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true)).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("node").
			Label("Node").
			// nodename is consistent across node_exporter metrics.
			Query(promVariableQuery(`label_values(node_uname_info{` + clusterFilter + `}, nodename)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	d.Panel(elementCPUUsageCurrent, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelCPUUsageCurrent).
		Title("CPU Usage (Current)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Orientation(common.VizOrientationAuto).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+clusterFilter+`}[$__rate_interval]) `+joinNode+`) * 100)`).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementMemoryUsageCurrent, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelMemoryUsageCurrent).
		Title("Memory Usage (Current)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Orientation(common.VizOrientationAuto).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
				Expr(byNode(`(1 - node_memory_MemAvailable_bytes{`+clusterFilter+`} / node_memory_MemTotal_bytes{`+clusterFilter+`}) `+joinNode+` * 100`)).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementPodsRunning, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelPodsRunning).
		Title("Pods Running").
		Description("Worker nodes only -- k0s controllers run no kubelet unless started with --enable-worker, so a control-plane node is absent here, not zero.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Deduplicate node_uname_info before joining kubelet node labels.
				Expr(`sum by (nodename) (kubelet_running_pods{`+clusterFilter+`} * on(node) group_left(nodename) (label_replace(max by (nodename) (node_uname_info{`+clusterFilter+`, `+nodeFilter+`}), "node", "$1", "nodename", "(.*)")))`).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementCPUCores, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelCPUCores).
		Title("CPU Cores").
		Description("Logical cores per node. Inventory, not a measurement -- it changes only when a VM is resized.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// Core count has no meaningful maximum, so use a stat.
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count by (nodename) (node_cpu_seconds_total{mode="idle", `+clusterFilter+`} `+joinNode+`)`).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementMemoryTotal, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelMemoryTotal).
		Title("Memory Total").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("bytes").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`node_memory_MemTotal_bytes{`+clusterFilter+`} `+joinNode)).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementUptime, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelUptime).
		Title("Uptime").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			GraphMode(common.BigValueGraphModeNone).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			JustifyMode(common.BigValueJustifyModeCenter).
			Thresholds(dashboardv2.NewThresholdsConfigBuilder().
				Mode(dashboardv2.ThresholdsModeAbsolute).
				Steps([]dashboardv2.Threshold{
					{Value: nil, Color: "red"},
					{Value: new(float64(3600)), Color: "yellow"},
					{Value: new(float64(86400)), Color: "green"},
				}))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`(time() - node_boot_time_seconds{`+clusterFilter+`}) `+joinNode)).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementCPUUsage, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelCPUUsage).
		Title("CPU Usage (%)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+clusterFilter+`}[$__rate_interval]) `+joinNode+`) * 100)`).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementLoadAveragePerCPU, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelLoadAveragePerCPU).
		Title("Load Average per CPU").
		Description("Run-queue length divided by the node's CPU count -- the fraction of CPUs with work waiting, so 100% is saturated regardless of node size (red line). Raw load averages are not comparable across this cluster: k0s-cp1 has 2 CPUs against 10 on the workers, so the same load1 of 0.08 is 13.5%/CPU on cp1 vs 0.9% on worker1.").
		Visualization(timeseries.NewVisualizationV2Builder().
			// Match node-overview's normalized percentage display.
			Unit("percentunit").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(saturationThresholds).
			ThresholdsStyle(common.NewGraphThresholdsStyleConfigBuilder().
				Mode(common.GraphThresholdsStyleModeLine)).
			// 5m/15m as dashed to distinguish from 1m (solid) without adding noise.
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* 5m$"}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* 15m$"}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(perCore(`node_load1{`+clusterFilter+`}`)+` `+joinNode)).
				LegendFormat("{{nodename}} 1m"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(perCore(`node_load5{`+clusterFilter+`}`)+` `+joinNode)).
				LegendFormat("{{nodename}} 5m"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(perCore(`node_load15{`+clusterFilter+`}`)+` `+joinNode)).
				LegendFormat("{{nodename}} 15m")))))

	d.Panel(elementMemoryUsage, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelMemoryUsage).
		Title("Memory Usage").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			// Draw available memory dotted against solid used memory.
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Available$"}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`(node_memory_MemTotal_bytes{`+clusterFilter+`} - node_memory_MemAvailable_bytes{`+clusterFilter+`}) `+joinNode)).
				LegendFormat("{{nodename}} Used"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`node_memory_MemAvailable_bytes{`+clusterFilter+`} `+joinNode)).
				LegendFormat("{{nodename}} Available")))))

	d.Panel(elementCPUPressureSome, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelCPUPressureSome).
		Title("CPU Pressure (some)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`rate(node_pressure_cpu_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}}")))))

	d.Panel(elementMemoryPressure, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelMemoryPressure).
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
				Expr(byNode(`rate(node_pressure_memory_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}} some"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`rate(node_pressure_memory_stalled_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}} full")))))

	d.Panel(elementIOPressure, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelIOPressure).
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
				Expr(byNode(`rate(node_pressure_io_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}} some"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`rate(node_pressure_io_stalled_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}} full")))))

	d.Panel(elementFilesystemUsage, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelFilesystemUsage).
		Title("Filesystem Usage (%)").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Orientation(common.VizOrientationHorizontal).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc((1 - node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`} / node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`}) `+joinNode+` * 100)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{mountpoint}}")))))

	d.Panel(elementDiskSpaceUsed, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelDiskSpaceUsed).
		Title("Disk Space Used").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Total$"}, []dashboardv2.DynamicConfigValue{
				{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`(node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`} - node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`}) `+joinNode, "mountpoint")).
				LegendFormat("{{nodename}} {{mountpoint}} Used"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byNode(`node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`} `+joinNode, "mountpoint")).
				LegendFormat("{{nodename}} {{mountpoint}} Total")))))

	d.Panel(elementTimeUntilFilesystemFull, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelTimeUntilFilesystemFull).
		Title("Time Until Filesystem Full").
		// deriv is negative while filling, so /-86400 gives positive days;
		// the clamp stops idle filesystems flattening the bar scale.
		Description("Days until free space reaches zero, projected from the last 7 days. Capped at 3650 d; filesystems that are not filling are omitted.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("d").
			Min(0).
			Decimals(0).
			Thresholds(timeToFullThresholds()).
			Orientation(common.VizOrientationHorizontal)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort(clamp_max((node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`} / deriv(node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`}[7d]) / -86400) > 0, 3650) `+joinNode+`)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{mountpoint}}")))))

	d.Panel(elementDiskIO, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelDiskIO).
		Title("Disk I/O").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("Bps").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(zeroLineThresholds).
			ThresholdsStyle(zeroLineStyle).
			OverrideByQuery("Write", []dashboardv2.DynamicConfigValue{
				{Id: "custom.transform", Value: "negative-Y"},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Exclude dm-*, loop*, and sr* to avoid double-counting or noise from virtual/optical devices.
				Expr(byNode(`rate(node_disk_read_bytes_total{`+clusterFilter+`, device!~"dm-.*|loop.*|sr.*"}[$__rate_interval]) `+joinNode, "device")).
				LegendFormat("{{nodename}} {{device}} Read"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Write").Query(promQuery().
				Expr(byNode(`rate(node_disk_written_bytes_total{`+clusterFilter+`, device!~"dm-.*|loop.*|sr.*"}[$__rate_interval]) `+joinNode, "device")).
				LegendFormat("{{nodename}} {{device}} Write")))))

	d.Panel(elementNetworkIO, dashboardv2.NewPanelBuilder().
		Id(k8sNodePanelNetworkIO).
		Title("Network I/O").
		Description("Traffic across each node's physical interfaces. The CNI's per-pod veths and Cilium's own devices are excluded, so pod traffic is counted once, where it leaves the node, rather than again on every virtual hop.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("Bps").
			Tooltip(tooltipAll).
			Legend(legend).
			Thresholds(zeroLineThresholds).
			ThresholdsStyle(zeroLineStyle).
			OverrideByQuery("Tx", []dashboardv2.DynamicConfigValue{
				{Id: "custom.transform", Value: "negative-Y"},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (nodename) (rate(node_network_receive_bytes_total{`+clusterFilter+`, `+netDeviceFilter+`} [$__rate_interval]) `+joinNode+`)`).
				LegendFormat("{{nodename}} Rx"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Tx").Query(promQuery().
				Expr(`sum by (nodename) (rate(node_network_transmit_bytes_total{`+clusterFilter+`, `+netDeviceFilter+`}[$__rate_interval]) `+joinNode+`)`).
				LegendFormat("{{nodename}} Tx")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(chartGrid().
			WithItem(elementCPUUsageCurrent).
			WithItem(elementMemoryUsageCurrent))).
		Row(dashboardv2.NewRowBuilder().Title("Node Capacity").AutoGridLayout(chartGrid().
			WithItem(elementPodsRunning).
			WithItem(elementCPUCores).
			WithItem(elementMemoryTotal).
			WithItem(elementUptime))).
		Row(dashboardv2.NewRowBuilder().Title("CPU Usage (%)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCPUUsage))).
		Row(dashboardv2.NewRowBuilder().Title("Load Average per CPU").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementLoadAveragePerCPU))).
		Row(dashboardv2.NewRowBuilder().Title("Memory").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementMemoryUsage))).
		// PSI `some` is partial stall time; `full` is total stall time.
		// Linux exposes no CPU `full` series.
		Row(dashboardv2.NewRowBuilder().Title("CPU Pressure (some)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCPUPressureSome))).
		Row(dashboardv2.NewRowBuilder().Title("Memory Pressure").AutoGridLayout(chartGrid().
			WithItem(elementMemoryPressure).
			WithItem(elementIOPressure))).
		Row(dashboardv2.NewRowBuilder().Title("Disk").AutoGridLayout(chartGrid().
			WithItem(elementFilesystemUsage).
			WithItem(elementDiskSpaceUsed))).
		Row(dashboardv2.NewRowBuilder().Title("Time Until Filesystem Full").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementTimeUntilFilesystemFull))).
		Row(dashboardv2.NewRowBuilder().Title("Disk I/O").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDiskIO))).
		Row(dashboardv2.NewRowBuilder().Title("Network").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementNetworkIO)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
