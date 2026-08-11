package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildArgocdOverview defines ArgoCD application health, sync activity, and
// repo-server performance across prd and sandbox.
//
// Metrics come from the four per-component ServiceMonitors
// (job="argocd-<component>-metrics", see docs/plans/prometheus-scrape-gaps.md).
func buildArgocdOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const clusterFilter = `cluster=~"$cluster"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()

	d, err := dashboard.NewDashboardBuilder("ArgoCD Overview").
		Uid("argocd-overview").
		Tags([]string{"argocd", "gitops", "infrastructure"}).
		Timezone("browser").
		Time("now-24h", "now").
		Refresh("1m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(promDatasourceVariable()).
		WithVariable(
			dashboard.NewQueryVariableBuilder("cluster").
				Label("Cluster").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(argocd_app_info, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Apps Not Healthy").
				Description("Applications whose health status is not Healthy.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(argocd_app_info{` + clusterFilter + `,health_status!="Healthy"}) or vector(0)`).
					LegendFormat("Not Healthy"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Apps OutOfSync").
				Description("Applications whose sync status is not Synced.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(argocd_app_info{` + clusterFilter + `,sync_status!="Synced"}) or vector(0)`).
					LegendFormat("OutOfSync"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Sync Failures (1h)").
				Description("Sync operations that ended in Error or Failed in the last hour.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(argocd_app_sync_total{` + clusterFilter + `,phase=~"Error|Failed"}[1h]))) or vector(0)`).
					LegendFormat("Failures"),
				),
		).
		// Second summary line: reachability of the things ArgoCD depends on,
		// as opposed to the application state counters above.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Cluster Connections Down").
				Description("Destination clusters the application-controller cannot reach.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(argocd_cluster_connection_status{` + clusterFilter + `} == 0) or vector(0)`).
					LegendFormat("Down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Git ls-remote Failures (1h)").
				Description("Failed git ls-remote calls from the repo-server in the last hour. Nonzero means ArgoCD could not reach the Git remote, so every application stops seeing new commits.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Aggregate away repo: on this metric the label holds the
					// local temp clone path (/tmp/_argocd-repo/<uuid>), not the
					// remote URL, so it churns on every repo-server restart.
					Expr(`ceil(sum(increase(argocd_git_lsremote_fail_total{` + clusterFilter + `}[1h]))) or vector(0)`).
					LegendFormat("Failures"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Applications")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Apps by Health Status").
				Description("Application count per health status.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (cluster, health_status) (argocd_app_info{`+clusterFilter+`})`).
					LegendFormat("{{cluster}} {{health_status}}"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Healthy"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* (Degraded|Missing)"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Progressing"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Apps by Sync Status").
				Description("Application count per sync status.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (cluster, sync_status) (argocd_app_info{`+clusterFilter+`})`).
					LegendFormat("{{cluster}} {{sync_status}}"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Synced"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* OutOfSync"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
				}),
		).
		WithPanel(
			table.NewPanelBuilder().
				Title("Apps Needing Attention").
				Description("Applications that are not Healthy or not Synced. Empty is good.").
				Datasource(ds).
				Span(24).Height(6).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`argocd_app_info{` + clusterFilter + `,health_status!="Healthy"} or argocd_app_info{` + clusterFilter + `,sync_status!="Synced"}`).
					Instant().Format(prometheus.PromQueryFormatTable),
				).
				WithTransformation(dashboard.DataTransformerConfig{
					Id: "organize",
					Options: map[string]any{
						"includeByName": map[string]any{
							"cluster":        true,
							"name":           true,
							"project":        true,
							"health_status":  true,
							"sync_status":    true,
							"dest_namespace": true,
						},
						"indexByName": map[string]any{
							"cluster":        0,
							"name":           1,
							"project":        2,
							"dest_namespace": 3,
							"health_status":  4,
							"sync_status":    5,
						},
						"renameByName": map[string]any{
							"name":           "Application",
							"project":        "Project",
							"dest_namespace": "Namespace",
							"health_status":  "Health",
							"sync_status":    "Sync",
						},
					},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Sync & Reconcile")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Sync Activity").
				Description("Sync operations per second by result phase.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, phase) (rate(argocd_app_sync_total{`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{phase}}"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* (Error|Failed)"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Succeeded"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("App Reconcile Latency").
				Description("Application reconciliation duration percentiles in the application-controller.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				// Reconciles arrive in bursts roughly every 2 minutes. Without a
				// floor, $__rate_interval shrinks to ~1m when zoomed in, every
				// bucket rate becomes 0, and histogram_quantile returns NaN --
				// which renders as a broken line.
				Interval("5m").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(argocd_app_reconcile_bucket{` + clusterFilter + `}[$__rate_interval])))`).
					LegendFormat("{{cluster}} p99"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.50, sum by (cluster, le) (rate(argocd_app_reconcile_bucket{` + clusterFilter + `}[$__rate_interval])))`).
					LegendFormat("{{cluster}} p50"),
				),
		).
		WithRow(dashboard.NewRowBuilder("repo-server")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Git Requests").
				Description("Git requests per second from the repo-server, by request type.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, request_type) (rate(argocd_git_request_total{` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{request_type}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Git Request Duration p95").
				Description("95th percentile git request duration, by request type. Slow fetches point at repo size or network issues. Git requests are sparse, so samples are drawn as points; a gap means no request occurred, not a failure.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				// ls-remote polls every ~3 minutes and fetch only fires on an
				// actual repo change, so a short window leaves every bucket rate
				// at 0 and histogram_quantile returns NaN. The floor keeps
				// ls-remote continuous; fetch stays sparse by nature, hence the
				// always-visible points.
				Interval("15m").
				ShowPoints(common.VisibilityModeAlways).
				PointSize(6).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.95, sum by (cluster, request_type, le) (rate(argocd_git_request_duration_seconds_bucket{` + clusterFilter + `}[$__rate_interval])))`).
					LegendFormat("{{cluster}} {{request_type}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
