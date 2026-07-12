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
		signalRegex = `(?i)(quorum.*(lost|error|fail)|corosync.*(error|fail)|cluster.*(lost|error|fail)|ha[- ]?(crm|lrm)?.*(error|fail)|backup.*(error|fail)|vzdump.*(error|fail)|zfs.*(error|fault|degrad)|i/o error|out of memory|oom-kill|killed process|apparmor="DENIED")`
	)

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "yellow"},
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
				Query(dashboard.StringOrMap{String: strPtr(`label_values({host=~"` + proxmoxHosts + `"}, host)`)}).
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
				Query(dashboard.StringOrMap{String: strPtr(`label_values({host=~"$node"}, appname)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("severity").
				Label("Severity").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values({host=~"$node"}, severity)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(6).Height(4).
				Unit("cps").
				Min(0).
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
				Description("Quorum, HA, backup, storage/ZFS, OOM, and AppArmor denial messages.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds()).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + messageBase + ` |~ "` + signalRegex + `" [24h])) or vector(0)`).
					Instant(true).
					LegendFormat("signals"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Errors and Warnings")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Error Rate by Node").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + errSel + `[5m])) or sum by (host) (rate(` + base + `[5m])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warning Rate by Node").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + warnSel + `[5m])) or sum by (host) (rate(` + base + `[5m])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Error Rate by Service").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate(` + errSel + `[5m]))`).
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
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate({host=~"$node", appname=~"pve.*|corosync|qmeventd|vzdump"}[5m]))`).
					LegendFormat("{{appname}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Operational Signals")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Signal Rate by Category").
				Description("Message-pattern indicators; inspect the matching logs below before treating a signal as an incident.").
				Datasource(ds).
				Span(24).Height(9).
				Unit("cps").
				Min(0).
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: boolPtr(true)}).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + messageBase + ` |~ "(?i)(quorum.*(lost|error|fail)|corosync.*(error|fail)|cluster.*(lost|error|fail))" [5m]))`).
					LegendFormat("Cluster / quorum"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + messageBase + ` |~ "(?i)(ha[- ]?(crm|lrm)?.*(error|fail))" [5m]))`).
					LegendFormat("HA"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + messageBase + ` |~ "(?i)((backup|vzdump).*(error|fail))" [5m]))`).
					LegendFormat("Backup"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + messageBase + ` |~ "(?i)(zfs.*(error|fault|degrad)|i/o error)" [5m]))`).
					LegendFormat("Storage / ZFS"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + messageBase + ` |~ "(?i)(out of memory|oom-kill|killed process)" [5m]))`).
					LegendFormat("OOM"),
				).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + messageBase + ` |~ "(?i)apparmor=\\\"DENIED\\\"" [5m]))`).
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
					Expr(messageBase + ` |~ "` + signalRegex + `" | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
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
