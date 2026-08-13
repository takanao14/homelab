package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildProxmoxLogs defines the Proxmox VE host journal dashboard backed by Loki.
// The hypervisors forward RFC 5424 syslog with labels: host, appname, severity.
// The host regex comes from the shared inventory (../values/proxmox-nodes.yaml).
//
// Window and baseline conventions follow dns_logs.go; see the comment at the top
// of that file. Aggregation windows use $__auto, Interval("1m") floors it, and
// every timeseries pins its series to zero because LogQL emits no sample for an
// empty window. That last point matters more here than anywhere else: a healthy day on
// these hosts produces a couple of error-severity lines and no operational
// signals at all, so without a baseline most of these panels render as "No data"
// and the reader cannot tell a healthy cluster from a broken query.
//
// Counts are given as magnitudes rather than figures throughout. A day's errors
// here moved from 9 to 2 over the course of an afternoon, so an exact number
// baked into a description is stale before it is read; what stays true is the
// order of magnitude and which services produce them.
//
// The error, warning and signal panels plot count_over_time() as bars, while
// the volume panel keeps rate() in counts per second; see the same split
// explained in syslog.go. At a few events a day a per-second rate lands in
// thousandths and moves with the zoom, which is no way to read an error count.
func buildProxmoxLogs() (*dashboard.Dashboard, error) {
	proxmoxHosts, err := loadProxmoxHostRegex()
	if err != nil {
		return nil, err
	}

	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		base        = `{host=~"$node", appname=~"$appname", severity=~"$severity"}`
		baseJSON    = `{host=~"$node", appname=~"$appname", severity=~"$severity"} | json | __error__=""`
		errSel      = `{host=~"$node", appname=~"$appname", severity=~"emerg|alert|crit|err|error|[0-3]"}`
		warnSel     = `{host=~"$node", appname=~"$appname", severity=~"warning|warn|4"}`
		messageBase = `{host=~"$node", appname=~"$appname", severity=~"$severity"} | json | __error__="" | line_format "{{.message}}"`
		pveServices = `{host=~"$node", appname=~"pve.*|corosync|qmeventd|vzdump"}`

		// The operational signal patterns, one per category on the Signal Rate
		// panel. signalRegex is composed from them rather than written out a
		// second time: the summary tile and the breakdown have to agree about
		// what a signal is, and when they were separate strings nothing enforced
		// it. Each keeps its own (?i) so it still works standalone.
		sigCluster  = `(?i)(quorum.*(lost|error|fail)|corosync.*(error|fail)|cluster.*(lost|error|fail))`
		sigHA       = `(?i)(ha[- ]?(crm|lrm)?.*(error|fail))`
		sigBackup   = `(?i)((backup|vzdump).*(error|fail))`
		sigStorage  = `(?i)(zfs.*(error|fault|degrad)|i/o error)`
		sigOOM      = `(?i)(out of memory|oom-kill|killed process)`
		sigAppArmor = `(?i)apparmor="DENIED"`
		signalRegex = sigCluster + `|` + sigHA + `|` + sigBackup + `|` +
			sigStorage + `|` + sigOOM + `|` + sigAppArmor
	)

	// matches renders a LogQL line filter using a backtick string. LogQL's
	// double-quoted strings interpret backslash escapes, so sigAppArmor's
	// embedded quotes closed the literal early and turned every query built from
	// signalRegex into "parse error: unexpected IDENTIFIER" -- the two panels
	// below returned nothing at all, which read on the dashboard exactly like a
	// cluster with no signals to report. Backtick strings take the pattern
	// verbatim, so the escaping cannot drift out of step with the pattern again.
	matches := func(pattern string) string {
		return " |~ `" + pattern + "`"
	}

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	d, err := dashboard.NewDashboardBuilder("Proxmox Logs").
		Uid("proxmox-logs").
		Tags([]string{"proxmox", "logs", "infrastructure"}).
		Timezone("browser").
		Time("now-6h", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			lokiDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"` + proxmoxHosts + `"}, host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				AllValue(proxmoxHosts),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("appname").
				Label("Service").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$node"}, appname)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// ".+" rather than the values seen in the current range, matching the
				// node variable above. vzdump and qmeventd only speak when a backup or
				// a VM event happens, so the enumerated form would drop them from
				// "All" in any view that does not already contain one.
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("severity").
				Label("Severity").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$node"}, severity)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// Same reasoning, and it matters most here: a healthy range contains
				// only info, notice and warning, so the enumerated "All" would not
				// include crit or emerg. The operational signal panels select through
				// $severity, and an OOM or a quorum loss arrives at exactly the
				// severity that was missing from the list.
				AllValue(".+"),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("cps").
				Min(0).
				Thresholds(measurementThresholds()).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + base + `[5m])) or vector(0)`).
					Instant(true).
					LegendFormat("logs/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Errors (1h)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds()).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + errSel + `[1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Warnings (1h)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(warnThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + warnSel + `[1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Operational Signals (24h)").
				Description("Empty is good. Quorum, HA, backup, storage/ZFS, OOM, and AppArmor denial messages; a healthy cluster matches none of them, so any non-zero value here is worth reading the log panel for.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds()).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(signalRegex) + ` [24h])) or vector(0)`).
					Instant(true).
					LegendFormat("signals"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Errors & Warnings")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Errors by Node").
				Description("Empty is good. A healthy day on these hosts produces a handful of error-severity lines at most, so a flat set of zero bars is the normal reading.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					// The baseline is keyed on $__range rather than the current window,
					// which is itself empty for a node that has gone quiet.
					Expr(`sum by (host) (count_over_time(` + errSel + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warnings by Node").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (count_over_time(` + warnSel + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Errors by Service").
				Description("Empty is good. Every service on the hosts is listed at zero so the panel still reads as healthy when nothing has erred; a service lifts a bar off the baseline only when it actually logs an error. In practice that is pveproxy and rsyslogd, a handful of lines a day.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					// Baselined on base, not errSel. Keying the zero fill on the error
					// selector looks tighter but collapses in the one case the baseline
					// exists for: with no errors in view, errSel matches nothing, so
					// both terms are empty and the panel reads "No data" rather than a
					// row of zeroes. The Proxmox hosts carry 10 distinct appnames, so
					// enumerating all of them costs a legend of 10 flat lines.
					Expr(`sum by (appname) (count_over_time(` + errSel + `[$__auto]))` +
						` or sum by (appname) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{appname}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Proxmox Service Log Rate").
				Description("Activity from pve*, corosync, qmeventd, and vzdump services.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate(` + pveServices + `[$__auto]))` +
						` or sum by (appname) (count_over_time(` + pveServices + `[$__range])) * 0`).
					LegendFormat("{{appname}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Operational Signals")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Signals by Category").
				Description("Empty is good: six flat zero bars is the normal reading, and none of these patterns match on a healthy cluster. Message-pattern indicators; inspect the matching logs below before treating a signal as an incident.").
				Datasource(ds).
				Span(24).Height(9).
				Unit("short").
				Min(0).
				Interval("1m").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				// "or vector(0)" on every category rather than the by-label baseline
				// used elsewhere: each target is already a scalar sum, and without it
				// a quiet cluster renders the whole panel as "No data" -- the six
				// categories it is watching would not even appear in the legend, so
				// there would be no way to tell a healthy cluster from a broken query.
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(sigCluster) + ` [$__auto])) or vector(0)`).
					LegendFormat("Cluster / quorum"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(sigHA) + ` [$__auto])) or vector(0)`).
					LegendFormat("HA"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(sigBackup) + ` [$__auto])) or vector(0)`).
					LegendFormat("Backup"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(sigStorage) + ` [$__auto])) or vector(0)`).
					LegendFormat("Storage / ZFS"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(sigOOM) + ` [$__auto])) or vector(0)`).
					LegendFormat("OOM"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + matches(sigAppArmor) + ` [$__auto])) or vector(0)`).
					LegendFormat("AppArmor denied"),
				),
		).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Matching Operational Signals").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				SortOrder(common.LogsSortOrderDescending).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(messageBase + matches(signalRegex) + ` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
					MaxLines(500),
				),
		).
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Proxmox Host Logs").
				Datasource(ds).
				Span(24).Height(14).
				ShowTime(true).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				SortOrder(common.LogsSortOrderDescending).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(baseJSON + ` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
					MaxLines(1000),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
