package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/logs"
	"github.com/grafana/grafana-foundation-sdk/go/loki"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildSyslog defines the syslog dashboard backed by Loki, covering both the
// network devices and the Proxmox hosts that forward over RFC 5424.
// Logs are JSON-parsed syslog entries with fields: host, severity, appname, message.
//
// The three window/baseline conventions are the ones established on the DNS
// Query Logs dashboard; see the comment at the top of dns_logs.go for the full
// reasoning. In short: rate() windows use $__auto so the window tracks the zoom
// instead of reading five minutes out of every step once the range is widened,
// Interval("1m") floors that window because the busiest host here runs about
// 0.55 lines/s and errors run 11 per day, and every by-label query pins its
// series to zero over $__range because LogQL emits no sample at all for an
// empty window -- a host that falls silent would otherwise vanish from the
// stack rather than read as zero.
//
// The instant stats keep their fixed windows: [5m] and [1h] there are the
// quantity the tile reports, not an artifact of the zoom level.
//
// The volume panels plot rate() in counts per second and the error and warning
// panels plot count_over_time() as bars, for the reason set out over "Sync
// Activity" in argocd.go. A per-second rate suits the volume panels, where the
// busiest host runs about 0.55 lines a second. It does not suit events that
// arrive ten times a day: a single error in a one-minute bucket is 0.0167,
// which Grafana renders as "16.7 mc/s", and because the bucket follows the zoom
// the same one error reads 8.3 mc/s over a day and 1.4 mc/s over a week. A
// count is the same number at every zoom level for the bucket it sits in.
// count_over_time needs no round(): unlike increase() it does not extrapolate,
// and Interval("1m") raises the step itself, so the buckets tile exactly rather
// than overlapping and counting an event twice.
func buildSyslog() (*dashboard.Dashboard, error) {
	ds := lokiDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		// Syslog has no job label; filter by severity to exclude DNS query logs.
		base = `{host=~"$host", severity=~"$severity"}`
		// baseAll ignores $severity, for the panels that always break down across
		// every severity regardless of the dropdown selection.
		baseAll = `{host=~"$host", severity=~".+"}`
		// baseApp additionally filters by the $appname variable for app-scoped panels.
		baseApp = `{host=~"$host", severity=~"$severity"} | json | __error__="" | appname=~"$appname"`
		// Severity ships as either RFC5424 numeric (0=emerg … 3=err, 4=warning, 6=info)
		// or text depending on the device, so error/warning selectors match both forms.
		// These KPI selectors intentionally ignore $severity so the counts stay meaningful
		// regardless of the dropdown selection.
		errSel  = `{host=~"$host", severity=~"emerg|alert|crit|err|error|[0-3]"}`
		warnSel = `{host=~"$host", severity=~"warning|warn|4"}`
		// networkDevices are the syslog senders that are appliances rather than
		// Linux hosts. Warning means something different on each side, and the
		// baselines are three orders of magnitude apart: the appliances log
		// packet-level events at warning severity and produce about 1870 lines a
		// day between them -- bgw1 alone contributes ICMP redirect discards, TCP
		// resets to a port nothing listens on, and DHCP leases it cannot find --
		// while node1-5, pve and rpi3 together produce 34, almost all pvestatd.
		// Summed into one tile the 34 were invisible and the count could never
		// move enough to mean anything.
		//
		// The list enumerates the appliances rather than the Linux hosts so that
		// the fallback is the safe one. A new server lands in the tile that goes
		// yellow, which is where an unattended warning should surface; a new
		// switch lands there too, but as steady noise that is obvious on sight and
		// fixed by adding one name here. Deriving the Linux side from the Proxmox
		// inventory would have put rpi3 on the appliance side, where its systemd
		// and cron warnings would go unread.
		networkDevices = `bgw1|wifi-ap1|lab-sw1`
		warnSelDevice  = `{host=~"$host", severity=~"warning|warn|4", host=~"` + networkDevices + `"}`
		warnSelHost    = `{host=~"$host", severity=~"warning|warn|4", host!~"` + networkDevices + `"}`
	)

	issueThresholds := issueThresholds()

	warnThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(1)), Color: "yellow"},
		})

	// No threshold on the network device tile: the steady state is around 78 an
	// hour, and nothing measured so far says where "too many" begins. A coloured
	// band would be a guess dressed up as a judgement, so the tile reports the
	// number and leaves the reading to whoever is looking at the trend.
	neutralThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "text"},
		})

	d, err := dashboard.NewDashboardBuilder("Syslog").
		Uid("syslog").
		Tags([]string{"syslog", "network", "logs", "infrastructure"}).
		Timezone("browser").
		Time("now-3h", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			lokiDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("host").
				Label("Host").
				Datasource(ds).
				// Scoped to the streams that carry a severity label. The host label
				// is shared with the journald and DNS query log streams, so the
				// unscoped label_values(host) offered 21 hosts of which only 9 have
				// syslog: picking any of the other 12 emptied the whole dashboard.
				Query(dashboard.StringOrMap{String: new(`label_values({severity=~".+"}, host)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				// All expands to ".+" rather than the values seen in the current
				// range. lab-sw1 logged once in the last day, so the enumerated form
				// would silently drop it from "All" whenever the view does not happen
				// to contain that line. Every selector here also constrains severity,
				// which is what keeps the journald and DNS streams out.
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("severity").
				Label("Severity").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$host"}, severity)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				AllValue(".+"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("appname").
				Label("App").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values({host=~"$host"}, appname)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true).
				AllValue(".+"),
		).

		// Row 1: Summary stats
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Log Rate").
				Datasource(ds).
				Span(8).Height(4).
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
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + errSel + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Host Warnings (1h)").
				Description("Linux hosts only: node1-5, pve, rpi3. Zero in most hours and one to three in the rest, almost all pvestatd, so yellow here is worth a look rather than the permanent state it used to be. The network appliances are counted separately below and the two tiles together are every warning-severity line.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(warnThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + warnSelHost + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Network Device Warnings (1h)").
				Description("bgw1, wifi-ap1 and lab-sw1, which log routine packet-level events at warning severity: ICMP redirect discards, TCP resets, DHCP leases not found. Watch the level, not the presence -- the steady state is around 78 an hour, so this tile is deliberately uncoloured.").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Thresholds(neutralThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum(count_over_time(` + warnSelDevice + ` [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("warnings"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Parse Errors (1h)").
				Datasource(ds).
				Span(12).Height(4).
				Unit("short").
				Min(0).
				Description("Empty is good: syslog lines this dashboard could not parse as JSON. Zero over the last day.").
				Thresholds(issueThresholds).
				WithTarget(loki.NewDataqueryBuilder().
					// Scoped by severity like every other selector here. Without it the
					// tile also counted parse failures in the journald and DNS query
					// log streams, which share the host label and are not syslog.
					Expr(`sum(count_over_time(` + baseAll + ` | json | __error__!="" [1h])) or vector(0)`).
					Instant(true).
					LegendFormat("errors"),
				),
		).

		// Row 2: Volume trends
		WithRow(dashboard.NewRowBuilder("Volume Trends")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				FillOpacity(10).
				Tooltip(tooltipAll).
				Legend(legend).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (host) (rate(` + base + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by Severity").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					// Always break down across all severities, independent of the $severity filter.
					Expr(`sum by (severity) (rate(`+baseAll+`[$__auto]))`+
						` or sum by (severity) (count_over_time(`+baseAll+`[$__range])) * 0`).
					LegendFormat("{{severity}}"),
				).
				// Semantic coloring; covers both numeric and text severity values.
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(emerg|alert|crit|err|error|[0-3])$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(warning|warn|4)$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "yellow"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(notice|5)$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "blue"}},
				}).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: "/^(info|6)$/"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "green"}},
				}),
		).

		// Row 3: App breakdown + errors/warnings by host
		WithRow(dashboard.NewRowBuilder("App Breakdown")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Log Volume by App").
				Datasource(ds).
				Span(12).Height(8).
				Unit("cps").
				Min(0).
				Interval("1m").
				Tooltip(tooltipAll).
				Legend(legend).
				FillOpacity(10).
				SpanNulls(common.BoolOrFloat64{Bool: new(true)}).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				WithTarget(loki.NewDataqueryBuilder().
					Expr(`sum by (appname) (rate(` + baseApp + `[$__auto]))` +
						` or sum by (appname) (count_over_time(` + baseApp + `[$__range])) * 0`).
					LegendFormat("{{appname}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Errors by Host").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				Min(0).
				Interval("1m").
				Description("Empty is good. The whole estate logs error severity on the order of ten lines a day, so a flat set of zero bars is the normal reading.").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(loki.NewDataqueryBuilder().
					// "* 0" tail keeps every host series present even at zero errors.
					// Keyed on $__range rather than the current window, which is itself
					// empty for a host that has gone quiet.
					Expr(`sum by (host) (count_over_time(` + errSel + `[$__auto]))` +
						` or sum by (host) (count_over_time(` + base + `[$__range])) * 0`).
					LegendFormat("{{host}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Warnings by Host").
				Datasource(ds).
				Span(24).Height(8).
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

		// Row 4: Log browser
		WithRow(dashboard.NewRowBuilder("Logs")).
		WithPanel(
			logs.NewPanelBuilder().
				Title("Syslog").
				Datasource(ds).
				Span(24).Height(12).
				ShowTime(true).
				SortOrder(common.LogsSortOrderDescending).
				EnableLogDetails(true).
				ShowLogContextToggle(true).
				ShowControls(true).
				ShowFieldSelector(true).
				WithTarget(loki.NewDataqueryBuilder().
					// line_format is what picks the displayed fields: the logs panel
					// has no displayedFields option in the schema, and
					// ShowFieldSelector only lets a viewer choose fields for the
					// session, with nothing persisted back to the dashboard. The
					// three labels in front of the message are the ones the panel
					// cannot be filtered down to from the row itself -- $host,
					// $severity and $appname are all set to All by default.
					Expr(baseApp + ` | line_format "{{.host}} [{{.severity}}] {{.appname}}: {{.message}}"`).
					MaxLines(500),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
