package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildProxmoxOverview defines the Proxmox VE cluster dashboard.
// pve-exporter id label identifies resource type:
//   - node/*      : PVE host resources
//   - qemu/*      : QEMU virtual machines
//   - lxc/*       : LXC containers
//   - storage/*/* : Storage pools
func buildProxmoxOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// pve-exporter instance labels lack the port suffix that node_exporter adds (:9100).
	// instFilter uses an optional port group so both exporters match the same $instance variable.
	// joinNodeExporter joins with node_exporter metrics, which must have their instance port stripped and exposed as an 'addr' label.
	const (
		instFilter = `instance=~"$instance(:.*)?"`
		// joinNode is for queries that aggregate away id (e.g. count by (instance)).
		joinNode = `* on(instance) group_left(name) pve_node_info`
		// joinNodeID is for raw per-node metrics that retain the id label; avoids
		// many-to-many matching when a single pve-exporter reports all cluster nodes.
		joinNodeID    = `* on(id, instance) group_left(name) pve_node_info`
		joinGuest     = `* on(id, instance) group_left(name) pve_guest_info`
		joinGuestNode = `* on(id, instance) group_left(node) pve_guest_info`
		joinStorage   = `* on(id, instance) group_left(storage, node) pve_storage_info`
		// joinNodeExporter bridges per-node node_exporter instances (IP:9100) to pve node
		// names via node_uname_info (hostname) → pve_node_info (name), because a single
		// cluster-level pve-exporter does not expose individual node IPs in its instance label.
		joinNodeExporter = `* on(instance) group_left(nodename) node_uname_info * on(nodename) group_left(name) label_replace(pve_node_info, "nodename", "$1", "name", "(.*)")`
	)

	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)

	cpuThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(0.8), Color: "yellow"},
			{Value: float64Ptr(0.9), Color: "red"},
		})

	pctThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(80), Color: "yellow"},
			{Value: float64Ptr(90), Color: "red"},
		})

	issueThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("Proxmox Overview").
		Uid("proxmox-overview").
		Tags([]string{"proxmox", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(pve_node_info, name)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		// Hidden variable: resolves $node (name) to $instance (IP).
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(pve_node_info{name=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).

		// Row 1: Guest counts + node resource summary
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running VMs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (node) ((pve_up{id=~"qemu/.*", ` + instFilter + `} == 1) ` + joinGuestNode + `)`).
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Stopped VMs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (node) ((pve_up{id=~"qemu/.*", ` + instFilter + `} ` + joinGuestNode + `) == bool 0)`).
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running LXCs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (node) ((pve_up{id=~"lxc/.*", ` + instFilter + `} == 1) ` + joinGuestNode + `)`).
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Stopped LXCs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (node) ((pve_up{id=~"lxc/.*", ` + instFilter + `} ` + joinGuestNode + `) == bool 0)`).
					LegendFormat("{{node}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node CPU Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percentunit").
				Decimals(1).
				Thresholds(cpuThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_cpu_usage_ratio{id=~"node/.*", ` + instFilter + `} ` + joinNodeID).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node Memory Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_usage_bytes{id=~"node/.*", ` + instFilter + `} / pve_memory_size_bytes{id=~"node/.*", ` + instFilter + `} * 100 ` + joinNodeID).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Storage Usage").
				Datasource(ds).
				Span(12).Height(4).
				Unit("percent").
				Decimals(1).
				Thresholds(pctThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_disk_usage_bytes{id=~"storage/.*", ` + instFilter + `} / pve_disk_size_bytes{id=~"storage/.*", ` + instFilter + `} * 100 ` + joinStorage).
					LegendFormat("{{node}}/{{storage}}"),
				),
		).

		// Row 2: Node resource trends
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_cpu_usage_ratio{id=~"node/.*", ` + instFilter + `} ` + joinNodeID).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Memory Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_usage_bytes{id=~"node/.*", ` + instFilter + `} ` + joinNodeID).
					LegendFormat("{{name}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_size_bytes{id=~"node/.*", ` + instFilter + `} ` + joinNodeID).
					LegendFormat("{{name}} Total"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Intel: x86_pkg_temp / RPi: cpu-thermal
					Expr(`node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal"} ` + joinNodeExporter).
					LegendFormat("{{name}} CPU {{type}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Ryzen: k10temp exposed as PCI device (0000:00:18.x), temp1=Tctl
					Expr(`node_hwmon_temp_celsius{chip=~".*_0000:00:18_.*", sensor="temp1"} ` + joinNodeExporter).
					LegendFormat("{{name}} CPU Tctl"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`smartmon_temperature_celsius_raw_value ` + joinNodeExporter).
					LegendFormat("{{name}} Disk {{device}}"),
				),
		).

		// Row 3: Guest resource trends
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_cpu_usage_ratio{id=~"qemu/.*|lxc/.*", ` + instFilter + `} ` + joinGuest).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest Memory Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_usage_bytes{id=~"qemu/.*|lxc/.*", ` + instFilter + `} ` + joinGuest).
					LegendFormat("{{name}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_size_bytes{id=~"qemu/.*|lxc/.*", ` + instFilter + `} ` + joinGuest).
					LegendFormat("{{name}} Total"),
				),
		).

		// Row 4: Storage
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Storage Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_disk_usage_bytes{id=~"storage/.*", ` + instFilter + `} ` + joinStorage).
					LegendFormat("{{node}}/{{storage}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_disk_size_bytes{id=~"storage/.*", ` + instFilter + `} ` + joinStorage).
					LegendFormat("{{node}}/{{storage}} Total"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
