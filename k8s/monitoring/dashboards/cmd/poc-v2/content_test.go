package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const classicDir = "../../../charts/dashboards/dashboards"

// classicEnums maps Classic integer encodings to the v2 strings that encode
// the same value, keyed by field. Fields absent here are compared verbatim.
var classicEnums = map[string][]string{
	"graphTooltip": {"Off", "Crosshair", "Tooltip"},
	"refresh":      {"never", "onDashboardLoad", "onTimeRangeChanged"},
	"hide":         {"dontHide", "hideLabel", "hideVariable"},
	"sort": {
		"disabled", "alphabeticalAsc", "alphabeticalDesc", "numericalAsc", "numericalDesc",
		"alphabeticalCaseInsensitiveAsc", "alphabeticalCaseInsensitiveDesc", "naturalAsc", "naturalDesc",
	},
}

var targetFields = []string{"expr", "legendFormat", "instant", "range", "format", "interval", "queryType", "editorMode"}

var queryOptionFields = []string{"interval", "maxDataPoints", "timeFrom", "timeShift"}

func TestTranslationsKeepClassicContent(t *testing.T) {
	for _, tr := range translations {
		if tr.cutover {
			continue
		}
		d, err := tr.build()
		if err != nil {
			t.Fatalf("%s: %v", tr.name, err)
		}
		v2, err := toGeneric(d)
		if err != nil {
			t.Fatalf("%s: %v", tr.name, err)
		}
		raw, err := os.ReadFile(filepath.Join(classicDir, tr.name+".json"))
		if err != nil {
			t.Errorf("%s: Classic dashboard required before cutover: %v", tr.name, err)
			continue
		}
		var classic map[string]any
		if err := json.Unmarshal(raw, &classic); err != nil {
			t.Fatalf("%s: %v", tr.name, err)
		}
		if diffs := compareContent(tr.name, classic, v2); len(diffs) > 0 {
			t.Errorf("content differs from Classic:\n%s", strings.Join(diffs, "\n"))
		}
	}
}

func TestCompareContent(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		classic string
		v2      string
		diffs   []string
	}{
		{name: "drops empty values until stable", in: `{"a":null,"b":{},"c":[],"d":{"e":{"f":[]}},"g":[{}],"h":0,"i":"","j":false}`, want: `{"g":[{}],"h":0,"i":"","j":false}`},
		{name: "keeps non-empty arrays and nested values", in: `[{"a":[1,null],"b":{"c":null}}]`, want: `[{"a":[1,null]}]`},
		{
			name: "detects a changed legend",
			classic: `{"title":"T","graphTooltip":1,"panels":[{"type":"row","title":"R","panels":[` +
				`{"type":"stat","title":"P","datasource":{"type":"prometheus"},"targets":[{"expr":"up","legendFormat":"a"}]}]}]}`,
			v2: `{"title":"T","cursorSync":"Crosshair","elements":{"p":{"kind":"Panel","spec":{"title":"P",` +
				`"vizConfig":{"group":"stat","spec":{}},"data":{"spec":{"queries":[{"spec":{"refId":"A",` +
				`"query":{"group":"prometheus","spec":{"expr":"up","legendFormat":"b"}}}}]}}}}}}`,
			diffs: []string{`x: panel "P": target[0]: legendFormat: classic="a" v2="b"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.in != "" {
				var v any
				if err := json.Unmarshal([]byte(c.in), &v); err != nil {
					t.Fatal(err)
				}
				if got := encode(normalize(v)); got != c.want {
					t.Errorf("normalize = %s, want %s", got, c.want)
				}
				return
			}
			var classic, v2 map[string]any
			if err := json.Unmarshal([]byte(c.classic), &classic); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(c.v2), &v2); err != nil {
				t.Fatal(err)
			}
			if got := compareContent("x", classic, v2); !slices.Equal(got, c.diffs) {
				t.Errorf("diffs:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(c.diffs, "\n"))
			}
		})
	}
}

type differ struct {
	dashboard string
	lines     []string
}

func (d *differ) check(location string, classic, v2 any) {
	// An empty compared value stands for a key the normalization would drop.
	c, v := normalize(classic), normalize(v2)
	if isEmpty(c) {
		c = nil
	}
	if isEmpty(v) {
		v = nil
	}
	if !reflect.DeepEqual(c, v) {
		d.lines = append(d.lines, fmt.Sprintf("%s: %s: classic=%s v2=%s", d.dashboard, location, encode(c), encode(v)))
	}
}

// checkEnum compares a Classic integer enum with its v2 string spelling.
func (d *differ) checkEnum(location, field string, classic, v2 any) {
	if f, ok := classic.(float64); ok {
		if names, i := classicEnums[field], int(f); float64(i) == f && i >= 0 && i < len(names) {
			classic = names[i]
		}
	}
	d.check(location, classic, v2)
}

func compareContent(name string, classic, v2 map[string]any) []string {
	d := &differ{dashboard: name}
	ts := object(v2["timeSettings"])
	d.check("title", classic["title"], v2["title"])
	d.check("tags", classic["tags"], v2["tags"])
	d.check("time.from", object(classic["time"])["from"], ts["from"])
	d.check("time.to", object(classic["time"])["to"], ts["to"])
	d.check("refresh", classic["refresh"], ts["autoRefresh"])
	d.check("timezone", classic["timezone"], ts["timezone"])
	d.checkEnum("graphTooltip", "graphTooltip", classic["graphTooltip"], v2["cursorSync"])
	compareVariables(d, array(object(classic["templating"])["list"]), array(v2["variables"]))
	comparePanels(d, classic, v2)
	return d.lines
}

func compareVariables(d *differ, classic, v2 []any) {
	kinds := map[string]string{"datasource": "DatasourceVariable", "query": "QueryVariable", "custom": "CustomVariable"}
	if len(classic) != len(v2) {
		d.check("variables: count", len(classic), len(v2))
	}
	for i := range min(len(classic), len(v2)) {
		c, v := object(classic[i]), object(v2[i])
		spec := object(v["spec"])
		loc := fmt.Sprintf("variable[%d]", i)
		d.check(loc+": name", c["name"], spec["name"])
		d.check(loc+": label", c["label"], spec["label"])
		typ, _ := c["type"].(string)
		kind, known := kinds[typ]
		if !known {
			kind = "unsupported Classic type " + typ
		}
		d.check(loc+": kind", kind, v["kind"])
		if !known || kind != v["kind"] {
			continue
		}
		switch typ {
		case "datasource":
			d.check(loc+": pluginId", c["query"], spec["pluginId"])
		case "query":
			q := c["query"]
			if m, ok := q.(map[string]any); ok {
				q = m["query"]
			}
			d.check(loc+": query", q, object(object(spec["query"])["spec"])["query"])
			for _, f := range []string{"multi", "includeAll", "refresh", "sort", "hide"} {
				d.checkEnum(loc+": "+f, f, c[f], spec[f])
			}
		case "custom":
			d.check(loc+": query", c["query"], spec["query"])
		}
	}
}

func comparePanels(d *differ, classic, v2 map[string]any) {
	classicPanels := map[string]map[string]any{}
	var collect func([]any)
	collect = func(panels []any) {
		for _, p := range panels {
			panel := object(p)
			if panel["type"] == "row" {
				collect(array(panel["panels"]))
				continue
			}
			title, _ := panel["title"].(string)
			if classicPanels[title] != nil {
				d.check("panel titles: duplicate", title, nil)
			}
			classicPanels[title] = panel
		}
	}
	collect(array(classic["panels"]))

	v2Panels := map[string]map[string]any{}
	for _, e := range object(v2["elements"]) {
		el := object(e)
		if el["kind"] != "Panel" {
			continue
		}
		spec := object(el["spec"])
		title, _ := spec["title"].(string)
		if v2Panels[title] != nil {
			d.check("panel titles: duplicate", nil, title)
		}
		v2Panels[title] = spec
	}

	titles := make([]string, 0, len(classicPanels)+len(v2Panels))
	for title := range classicPanels {
		titles = append(titles, title)
	}
	for title := range v2Panels {
		if classicPanels[title] == nil {
			titles = append(titles, title)
		}
	}
	slices.Sort(titles)
	for _, title := range titles {
		c, v := classicPanels[title], v2Panels[title]
		switch {
		case c == nil:
			d.check("panel titles", nil, title)
		case v == nil:
			d.check("panel titles", title, nil)
		default:
			comparePanel(d, fmt.Sprintf("panel %q", title), c, v)
		}
	}
}

func comparePanel(d *differ, loc string, c, v map[string]any) {
	viz := object(v["vizConfig"])
	vizSpec := object(viz["spec"])
	data := object(object(v["data"])["spec"])
	d.check(loc+": type", c["type"], viz["group"])
	d.check(loc+": description", c["description"], v["description"])
	d.check(loc+": fieldConfig.defaults", object(c["fieldConfig"])["defaults"], object(vizSpec["fieldConfig"])["defaults"])
	d.check(loc+": fieldConfig.overrides", object(c["fieldConfig"])["overrides"], object(vizSpec["fieldConfig"])["overrides"])
	d.check(loc+": options", c["options"], vizSpec["options"])
	d.check(loc+": transformations", c["transformations"], data["transformations"])

	targets, queries := array(c["targets"]), array(data["queries"])
	if len(targets) != len(queries) {
		d.check(loc+": target count", len(targets), len(queries))
	}
	for i := range min(len(targets), len(queries)) {
		target, pq := object(targets[i]), object(object(queries[i])["spec"])
		query := object(pq["query"])
		qspec := object(query["spec"])
		tloc := fmt.Sprintf("%s: target[%d]", loc, i)
		for _, f := range targetFields {
			d.check(tloc+": "+f, target[f], qspec[f])
		}
		dsType := object(target["datasource"])["type"]
		if dsType == nil {
			dsType = object(c["datasource"])["type"]
		}
		d.check(tloc+": datasource type", dsType, query["group"])
		if ref, ok := target["refId"]; ok {
			d.check(tloc+": refId", ref, pq["refId"])
		}
	}

	opts := object(data["queryOptions"])
	for _, f := range queryOptionFields {
		d.check(loc+": "+f, c[f], opts[f])
	}
}

// normalize drops keys whose value is null, {}, or [] until none remain,
// because the Classic and v2 producers differ in emitting unset fields.
func normalize(v any) any {
	for {
		next := dropEmpty(v)
		if reflect.DeepEqual(next, v) {
			return next
		}
		v = next
	}
}

func dropEmpty(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if !isEmpty(e) {
				out[k] = dropEmpty(e)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = dropEmpty(e)
		}
		return out
	}
	return v
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

func toGeneric(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(data, &out)
}

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func array(v any) []any {
	a, _ := v.([]any)
	return a
}

func encode(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<%v>", err)
	}
	return string(data)
}
