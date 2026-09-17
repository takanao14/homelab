package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func buildCertManager() (*dashboardv2.Dashboard, error) {
	ds := dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("$datasource")
	tooltip := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
	legend := common.NewVizLegendOptionsBuilder().ShowLegend(true).DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom)
	issueThresholds := dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "green"}, {Value: new(float64(1)), Color: "red"}})
	targetThresholds := dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{{Value: nil, Color: "red"}, {Value: new(float64(1)), Color: "green"}})
	targetMappings := []dashboardv2.ValueMapping{{ValueMap: &dashboardv2.ValueMap{Type: "value", Options: map[string]dashboardv2.ValueMappingResult{"0": {Text: new("DOWN"), Color: new("red")}, "1": {Text: new("UP"), Color: new("green")}}}}}
	expiryThresholds := map[string]any{"mode": "absolute", "steps": []map[string]any{{"value": nil, "color": "red"}, {"value": 7.0, "color": "orange"}, {"value": 21.0, "color": "green"}}}
	readyThresholds := map[string]any{"mode": "absolute", "steps": []map[string]any{{"value": nil, "color": "red"}, {"value": 1.0, "color": "green"}}}
	readyMappings := []map[string]any{{"type": "value", "options": map[string]any{"0": map[string]any{"text": "Not Ready", "index": 0}, "1": map[string]any{"text": "Ready", "index": 1}}}}

	const clusterFilter = `cluster=~"$cluster"`
	d := dashboardv2.NewDashboardBuilder("cert-manager Overview V2 PoC").
		Tags([]string{"cert-manager", "certificates", "infrastructure", "poc"}).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-7d").To("now").AutoRefresh("5m")).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("datasource").Label("Datasource").PluginId("prometheus")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").Label("Cluster").Query(prometheusVariableQuery{"$datasource", `label_values(kube_node_info, cluster)`}).Definition(`label_values(kube_node_info, cluster)`).Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).Sort(dashboardv2.VariableSortAlphabeticalAsc).Multi(true).IncludeAll(true))

	d.Panel("panel-2", dashboardv2.NewPanelBuilder().Id(2).Title("Scrape Target").Description("Whether Prometheus can reach the cert-manager metrics endpoint. No data means the cert-manager ArgoCD application is not present in this cluster.").Visualization(stat.NewVisualizationV2Builder().Thresholds(targetThresholds).Mappings(targetMappings).ColorMode(common.BigValueColorModeBackground).GraphMode(common.BigValueGraphModeNone)).Data(promInstant(ds, `min by (cluster) (up{job="cert-manager",`+clusterFilter+`}) or (0 * max by (cluster) (argocd_app_info{`+clusterFilter+`,name="cert-manager"}))`, "Target")))
	d.Panel("panel-3", issueStat(3, "Certs Not Ready", "Certificates where the Ready condition is not True.", ds, `count(certmanager_certificate_ready_status{`+clusterFilter+`,condition="True"} != 1) or vector(0)`, "Not Ready", issueThresholds))
	d.Panel("panel-4", issueStat(4, "ClusterIssuers Not Ready", "ClusterIssuers where the Ready condition is not True.", ds, `count(certmanager_clusterissuer_ready_status{`+clusterFilter+`,condition="True"} != 1) or vector(0)`, "Not Ready", issueThresholds))
	d.Panel("panel-5", issueStat(5, "Sync Errors (1h)", "cert-manager controller reconciliation errors in the last hour.", ds, `ceil(sum(increase(certmanager_controller_sync_error_count{`+clusterFilter+`}[1h]))) or vector(0)`, "Errors", issueThresholds))
	d.Panel("panel-6", issueStat(6, "ACME Errors", "Non-2xx responses from the ACME endpoint over the dashboard time range, including Let's Encrypt rate limits. Nonzero means issuance or renewal is being rejected, well before the expiry countdown reflects it.", ds, `ceil(sum(increase(certmanager_http_acme_client_request_count{`+clusterFilter+`,status=~"4..|5.."}[$__range]))) or vector(0)`, "Errors", issueThresholds))

	certificateViz := table.NewVisualizationV2Builder().
		OverrideByName("Days Until Expiry", []dashboardv2.DynamicConfigValue{{Id: "unit", Value: "d"}, {Id: "decimals", Value: 1}, {Id: "thresholds", Value: expiryThresholds}, {Id: "custom.displayMode", Value: "color-background"}}).
		OverrideByName("Days Until Renewal", []dashboardv2.DynamicConfigValue{{Id: "unit", Value: "d"}, {Id: "decimals", Value: 1}}).
		OverrideByName("Ready", []dashboardv2.DynamicConfigValue{{Id: "decimals", Value: 0}, {Id: "thresholds", Value: readyThresholds}, {Id: "custom.displayMode", Value: "color-background"}, {Id: "mappings", Value: readyMappings}})
	d.Panel("panel-8", dashboardv2.NewPanelBuilder().Id(8).Title("Certificate Status").Description("Expiry, renewal schedule, and ready state for each certificate. Days Until Renewal shows when cert-manager will begin renewal attempts; negative means renewal is already in progress.").Visualization(certificateViz).Data(dashboardv2.NewQueryGroupBuilder().
		Target(promTarget("A", ds, `sort((certmanager_certificate_expiration_timestamp_seconds{`+clusterFilter+`} - time()) / 86400)`, "", true, prometheus.PromQueryFormatTable)).
		Target(promTarget("B", ds, `sort((certmanager_certificate_renewal_timestamp_seconds{`+clusterFilter+`} - time()) / 86400)`, "", true, prometheus.PromQueryFormatTable)).
		Target(promTarget("C", ds, `certmanager_certificate_ready_status{`+clusterFilter+`,condition="True"}`, "", true, prometheus.PromQueryFormatTable)).
		Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).
		Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
			"excludeByName": map[string]any{"Time": true, "container": true, "endpoint": true, "instance": true, "pod": true, "service": true, "namespace": true, "prometheus": true, "prometheus_replica": true, "condition": true, "issuer_kind": true},
			"indexByName":   map[string]any{"cluster": 0, "exported_namespace": 1, "name": 2, "issuer_name": 3, "Value #C": 4, "Value #A": 5, "Value #B": 6},
			"renameByName":  map[string]any{"exported_namespace": "Namespace", "name": "Certificate", "issuer_name": "Issuer", "Value #C": "Ready", "Value #A": "Days Until Expiry", "Value #B": "Days Until Renewal"},
		}))))

	issuerViz := table.NewVisualizationV2Builder().OverrideByName("Ready", []dashboardv2.DynamicConfigValue{{Id: "decimals", Value: 0}, {Id: "thresholds", Value: readyThresholds}, {Id: "custom.displayMode", Value: "color-background"}, {Id: "mappings", Value: readyMappings}})
	d.Panel("panel-9", dashboardv2.NewPanelBuilder().Id(9).Title("ClusterIssuer Status").Description("Ready state of each ClusterIssuer.").Visualization(issuerViz).Data(dashboardv2.NewQueryGroupBuilder().
		Target(promTarget("A", ds, `certmanager_clusterissuer_ready_status{`+clusterFilter+`,condition="True"}`, "{{cluster}} {{name}}", true, prometheus.PromQueryFormatTable)).
		Transformation(dashboardv2.NewTransformationBuilder().Group("merge").Options(map[string]any{})).
		Transformation(dashboardv2.NewTransformationBuilder().Group("organize").Options(map[string]any{
			"excludeByName": map[string]any{"Time": true, "container": true, "endpoint": true, "instance": true, "pod": true, "service": true, "namespace": true, "prometheus": true, "prometheus_replica": true, "condition": true},
			"indexByName":   map[string]any{"cluster": 0, "name": 1, "Value": 2},
			"renameByName":  map[string]any{"name": "Issuer", "Value": "Ready"},
		}))))
	d.Panel("panel-10", dashboardv2.NewPanelBuilder().Id(10).Title("Days Until Expiry").Description("Certificate expiry countdown. A jump upward indicates a successful renewal; below zero the certificate has already expired.").Visualization(timeseries.NewVisualizationV2Builder().Unit("d").AxisSoftMin(0).Tooltip(tooltip).Legend(legend)).Data(promData(ds, `(max by (cluster, exported_namespace, name) (certmanager_certificate_expiration_timestamp_seconds{`+clusterFilter+`}) - time()) / 86400`, "{{cluster}} {{exported_namespace}}/{{name}}")))

	d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").GridLayout(dashboardv2.NewGridBuilder().Item(dashboardv2.GridItem("panel-2").X(0).Y(0).Width(8).Height(4)).Item(dashboardv2.GridItem("panel-3").X(8).Y(0).Width(8).Height(4)).Item(dashboardv2.GridItem("panel-4").X(16).Y(0).Width(8).Height(4)).Item(dashboardv2.GridItem("panel-5").X(0).Y(4).Width(12).Height(4)).Item(dashboardv2.GridItem("panel-6").X(12).Y(4).Width(12).Height(4)))).
		Row(dashboardv2.NewRowBuilder().Title("Certificates & Issuers").GridLayout(dashboardv2.NewGridBuilder().Item(dashboardv2.GridItem("panel-8").X(0).Y(0).Width(16).Height(6)).Item(dashboardv2.GridItem("panel-9").X(16).Y(0).Width(8).Height(6)).Item(dashboardv2.GridItem("panel-10").X(0).Y(6).Width(24).Height(6)))))
	result, err := d.Build()
	return &result, err
}

func issueStat(id float64, title, description string, ds *dashboardv2.Dashboardv2DataQueryKindDatasourceBuilder, expr, legend string, thresholds *dashboardv2.ThresholdsConfigBuilder) *dashboardv2.PanelBuilder {
	return dashboardv2.NewPanelBuilder().Id(id).Title(title).Description(description).Visualization(stat.NewVisualizationV2Builder().Unit("short").Min(0).Thresholds(thresholds).ColorMode(common.BigValueColorModeBackground).Orientation(common.VizOrientationAuto)).Data(promData(ds, expr, legend))
}

func promTarget(refID string, ds *dashboardv2.Dashboardv2DataQueryKindDatasourceBuilder, expr, legend string, instant bool, format prometheus.PromQueryFormat) *dashboardv2.TargetBuilder {
	query := prometheus.NewQueryV2Builder().Datasource(ds).Expr(expr).Instant(instant).Range(!instant).Format(format)
	if legend != "" {
		query.LegendFormat(legend)
	}
	return dashboardv2.NewTargetBuilder().RefId(refID).Query(query)
}

func promInstant(ds *dashboardv2.Dashboardv2DataQueryKindDatasourceBuilder, expr, legend string) *dashboardv2.QueryGroupBuilder {
	return dashboardv2.NewQueryGroupBuilder().Target(dashboardv2.NewTargetBuilder().RefId("A").Query(prometheus.NewQueryV2Builder().Datasource(ds).Expr(expr).LegendFormat(legend).Instant(true).Range(false)))
}
