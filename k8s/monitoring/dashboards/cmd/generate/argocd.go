package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	argocdPanelAppsNotHealthy         = 1
	argocdPanelAppsOutOfSync          = 2
	argocdPanelSyncFailures1h         = 3
	argocdPanelClusterConnectionsDown = 4
	argocdPanelGitLsRemoteFailures1h  = 5
	argocdPanelAppsByHealthStatus     = 6
	argocdPanelAppsBySyncStatus       = 7
	argocdPanelAppsNeedingAttention   = 8
	argocdPanelSyncActivity           = 9
	argocdPanelAppReconcileLatency    = 10
	argocdPanelGitRequests            = 11
	argocdPanelGitRequestDurationP95  = 12
)

// buildArgocdOverview covers application health, sync activity, reconciliation,
// and repo-server performance from per-component ServiceMonitors.
func buildArgocdOverview() (*dashboardv2.Dashboard, error) {
	const (
		elementAppsNotHealthy         = "apps-not-healthy"
		elementAppsOutOfSync          = "apps-outofsync"
		elementSyncFailures1h         = "sync-failures-1h"
		elementClusterConnectionsDown = "cluster-connections-down"
		elementGitLsRemoteFailures1h  = "git-ls-remote-failures-1h"
		elementAppsByHealthStatus     = "apps-by-health-status"
		elementAppsBySyncStatus       = "apps-by-sync-status"
		elementAppsNeedingAttention   = "apps-needing-attention"
		elementSyncActivity           = "sync-activity"
		elementAppReconcileLatency    = "app-reconcile-latency"
		elementGitRequests            = "git-requests"
		elementGitRequestDurationP95  = "git-request-duration-p95"
	)

	const clusterFilter = `cluster=~"$cluster"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()

	d := dashboardv2.NewDashboardBuilder("ArgoCD Overview").
		Tags([]string{"argocd", "gitops", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-24h").To("now").AutoRefresh("1m")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			Query(promVariableQuery(`label_values(argocd_app_info, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	d.Panel(elementAppsNotHealthy, dashboardv2.NewPanelBuilder().
		Id(argocdPanelAppsNotHealthy).
		Title("Apps Not Healthy").
		Description("Applications whose health status is not Healthy.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count(argocd_app_info{`+clusterFilter+`,health_status!="Healthy"}) or vector(0)`).
				LegendFormat("Not Healthy")))))

	d.Panel(elementAppsOutOfSync, dashboardv2.NewPanelBuilder().
		Id(argocdPanelAppsOutOfSync).
		Title("Apps OutOfSync").
		Description("Applications whose sync status is not Synced.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count(argocd_app_info{`+clusterFilter+`,sync_status!="Synced"}) or vector(0)`).
				LegendFormat("OutOfSync")))))

	d.Panel(elementSyncFailures1h, dashboardv2.NewPanelBuilder().
		Id(argocdPanelSyncFailures1h).
		Title("Sync Failures (1h)").
		Description("Sync operations that ended in Error or Failed in the last hour.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`ceil(sum(increase(argocd_app_sync_total{`+clusterFilter+`,phase=~"Error|Failed"}[1h]))) or vector(0)`).
				LegendFormat("Failures")))))

	// Second line covers Argo CD dependency reachability.
	d.Panel(elementClusterConnectionsDown, dashboardv2.NewPanelBuilder().
		Id(argocdPanelClusterConnectionsDown).
		Title("Cluster Connections Down").
		Description("Destination clusters the application-controller cannot reach.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count(argocd_cluster_connection_status{`+clusterFilter+`} == 0) or vector(0)`).
				LegendFormat("Down")))))

	d.Panel(elementGitLsRemoteFailures1h, dashboardv2.NewPanelBuilder().
		Id(argocdPanelGitLsRemoteFailures1h).
		Title("Git ls-remote Failures (1h)").
		Description("Failed git ls-remote calls from the repo-server in the last hour. Nonzero means ArgoCD could not reach the Git remote, so every application stops seeing new commits.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				// Drop restart-specific local clone paths from the repo label.
				Expr(`ceil(sum(increase(argocd_git_lsremote_fail_total{`+clusterFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Failures")))))

	d.Panel(elementAppsByHealthStatus, dashboardv2.NewPanelBuilder().
		Id(argocdPanelAppsByHealthStatus).
		Title("Apps by Health Status").
		Description("Application count per health status.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Healthy"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* (Degraded|Missing)"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Progressing"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count by (cluster, health_status) (argocd_app_info{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{health_status}}")))))

	d.Panel(elementAppsBySyncStatus, dashboardv2.NewPanelBuilder().
		Id(argocdPanelAppsBySyncStatus).
		Title("Apps by Sync Status").
		Description("Application count per sync status.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Tooltip(tooltipAll).
			Legend(legend).
			FillOpacity(10).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Synced"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* OutOfSync"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "orange"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count by (cluster, sync_status) (argocd_app_info{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{sync_status}}")))))

	d.Panel(elementAppsNeedingAttention, dashboardv2.NewPanelBuilder().
		Id(argocdPanelAppsNeedingAttention).
		Title("Apps Needing Attention").
		Description("Applications that are not Healthy or not Synced. Empty is good.").
		Visualization(table.NewVisualizationV2Builder()).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`argocd_app_info{`+clusterFilter+`,health_status!="Healthy"} or argocd_app_info{`+clusterFilter+`,sync_status!="Synced"}`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable))).Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
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
		}))))

	d.Panel(elementSyncActivity, dashboardv2.NewPanelBuilder().
		Id(argocdPanelSyncActivity).
		Title("Sync Activity").
		Description("Completed sync operations per interval, by result phase. Dry-run syncs are excluded.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			DrawStyle(common.GraphDrawStyleBars).
			// Fill bars solid rather than applying line-area opacity.
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* (Error|Failed)"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Succeeded"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			// Separate targets fix stack order with failures above successes.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`round(sum by (cluster) (increase(argocd_app_sync_total{`+clusterFilter+`,dry_run="false",phase="Succeeded"}[$__interval])))`).
				LegendFormat("{{cluster}} Succeeded"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`round(sum by (cluster, phase) (increase(argocd_app_sync_total{`+clusterFilter+`,dry_run="false",phase=~"Error|Failed"}[$__interval])))`).
				LegendFormat("{{cluster}} {{phase}}"))). // Count sparse sync events per non-overlapping bucket. Round increase()
			// extrapolation and floor bars at ten minutes for readability.
			QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("10m"))))

	d.Panel(elementAppReconcileLatency, dashboardv2.NewPanelBuilder().
		Id(argocdPanelAppReconcileLatency).
		Title("App Reconcile Latency").
		Description("Application reconciliation duration percentiles in the application-controller.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`histogram_quantile(0.99, sum by (cluster, le) (rate(argocd_app_reconcile_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p99"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`histogram_quantile(0.50, sum by (cluster, le) (rate(argocd_app_reconcile_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p50"))). // Floor bursty reconcile histograms so quantiles retain enough samples.
			QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("5m"))))

	d.Panel(elementGitRequests, dashboardv2.NewPanelBuilder().
		Id(argocdPanelGitRequests).
		Title("Git Requests").
		Description("Git requests from the repo-server per interval, by request type. ls-remote is the polling loop and should hold a steady height; a drop to zero means polling stopped.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// Separate request types to keep rare fetch bars above ls-remote.
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`round(sum by (cluster) (increase(argocd_git_request_total{`+clusterFilter+`,request_type="ls-remote"}[$__interval])))`).
				LegendFormat("{{cluster}} ls-remote"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`round(sum by (cluster) (increase(argocd_git_request_total{`+clusterFilter+`,request_type="fetch"}[$__interval])))`).
				LegendFormat("{{cluster}} fetch"))). // Count sparse Git requests per bucket so rare fetches remain visible.
			QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("10m"))))

	d.Panel(elementGitRequestDurationP95, dashboardv2.NewPanelBuilder().
		Id(argocdPanelGitRequestDurationP95).
		Title("Git Request Duration p95").
		Description("95th percentile git request duration, by request type. Slow fetches point at repo size or network issues. Git requests are sparse, so samples are drawn as points; a gap means no request occurred, not a failure.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			ShowPoints(common.VisibilityModeAlways).
			PointSize(6).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`histogram_quantile(0.95, sum by (cluster, request_type, le) (rate(argocd_git_request_duration_seconds_bucket{`+clusterFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} {{request_type}}"))). // Floor sparse Git latency windows; show naturally sparse fetches as points.
			QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("15m"))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementAppsNotHealthy).
			WithItem(elementAppsOutOfSync).
			WithItem(elementSyncFailures1h).
			WithItem(elementClusterConnectionsDown).
			WithItem(elementGitLsRemoteFailures1h))).
		Row(dashboardv2.NewRowBuilder().Title("Applications").AutoGridLayout(chartGrid().
			WithItem(elementAppsByHealthStatus).
			WithItem(elementAppsBySyncStatus))).
		Row(dashboardv2.NewRowBuilder().Title("Apps Needing Attention").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementAppsNeedingAttention))).
		Row(dashboardv2.NewRowBuilder().Title("Sync & Reconcile").AutoGridLayout(chartGrid().
			WithItem(elementSyncActivity).
			WithItem(elementAppReconcileLatency))).
		Row(dashboardv2.NewRowBuilder().Title("repo-server").AutoGridLayout(chartGrid().
			WithItem(elementGitRequests).
			WithItem(elementGitRequestDurationP95)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
