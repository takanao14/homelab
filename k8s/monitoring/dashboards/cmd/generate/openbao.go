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

	// usageGauge holds a metric from OpenBao's periodic usage-gauge collection.
	//
	// vault_token_count and vault_identity_num_entities are not emitted on every
	// scrape. They come from a background loop that runs every ten minutes, so the
	// series exists for one scrape and is then absent until the loop runs again:
	// sampled at one-minute resolution over six hours, vault_token_count was
	// present for 36 of 360 minutes and vault_identity_num_entities the same. An
	// instant query only reaches back five minutes, so the stat that reads it went
	// blank for roughly half of every ten-minute cycle, and the timeseries drew a
	// point every ten minutes with line segments guessing at the gaps.
	//
	// A fifteen-minute window is longer than the ten-minute period by enough to
	// always contain a sample, without being so long that a genuinely stopped
	// collector goes unnoticed for an hour. Metrics that are emitted every scrape,
	// vault_expire_num_leases among them, need none of this and do not get it.
	usageGauge := func(expr string) string {
		return `last_over_time(` + expr + `[15m])`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	// sparseLatency styles the three summary-quantile panels, which are drawn from
	// go-metrics summaries and are mostly holes.
	//
	// The holes are NaN, not missing samples: the series is published on every
	// scrape and carries NaN for any interval with nothing to measure, so there is
	// no zero baseline to add -- a latency of 0 ms would be a lie where a gap is
	// merely unknown. Measured over six hours at 30s resolution, only 86 of 720
	// samples of vault_core_handle_request p99 were real numbers; the panels were
	// 88% gap.
	//
	// Connecting them is the readable answer, but not unconditionally. Bounding the
	// span at fifteen minutes keeps normal sparsity joined up while leaving a
	// genuine quiet spell visible as a break: measured across the last 24 hours,
	// every 15-minute window contained at least one real sample, for both request
	// latency and raft commit time, so nothing that happens normally reaches the
	// bound.
	//
	// Points are shown always. A connected line over 12% real data would otherwise
	// present interpolation as measurement; the markers say where the readings are.
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
				// Plotted raw, for the same reason as Raft Applies below: these are
				// go-metrics counters reaching Prometheus through the go-metrics sink,
				// which publishes the count for the last collection interval and then
				// starts over. Measured over thirty minutes the request count read
				// 3,3,6,3,3,3,3,29,3,6,3,3,3 -- it falls as well as rises, so it is a
				// gauge in everything but name.
				//
				// rate() over that reported 0 at almost every step, because every fall
				// looks like a counter reset: a service handling three requests a minute
				// was drawn as a flat zero line. That is the panel saying "no traffic"
				// about a service that is working.
				// Both series are pinned to zero against up{}, because the sink emits
				// nothing at all for an interval with no activity rather than emitting a
				// zero -- the same shape as LogQL's empty windows. Neither counter can
				// serve as the other's baseline: measured at two-minute resolution the
				// request count was itself absent for six of eleven steps. up{} is the
				// only series here that exists on every scrape.
				//
				// sum() is what makes the fallback work at all. Without it the two sides
				// carry different label sets -- the metric keeps __name__, the * 0 term
				// drops it -- so `or` treats them as unrelated series and the gaps stay
				// open. There is one OpenBao node, so collapsing labels costs nothing.
				//
				// The zeros are real readings, not padding: retention is 60s against a
				// 30s scrape, so each collection interval is observed about twice and a
				// quiet minute genuinely reports nothing.
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
					// Plotted raw, without rate(). vault_raft_apply is a go-metrics
					// counter reaching Prometheus through the go-metrics sink, which
					// publishes the count for the last collection interval and then
					// starts over -- it is a gauge in everything but name and does not
					// increase monotonically. Observed over two hours it read
					// 11,11,11,11,11,27,11,11,11,11,11,27,11. rate() treats each fall
					// back to 11 as a counter reset and adds the 11 again as fresh
					// increase, which is how the panel arrived at 0.0588 ops from a
					// series whose real meaning is "11 applies in that interval".
					//
					// Pinned to zero against up{} for the same reason as the request
					// panel: an interval with no applies publishes no series at all, so
					// four of ten steps were gaps that read as "no data" rather than as
					// the "nothing was written" they actually mean.
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
