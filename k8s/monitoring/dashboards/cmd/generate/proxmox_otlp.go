package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildProxmoxOtlpOverview defines the Proxmox VE cluster dashboard using native OTLP metrics
// pushed by PVE 9 via OTLP/HTTP.
//
// Label structure (no join expressions required):
//
//	proxmox_node_*  : {job="proxmox-ve", node="<hostname>"}
//	proxmox_vm_*    : {job="proxmox-ve", node="<hostname>", name="<vm>", type="qemu|lxc", vmid="<id>"}
//	proxmox_storage_*: {job="proxmox-ve", node="<hostname>", storage="<pool>"}
//
// Differences from the pve-exporter dashboard (proxmox.go):
//   - Stopped guest counts are unavailable: OTLP only emits metrics for running guests.
//     Replaced with Node Network I/O summary stats.
//   - Temperature join uses target_info{job="proxmox-ve"} + node_uname_info instead of
//     the pve_node_info instance chain.
//   - Additional panels: Node Network I/O, Guest Disk I/O, Guest Network I/O.
func buildProxmoxOtlpOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		job        = `job="proxmox-ve"`
		nodeFilter = `job="proxmox-ve", node=~"$node"`
	)

	tooltipAll := defaultTooltip()
	tooltipSingle := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeSingle)
	legend := defaultLegend()

	// zeroLine draws a solid reference line at y=0 for bidirectional I/O panels.
	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	cpuThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(0.8), Color: "yellow"},
			{Value: float64Ptr(0.9), Color: "red"},
		})

	pctThresholds := capacityThresholds()

	d, err := dashboard.NewDashboardBuilder("Proxmox Overview (OTLP)").
		Uid("proxmox-otlp-overview").
		Tags([]string{"proxmox", "infrastructure", "otlp"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(proxmox_node_cpustat_cpu_percent{` + job + `}, node)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).

		// Summary: guest counts + node resource snapshot.
		// Stopped guest counts are not available via OTLP (only running guests emit metrics).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running VMs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (node) (proxmox_vm_cpu_percent{` + nodeFilter + `, type="qemu"}) or on(node) count by (node) (proxmox_node_cpustat_cpu_percent{` + nodeFilter + `}) * 0`).
					Instant().
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running LXCs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (node) (proxmox_vm_cpu_percent{` + nodeFilter + `, type="lxc"}) or on(node) count by (node) (proxmox_node_cpustat_cpu_percent{` + nodeFilter + `}) * 0`).
					Instant().
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node CPU Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percentunit").
				Min(0).
				Max(1).
				Decimals(1).
				Thresholds(cpuThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_node_cpustat_cpu_percent{` + nodeFilter + `}`).
					Instant().
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node Memory Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Min(0).
				Max(100).
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_node_memory_memused_bytes{` + nodeFilter + `} / proxmox_node_memory_memtotal_bytes{` + nodeFilter + `} * 100`).
					Instant().
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Storage Usage").
				Datasource(ds).
				Span(12).Height(4).
				Unit("percent").
				Min(0).
				Max(100).
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_storage_used_bytes{` + nodeFilter + `} / proxmox_storage_total_bytes{` + nodeFilter + `} * 100`).
					Instant().
					LegendFormat("{{node}}/{{storage}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				// proxmox_node_blockstat_per_percent reflects the PVE OS root filesystem usage.
				// No mount-point label is exposed; one value is emitted per node.
				Title("Node OS Disk Usage").
				Datasource(ds).
				Span(12).Height(4).
				Unit("percent").
				Min(0).
				Max(100).
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_node_blockstat_per_percent{` + nodeFilter + `}`).
					Instant().
					LegendFormat("{{node}}"),
				),
		).

		// Node: per-host resource trends
		WithRow(dashboard.NewRowBuilder("Node")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				Min(0).
				Max(1).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_node_cpustat_cpu_percent{` + nodeFilter + `}`).
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Memory Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Used").
					Expr(`proxmox_node_memory_memused_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Total").
					Expr(`proxmox_node_memory_memtotal_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}} Total"),
				).
				OverrideByQuery("Total", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Swap Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Used").
					Expr(`proxmox_node_memory_swapused_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Total").
					Expr(`proxmox_node_memory_swaptotal_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}} Total"),
				).
				OverrideByQuery("Total", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Network I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Rx").
					// Exclude loopback, per-VM tap, and firewall internal interfaces.
					// Keep physical NICs (nic*), bridges (vmbr*), and SDN vnets (vnets*).
					Expr(`rate(proxmox_node_network_receive_bytes_total{`+nodeFilter+`, device!~"lo|tap.*|fwbr.*|fwpr.*|fwln.*|veth.*|nic0"}[$__rate_interval])`).
					LegendFormat("{{node}} {{device}} RX"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Tx").
					Expr(`rate(proxmox_node_network_transmit_bytes_total{`+nodeFilter+`, device!~"lo|tap.*|fwbr.*|fwpr.*|fwln.*|veth.*|nic0"}[$__rate_interval])`).
					LegendFormat("{{node}} {{device}} TX"),
				).
				OverrideByQuery("Tx", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Filter node_uname_info to PVE nodes only by joining against target_info{job="proxmox-ve"},
					// mapping proxmox_node → nodename. Then join temperature metrics on nodename.
					Expr(`node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal"} * on(instance) group_left(nodename)
  (node_uname_info * on(nodename) group_left()
    label_replace(target_info{` + job + `, proxmox_node=~"$node"}, "nodename", "$1", "proxmox_node", "(.*)"))`).
					LegendFormat("{{nodename}} CPU {{type}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Ryzen: k10temp exposed as PCI device (0000:00:18.x), temp1=Tctl
					Expr(`node_hwmon_temp_celsius{chip=~".*_0000:00:18_.*", sensor="temp1"} * on(instance) group_left(nodename)
  (node_uname_info * on(nodename) group_left()
    label_replace(target_info{` + job + `, proxmox_node=~"$node"}, "nodename", "$1", "proxmox_node", "(.*)"))`).
					LegendFormat("{{nodename}} CPU Tctl"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_temperature_celsius_raw_value * on(instance) group_left(nodename)
  (node_uname_info * on(nodename) group_left()
    label_replace(target_info{` + job + `, proxmox_node=~"$node"}, "nodename", "$1", "proxmox_node", "(.*)"))`).
					LegendFormat("{{nodename}} Disk {{disk}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Storage Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Used").
					Expr(`proxmox_storage_used_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}}/{{storage}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Total").
					Expr(`proxmox_storage_total_bytes{`+nodeFilter+`}`).
					LegendFormat("{{node}}/{{storage}} Total"),
				).
				OverrideByQuery("Total", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Load Average (normalized by CPU count)").
				Datasource(ds).
				Span(24).Height(8).
				// Values above 1.0 indicate saturation (more runnable tasks than CPUs).
				Unit("short").
				Min(0).
				Tooltip(tooltipSingle).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Avg1").
					Expr(`proxmox_node_cpustat_avg1_ratio{`+nodeFilter+`} / proxmox_node_cpustat_cpus_ratio{`+nodeFilter+`}`).
					LegendFormat("{{node}} 1m"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Avg5").
					Expr(`proxmox_node_cpustat_avg5_ratio{`+nodeFilter+`} / proxmox_node_cpustat_cpus_ratio{`+nodeFilter+`}`).
					LegendFormat("{{node}} 5m"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Avg15").
					Expr(`proxmox_node_cpustat_avg15_ratio{`+nodeFilter+`} / proxmox_node_cpustat_cpus_ratio{`+nodeFilter+`}`).
					LegendFormat("{{node}} 15m"),
				).
				OverrideByQuery("Avg5", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}).
				OverrideByQuery("Avg15", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node ZFS ARC Size").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_node_memory_arcsize_bytes{` + nodeFilter + `}`).
					LegendFormat("{{node}} ARC"),
				),
		).

		// Guest: per-VM/LXC resource trends
		WithRow(dashboard.NewRowBuilder("Guest")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Guest Uptime").
				Datasource(ds).
				Span(24).Height(6).
				GraphMode(common.BigValueGraphModeNone).
				Unit("s").
				Min(0).
				Decimals(0).
				// Show 0 as red to highlight recently restarted guests.
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Thresholds(
					dashboard.NewThresholdsConfigBuilder().
						Mode(dashboard.ThresholdsModeAbsolute).
						Steps([]dashboard.Threshold{
							{Value: nil, Color: "red"},
							{Value: float64Ptr(300), Color: "yellow"},
							{Value: float64Ptr(3600), Color: "green"},
						}),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_vm_uptime_seconds{` + nodeFilter + `}`).
					Instant().
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				Min(0).
				Max(1).
				Tooltip(tooltipSingle).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_vm_cpu_percent{` + nodeFilter + `}`).
					LegendFormat("{{name}} ({{type}})"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest Memory Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipSingle).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Used").
					Expr(`proxmox_vm_mem_bytes{`+nodeFilter+`}`).
					LegendFormat("{{name}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Total").
					Expr(`proxmox_vm_maxmem_bytes{`+nodeFilter+`}`).
					LegendFormat("{{name}} Total"),
				).
				OverrideByQuery("Total", []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest Disk I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(tooltipSingle).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Read").
					Expr(`rate(proxmox_vm_diskread_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
					LegendFormat("{{name}} Read"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Write").
					Expr(`rate(proxmox_vm_diskwrite_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
					LegendFormat("{{name}} Write"),
				).
				OverrideByQuery("Write", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest Network I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(tooltipSingle).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Rx").
					Expr(`rate(proxmox_vm_netin_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
					LegendFormat("{{name}} RX"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Tx").
					Expr(`rate(proxmox_vm_netout_bytes_total{`+nodeFilter+`}[$__rate_interval])`).
					LegendFormat("{{name}} TX"),
				).
				OverrideByQuery("Tx", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest CPU Pressure (PSI some)").
				Datasource(ds).
				Span(24).Height(8).
				// pressurecpusome_percent: % of time at least one task was stalled on CPU.
				Unit("percent").
				Min(0).
				Max(100).
				Tooltip(tooltipSingle).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_vm_pressurecpusome_percent{` + nodeFilter + `}`).
					LegendFormat("{{name}} ({{type}})"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest I/O Pressure (PSI some)").
				Datasource(ds).
				Span(24).Height(8).
				// Despite its metric suffix, pressureiosome_ratio is emitted on a 0-100 percent scale.
				Unit("percent").
				Min(0).
				Max(100).
				Tooltip(tooltipSingle).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_vm_pressureiosome_ratio{` + nodeFilter + `}`).
					LegendFormat("{{name}} ({{type}})"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
