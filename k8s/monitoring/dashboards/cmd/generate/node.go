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
		instFilter   = `instance=~"$instance"`
		joinNodename = `* on(instance) group_left(nodename) node_uname_info`
	)

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
				Title("Load Average (1m)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_load1{` + instFilter + `} ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(2),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdContinuousRdYlGr)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_time_seconds{` + instFilter + `} - node_boot_time_seconds{` + instFilter + `}) ` + joinNodename).
					LegendFormat("{{nodename}}"),
				).Decimals(2),
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
