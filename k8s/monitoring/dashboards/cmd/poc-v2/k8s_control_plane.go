package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/gauge"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	k8sControlPlanePanelAPI5xx5m                         = 1
	k8sControlPlanePanelAPI4295m                         = 2
	k8sControlPlanePanelAPFQueued                        = 3
	k8sControlPlanePanelDNSSERVFAIL5m                    = 4
	k8sControlPlanePanelUnschedulablePods                = 5
	k8sControlPlanePanelFailedJobsCurrent                = 6
	k8sControlPlanePanelCordonedNodesCurrent             = 7
	k8sControlPlanePanelNodePressureConditions           = 8
	k8sControlPlanePanelPVNotBound                       = 9
	k8sControlPlanePanelAPIRequestRateByCode             = 10
	k8sControlPlanePanelAPIRequestLatencyP99ByVerb       = 11
	k8sControlPlanePanelAPIInflightRequestsByKind        = 12
	k8sControlPlanePanelAPFQueuedRequestsByPriority      = 13
	k8sControlPlanePanelAPFExecutingRequestsByPriority   = 14
	k8sControlPlanePanelAPFRejectedRequests              = 15
	k8sControlPlanePanelAPIStorageErrors                 = 16
	k8sControlPlanePanelEtcdRequestLatencyP99ByOperation = 17
	k8sControlPlanePanelEtcdRequestRateByOperation       = 18
	k8sControlPlanePanelEtcdRequestErrors                = 19
	k8sControlPlanePanelActiveWatchRequests              = 20
	k8sControlPlanePanelEtcdObjectCountByResourceTop20   = 21
	k8sControlPlanePanelCoreDNSPanics1h                  = 22
	k8sControlPlanePanelCoreDNSForwardFailures5m         = 23
	k8sControlPlanePanelCoreDNSReloadFailures1h          = 24
	k8sControlPlanePanelDNSResponseRateByRCODE           = 25
	k8sControlPlanePanelDNSRequestLatencyP99             = 26
	k8sControlPlanePanelCoreDNSCacheHitRatio             = 27
	k8sControlPlanePanelCoreDNSUpstreamProxyLatencyP99   = 28
	k8sControlPlanePanelUnschedulablePodsByNamespace     = 29
	k8sControlPlanePanelCordonedNodes                    = 30
	k8sControlPlanePanelContainerWaitingReasons          = 31
	k8sControlPlanePanelFailedJobs                       = 32
	k8sControlPlanePanelSchedulingAttemptsByResult       = 33
	k8sControlPlanePanelPendingPodsByQueue               = 34
	k8sControlPlanePanelSchedulingAttemptLatencyP99      = 35
	k8sControlPlanePanelWorkqueueDepth                   = 36
	k8sControlPlanePanelWorkqueueWorkDurationP99         = 37
	k8sControlPlanePanelWorkqueueRetries                 = 38
	k8sControlPlanePanelCPURequestedVsAllocatable        = 39
	k8sControlPlanePanelMemoryRequestedVsAllocatable     = 40
	k8sControlPlanePanelPodsUsedVsAllocatable            = 41
)

// buildK8sControlPlaneOverview complements workload views with control-plane
// and DNS health. Trends use $__rate_interval; named 5m/1h KPIs stay fixed.
func buildK8sControlPlaneOverview() (*dashboardv2.Dashboard, error) {
	const (
		clusterFilter = `cluster=~"$cluster"`
		nsFilter      = `namespace=~"$namespace"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	capacityThresholds := capacityThresholds()

	d := dashboardv2.NewDashboardBuilder("K8s Control Plane Overview").
		Tags([]string{"kubernetes", "control-plane", "dns", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("30s")).
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
		// Nine zero-is-healthy counters form three concern-based rows of three.
		Row("Summary", statGrid(4)).
		Panel(k8sControlPlanePanelAPI5xx5m, dashboardv2.NewPanelBuilder().
			Title("API 5xx (5m)").
			Description("API server responses with 5xx status codes in the last 5 minutes.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(apiserver_request_total{`+clusterFilter+`,code=~"5.."}[5m]))) or vector(0)`).
				LegendFormat("5xx")))).
		Panel(k8sControlPlanePanelAPI4295m, dashboardv2.NewPanelBuilder().
			Title("API 429 (5m)").
			Description("HTTP 429 responses in the last 5 minutes. 429 indicates API priority and fairness throttling or overload.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(apiserver_request_total{`+clusterFilter+`,code="429"}[5m]))) or vector(0)`).
				LegendFormat("429")))).
		Panel(k8sControlPlanePanelAPFQueued, dashboardv2.NewPanelBuilder().
			Title("APF Queued").
			Description("Current requests pending in API Priority and Fairness queues.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(apiserver_flowcontrol_current_inqueue_requests{`+clusterFilter+`}) or vector(0)`).
				LegendFormat("Queued")))).
		Panel(k8sControlPlanePanelDNSSERVFAIL5m, dashboardv2.NewPanelBuilder().
			Title("DNS SERVFAIL (5m)").
			Description("CoreDNS SERVFAIL responses in the last 5 minutes.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(coredns_dns_responses_total{`+clusterFilter+`,rcode="SERVFAIL"}[5m]))) or vector(0)`).
				LegendFormat("SERVFAIL")))).
		Panel(k8sControlPlanePanelUnschedulablePods, dashboardv2.NewPanelBuilder().
			Title("Unschedulable Pods").
			Description("Pods currently marked unschedulable.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(kube_pod_status_unschedulable{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Unschedulable")))).
		Panel(k8sControlPlanePanelFailedJobsCurrent, dashboardv2.NewPanelBuilder().
			Title("Failed Jobs (Current)").
			Description("Jobs with failed pods in the selected namespaces.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(kube_job_status_failed{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Failed")))).
		Panel(k8sControlPlanePanelCordonedNodesCurrent, dashboardv2.NewPanelBuilder().
			Title("Cordoned Nodes (Current)").
			Description("Nodes marked unschedulable.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(kube_node_spec_unschedulable{`+clusterFilter+`}) or vector(0)`).
				LegendFormat("Cordoned")))).
		Panel(k8sControlPlanePanelNodePressureConditions, dashboardv2.NewPanelBuilder().
			Title("Node Pressure Conditions").
			Description("Nodes currently reporting MemoryPressure, DiskPressure, PIDPressure, or NetworkUnavailable.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// status="true" only selects the "true" series; == 1 checks it's actually asserted.
				Expr(`count(kube_node_status_condition{`+clusterFilter+`,condition=~"MemoryPressure|DiskPressure|PIDPressure|NetworkUnavailable",status="true"} == 1) or vector(0)`).
				LegendFormat("Under pressure")))).
		Panel(k8sControlPlanePanelPVNotBound, dashboardv2.NewPanelBuilder().
			Title("PV Not Bound").
			Description("PersistentVolumes not in the Bound phase.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(kube_persistentvolume_status_phase{`+clusterFilter+`,phase!="Bound"} == 1) or vector(0)`).
				LegendFormat("Not bound")))).
		Row("API Server", chartGrid()).
		Panel(k8sControlPlanePanelAPIRequestRateByCode, dashboardv2.NewPanelBuilder().
			Title("API Request Rate by Code").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, code) (rate(apiserver_request_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{code}}")))).
		Panel(k8sControlPlanePanelAPIRequestLatencyP99ByVerb, dashboardv2.NewPanelBuilder().
			Title("API Request Latency p99 by Verb").
			Description("99th percentile API request latency by verb. Long-running WATCH requests are excluded.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le, verb) (rate(apiserver_request_duration_seconds_bucket{`+clusterFilter+`,subresource="",verb!="WATCH"}[$__rate_interval]))) and on (cluster, verb) sum by (cluster, verb) (rate(apiserver_request_duration_seconds_count{`+clusterFilter+`,subresource="",verb!="WATCH"}[$__rate_interval])) > 0`).
				LegendFormat("{{cluster}} {{verb}}")))).
		Row("API Inflight Requests by Kind", chartGrid()).
		Panel(k8sControlPlanePanelAPIInflightRequestsByKind, dashboardv2.NewPanelBuilder().
			Title("API Inflight Requests by Kind").
			Description("Current API requests actively being handled, split by read-only and mutating requests.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, request_kind) (apiserver_current_inflight_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{request_kind}}")))).
		Panel(k8sControlPlanePanelAPFQueuedRequestsByPriority, dashboardv2.NewPanelBuilder().
			Title("APF Queued Requests by Priority").
			Description("Current requests pending in API Priority and Fairness queues.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, priority_level) (apiserver_flowcontrol_current_inqueue_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{priority_level}}")))).
		Row("APF Executing Requests by Priority", chartGrid()).
		Panel(k8sControlPlanePanelAPFExecutingRequestsByPriority, dashboardv2.NewPanelBuilder().
			Title("APF Executing Requests by Priority").
			Description("Current requests executing under API Priority and Fairness.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, priority_level) (apiserver_flowcontrol_current_executing_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{priority_level}}")))).
		Panel(k8sControlPlanePanelAPFRejectedRequests, dashboardv2.NewPanelBuilder().
			Title("APF Rejected Requests").
			Description("API Priority and Fairness rejected requests. Nonzero values indicate overload or queue shedding.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, flow_schema, priority_level) (rate(apiserver_flowcontrol_rejected_requests_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{priority_level}} {{flow_schema}}")))).
		Row("API Storage Errors", chartGrid()).
		Panel(k8sControlPlanePanelAPIStorageErrors, dashboardv2.NewPanelBuilder().
			Title("API Storage Errors").
			Description("Storage decode and data key generation errors. Nonzero values deserve investigation.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (rate(apiserver_storage_decode_errors_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} decode errors"),
				promQuery().
					Expr(`sum by (cluster) (rate(apiserver_storage_data_key_generation_failures_total{`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} key generation failures")))).
		Row("etcd / Storage", chartGrid()).
		Panel(k8sControlPlanePanelEtcdRequestLatencyP99ByOperation, dashboardv2.NewPanelBuilder().
			Title("etcd Request Latency p99 by Operation").
			Description("99th percentile latency of API server requests to the etcd/kine backend, by operation. The backend is the usual root cause of control-plane slowness.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le, operation) (rate(etcd_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} {{operation}}")))).
		Panel(k8sControlPlanePanelEtcdRequestRateByOperation, dashboardv2.NewPanelBuilder().
			Title("etcd Request Rate by Operation").
			Description("Rate of API server requests to the etcd/kine backend, by operation.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, operation) (rate(etcd_request_duration_seconds_count{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{operation}}")))).
		Row("etcd Request Errors", chartGrid()).
		Panel(k8sControlPlanePanelEtcdRequestErrors, dashboardv2.NewPanelBuilder().
			Title("etcd Request Errors").
			Description("Errors returned by the etcd/kine backend. Nonzero values deserve investigation.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, operation) (rate(etcd_request_errors_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{operation}}")))).
		Panel(k8sControlPlanePanelActiveWatchRequests, dashboardv2.NewPanelBuilder().
			Title("Active Watch Requests").
			Description("Long-running (watch) requests currently held open against the API server. A steady climb can indicate a watch leak in a controller or client.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (apiserver_longrunning_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}}")))).
		Row("etcd Object Count by Resource (Top 20)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sControlPlanePanelEtcdObjectCountByResourceTop20, dashboardv2.NewPanelBuilder().
			Title("etcd Object Count by Resource (Top 20)").
			Description("Number of objects stored in etcd/kine per resource type. Helps spot unexpected growth that bloats the backend.").
			Visualization(table.NewVisualizationV2Builder().
				OverrideByName("Objects", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 0},
				})).
			Data(queries(promQuery().
				Expr(`sort_desc(topk(20, sum by (cluster, resource) (apiserver_storage_objects{`+clusterFilter+`})))`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{resource}}")).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
				"excludeByName": map[string]any{"Time": true},
				"indexByName": map[string]any{
					"cluster":  0,
					"resource": 1,
					"Value":    2,
				},
				"renameByName": map[string]any{
					"Value": "Objects",
				},
			})))).
		Row("CoreDNS", statGrid(3)).
		Panel(k8sControlPlanePanelCoreDNSPanics1h, dashboardv2.NewPanelBuilder().
			Title("CoreDNS Panics (1h)").
			Description("CoreDNS process panics in the last hour.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(coredns_panics_total{`+clusterFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Panics")))).
		Panel(k8sControlPlanePanelCoreDNSForwardFailures5m, dashboardv2.NewPanelBuilder().
			Title("CoreDNS Forward Failures (5m)").
			Description("Upstream DNS forwarder healthcheck failures in the last 5 minutes.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(coredns_forward_healthcheck_broken_total{`+clusterFilter+`}[5m]))) or vector(0)`).
				LegendFormat("Failures")))).
		Panel(k8sControlPlanePanelCoreDNSReloadFailures1h, dashboardv2.NewPanelBuilder().
			Title("CoreDNS Reload Failures (1h)").
			Description("CoreDNS reload failures in the last hour.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`ceil(sum(increase(coredns_reload_failed_total{`+clusterFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Reload failures")))).
		Row("DNS Response Rate by RCODE", chartGrid()).
		Panel(k8sControlPlanePanelDNSResponseRateByRCODE, dashboardv2.NewPanelBuilder().
			Title("DNS Response Rate by RCODE").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, rcode) (rate(coredns_dns_responses_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{rcode}}")))).
		Panel(k8sControlPlanePanelDNSRequestLatencyP99, dashboardv2.NewPanelBuilder().
			Title("DNS Request Latency p99").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99")))).
		Row("CoreDNS Cache Hit Ratio", chartGrid()).
		Panel(k8sControlPlanePanelCoreDNSCacheHitRatio, dashboardv2.NewPanelBuilder().
			Title("CoreDNS Cache Hit Ratio").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Max(1).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (rate(coredns_cache_hits_total{`+clusterFilter+`}[$__rate_interval])) / clamp_min(sum by (cluster) (rate(coredns_cache_requests_total{`+clusterFilter+`}[$__rate_interval])), 1e-9)`).
				LegendFormat("{{cluster}} hit ratio")))).
		Panel(k8sControlPlanePanelCoreDNSUpstreamProxyLatencyP99, dashboardv2.NewPanelBuilder().
			Title("CoreDNS Upstream Proxy Latency p99").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le, proxy_name, to) (rate(coredns_proxy_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} {{proxy_name}} {{to}}")))).
		Row("Scheduling", chartGrid()).
		Panel(k8sControlPlanePanelUnschedulablePodsByNamespace, dashboardv2.NewPanelBuilder().
			Title("Unschedulable Pods by Namespace").
			Description("Pods currently marked unschedulable, grouped by namespace. Empty is good: kube-state-metrics only emits this series while a pod is actually unschedulable, so nothing to show means nothing is stuck.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(issueThresholds).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`sort_desc(sum by (cluster, namespace) (kube_pod_status_unschedulable{`+clusterFilter+`,`+nsFilter+`}))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}")))).
		Panel(k8sControlPlanePanelCordonedNodes, dashboardv2.NewPanelBuilder().
			Title("Cordoned Nodes").
			Description("Nodes marked unschedulable.").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(issueThresholds).
				Decimals(0)).
			Data(queries(promQuery().
				Expr(`sort_desc(kube_node_spec_unschedulable{`+clusterFilter+`} > 0)`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{node}}")))).
		Row("Container Waiting Reasons", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sControlPlanePanelContainerWaitingReasons, dashboardv2.NewPanelBuilder().
			Title("Container Waiting Reasons").
			Description("Containers currently waiting, grouped by reason. Empty is good: the reason label only exists while a container is waiting, so an empty table means every container is past that state.").
			Visualization(table.NewVisualizationV2Builder().
				OverrideByName("Containers", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 0},
				})).
			Data(queries(promQuery().
				Expr(`sum by (cluster, namespace, pod, container, reason) (kube_pod_container_status_waiting_reason{`+clusterFilter+`,`+nsFilter+`} == 1)`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}} {{reason}}")).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
				"excludeByName": map[string]any{"Time": true},
				"indexByName": map[string]any{
					"cluster":   0,
					"namespace": 1,
					"pod":       2,
					"container": 3,
					"reason":    4,
					"Value":     5,
				},
				"renameByName": map[string]any{
					"Value": "Containers",
				},
			})))).
		Row("Failed Jobs", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(k8sControlPlanePanelFailedJobs, dashboardv2.NewPanelBuilder().
			Title("Failed Jobs").
			Description("Jobs with one or more failed pods. Empty is good, and stays empty while the cluster runs no Jobs at all -- kube_job_* series exist only per Job object.").
			Visualization(table.NewVisualizationV2Builder().
				OverrideByName("Failed Pods", []dashboardv2.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 0},
				})).
			Data(queries(promQuery().
				Expr(`sort_desc(kube_job_status_failed{`+clusterFilter+`,`+nsFilter+`} > 0)`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{namespace}}/{{job_name}}")).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
				"excludeByName": map[string]any{"Time": true},
				"indexByName": map[string]any{
					"cluster":   0,
					"namespace": 1,
					"job_name":  2,
					"Value":     3,
				},
				"renameByName": map[string]any{
					"Value": "Failed Pods",
				},
			})))).
		Row("Scheduler", chartGrid()).
		Panel(k8sControlPlanePanelSchedulingAttemptsByResult, dashboardv2.NewPanelBuilder().
			Title("Scheduling Attempts by Result").
			Description("Rate of pod scheduling attempts by outcome. 'unschedulable' or 'error' indicate the scheduler cannot place pods.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, result) (rate(scheduler_schedule_attempts_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{result}}")))).
		Panel(k8sControlPlanePanelPendingPodsByQueue, dashboardv2.NewPanelBuilder().
			Title("Pending Pods by Queue").
			Description("Pods waiting to be scheduled, split by scheduler queue (active, backoff, unschedulable, gated). A sustained backlog signals scheduling problems.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, queue) (scheduler_pending_pods{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{queue}}")))).
		Row("Scheduling Attempt Latency p99", chartGrid()).
		Panel(k8sControlPlanePanelSchedulingAttemptLatencyP99, dashboardv2.NewPanelBuilder().
			Title("Scheduling Attempt Latency p99").
			Description("99th percentile end-to-end scheduling attempt duration.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(scheduler_scheduling_attempt_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99")))).
		Row("Controller Manager", chartGrid()).
		Panel(k8sControlPlanePanelWorkqueueDepth, dashboardv2.NewPanelBuilder().
			Title("Workqueue Depth").
			Description("Total controller-manager workqueue depth. A sustained rise means controllers are falling behind on reconciliation.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (workqueue_depth{`+clusterFilter+`,job="kube-controller-manager"})`).
				LegendFormat("{{cluster}}")))).
		Panel(k8sControlPlanePanelWorkqueueWorkDurationP99, dashboardv2.NewPanelBuilder().
			Title("Workqueue Work Duration p99").
			Description("99th percentile time controllers spend processing a single workqueue item.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(workqueue_work_duration_seconds_bucket{`+clusterFilter+`,job="kube-controller-manager"}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99")))).
		Row("Workqueue Retries", chartGrid()).
		Panel(k8sControlPlanePanelWorkqueueRetries, dashboardv2.NewPanelBuilder().
			Title("Workqueue Retries").
			Description("Rate of workqueue item retries. Elevated retries indicate controllers repeatedly failing to reconcile.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (rate(workqueue_retries_total{`+clusterFilter+`,job="kube-controller-manager"}[$__rate_interval]))`).
				LegendFormat("{{cluster}}")))).
		// Use radial gauges for single bounded cluster percentages; bar gauges are
		// reserved for ranked multi-item panels.
		Row("Capacity", chartGrid()).
		Panel(k8sControlPlanePanelCPURequestedVsAllocatable, dashboardv2.NewPanelBuilder().
			Title("CPU Requested vs Allocatable").
			Description("Total CPU requests as a percentage of cluster allocatable CPU.").
			Visualization(gauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds).
				ShowThresholdMarkers(true).
				ShowThresholdLabels(false).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc(100 * sum by (cluster) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="cpu",`+nsFilter+`}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{`+clusterFilter+`,resource="cpu"}), 1e-9))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))).
		Panel(k8sControlPlanePanelMemoryRequestedVsAllocatable, dashboardv2.NewPanelBuilder().
			Title("Memory Requested vs Allocatable").
			Description("Total memory requests as a percentage of cluster allocatable memory.").
			Visualization(gauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds).
				ShowThresholdMarkers(true).
				ShowThresholdLabels(false).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc(100 * sum by (cluster) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="memory",`+nsFilter+`}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{`+clusterFilter+`,resource="memory"}), 1e-9))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))).
		Row("Pods Used vs Allocatable", chartGrid()).
		Panel(k8sControlPlanePanelPodsUsedVsAllocatable, dashboardv2.NewPanelBuilder().
			Title("Pods Used vs Allocatable").
			Description("Running pods as a percentage of cluster allocatable pod capacity.").
			Visualization(gauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Max(100).
				Thresholds(capacityThresholds).
				ShowThresholdMarkers(true).
				ShowThresholdLabels(false).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`sort_desc(100 * sum by (cluster) (kubelet_running_pods{`+clusterFilter+`}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{`+clusterFilter+`,resource="pods"}), 1e-9))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))).
		Build()
}
