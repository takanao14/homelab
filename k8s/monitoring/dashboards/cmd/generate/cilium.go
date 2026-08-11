package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildCiliumOverview defines Cilium CNI health: agent/operator status, packet
// drops, policy verdicts, BPF datapath pressure, endpoint health, and Hubble flows.
//
// Two kinds of range window appear below and they are not interchangeable. The
// rate() trends use $__rate_interval so the window grows with the zoom level:
// they previously used a fixed [5m], which is fine at the default 6h but breaks
// once the step exceeds it. At 7d the step is 15m, so Prometheus only ever
// looked at 5 minutes out of every 15 and the remaining two thirds of the data
// were never read -- the largest drop spike in a 7-day view read 0.278 with
// [5m] against 0.606 with a step-sized window, and shorter bursts disappeared
// outright. The increase() windows in the "(5m)" panels stay fixed on purpose:
// there the five minutes is the quantity the panel is named for, not a
// rendering detail.
func buildCiliumOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const clusterFilter = `cluster=~"$cluster"`

	// Cilium's drop counters are overwhelmingly one benign reason: over 30 days
	// prd logged 33,143 "Unsupported L3 protocol" drops (non-IP frames the
	// datapath does not handle) against 3 "Policy denied". Left on the default
	// palette the noise takes a colour and the three drops that matter do not,
	// so the benign series is pinned to grey and policy denials to red. The
	// panel then reads by colour alone: grey only means nothing happened.
	const benignDropColor = "#808080"

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	capacityThresholds := capacityThresholds()

	d, err := dashboard.NewDashboardBuilder("Cilium Overview").
		Uid("cilium-overview").
		Tags([]string{"cilium", "cni", "network", "infrastructure"}).
		Timezone("browser").
		Time("now-6h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(promDatasourceVariable()).
		WithVariable(
			dashboard.NewQueryVariableBuilder("cluster").
				Label("Cluster").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(cilium_endpoint_state, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Endpoints Not Ready").
				Description("Cilium endpoints not in the ready state.").
				Datasource(ds).
				Span(5).Height(4).Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(cilium_endpoint_state{` + clusterFilter + `,endpoint_state!="ready"}) or vector(0)`).
					LegendFormat("Not ready"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Controllers Failing").
				Description("Cilium controllers currently failing their reconciliation runs.").
				Datasource(ds).
				Span(5).Height(4).Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(cilium_controllers_failing{` + clusterFilter + `}) or vector(0)`).
					LegendFormat("Failing"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Unreachable Nodes").
				Description("Nodes Cilium cannot reach via the health checker.").
				Datasource(ds).
				Span(5).Height(4).Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(cilium_unreachable_nodes{` + clusterFilter + `}) or vector(0)`).
					LegendFormat("Unreachable"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Agent Errors (5m)").
				Description("Cilium agent log messages at ERROR level in the last 5 minutes.").
				Datasource(ds).
				Span(5).Height(4).Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(cilium_errors_warnings_total{` + clusterFilter + `,level="ERROR"}[5m]))) or vector(0)`).
					LegendFormat("Errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Hubble Lost Events (5m)").
				Description("Hubble events lost at any processing stage in the last 5 minutes. See the node/source breakdown below for the cause.").
				Datasource(ds).
				Span(4).Height(4).Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(hubble_lost_events_total{` + clusterFilter + `}[5m]))) or vector(0)`).
					LegendFormat("Lost"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Hubble Diagnostics")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Hubble Lost Events by Node / Source (5m)").
				Description("Rolling 5-minute lost event count by node and pipeline stage. hubble_ring_buffer indicates a slow reader; observer_events_queue indicates Hubble processing backlog; perf_event_ring_buffer indicates datapath-to-userspace loss.").
				Datasource(ds).
				Span(24).Height(8).Unit("short").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum by (cluster, node, source) (increase(hubble_lost_events_total{` + clusterFilter + `}[5m])))`).
					LegendFormat("{{cluster}} {{node}} {{source}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Drops & Policy")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Drop Rate by Reason").
				Description("Packets dropped by the datapath, by reason. Unsupported L3 protocol is benign background noise and is drawn in grey; any coloured line is worth reading.").
				Datasource(ds).
				Span(12).Height(8).Unit("pps").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, reason) (rate(cilium_drop_count_total{`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{reason}}"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Unsupported L3 protocol"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": benignDropColor}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* Policy denied"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Hubble Flow Verdicts").
				Description("Observed network flows by verdict. DROPPED flows indicate policy denials or datapath drops.").
				Datasource(ds).
				Span(12).Height(8).Unit("pps").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, verdict) (rate(hubble_flows_processed_total{` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{verdict}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Hubble Drops by Reason").
				Description("Dropped flows observed by Hubble, grouped by reason. Same colour rule as Drop Rate by Reason: grey is benign, coloured is not. Hubble spells the reasons in upper snake case.").
				Datasource(ds).
				Span(24).Height(8).Unit("pps").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, reason) (rate(hubble_drop_total{`+clusterFilter+`}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{reason}}"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* UNSUPPORTED_L3_PROTOCOL"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": benignDropColor}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* POLICY_DENIED"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Endpoints & Agent Health")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Endpoint State").
				Description("Cilium endpoints by state. Anything other than 'ready' should be transient.").
				Datasource(ds).
				Span(8).Height(8).Unit("short").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, endpoint_state) (cilium_endpoint_state{` + clusterFilter + `})`).
					LegendFormat("{{cluster}} {{endpoint_state}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Endpoint Regenerations").
				Description("Rate of endpoint datapath regenerations. Spikes accompany policy or identity churn.").
				Datasource(ds).
				Span(8).Height(8).Unit("ops").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (rate(cilium_endpoint_regenerations_total{` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Agent Errors & Warnings").
				Description("Rate of Cilium agent log messages by level.").
				Datasource(ds).
				Span(8).Height(8).Unit("ops").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, level) (rate(cilium_errors_warnings_total{` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{level}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("BPF Datapath & Hubble Flows")).
		WithPanel(
			bargauge.NewPanelBuilder().
				// topk is a cardinality guard, not a promise of ten bars: prd
				// currently exposes seven maps, and enabling features such as
				// Egress Gateway or L7 policy adds more. Naming a count in the
				// title would misdescribe every environment but the one that
				// happens to hit it.
				Title("BPF Map Pressure").
				Description("Fill ratio of the busiest eBPF maps. Sustained high pressure (>80%) risks map exhaustion and packet drops.").
				Datasource(ds).
				Span(8).Height(8).Unit("percent").Min(0).Max(100).
				Orientation(common.VizOrientationHorizontal).
				Thresholds(capacityThresholds).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sort_desc(topk(10, 100 * max by (cluster, map_name) (cilium_bpf_map_pressure{` + clusterFilter + `})))`).
					Instant().
					LegendFormat("{{cluster}} {{map_name}}"),
				).
				Decimals(2),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Conntrack GC Entries").
				Description("Connection-tracking entries seen by the last garbage-collection run.").
				Datasource(ds).
				Span(8).Height(8).Unit("short").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster) (cilium_datapath_conntrack_gc_entries{` + clusterFilter + `})`).
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Hubble Flows by Type").
				Description("Rate of observed flows by type (Trace, Drop, L7, etc.).").
				Datasource(ds).
				Span(8).Height(8).Unit("pps").Min(0).
				Tooltip(tooltipAll).Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (cluster, type) (rate(hubble_flows_processed_total{` + clusterFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} {{type}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
