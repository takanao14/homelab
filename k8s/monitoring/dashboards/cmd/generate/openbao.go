package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildOpenbaoOverview covers external OpenBao health, requests, Raft, leases,
// and tokens. Vault-compatible metrics retain the vault_* prefix and use their
// own Raft cluster label, so panels filter only by scrape job. Latency is ms.
func buildOpenbaoOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const openbao = `job="scrapeConfig/monitoring/openbao"`

	// usageGauge carries ten-minute background metrics forward for 15 minutes.
	// Per-scrape gauges such as lease count do not need this window.
	usageGauge := func(expr string) string {
		return `last_over_time(` + expr + `[15m])`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// sparseLatency joins NaN gaps under 15 minutes while retaining point markers
	// for real summary samples. Longer quiet periods remain visible.
	spanMillis := float64(15 * 60 * 1000)

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
				Description("Live service tokens in the token store. OpenBao recomputes this every ten minutes rather than on each scrape, so the reading here can be up to that old.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").Min(0).
				Thresholds(measurementThresholds()).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(` + usageGauge(`vault_token_count{`+openbao+`}`) + `)`).
					Instant().
					LegendFormat("Tokens"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Requests")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Requests per Collection Interval").
				Description("Core requests and login requests, as counted by OpenBao over its " +
					"own metrics interval (prometheus_retention_time, 60s). Not a per-second " +
					"rate: the value is already a count and is republished rather than " +
					"accumulated, so a step up means more requests in that interval.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				// OpenBao's go-metrics sink republishes per-interval request counts rather
				// than monotonic counters. Plot raw values and use up{} as a zero baseline
				// when quiet intervals omit the series. sum() aligns label sets.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(vault_core_handle_request_count{` + openbao + `})` +
						` or sum(up{` + openbao + `}) * 0`).
					LegendFormat("requests"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(vault_core_handle_login_request_count{` + openbao + `})` +
						` or sum(up{` + openbao + `}) * 0`).
					LegendFormat("logins"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Request Latency").
				Description("Core request handling latency percentiles. The summary reports NaN for an interval with no requests, so the markers are the real readings and the line only joins them across gaps shorter than 15 minutes.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Float64: &spanMillis}).
				ShowPoints(common.VisibilityModeAlways).
				PointSize(5).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_core_handle_request{` + openbao + `,quantile=~"0.5|0.9|0.99"}`).
					LegendFormat("p{{quantile}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Raft Storage")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Raft Applies per Collection Interval").
				Description("Raft log entries applied to the FSM, as counted by OpenBao over its own metrics interval. Not a per-second rate: the value is already a count, and it is republished rather than accumulated, so a step up means more writes in that interval and not a rising total.").
				Datasource(ds).
				Span(8).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Raft apply is also a republished interval count, so plot it raw and
					// baseline omitted quiet intervals against up{}.
					Expr(`sum(vault_raft_apply{` + openbao + `})` +
						` or sum(up{` + openbao + `}) * 0`).
					LegendFormat("applies"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Raft Commit Time").
				Description("Time to commit a raft log entry. Reported only for intervals that had writes, so the markers are the real readings (see Request Latency).").
				Datasource(ds).
				Span(8).Height(8).
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Float64: &spanMillis}).
				ShowPoints(common.VisibilityModeAlways).
				PointSize(5).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`vault_raft_commitTime{` + openbao + `,quantile=~"0.5|0.9|0.99"}`).
					LegendFormat("p{{quantile}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("BoltDB Store Logs Time").
				Description("Time to persist raft log entries to BoltDB — the usual disk-latency bottleneck. Reported only for intervals that had writes (see Request Latency).").
				Datasource(ds).
				Span(8).Height(8).
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Float64: &spanMillis}).
				ShowPoints(common.VisibilityModeAlways).
				PointSize(5).
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
				Description("Service token count and identity entities over time. Both are recomputed every ten minutes, not per scrape, so each step repeats the last collected value instead of drawing a line between two distant points. A runaway token count points at a login loop (e.g. an ESO auth misconfiguration).").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(` + usageGauge(`vault_token_count{`+openbao+`}`) + `)`).
					LegendFormat("tokens"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(usageGauge(`vault_identity_num_entities{` + openbao + `}`)).
					LegendFormat("entities"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
