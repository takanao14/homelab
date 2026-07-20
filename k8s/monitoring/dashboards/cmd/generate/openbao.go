package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildOpenbaoOverview defines the OpenBao (external VM, 192.168.40.30) health
// dashboard: seal/active status, request rate and latency, raft storage
// performance, and lease/token inventory.
//
// OpenBao keeps the vault_* metric prefix for compatibility, and its metrics
// carry their own cluster label (the raft cluster name, not the Kubernetes
// environment name), so
// panels filter on the scrape job only — no $cluster variable here.
// Latency summaries (vault_core_handle_request, vault_raft_*) are emitted in
// milliseconds.
func buildOpenbaoOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const openbao = `job="scrapeConfig/monitoring/openbao"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// downThresholds colors boolean up/unsealed/active stats: red for 0, green for 1.
	downThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})

	d, err := dashboard.NewDashboardBuilder("OpenBao Overview").
		Uid("openbao-overview").
		Tags([]string{"openbao", "secrets", "infrastructure"}).
		Timezone("browser").
		Time("now-24h", "now").
		Refresh("1m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(promDatasourceVariable()).
		WithRow(dashboard.NewRowBuilder("Status")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Scrape Target").
				Description("Whether Prometheus can reach the OpenBao metrics endpoint.").
				Datasource(ds).
				Span(6).Height(4).
				Thresholds(downThresholds).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: new("DOWN"), Color: new("red")},
							"1": {Text: new("UP"), Color: new("green")},
						},
					}},
				}).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`up{` + openbao + `}`).
					Instant().
					LegendFormat("Up"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Seal Status").
				Description("OpenBao seal status. Sealed means all secret operations fail until unsealed (static seal auto-unseals on restart).").
				Datasource(ds).
				Span(6).Height(4).
				Thresholds(downThresholds).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: new("Sealed"), Color: new("red")},
							"1": {Text: new("Unsealed"), Color: new("green")},
						},
					}},
				}).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_core_unsealed{` + openbao + `}`).
					Instant().
					LegendFormat("Seal"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("HA Mode").
				Description("Whether this node is the active node (single-node raft: always Active when healthy).").
				Datasource(ds).
				Span(6).Height(4).
				Thresholds(downThresholds).
				Mappings([]dashboard.ValueMapping{
					{ValueMap: &dashboard.ValueMap{
						Type: dashboard.MappingTypeValueToText,
						Options: map[string]dashboard.ValueMappingResult{
							"0": {Text: new("Standby"), Color: new("yellow")},
							"1": {Text: new("Active"), Color: new("green")},
						},
					}},
				}).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_core_active{` + openbao + `}`).
					Instant().
					LegendFormat("Active"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Service Tokens").
				Description("Number of live service tokens in the token store.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").Min(0).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(vault_token_count{` + openbao + `})`).
					Instant().
					LegendFormat("Tokens"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Requests")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Request Rate").
				Description("Core request and login-request throughput.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(vault_core_handle_request_count{` + openbao + `}[$__rate_interval])`).
					LegendFormat("requests"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(vault_core_handle_login_request_count{` + openbao + `}[$__rate_interval])`).
					LegendFormat("logins"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Request Latency").
				Description("Core request handling latency percentiles (summary quantiles; gaps mean no requests in the window).").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_core_handle_request{` + openbao + `,quantile=~"0.5|0.9|0.99"}`).
					LegendFormat("p{{quantile}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Raft Storage")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Raft Apply Rate").
				Description("Raft log entries applied to the FSM per second (write activity).").
				Datasource(ds).
				Span(8).Height(8).
				Unit("ops").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(vault_raft_apply{` + openbao + `}[$__rate_interval])`).
					LegendFormat("applies"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Raft Commit Time").
				Description("Time to commit a raft log entry (summary quantiles).").
				Datasource(ds).
				Span(8).Height(8).
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_raft_commitTime{` + openbao + `,quantile=~"0.5|0.9|0.99"}`).
					LegendFormat("p{{quantile}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("BoltDB Store Logs Time").
				Description("Time to persist raft log entries to BoltDB — the usual disk-latency bottleneck.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_raft_boltdb_storeLogs{` + openbao + `,quantile=~"0.5|0.9|0.99"}`).
					LegendFormat("p{{quantile}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Leases & Identity")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Leases").
				Description("Live leases in the expiration manager. Irrevocable leases indicate revocation failures.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_expire_num_leases{`+openbao+`}`).
					LegendFormat("leases"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_expire_num_irrevocable_leases{`+openbao+`}`).
					LegendFormat("irrevocable"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byName", Options: "irrevocable"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Tokens & Identity Entities").
				Description("Service token count and identity entities over time. A runaway token count points at a login loop (e.g. an ESO auth misconfiguration).").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(vault_token_count{` + openbao + `})`).
					LegendFormat("tokens"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_identity_num_entities{` + openbao + `}`).
					LegendFormat("entities"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
