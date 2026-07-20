package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildKubernetesOverview defines the Kubernetes cluster overview dashboard.
// Uses kube-state-metrics (kube_*) and cAdvisor (container_*) from kube-prometheus-stack.
func buildKubernetesOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		nsFilter      = `namespace=~"$namespace"`
		clusterFilter = `cluster=~"$cluster"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()
	issueThresholds := issueThresholds()

	d, err := dashboard.NewDashboardBuilder("Kubernetes Overview").
		Uid("kubernetes-overview").
		Tags([]string{"kubernetes", "infrastructure"}).
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
				Query(dashboard.StringOrMap{String: new(`label_values(kube_node_info, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("namespace").
				Label("Namespace").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(kube_namespace_status_phase{` + clusterFilter + `}, namespace)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Cluster Health")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Nodes Ready").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_node_status_condition{` + clusterFilter + `,condition="Ready",status="true"})`).
					LegendFormat("Ready"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Nodes Total").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_node_info{` + clusterFilter + `})`).
					LegendFormat("Total"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Pods Running").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// == 1 filters to the active phase only; other phases exist as 0-valued series.
					Expr(`count(kube_pod_status_phase{` + clusterFilter + `,phase="Running",` + nsFilter + `} == 1) or vector(0)`).
					LegendFormat("Running"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Pods Not Running").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Exclude Succeeded (completed Jobs) — only flag genuinely unhealthy phases.
					Expr(`count(kube_pod_status_phase{` + clusterFilter + `,phase!="Running",phase!="Succeeded",` + nsFilter + `} == 1) or vector(0)`).
					LegendFormat("Issues"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Deployments Healthy").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_deployment_status_replicas_available{` + clusterFilter + `,` + nsFilter + `} == kube_deployment_spec_replicas{` + clusterFilter + `,` + nsFilter + `}) or vector(0)`).
					LegendFormat("Healthy"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Deployments Degraded").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_deployment_status_replicas_available{` + clusterFilter + `,` + nsFilter + `} < kube_deployment_spec_replicas{` + clusterFilter + `,` + nsFilter + `}) or vector(0)`).
					LegendFormat("Degraded"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Container Restarts (1h)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(kube_pod_container_status_restarts_total{` + clusterFilter + `,` + nsFilter + `}[1h]))) or vector(0)`).
					LegendFormat("Restarts"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DaemonSets Degraded").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_daemonset_status_number_ready{` + clusterFilter + `,` + nsFilter + `} < kube_daemonset_status_desired_number_scheduled{` + clusterFilter + `,` + nsFilter + `}) or vector(0)`).
					LegendFormat("Degraded"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("StatefulSets Degraded").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_statefulset_status_replicas_ready{` + clusterFilter + `,` + nsFilter + `} < kube_statefulset_replicas{` + clusterFilter + `,` + nsFilter + `}) or vector(0)`).
					LegendFormat("Degraded"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("OOMKilled Containers").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Counts containers whose most recent termination reason was OOMKilled.
					Expr(`count(kube_pod_container_status_last_terminated_reason{` + clusterFilter + `,reason="OOMKilled",` + nsFilter + `} == 1) or vector(0)`).
					LegendFormat("OOMKilled"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node Pressure Conditions").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Any active MemoryPressure, DiskPressure, or PIDPressure across all nodes.
					Expr(`count(kube_node_status_condition{` + clusterFilter + `,condition=~"MemoryPressure|DiskPressure|PIDPressure",status="true"} == 1) or vector(0)`).
					LegendFormat("Pressure"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Resource Usage")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage by Namespace").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// container="" excludes pause containers; pod="" excludes node-level cgroup rollups.
					Expr(`sum by (cluster, namespace) (rate(container_cpu_usage_seconds_total{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{namespace}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Usage by Namespace").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, namespace) (container_memory_working_set_bytes{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""})`).
					LegendFormat("{{cluster}} {{namespace}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Resource Requests vs Actual")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU: Requested vs Actual").
				Datasource(ds).
				Span(12).Height(8).
				Unit("none").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_pod_container_resource_requests{` + clusterFilter + `,resource="cpu",` + nsFilter + `})`).
					LegendFormat("Requested"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(container_cpu_usage_seconds_total{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""}[$__rate_interval]))`).
					LegendFormat("Actual"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory: Requested vs Actual").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_pod_container_resource_requests{` + clusterFilter + `,resource="memory",` + nsFilter + `})`).
					LegendFormat("Requested"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(container_memory_working_set_bytes{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""})`).
					LegendFormat("Actual"),
				),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("Container Resources: Requested vs Actual").
				Datasource(ds).
				Span(24).Height(10).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="cpu",`+nsFilter+`})`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("A"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// container!="" excludes pause containers; pod!="" excludes node-level cgroup rollups.
					Expr(`sum by (cluster, namespace, pod, container) (rate(container_cpu_usage_seconds_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[$__rate_interval]))`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("B"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="memory",`+nsFilter+`})`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("C"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, namespace, pod, container) (container_memory_working_set_bytes{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""})`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("D"),
				).
				WithTransformation(dashboard.DataTransformerConfig{
					Id:      "merge",
					Options: map[string]any{},
				}).
				WithTransformation(dashboard.DataTransformerConfig{
					Id: "organize",
					Options: map[string]any{
						"excludeByName": map[string]any{"Time": true},
						"indexByName": map[string]any{
							"cluster":   0,
							"namespace": 1,
							"pod":       2,
							"container": 3,
							"Value #A":  4,
							"Value #B":  5,
							"Value #C":  6,
							"Value #D":  7,
						},
						"renameByName": map[string]any{
							"Value #A": "CPU Request",
							"Value #B": "CPU Actual",
							"Value #C": "Mem Request",
							"Value #D": "Mem Actual",
						},
					},
				}).
				OverrideByName("CPU Request", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "none"},
					{Id: "decimals", Value: 3},
				}).
				OverrideByName("CPU Actual", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "none"},
					{Id: "decimals", Value: 3},
				}).
				OverrideByName("Mem Request", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "bytes"},
				}).
				OverrideByName("Mem Actual", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "bytes"},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Container Runtime Health")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Top CPU Throttled Containers (5m)").
				Description("Percentage of CPU scheduling periods throttled in the last 5 minutes. Idle containers and zero values are excluded.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(dashboard.NewThresholdsConfigBuilder().
					Mode(dashboard.ThresholdsModeAbsolute).
					Steps([]dashboard.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(20)), Color: "yellow"},
						{Value: new(float64(50)), Color: "red"},
					})).
				WithTarget(prometheus.NewDataqueryBuilder().
					// clamp_min avoids NaN when an idle container has no scheduling periods.
					Expr(`sort_desc(topk(10, (100 * sum by (cluster, namespace, pod, container) (rate(container_cpu_cfs_throttled_periods_total{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""}[$__rate_interval])) / clamp_min(sum by (cluster, namespace, pod, container) (rate(container_cpu_cfs_periods_total{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""}[$__rate_interval])), 1e-9)) > 0)) or on() label_replace(vector(0), "cluster", "No throttling", "", "")`).
					Instant().
					LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}}"),
				).
				Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Container OOM Events (1h)").
				Description("Containers with one or more cgroup OOM events in the last hour.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(sum by (cluster, namespace, pod, container) (increase(container_oom_events_total{` + clusterFilter + `,` + nsFilter + `,container!="",pod!=""}[1h])) > 0) or on() label_replace(vector(0), "cluster", "No OOM events", "", "")`).
					Instant().
					LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}}"),
				).
				Decimals(0),
		).
		WithRow(dashboard.NewRowBuilder("Pod Health")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Pod Phase Count").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (cluster, phase) (kube_pod_status_phase{` + clusterFilter + `,` + nsFilter + `} == 1)`).
					LegendFormat("{{cluster}} {{phase}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Top Restarting Containers (1h)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					// > 0 excludes containers with no restarts; sort_desc orders by count without topk's per-step instability.
					Expr(`sort_desc(sum by (cluster, namespace, pod, container) (increase(kube_pod_container_status_restarts_total{` + clusterFilter + `,` + nsFilter + `}[1h])) > 0) or on() label_replace(vector(0), "cluster", "No restarts", "", "")`).
					Instant().
					LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}}"),
				).
				Decimals(0),
		).
		WithRow(dashboard.NewRowBuilder("Network")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network I/O by Namespace").
				Datasource(ds).
				Span(24).Height(12).
				Unit("Bps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Rx").
					// pod!="" excludes node-level aggregates.
					Expr(`sum by (cluster, namespace) (rate(container_network_receive_bytes_total{`+clusterFilter+`,`+nsFilter+`,pod!=""}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{namespace}} Rx"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Tx").
					Expr(`sum by (cluster, namespace) (rate(container_network_transmit_bytes_total{`+clusterFilter+`,`+nsFilter+`,pod!=""}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{namespace}} Tx"),
				).
				OverrideByQuery("Tx", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Storage")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("PVC Disk Usage (%)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(kubelet_volume_stats_used_bytes{` + clusterFilter + `,` + nsFilter + `} / kubelet_volume_stats_capacity_bytes{` + clusterFilter + `,` + nsFilter + `} * 100)`).
					Instant().
					LegendFormat("{{cluster}} {{namespace}}/{{persistentvolumeclaim}}"),
				).
				Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("PVC Status").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (cluster, phase) (kube_persistentvolumeclaim_status_phase{` + clusterFilter + `,` + nsFilter + `} == 1)`).
					LegendFormat("{{cluster}} {{phase}}"),
				).
				Decimals(0),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("PVC Inode Usage (%)").
				Description("Percentage of filesystem inodes used by each PVC. Inode exhaustion can occur before disk capacity is full.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(100 * (1 - kubelet_volume_stats_inodes_free{` + clusterFilter + `,` + nsFilter + `} / clamp_min(kubelet_volume_stats_inodes{` + clusterFilter + `,` + nsFilter + `}, 1)))`).
					Instant().
					LegendFormat("{{cluster}} {{namespace}}/{{persistentvolumeclaim}}"),
				).
				Decimals(1),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
