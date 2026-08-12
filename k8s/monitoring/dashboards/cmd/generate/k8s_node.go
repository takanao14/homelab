package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func buildK8sNodeOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		clusterFilter = `cluster=~"$cluster"`
		nodeFilter    = `nodename=~"$node"`
		// fsFilter excludes pseudo/boot filesystems consistent with node-overview.
		fsFilter = `fstype=~"ext[234]|xfs|btrfs|zfs|vfat",mountpoint!~"/var/lib/docker/.*|/boot/efi|/boot/firmware"`
		// netDeviceFilter drops the virtual interfaces the CNI creates, leaving the
		// bytes that actually crossed a wire.
		//
		// The lxc.* and cilium_.* terms are the ones that matter here. Cilium names
		// the pod side of each veth pair lxc<hash> rather than veth<hash>, so the
		// Docker-era exclusions above it matched none of them: 41 lxc* interfaces
		// were being summed into the per-node total alongside eth0, counting every
		// pod packet at least twice and vxlan-encapsulated traffic three times.
		// Measured before this filter, receive rate against the physical NIC alone:
		// k0s-worker1 243 kB/s vs 107, gpuvm1 19.8 kB/s vs 2.9, k0s-worker2
		// 11.3 kB/s vs 4.7. k0s-cp1 agreed exactly, because it runs no pods -- the
		// error scaled with how much work a node was doing, which is the worst
		// shape for it to take.
		//
		// Written as exclusions rather than an eth[0-9]+|ens.*|enp.* allowlist so
		// that an interface nobody anticipated still shows up. A bond, a second
		// NIC, or a rename would silently vanish from an allowlist; here it appears
		// and can be dealt with.
		netDeviceFilter = `device!~"lo|veth.*|docker.*|br-.*|lxc.*|cilium_.*"`
	)

	// joinNode copies nodename onto query results so legends show hostnames.
	// max by dedupes node_uname_info: a kernel upgrade + reboot leaves two series
	// for the same instance within the dashboard window (differing release/version
	// labels), which would otherwise fail the join with a many-to-many error.
	joinNode := `* on(instance) group_left(nodename) max by (instance, nodename) (node_uname_info{` + clusterFilter + `, ` + nodeFilter + `})`

	// perCore divides a per-instance reading by that instance's logical CPU count.
	// The cluster is not uniform -- 2 CPUs on k0s-cp1 against 10 on the workers --
	// so a raw load average puts readings that mean opposite things at the same
	// height. Measured while writing this: cp1 and k0s-worker1 both reported
	// load1 = 0.08, which is 13.5% per CPU on cp1 and 0.9% on worker1, a factor of
	// fifteen. Per CPU, 100% is a saturated node whatever its size. node-overview
	// computes the same thing as normByCPU and displays it the same way.
	perCore := func(expr string) string {
		return expr + ` / on(instance) group_left() count by (instance) (node_cpu_seconds_total{mode="idle", ` + clusterFilter + `})`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()

	// A reference line at 1.0 -- one runnable task per CPU, shown as 100% by the
	// percentunit display. Transparent below so only the line itself is drawn,
	// matching the zero-line treatment used on the I/O panels. The axis is left to
	// autoscale rather than pinned to 1: the cluster idles three orders of
	// magnitude below saturation, and forcing the line into view would flatten
	// every real difference to nothing.
	saturationThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "transparent"},
			{Value: new(float64(1)), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("K8s Node Overview").
		Uid("k8s-node-overview").
		Tags([]string{"kubernetes", "nodes", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("cluster").
				Label("Cluster").
				Datasource(ds).
				// Use node_uname_info as it is present on both k8s and external nodes.
				Query(dashboard.StringOrMap{String: new(`label_values(node_uname_info, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				// We prefer 'nodename' as it is the most consistent label across node_exporter metrics.
				Query(dashboard.StringOrMap{String: new(`label_values(node_uname_info{` + clusterFilter + `}, nodename)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("CPU Usage").
				Datasource(ds).
				Span(12).Height(6).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + clusterFilter + `}[$__rate_interval]) ` + joinNode + `) * 100)`).
					LegendFormat("{{nodename}}"),
				).Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(12).Height(6).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// MemAvailable includes reclaimable cache, giving a more realistic usage figure than MemFree.
					Expr(`(1 - node_memory_MemAvailable_bytes{` + clusterFilter + `} / node_memory_MemTotal_bytes{` + clusterFilter + `}) ` + joinNode + ` * 100`).
					LegendFormat("{{nodename}}"),
				).Decimals(1),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Pods Running").
				Description("Worker nodes only. k0s controllers do not run a kubelet unless started with --enable-worker, so a control-plane node is absent here rather than reading zero: the metric this counts does not exist on it.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Join 'node' from kubelet with 'nodename' from node_uname_info.
					// max by runs before label_replace so the join sees one series per
					// node; see joinNode above for why node_uname_info needs deduping.
					Expr(`sum by (nodename) (kubelet_running_pods{` + clusterFilter + `} * on(node) group_left(nodename) (label_replace(max by (nodename) (node_uname_info{` + clusterFilter + `, ` + nodeFilter + `}), "node", "$1", "nodename", "(.*)")))`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CPU Cores").
				Description("Logical cores per node. Inventory, not a measurement -- it changes only when a VM is resized.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				// A stat rather than a bar gauge, matching Memory Total beside it.
				// Bar length only means something against a maximum, and a core
				// count has none: with no Max set Grafana scaled each bar against
				// the largest node, so k0s-cp1 drew a fifth of a bar. That reads as
				// a level with headroom, directly under a bar gauge where it is one.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (nodename) (node_cpu_seconds_total{mode="idle", ` + clusterFilter + `} ` + joinNode + `)`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Memory Total").
				Datasource(ds).
				Span(12).Height(4).
				Unit("bytes").
				Min(0).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_memory_MemTotal_bytes{` + clusterFilter + `} ` + joinNode).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				Span(12).Height(4).
				Unit("s").
				Min(0).
				GraphMode(common.BigValueGraphModeNone).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				JustifyMode(common.BigValueJustifyModeCenter).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "red"},
						{Value: new(float64(3600)), Color: "yellow"},
						{Value: new(float64(86400)), Color: "green"},
					}),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(time() - node_boot_time_seconds{` + clusterFilter + `}) ` + joinNode).
					LegendFormat("{{nodename}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("CPU")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", ` + clusterFilter + `}[$__rate_interval]) ` + joinNode + `) * 100)`).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Load Average per CPU").
				Description("Run-queue length divided by the node's CPU count, so 100% is a saturated node whatever its size and the red line marks it. Raw load averages are not comparable across this cluster: k0s-cp1 has 2 CPUs against 10 on the workers, so the same load1 of 0.08 is 13.5% per CPU on cp1 and 0.9% on worker1. Read a value here as the fraction of the node's CPUs that had work waiting.").
				Datasource(ds).
				Span(24).Height(8).
				// percentunit, matching "Load Average per CPU (1m)" on node-overview.
				// The quantity is the same on both dashboards and was reading 0.135
				// on one and 13.5% on the other, which invites the two to be compared
				// as though they were different measurements.
				Unit("percentunit").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(saturationThresholds).
				ThresholdsStyle(common.NewGraphThresholdsStyleConfigBuilder().
					Mode(common.GraphThresholdsStyleModeLine)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(perCore(`node_load1{`+clusterFilter+`}`)+` `+joinNode).
					LegendFormat("{{nodename}} 1m"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(perCore(`node_load5{`+clusterFilter+`}`)+` `+joinNode).
					LegendFormat("{{nodename}} 5m"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(perCore(`node_load15{`+clusterFilter+`}`)+` `+joinNode).
					LegendFormat("{{nodename}} 15m"),
				).
				// 5m/15m as dashed to distinguish from 1m (solid) without adding noise.
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* 5m$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* 15m$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Memory")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_memory_MemTotal_bytes{`+clusterFilter+`} - node_memory_MemAvailable_bytes{`+clusterFilter+`}) `+joinNode).
					LegendFormat("{{nodename}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_memory_MemAvailable_bytes{`+clusterFilter+`} `+joinNode).
					LegendFormat("{{nodename}} Available"),
				).
				// Available dotted, Used solid. The two are complements summing to
				// total, so drawn identically they read as two independent figures
				// and it takes a moment to work out which is which. Dotted matches
				// how "Disk Space Used" distinguishes its reference line, so the
				// same visual convention means the same thing on both panels.
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Available$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		// PSI: fraction of time at least one task was stalled ("some") or all
		// tasks were stalled ("full") waiting on the resource. CPU has no "full"
		// series since the kernel doesn't track fully stalled CPU time.
		WithRow(dashboard.NewRowBuilder("Pressure (PSI)")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Pressure (some)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(node_pressure_cpu_waiting_seconds_total{` + clusterFilter + `}[$__rate_interval]) * 100 ` + joinNode).
					LegendFormat("{{nodename}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Pressure").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Some").
					Expr(`rate(node_pressure_memory_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode).
					LegendFormat("{{nodename}} some"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Full").
					Expr(`rate(node_pressure_memory_stalled_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode).
					LegendFormat("{{nodename}} full"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("IO Pressure").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Some").
					Expr(`rate(node_pressure_io_waiting_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode).
					LegendFormat("{{nodename}} some"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Full").
					Expr(`rate(node_pressure_io_stalled_seconds_total{`+clusterFilter+`}[$__rate_interval]) * 100 `+joinNode).
					LegendFormat("{{nodename}} full"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* full$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 8}}},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Disk")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Filesystem Usage (%)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc((1 - node_filesystem_avail_bytes{` + clusterFilter + `, ` + fsFilter + `} / node_filesystem_size_bytes{` + clusterFilter + `, ` + fsFilter + `}) ` + joinNode + ` * 100)`).
					Instant().
					LegendFormat("{{nodename}} {{mountpoint}}"),
				).Decimals(1),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Disk Space Used").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`} - node_filesystem_avail_bytes{`+clusterFilter+`, `+fsFilter+`}) `+joinNode).
					LegendFormat("{{nodename}} {{mountpoint}} Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`node_filesystem_size_bytes{`+clusterFilter+`, `+fsFilter+`} `+joinNode).
					LegendFormat("{{nodename}} {{mountpoint}} Total"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Total$"}, []dashboard.DynamicConfigValue{
					{Id: "custom.lineStyle", Value: map[string]any{"fill": "dot", "dash": []int{2, 4}}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Disk I/O").
				Datasource(ds).
				Span(24).Height(8).
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Read").
					// Exclude dm-*, loop*, and sr* to avoid double-counting or noise from virtual/optical devices.
					Expr(`rate(node_disk_read_bytes_total{`+clusterFilter+`, device!~"dm-.*|loop.*|sr.*"}[$__rate_interval]) `+joinNode).
					LegendFormat("{{nodename}} {{device}} Read"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Write").
					Expr(`rate(node_disk_written_bytes_total{`+clusterFilter+`, device!~"dm-.*|loop.*|sr.*"}[$__rate_interval]) `+joinNode).
					LegendFormat("{{nodename}} {{device}} Write"),
				).
				OverrideByQuery("Write", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Network")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network I/O").
				Description("Traffic across each node's physical interfaces. The CNI's per-pod veths and Cilium's own devices are excluded, so pod traffic is counted once, where it leaves the node, rather than again on every virtual hop.").
				Datasource(ds).
				Span(24).Height(12).
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Rx").
					Expr(`sum by (nodename) (rate(node_network_receive_bytes_total{`+clusterFilter+`, `+netDeviceFilter+`} [$__rate_interval]) `+joinNode+`)`).
					LegendFormat("{{nodename}} Rx"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Tx").
					Expr(`sum by (nodename) (rate(node_network_transmit_bytes_total{`+clusterFilter+`, `+netDeviceFilter+`}[$__rate_interval]) `+joinNode+`)`).
					LegendFormat("{{nodename}} Tx"),
				).
				OverrideByQuery("Tx", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
