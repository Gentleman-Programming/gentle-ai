package telemetrycollector

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// runtimeSQLTableRef matches the retired SQLite table identifiers
// (runtime_rows, runtime_deliveries) only as their own token, so it does not
// false-positive on the new gentle_runtime_rows_total /
// gentle_runtime_deliveries_total PromQL metric names, which legitimately
// contain the same substrings.
var runtimeSQLTableRef = regexp.MustCompile(`(^|[^a-zA-Z0-9_])runtime_(rows|deliveries)([^a-zA-Z0-9_]|$)`)

func TestRuntimeDashboard(t *testing.T) {
	data, err := os.ReadFile("../../deploy/telemetry/grafana/dashboards/gentle-ai-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var dashboard struct {
		UID      string
		Title    string
		Timezone string
		Refresh  string
		Time     struct{ From, To string }
		Panels   []struct {
			ID          int
			Title       string
			Type        string
			TimeFrom    string
			Description string
			GridPos     struct{ X, Y, W, H int }
			Datasource  struct{ Type, UID string }
			Targets     []struct {
				QueryType, QueryText, RawQueryText, Expr string
				TimeColumns                              []string
				Datasource                               struct{ Type, UID string }
			}
			FieldConfig map[string]any
			Options     map[string]any
		}
	}
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.UID != "gentle-ai-usage" || dashboard.Title != "Axiom — Usage" {
		t.Fatalf("dashboard identity changed: %q %q", dashboard.UID, dashboard.Title)
	}
	if dashboard.Timezone != "browser" || dashboard.Refresh != "1m" || dashboard.Time.From != "2026-09-10T00:00:00.000Z" || dashboard.Time.To != "now" {
		t.Fatalf("dashboard defaults = timezone %q, refresh %q, range %q to %q", dashboard.Timezone, dashboard.Refresh, dashboard.Time.From, dashboard.Time.To)
	}
	// runtimeSections holds the two dashboard rows whose panels were moved
	// off the SQLite datasource onto VictoriaMetrics/PromQL in T3; every
	// other section is untouched and keeps its original SQL assertions.
	runtimeSections := map[string]bool{"Live activity": true, "Subagents": true}

	wantPanels := []struct{ section, title, kind string }{
		{"Active users", "Active users, last 24h", "stat"},
		{"Active users", "Active users, last 6h", "stat"},
		{"Active users", "Active users, last 1h", "stat"},
		{"Active users", "Active users, last 15 min", "stat"},
		{"Adoption", "Unique installs all-time", "stat"},
		{"Adoption", "Active installs yesterday", "stat"},
		{"Adoption", "Active installs in range", "stat"},
		{"Adoption", "New installs in range", "stat"},
		{"Adoption", "Heartbeats in range", "stat"},
		{"Adoption", "RDD adoption %", "stat"},
		{"Adoption", "npm downloads latest day", "stat"},
		{"Adoption", "GitHub release downloads", "stat"},
		{"Growth", "Daily active installs", "timeseries"},
		{"Growth", "Daily new installs", "timeseries"},
		{"Growth", "Cumulative unique installs", "timeseries"},
		{"Where Axiom runs", "Agent adoption", "barchart"},
		{"Where Axiom runs", "Component adoption", "barchart"},
		{"Where Axiom runs", "OS and architecture", "barchart"},
		{"Where Axiom runs", "Version adoption", "barchart"},
		{"Live activity", "Deliveries, last 15 min", "stat"},
		{"Live activity", "Responses, last 15 min", "stat"},
		{"Live activity", "Tokens processed, last 15 min", "stat"},
		{"Live activity", "Hosts active, last 15 min", "stat"},
		{"Live activity", "Responses per minute by host (last 3h)", "timeseries"},
		{"Live activity", "Tokens processed per minute (last 3h)", "timeseries"},
		{"Live activity", "Tokens processed", "stat"},
		{"Live activity", "Responses", "stat"},
		{"Live activity", "Deliveries", "stat"},
		{"Live activity", "Hosts reporting", "stat"},
		{"Subagents", "Subagent coverage", "stat"},
		{"Subagents", "Responses by subagent", "barchart"},
		{"Subagents", "Tokens processed by subagent", "barchart"},
		{"Subagents", "Subagent model and effort selection", "table"},
		{"Subagents", "Most popular models per subagent", "table"},
		{"Subagents", "Top model per subagent", "barchart"},
		{"Subagents", "Usage by host, subagent, model and effort", "table"},
		{"Subagents", "Tokens processed by host", "barchart"},
		{"Subagents", "Responses by host", "barchart"},
		{"Subagents", "Tokens by model", "table"},
		{"Subagents", "Responses and tokens by selected effort", "table"},
		{"Subagents", "Tokens processed per hour by host (last 24h)", "timeseries"},
		{"Subagents", "Token coverage per field", "table"},
		{"Subagents", "Error observations by category", "barchart"},
		{"Subagents", "Measured duration by kind", "table"},
	}
	var gotPanels []struct{ section, title, kind string }
	section := ""
	seenIDs := map[int]bool{}
	wantGrid := map[int][4]int{
		550: {0, 0, 24, 1},
		551: {0, 1, 6, 4},
		552: {6, 1, 6, 4},
		553: {12, 1, 6, 4},
		554: {18, 1, 6, 4},
		100: {0, 5, 24, 1},
		101: {0, 6, 6, 4},
		102: {6, 6, 6, 4},
		103: {12, 6, 6, 4},
		104: {18, 6, 6, 4},
		105: {0, 10, 6, 4},
		106: {6, 10, 6, 4},
		107: {12, 10, 6, 4},
		108: {18, 10, 6, 4},
		200: {0, 14, 24, 1},
		201: {0, 15, 8, 8},
		202: {8, 15, 8, 8},
		203: {16, 15, 8, 8},
		300: {0, 23, 24, 1},
		301: {0, 24, 6, 8},
		302: {6, 24, 6, 8},
		303: {12, 24, 6, 8},
		304: {18, 24, 6, 8},
		400: {0, 32, 24, 1},
		600: {0, 33, 24, 1},
		601: {0, 34, 6, 4},
		602: {6, 34, 6, 4},
		603: {12, 34, 6, 4},
		604: {18, 34, 6, 4},
		605: {0, 38, 12, 8},
		606: {12, 38, 12, 8},
		401: {0, 46, 6, 4},
		402: {6, 46, 6, 4},
		403: {12, 46, 6, 4},
		404: {18, 46, 6, 4},
		500: {0, 50, 24, 1},
		501: {0, 51, 6, 4},
		502: {6, 51, 9, 7},
		503: {15, 51, 9, 7},
		504: {0, 58, 24, 9},
		505: {0, 67, 24, 9},
		506: {0, 76, 24, 7},
		409: {0, 83, 24, 9},
		405: {0, 92, 12, 7},
		406: {12, 92, 12, 7},
		407: {0, 99, 12, 9},
		408: {12, 99, 12, 9},
		410: {0, 108, 12, 8},
		411: {12, 108, 12, 9},
		412: {0, 117, 12, 7},
		413: {12, 117, 12, 9},
	}
	for i, panel := range dashboard.Panels {
		if seenIDs[panel.ID] {
			t.Fatalf("duplicate panel id %d", panel.ID)
		}
		seenIDs[panel.ID] = true
		g := panel.GridPos
		if want, ok := wantGrid[panel.ID]; ok {
			if got := [4]int{g.X, g.Y, g.W, g.H}; got != want {
				t.Errorf("panel %d grid = %v; want %v", panel.ID, got, want)
			}
			delete(wantGrid, panel.ID)
		}
		for _, other := range dashboard.Panels[:i] {
			o := other.GridPos
			if g.X < o.X+o.W && o.X < g.X+g.W && g.Y < o.Y+o.H && o.Y < g.Y+g.H {
				t.Errorf("panels %d and %d overlap", panel.ID, other.ID)
			}
		}
		if panel.Type == "row" {
			section = panel.Title
			continue
		}
		gotPanels = append(gotPanels, struct{ section, title, kind string }{section, panel.Title, panel.Type})
		if panel.Description == "" {
			t.Errorf("panel %q has no description", panel.Title)
		}
		if runtimeSections[section] {
			// T3: these panels were rewritten onto the VictoriaMetrics
			// Prometheus datasource; assert they never regress back to
			// the retired SQLite runtime_rows/runtime_deliveries tables.
			if panel.Datasource.Type != "prometheus" || panel.Datasource.UID != "gentle-runtime-vm" {
				t.Errorf("runtime panel %q must use the gentle-runtime-vm Prometheus datasource, got %+v", panel.Title, panel.Datasource)
			}
			if len(panel.Targets) == 0 {
				t.Errorf("runtime panel %q has no targets", panel.Title)
			}
			for _, tg := range panel.Targets {
				if tg.Datasource.Type != "prometheus" || tg.Datasource.UID != "gentle-runtime-vm" {
					t.Errorf("runtime panel %q target %s must use the gentle-runtime-vm Prometheus datasource, got %+v", panel.Title, tg.QueryType, tg.Datasource)
				}
				if tg.Expr == "" {
					t.Errorf("runtime panel %q target has no PromQL expr", panel.Title)
				}
				if tg.QueryText != "" || tg.RawQueryText != "" {
					t.Errorf("runtime panel %q target still carries a SQL queryText/rawQueryText: %q / %q", panel.Title, tg.QueryText, tg.RawQueryText)
				}
				if runtimeSQLTableRef.MatchString(tg.Expr) {
					t.Errorf("runtime panel %q PromQL expr still references a retired SQLite runtime table: %q", panel.Title, tg.Expr)
				}
			}
			continue
		}
		if panel.Datasource.UID != "gentle-telemetry-sqlite" || len(panel.Targets) != 1 {
			t.Errorf("panel %q has wrong datasource or target count", panel.Title)
			continue
		}
		target := panel.Targets[0]
		if target.QueryType != "table" || target.RawQueryText == "" || target.QueryText != target.RawQueryText {
			t.Errorf("panel %q must use one identical table queryText/rawQueryText pair", panel.Title)
		}
		if strings.Contains(target.RawQueryText, "received_at >= ${__from}") && !strings.Contains(target.RawQueryText, "received_at >= ${__from} * 1000000") {
			t.Errorf("panel %q does not convert the millisecond lower bound to nanoseconds", panel.Title)
		}
		if strings.Contains(target.RawQueryText, "received_at <= ${__to}") && !strings.Contains(target.RawQueryText, "received_at <= ${__to} * 1000000") {
			t.Errorf("panel %q does not convert the millisecond upper bound to nanoseconds", panel.Title)
		}
		if panel.Type == "timeseries" && !reflect.DeepEqual(target.TimeColumns, []string{"time"}) {
			t.Errorf("time-series panel %q does not identify its numeric time column", panel.Title)
		}
		defaults, ok := panel.FieldConfig["defaults"].(map[string]any)
		if !ok {
			t.Errorf("panel %q has no field defaults", panel.Title)
		} else {
			wantUnit := "short"
			if panel.Type == "stat" && panel.Title != "RDD adoption %" && panel.Title != "Subagent coverage" {
				wantUnit = "locale"
			} else if panel.Title == "RDD adoption %" || panel.Title == "Subagent coverage" {
				wantUnit = "percent"
			}
			if defaults["unit"] != wantUnit {
				t.Errorf("panel %q unit = %v; want %q", panel.Title, defaults["unit"], wantUnit)
			}
		}
		configJSON, err := json.Marshal(panel.FieldConfig)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(configJSON), `"axisPlacement":"right"`) {
			t.Errorf("panel %q introduces a second y-axis", panel.Title)
		}
		if panel.Type == "stat" {
			if panel.Options["textMode"] != "value" || panel.Options["colorMode"] != "value" || panel.Options["graphMode"] != "none" {
				t.Errorf("stat panel %q does not use exact-value neutral presentation", panel.Title)
			}
			color := "#73BF69"
			if panel.Title == "Subagent coverage" {
				color = "#8AB8A8"
			}
			if !strings.Contains(string(configJSON), `"color":{"fixedColor":"`+color+`","mode":"fixed"}`) || strings.Contains(string(configJSON), `"thresholds"`) {
				t.Errorf("stat panel %q does not use one fixed color without thresholds", panel.Title)
			}
		}
		if panel.Type == "table" {
			footer, ok := panel.Options["footer"].(map[string]any)
			if !ok || footer["enablePagination"] != false {
				t.Errorf("table panel %q must scroll without pagination", panel.Title)
			}
			if panel.GridPos.H != 9 {
				t.Errorf("table panel %q height = %d; want 9", panel.Title, panel.GridPos.H)
			}
		} else if panel.Type == "stat" && panel.GridPos.H != 4 {
			t.Errorf("stat panel %q height = %d; want 4", panel.Title, panel.GridPos.H)
		} else if panel.Type == "timeseries" && panel.GridPos.H != 8 {
			t.Errorf("time-series panel %q height = %d; want 8", panel.Title, panel.GridPos.H)
		}
		if panel.Title == "Version adoption" {
			for _, fragment := range []string{"instr(key, '-')", "substr(key, 1, instr(key, '-') - 1)", "' (main)'", "GROUP BY Version", "LIMIT 10"} {
				if !strings.Contains(target.RawQueryText, fragment) {
					t.Errorf("version normalization query missing %q", fragment)
				}
			}
		}
		if panel.Title == "Agent adoption" || panel.Title == "Component adoption" || panel.Title == "OS and architecture" || panel.Title == "Version adoption" {
			if panel.Options["showValue"] != "always" || !strings.Contains(string(configJSON), `"fixedColor":"#8AB8A8"`) {
				t.Errorf("categorical adoption panel %q must show values with one neutral color", panel.Title)
			}
		}
		if seconds, ok := map[string]string{
			"Active users, last 24h":    "86400",
			"Active users, last 6h":     "21600",
			"Active users, last 1h":     "3600",
			"Active users, last 15 min": "900",
		}[panel.Title]; ok {
			if strings.Contains(target.RawQueryText, "${__from}") || !strings.Contains(target.RawQueryText, "strftime('%s', 'now')") || !strings.Contains(target.RawQueryText, "- "+seconds+") * 1000000000") || !strings.Contains(target.RawQueryText, "COUNT(DISTINCT install_id)") {
				t.Errorf("active-user panel %q must use its fixed nanosecond wall-clock window", panel.Title)
			}
		}
		if panel.Title == "Active users, last 24h" && panel.Description != "Installs that opened an Axiom session in the last 24 hours; each install reports at most once per day, so shorter windows undercount." {
			t.Error("24-hour active-user panel must explain opportunistic daily reporting")
		}
	}
	if len(wantGrid) != 0 {
		t.Fatalf("missing position-pinned panels: %v", wantGrid)
	}
	if !reflect.DeepEqual(gotPanels, wantPanels) {
		t.Fatalf("panels = %#v; want %#v", gotPanels, wantPanels)
	}

	s, err := OpenStorage(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	to := time.Date(2026, 1, 3, 23, 59, 59, 0, time.UTC).UnixMilli()
	for _, event := range []struct {
		received                                             int64
		kind, install, version, os, arch, agents, components string
		rdd                                                  int
	}{
		{from * 1000000, "install", "install-a", "1.0.0", "linux", "amd64", `["codex"]`, `["sdd","skills"]`, 1},
		{time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC).UnixNano(), "heartbeat", "install-a", "1.0.0", "linux", "amd64", `["codex"]`, `["sdd","skills"]`, 1},
		{time.Date(2026, 1, 2, 13, 0, 0, 0, time.UTC).UnixNano(), "install", "install-b", "1.1.0", "darwin", "arm64", `["pi"]`, `["engram"]`, 0},
		{to * 1000000, "heartbeat", "install-b", "1.1.0", "darwin", "arm64", `["pi"]`, `["engram"]`, 0},
	} {
		if _, err := s.db.Exec(`INSERT INTO events(received_at,event,install_id,version,os,arch,agents_json,components_json,rdd_enabled,counters_json) VALUES(?,?,?,?,?,?,?,?,?,NULL)`, event.received, event.kind, event.install, event.version, event.os, event.arch, event.agents, event.components, event.rdd); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, recent := range []struct {
		received int64
		install  string
	}{
		{now.Add(-5 * time.Minute).UnixNano(), "install-a"},
		{now.Add(-30 * time.Minute).UnixNano(), "install-b"},
	} {
		if _, err := s.db.Exec(`INSERT INTO events(received_at,event,install_id,version,os,arch,agents_json,components_json,rdd_enabled,counters_json) VALUES(?,'heartbeat',?,'1.1.0','linux','amd64','["codex"]','["sdd"]',1,NULL)`, recent.received, recent.install); err != nil {
			t.Fatal(err)
		}
	}
	for _, rollup := range []struct {
		day, metric, key string
		value            int
	}{
		{"2026-01-01", "active_install", "install-a", 1},
		{"2026-01-02", "active_install", "install-a", 1},
		{"2026-01-02", "active_install", "install-b", 1},
		{"2026-01-02", "agent", "codex", 1},
		{"2026-01-02", "agent", "pi", 1},
		{"2026-01-02", "component", "sdd", 1},
		{"2026-01-02", "component", "engram", 1},
		{"2026-01-02", "rdd_enabled", "true", 1},
		{"2026-01-02", "rdd_enabled", "false", 1},
		{"2026-01-02", "version", "1.0.0", 1},
		{"2026-01-02", "version", "1.1.0", 1},
		{"2026-01-02", "version", "1.1.0-0.20260102", 1},
		{"2026-01-02", "version", "1.1.0-20260102", 2},
		{"2026-01-02", "npm_downloads_day", "gentle-pi", 100},
		{"2026-01-02", "npm_downloads_day", "gentle-engram", 200},
		{"2026-01-02", "github_release_downloads_total", "v1.0.0", 10},
		{"2026-01-03", "github_release_downloads_total", "v1.0.0", 20},
		{"2026-01-03", "github_release_downloads_total", "v1.1.0", 10},
	} {
		if _, err := s.db.Exec(`INSERT INTO rollups_daily(day,metric,key,value) VALUES(?,?,?,?)`, rollup.day, rollup.metric, rollup.key, rollup.value); err != nil {
			t.Fatal(err)
		}
	}

	queries := map[string]string{}
	for _, panel := range dashboard.Panels {
		if panel.Type == "row" || panel.Datasource.UID != "gentle-telemetry-sqlite" {
			// T3 moved the runtime panels (Live activity, Subagents) onto
			// VictoriaMetrics/PromQL; they have no SQL to exercise here.
			continue
		}
		query := strings.NewReplacer("${__from}", fmt.Sprint(from), "${__to}", fmt.Sprint(to)).Replace(panel.Targets[0].RawQueryText)
		queries[panel.Title] = query
		t.Run("SQL/"+panel.Title, func(t *testing.T) {
			rows, err := s.db.Query(query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if !rows.Next() {
				t.Fatalf("synthetic fixture did not exercise query: %v", rows.Err())
			}
		})
	}

	assertSingleNumber := func(title string, want float64) {
		t.Helper()
		var got float64
		if err := s.db.QueryRow(queries[title]).Scan(&got); err != nil {
			t.Fatalf("%s: %v", title, err)
		}
		if got != want {
			t.Fatalf("%s = %v; want %v", title, got, want)
		}
	}
	assertSingleNumber("Unique installs all-time", 2)
	assertSingleNumber("Active installs yesterday", 2)
	assertSingleNumber("Active installs in range", 2)
	assertSingleNumber("New installs in range", 2)
	assertSingleNumber("Heartbeats in range", 2)
	assertSingleNumber("RDD adoption %", 50)
	assertSingleNumber("npm downloads latest day", 300)
	assertSingleNumber("GitHub release downloads", 30)
	assertSingleNumber("Active users, last 24h", 2)
	assertSingleNumber("Active users, last 6h", 2)
	assertSingleNumber("Active users, last 1h", 2)
	assertSingleNumber("Active users, last 15 min", 1)

	versionRows, err := s.db.Query(queries["Version adoption"])
	if err != nil {
		t.Fatal(err)
	}
	defer versionRows.Close()
	gotVersions := map[string]int64{}
	for versionRows.Next() {
		var version string
		var installs int64
		if err := versionRows.Scan(&version, &installs); err != nil {
			t.Fatal(err)
		}
		gotVersions[version] = installs
	}
	wantVersions := map[string]int64{"1.0.0": 1, "1.1.0": 1, "1.1.0 (main)": 3}
	if !reflect.DeepEqual(gotVersions, wantVersions) {
		t.Fatalf("versions = %#v; want %#v", gotVersions, wantVersions)
	}
}

func TestRuntimeDashboardEpochBounds(t *testing.T) {
	s, err := OpenStorage(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Largest whole millisecond representable in signed Unix nanoseconds.
	for _, ms := range []int64{0, 1767312000000, 9223372036854} {
		var kind string
		var ns int64
		if err := s.db.QueryRow(`SELECT typeof(? * 1000000), ? * 1000000`, ms, ms).Scan(&kind, &ns); err != nil {
			t.Fatal(err)
		}
		if kind != "integer" || ns != ms*1000000 {
			t.Fatalf("epoch conversion overflow: %d => %s %d", ms, kind, ns)
		}
	}
}
