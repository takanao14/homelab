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
		elementAPI5xx5m                         = "api-5xx-5m"
		elementAPI4295m                         = "api-429-5m"
		elementAPFQueued                        = "apf-queued"
		elementDNSSERVFAIL5m                    = "dns-servfail-5m"
		elementUnschedulablePods                = "unschedulable-pods"
		elementFailedJobsCurrent                = "failed-jobs-current"
		elementCordonedNodesCurrent             = "cordoned-nodes-current"
		elementNodePressureConditions           = "node-pressure-conditions"
		elementPVNotBound                       = "pv-not-bound"
		elementAPIRequestRateByCode             = "api-request-rate-by-code"
		elementAPIRequestLatencyP99ByVerb       = "api-request-latency-p99-by-verb"
		elementAPIInflightRequestsByKind        = "api-inflight-requests-by-kind"
		elementAPFQueuedRequestsByPriority      = "apf-queued-requests-by-priority"
		elementAPFExecutingRequestsByPriority   = "apf-executing-requests-by-priority"
		elementAPFRejectedRequests              = "apf-rejected-requests"
		elementAPIStorageErrors                 = "api-storage-errors"
		elementEtcdRequestLatencyP99ByOperation = "etcd-request-latency-p99-by-operation"
		elementEtcdRequestRateByOperation       = "etcd-request-rate-by-operation"
		elementEtcdRequestErrors                = "etcd-request-errors"
		elementActiveWatchRequests              = "active-watch-requests"
		elementEtcdObjectCountByResourceTop20   = "etcd-object-count-by-resource-top-20"
		elementCoreDNSPanics1h                  = "coredns-panics-1h"
		elementCoreDNSForwardFailures5m         = "coredns-forward-failures-5m"
		elementCoreDNSReloadFailures1h          = "coredns-reload-failures-1h"
		elementDNSResponseRateByRCODE           = "dns-response-rate-by-rcode"
		elementDNSRequestLatencyP99             = "dns-request-latency-p99"
		elementCoreDNSCacheHitRatio             = "coredns-cache-hit-ratio"
		elementCoreDNSUpstreamProxyLatencyP99   = "coredns-upstream-proxy-latency-p99"
		elementUnschedulablePodsByNamespace     = "unschedulable-pods-by-namespace"
		elementCordonedNodes                    = "cordoned-nodes"
		elementContainerWaitingReasons          = "container-waiting-reasons"
		elementFailedJobs                       = "failed-jobs"
		elementSchedulingAttemptsByResult       = "scheduling-attempts-by-result"
		elementPendingPodsByQueue               = "pending-pods-by-queue"
		elementSchedulingAttemptLatencyP99      = "scheduling-attempt-latency-p99"
		elementWorkqueueDepth                   = "workqueue-depth"
		elementWorkqueueWorkDurationP99         = "workqueue-work-duration-p99"
		elementWorkqueueRetries                 = "workqueue-retries"
		elementCPURequestedVsAllocatable        = "cpu-requested-vs-allocatable"
		elementMemoryRequestedVsAllocatable     = "memory-requested-vs-allocatable"
		elementPodsUsedVsAllocatable            = "pods-used-vs-allocatable"
	)

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

	d.Panel(elementAPI5xx5m, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPI5xx5m).
		Title("API 5xx (5m)").
		Description("API server responses with 5xx status codes in the last 5 minutes.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(apiserver_request_total{`+clusterFilter+`,code=~"5.."}[5m]))) or vector(0)`).
				LegendFormat("5xx")))))

	d.Panel(elementAPI4295m, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPI4295m).
		Title("API 429 (5m)").
		Description("HTTP 429 responses in the last 5 minutes. 429 indicates API priority and fairness throttling or overload.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(apiserver_request_total{`+clusterFilter+`,code="429"}[5m]))) or vector(0)`).
				LegendFormat("429")))))

	d.Panel(elementAPFQueued, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPFQueued).
		Title("APF Queued").
		Description("Current requests pending in API Priority and Fairness queues.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum(apiserver_flowcontrol_current_inqueue_requests{`+clusterFilter+`}) or vector(0)`).
				LegendFormat("Queued")))))

	d.Panel(elementDNSSERVFAIL5m, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelDNSSERVFAIL5m).
		Title("DNS SERVFAIL (5m)").
		Description("CoreDNS SERVFAIL responses in the last 5 minutes.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(coredns_dns_responses_total{`+clusterFilter+`,rcode="SERVFAIL"}[5m]))) or vector(0)`).
				LegendFormat("SERVFAIL")))))

	d.Panel(elementUnschedulablePods, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelUnschedulablePods).
		Title("Unschedulable Pods").
		Description("Pods currently marked unschedulable.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum(kube_pod_status_unschedulable{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Unschedulable")))))

	d.Panel(elementFailedJobsCurrent, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelFailedJobsCurrent).
		Title("Failed Jobs (Current)").
		Description("Jobs with failed pods in the selected namespaces.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum(kube_job_status_failed{`+clusterFilter+`,`+nsFilter+`}) or vector(0)`).
				LegendFormat("Failed")))))

	d.Panel(elementCordonedNodesCurrent, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCordonedNodesCurrent).
		Title("Cordoned Nodes (Current)").
		Description("Nodes marked unschedulable.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum(kube_node_spec_unschedulable{`+clusterFilter+`}) or vector(0)`).
				LegendFormat("Cordoned")))))

	d.Panel(elementNodePressureConditions, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelNodePressureConditions).
		Title("Node Pressure Conditions").
		Description("Nodes currently reporting MemoryPressure, DiskPressure, PIDPressure, or NetworkUnavailable.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// status="true" only selects the "true" series; == 1 checks it's actually asserted.
				Expr(`count(kube_node_status_condition{`+clusterFilter+`,condition=~"MemoryPressure|DiskPressure|PIDPressure|NetworkUnavailable",status="true"} == 1) or vector(0)`).
				LegendFormat("Under pressure")))))

	d.Panel(elementPVNotBound, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelPVNotBound).
		Title("PV Not Bound").
		Description("PersistentVolumes not in the Bound phase.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(kube_persistentvolume_status_phase{`+clusterFilter+`,phase!="Bound"} == 1) or vector(0)`).
				LegendFormat("Not bound")))))

	d.Panel(elementAPIRequestRateByCode, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPIRequestRateByCode).
		Title("API Request Rate by Code").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, code) (rate(apiserver_request_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{code}}")))))

	d.Panel(elementAPIRequestLatencyP99ByVerb, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPIRequestLatencyP99ByVerb).
		Title("API Request Latency p99 by Verb").
		Description("99th percentile API request latency by verb. Long-running WATCH requests are excluded.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le, verb) (rate(apiserver_request_duration_seconds_bucket{`+clusterFilter+`,subresource="",verb!="WATCH"}[$__rate_interval]))) and on (cluster, verb) sum by (cluster, verb) (rate(apiserver_request_duration_seconds_count{`+clusterFilter+`,subresource="",verb!="WATCH"}[$__rate_interval])) > 0`).
				LegendFormat("{{cluster}} {{verb}}")))))

	d.Panel(elementAPIInflightRequestsByKind, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPIInflightRequestsByKind).
		Title("API Inflight Requests by Kind").
		Description("Current API requests actively being handled, split by read-only and mutating requests.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, request_kind) (apiserver_current_inflight_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{request_kind}}")))))

	d.Panel(elementAPFQueuedRequestsByPriority, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPFQueuedRequestsByPriority).
		Title("APF Queued Requests by Priority").
		Description("Current requests pending in API Priority and Fairness queues.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, priority_level) (apiserver_flowcontrol_current_inqueue_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{priority_level}}")))))

	d.Panel(elementAPFExecutingRequestsByPriority, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPFExecutingRequestsByPriority).
		Title("APF Executing Requests by Priority").
		Description("Current requests executing under API Priority and Fairness.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, priority_level) (apiserver_flowcontrol_current_executing_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{priority_level}}")))))

	d.Panel(elementAPFRejectedRequests, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPFRejectedRequests).
		Title("APF Rejected Requests").
		Description("API Priority and Fairness rejected requests. Nonzero values indicate overload or queue shedding.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, flow_schema, priority_level) (rate(apiserver_flowcontrol_rejected_requests_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{priority_level}} {{flow_schema}}")))))

	d.Panel(elementAPIStorageErrors, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelAPIStorageErrors).
		Title("API Storage Errors").
		Description("Storage decode and data key generation errors. Nonzero values deserve investigation.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster) (rate(apiserver_storage_decode_errors_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} decode errors"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster) (rate(apiserver_storage_data_key_generation_failures_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} key generation failures")))))

	d.Panel(elementEtcdRequestLatencyP99ByOperation, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelEtcdRequestLatencyP99ByOperation).
		Title("etcd Request Latency p99 by Operation").
		Description("99th percentile latency of API server requests to the etcd/kine backend, by operation. The backend is the usual root cause of control-plane slowness.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le, operation) (rate(etcd_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} {{operation}}")))))

	d.Panel(elementEtcdRequestRateByOperation, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelEtcdRequestRateByOperation).
		Title("etcd Request Rate by Operation").
		Description("Rate of API server requests to the etcd/kine backend, by operation.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, operation) (rate(etcd_request_duration_seconds_count{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{operation}}")))))

	d.Panel(elementEtcdRequestErrors, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelEtcdRequestErrors).
		Title("etcd Request Errors").
		Description("Errors returned by the etcd/kine backend. Nonzero values deserve investigation.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, operation) (rate(etcd_request_errors_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{operation}}")))))

	d.Panel(elementActiveWatchRequests, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelActiveWatchRequests).
		Title("Active Watch Requests").
		Description("Long-running (watch) requests currently held open against the API server. A steady climb can indicate a watch leak in a controller or client.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster) (apiserver_longrunning_requests{`+clusterFilter+`})`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementEtcdObjectCountByResourceTop20, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelEtcdObjectCountByResourceTop20).
		Title("etcd Object Count by Resource (Top 20)").
		Description("Number of objects stored in etcd/kine per resource type. Helps spot unexpected growth that bloats the backend.").
		Visualization(table.NewVisualizationV2Builder().
			OverrideByName("Objects", []dashboardv2.DynamicConfigValue{
				{Id: "unit", Value: "short"},
				{Id: "decimals", Value: 0},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(topk(20, sum by (cluster, resource) (apiserver_storage_objects{`+clusterFilter+`})))`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{resource}}"))).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
			"excludeByName": map[string]any{"Time": true},
			"indexByName": map[string]any{
				"cluster":  0,
				"resource": 1,
				"Value":    2,
			},
			"renameByName": map[string]any{
				"Value": "Objects",
			},
		}))))

	d.Panel(elementCoreDNSPanics1h, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCoreDNSPanics1h).
		Title("CoreDNS Panics (1h)").
		Description("CoreDNS process panics in the last hour.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(coredns_panics_total{`+clusterFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Panics")))))

	d.Panel(elementCoreDNSForwardFailures5m, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCoreDNSForwardFailures5m).
		Title("CoreDNS Forward Failures (5m)").
		Description("Upstream DNS forwarder healthcheck failures in the last 5 minutes.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(coredns_forward_healthcheck_broken_total{`+clusterFilter+`}[5m]))) or vector(0)`).
				LegendFormat("Failures")))))

	d.Panel(elementCoreDNSReloadFailures1h, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCoreDNSReloadFailures1h).
		Title("CoreDNS Reload Failures (1h)").
		Description("CoreDNS reload failures in the last hour.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(coredns_reload_failed_total{`+clusterFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Reload failures")))))

	d.Panel(elementDNSResponseRateByRCODE, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelDNSResponseRateByRCODE).
		Title("DNS Response Rate by RCODE").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, rcode) (rate(coredns_dns_responses_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{rcode}}")))))

	d.Panel(elementDNSRequestLatencyP99, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelDNSRequestLatencyP99).
		Title("DNS Request Latency p99").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99")))))

	d.Panel(elementCoreDNSCacheHitRatio, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCoreDNSCacheHitRatio).
		Title("CoreDNS Cache Hit Ratio").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percentunit").
			Min(0).
			Max(1).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster) (rate(coredns_cache_hits_total{`+clusterFilter+`}[$__rate_interval])) / clamp_min(sum by (cluster) (rate(coredns_cache_requests_total{`+clusterFilter+`}[$__rate_interval])), 1e-9)`).
				LegendFormat("{{cluster}} hit ratio")))))

	d.Panel(elementCoreDNSUpstreamProxyLatencyP99, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCoreDNSUpstreamProxyLatencyP99).
		Title("CoreDNS Upstream Proxy Latency p99").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le, proxy_name, to) (rate(coredns_proxy_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} {{proxy_name}} {{to}}")))))

	d.Panel(elementUnschedulablePodsByNamespace, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelUnschedulablePodsByNamespace).
		Title("Unschedulable Pods by Namespace").
		Description("Pods currently marked unschedulable, grouped by namespace. Empty is good: kube-state-metrics only emits this series while a pod is actually unschedulable, so nothing to show means nothing is stuck.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Orientation(common.VizOrientationHorizontal).
			Thresholds(issueThresholds).
			Decimals(0)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(sum by (cluster, namespace) (kube_pod_status_unschedulable{`+clusterFilter+`,`+nsFilter+`}))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{namespace}}")))))

	d.Panel(elementCordonedNodes, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCordonedNodes).
		Title("Cordoned Nodes").
		Description("Nodes marked unschedulable.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Orientation(common.VizOrientationHorizontal).
			Thresholds(issueThresholds).
			Decimals(0)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(kube_node_spec_unschedulable{`+clusterFilter+`} > 0)`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{node}}")))))

	d.Panel(elementContainerWaitingReasons, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelContainerWaitingReasons).
		Title("Container Waiting Reasons").
		Description("Containers currently waiting, grouped by reason. Empty is good: the reason label only exists while a container is waiting, so an empty table means every container is past that state.").
		Visualization(table.NewVisualizationV2Builder().
			OverrideByName("Containers", []dashboardv2.DynamicConfigValue{
				{Id: "unit", Value: "short"},
				{Id: "decimals", Value: 0},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, namespace, pod, container, reason) (kube_pod_container_status_waiting_reason{`+clusterFilter+`,`+nsFilter+`} == 1)`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}} {{reason}}"))).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
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
		}))))

	d.Panel(elementFailedJobs, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelFailedJobs).
		Title("Failed Jobs").
		Description("Jobs with one or more failed pods. Empty is good, and stays empty while the cluster runs no Jobs at all -- kube_job_* series exist only per Job object.").
		Visualization(table.NewVisualizationV2Builder().
			OverrideByName("Failed Pods", []dashboardv2.DynamicConfigValue{
				{Id: "unit", Value: "short"},
				{Id: "decimals", Value: 0},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(kube_job_status_failed{`+clusterFilter+`,`+nsFilter+`} > 0)`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{namespace}}/{{job_name}}"))).Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
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
		}))))

	d.Panel(elementSchedulingAttemptsByResult, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelSchedulingAttemptsByResult).
		Title("Scheduling Attempts by Result").
		Description("Rate of pod scheduling attempts by outcome. 'unschedulable' or 'error' indicate the scheduler cannot place pods.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("reqps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, result) (rate(scheduler_schedule_attempts_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{result}}")))))

	d.Panel(elementPendingPodsByQueue, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelPendingPodsByQueue).
		Title("Pending Pods by Queue").
		Description("Pods waiting to be scheduled, split by scheduler queue (active, backoff, unschedulable, gated). A sustained backlog signals scheduling problems.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster, queue) (scheduler_pending_pods{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{queue}}")))))

	d.Panel(elementSchedulingAttemptLatencyP99, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelSchedulingAttemptLatencyP99).
		Title("Scheduling Attempt Latency p99").
		Description("99th percentile end-to-end scheduling attempt duration.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(scheduler_scheduling_attempt_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99")))))

	d.Panel(elementWorkqueueDepth, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelWorkqueueDepth).
		Title("Workqueue Depth").
		Description("Total controller-manager workqueue depth. A sustained rise means controllers are falling behind on reconciliation.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster) (workqueue_depth{`+clusterFilter+`,job="kube-controller-manager"})`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementWorkqueueWorkDurationP99, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelWorkqueueWorkDurationP99).
		Title("Workqueue Work Duration p99").
		Description("99th percentile time controllers spend processing a single workqueue item.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(workqueue_work_duration_seconds_bucket{`+clusterFilter+`,job="kube-controller-manager"}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99")))))

	d.Panel(elementWorkqueueRetries, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelWorkqueueRetries).
		Title("Workqueue Retries").
		Description("Rate of workqueue item retries. Elevated retries indicate controllers repeatedly failing to reconcile.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (cluster) (rate(workqueue_retries_total{`+clusterFilter+`,job="kube-controller-manager"}[$__rate_interval]))`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementCPURequestedVsAllocatable, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelCPURequestedVsAllocatable).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(100 * sum by (cluster) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="cpu",`+nsFilter+`}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{`+clusterFilter+`,resource="cpu"}), 1e-9))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementMemoryRequestedVsAllocatable, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelMemoryRequestedVsAllocatable).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(100 * sum by (cluster) (kube_pod_container_resource_requests{`+clusterFilter+`,resource="memory",`+nsFilter+`}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{`+clusterFilter+`,resource="memory"}), 1e-9))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementPodsUsedVsAllocatable, dashboardv2.NewPanelBuilder().
		Id(k8sControlPlanePanelPodsUsedVsAllocatable).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort_desc(100 * sum by (cluster) (kubelet_running_pods{`+clusterFilter+`}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{`+clusterFilter+`,resource="pods"}), 1e-9))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		// Nine zero-is-healthy counters form three concern-based rows of three.
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementAPI5xx5m).
			WithItem(elementAPI4295m).
			WithItem(elementAPFQueued).
			WithItem(elementDNSSERVFAIL5m).
			WithItem(elementUnschedulablePods).
			WithItem(elementFailedJobsCurrent).
			WithItem(elementCordonedNodesCurrent).
			WithItem(elementNodePressureConditions).
			WithItem(elementPVNotBound))).
		Row(dashboardv2.NewRowBuilder().Title("API Server").AutoGridLayout(chartGrid().
			WithItem(elementAPIRequestRateByCode).
			WithItem(elementAPIRequestLatencyP99ByVerb))).
		Row(dashboardv2.NewRowBuilder().Title("API Inflight Requests by Kind").AutoGridLayout(chartGrid().
			WithItem(elementAPIInflightRequestsByKind).
			WithItem(elementAPFQueuedRequestsByPriority))).
		Row(dashboardv2.NewRowBuilder().Title("APF Executing Requests by Priority").AutoGridLayout(chartGrid().
			WithItem(elementAPFExecutingRequestsByPriority).
			WithItem(elementAPFRejectedRequests))).
		Row(dashboardv2.NewRowBuilder().Title("API Storage Errors").AutoGridLayout(chartGrid().
			WithItem(elementAPIStorageErrors))).
		Row(dashboardv2.NewRowBuilder().Title("etcd / Storage").AutoGridLayout(chartGrid().
			WithItem(elementEtcdRequestLatencyP99ByOperation).
			WithItem(elementEtcdRequestRateByOperation))).
		Row(dashboardv2.NewRowBuilder().Title("etcd Request Errors").AutoGridLayout(chartGrid().
			WithItem(elementEtcdRequestErrors).
			WithItem(elementActiveWatchRequests))).
		Row(dashboardv2.NewRowBuilder().Title("etcd Object Count by Resource (Top 20)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementEtcdObjectCountByResourceTop20))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS").AutoGridLayout(statGrid(3).
			WithItem(elementCoreDNSPanics1h).
			WithItem(elementCoreDNSForwardFailures5m).
			WithItem(elementCoreDNSReloadFailures1h))).
		Row(dashboardv2.NewRowBuilder().Title("DNS Response Rate by RCODE").AutoGridLayout(chartGrid().
			WithItem(elementDNSResponseRateByRCODE).
			WithItem(elementDNSRequestLatencyP99))).
		Row(dashboardv2.NewRowBuilder().Title("CoreDNS Cache Hit Ratio").AutoGridLayout(chartGrid().
			WithItem(elementCoreDNSCacheHitRatio).
			WithItem(elementCoreDNSUpstreamProxyLatencyP99))).
		Row(dashboardv2.NewRowBuilder().Title("Scheduling").AutoGridLayout(chartGrid().
			WithItem(elementUnschedulablePodsByNamespace).
			WithItem(elementCordonedNodes))).
		Row(dashboardv2.NewRowBuilder().Title("Container Waiting Reasons").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementContainerWaitingReasons))).
		Row(dashboardv2.NewRowBuilder().Title("Failed Jobs").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementFailedJobs))).
		Row(dashboardv2.NewRowBuilder().Title("Scheduler").AutoGridLayout(chartGrid().
			WithItem(elementSchedulingAttemptsByResult).
			WithItem(elementPendingPodsByQueue))).
		Row(dashboardv2.NewRowBuilder().Title("Scheduling Attempt Latency p99").AutoGridLayout(chartGrid().
			WithItem(elementSchedulingAttemptLatencyP99))).
		Row(dashboardv2.NewRowBuilder().Title("Controller Manager").AutoGridLayout(chartGrid().
			WithItem(elementWorkqueueDepth).
			WithItem(elementWorkqueueWorkDurationP99))).
		Row(dashboardv2.NewRowBuilder().Title("Workqueue Retries").AutoGridLayout(chartGrid().
			WithItem(elementWorkqueueRetries))).
		// Use radial gauges for single bounded cluster percentages; bar gauges are
		// reserved for ranked multi-item panels.
		Row(dashboardv2.NewRowBuilder().Title("Capacity").AutoGridLayout(chartGrid().
			WithItem(elementCPURequestedVsAllocatable).
			WithItem(elementMemoryRequestedVsAllocatable))).
		Row(dashboardv2.NewRowBuilder().Title("Pods Used vs Allocatable").AutoGridLayout(chartGrid().
			WithItem(elementPodsUsedVsAllocatable)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
