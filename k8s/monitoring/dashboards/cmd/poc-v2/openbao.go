package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	openbaoPanelScrapeTarget                     = 1
	openbaoPanelSealStatus                       = 2
	openbaoPanelHAMode                           = 3
	openbaoPanelServiceTokens                    = 4
	openbaoPanelRequestsPerCollectionInterval    = 5
	openbaoPanelRequestLatency                   = 6
	openbaoPanelRaftAppliesPerCollectionInterval = 7
	openbaoPanelRaftCommitTime                   = 8
	openbaoPanelBoltDBStoreLogsTime              = 9
	openbaoPanelLeases                           = 10
	openbaoPanelTokensIdentityEntities           = 11
)

// buildOpenbaoOverview covers external OpenBao health, requests, Raft, leases,
// and tokens. Vault-compatible metrics retain the vault_* prefix and use their
// own Raft cluster label, so panels filter only by scrape job. Latency is ms.
func buildOpenbaoOverview() (*dashboardv2.Dashboard, error) {
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
	downThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})

	d := dashboardv2.NewDashboardBuilder("OpenBao Overview").
		Tags([]string{"openbao", "secrets", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-24h").To("now").AutoRefresh("1m")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable())

	return newLayout(d).
		Row("Status", statGrid(4)).
		Panel(openbaoPanelScrapeTarget, dashboardv2.NewPanelBuilder().
			Title("Scrape Target").
			Description("Whether Prometheus can reach the OpenBao metrics endpoint.").
			Visualization(stat.NewVisualizationV2Builder().
				Thresholds(downThresholds).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("DOWN"), Color: new("red")},
							"1": {Text: new("UP"), Color: new("green")},
						},
					}},
				}).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone)).
			Data(queries(promQuery().
				Expr(`up{`+openbao+`}`).
				Instant(true).Range(false).
				LegendFormat("Up")))).
		Panel(openbaoPanelSealStatus, dashboardv2.NewPanelBuilder().
			Title("Seal Status").
			Description("OpenBao seal status. Sealed means all secret operations fail until unsealed (static seal auto-unseals on restart).").
			Visualization(stat.NewVisualizationV2Builder().
				Thresholds(downThresholds).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("Sealed"), Color: new("red")},
							"1": {Text: new("Unsealed"), Color: new("green")},
						},
					}},
				}).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone)).
			Data(queries(promQuery().
				Expr(`vault_core_unsealed{`+openbao+`}`).
				Instant(true).Range(false).
				LegendFormat("Seal")))).
		Panel(openbaoPanelHAMode, dashboardv2.NewPanelBuilder().
			Title("HA Mode").
			Description("Whether this node is the active node (single-node raft: always Active when healthy).").
			Visualization(stat.NewVisualizationV2Builder().
				Thresholds(downThresholds).
				Mappings([]dashboardv2.ValueMapping{
					{ValueMap: &dashboardv2.ValueMap{
						Type: dashboardv2.MappingTypeValue,
						Options: map[string]dashboardv2.ValueMappingResult{
							"0": {Text: new("Standby"), Color: new("yellow")},
							"1": {Text: new("Active"), Color: new("green")},
						},
					}},
				}).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone)).
			Data(queries(promQuery().
				Expr(`vault_core_active{`+openbao+`}`).
				Instant(true).Range(false).
				LegendFormat("Active")))).
		Panel(openbaoPanelServiceTokens, dashboardv2.NewPanelBuilder().
			Title("Service Tokens").
			Description("Live service tokens in the token store. OpenBao recomputes this every ten minutes rather than on each scrape, so the reading here can be up to that old.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds())).
			Data(queries(promQuery().
				Expr(`sum(`+usageGauge(`vault_token_count{`+openbao+`}`)+`)`).
				Instant(true).Range(false).
				LegendFormat("Tokens")))).
		Row("Requests", chartGrid()).
		Panel(openbaoPanelRequestsPerCollectionInterval, dashboardv2.NewPanelBuilder().
			Title("Requests per Collection Interval").
			Description("Core requests and login requests, as counted by OpenBao over its "+
				"own metrics interval (prometheus_retention_time, 60s). Not a per-second "+
				"rate: the value is already a count and is republished rather than "+
				"accumulated, so a step up means more requests in that interval.").
			Visualization(timeseries.NewVisualizationV2Builder().
					Unit("short").
					Min(0).
					Tooltip(tooltipAll).
					Legend(legend)).
			Data(queries( // OpenBao's go-metrics sink republishes per-interval request counts rather
				// than monotonic counters. Plot raw values and use up{} as a zero baseline
				// when quiet intervals omit the series. sum() aligns label sets.
				promQuery().
					Expr(`sum(vault_core_handle_request_count{`+openbao+`})`+
						` or sum(up{`+openbao+`}) * 0`).
					LegendFormat("requests"),
				promQuery().
					Expr(`sum(vault_core_handle_login_request_count{`+openbao+`})`+
						` or sum(up{`+openbao+`}) * 0`).
					LegendFormat("logins")))).
		Panel(openbaoPanelRequestLatency, dashboardv2.NewPanelBuilder().
			Title("Request Latency").
			Description("Core request handling latency percentiles. The summary reports NaN for an interval with no requests, so the markers are the real readings and the line only joins them across gaps shorter than 15 minutes.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Float64: &spanMillis}).
				ShowPoints(common.VisibilityModeAlways).
				PointSize(5)).
			Data(queries(promQuery().
				Expr(`vault_core_handle_request{`+openbao+`,quantile=~"0.5|0.9|0.99"}`).
				LegendFormat("p{{quantile}}")))).
		Row("Raft Storage", chartGrid()).
		Panel(openbaoPanelRaftAppliesPerCollectionInterval, dashboardv2.NewPanelBuilder().
			Title("Raft Applies per Collection Interval").
			Description("Raft log entries applied to the FSM, as counted by OpenBao over its own metrics interval. Not a per-second rate: the value is already a count, and it is republished rather than accumulated, so a step up means more writes in that interval and not a rising total.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				// Raft apply is also a republished interval count, so plot it raw and
				// baseline omitted quiet intervals against up{}.
				Expr(`sum(vault_raft_apply{`+openbao+`})`+
					` or sum(up{`+openbao+`}) * 0`).
				LegendFormat("applies")))).
		Panel(openbaoPanelRaftCommitTime, dashboardv2.NewPanelBuilder().
			Title("Raft Commit Time").
			Description("Time to commit a raft log entry. Reported only for intervals that had writes, so the markers are the real readings (see Request Latency).").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Float64: &spanMillis}).
				ShowPoints(common.VisibilityModeAlways).
				PointSize(5)).
			Data(queries(promQuery().
				Expr(`vault_raft_commitTime{`+openbao+`,quantile=~"0.5|0.9|0.99"}`).
				LegendFormat("p{{quantile}}")))).
		Panel(openbaoPanelBoltDBStoreLogsTime, dashboardv2.NewPanelBuilder().
			Title("BoltDB Store Logs Time").
			Description("Time to persist raft log entries to BoltDB — the usual disk-latency bottleneck. Reported only for intervals that had writes (see Request Latency).").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ms").
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Float64: &spanMillis}).
				ShowPoints(common.VisibilityModeAlways).
				PointSize(5)).
			Data(queries(promQuery().
				Expr(`vault_raft_boltdb_storeLogs{`+openbao+`,quantile=~"0.5|0.9|0.99"}`).
				LegendFormat("p{{quantile}}")))).
		Row("Leases & Identity", chartGrid()).
		Panel(openbaoPanelLeases, dashboardv2.NewPanelBuilder().
			Title("Leases").
			Description("Live leases in the expiration manager. Irrevocable leases indicate revocation failures.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Tooltip(tooltipAll).
				Legend(legend).
				Override(dashboardv2.MatcherConfig{Id: "byName", Options: "irrevocable"}, []dashboardv2.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				})).
			Data(queries(promQuery().
				Expr(`vault_expire_num_leases{`+openbao+`}`).
				LegendFormat("leases"),
				promQuery().
					Expr(`vault_expire_num_irrevocable_leases{`+openbao+`}`).
					LegendFormat("irrevocable")))).
		Panel(openbaoPanelTokensIdentityEntities, dashboardv2.NewPanelBuilder().
			Title("Tokens & Identity Entities").
			Description("Service token count and identity entities over time. Both are recomputed every ten minutes, not per scrape, so each step repeats the last collected value instead of drawing a line between two distant points. A runaway token count points at a login loop (e.g. an ESO auth misconfiguration).").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`sum(`+usageGauge(`vault_token_count{`+openbao+`}`)+`)`).
				LegendFormat("tokens"),
				promQuery().
					Expr(usageGauge(`vault_identity_num_entities{`+openbao+`}`)).
					LegendFormat("entities")))).
		Build()
}
