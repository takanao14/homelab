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

	return newLayout(d).
		Row("Summary", chartGrid()).
		Panel(k8sNodePanelCPUUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("CPU Usage (Current)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationAuto).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+clusterFilter+`}[$__rate_interval]) `+joinNode+`) * 100)`).
				LegendFormat("{{nodename}}")))).
		Panel(k8sNodePanelMemoryUsageCurrent, dashboardv2.NewPanelBuilder().
			Title("Memory Usage (Current)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationAuto).
				Decimals(1)).
			Data(queries(promQuery().
				// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
				Expr(byNode(`(1 - node_memory_MemAvailable_bytes{`+clusterFilter+`} / node_memory_MemTotal_bytes{`+clusterFilter+`}) `+joinNode+` * 100`)).
				LegendFormat("{{nodename}}")))).
		Row("Node Capacity", chartGrid()).
		Panel(k8sNodePanelPodsRunning, dashboardv2.NewPanelBuilder().
			Title("Pods Running").
			Description("Worker nodes only -- k0s controllers run no kubelet unless started with --enable-worker, so a control-plane node is absent here, not zero.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				// Deduplicate node_uname_info before joining kubelet node labels.
				Expr(`sum by (nodename) (kubelet_running_pods{`+clusterFilter+`} * on(node) group_left(nodename) (label_replace(max by (nodename) (node_uname_info{`+clusterFilter+`, `+nodeFilter+`}), "node", "$1", "nodename", "(.*)")))`).
				LegendFormat("{{nodename}}")))).
		Panel(k8sNodePanelCPUCores, dashboardv2.NewPanelBuilder().
			Title("CPU Cores").
			Description("Logical cores per node. Inventory, not a measurement -- it changes only when a VM is resized.").
			Visualization(stat.NewVisualizationV2Builder().
					Unit("short").
					Min(0).
					Thresholds(measurementThresholds()).
					Orientation(common.VizOrientationAuto).
					JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries( // Core count has no meaningful maximum, so use a stat.
				promQuery().
					Expr(`count by (nodename) (node_cpu_seconds_total{mode="idle", `+clusterFilter+`} `+joinNode+`)`).
					LegendFormat("{{nodename}}")))).
		Panel(k8sNodePanelMemoryTotal, dashboardv2.NewPanelBuilder().
			Title("Memory Total").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter)).
			Data(queries(promQuery().
				Expr(byNode(`node_memory_MemTotal_bytes{`+clusterFilter+`} `+joinNode)).
				LegendFormat("{{nodename}}")))).
		Panel(k8sNodePanelUptime, dashboardv2.NewPanelBuilder().
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
			Data(queries(promQuery().
				Expr(byNode(`(time() - node_boot_time_seconds{`+clusterFilter+`}) `+joinNode)).
				LegendFormat("{{nodename}}")))).
		Row("CPU Usage (%)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelCPUUsage, dashboardv2.NewPanelBuilder().
			Title("CPU Usage (%)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", `+clusterFilter+`}[$__rate_interval]) `+joinNode+`) * 100)`).
				LegendFormat("{{nodename}}")))).
		Row("Load Average per CPU", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelLoadAveragePerCPU, dashboardv2.NewPanelBuilder().
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
			Data(queries(promQuery().
				Expr(byNode(perCore(`node_load1{`+clusterFilter+`}`)+` `+joinNode)).
				LegendFormat("{{nodename}} 1m"),
				promQuery().
					Expr(byNode(perCore(`node_load5{`+clusterFilter+`}`)+` `+joinNode)).
					LegendFormat("{{nodename}} 5m"),
				promQuery().
					Expr(byNode(perCore(`node_load15{`+clusterFilter+`}`)+` `+joinNode)).
					LegendFormat("{{nodename}} 15m")))).
		Row("Memory", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelMemoryUsage, dashboardv2.NewPanelBuilder().
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
			Data(queries(promQuery().
				Expr(byNode(`(node_memory_MemTotal_bytes{`+clusterFilter+`} - node_memory_MemAvailable_bytes{`+clusterFilter+`}) `+joinNode)).
				LegendFormat("{{nodename}} Used"),
				promQuery().
					Expr(byNode(`node_memory_MemAvailable_bytes{`+clusterFilter+`} `+joinNode)).
					LegendFormat("{{nodename}} Available")))).
		// PSI `some` is partial stall time; `full` is total stall time.
		// Linux exposes no CPU `full` series.
		Row("CPU Pressure (some)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelCPUPressureSome, dashboardv2.NewPanelBuilder().
			Title("CPU Pressure (some)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byNode(`rate(node_pressure_cpu_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}}")))).
		Row("Memory Pressure", chartGrid()).
		Panel(k8sNodePanelMemoryPressure, dashboardv2.NewPanelBuilder().
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
				Expr(byNode(`rate(node_pressure_memory_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}} some")),
				queryWithRef("Full", promQuery().
					RefId("Full").
					Expr(byNode(`rate(node_pressure_memory_stalled_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
					LegendFormat("{{nodename}} full"))))).
		Panel(k8sNodePanelIOPressure, dashboardv2.NewPanelBuilder().
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
				Expr(byNode(`rate(node_pressure_io_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
				LegendFormat("{{nodename}} some")),
				queryWithRef("Full", promQuery().
					RefId("Full").
					Expr(byNode(`rate(node_pressure_io_stalled_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode)).
					LegendFormat("{{nodename}} full"))))).
		Row("Disk", chartGrid()).
		Panel(k8sNodePanelFilesystemUsage, dashboardv2.NewPanelBuilder().
			Title("Filesystem Usage (%)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc((1 - node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`} / node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`}) `+joinNode+` * 100)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{mountpoint}}")))).
		Panel(k8sNodePanelDiskSpaceUsed, dashboardv2.NewPanelBuilder().
			Title("Disk Space Used").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Total$"}, []dashboardv2.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				})).
			Data(queries(promQuery().
				Expr(byNode(`(node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`} - node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`}) `+joinNode, "mountpoint")).
				LegendFormat("{{nodename}} {{mountpoint}} Used"),
				promQuery().
					Expr(byNode(`node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`} `+joinNode, "mountpoint")).
					LegendFormat("{{nodename}} {{mountpoint}} Total")))).
		Row("Time Until Filesystem Full", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelTimeUntilFilesystemFull, dashboardv2.NewPanelBuilder().
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
			Data(queries(promQuery().
				Expr(`sort(clamp_max((node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`} / deriv(node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`}[7d]) / -86400) > 0, 3650) `+joinNode+`)`).
				Instant(true).Range(false).
				LegendFormat("{{nodename}} {{mountpoint}}")))).
		Row("Disk I/O", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelDiskIO, dashboardv2.NewPanelBuilder().
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
			Data(queries(queryWithRef("Read", promQuery().
				RefId("Read").
				// Exclude dm-*, loop*, and sr* to avoid double-counting or noise from virtual/optical devices.
				Expr(byNode(`rate(node_disk_read_bytes_total{`+clusterFilter+`, device!~"dm-.*|loop.*|sr.*"}[$__rate_interval]) `+joinNode, "device")).
				LegendFormat("{{nodename}} {{device}} Read")),
				queryWithRef("Write", promQuery().
					RefId("Write").
					Expr(byNode(`rate(node_disk_written_bytes_total{`+clusterFilter+`, device!~"dm-.*|loop.*|sr.*"}[$__rate_interval]) `+joinNode, "device")).
					LegendFormat("{{nodename}} {{device}} Write"))))).
		Row("Network", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sNodePanelNetworkIO, dashboardv2.NewPanelBuilder().
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
			Data(queries(queryWithRef("Rx", promQuery().
				RefId("Rx").
				Expr(`sum by (nodename) (rate(node_network_receive_bytes_total{`+clusterFilter+`, `+netDeviceFilter+`} [$__rate_interval]) `+joinNode+`)`).
				LegendFormat("{{nodename}} Rx")),
				queryWithRef("Tx", promQuery().
					RefId("Tx").
					Expr(`sum by (nodename) (rate(node_network_transmit_bytes_total{`+clusterFilter+`, `+netDeviceFilter+`}[$__rate_interval]) `+joinNode+`)`).
					LegendFormat("{{nodename}} Tx"))))).
		Build()
}
