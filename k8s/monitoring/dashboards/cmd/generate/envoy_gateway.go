package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildEnvoyGatewayOverview defines the Envoy Gateway data-plane and control-plane
// dashboard: listener/HTTPRoute traffic, response codes, upstream latency, and
// xDS sync health.
//
// Scrape topology (see docs/plans/prometheus-scrape-gaps.md):
//   - job="monitoring/envoy-gateway-proxy": Envoy proxy pods (PodMonitor), data plane
//   - job="envoy-gateway": controller (ServiceMonitor), control plane
func buildEnvoyGatewayOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const (
		clusterFilter = `cluster=~"$cluster"`
		proxyJob      = `job="monitoring/envoy-gateway-proxy"`
		controllerJob = `job="envoy-gateway"`
		// Traffic listeners only — exclude Envoy's admin and Envoy Gateway's
		// internal readiness/stats listeners.
		trafficListeners = `envoy_http_conn_manager_prefix!~"admin|eg-.*"`
		// Upstream clusters created per HTTPRoute rule; excludes xds_cluster
		// and prometheus_stats.
		httprouteClusters = `envoy_cluster_name=~"httproute/.*"`
	)

	// routeLabel rewrites the raw envoy_cluster_name
	// ("httproute/<namespace>/<name>/rule/<n>") into a compact
	// "namespace/name" route label for legends.
	routeLabel := func(expr string) string {
		return `label_replace(` + expr + `, "route", "$1/$2", "envoy_cluster_name", "httproute/([^/]+)/([^/]+)/.*")`
	}

	// byRoute collapses the per-rule series that routeLabel leaves behind. The
	// rewrite adds a route label but does not remove envoy_cluster_name, so an
	// HTTPRoute with several rules stays several series that merely share a
	// legend: monitoring/alloy has rule/0 and rule/1 and drew two lines both
	// captioned "prd monitoring/alloy". Multiple rules are ordinary Gateway API
	// usage -- any path- or header-based split produces them -- and a panel
	// titled "by HTTPRoute" should show one line per route.
	byRoute := func(expr string) string {
		return `sum by (cluster, route) (` + routeLabel(expr) + `)`
	}

	// byRouteQuantile does the same for the latency histogram, where the merge
	// has to happen on the buckets: taking a quantile per rule and relabelling
	// afterwards yields one line per rule, and averaging those quantiles would
	// be meaningless. Summing the buckets by route first gives the quantile
	// across the whole route.
	byRouteQuantile := func(quantile, expr string) string {
		return `histogram_quantile(` + quantile + `, sum by (cluster, route, le) (` + routeLabel(expr) + `))`
	}

	// withRouteBaseline pins every route to zero so a route that has never
	// returned a given response class still draws a line. envoy_cluster_upstream_rq_xx
	// only exists once a class has actually occurred, so today the 4xx panel
	// carries three routes of seven and the 5xx panel two: the other routes are
	// absent rather than zero, and nothing on screen distinguishes "healthy"
	// from "not measured". The baseline enumerates routes from the total request
	// counter, which exists for all of them, and multiplies it away.
	//
	// This is deliberately not applied to the latency panel. A missing quantile
	// means no request was served, and writing 0 ms there would claim the route
	// answered instantly.
	withRouteBaseline := func(expr string) string {
		return expr + ` or ` + byRoute(
			`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_total{`+
				proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`}[$__rate_interval]))`) + ` * 0`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()

	// Upstream latency thresholds in milliseconds.
	latencyThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(250)), Color: "yellow"},
			{Value: new(float64(1000)), Color: "red"},
		})

	// Any sustained 5xx rate is an issue.
	errorRateThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(0.001), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("Envoy Gateway Overview").
		Uid("envoy-gateway-overview").
		Tags([]string{"envoy-gateway", "gateway-api", "infrastructure"}).
		Timezone("browser").
		Time("now-24h", "now").
		Refresh("1m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(promDatasourceVariable()).
		WithVariable(
			dashboard.NewQueryVariableBuilder("cluster").
				Label("Cluster").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(envoy_server_live, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("xDS Disconnected Proxies").
				Description("Envoy proxies without a live xDS connection to the envoy-gateway controller.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(envoy_control_plane_connected_state{` + proxyJob + `,` + clusterFilter + `} == 0) or vector(0)`).
					LegendFormat("Disconnected"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Listener RPS").
				Description("Total downstream requests per second across traffic listeners.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(envoy_http_downstream_rq_total{` + proxyJob + `,` + clusterFilter + `,` + trafficListeners + `}[$__rate_interval]))`).
					LegendFormat("RPS"),
				).Decimals(2),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Upstream 5xx Rate").
				Description("5xx responses per second from HTTPRoute upstreams.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("reqps").
				Thresholds(errorRateThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(rate(envoy_cluster_upstream_rq_xx{` + proxyJob + `,` + clusterFilter + `,` + httprouteClusters + `,envoy_response_code_class="5"}[$__rate_interval])) or vector(0)`).
					LegendFormat("5xx"),
				).Decimals(3),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Upstream Latency p99").
				Description("99th percentile upstream request time across all HTTPRoutes.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("ms").
				// Even summed over every route this gateway sees only about
				// 9,500 requests a day, so at the ~2m window the default range
				// produces, 69% of windows contain no request at all: every
				// bucket rate is 0, histogram_quantile returns NaN, and the tile
				// falls back to whatever it last managed to compute. The floor
				// makes the window wide enough to hold requests.
				Interval("15m").
				Thresholds(latencyThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (le) (rate(envoy_cluster_upstream_rq_time_bucket{` + proxyJob + `,` + clusterFilter + `,` + httprouteClusters + `}[$__rate_interval])))`).
					LegendFormat("p99"),
				).Decimals(1),
		).
		WithRow(dashboard.NewRowBuilder("Traffic")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Request Rate by Listener").
				Description("Downstream requests per second per traffic listener.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, envoy_http_conn_manager_prefix) (rate(envoy_http_downstream_rq_total{` + proxyJob + `,` + clusterFilter + `,` + trafficListeners + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{envoy_http_conn_manager_prefix}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Request Rate by HTTPRoute").
				Description("Upstream requests per second per HTTPRoute.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byRoute(`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_total{` + proxyJob + `,` + clusterFilter + `,` + httprouteClusters + `}[$__rate_interval]))`)).
					LegendFormat("{{cluster}} {{route}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Downstream Response Codes").
				Description("Responses per second by status code class, across traffic listeners.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (envoy_response_code_class) (rate(envoy_http_downstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+trafficListeners+`}[$__rate_interval]))`).
					LegendFormat("{{envoy_response_code_class}}xx"),
				).
				// Semantic coloring: 2xx=green, 3xx=blue, 4xx=orange, 5xx=red
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "2xx"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "3xx"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "blue"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "4xx"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "5xx"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("4xx / 5xx by HTTPRoute").
				Description("Upstream error responses per second per HTTPRoute.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("reqps").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(withRouteBaseline(byRoute(`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`,envoy_response_code_class="4"}[$__rate_interval]))`))).
					LegendFormat("{{cluster}} {{route}} 4xx"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(withRouteBaseline(byRoute(`sum by (cluster, envoy_cluster_name) (rate(envoy_cluster_upstream_rq_xx{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`,envoy_response_code_class="5"}[$__rate_interval]))`))).
					LegendFormat("{{cluster}} {{route}} 5xx"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* 5xx"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Latency & Connections")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Upstream Latency p99 by HTTPRoute").
				Description("99th percentile upstream request time per HTTPRoute. Routes here are sparse, so samples are drawn as points; a gap means no request was served in that window, not that the route was slow.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ms").
				// Per route the traffic is thinner still than the summary tile's:
				// grafana serves 1,170 requests a day and had no request at all
				// in 78% of 2-minute windows, which is 78% of the line missing.
				// A 15m floor takes that to 14%. It cannot fix the quiet routes
				// -- pdns-ui sees 14 requests a day, argocd 80 -- so the
				// remaining samples are drawn as points rather than pretending a
				// line runs between them.
				Interval("15m").
				ShowPoints(common.VisibilityModeAlways).
				PointSize(6).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byRouteQuantile("0.99", `sum by (cluster, envoy_cluster_name, le) (rate(envoy_cluster_upstream_rq_time_bucket{`+proxyJob+`,`+clusterFilter+`,`+httprouteClusters+`}[$__rate_interval]))`)).
					LegendFormat("{{cluster}} {{route}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Downstream Active Connections").
				Description("Active downstream connections per traffic listener.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, envoy_http_conn_manager_prefix) (envoy_http_downstream_cx_active{` + proxyJob + `,` + clusterFilter + `,` + trafficListeners + `})`).
					LegendFormat("{{cluster}} {{envoy_http_conn_manager_prefix}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("xDS / Control Plane")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("xDS Connected State").
				Description("Per-proxy xDS connection to the envoy-gateway controller (1 = connected).").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).Max(1).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`min by (cluster, pod) (envoy_control_plane_connected_state{` + proxyJob + `,` + clusterFilter + `})`).
					LegendFormat("{{cluster}} {{pod}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("xDS Update Failures").
				Description("CDS/LDS update failures and rejections per second. Rejections indicate config the proxy refused (NACK).").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(issueThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(envoy_cluster_manager_cds_update_failure{` + proxyJob + `,` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} CDS failure"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(envoy_cluster_manager_cds_update_rejected{` + proxyJob + `,` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} CDS rejected"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(envoy_listener_manager_lds_update_failure{` + proxyJob + `,` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} LDS failure"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(envoy_listener_manager_lds_update_rejected{` + proxyJob + `,` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} LDS rejected"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Controller Reconcile Errors").
				Description("controller-runtime reconcile errors per second in the envoy-gateway controller.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				// The controller label is gatewayapi-<timestamp>, unique per
				// restart, so aggregate by cluster only to avoid series churn.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(controller_runtime_reconcile_errors_total{` + controllerJob + `,` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Gateway API Status Updates").
				Description("Status updates actually written by the envoy-gateway controller, per resource kind. Evaluations that concluded nothing needed writing (status=\"no_action\") are excluded.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// status="success" only. The counter also carries
					// status="no_action", meaning the controller looked and
					// decided nothing needed writing, and summing the label away
					// counted those as updates: HTTPRoute read 31 against 20
					// actually written. no_action is steady evaluation noise
					// here, but a sharp rise in it would mean the controller has
					// stopped seeing changes -- worth splitting out by status if
					// that ever needs watching.
					Expr(`sum by (cluster, kind) (rate(status_update_total{` + controllerJob + `,` + clusterFilter + `,status="success"}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{kind}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
