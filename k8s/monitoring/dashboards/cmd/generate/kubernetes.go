package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	kubernetesPanelNodesReady                          = 1
	kubernetesPanelNodesNotReady                       = 2
	kubernetesPanelNodesTotal                          = 3
	kubernetesPanelNodePressureConditions              = 4
	kubernetesPanelPodsRunning                         = 5
	kubernetesPanelPodsNotRunning                      = 6
	kubernetesPanelContainerRestarts1h                 = 7
	kubernetesPanelOOMKilledContainers                 = 8
	kubernetesPanelDeploymentsHealthy                  = 9
	kubernetesPanelDeploymentsDegraded                 = 10
	kubernetesPanelDaemonSetsDegraded                  = 11
	kubernetesPanelStatefulSetsDegraded                = 12
	kubernetesPanelCPUUsageByNamespace                 = 13
	kubernetesPanelMemoryUsageByNamespace              = 14
	kubernetesPanelCPURequestedVsActual                = 15
	kubernetesPanelMemoryRequestedVsActual             = 16
	kubernetesPanelContainerResourcesRequestedVsActual = 17
	kubernetesPanelTopCPUThrottledContainers5m         = 18
	kubernetesPanelContainerOOMEvents1h                = 19
	kubernetesPanelPodPhaseCount                       = 20
	kubernetesPanelTopRestartingContainers1h           = 21
	kubernetesPanelNetworkIOByNamespace                = 22
	kubernetesPanelPVCDiskUsage                        = 23
	kubernetesPanelPVCStatus                           = 24
	kubernetesPanelPVCInodeUsage                       = 25
)

// buildKubernetesOverview uses kube-state-metrics and cAdvisor for workload
// and cluster resource health.
func buildKubernetesOverview() (*dashboardv2.Dashboard, error) {
	const (
		nsFilter      = `namespace=~"$namespace"`
		clusterFilter = `cluster=~"$cluster"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()
	issueThresholds := issueThresholds()

	d := dashboardv2.NewDashboardBuilder("Kubernetes Overview").
		Tags([]string{"kubernetes", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-1d").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			Query(promVariableQuery(`label_values(kube_node_info, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true)).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("namespace").
			Label("Namespace").
			Query(promVariableQuery(`label_values(kube_namespace_status_phase{` + clusterFilter + `}, namespace)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	return newLayout(d).
		Row("Cluster Health", statGrid(4)).
		Panel(kubernetesPanelNodesReady, dashboardv2.NewPanelBuilder().
			Title("Nodes Ready").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_node_status_condition{`+clusterFilter+`,condition="Ready",status="true"})`).
				LegendFormat("Ready")))).
		Panel(kubernetesPanelNodesNotReady, dashboardv2.NewPanelBuilder().
			Title("Nodes NotReady").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_node_status_condition{`+clusterFilter+`,condition="Ready",status="true"} == 0) or vector(0)`).
				LegendFormat("NotReady")))).
		Panel(kubernetesPanelNodesTotal, dashboardv2.NewPanelBuilder().
			Title("Nodes Total").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_node_info{`+clusterFilter+`})`).
				LegendFormat("Total")))).
		Panel(kubernetesPanelNodePressureConditions, dashboardv2.NewPanelBuilder().
			Title("Node Pressure Conditions").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// Any active MemoryPressure, DiskPressure, or PIDPressure across all nodes.
				Expr(`count(kube_node_status_condition{`+clusterFilter+`,condition=~"MemoryPressure|DiskPressure|PIDPressure",status="true"} == 1) or vector(0)`).
				LegendFormat("Pressure")))).
		Panel(kubernetesPanelPodsRunning, dashboardv2.NewPanelBuilder().
			Title("Pods Running").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// == 1 filters to the active phase only; other phases exist as 0-valued series.
				Expr(`count(kube_pod_status_phase{`+clusterFilter+`,phase="Running",`+nsFilter+`} == 1) or vector(0)`).
				LegendFormat("Running")))).
		Panel(kubernetesPanelPodsNotRunning, dashboardv2.NewPanelBuilder().
			Title("Pods Not Running").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// Exclude Succeeded (completed Jobs) — only flag genuinely unhealthy phases.
				Expr(`count(kube_pod_status_phase{`+clusterFilter+`,phase!="Running",phase!="Succeeded",`+nsFilter+`} == 1) or vector(0)`).
				LegendFormat("Issues")))).
		Panel(kubernetesPanelContainerRestarts1h, dashboardv2.NewPanelBuilder().
			Title("Container Restarts (1h)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(kube_pod_container_status_restarts_total{`+clusterFilter+`,`+nsFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Restarts")))).
		Panel(kubernetesPanelOOMKilledContainers, dashboardv2.NewPanelBuilder().
			Title("OOMKilled Containers").
			Description("Containers that exist right now and whose last exit was an OOM kill -- a standing state, not a rate, so unlike the (1h) tiles around it there is no window. It clears when the pod is replaced, which in practice is quick: over the last 30 days this read non-zero for one hour out of 720. For OOM activity within a window, use \"Container OOM Events (1h)\" below, which counts cgroup events instead and can legitimately disagree with this.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// Counts containers whose most recent termination reason was OOMKilled.
				Expr(`count(kube_pod_container_status_last_terminated_reason{`+clusterFilter+`,reason="OOMKilled",`+nsFilter+`} == 1) or vector(0)`).
				LegendFormat("OOMKilled")))).
		Panel(kubernetesPanelDeploymentsHealthy, dashboardv2.NewPanelBuilder().
			Title("Deployments Healthy").
			Description("Deployments meant to be running that are. Ones deliberately scaled to zero are not counted, so this plus Degraded is less than the total whenever something is parked.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// Exclude deployments intentionally scaled to zero; 0 available == 0 desired
				// is not a running healthy deployment.
				Expr(`count((kube_deployment_status_replicas_available{`+clusterFilter+`,`+nsFilter+`} == kube_deployment_spec_replicas{`+clusterFilter+`,`+nsFilter+`}) and (kube_deployment_spec_replicas{`+clusterFilter+`,`+nsFilter+`} > 0)) or vector(0)`).
				LegendFormat("Healthy")))).
		Panel(kubernetesPanelDeploymentsDegraded, dashboardv2.NewPanelBuilder().
			Title("Deployments Degraded").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_deployment_status_replicas_available{`+clusterFilter+`,`+nsFilter+`} < kube_deployment_spec_replicas{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Degraded")))).
		Panel(kubernetesPanelDaemonSetsDegraded, dashboardv2.NewPanelBuilder().
			Title("DaemonSets Degraded").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_daemonset_status_number_ready{`+clusterFilter+`,`+nsFilter+`} < kube_daemonset_status_desired_number_scheduled{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Degraded")))).
		Panel(kubernetesPanelStatefulSetsDegraded, dashboardv2.NewPanelBuilder().
			Title("StatefulSets Degraded").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_statefulset_status_replicas_ready{`+clusterFilter+`,`+nsFilter+`} < kube_statefulset_replicas{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Degraded")))).
		Row("Resource Usage", chartGrid()).
		Panel(kubernetesPanelCPUUsageByNamespace, dashboardv2.NewPanelBuilder().
			Title("CPU Usage by Namespace").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// container="" excludes pause containers; pod="" excludes node-level cgroup rollups.
				Expr(`sum by (cluster, namespace) (rate(container_cpu_usage_seconds_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{namespace}}")))).
		Panel(kubernetesPanelMemoryUsageByNamespace, dashboardv2.NewPanelBuilder().
			Title("Memory Usage by Namespace").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, namespace) (container_memory_working_set_bytes{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""})`).
				LegendFormat("{{cluster}} {{namespace}}")))).
		Row("Resource Requests vs Actual", chartGrid()).
		Panel(kubernetesPanelCPURequestedVsActual, dashboardv2.NewPanelBuilder().
			Title("CPU: Requested vs Actual").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("none").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(kube_pod_container_resource_requests{`+clusterFilter+`,resource="cpu",`+nsFilter+`})`).
				LegendFormat("Requested"),
				promQuery().
					Expr(`sum(rate(container_cpu_usage_seconds_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[$__rate_interval]))`).
					LegendFormat("Actual")))).
		Panel(kubernetesPanelMemoryRequestedVsActual, dashboardv2.NewPanelBuilder().
			Title("Memory: Requested vs Actual").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(kube_pod_container_resource_requests{`+clusterFilter+`,resource="memory",`+nsFilter+`})`).
				LegendFormat("Requested"),
				promQuery().
					Expr(`sum(container_memory_working_set_bytes{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""})`).
					LegendFormat("Actual")))).
		Row("Container Resources: Requested vs Actual", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(kubernetesPanelContainerResourcesRequestedVsActual, dashboardv2.NewPanelBuilder().
			Title("Container Resources: Requested vs Actual").
			Visualization(table.NewVisualizationV2Builder().
				OverrideByName("CPU Request", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "none"},
					{Id: "decimals", Value: 3},
				}).
				OverrideByName("CPU Actual", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "none"},
					{Id: "decimals", Value: 3},
				}).
				OverrideByName("Mem Request", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "bytes"},
				}).
				OverrideByName("Mem Actual", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "bytes"},
				})).
			Data(queries(queryWithRef("A", promQuery().
				Expr(`sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="cpu",`+nsFilter+`})`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).RefId("A")),
				queryWithRef("B", promQuery().
					// container!="" excludes pause containers; pod!="" excludes node-level cgroup rollups.
					Expr(`sum by (cluster, namespace, pod, container) (rate(container_cpu_usage_seconds_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[$__rate_interval]))`).
					Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).RefId("B")),
				queryWithRef("C", promQuery().
					Expr(`sum by (cluster, namespace, pod, container) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="memory",`+nsFilter+`})`).
					Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).RefId("C")),
				queryWithRef("D", promQuery().
					Expr(`sum by (cluster, namespace, pod, container) (container_memory_working_set_bytes{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""})`).
					Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).RefId("D"))).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
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
			})))).
		Row("Container Runtime Health", chartGrid()).
		Panel(kubernetesPanelTopCPUThrottledContainers5m, dashboardv2.NewPanelBuilder().
			Title("Top CPU Throttled Containers (5m)").
			Description("Percentage of CPU scheduling periods throttled in the last 5 minutes. Idle containers and zero values are excluded. The window is fixed at 5 minutes and does not follow the dashboard range.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(dashboardv2.NewThresholdsConfigBuilder().
					Mode(dashboardv2.ThresholdsModeAbsolute).
					Steps([]dashboardv2.Threshold{
						{Value: nil, Color: "green"},
						{Value: new(float64(20)), Color: "yellow"},
						{Value: new(float64(50)), Color: "red"},
					})).
				Decimals(1)).
			Data(queries(promQuery().
				// Avoid NaN for idle containers. Keep the named five-minute window fixed;
				// $__rate_interval would change this instant KPI with dashboard zoom.
				Expr(`sort_desc(topk(10, (100 * sum by (cluster, namespace, pod, container) (rate(container_cpu_cfs_throttled_periods_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[5m])) / clamp_min(sum by (cluster, namespace, pod, container) (rate(container_cpu_cfs_periods_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[5m])), 1e-9)) > 0)) or on() label_replace(vector(0), "cluster", "No throttling", "", "")`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}}")))).
		Panel(kubernetesPanelContainerOOMEvents1h, dashboardv2.NewPanelBuilder().
			Title("Container OOM Events (1h)").
			Description("Containers with one or more cgroup OOM events in the last hour.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(issueThresholds).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`sort_desc(sum by (cluster, namespace, pod, container) (increase(container_oom_events_total{`+clusterFilter+`,`+nsFilter+`,container!="",pod!=""}[1h])) > 0) or on() label_replace(vector(0), "cluster", "No OOM events", "", "")`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}}")))).
		Row("Pod Health", chartGrid()).
		Panel(kubernetesPanelPodPhaseCount, dashboardv2.NewPanelBuilder().
			Title("Pod Phase Count").
			Description("Every phase is drawn, including the ones at zero, so a flat line at zero means no pods are Pending or Failed rather than that nobody is looking.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Sum phase gauges so empty phases remain visible at zero. Filtering ==1
				// removes those series entirely.
				Expr(`sum by (cluster, phase) (kube_pod_status_phase{`+clusterFilter+`,`+nsFilter+`})`).
				LegendFormat("{{cluster}} {{phase}}")))).
		Panel(kubernetesPanelTopRestartingContainers1h, dashboardv2.NewPanelBuilder().
			Title("Top Restarting Containers (1h)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Decimals(0)).
			Data(queries(promQuery().
				// Exclude zero restarts; sort without topk's per-step instability.
				Expr(`sort_desc(sum by (cluster, namespace, pod, container) (increase(kube_pod_container_status_restarts_total{`+clusterFilter+`,`+nsFilter+`}[1h])) > 0) or on() label_replace(vector(0), "cluster", "No restarts", "", "")`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}}")))).
		Row("Network", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(kubernetesPanelNetworkIOByNamespace, dashboardv2.NewPanelBuilder().
			Title("Network I/O by Namespace").
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
				// pod!="" excludes node-level aggregates.
				Expr(`sum by (cluster, namespace) (rate(container_network_receive_bytes_total{`+clusterFilter+`,`+nsFilter+`,pod!=""}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{namespace}} Rx")),
				queryWithRef("Tx", promQuery().
					RefId("Tx").
					Expr(`sum by (cluster, namespace) (rate(container_network_transmit_bytes_total{`+clusterFilter+`,`+nsFilter+`,pod!=""}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{namespace}} Tx"))))).
		Row("Storage", chartGrid()).
		Panel(kubernetesPanelPVCDiskUsage, dashboardv2.NewPanelBuilder().
			Title("PVC Disk Usage (%)").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc(kubelet_volume_stats_used_bytes{`+clusterFilter+`,`+nsFilter+`} / kubelet_volume_stats_capacity_bytes{`+clusterFilter+`,`+nsFilter+`} * 100)`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}/{{persistentvolumeclaim}}")))).
		Panel(kubernetesPanelPVCStatus, dashboardv2.NewPanelBuilder().
			Title("PVC Status").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`count by (cluster, phase) (kube_persistentvolumeclaim_status_phase{`+clusterFilter+`,`+nsFilter+`} == 1)`).
				LegendFormat("{{cluster}} {{phase}}")))).
		Row("PVC Inode Usage (%)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(kubernetesPanelPVCInodeUsage, dashboardv2.NewPanelBuilder().
			Title("PVC Inode Usage (%)").
			Description("Percentage of filesystem inodes used by each PVC. Inode exhaustion can occur before disk capacity is full.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds()).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc(100 * (1 - kubelet_volume_stats_inodes_free{`+clusterFilter+`,`+nsFilter+`} / clamp_min(kubelet_volume_stats_inodes{`+clusterFilter+`,`+nsFilter+`}, 1)))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}/{{persistentvolumeclaim}}")))).
		Build()
}
