package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildProxmoxLogs covers inventory-derived PVE RFC 5424 journals in Loki.
// Follow dns_logs window and zero-baseline conventions. Use rates for volume
// and count_over_time bars for sparse errors and operational signals.
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

		// Compose shared signal patterns once so summary and breakdown agree.
		sigCluster  = `(?i)(quorum.*(lost|error|fail)|corosync.*(error|fail)|cluster.*(lost|error|fail))`
		sigHA       = `(?i)(ha[- ]?(crm|lrm)?.*(error|fail))`
		sigBackup   = `(?i)((backup|vzdump).*(error|fail))`
		sigStorage  = `(?i)(zfs.*(error|fault|degrad)|i/o error)`
		sigOOM      = `(?i)(out of memory|oom-kill|killed process)`
		sigAppArmor = `(?i)apparmor="DENIED"`
		signalRegex = sigCluster + `|` + sigHA + `|` + sigBackup + `|` +
			sigStorage + `|` + sigOOM + `|` + sigAppArmor
	)

	// Use LogQL backtick strings so embedded regex quotes cannot break queries.
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
				// Use .+ for All so silent services remain in scope.
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
				// Keep every severity in All, including quiet critical levels.
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
					// Key zero on the full range so quiet nodes remain visible.
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
					// Baseline from unfiltered app streams; an empty error selector cannot
					// enumerate zero rows.
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
				// Scalar categories use vector(0) so a quiet cluster stays explicit.
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
