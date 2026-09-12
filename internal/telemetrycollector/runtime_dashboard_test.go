package telemetrycollector

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
			Datasource  struct{ UID string }
			Targets     []struct {
				QueryType, QueryText, RawQueryText string
				TimeColumns                        []string
			}
			FieldConfig map[string]any
			Options     map[string]any
		}
	}
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.UID != "gentle-ai-usage" || dashboard.Title != "Gentle AI — Usage" {
		t.Fatalf("dashboard identity changed: %q %q", dashboard.UID, dashboard.Title)
	}
	if dashboard.Timezone != "browser" || dashboard.Refresh != "1m" || dashboard.Time.From != "now-7d" || dashboard.Time.To != "now" {
		t.Fatalf("dashboard defaults = timezone %q, refresh %q, range %q to %q", dashboard.Timezone, dashboard.Refresh, dashboard.Time.From, dashboard.Time.To)
	}

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
		{"Where Gentle AI runs", "Agent adoption", "barchart"},
		{"Where Gentle AI runs", "Component adoption", "barchart"},
		{"Where Gentle AI runs", "OS and architecture", "barchart"},
		{"Where Gentle AI runs", "Version adoption", "barchart"},
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
		if panel.Title == "Tokens processed by host" || panel.Title == "Responses by host" {
			if panel.Options["xField"] != "Host" || panel.Options["colorByField"] != "Host" {
				t.Errorf("host bar panel %q must use Host as its category and color field", panel.Title)
			}
			if !strings.Contains(target.RawQueryText, "CAST(") || !strings.Contains(target.RawQueryText, " AS TEXT) AS Host") || !strings.Contains(target.RawQueryText, "GROUP BY Host") {
				t.Errorf("host bar panel %q must return long-form text host rows", panel.Title)
			}
			for host, color := range map[string]string{"pi": "#73BF69", "opencode": "#5794F2", "claude-code": "#FF9830", "codex": "#B877D9"} {
				if !strings.Contains(string(configJSON), `"`+host+`":{"color":"`+color+`"`) {
					t.Errorf("host bar panel %q does not map %s to its fixed color", panel.Title, host)
				}
			}
		}
		if panel.Title == "Tokens processed per hour by host (last 24h)" {
			if panel.TimeFrom != "24h" {
				t.Error("runtime token trend must override its range to the last 24 hours")
			}
			for _, host := range []string{"pi", "opencode", "claude-code", "codex"} {
				if !strings.Contains(string(configJSON), `"options":"`+host+`"`) {
					t.Errorf("runtime token trend does not pin the %s series color", host)
				}
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
		if panel.Title == "Usage by host, subagent, model and effort" {
			if panel.GridPos.X != 0 || panel.GridPos.Y != 83 || panel.GridPos.W != 24 || !strings.Contains(panel.Description, "which subagent used which model at which effort and how many tokens") {
				t.Error("usage table must remain a full-width runtime centerpiece after the Subagents section")
			}
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
		if panel.Title == "Responses by subagent" || panel.Title == "Tokens processed by subagent" {
			if panel.Options["xField"] != "Subagent" || panel.Options["showValue"] != "always" || !strings.Contains(string(configJSON), `"fixedColor":"#8AB8A8"`) {
				t.Errorf("subagent bar panel %q must be a labeled neutral categorical chart", panel.Title)
			}
			for _, fragment := range []string{" AS TEXT) AS Subagent", "NOT IN ('orchestrator', 'unknown')", "GROUP BY Subagent", " DESC"} {
				if !strings.Contains(target.RawQueryText, fragment) {
					t.Errorf("subagent bar query %q missing %q", panel.Title, fragment)
				}
			}
		}
		if panel.Title == "Responses by subagent" {
			for _, fragment := range []string{"$.responses", "$.launches", "COALESCE"} {
				if !strings.Contains(target.RawQueryText, fragment) {
					t.Errorf("subagent response query must count launch-only rows; missing %q", fragment)
				}
			}
		}
		if panel.Title == "Subagent model and effort selection" {
			if panel.GridPos.W != 24 || !strings.Contains(target.RawQueryText, "NOT IN ('orchestrator', 'unknown')") || !strings.Contains(target.RawQueryText, `ORDER BY Responses DESC, "Tokens processed" DESC`) {
				t.Error("subagent selection table must be full-width, filtered, and ordered by activity")
			}
		}
		if panel.Title == "Most popular models per subagent" {
			for _, fragment := range []string{"ROW_NUMBER() OVER (PARTITION BY p.Subagent", "SUM(p.Observations) OVER (PARTITION BY p.Subagent)", "group_concat(DISTINCT Host)", "ORDER BY Subagent ASC, Rank ASC", "NOT IN ('orchestrator', 'unknown')"} {
				if !strings.Contains(target.RawQueryText, fragment) {
					t.Errorf("popular-model query missing %q", fragment)
				}
			}
			if panel.GridPos.W != 24 || !strings.Contains(string(configJSON), `"options":"Share of subagent (%)"`) || !strings.Contains(string(configJSON), `"value":"percent"`) {
				t.Error("popular-model table must be full-width and format share as percent")
			}
		}
		if panel.Title == "Top model per subagent" {
			if panel.Options["xField"] != "Subagent and model" || panel.Options["showValue"] != "always" || !strings.Contains(string(configJSON), `"fixedColor":"#8AB8A8"`) || !strings.Contains(target.RawQueryText, "WHERE Rank = 1") {
				t.Error("top-model panel must be a labeled neutral long-format rank-one chart")
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
		if panel.Title == "Active users, last 24h" && panel.Description != "Installs that opened a Gentle AI session in the last 24 hours; each install reports at most once per day, so shorter windows undercount." {
			t.Error("24-hour active-user panel must explain opportunistic daily reporting")
		}
		if panel.Title == "Deliveries, last 15 min" || panel.Title == "Responses, last 15 min" || panel.Title == "Tokens processed, last 15 min" || panel.Title == "Hosts active, last 15 min" {
			if strings.Contains(target.RawQueryText, "${__from}") || !strings.Contains(target.RawQueryText, "- 900) * 1000000000") || !strings.Contains(panel.Description, "counts agent responses, not people") {
				t.Errorf("live stat %q must use a fixed 15-minute window and explain anonymity", panel.Title)
			}
		}
		if panel.Title == "Responses per minute by host (last 3h)" || panel.Title == "Tokens processed per minute (last 3h)" {
			if panel.TimeFrom != "3h" || strings.Contains(target.RawQueryText, "${__from}") || !strings.Contains(target.RawQueryText, "- 10800) * 1000000000") || !strings.Contains(panel.Description, "counts agent responses, not people") {
				t.Errorf("live series %q must use a fixed three-hour anonymous-activity window", panel.Title)
			}
		}
		if panel.Title == "Responses per minute by host (last 3h)" {
			if !strings.Contains(string(configJSON), `"stacking":{"group":"A","mode":"normal"}`) {
				t.Error("live response series must be stacked")
			}
			for _, host := range []string{"pi", "opencode", "claude-code", "codex"} {
				if !strings.Contains(string(configJSON), `"options":"`+host+`"`) {
					t.Errorf("live response series does not pin the %s color", host)
				}
			}
		}
		if panel.Title == "Tokens processed per minute (last 3h)" && !strings.Contains(string(configJSON), `"fixedColor":"#8AB8A8"`) {
			t.Error("live token series must use the neutral color")
		}
		if _, ok := map[string]bool{
			"Top model per subagent":         true,
			"Responses by subagent":          true,
			"Tokens processed by subagent":   true,
			"Tokens processed by host":       true,
			"Responses by host":              true,
			"Error observations by category": true,
		}[panel.Title]; ok {
			if panel.GridPos.H != 7 || panel.Options["barWidth"] != 0.6 || panel.Options["groupWidth"] != 0.7 {
				t.Errorf("low-cardinality bar %q must use h=7, barWidth=0.6, groupWidth=0.7", panel.Title)
			}
		}
		if panel.Title == "Subagent coverage" {
			if panel.Description != "Share of observations attributed to a named subagent." || !strings.Contains(target.RawQueryText, "NOT IN ('orchestrator', 'unknown')") {
				t.Error("subagent coverage must describe and calculate named-subagent attribution")
			}
		}
		if panel.Title == "Measured duration by kind" && (!strings.Contains(string(configJSON), `"options":"Average ms"`) || !strings.Contains(string(configJSON), `"value":"ms"`)) {
			t.Error("duration table must format only its average column as milliseconds")
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

	token := func(sum int, reported, unavailable, unsupported int) map[string]int {
		return map[string]int{"sum": sum, "reported": reported, "unavailable": unavailable, "unsupported": unsupported}
	}
	runtimeFixtures := []struct {
		id, host string
		received int64
		row      map[string]any
	}{
		{"delivery-codex", "codex", time.Date(2026, 1, 2, 10, 15, 0, 0, time.UTC).UnixNano(), map[string]any{
			"model": map[string]string{"provider": "openai", "id": "gpt-5"}, "model_evidence": "response", "agent_kind": "orchestrator", "agent_class": "orchestrator", "selected_effort": "high", "effective_effort": "high", "launches": 1, "responses": 3,
			"input_tokens": token(10, 1, 0, 0), "output_tokens": token(5, 1, 0, 0), "cache_read_tokens": token(2, 1, 0, 0), "cache_creation_tokens": token(1, 1, 0, 0), "reasoning_tokens": token(3, 1, 0, 0), "total_tokens": token(999, 1, 0, 0),
			"error_category": "none", "duration": map[string]any{"kind": "request", "measured_count": 2, "sum_ms": 50.0},
		}},
		{"delivery-opencode", "opencode", time.Date(2026, 1, 2, 11, 30, 0, 0, time.UTC).UnixNano(), map[string]any{
			"model": map[string]string{"provider": "custom", "id": "custom"}, "model_evidence": "unknown", "agent_kind": "worker", "agent_class": "unknown", "selected_effort": "unknown", "effective_effort": "unknown", "launches": 2, "responses": 2,
			"input_tokens": token(7, 1, 0, 0), "output_tokens": token(4, 1, 0, 0), "cache_read_tokens": token(99, 0, 1, 0), "cache_creation_tokens": token(99, 0, 0, 1), "reasoning_tokens": token(99, 0, 1, 0), "total_tokens": token(999, 0, 1, 0),
			"error_category": "provider", "duration": map[string]any{"kind": "message", "measured_count": 1, "sum_ms": 40.0},
		}},
		{"delivery-sdd", "pi", time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC).UnixNano(), map[string]any{
			"model": map[string]string{"provider": "openai", "id": "gpt-5.6-sol"}, "model_evidence": "selected", "agent_kind": "built_in", "agent_class": "sdd-apply", "selected_effort": "high", "effective_effort": "high", "launches": 3, "responses": 0,
			"input_tokens": token(6, 1, 0, 0), "output_tokens": token(4, 1, 0, 0), "cache_read_tokens": token(0, 0, 1, 0), "cache_creation_tokens": token(0, 0, 1, 0), "reasoning_tokens": token(0, 0, 1, 0), "total_tokens": token(0, 0, 0, 1),
			"error_category": "none", "duration": map[string]any{"kind": "unavailable", "measured_count": 0, "sum_ms": nil},
		}},
		{"delivery-review", "claude-code", time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC).UnixNano(), map[string]any{
			"model": map[string]string{"provider": "anthropic", "id": "claude-sonnet-5"}, "model_evidence": "response", "agent_kind": "built_in", "agent_class": "review-risk", "selected_effort": "xhigh", "effective_effort": "xhigh", "launches": 1, "responses": 5,
			"input_tokens": token(20, 1, 0, 0), "output_tokens": token(10, 1, 0, 0), "cache_read_tokens": token(5, 1, 0, 0), "cache_creation_tokens": token(0, 0, 1, 0), "reasoning_tokens": token(2, 1, 0, 0), "total_tokens": token(37, 1, 0, 0),
			"error_category": "none", "duration": map[string]any{"kind": "unavailable", "measured_count": 0, "sum_ms": nil},
		}},
	}
	for _, fixture := range runtimeFixtures {
		payload, err := json.Marshal(map[string]string{"host": fixture.host})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(fixture.row)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO runtime_deliveries(delivery_id,received_at,canonical_payload) VALUES(?,?,?)`, fixture.id, fixture.received, string(payload)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO runtime_rows(delivery_id,ordinal,row_json) VALUES(?,0,?)`, fixture.id, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	unavailableDuration, err := json.Marshal(map[string]any{
		"model": map[string]string{"provider": "openai", "id": "gpt-5"}, "model_evidence": "response", "agent_kind": "orchestrator", "agent_class": "orchestrator", "selected_effort": "high", "effective_effort": "high", "launches": 0, "responses": 0,
		"input_tokens": token(0, 0, 1, 0), "output_tokens": token(0, 0, 1, 0), "cache_read_tokens": token(0, 0, 1, 0), "cache_creation_tokens": token(0, 0, 1, 0), "reasoning_tokens": token(0, 0, 1, 0), "total_tokens": token(0, 0, 1, 0),
		"error_category": "none", "duration": map[string]any{"kind": "unavailable", "measured_count": 0, "sum_ms": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO runtime_rows(delivery_id,ordinal,row_json) VALUES('delivery-codex',1,?)`, string(unavailableDuration)); err != nil {
		t.Fatal(err)
	}
	for _, additional := range []struct {
		delivery string
		row      map[string]any
	}{
		{"delivery-sdd", map[string]any{
			"model": map[string]string{"provider": "openai-codex", "id": "gpt-5.6-terra"}, "model_evidence": "selected", "agent_kind": "built_in", "agent_class": "sdd-apply", "selected_effort": "medium", "effective_effort": "medium", "launches": 1, "responses": 0,
			"input_tokens": token(5, 1, 0, 0), "output_tokens": token(3, 1, 0, 0), "cache_read_tokens": token(0, 0, 1, 0), "cache_creation_tokens": token(0, 0, 1, 0), "reasoning_tokens": token(0, 0, 1, 0), "total_tokens": token(0, 0, 0, 1),
			"error_category": "none", "duration": map[string]any{"kind": "unavailable", "measured_count": 0, "sum_ms": nil},
		}},
		{"delivery-review", map[string]any{
			"model": map[string]string{"provider": "anthropic", "id": "claude-haiku-4-5"}, "model_evidence": "response", "agent_kind": "built_in", "agent_class": "review-risk", "selected_effort": "high", "effective_effort": "high", "launches": 0, "responses": 2,
			"input_tokens": token(8, 1, 0, 0), "output_tokens": token(4, 1, 0, 0), "cache_read_tokens": token(0, 0, 1, 0), "cache_creation_tokens": token(0, 0, 1, 0), "reasoning_tokens": token(0, 0, 1, 0), "total_tokens": token(12, 1, 0, 0),
			"error_category": "none", "duration": map[string]any{"kind": "unavailable", "measured_count": 0, "sum_ms": nil},
		}},
	} {
		raw, err := json.Marshal(additional.row)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO runtime_rows(delivery_id,ordinal,row_json) VALUES(?,1,?)`, additional.delivery, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	for _, live := range []struct {
		id, host                 string
		received                 int64
		responses, input, output int
	}{
		{"live-codex", "codex", now.Add(-5 * time.Minute).UnixNano(), 3, 6, 4},
		{"live-pi", "pi", now.Add(-10 * time.Minute).UnixNano(), 2, 3, 2},
	} {
		payload, err := json.Marshal(map[string]string{"host": live.host})
		if err != nil {
			t.Fatal(err)
		}
		row, err := json.Marshal(map[string]any{
			"model": map[string]string{"provider": "unknown", "id": "unknown"}, "model_evidence": "unknown", "agent_kind": "orchestrator", "agent_class": "orchestrator", "selected_effort": "unknown", "effective_effort": "unknown", "launches": 0, "responses": live.responses,
			"input_tokens": token(live.input, 1, 0, 0), "output_tokens": token(live.output, 1, 0, 0), "cache_read_tokens": token(0, 0, 1, 0), "cache_creation_tokens": token(0, 0, 1, 0), "reasoning_tokens": token(0, 0, 1, 0), "total_tokens": token(live.input+live.output, 1, 0, 0),
			"error_category": "none", "duration": map[string]any{"kind": "unavailable", "measured_count": 0, "sum_ms": nil},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO runtime_deliveries(delivery_id,received_at,canonical_payload) VALUES(?,?,?)`, live.id, live.received, string(payload)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO runtime_rows(delivery_id,ordinal,row_json) VALUES(?,0,?)`, live.id, string(row)); err != nil {
			t.Fatal(err)
		}
	}

	queries := map[string]string{}
	for _, panel := range dashboard.Panels {
		if panel.Type == "row" {
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
	assertSingleNumber("Deliveries, last 15 min", 2)
	assertSingleNumber("Responses, last 15 min", 5)
	assertSingleNumber("Tokens processed, last 15 min", 15)
	assertSingleNumber("Hosts active, last 15 min", 2)
	assertSingleNumber("Tokens processed", 99)
	assertSingleNumber("Responses", 12)
	assertSingleNumber("Deliveries", 4)
	assertSingleNumber("Hosts reporting", 4)
	assertSingleNumber("Subagent coverage", 100.0*4.0/7.0)

	for _, tc := range []struct {
		title string
		want  [][]any
	}{
		{"Tokens processed by host", [][]any{{"claude-code", int64(49)}, {"codex", int64(21)}, {"pi", int64(18)}, {"opencode", int64(11)}}},
		{"Responses by host", [][]any{{"claude-code", int64(7)}, {"codex", int64(3)}, {"opencode", int64(2)}, {"pi", int64(0)}}},
	} {
		t.Run(tc.title+" long format", func(t *testing.T) {
			rows, err := s.db.Query(queries[tc.title])
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if columns, err := rows.Columns(); err != nil || !reflect.DeepEqual(columns, []string{"Host", strings.TrimSuffix(tc.title, " by host")}) {
				t.Fatalf("columns = %v, err = %v", columns, err)
			}
			var got [][]any
			for rows.Next() {
				var host string
				var measure int64
				if err := rows.Scan(&host, &measure); err != nil {
					t.Fatal(err)
				}
				got = append(got, []any{host, measure})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rows = %#v; want %#v", got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		title string
		want  [][]any
	}{
		{"Responses by subagent", [][]any{{"review-risk", int64(8)}, {"sdd-apply", int64(4)}}},
		{"Tokens processed by subagent", [][]any{{"review-risk", int64(49)}, {"sdd-apply", int64(18)}}},
	} {
		t.Run(tc.title+" named rows", func(t *testing.T) {
			rows, err := s.db.Query(queries[tc.title])
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var got [][]any
			for rows.Next() {
				var subagent string
				var measure int64
				if err := rows.Scan(&subagent, &measure); err != nil {
					t.Fatal(err)
				}
				got = append(got, []any{subagent, measure})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rows = %#v; want %#v", got, tc.want)
			}
		})
	}

	subagentRows, err := s.db.Query(queries["Subagent model and effort selection"])
	if err != nil {
		t.Fatal(err)
	}
	defer subagentRows.Close()
	if columns, err := subagentRows.Columns(); err != nil || !reflect.DeepEqual(columns, []string{"Host", "Subagent", "Provider/model", "Evidence", "Selected effort", "Effective effort", "Launches", "Responses", "Tokens processed", "Avg tokens per response"}) {
		t.Fatalf("subagent columns = %v, err = %v", columns, err)
	}
	var gotSubagents [][]any
	for subagentRows.Next() {
		values, pointers := make([]any, 10), make([]any, 10)
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := subagentRows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		gotSubagents = append(gotSubagents, values)
	}
	wantSubagents := [][]any{
		{"claude-code", "review-risk", "anthropic/claude-sonnet-5", "response", "xhigh", "xhigh", int64(1), int64(5), int64(37), 7.4},
		{"claude-code", "review-risk", "anthropic/claude-haiku-4-5", "response", "high", "high", int64(0), int64(2), int64(12), float64(6)},
		{"pi", "sdd-apply", "openai/gpt-5.6-sol", "selected", "high", "high", int64(3), int64(0), int64(10), nil},
		{"pi", "sdd-apply", "openai-codex/gpt-5.6-terra", "selected", "medium", "medium", int64(1), int64(0), int64(8), nil},
	}
	if !reflect.DeepEqual(gotSubagents, wantSubagents) {
		t.Fatalf("subagent rows = %#v; want %#v", gotSubagents, wantSubagents)
	}

	popularRows, err := s.db.Query(queries["Most popular models per subagent"])
	if err != nil {
		t.Fatal(err)
	}
	defer popularRows.Close()
	if columns, err := popularRows.Columns(); err != nil || !reflect.DeepEqual(columns, []string{"Subagent", "Rank", "Provider/model", "Most common selected effort", "Observations", "Share of subagent (%)", "Hosts"}) {
		t.Fatalf("popular-model columns = %v, err = %v", columns, err)
	}
	var gotPopular [][]any
	for popularRows.Next() {
		values, pointers := make([]any, 7), make([]any, 7)
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := popularRows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		gotPopular = append(gotPopular, values)
	}
	wantPopular := [][]any{
		{"review-risk", int64(1), "anthropic/claude-sonnet-5", "xhigh", int64(6), float64(75), "claude-code"},
		{"review-risk", int64(2), "anthropic/claude-haiku-4-5", "high", int64(2), float64(25), "claude-code"},
		{"sdd-apply", int64(1), "openai/gpt-5.6-sol", "high", int64(3), float64(75), "pi"},
		{"sdd-apply", int64(2), "openai-codex/gpt-5.6-terra", "medium", int64(1), float64(25), "pi"},
	}
	if !reflect.DeepEqual(gotPopular, wantPopular) {
		t.Fatalf("popular-model rows = %#v; want %#v", gotPopular, wantPopular)
	}

	topRows, err := s.db.Query(queries["Top model per subagent"])
	if err != nil {
		t.Fatal(err)
	}
	defer topRows.Close()
	var gotTop [][]any
	for topRows.Next() {
		var label string
		var observations int64
		if err := topRows.Scan(&label, &observations); err != nil {
			t.Fatal(err)
		}
		gotTop = append(gotTop, []any{label, observations})
	}
	wantTop := [][]any{
		{"review-risk · anthropic/claude-sonnet-5", int64(6)},
		{"sdd-apply · openai/gpt-5.6-sol", int64(3)},
	}
	if !reflect.DeepEqual(gotTop, wantTop) {
		t.Fatalf("top-model rows = %#v; want %#v", gotTop, wantTop)
	}

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

	durationRows, err := s.db.Query(queries["Measured duration by kind"])
	if err != nil {
		t.Fatal(err)
	}
	defer durationRows.Close()
	var gotDurations [][]any
	for durationRows.Next() {
		var kind string
		var measured int64
		var average float64
		if err := durationRows.Scan(&kind, &measured, &average); err != nil {
			t.Fatal(err)
		}
		gotDurations = append(gotDurations, []any{kind, measured, average})
	}
	wantDurations := [][]any{{"request", int64(2), float64(25)}, {"message", int64(1), float64(40)}}
	if !reflect.DeepEqual(gotDurations, wantDurations) {
		t.Fatalf("duration rows = %#v; want %#v", gotDurations, wantDurations)
	}
	if strings.Contains(queries["Measured duration by kind"], "= 'request'") || !strings.Contains(queries["Measured duration by kind"], "<> 'unavailable'") {
		t.Fatal("duration panel must include every measured kind and exclude unavailable rows")
	}

	usageRows, err := s.db.Query(queries["Usage by host, subagent, model and effort"])
	if err != nil {
		t.Fatal(err)
	}
	defer usageRows.Close()
	var gotUsage [][]any
	for usageRows.Next() {
		values, pointers := make([]any, 9), make([]any, 9)
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := usageRows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		gotUsage = append(gotUsage, values)
	}
	wantUsage := [][]any{
		{"claude-code", "built_in", "review-risk", "anthropic/claude-sonnet-5", "xhigh", "xhigh", int64(1), int64(5), int64(37)},
		{"codex", "orchestrator", "orchestrator", "openai/gpt-5", "high", "high", int64(1), int64(3), int64(21)},
		{"opencode", "worker", "unknown", "custom/custom", "unknown", "unknown", int64(2), int64(2), int64(11)},
		{"pi", "built_in", "sdd-apply", "openai/gpt-5.6-sol", "high", "high", int64(3), int64(0), int64(10)},
		{"claude-code", "built_in", "review-risk", "anthropic/claude-haiku-4-5", "high", "high", int64(0), int64(2), int64(12)},
		{"pi", "built_in", "sdd-apply", "openai-codex/gpt-5.6-terra", "medium", "medium", int64(1), int64(0), int64(8)},
	}
	if !reflect.DeepEqual(gotUsage, wantUsage) {
		t.Fatalf("usage rows = %#v; want %#v", gotUsage, wantUsage)
	}
	if strings.Contains(queries["Tokens processed"], "$.total_tokens.sum") {
		t.Fatal("primary token measure must be derived from token categories, not total_tokens")
	}
	if !strings.Contains(queries["Token coverage per field"], "$.total_tokens.reported") {
		t.Fatal("coverage must retain explicitly reported total_tokens")
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
