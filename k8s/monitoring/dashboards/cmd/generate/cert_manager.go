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
	certManagerPanelScrapeTarget           = 1
	certManagerPanelCertsNotReady          = 2
	certManagerPanelClusterIssuersNotReady = 3
	certManagerPanelSyncErrors1h           = 4
	certManagerPanelACMEErrors             = 5
	certManagerPanelCertificateStatus      = 6
	certManagerPanelClusterIssuerStatus    = 7
	certManagerPanelDaysUntilExpiry        = 8
)

// buildCertManagerOverview covers certificate and issuer health; expiry jumps
// upward after successful renewal.
func buildCertManagerOverview() (*dashboardv2.Dashboard, error) {
	const (
		elementScrapeTarget           = "scrape-target"
		elementCertsNotReady          = "certs-not-ready"
		elementClusterIssuersNotReady = "clusterissuers-not-ready"
		elementSyncErrors1h           = "sync-errors-1h"
		elementACMEErrors             = "acme-errors"
		elementCertificateStatus      = "certificate-status"
		elementClusterIssuerStatus    = "clusterissuer-status"
		elementDaysUntilExpiry        = "days-until-expiry"
	)

	const clusterFilter = `cluster=~"$cluster"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	targetThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})
	targetMappings := []dashboardv2.ValueMapping{
		{ValueMap: &dashboardv2.ValueMap{
			Type: dashboardv2.MappingTypeValue,
			Options: map[string]dashboardv2.ValueMappingResult{
				"0": {Text: new("DOWN"), Color: new("red")},
				"1": {Text: new("UP"), Color: new("green")},
			},
		}},
	}

	// expiryThresholds colors the "Days Until Expiry" table column:
	// red below 7 d (critical), orange below 21 d (renewal failing), green otherwise.
	expiryThresholds := map[string]any{
		"mode": "absolute",
		"steps": []map[string]any{
			{"value": nil, "color": "red"},
			{"value": 7.0, "color": "orange"},
			{"value": 21.0, "color": "green"},
		},
	}
	// readyThresholds colors boolean-style 0/1 columns: red for 0, green for 1.
	readyThresholds := map[string]any{
		"mode": "absolute",
		"steps": []map[string]any{
			{"value": nil, "color": "red"},
			{"value": 1.0, "color": "green"},
		},
	}
	// readyMappings translates 0→"Not Ready", 1→"Ready" in table cells.
	readyMappings := []map[string]any{
		{
			"type": "value",
			"options": map[string]any{
				"0": map[string]any{"text": "Not Ready", "index": 0},
				"1": map[string]any{"text": "Ready", "index": 1},
			},
		},
	}

	d := dashboardv2.NewDashboardBuilder("cert-manager Overview").
		Tags([]string{"cert-manager", "certificates", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-7d").To("now").AutoRefresh("5m")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			Query(promVariableQuery(`label_values(kube_node_info, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	d.Panel(elementScrapeTarget, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelScrapeTarget).
		Title("Scrape Target").
		Description("Whether Prometheus can reach the cert-manager metrics endpoint. No data means the cert-manager ArgoCD application is not present in this cluster.").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(targetThresholds).
			Mappings(targetMappings).
			ColorMode(common.BigValueColorModeBackground).
			GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Use Argo CD presence to distinguish a missing target from a cluster that
				// intentionally omits cert-manager.
				Expr(`min by (cluster) (up{job="cert-manager",`+clusterFilter+`}) or (0 * max by (cluster) (argocd_app_info{`+clusterFilter+`,name="cert-manager"}))`).
				Instant(true).Range(false).
				LegendFormat("Target")))))

	d.Panel(elementCertsNotReady, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelCertsNotReady).
		Title("Certs Not Ready").
		Description("Certificates where the Ready condition is not True.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(certmanager_certificate_ready_status{`+clusterFilter+`,condition="True"} != 1) or vector(0)`).
				LegendFormat("Not Ready")))))

	d.Panel(elementClusterIssuersNotReady, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelClusterIssuersNotReady).
		Title("ClusterIssuers Not Ready").
		Description("ClusterIssuers where the Ready condition is not True.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(certmanager_clusterissuer_ready_status{`+clusterFilter+`,condition="True"} != 1) or vector(0)`).
				LegendFormat("Not Ready")))))

	d.Panel(elementSyncErrors1h, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelSyncErrors1h).
		Title("Sync Errors (1h)").
		Description("cert-manager controller reconciliation errors in the last hour.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(certmanager_controller_sync_error_count{`+clusterFilter+`}[1h]))) or vector(0)`).
				LegendFormat("Errors")))))

	d.Panel(elementACMEErrors, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelACMEErrors).
		Title("ACME Errors").
		Description("Non-2xx responses from the ACME endpoint over the dashboard time range, including Let's Encrypt rate limits. Nonzero means issuance or renewal is being rejected, well before the expiry countdown reflects it.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Follow the selected range because ACME activity is months apart.
				Expr(`ceil(sum(increase(certmanager_http_acme_client_request_count{`+clusterFilter+`,status=~"4..|5.."}[$__range]))) or vector(0)`).
				LegendFormat("Errors")))))

	d.Panel(elementCertificateStatus, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelCertificateStatus).
		Title("Certificate Status").
		Description("Expiry, renewal schedule, and ready state for each certificate. Days Until Renewal shows when cert-manager will begin renewal attempts; negative means renewal is already in progress.").
		Visualization(table.NewVisualizationV2Builder().
			OverrideByName("Days Until Expiry", []dashboardv2.DynamicConfigValue{
				{Id: "unit", Value: "d"},
				{Id: "decimals", Value: 1},
				{Id: "thresholds", Value: expiryThresholds},
				{Id: "custom.cellOptions", Value: common.NewTableColoredBackgroundCellOptions()},
			}).
			OverrideByName("Days Until Renewal", []dashboardv2.DynamicConfigValue{
				{Id: "unit", Value: "d"},
				{Id: "decimals", Value: 1},
			}).
			OverrideByName("Ready", []dashboardv2.DynamicConfigValue{
				{Id: "decimals", Value: 0},
				{Id: "thresholds", Value: readyThresholds},
				{Id: "custom.cellOptions", Value: common.NewTableColoredBackgroundCellOptions()},
				{Id: "mappings", Value: readyMappings},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort((certmanager_certificate_expiration_timestamp_seconds{`+clusterFilter+`} - time()) / 86400)`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sort((certmanager_certificate_renewal_timestamp_seconds{`+clusterFilter+`} - time()) / 86400)`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`certmanager_certificate_ready_status{`+clusterFilter+`,condition="True"}`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable))).
			Transformation(dashboardv2.NewTransformationBuilder().
				Group("merge").
				Options(map[string]any{})).
			Transformation(dashboardv2.NewTransformationBuilder().
				Group("organize").
				Options(map[string]any{
					"excludeByName": map[string]any{
						"Time":               true,
						"container":          true,
						"endpoint":           true,
						"instance":           true,
						"pod":                true,
						"service":            true,
						"namespace":          true,
						"prometheus":         true,
						"prometheus_replica": true,
						"condition":          true,
						"issuer_kind":        true,
					},
					"indexByName": map[string]any{
						"cluster":            0,
						"exported_namespace": 1,
						"name":               2,
						"issuer_name":        3,
						"Value #C":           4,
						"Value #A":           5,
						"Value #B":           6,
					},
					"renameByName": map[string]any{
						"exported_namespace": "Namespace",
						"name":               "Certificate",
						"issuer_name":        "Issuer",
						"Value #C":           "Ready",
						"Value #A":           "Days Until Expiry",
						"Value #B":           "Days Until Renewal",
					},
				}))))

	d.Panel(elementClusterIssuerStatus, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelClusterIssuerStatus).
		Title("ClusterIssuer Status").
		Description("Ready state of each ClusterIssuer.").
		Visualization(table.NewVisualizationV2Builder().
			OverrideByName("Ready", []dashboardv2.DynamicConfigValue{
				{Id: "decimals", Value: 0},
				{Id: "thresholds", Value: readyThresholds},
				{Id: "custom.cellOptions", Value: common.NewTableColoredBackgroundCellOptions()},
				{Id: "mappings", Value: readyMappings},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`certmanager_clusterissuer_ready_status{`+clusterFilter+`,condition="True"}`).
				Instant(true).Range(false).Format(prometheus.PromQueryFormatTable).
				LegendFormat("{{cluster}} {{name}}"))).
			Transformation(dashboardv2.NewTransformationBuilder().
				Group("merge").
				Options(map[string]any{})).
			Transformation(dashboardv2.NewTransformationBuilder().
				Group("organize").
				Options(map[string]any{
					"excludeByName": map[string]any{
						"Time":               true,
						"container":          true,
						"endpoint":           true,
						"instance":           true,
						"pod":                true,
						"service":            true,
						"namespace":          true,
						"prometheus":         true,
						"prometheus_replica": true,
						"condition":          true,
					},
					"indexByName": map[string]any{
						"cluster": 0,
						"name":    1,
						"Value":   2,
					},
					"renameByName": map[string]any{
						"name":  "Issuer",
						"Value": "Ready",
					},
				}))))

	d.Panel(elementDaysUntilExpiry, dashboardv2.NewPanelBuilder().
		Id(certManagerPanelDaysUntilExpiry).
		Title("Days Until Expiry").
		Description("Certificate expiry countdown. A jump upward indicates a successful renewal; below zero the certificate has already expired.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("d").
			// Anchor at zero while allowing expired certificates below it.
			AxisSoftMin(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Aggregate pod churn to legend labels; max also deduplicates rollouts.
				Expr(`(max by (cluster, exported_namespace, name) (certmanager_certificate_expiration_timestamp_seconds{`+clusterFilter+`}) - time()) / 86400`).
				LegendFormat("{{cluster}} {{exported_namespace}}/{{name}}")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(3).
			WithItem(elementScrapeTarget).
			WithItem(elementCertsNotReady).
			WithItem(elementClusterIssuersNotReady))).
		Row(dashboardv2.NewRowBuilder().Title("Errors").AutoGridLayout(statGrid(2).
			WithItem(elementSyncErrors1h).
			WithItem(elementACMEErrors))).
		// Keep certificate and issuer tables adjacent with room for dense columns.
		Row(dashboardv2.NewRowBuilder().Title("Certificates & Issuers").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementCertificateStatus).
			WithItem(elementClusterIssuerStatus))).
		Row(dashboardv2.NewRowBuilder().Title("Days Until Expiry").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementDaysUntilExpiry)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
