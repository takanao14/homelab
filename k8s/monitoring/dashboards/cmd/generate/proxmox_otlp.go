package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildProxmoxOtlpOverview covers PVE 9 native OTLP node, guest, and storage
// metrics. OTLP emits running guests only and already carries usable labels.
func buildProxmoxOtlpOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

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

	cpuThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(0.8)), Color: "yellow"},
			{Value: new(float64(0.9)), Color: "red"},
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
				Query(dashboard.StringOrMap{String: new(`label_values(proxmox_node_cpustat_cpu_percent{` + job + `}, node)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).

		// Summary combines running guest counts and node resources.
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running VMs").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
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
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
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
				Span(12).Height(4).
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
				Span(12).Height(4).
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
				Span(24).Height(4).
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
				Span(24).Height(4).
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
					// Exclude loopback, guest, firewall, and unused nic0 devices. Keep nic1,
					// bridges, and SDN vnets; parallel NIC/bridge lines show traffic layers.
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
					// Map OTLP node labels to nodenames before joining temperatures.
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
				Title("Node Load Average per CPU").
				Description("Run-queue length divided by the node's CPU count, so 100% is a saturated node whatever its size. The hosts here run from 6 to 16 CPUs, which is why the raw averages are not comparable between them.").
				Datasource(ds).
				Span(24).Height(8).
				// Match normalized load percentage across node dashboards.
				Unit("percentunit").
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
				Description("Only nodes with a pool imported appear; the others load the ZFS module but hold an empty ARC of about a kilobyte, which on a bytes axis beside a multi-gigabyte ARC is indistinguishable from zero while still occupying the legend. A node that gains a pool shows up here on its own.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`proxmox_node_memory_arcsize_bytes{` + nodeFilter + `}` + zfsActive).
					LegendFormat("{{node}} ARC"),
				),
		).

		// Guest: per-VM/LXC resource trends
		WithRow(dashboard.NewRowBuilder("Guest")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Guest Uptime").
				Datasource(ds).
				Span(24).Height(8).
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
							{Value: new(float64(300)), Color: "yellow"},
							{Value: new(float64(3600)), Color: "green"},
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
				Description("Percentage of time at least one task was stalled waiting for CPU. The axis grows to 100 only if a reading approaches it; on this fleet the busiest moment in 30 days was 2%, and a fixed 0-100 axis flattened all of that into the bottom pixel.").
				Datasource(ds).
				Span(24).Height(8).
				// pressurecpusome_percent: % of time at least one task was stalled on CPU.
				Unit("percent").
				Min(0).
				// CPU pressure is normally low, so use a soft 100% ceiling; I/O pressure
				// can approach 100% and uses a hard bound.
				AxisSoftMax(100).
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
