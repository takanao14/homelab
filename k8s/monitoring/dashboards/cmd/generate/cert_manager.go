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
	targetThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})
	targetMappings := []dashboard.ValueMapping{
		{ValueMap: &dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
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
				Query(dashboard.StringOrMap{String: new(`label_values(kube_node_info, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Scrape Target").
				Description("Whether Prometheus can reach the cert-manager metrics endpoint. No data means the cert-manager ArgoCD application is not present in this cluster.").
				Datasource(ds).
				Span(8).Height(4).
				Thresholds(targetThresholds).
				Mappings(targetMappings).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone).
				WithTarget(prometheus.NewDataqueryBuilder().
					// ArgoCD defines whether this cluster is expected to run cert-manager.
					// The zero fallback catches a target that disappears from service
					// discovery without marking clusters that intentionally omit the app
					// as DOWN.
					Expr(`min by (cluster) (up{job="cert-manager",` + clusterFilter + `}) or (0 * max by (cluster) (argocd_app_info{` + clusterFilter + `,name="cert-manager"}))`).
					Instant().
					LegendFormat("Target"),
				),
		).
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
				Span(12).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(certmanager_controller_sync_error_count{` + clusterFilter + `}[1h]))) or vector(0)`).
					LegendFormat("Errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ACME Errors").
				Description("Non-2xx responses from the ACME endpoint over the dashboard time range, including Let's Encrypt rate limits. Nonzero means issuance or renewal is being rejected, well before the expiry countdown reflects it.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// $__range rather than the fixed [1h] its neighbour uses:
					// a 90-day certificate only talks to ACME every couple of
					// months, so an hour of history is almost always empty and
					// says nothing. Following the time picker lets the panel
					// answer "has ACME rejected us lately" at whatever range the
					// operator is already looking at.
					Expr(`ceil(sum(increase(certmanager_http_acme_client_request_count{` + clusterFilter + `,status=~"4..|5.."}[$__range]))) or vector(0)`).
					LegendFormat("Errors"),
				),
		).
		// One row for both tables: a certificate is only as healthy as the
		// issuer that signs it, so they are read together. The 16/8 split
		// follows their real density -- seven columns against three -- rather
		// than giving the issuer table a full-width row of its own for what is
		// a two-line answer.
		WithRow(dashboard.NewRowBuilder("Certificates & Issuers")).
		WithPanel(
			table.NewPanelBuilder().
				Title("Certificate Status").
				Description("Expiry, renewal schedule, and ready state for each certificate. Days Until Renewal shows when cert-manager will begin renewal attempts; negative means renewal is already in progress.").
				Datasource(ds).
				Span(16).Height(6).
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
			table.NewPanelBuilder().
				Title("ClusterIssuer Status").
				Description("Ready state of each ClusterIssuer.").
				Datasource(ds).
				Span(8).Height(6).
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
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Days Until Expiry").
				Description("Certificate expiry countdown. A jump upward indicates a successful renewal; below zero the certificate has already expired.").
				Datasource(ds).
				Span(24).Height(6).
				Unit("d").
				// Soft, not a hard Min(0): zero is the meaningful origin for a
				// countdown and the axis should stay anchored there rather than
				// auto-zooming into the top of the range, but an expired
				// certificate goes negative and a hard floor would push exactly
				// that case off the bottom of the chart.
				AxisSoftMin(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Aggregated down to the labels the legend actually prints.
					// The raw metric also carries pod and instance, so every
					// cert-manager pod replacement starts a fresh series: one
					// certificate showed up as four identically-named, broken
					// line segments across 7 days. max() over the group also
					// absorbs the moment during a rollout when two pods report
					// the same certificate at once.
					Expr(`(max by (cluster, exported_namespace, name) (certmanager_certificate_expiration_timestamp_seconds{` + clusterFilter + `}) - time()) / 86400`).
					LegendFormat("{{cluster}} {{exported_namespace}}/{{name}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
