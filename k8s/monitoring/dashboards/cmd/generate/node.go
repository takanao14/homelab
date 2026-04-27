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
	)

	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)

	d, err := dashboard.NewDashboardBuilder("Node Overview").
		Uid("node-overview").
		Tags([]string{"nodes", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		// Bare-metal nodes only: filtered by scrapeConfig job to exclude k8s/VM nodes.
		// nodename!="gpuvm" is required to filter out stale data from when gpuvm was misconfigured.
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename!="gpuvm"}, nodename)`)}).
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
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info{job="scrapeConfig/monitoring/node-exporter-external",nodename!="gpuvm",nodename=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("CPU Usage").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + instFilter + `}[5m]) ` + joinNodename + `) * 100)`).
					LegendFormat("{{nodename}}"),
				).
				Decimals(1),
		).
		// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(1 - node_memory_MemAvailable_bytes{` + instFilter + `} / node_memory_MemTotal_bytes{` + instFilter + `}) ` + joinNodename + ` * 100`).
					LegendFormat("{{nodename}}"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Load Average (1m) per CPU").
				Datasource(ds).
				Span(12).Height(4).
				Unit("percentunit").
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(0.7), Color: "yellow"},
						{Value: float64Ptr(1.0), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_load1{` + instFilter + `} ` + normByCPU + `) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(0),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				Span(12).Height(4).
				Unit("s").
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: float64Ptr(3600), Color: "yellow"},
						{Value: float64Ptr(86400), Color: "green"},
					}),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(time() - node_boot_time_seconds{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(2),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Filesystem Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationVertical).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(80), Color: "yellow"},
						{Value: float64Ptr(90), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc((1 - node_filesystem_avail_bytes{` + instFilter + `,` + fsFilter + `} / node_filesystem_size_bytes{` + instFilter + `,` + fsFilter + `}) * 100 ` + joinNodename + `)`).
					LegendFormat("{{nodename}} {{mountpoint}}"),
				).
				Decimals(1),
		).
		WithRow(dashboard.NewRowBuilder("CPU")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + instFilter + `}[5m]) ` + joinNodename + `) * 100)`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Load Average per CPU (1m)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				Tooltip(tooltipAll).
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
				Tooltip(tooltipAll).
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
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(1 - node_memory_MemAvailable_bytes{` + instFilter + `} / node_memory_MemTotal_bytes{` + instFilter + `}) * 100 ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Temperature & Throttling")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Temperature").
				Datasource(ds).
				Span(12).Height(8).
				Unit("celsius").
				Tooltip(tooltipAll).
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
				Title("CPU Throttling & Power Issues").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rate(node_cpu_package_throttles_total{` + instFilter + `}[5m])) ` + joinNodename).
					LegendFormat("{{nodename}} Throttles"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_thermal_throttling{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} RPi Thermal Throttled"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_occurred{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} RPi Thermal Throttled Occurred"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_under_voltage{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} RPi Under Voltage"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(rpi_throttled_under_voltage_occurred{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} RPi Under Voltage Occurred"),
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
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Rx").
					// Keep physical NICs and vmbr (Proxmox bridges); exclude per-VM/LXC virtual interfaces.
					Expr(`rate(node_network_receive_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*"}[5m]) `+joinNodename).
					LegendFormat("{{nodename}} Rx {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Tx").
					Expr(`rate(node_network_transmit_bytes_total{`+instFilter+`, device!~"lo|veth.*|docker.*|br-.*|fwbr.*|fwpr.*|fwln.*|tap.*|tun.*|virbr.*|cilium.*"}[5m]) `+joinNodename).
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
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_disk_read_bytes_total{` + instFilter + `, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[5m]) ` + joinNodename).
					LegendFormat("{{nodename}} Read {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_disk_written_bytes_total{` + instFilter + `, device=~"[svh]d[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+"}[5m]) ` + joinNodename).
					LegendFormat("{{nodename}} Write {{device}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Filesystem Usage").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: float64Ptr(80), Color: "yellow"},
						{Value: float64Ptr(90), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc((1 - node_filesystem_avail_bytes{` + instFilter + `,` + fsFilter + `} / node_filesystem_size_bytes{` + instFilter + `,` + fsFilter + `}) * 100) ` + joinNodename).
					LegendFormat("{{nodename}} {{mountpoint}}"),
				).
				Decimals(1),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Filesystem Usage Trend").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(1 - node_filesystem_avail_bytes{` + instFilter + `,` + fsFilter + `} / node_filesystem_size_bytes{` + instFilter + `,` + fsFilter + `}) * 100 ` + joinNodename).
					LegendFormat("{{nodename}} {{mountpoint}}"),
				),
		).
		// ZFS ARC metrics: pve (has ZFS pools) shown as solid lines; other nodes
		// that have the ZFS module loaded but no pools are shown as dashed lines.
		WithRow(dashboard.NewRowBuilder("ZFS")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("ZFS ARC Size").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("A").
					Expr(`node_zfs_arc_size{`+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{nodename="pve"})`).
					LegendFormat("{{nodename}} ARC Size"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("B").
					Expr(`node_zfs_arc_c_max{`+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{nodename="pve"})`).
					LegendFormat("{{nodename}} ARC Max"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("C").
					Expr(`node_zfs_arc_c_min{`+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{nodename="pve"})`).
					LegendFormat("{{nodename}} ARC Min"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("D").
					Expr(`node_zfs_arc_size{`+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{nodename!="pve"})`).
					LegendFormat("{{nodename}} ARC Size"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("E").
					Expr(`node_zfs_arc_c_max{`+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{nodename!="pve"})`).
					LegendFormat("{{nodename}} ARC Max"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("F").
					Expr(`node_zfs_arc_c_min{`+instFilter+`} * on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{nodename!="pve"})`).
					LegendFormat("{{nodename}} ARC Min"),
				).
				OverrideByQuery("D", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]interface{}{"fill": "dash", "dash": []int{8, 10}}},
				}).
				OverrideByQuery("E", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]interface{}{"fill": "dash", "dash": []int{8, 10}}},
				}).
				OverrideByQuery("F", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]interface{}{"fill": "dash", "dash": []int{8, 10}}},
				}),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
