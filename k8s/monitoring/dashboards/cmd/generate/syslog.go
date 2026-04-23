package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildSyslog defines the network device syslog dashboard backed by Loki.
// Logs are JSON-parsed syslog entries with fields: host, severity, appname, message.
func buildSyslog() (*dashboard.Dashboard, error) {
	dsType := "loki"
	dsUID := "$datasource"
	ds := common.DataSourceRef{Type: &dsType, Uid: &dsUID}

	const (
		// Syslog has no job label; filter by severity to exclude DNS query logs.
		base     = `{host=~"$host", severity=~"$severity"}`
		baseJSON = `{host=~"$host", severity=~"$severity"} | json | __error__=""`
	)

	issueThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(1), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("Syslog").
		Uid("syslog").
		Tags([]string{"syslog", "network", "logs", "infrastructure"}).
		Timezone("browser").
		Time("now-6h", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("loki"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("host").
				Label("Host").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("severity").
				Label("Severity").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(severity)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("appname").
				Label("App").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(appname)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).

		// Row 1: Summary stats
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(rate(` + base + `[5m]))`).
					LegendFormat("logs/s"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Warning Logs (1h)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time({host=~"$host", severity="warning"}[1h]))`).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Parse Errors (1h)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time({host=~"$host"} | json | parse_error="true" [1h]))`).
					LegendFormat("errors"),
				),
		).

		// Row 2: Volume trends
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + base + `[5m]))`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Severity").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (severity) (rate({host=~"$host", severity=~".+"}[5m]))`).
					LegendFormat("{{severity}}"),
				),
		).

		// Row 3: App breakdown + warnings
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by App").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate({host=~"$host", appname=~"$appname"}[5m]))`).
					LegendFormat("{{appname}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warning Rate by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate({host=~"$host", severity="warning"}[5m]))`).
					LegendFormat("{{host}}"),
				),
		).

		// Row 4: Log browser
		WithPanel(
			logs.NewPanelBuilder().
				Title("Syslog").
				Datasource(ds).
				Span(24).Height(12).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(baseJSON + ` | appname=~"$appname" | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
