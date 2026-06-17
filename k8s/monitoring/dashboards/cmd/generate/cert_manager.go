package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildCertManagerOverview defines cert-manager certificate and issuer health.
// The timeseries shows the expiry countdown — a jump upward indicates a successful renewal.
func buildCertManagerOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const clusterFilter = `cluster=~"$cluster"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()

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

	d, err := dashboard.NewDashboardBuilder("cert-manager Overview").
		Uid("cert-manager-overview").
		Tags([]string{"cert-manager", "certificates", "infrastructure"}).
		Timezone("browser").
		Time("now-7d", "now").
		Refresh("5m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(promDatasourceVariable()).
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
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Certs Not Ready").
				Description("Certificates where the Ready condition is not True.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(certmanager_certificate_ready_status{` + clusterFilter + `,condition="True"} != 1) or vector(0)`).
					LegendFormat("Not Ready"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ClusterIssuers Not Ready").
				Description("ClusterIssuers where the Ready condition is not True.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(certmanager_clusterissuer_ready_status{` + clusterFilter + `,condition="True"} != 1) or vector(0)`).
					LegendFormat("Not Ready"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Sync Errors (1h)").
				Description("cert-manager controller reconciliation errors in the last hour.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(certmanager_controller_sync_error_count{` + clusterFilter + `}[1h]))) or vector(0)`).
					LegendFormat("Errors"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Certificates")).
		WithPanel(
			table.NewPanelBuilder().
				Title("Certificate Status").
				Description("Expiry, renewal schedule, and ready state for each certificate. Days Until Renewal shows when cert-manager will begin renewal attempts; negative means renewal is already in progress.").
				Datasource(ds).
				Span(24).Height(10).
				// A: days until expiry, B: days until renewal trigger, C: ready (0/1)
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort((certmanager_certificate_expiration_timestamp_seconds{`+clusterFilter+`} - time()) / 86400)`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("A"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort((certmanager_certificate_renewal_timestamp_seconds{`+clusterFilter+`} - time()) / 86400)`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("B"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`certmanager_certificate_ready_status{`+clusterFilter+`,condition="True"}`).
					Instant().Format(prometheus.PromQueryFormatTable).RefId("C"),
				).
				WithTransformation(dashboard.DataTransformerConfig{
					Id:      "merge",
					Options: map[string]any{},
				}).
				WithTransformation(dashboard.DataTransformerConfig{
					Id: "organize",
					Options: map[string]any{
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
					},
				}).
				OverrideByName("Days Until Expiry", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "d"},
					{Id: "decimals", Value: 1},
					{Id: "thresholds", Value: expiryThresholds},
					{Id: "custom.displayMode", Value: "color-background"},
				}).
				OverrideByName("Days Until Renewal", []dashboard.DynamicConfigValue{
					{Id: "unit", Value: "d"},
					{Id: "decimals", Value: 1},
				}).
				OverrideByName("Ready", []dashboard.DynamicConfigValue{
					{Id: "decimals", Value: 0},
					{Id: "thresholds", Value: readyThresholds},
					{Id: "custom.displayMode", Value: "color-background"},
					{Id: "mappings", Value: readyMappings},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Days Until Expiry").
				Description("Certificate expiry countdown. A jump upward indicates a successful renewal.").
				Datasource(ds).
				Span(24).Height(8).
				Unit("d").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`(certmanager_certificate_expiration_timestamp_seconds{` + clusterFilter + `} - time()) / 86400`).
					LegendFormat("{{cluster}} {{exported_namespace}}/{{name}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("ClusterIssuers")).
		WithPanel(
			table.NewPanelBuilder().
				Title("ClusterIssuer Status").
				Description("Ready state of each ClusterIssuer.").
				Datasource(ds).
				Span(24).Height(6).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`certmanager_clusterissuer_ready_status{`+clusterFilter+`,condition="True"}`).
					Instant().Format(prometheus.PromQueryFormatTable).
					LegendFormat("{{cluster}} {{name}}"),
				).
				WithTransformation(dashboard.DataTransformerConfig{
					Id:      "merge",
					Options: map[string]any{},
				}).
				WithTransformation(dashboard.DataTransformerConfig{
					Id: "organize",
					Options: map[string]any{
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
					},
				}).
				OverrideByName("Ready", []dashboard.DynamicConfigValue{
					{Id: "decimals", Value: 0},
					{Id: "thresholds", Value: readyThresholds},
					{Id: "custom.displayMode", Value: "color-background"},
					{Id: "mappings", Value: readyMappings},
				}),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
