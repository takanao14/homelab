package main

import (
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
		instFilter       = `instance=~"$instance(:.*)?"`
		joinNode         = `* on(instance) group_left(name) pve_node_info`
		joinGuest        = `* on(id, instance) group_left(name) pve_guest_info`
		joinStorage      = `* on(id, instance) group_left(storage, node) pve_storage_info`
		joinNodeExporter = `* on(addr) group_left(name) (label_replace(pve_node_info, "addr", "$1", "instance", "(.*)"))`
	)

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
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running VMs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (pve_up{id=~"qemu/.*", ` + instFilter + `} == 1) ` + joinNode).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Running LXCs").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (pve_up{id=~"lxc/.*", ` + instFilter + `} == 1) ` + joinNode).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node CPU Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percentunit").
				Decimals(1).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_cpu_usage_ratio{id=~"node/.*", ` + instFilter + `} ` + joinNode).
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
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_usage_bytes{id=~"node/.*", ` + instFilter + `} / pve_memory_size_bytes{id=~"node/.*", ` + instFilter + `} * 100 ` + joinNode).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Storage Usage").
				Datasource(ds).
				Span(24).Height(4).
				Unit("percent").
				Decimals(1).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_disk_usage_bytes{id=~"storage/.*", ` + instFilter + `} / pve_disk_size_bytes{id=~"storage/.*", ` + instFilter + `} * 100 ` + joinStorage).
					LegendFormat("{{node}}/{{storage}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_cpu_usage_ratio{id=~"node/.*", ` + instFilter + `} ` + joinNode).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Memory Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_usage_bytes{id=~"node/.*", ` + instFilter + `} ` + joinNode).
					LegendFormat("{{name}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_size_bytes{id=~"node/.*", ` + instFilter + `} ` + joinNode).
					LegendFormat("{{name}} Total"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Node Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(label_replace((node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal", ` + instFilter + `}) or (node_hwmon_temp_celsius{chip=~".*18_3", sensor="temp3", ` + instFilter + `}), "addr", "$1", "instance", "(.*):9100")) ` + joinNodeExporter).
					LegendFormat("{{name}} CPU {{type}}{{chip}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(label_replace(smartmon_temperature_celsius_raw_value{` + instFilter + `}, "addr", "$1", "instance", "(.*):9100")) ` + joinNodeExporter).
					LegendFormat("{{name}} Disk {{device}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Guest CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percentunit").
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
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pve_memory_usage_bytes{id=~"qemu/.*|lxc/.*", ` + instFilter + `} ` + joinGuest).
					LegendFormat("{{name}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Storage Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
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
