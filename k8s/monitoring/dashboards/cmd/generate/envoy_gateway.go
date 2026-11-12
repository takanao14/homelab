package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	envoyGatewayPanelXDSDisconnectedProxies        = 1
	envoyGatewayPanelListenerRPS                   = 2
	envoyGatewayPanelUpstream5xxRate               = 3
	envoyGatewayPanelUpstreamLatencyP99            = 4
	envoyGatewayPanelRequestRateByListener         = 5
	envoyGatewayPanelRequestRateByHTTPRoute        = 6
	envoyGatewayPanelDownstreamResponseCodes       = 7
	envoyGatewayPanel4xx5xxByHTTPRoute             = 8
	envoyGatewayPanelUpstreamLatencyP99ByHTTPRoute = 9
	envoyGatewayPanelDownstreamActiveConnections   = 10
	envoyGatewayPanelXDSConnectedState             = 11
	envoyGatewayPanelXDSUpdateFailures             = 12
	envoyGatewayPanelControllerReconcileErrors     = 13
	envoyGatewayPanelGatewayAPIStatusUpdates       = 14
)

// buildEnvoyGatewayOverview covers Envoy data-plane traffic and latency plus
// controller xDS health. Proxy and controller jobs are scraped separately.
func buildEnvoyGatewayOverview() (*dashboardv2.Dashboard, error) {
	const (
		clusterFilter = `cluster=~"$cluster"`
		proxyJob      = `job="monitoring/envoy-gateway-proxy"`
		controllerJob = `job="envoy-gateway"`
		// Exclude admin, readiness, and stats listeners.
		trafficListeners = `envoy_http_conn_manager_prefix!~"admin|eg-.*"`
		// Select HTTPRoute upstreams; exclude xDS and stats clusters.
		httprouteClusters = `envoy_cluster_name=~"httproute/.*"`
	)

	// routeLabel converts raw rule cluster names to namespace/name legends.
	routeLabel := func(expr string) string {
		return `label_replace(` + expr + `, "route", "$1/$2", "envoy_cluster_name", "httproute/([^/]+)/([^/]+)/.*")`
	}

	// byRoute merges multiple Gateway API rules into one series per HTTPRoute.
	byRoute := func(expr string) string {
		return `sum by (cluster, route) (` + routeLabel(expr) + `)`
	}

	// byRouteQuantile merges histogram buckets before calculating route latency.
	byRouteQuantile := func(quantile, expr string) string {
		return `histogram_quantile(` + quantile + `, sum by (cluster, route, le) (` + routeLabel(expr) + `))`
	}

	// withRouteBaseline supplies zero response-class series from total requests.
	// Do not apply it to latency: no requests must remain absent, not 0 ms.
	withRouteBaseline := func(expr string) string {
		return expr + ` or ` + byRoute(
			`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_total{`+
				proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`}[$__rate_interval]))`) + ` * 0`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()

	// Upstream latency thresholds in milliseconds.
	latencyThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(250)), Color: "yellow"},
			{Value: new(float64(1000)), Color: "red"},
		})

	// Any sustained 5xx rate is an issue.
	errorRateThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(0.001), Color: "red"},
		})

	d := dashboardv2.NewDashboardBuilder("Envoy Gateway Overview").
		Tags([]string{"envoy-gateway", "gateway-api", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-24h").To("now").AutoRefresh("1m")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			Query(promVariableQuery(`label_values(envoy_server_live, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	return newLayout(d).
		Row("Summary", statGrid(4)).
		Panel(envoyGatewayPanelXDSDisconnectedProxies, dashboardv2.NewPanelBuilder().
			Title("xDS Disconnected Proxies").
			Description("Envoy proxies without a live xDS connection to the envoy-gateway controller.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`count(envoy_control_plane_connected_state{`+proxyJob+`,`+clusterFilter+`} == 0) or vector(0)`).
				LegendFormat("Disconnected")))).
		Panel(envoyGatewayPanelListenerRPS, dashboardv2.NewPanelBuilder().
			Title("Listener RPS").
			Description("Total downstream requests per second across traffic listeners.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("reqps").
				Thresholds(measurementThresholds()).
				Decimals(2)).
			Data(queries(promQuery().
				Expr(`sum(rate(envoy_http_downstream_rq_total{`+proxyJob+`,`+clusterFilter+`,`+trafficListeners+`}[$__rate_interval]))`).
				LegendFormat("RPS")))).
		Panel(envoyGatewayPanelUpstream5xxRate, dashboardv2.NewPanelBuilder().
			Title("Upstream 5xx Rate").
			Description("5xx responses per second from HTTPRoute upstreams.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("reqps").
				Thresholds(errorRateThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Decimals(3)).
			Data(queries(promQuery().
				Expr(`sum(rate(envoy_cluster_upstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`,envoy_response_code_class="5"}[$__rate_interval])) or vector(0)`).
				LegendFormat("5xx")))).
		Panel(envoyGatewayPanelUpstreamLatencyP99, dashboardv2.NewPanelBuilder().
			Title("Upstream Latency p99").
			Description("99th percentile upstream request time across all HTTPRoutes.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("ms").
				Thresholds(latencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(`histogram_quantile(0.99, sum by (le) (rate(envoy_cluster_upstream_rq_time_bucket{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`}[$__rate_interval])))`).
				LegendFormat("p99")). // Sparse traffic needs a wider window for a stable summary quantile.
				QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("15m")))).
		Row("Traffic", chartGrid()).
		Panel(envoyGatewayPanelRequestRateByListener, dashboardv2.NewPanelBuilder().
			Title("Request Rate by Listener").
			Description("Downstream requests per second per traffic listener.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, envoy_http_conn_manager_prefix) (rate(envoy_http_downstream_rq_total{`+proxyJob+`,`+clusterFilter+`,`+trafficListeners+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{envoy_http_conn_manager_prefix}}")))).
		Panel(envoyGatewayPanelRequestRateByHTTPRoute, dashboardv2.NewPanelBuilder().
			Title("Request Rate by HTTPRoute").
			Description("Upstream requests per second per HTTPRoute.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byRoute(`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_total{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`}[$__rate_interval]))`)).
				LegendFormat("{{cluster}} {{route}}")))).
		Row("Downstream Response Codes", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(envoyGatewayPanelDownstreamResponseCodes, dashboardv2.NewPanelBuilder().
			Title("Downstream Response Codes").
			Description("Responses per second by status code class, across traffic listeners.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				// Semantic coloring: 2xx=green, 3xx=blue, 4xx=orange, 5xx=red
				Override(dashboardv2.MatcherConfig{Id: "byName", Options: "2xx"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}).
				Override(dashboardv2.MatcherConfig{Id: "byName", Options: "3xx"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "blue"}},
				}).
				Override(dashboardv2.MatcherConfig{Id: "byName", Options: "4xx"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
				}).
				Override(dashboardv2.MatcherConfig{Id: "byName", Options: "5xx"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				})).
			Data(queries(promQuery().
				Expr(`sum by (envoy_response_code_class) (rate(envoy_http_downstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+trafficListeners+`}[$__rate_interval]))`).
				LegendFormat("{{envoy_response_code_class}}xx")))).
		Row("4xx / 5xx by HTTPRoute", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(envoyGatewayPanel4xx5xxByHTTPRoute, dashboardv2.NewPanelBuilder().
			Title("4xx / 5xx by HTTPRoute").
			Description("Upstream error responses per second per HTTPRoute.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* 5xx"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				})).
			Data(queries(promQuery().
				Expr(withRouteBaseline(byRoute(`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`,envoy_response_code_class="4"}[$__rate_interval]))`))).
				LegendFormat("{{cluster}} {{route}} 4xx"),
				promQuery().
					Expr(withRouteBaseline(byRoute(`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`,envoy_response_code_class="5"}[$__rate_interval]))`))).
					LegendFormat("{{cluster}} {{route}} 5xx")))).
		Row("Latency & Connections", chartGrid()).
		Panel(envoyGatewayPanelUpstreamLatencyP99ByHTTPRoute, dashboardv2.NewPanelBuilder().
			Title("Upstream Latency p99 by HTTPRoute").
			Description("99th percentile upstream request time per HTTPRoute. Routes here are sparse, so samples are drawn as points; a gap means no request was served in that window, not that the route was slow.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ms").
				ShowPoints(common.VisibilityModeAlways).
				PointSize(6).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byRouteQuantile("0.99", `sum by (cluster, envoy_cluster_name, le) (rate(envoy_cluster_upstream_rq_time_bucket{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`}[$__rate_interval]))`)).
				LegendFormat("{{cluster}} {{route}}")). // A 15-minute floor reduces sparse gaps; draw remaining samples as points.
				QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("15m")))).
		Panel(envoyGatewayPanelDownstreamActiveConnections, dashboardv2.NewPanelBuilder().
			Title("Downstream Active Connections").
			Description("Active downstream connections per traffic listener.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum by (cluster, envoy_http_conn_manager_prefix) (envoy_http_downstream_cx_active{`+proxyJob+`,`+clusterFilter+`,`+trafficListeners+`})`).
				LegendFormat("{{cluster}} {{envoy_http_conn_manager_prefix}}")))).
		Row("xDS / Control Plane", chartGrid()).
		Panel(envoyGatewayPanelXDSConnectedState, dashboardv2.NewPanelBuilder().
			Title("xDS Connected State").
			Description("Per-proxy xDS connection to the envoy-gateway controller (1 = connected).").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Max(1).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`min by (cluster, pod) (envoy_control_plane_connected_state{`+proxyJob+`,`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{pod}}")))).
		Panel(envoyGatewayPanelXDSUpdateFailures, dashboardv2.NewPanelBuilder().
			Title("xDS Update Failures").
			Description("CDS/LDS update failures and rejections per second. Rejections indicate config the proxy refused (NACK).").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(issueThresholds)).
			Data(queries(promQuery().
				Expr(`sum by (cluster) (rate(envoy_cluster_manager_cds_update_failure{`+proxyJob+`,`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} CDS failure"),
				promQuery().
					Expr(`sum by (cluster) (rate(envoy_cluster_manager_cds_update_rejected{`+proxyJob+`,`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} CDS rejected"),
				promQuery().
					Expr(`sum by (cluster) (rate(envoy_listener_manager_lds_update_failure{`+proxyJob+`,`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} LDS failure"),
				promQuery().
					Expr(`sum by (cluster) (rate(envoy_listener_manager_lds_update_rejected{`+proxyJob+`,`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} LDS rejected")))).
		Panel(envoyGatewayPanelControllerReconcileErrors, dashboardv2.NewPanelBuilder().
			Title("Controller Reconcile Errors").
			Description("controller-runtime reconcile errors per second in the envoy-gateway controller.").
			Visualization(timeseries.NewVisualizationV2Builder().
					Unit("ops").
					Tooltip(tooltipAll).
					Legend(legend)).
			Data(queries( // Aggregate away restart-specific controller labels.
				promQuery().
					Expr(`sum by (cluster) (rate(controller_runtime_reconcile_errors_total{`+controllerJob+`,`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}}")))).
		Panel(envoyGatewayPanelGatewayAPIStatusUpdates, dashboardv2.NewPanelBuilder().
			Title("Gateway API Status Updates").
			Description("Status updates actually written by the envoy-gateway controller, per resource kind. Evaluations that concluded nothing needed writing (status=\"no_action\") are excluded.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Count written status updates only; no_action is evaluation noise.
				Expr(`sum by (cluster, kind) (rate(status_update_total{`+controllerJob+`,`+clusterFilter+`,status="success"}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{kind}}")))).
		Build()
}
