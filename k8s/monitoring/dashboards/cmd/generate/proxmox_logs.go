package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	proxmoxLogsPanelLogRate                    = 1
	proxmoxLogsPanelErrors1h                   = 2
	proxmoxLogsPanelWarnings1h                 = 3
	proxmoxLogsPanelOperationalSignals24h      = 4
	proxmoxLogsPanelErrorsByNode               = 5
	proxmoxLogsPanelWarningsByNode             = 6
	proxmoxLogsPanelErrorsByService            = 7
	proxmoxLogsPanelProxmoxServiceLogRate      = 8
	proxmoxLogsPanelSignalsByCategory          = 9
	proxmoxLogsPanelMatchingOperationalSignals = 10
	proxmoxLogsPanelProxmoxHostLogs            = 11
)

// buildProxmoxLogs covers inventory-derived PVE RFC 5424 journals in Loki.
// Follow dns_logs window and zero-baseline conventions. Use rates for volume
// and count_over_time bars for sparse errors and operational signals.
func buildProxmoxLogs() (*dashboardv2.Dashboard, error) {
	const (
		elementLogRate                    = "log-rate"
		elementErrors1h                   = "errors-1h"
		elementWarnings1h                 = "warnings-1h"
		elementOperationalSignals24h      = "operational-signals-24h"
		elementErrorsByNode               = "errors-by-node"
		elementWarningsByNode             = "warnings-by-node"
		elementErrorsByService            = "errors-by-service"
		elementProxmoxServiceLogRate      = "proxmox-service-log-rate"
		elementSignalsByCategory          = "signals-by-category"
		elementMatchingOperationalSignals = "matching-operational-signals"
		elementProxmoxHostLogs            = "proxmox-host-logs"
	)

	proxmoxHosts, err := loadProxmoxHostRegex()
	if err != nil {
		return nil, err
	}

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

	warnThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	d := dashboardv2.NewDashboardBuilder("Proxmox Logs").
		Tags([]string{"proxmox", "logs", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("60s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(lokiDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("node").
			Label("Node").
			Query(lokiVariableQuery(`label_values({host=~"` + proxmoxHosts + `"}, host)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			AllValue(proxmoxHosts)).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("appname").
			Label("Service").
			Query(lokiVariableQuery(`label_values({host=~"$node"}, appname)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			// Use .+ for All so silent services remain in scope.
			AllValue(".+")).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("severity").
			Label("Severity").
			Query(lokiVariableQuery(`label_values({host=~"$node"}, severity)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true).
			// Keep every severity in All, including quiet critical levels.
			AllValue(".+"))

	d.Panel(elementLogRate, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelLogRate).
		Title("Log Rate").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			Thresholds(measurementThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(rate(`+base+`[5m])) or vector(0)`).
				Instant(true).
				LegendFormat("logs/s")))))

	d.Panel(elementErrors1h, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelErrors1h).
		Title("Errors (1h)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+errSel+`[1h])) or vector(0)`).
				Instant(true).
				LegendFormat("errors")))))

	d.Panel(elementWarnings1h, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelWarnings1h).
		Title("Warnings (1h)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(warnThresholds)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+warnSel+`[1h])) or vector(0)`).
				Instant(true).
				LegendFormat("warnings")))))

	d.Panel(elementOperationalSignals24h, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelOperationalSignals24h).
		Title("Operational Signals (24h)").
		Description("Empty is good. Quorum, HA, backup, storage/ZFS, OOM, and AppArmor denial messages; a healthy cluster matches none of them, so any non-zero value here is worth reading the log panel for.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds())).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(signalRegex)+` [24h])) or vector(0)`).
				Instant(true).
				LegendFormat("signals")))))

	d.Panel(elementErrorsByNode, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelErrorsByNode).
		Title("Errors by Node").
		Description("Empty is good. A healthy day on these hosts produces a handful of error-severity lines at most, so a flat set of zero bars is the normal reading.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				// Key zero on the full range so quiet nodes remain visible.
				Expr(`sum by (host) (count_over_time(`+errSel+`[$__auto]))`+
					` or sum by (host) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{host}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementWarningsByNode, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelWarningsByNode).
		Title("Warnings by Node").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum by (host) (count_over_time(`+warnSel+`[$__auto]))`+
					` or sum by (host) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{host}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementErrorsByService, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelErrorsByService).
		Title("Errors by Service").
		Description("Empty is good. Every service on the hosts is listed at zero so the panel still reads as healthy when nothing has erred; a service lifts a bar off the baseline only when it actually logs an error. In practice that is pveproxy and rsyslogd, a handful of lines a day.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				// Baseline from unfiltered app streams; an empty error selector cannot
				// enumerate zero rows.
				Expr(`sum by (appname) (count_over_time(`+errSel+`[$__auto]))`+
					` or sum by (appname) (count_over_time(`+base+`[$__range])) * 0`).
				LegendFormat("{{appname}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementProxmoxServiceLogRate, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelProxmoxServiceLogRate).
		Title("Proxmox Service Log Rate").
		Description("Activity from pve*, corosync, qmeventd, and vzdump services.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("cps").
			Min(0).
			FillOpacity(10).
			Tooltip(tooltipAll).
			Legend(legend).
			SpanNulls(common.BoolOrFloat64{Bool: new(true)})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum by (appname) (rate(`+pveServices+`[$__auto]))`+
					` or sum by (appname) (count_over_time(`+pveServices+`[$__range])) * 0`).
				LegendFormat("{{appname}}"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementSignalsByCategory, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelSignalsByCategory).
		Title("Signals by Category").
		Description("Empty is good: six flat zero bars is the normal reading, and none of these patterns match on a healthy cluster. Message-pattern indicators; inspect the matching logs below before treating a signal as an incident.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// Scalar categories use vector(0) so a quiet cluster stays explicit.
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(sigCluster)+` [$__auto])) or vector(0)`).
				LegendFormat("Cluster / quorum"))).
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(sigHA)+` [$__auto])) or vector(0)`).
				LegendFormat("HA"))).
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(sigBackup)+` [$__auto])) or vector(0)`).
				LegendFormat("Backup"))).
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(sigStorage)+` [$__auto])) or vector(0)`).
				LegendFormat("Storage / ZFS"))).
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(sigOOM)+` [$__auto])) or vector(0)`).
				LegendFormat("OOM"))).
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(`sum(count_over_time(`+messageBase+matches(sigAppArmor)+` [$__auto])) or vector(0)`).
				LegendFormat("AppArmor denied"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1m"))))

	d.Panel(elementMatchingOperationalSignals, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelMatchingOperationalSignals).
		Title("Matching Operational Signals").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			EnableLogDetails(true).
			ShowLogContextToggle(true).
			ShowControls(true).
			ShowFieldSelector(true).
			SortOrder(common.LogsSortOrderDescending)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(messageBase+matches(signalRegex)+` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
				MaxLines(500)))))

	d.Panel(elementProxmoxHostLogs, dashboardv2.NewPanelBuilder().
		Id(proxmoxLogsPanelProxmoxHostLogs).
		Title("Proxmox Host Logs").
		Visualization(logs.NewVisualizationV2Builder().
			ShowTime(true).
			EnableLogDetails(true).
			ShowLogContextToggle(true).
			ShowControls(true).
			ShowFieldSelector(true).
			SortOrder(common.LogsSortOrderDescending)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(lokiQuery().
				Expr(baseJSON+` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
				MaxLines(1000)))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementLogRate).
			WithItem(elementErrors1h).
			WithItem(elementWarnings1h).
			WithItem(elementOperationalSignals24h))).
		Row(dashboardv2.NewRowBuilder().Title("Errors & Warnings").AutoGridLayout(chartGrid().
			WithItem(elementErrorsByNode).
			WithItem(elementWarningsByNode))).
		Row(dashboardv2.NewRowBuilder().Title("Errors by Service").AutoGridLayout(chartGrid().
			WithItem(elementErrorsByService).
			WithItem(elementProxmoxServiceLogRate))).
		Row(dashboardv2.NewRowBuilder().Title("Signals by Category").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementSignalsByCategory))).
		Row(dashboardv2.NewRowBuilder().Title("Matching Operational Signals").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementMatchingOperationalSignals))).
		Row(dashboardv2.NewRowBuilder().Title("Logs").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementProxmoxHostLogs)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
