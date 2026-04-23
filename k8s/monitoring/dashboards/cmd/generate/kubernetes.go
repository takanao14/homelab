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

	const nsFilter = `namespace=~"$namespace"`

	issueThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("Kubernetes Overview").
		Uid("kubernetes-overview").
		Tags([]string{"kubernetes", "infrastructure"}).
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
			dashboard.NewQueryVariableBuilder("namespace").
				Label("Namespace").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(kube_namespace_status_phase, namespace)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).

		// Row 1: Core workload health
		WithPanel(
			stat.NewPanelBuilder().
				Title("Nodes Ready").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_node_status_condition{condition="Ready",status="true"})`).
					LegendFormat("Ready"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Pods Running").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					// == 1 filters to the active phase only; other phases exist as 0-valued series.
					Expr(`count(kube_pod_status_phase{phase="Running",` + nsFilter + `} == 1) or vector(0)`).
					LegendFormat("Running"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Pods Not Running").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Exclude Succeeded (completed Jobs) — only flag genuinely unhealthy phases.
					Expr(`count(kube_pod_status_phase{phase!="Running",phase!="Succeeded",` + nsFilter + `} == 1) or vector(0)`).
					LegendFormat("Issues"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Deployments Healthy").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_deployment_status_replicas_available == kube_deployment_spec_replicas) or vector(0)`).
					LegendFormat("Healthy"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Deployments Degraded").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_deployment_status_replicas_available < kube_deployment_spec_replicas) or vector(0)`).
					LegendFormat("Degraded"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Container Restarts (1h)").
				Datasource(ds).
				Span(4).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(kube_pod_container_status_restarts_total{` + nsFilter + `}[1h])))`).
					LegendFormat("Restarts"),
				),
		).

		// Row 2: Extended workload + node health
		WithPanel(
			stat.NewPanelBuilder().
				Title("DaemonSets Degraded").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_daemonset_status_number_ready < kube_daemonset_status_desired_number_scheduled) or vector(0)`).
					LegendFormat("Degraded"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("StatefulSets Degraded").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_statefulset_status_replicas_ready < kube_statefulset_replicas) or vector(0)`).
					LegendFormat("Degraded"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("OOMKilled Containers").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Counts containers whose most recent termination reason was OOMKilled.
					Expr(`count(kube_pod_container_status_last_terminated_reason{reason="OOMKilled",` + nsFilter + `} == 1) or vector(0)`).
					LegendFormat("OOMKilled"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node Pressure Conditions").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Any active MemoryPressure, DiskPressure, or PIDPressure across all nodes.
					Expr(`count(kube_node_status_condition{condition=~"MemoryPressure|DiskPressure|PIDPressure",status="true"} == 1) or vector(0)`).
					LegendFormat("Pressure"),
				),
		).

		// Row 3: CPU and memory actual usage
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage by Namespace").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					// container="" excludes pause containers; pod="" excludes node-level cgroup rollups.
					Expr(`sum by (namespace) (rate(container_cpu_usage_seconds_total{` + nsFilter + `,container!="",pod!=""}[5m]))`).
					LegendFormat("{{namespace}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory Usage by Namespace").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (namespace) (container_memory_working_set_bytes{` + nsFilter + `,container!="",pod!=""})`).
					LegendFormat("{{namespace}}"),
				),
		).

		// Row 4: Request vs actual (efficiency view)
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU: Requested vs Actual").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_pod_container_resource_requests{resource="cpu",` + nsFilter + `})`).
					LegendFormat("Requested"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(container_cpu_usage_seconds_total{` + nsFilter + `,container!="",pod!=""}[5m]))`).
					LegendFormat("Actual"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory: Requested vs Actual").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_pod_container_resource_requests{resource="memory",` + nsFilter + `})`).
					LegendFormat("Requested"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(container_memory_working_set_bytes{` + nsFilter + `,container!="",pod!=""})`).
					LegendFormat("Actual"),
				),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("Container Resources: Requested vs Actual").
				Datasource(ds).
				Span(24).Height(10).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (namespace, pod, container) (kube_pod_container_resource_requests{resource="cpu",`+nsFilter+`})`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("A"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// container!="" excludes pause containers; pod!="" excludes node-level cgroup rollups.
					Expr(`sum by (namespace, pod, container) (rate(container_cpu_usage_seconds_total{`+nsFilter+`,container!="",pod!=""}[5m]))`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("B"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (namespace, pod, container) (kube_pod_container_resource_requests{resource="memory",`+nsFilter+`})`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("C"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (namespace, pod, container) (container_memory_working_set_bytes{`+nsFilter+`,container!="",pod!=""})`).
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
						"renameByName": map[string]any{
							"Value #A": "CPU Request",
							"Value #B": "CPU Actual",
							"Value #C": "Mem Request",
							"Value #D": "Mem Actual",
						},
					},
				}).
				OverrideByName("CPU Request", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 3},
				}).
				OverrideByName("CPU Actual", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 3},
				}).
				OverrideByName("Mem Request", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "bytes"},
				}).
				OverrideByName("Mem Actual", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "bytes"},
				}),
		).

		// Row 5: Pod health over time + restart hotspots
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Pod Phase Count").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (phase) (kube_pod_status_phase{` + nsFilter + `} == 1)`).
					LegendFormat("{{phase}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Container Restarts (1h)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					// > 0 excludes containers with no restarts; sort_desc orders by count without topk's per-step instability.
					Expr(`sort_desc(sum by (namespace, pod, container) (increase(kube_pod_container_status_restarts_total{` + nsFilter + `}[1h])) > 0)`).
					LegendFormat("{{namespace}}/{{pod}}/{{container}}"),
				).
				Decimals(0),
		).

		// Row 6: Network I/O
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network Receive by Namespace").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					// pod!="" excludes node-level aggregates.
					Expr(`sum by (namespace) (rate(container_network_receive_bytes_total{` + nsFilter + `,pod!=""}[5m]))`).
					LegendFormat("{{namespace}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network Transmit by Namespace").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (namespace) (rate(container_network_transmit_bytes_total{` + nsFilter + `,pod!=""}[5m]))`).
					LegendFormat("{{namespace}}"),
				),
		).

		// Row 7: Storage
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("PVC Disk Usage (%)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(kubelet_volume_stats_used_bytes{` + nsFilter + `} / kubelet_volume_stats_capacity_bytes{` + nsFilter + `} * 100)`).
					LegendFormat("{{namespace}}/{{persistentvolumeclaim}}"),
				).
				Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("PVC Status").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Orientation(common.VizOrientationHorizontal).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (phase) (kube_persistentvolumeclaim_status_phase == 1)`).
					LegendFormat("{{phase}}"),
				).
				Decimals(0),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
