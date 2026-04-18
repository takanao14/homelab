package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func main() {
	// Map of dashboard name to builder function.
	// The name becomes the output filename (e.g. "node-overview" → generated/node-overview.json).
	// To add a new dashboard, just add an entry here.
	dashboards := map[string]func() (*dashboard.Dashboard, error){
		"node-overview":    buildNodeOverview,
		"proxmox-overview": buildProxmoxOverview,
		"gpu-overview":     buildGpuOverview,
		"dns-overview":     buildDnsOverview,
		"network-overview": buildNetworkOverview,
		"uptime":           buildUptime,
	}

	outputDir := "generated"
	if len(os.Args) > 1 {
		outputDir = os.Args[1]
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("failed to create output dir: %v", err)
	}

	for name, build := range dashboards {
		d, err := build()
		if err != nil {
			log.Fatalf("failed to build dashboard %s: %v", name, err)
		}

		out, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal dashboard %s: %v", name, err)
		}

		path := filepath.Join(outputDir, name+".json")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			log.Fatalf("failed to write %s: %v", path, err)
		}
		fmt.Printf("generated: %s\n", path)
	}
}

// promDatasource returns a datasource ref using "$datasource" as the UID so that
// all panels switch together when the user changes the datasource dropdown variable.
func promDatasource() common.DataSourceRef {
	dsType := "prometheus"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

func buildNodeOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// Two-stage variable resolution: node_* metrics carry instance (IP:port) but
	// display names come from node_uname_info which has nodename. We expose $node
	// (nodename) in the UI and hide $instance (IP:port) resolved from it.
	// joinNodename copies nodename onto query results so legends show hostnames.
	const (
		instFilter   = `instance=~"$instance"`
		joinNodename = `* on(instance) group_left(nodename) node_uname_info`
	)

	d, err := dashboard.NewDashboardBuilder("Node Overview").
		Uid("node-overview").
		Tags([]string{"nodes", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		// Exclude k0s-worker1 (Kubernetes node monitored via kube-state-metrics).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info{nodename!="k0s-worker1"}, nodename)`)}).
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
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info{nodename=~"$node"}, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Multi(true).
				IncludeAll(true).
				Hide(dashboard.VariableHideHideVariable),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CPU Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + instFilter + `}[5m]) ` + joinNodename + `) * 100)`).
					LegendFormat("{{nodename}}"),
				),
		).
		// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(1 - node_memory_MemAvailable_bytes{` + instFilter + `} / node_memory_MemTotal_bytes{` + instFilter + `}) ` + joinNodename + ` * 100`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Load Average (1m)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_load1{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_time_seconds{` + instFilter + `} - node_boot_time_seconds{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + instFilter + `}[5m]) ` + joinNodename + `) * 100)`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Load Average").
				Datasource(ds).
				Span(24).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_load1{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} 1m"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_load5{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} 5m"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_load15{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} 15m"),
				),
		).
		// Used = Total - Available (buffers/cache are included in Available).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Used").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_memory_MemTotal_bytes{` + instFilter + `} - node_memory_MemAvailable_bytes{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_memory_MemAvailable_bytes{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} Available"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Temperature").
				Datasource(ds).
				Span(12).Height(8).
				Unit("celsius").
				// CPU temp: x86 Package (Intel), cpu-thermal (RPi), or k10temp chip (Ryzen)
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`((node_thermal_zone_temp{type=~"x86_pkg_temp|cpu-thermal", ` + instFilter + `}) or (node_hwmon_temp_celsius{chip=~".*18_3", sensor="temp3", ` + instFilter + `})) ` + joinNodename).
					LegendFormat("{{nodename}} CPU {{type}}{{chip}}"),
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
		// Exclude dm-* (device mapper / LVM virtual devices) to avoid double-counting with physical devices.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Disk I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_disk_read_bytes_total{` + instFilter + `, device!~"dm-.*"}[5m]) ` + joinNodename).
					LegendFormat("{{nodename}} Read {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_disk_written_bytes_total{` + instFilter + `, device!~"dm-.*"}[5m]) ` + joinNodename).
					LegendFormat("{{nodename}} Write {{device}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_network_receive_bytes_total{` + instFilter + `, device!="lo"}[5m]) ` + joinNodename).
					LegendFormat("{{nodename}} Rx {{device}}"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_network_transmit_bytes_total{` + instFilter + `, device!="lo"}[5m]) ` + joinNodename).
					LegendFormat("{{nodename}} Tx {{device}}"),
				),
		).
		// ZFS ARC metrics are only present on PVE hosts; other nodes will show no data.
		// ARC Size pinned to ARC Max means memory is being used efficiently.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("ZFS ARC Size").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_zfs_arc_size{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} ARC Size"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_zfs_arc_c_max{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} ARC Max"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_zfs_arc_c_min{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}} ARC Min"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

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
	// joinNodeExporter strips the port with label_replace before joining on instance.
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
		Time("now-1h", "now").
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

// buildGpuOverview defines the AMD GPU dashboard for the single RX 9060 XT on gpuvm.
// No variables needed; job label is sufficient to target the single GPU.
func buildGpuOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const gpuFilter = `job="scrapeConfig/monitoring/amd-gpu-external"`

	d, err := dashboard.NewDashboardBuilder("GPU Overview").
		Uid("gpu-overview").
		Tags([]string{"gpu", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("GFX Activity").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_gfx_activity{` + gpuFilter + `}`).
					LegendFormat("GFX Activity"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("VRAM Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_used_vram{` + gpuFilter + `} / amd_gpu_total_vram{` + gpuFilter + `} * 100`).
					LegendFormat("VRAM Usage"),
				),
		).
		// Edge temperature is the standard GPU die temperature metric.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Temperature (Edge)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("celsius").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_edge_temperature{` + gpuFilter + `}`).
					LegendFormat("Edge Temp"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("watt").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_power_usage{` + gpuFilter + `}`).
					LegendFormat("Power"),
				),
		).
		// gfx=graphics/compute, umc=memory controller, vcn=video codec engine
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("GPU Activity (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_gfx_activity{` + gpuFilter + `}`).
					LegendFormat("GFX"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_umc_activity{` + gpuFilter + `}`).
					LegendFormat("Memory Controller"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_vcn_activity{` + gpuFilter + `}`).
					LegendFormat("Video Codec"),
				),
		).
		// Metrics are in MiB; multiply to bytes so Grafana auto-scales the unit.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("VRAM").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_used_vram{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_total_vram{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Total"),
				),
		).
		// GTT = GPU-accessible system RAM (graphics translation table).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("GTT Memory").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_used_gtt{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_total_gtt{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Total"),
				),
		).
		// edge=die edge, junction=hotspot (highest temp point), memory=VRAM temperature
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_edge_temperature{` + gpuFilter + `}`).
					LegendFormat("Edge"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_junction_temperature{` + gpuFilter + `}`).
					LegendFormat("Junction (Hotspot)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_memory_temperature{` + gpuFilter + `}`).
					LegendFormat("Memory"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Power").
				Datasource(ds).
				Span(12).Height(8).
				Unit("watt").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_power_usage{` + gpuFilter + `}`).
					LegendFormat("Current"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_average_package_power{` + gpuFilter + `}`).
					LegendFormat("Average"),
				),
		).
		// Metrics are in MHz; multiply to Hz for Grafana unit auto-scaling.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Clock Speed").
				Datasource(ds).
				Span(12).Height(8).
				Unit("hertz").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_clock{` + gpuFilter + `, clock_type="GPU_CLOCK_TYPE_SYSTEM"} * 1000 * 1000`).
					LegendFormat("GPU Core"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_clock{` + gpuFilter + `, clock_type="GPU_CLOCK_TYPE_MEMORY"} * 1000 * 1000`).
					LegendFormat("Memory"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

// buildDnsOverview defines the DNS infrastructure dashboard.
// Two-server setup (192.168.10.241/242); no variables needed — {{instance}} distinguishes them.
//   - dnsdist  : DNS frontend / load balancer (port 8083)
//   - pdns-auth: authoritative DNS server (port 8081)
func buildDnsOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		dnsdist = `job="scrapeConfig/monitoring/dnsdist-external"`
		pdns    = `job="scrapeConfig/monitoring/pdns-auth-external"`
	)

	d, err := dashboard.NewDashboardBuilder("DNS Overview").
		Uid("dns-overview").
		Tags([]string{"dns", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist QPS").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_queries{` + dnsdist + `}[5m]))`).
					LegendFormat("QPS"),
				),
		).
		// clamp_min prevents division by zero when there are no queries yet.
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist Cache Hit Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(dnsdist_cache_hits{` + dnsdist + `}[5m])) / clamp_min(sum(rate(dnsdist_cache_hits{` + dnsdist + `}[5m]) + rate(dnsdist_cache_misses{` + dnsdist + `}[5m])), 1) * 100`).
					LegendFormat("Cache Hit Rate"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("dnsdist Avg Latency").
				Datasource(ds).
				Span(6).Height(4).
				Unit("µs").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(dnsdist_latency_avg100{` + dnsdist + `})`).
					LegendFormat("Avg Latency"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("pdns-auth QPS").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(pdns_auth_udp_queries{` + pdns + `}[5m]))`).
					LegendFormat("QPS"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_queries{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Queries"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_responses{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Responses"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Response Codes").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_frontend_noerror{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_frontend_nxdomain{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_frontend_servfail{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} SERVFAIL"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("µs").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dnsdist_latency_avg100{` + dnsdist + `}`).
					LegendFormat("{{instance}} avg100"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`dnsdist_latency_avg1000{` + dnsdist + `}`).
					LegendFormat("{{instance}} avg1000"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("dnsdist Cache").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_cache_hits{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Hits"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(dnsdist_cache_misses{` + dnsdist + `}[5m])`).
					LegendFormat("{{instance}} Misses"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Query Rate").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_udp_queries{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} UDP"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_tcp_queries{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} TCP"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Response Codes").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_noerror_packets{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} NOERROR"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_nxdomain_packets{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} NXDOMAIN"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(pdns_auth_servfail_packets{` + pdns + `}[5m])`).
					LegendFormat("{{instance}} SERVFAIL"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("pdns-auth Latency").
				Datasource(ds).
				Span(12).Height(8).
				Unit("µs").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`pdns_auth_latency{` + pdns + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

// buildNetworkOverview defines the network device dashboard using SNMP MIB-II metrics.
// Targets: 192.168.10.1 (router), 192.168.10.2 (switch).
// ifHC* counters are 64-bit, avoiding wrap-around on high-speed interfaces.
func buildNetworkOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// Exclude virtual/management interfaces to show only physical ports.
	const ifFilter = `ifDescr!~"Loopback.*|Null.*|bluetooth.*", instance=~"$instance"`

	d, err := dashboard.NewDashboardBuilder("Network Overview").
		Uid("network-overview").
		Tags([]string{"network", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("60s"). // SNMP scrapes are expensive; 60s is a reasonable interval.
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Label("Device").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(ifHCInOctets, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Interfaces Up").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} == 1)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Traffic In").
				Datasource(ds).
				Span(8).Height(4).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCInOctets{` + ifFilter + `}[5m]) * 8)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Traffic Out").
				Datasource(ds).
				Span(8).Height(4).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCOutOctets{` + ifFilter + `}[5m]) * 8)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Traffic In (bps)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifHCInOctets{` + ifFilter + `}[5m]) * 8`).
					LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Traffic Out (bps)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifHCOutOctets{` + ifFilter + `}[5m]) * 8`).
					LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Interface Errors").
				Datasource(ds).
				Span(12).Height(8).
				Unit("pps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInErrors{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutErrors{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Interface Discards").
				Datasource(ds).
				Span(12).Height(8).
				Unit("pps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInDiscards{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutDiscards{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

// buildUptime defines the availability monitoring dashboard.
// blackbox-exporter probes return probe_success (1=up, 0=down).
// ScrapeConfig job label format: scrapeConfig/<namespace>/<name>.
func buildUptime() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		icmpJob   = `job="scrapeConfig/monitoring/icmp-network-devices"`
		dnsExtJob = `job="scrapeConfig/monitoring/dns-external"`
		dnsIntJob = `job="scrapeConfig/monitoring/dns-internal"`
	)

	// nil threshold Value means -Infinity (base step).
	probeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: float64Ptr(1), Color: "green"},
		})

	probeValueMappings := []dashboard.ValueMapping{
		{ValueMap: &dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				"0": {Text: strPtr("DOWN"), Color: strPtr("red")},
				"1": {Text: strPtr("UP"), Color: strPtr("green")},
			},
		}},
	}

	d, err := dashboard.NewDashboardBuilder("Uptime").
		Uid("uptime").
		Tags([]string{"uptime", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ICMP Status").
				Datasource(ds).
				Span(24).Height(4).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS External Status").
				Datasource(ds).
				Span(12).Height(4).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsExtJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS Internal Status").
				Datasource(ds).
				Span(12).Height(4).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsIntJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			statetimeline.NewPanelBuilder().
				Title("ICMP Status History").
				Datasource(ds).
				Span(12).Height(8).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("ICMP Response Time").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_duration_seconds{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			statetimeline.NewPanelBuilder().
				Title("DNS Status History").
				Datasource(ds).
				Span(12).Height(8).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsExtJob + `}`).
					LegendFormat("{{instance}} External"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsIntJob + `}`).
					LegendFormat("{{instance}} Internal"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("DNS Response Time").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_dns_lookup_time_seconds{` + dnsExtJob + `}`).
					LegendFormat("{{instance}} External"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_dns_lookup_time_seconds{` + dnsIntJob + `}`).
					LegendFormat("{{instance}} Internal"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}
