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

// buildK8sControlPlaneOverview defines Kubernetes control-plane and DNS health.
// It complements kubernetes-overview, which focuses on workloads and node resources.
func buildK8sControlPlaneOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		clusterFilter = `cluster=~"$cluster"`
		nsFilter      = `namespace=~"$namespace"`
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	capacityThresholds := capacityThresholds()

	d, err := dashboard.NewDashboardBuilder("K8s Control Plane Overview").
		Uid("k8s-control-plane-overview").
		Tags([]string{"kubernetes", "control-plane", "dns", "infrastructure"}).
		Timezone("browser").
		Time("now-6h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("cluster").
				Label("Cluster").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(kube_node_info, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("namespace").
				Label("Namespace").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(kube_namespace_status_phase{` + clusterFilter + `}, namespace)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		// Summary tiles are all "0 = healthy" issue counters, laid out Span(8) so the
		// nine of them tile evenly as 3 columns x 3 rows. Panel order is the layout:
		// each row of three is one concern — API serving, scheduling, then node and
		// storage state. Keep additions in multiples of three, or the grid breaks.
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("API 5xx (5m)").
				Description("API server responses with 5xx status codes in the last 5 minutes.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(apiserver_request_total{` + clusterFilter + `,code=~"5.."}[5m]))) or vector(0)`).
					LegendFormat("5xx"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("API 429 (5m)").
				Description("HTTP 429 responses in the last 5 minutes. 429 indicates API priority and fairness throttling or overload.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(apiserver_request_total{` + clusterFilter + `,code="429"}[5m]))) or vector(0)`).
					LegendFormat("429"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("APF Queued").
				Description("Current requests pending in API Priority and Fairness queues.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(apiserver_flowcontrol_current_inqueue_requests{` + clusterFilter + `}) or vector(0)`).
					LegendFormat("Queued"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS SERVFAIL (5m)").
				Description("CoreDNS SERVFAIL responses in the last 5 minutes.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(coredns_dns_responses_total{` + clusterFilter + `,rcode="SERVFAIL"}[5m]))) or vector(0)`).
					LegendFormat("SERVFAIL"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unschedulable Pods").
				Description("Pods currently marked unschedulable.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_pod_status_unschedulable{` + clusterFilter + `,` + nsFilter + `}) or vector(0)`).
					LegendFormat("Unschedulable"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Failed Jobs").
				Description("Jobs with failed pods in the selected namespaces.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_job_status_failed{` + clusterFilter + `,` + nsFilter + `}) or vector(0)`).
					LegendFormat("Failed"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Cordoned Nodes").
				Description("Nodes marked unschedulable.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(kube_node_spec_unschedulable{` + clusterFilter + `}) or vector(0)`).
					LegendFormat("Cordoned"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Node Pressure Conditions").
				Description("Nodes currently reporting MemoryPressure, DiskPressure, PIDPressure, or NetworkUnavailable.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// status="true" only selects the "true" series; == 1 checks it's actually asserted.
					Expr(`count(kube_node_status_condition{` + clusterFilter + `,condition=~"MemoryPressure|DiskPressure|PIDPressure|NetworkUnavailable",status="true"} == 1) or vector(0)`).
					LegendFormat("Under pressure"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("PV Not Bound").
				Description("PersistentVolumes not in the Bound phase.").
				Datasource(ds).
				Span(8).Height(3).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(kube_persistentvolume_status_phase{` + clusterFilter + `,phase!="Bound"} == 1) or vector(0)`).
					LegendFormat("Not bound"),
				),
		).
		WithRow(dashboard.NewRowBuilder("API Server")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("API Request Rate by Code").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, code) (rate(apiserver_request_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} {{code}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("API Request Latency p99 by Verb").
				Description("99th percentile API request latency by verb. Long-running WATCH requests are excluded.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le, verb) (rate(apiserver_request_duration_seconds_bucket{` + clusterFilter + `,subresource="",verb!="WATCH"}[5m]))) and on (cluster, verb) sum by (cluster, verb) (rate(apiserver_request_duration_seconds_count{` + clusterFilter + `,subresource="",verb!="WATCH"}[5m])) > 0`).
					LegendFormat("{{cluster}} {{verb}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("API Inflight Requests by Kind").
				Description("Current API requests actively being handled, split by read-only and mutating requests.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, request_kind) (apiserver_current_inflight_requests{` + clusterFilter + `})`).
					LegendFormat("{{cluster}} {{request_kind}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("APF Queued Requests by Priority").
				Description("Current requests pending in API Priority and Fairness queues.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, priority_level) (apiserver_flowcontrol_current_inqueue_requests{` + clusterFilter + `})`).
					LegendFormat("{{cluster}} {{priority_level}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("APF Executing Requests by Priority").
				Description("Current requests executing under API Priority and Fairness.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, priority_level) (apiserver_flowcontrol_current_executing_requests{` + clusterFilter + `})`).
					LegendFormat("{{cluster}} {{priority_level}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("APF Rejected Requests").
				Description("API Priority and Fairness rejected requests. Nonzero values indicate overload or queue shedding.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, flow_schema, priority_level) (rate(apiserver_flowcontrol_rejected_requests_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} {{priority_level}} {{flow_schema}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("API Storage Errors").
				Description("Storage decode and data key generation errors. Nonzero values deserve investigation.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(apiserver_storage_decode_errors_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} decode errors"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(apiserver_storage_data_key_generation_failures_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} key generation failures"),
				),
		).
		WithRow(dashboard.NewRowBuilder("etcd / Storage")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("etcd Request Latency p99 by Operation").
				Description("99th percentile latency of API server requests to the etcd/kine backend, by operation. The backend is the usual root cause of control-plane slowness.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le, operation) (rate(etcd_request_duration_seconds_bucket{` + clusterFilter + `}[5m])))`).
					LegendFormat("{{cluster}} {{operation}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("etcd Request Rate by Operation").
				Description("Rate of API server requests to the etcd/kine backend, by operation.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, operation) (rate(etcd_request_duration_seconds_count{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} {{operation}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("etcd Request Errors").
				Description("Errors returned by the etcd/kine backend. Nonzero values deserve investigation.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, operation) (rate(etcd_request_errors_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} {{operation}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Active Watch Requests").
				Description("Long-running (watch) requests currently held open against the API server. A steady climb can indicate a watch leak in a controller or client.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (apiserver_longrunning_requests{` + clusterFilter + `})`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("etcd Object Count by Resource (Top 20)").
				Description("Number of objects stored in etcd/kine per resource type. Helps spot unexpected growth that bloats the backend.").
				Datasource(ds).
				Span(24).Height(9).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(topk(20, sum by (cluster, resource) (apiserver_storage_objects{`+clusterFilter+`})))`).
					Instant().Format(prometheus.PromQueryFormatTable).
					LegendFormat("{{cluster}} {{resource}}"),
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
							"cluster":  0,
							"resource": 1,
							"Value":    2,
						},
						"renameByName": map[string]any{
							"Value": "Objects",
						},
					},
				}).
				OverrideByName("Objects", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 0},
				}),
		).
		WithRow(dashboard.NewRowBuilder("CoreDNS")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS Panics (1h)").
				Description("CoreDNS process panics in the last hour.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(coredns_panics_total{` + clusterFilter + `}[1h]))) or vector(0)`).
					LegendFormat("Panics"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS Forward Failures (5m)").
				Description("Upstream DNS forwarder healthcheck failures in the last 5 minutes.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(coredns_forward_healthcheck_broken_total{` + clusterFilter + `}[5m]))) or vector(0)`).
					LegendFormat("Failures"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("CoreDNS Reload Failures (1h)").
				Description("CoreDNS reload failures in the last hour.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(coredns_reload_failed_total{` + clusterFilter + `}[1h]))) or vector(0)`).
					LegendFormat("Reload failures"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("DNS Response Rate by RCODE").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, rcode) (rate(coredns_dns_responses_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} {{rcode}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("DNS Request Latency p99").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(coredns_dns_request_duration_seconds_bucket{` + clusterFilter + `}[5m])))`).
					LegendFormat("{{cluster}} p99"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CoreDNS Cache Hit Ratio").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percentunit").
				Min(0).
				Max(1).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(coredns_cache_hits_total{` + clusterFilter + `}[5m])) / clamp_min(sum by (cluster) (rate(coredns_cache_requests_total{` + clusterFilter + `}[5m])), 1e-9)`).
					LegendFormat("{{cluster}} hit ratio"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CoreDNS Upstream Proxy Latency p99").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le, proxy_name, to) (rate(coredns_proxy_request_duration_seconds_bucket{` + clusterFilter + `}[5m])))`).
					LegendFormat("{{cluster}} {{proxy_name}} {{to}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Scheduling")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Unschedulable Pods by Namespace").
				Description("Pods currently marked unschedulable, grouped by namespace.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(sum by (cluster, namespace) (kube_pod_status_unschedulable{` + clusterFilter + `,` + nsFilter + `}))`).
					Instant().
					LegendFormat("{{cluster}} {{namespace}}"),
				).
				Decimals(0),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Cordoned Nodes").
				Description("Nodes marked unschedulable.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(kube_node_spec_unschedulable{` + clusterFilter + `} > 0)`).
					Instant().
					LegendFormat("{{cluster}} {{node}}"),
				).
				Decimals(0),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("Container Waiting Reasons").
				Description("Containers currently waiting, grouped by reason.").
				Datasource(ds).
				Span(8).Height(8).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, namespace, pod, container, reason) (kube_pod_container_status_waiting_reason{`+clusterFilter+`,`+nsFilter+`} == 1)`).
					Instant().Format(prometheus.PromQueryFormatTable).
					LegendFormat("{{cluster}} {{namespace}}/{{pod}}/{{container}} {{reason}}"),
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
							"reason":    4,
							"Value":     5,
						},
						"renameByName": map[string]any{
							"Value": "Containers",
						},
					},
				}).
				OverrideByName("Containers", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 0},
				}),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("Failed Jobs").
				Description("Jobs with one or more failed pods.").
				Datasource(ds).
				Span(24).Height(9).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(kube_job_status_failed{`+clusterFilter+`,`+nsFilter+`} > 0)`).
					Instant().Format(prometheus.PromQueryFormatTable).
					LegendFormat("{{cluster}} {{namespace}}/{{job_name}}"),
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
							"job_name":  2,
							"Value":     3,
						},
						"renameByName": map[string]any{
							"Value": "Failed Pods",
						},
					},
				}).
				OverrideByName("Failed Pods", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "short"},
					{Id: "decimals", Value: 0},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Scheduler")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Scheduling Attempts by Result").
				Description("Rate of pod scheduling attempts by outcome. 'unschedulable' or 'error' indicate the scheduler cannot place pods.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("reqps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, result) (rate(scheduler_schedule_attempts_total{` + clusterFilter + `}[5m]))`).
					LegendFormat("{{cluster}} {{result}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Pending Pods by Queue").
				Description("Pods waiting to be scheduled, split by scheduler queue (active, backoff, unschedulable, gated). A sustained backlog signals scheduling problems.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, queue) (scheduler_pending_pods{` + clusterFilter + `})`).
					LegendFormat("{{cluster}} {{queue}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Scheduling Attempt Latency p99").
				Description("99th percentile end-to-end scheduling attempt duration.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(scheduler_scheduling_attempt_duration_seconds_bucket{` + clusterFilter + `}[5m])))`).
					LegendFormat("{{cluster}} p99"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Controller Manager")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Workqueue Depth").
				Description("Total controller-manager workqueue depth. A sustained rise means controllers are falling behind on reconciliation.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (workqueue_depth{` + clusterFilter + `,job="kube-controller-manager"})`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Workqueue Work Duration p99").
				Description("99th percentile time controllers spend processing a single workqueue item.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(workqueue_work_duration_seconds_bucket{` + clusterFilter + `,job="kube-controller-manager"}[5m])))`).
					LegendFormat("{{cluster}} p99"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Workqueue Retries").
				Description("Rate of workqueue item retries. Elevated retries indicate controllers repeatedly failing to reconcile.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(workqueue_retries_total{` + clusterFilter + `,job="kube-controller-manager"}[5m]))`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Capacity")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("CPU Requested vs Allocatable").
				Description("Total CPU requests as a percentage of cluster allocatable CPU.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(100 * sum by (cluster) (kube_pod_container_resource_requests{` + clusterFilter + `,resource="cpu",` + nsFilter + `}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{` + clusterFilter + `,resource="cpu"}), 1e-9))`).
					Instant().
					LegendFormat("{{cluster}}"),
				).
				Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Memory Requested vs Allocatable").
				Description("Total memory requests as a percentage of cluster allocatable memory.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(100 * sum by (cluster) (kube_pod_container_resource_requests{` + clusterFilter + `,resource="memory",` + nsFilter + `}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{` + clusterFilter + `,resource="memory"}), 1e-9))`).
					Instant().
					LegendFormat("{{cluster}}"),
				).
				Decimals(1),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("Pods Used vs Allocatable").
				Description("Running pods as a percentage of cluster allocatable pod capacity.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("percent").
				Min(0).
				Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(100 * sum by (cluster) (kubelet_running_pods{` + clusterFilter + `}) / clamp_min(sum by (cluster) (kube_node_status_allocatable{` + clusterFilter + `,resource="pods"}), 1e-9))`).
					Instant().
					LegendFormat("{{cluster}}"),
				).
				Decimals(1),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
