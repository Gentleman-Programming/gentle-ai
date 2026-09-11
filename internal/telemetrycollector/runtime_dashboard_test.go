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
		Panels []struct {
			ID          int
			Title       string
			Type        string
			Description string
			GridPos     struct{ X, Y, W, H int }
			Datasource  struct{ UID string }
			Targets     []struct{ QueryText, RawQueryText string }
			FieldConfig map[string]any
		}
	}
	if err := json.Unmarshal(data, &dashboard); err != nil {
		t.Fatal(err)
	}
	wantRuntimeGrid := map[int][4]int{
		19: {0, 0, 24, 1}, 20: {0, 1, 24, 5},
		15: {0, 6, 24, 8}, 16: {0, 14, 24, 8},
		21: {0, 22, 24, 10}, 17: {0, 32, 24, 8},
		22: {0, 40, 12, 7}, 18: {12, 40, 12, 7},
	}
	for i, panel := range dashboard.Panels {
		g := panel.GridPos
		if want, ok := wantRuntimeGrid[panel.ID]; ok {
			if got := [4]int{g.X, g.Y, g.W, g.H}; got != want {
				t.Errorf("panel %d grid = %v; want %v", panel.ID, got, want)
			}
			delete(wantRuntimeGrid, panel.ID)
		}
		for _, other := range dashboard.Panels[:i] {
			o := other.GridPos
			if g.X < o.X+o.W && o.X < g.X+g.W && g.Y < o.Y+o.H && o.Y < g.Y+g.H {
				t.Errorf("panels %d and %d overlap", panel.ID, other.ID)
			}
		}
	}
	if len(wantRuntimeGrid) != 0 {
		t.Fatalf("missing runtime panels: %v", wantRuntimeGrid)
	}
	runtimeQueries := 0
	for _, panel := range dashboard.Panels {
		for _, target := range panel.Targets {
			if target.QueryText != target.RawQueryText {
				t.Fatalf("panel %d: query texts differ", panel.ID)
			}
		}
		if panel.ID < 15 {
			if panel.GridPos.Y < 48 {
				t.Fatalf("legacy panel %d precedes runtime section", panel.ID)
			}
			continue
		}
		if panel.ID == 19 {
			if panel.Type != "row" || panel.GridPos.Y != 0 {
				t.Fatal("runtime section must start at top")
			}
			continue
		}
		runtimeQueries++
		if panel.Description == "" || panel.Datasource.UID != "gentle-telemetry-sqlite" || panel.GridPos.Y+panel.GridPos.H > 47 {
			t.Fatalf("runtime panel %d: missing context, datasource or top layout", panel.ID)
		}
		if panel.ID == 16 {
			if panel.Type != "table" || panel.Title != "Usage by agent, model, and effort" {
				t.Error("usage panel must be a dimensional table")
			}
			for _, phrase := range []string{"receipt-time", "Responses and launches are separate", "— means not reported; it is not zero", "not sessions, users or people", "Response evidence", "Selected evidence", "Effective effort is never inferred", "unknown", "without identity inference", "No custom or private names"} {
				if !strings.Contains(panel.Description, phrase) {
					t.Errorf("launch description missing %q", phrase)
				}
			}
			for _, target := range panel.Targets {
				if !strings.HasSuffix(target.QueryText, `ORDER BY COALESCE(Responses, 0) + COALESCE(Launches, 0) DESC, "Agent observation" ASC, "Model observation" ASC, "Selected → effective" ASC LIMIT 1000`) {
					t.Error("launch combinations must be bounded and sorted by count then label")
				}
				group := strings.SplitN(target.QueryText, "GROUP BY", 2)
				for _, dimension := range []string{"agent_class", "agent_kind", "host", "model.provider", "model.id", "model_evidence", "selected_effort", "effective_effort"} {
					if len(group) != 2 || !strings.Contains(group[1], "$."+dimension+"'") {
						t.Errorf("launch grouping missing %s", dimension)
					}
				}
			}
		}
		if panel.ID == 21 {
			if panel.Type != "table" {
				t.Fatal("reported tokens must be a full-width dimensional table")
			}
			var wantConfig map[string]any
			if err := json.Unmarshal([]byte(`{
				"defaults": {"min": 0, "decimals": 0, "noValue": "Not reported", "custom": {"minWidth": 80}},
				"overrides": [
					{"matcher": {"id": "byName", "options": "Model observation"}, "properties": [{"id": "custom.width", "value": 340}]},
					{"matcher": {"id": "byName", "options": "Selected → effective"}, "properties": [{"id": "custom.width", "value": 180}]},
					{"matcher": {"id": "byRegexp", "options": "^(Input|Output|Cache read|Cache creation|Reasoning|Total)$"}, "properties": [{"id": "mappings", "value": [{"type": "special", "options": {"match": "null", "result": {"text": "—"}}}]}]},
					{"matcher": {"id": "byName", "options": "Cache creation"}, "properties": [{"id": "displayName", "value": "Cache write"}]}
				]
			}`), &wantConfig); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(panel.FieldConfig, wantConfig) {
				t.Errorf("reported token config = %v; want compact null mapping and Cache write display header", panel.FieldConfig)
			}
			if !strings.Contains(panel.Description, "— means not reported; it is not zero") {
				t.Error("reported token description must explain the null display")
			}
		}
		if panel.ID != 16 && panel.ID != 17 && panel.ID != 21 && panel.Type != "barchart" && panel.Type != "stat" {
			t.Fatalf("runtime panel %d exposes raw columns", panel.ID)
		}
		for _, target := range panel.Targets {
			if strings.Contains(target.QueryText, "GROUP BY") && !strings.HasSuffix(target.QueryText, "LIMIT 1000") {
				t.Fatalf("runtime panel %d: unbounded groups", panel.ID)
			}
		}
	}
	if runtimeQueries != 7 {
		t.Fatalf("expected to execute all seven runtime queries, found %d", runtimeQueries)
	}
	s, err := OpenStorage(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	from := time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC).UnixMilli()
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).UnixMilli()
	metrics := []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"}
	// Two observations share a delivery; receipt time, not a client date, groups them.
	for i, ns := range []int64{from*1000000 - 1, from * 1000000, to * 1000000, to*1000000 + 1} {
		if _, err := s.db.Exec(`INSERT INTO runtime_deliveries VALUES (?, ?, '{"host":"codex"}')`, fmt.Sprint(i), ns); err != nil {
			t.Fatal(err)
		}
		count := 1
		if i == 1 {
			count = 2
		}
		for ordinal := 0; ordinal < count; ordinal++ {
			kind, category, measured, duration := "request", "none", 1, 12.5
			coverage := "reported"
			if ordinal == 1 {
				kind, category, measured, duration, coverage = "unavailable", "unknown", 0, 0, "unsupported"
			} else if i == 2 {
				kind, category, duration, coverage = "message", "api", 7.25, "unavailable"
			}
			row := map[string]any{
				"model":          map[string]string{"provider": "openai", "id": "gpt-5"},
				"model_evidence": "response", "agent_class": "unknown", "agent_kind": "unknown",
				"selected_effort": "high", "effective_effort": "unavailable",
				"responses": 2, "launches": 3, "error_category": category,
				"duration": map[string]any{"kind": kind, "measured_count": measured, "sum_ms": duration},
			}
			if ordinal == 1 {
				row["duration"].(map[string]any)["sum_ms"] = nil
				row["launches"] = nil
				row["responses"] = nil
				row["model"] = map[string]string{"provider": "openai", "id": "null-only"}
				row["agent_class"] = "null-only"
			}
			if i == 2 {
				row["responses"], row["launches"] = "unsupported", "unsupported"
				row["model"] = map[string]string{"provider": "openai", "id": "gpt-4"}
				row["selected_effort"], row["effective_effort"] = "low", "low"
			}
			for index, metric := range metrics {
				// Nonzero unreported sums must never leak into usage.
				token := map[string]int{"reported": 0, "unavailable": 0, "unsupported": 0, "sum": 99}
				token[coverage] = 1
				if coverage == "reported" {
					token["sum"] = index // Includes explicitly reported zero; total is not inferred.
				}
				row[metric] = token
			}
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`INSERT INTO runtime_rows VALUES (?, ?, ?)`, fmt.Sprint(i), ordinal, string(raw)); err != nil {
				t.Fatal(err)
			}
		}
	}
	var tokenRows [][]any
	for _, index := range []int{3, 2, 0, 1, 4, 5} {
		label := []string{"Input", "Output", "Cache read", "Cache creation", "Reasoning", "Total"}[index]
		tokenRows = append(tokenRows, []any{label, int64(1), int64(1), int64(1)})
	}
	for _, tc := range []struct {
		title string
		want  [][]any
	}{
		{"Runtime overview", [][]any{{int64(3), int64(2), int64(3), int64(5), int64(2)}}},
		{"Responses by model", [][]any{{"openai/gpt-5 · response · codex", int64(2)}}},
		{"Usage by agent, model, and effort", [][]any{{"unknown / unknown", "codex · openai/gpt-5 · response", "high → unavailable", int64(2), int64(3)}}},
		{"Token coverage", tokenRows},
		{"Reported tokens", [][]any{
			{"codex · openai/gpt-4 · response", "low → low", nil, nil, nil, nil, nil, nil},
			{"codex · openai/gpt-5 · response", "high → unavailable", int64(0), int64(1), int64(2), int64(3), int64(4), int64(5)},
			{"codex · openai/null-only · response", "high → unavailable", nil, nil, nil, nil, nil, nil},
		}},
		{"Error observations", [][]any{{"api", int64(1)}, {"unknown", int64(1)}}},
		{"Measured duration", [][]any{{"message", 7.25}, {"request", 12.5}}},
	} {
		t.Run(tc.title, func(t *testing.T) {
			query, found := "", 0
			for _, panel := range dashboard.Panels {
				if panel.Title != tc.title {
					continue
				}
				found++
				if len(panel.Targets) != 1 || panel.Targets[0].QueryText != panel.Targets[0].RawQueryText {
					t.Fatal("expected one identical queryText/rawQueryText pair")
				}
				query = panel.Targets[0].QueryText
			}
			if found != 1 {
				t.Fatalf("expected one runtime panel, found %d", found)
			}
			bounds := "d.received_at >= ${__from} * 1000000 AND d.received_at <= ${__to} * 1000000"
			if !strings.Contains(query, bounds) {
				t.Fatal("missing inclusive nanosecond receipt bounds")
			}
			emptyQuery := strings.NewReplacer("${__from}", "0", "${__to}", "0").Replace(query)
			query = strings.NewReplacer("${__from}", fmt.Sprint(from), "${__to}", fmt.Sprint(to)).Replace(query)
			rows, err := s.db.Query(query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			columns, err := rows.Columns()
			if err != nil {
				t.Fatal(err)
			}
			if tc.title == "Usage by agent, model, and effort" {
				wantColumns := []string{"Agent observation", "Model observation", "Selected → effective", "Responses", "Launches"}
				if !reflect.DeepEqual(columns, wantColumns) {
					t.Fatalf("columns = %v; want %v", columns, wantColumns)
				}
			}
			if tc.title == "Reported tokens" {
				wantColumns := []string{"Model observation", "Selected → effective", "Input", "Output", "Cache read", "Cache creation", "Reasoning", "Total"}
				if !reflect.DeepEqual(columns, wantColumns) {
					t.Fatalf("columns (%d) = %v; want exactly %v", len(columns), columns, wantColumns)
				}
			}
			var got [][]any
			for rows.Next() {
				values, pointers := make([]any, len(columns)), make([]any, len(columns))
				for i := range values {
					pointers[i] = &values[i]
				}
				if err := rows.Scan(pointers...); err != nil {
					t.Fatal(err)
				}
				got = append(got, values)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v; want %#v", got, tc.want)
			}
			rows.Close()
			empty, err := s.db.Query(emptyQuery)
			if err != nil {
				t.Fatal(err)
			}
			defer empty.Close()
			if empty.Next() || empty.Err() != nil {
				t.Fatalf("empty receipt range must not invent zeros: %v", empty.Err())
			}
		})
	}
	t.Run("independent usage reports", func(t *testing.T) {
		// Isolate usage fixtures so other runtime panel expectations stay unchanged.
		tx, err := s.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		for i, report := range []struct{ agent, evidence, responses, launches string }{
			{"orchestrator", "response", "7", "null"},
			{"orchestrator", "response", "2", `"unsupported"`},
			{"subagent", "selected", "null", "6"},
			{"subagent", "selected", `"unsupported"`, "1"},
			{"zero", "response", "0", "1.5"},
			{"zero-launch", "selected", "true", "0"},
			{"excluded", "selected", "1.5", `"unsupported"`},
		} {
			raw := fmt.Sprintf(`{"agent_class":%q,"agent_kind":"unknown","model":{"provider":"openai","id":"gpt-5"},"model_evidence":%q,"selected_effort":"high","effective_effort":"unavailable","responses":%s,"launches":%s}`, report.agent, report.evidence, report.responses, report.launches)
			// Upper bound must be included, as must the original lower-bound row.
			if _, err := tx.Exec(`INSERT INTO runtime_rows VALUES ('2', ?, ?)`, i+10, raw); err != nil {
				t.Fatal(err)
			}
		}
		for _, panel := range dashboard.Panels {
			if panel.ID != 16 {
				continue
			}
			query := strings.NewReplacer("${__from}", fmt.Sprint(from), "${__to}", fmt.Sprint(to)).Replace(panel.Targets[0].QueryText)
			rows, err := tx.Query(query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var got [][]any
			for rows.Next() {
				var agent, model, effort string
				var responses, launches any
				if err := rows.Scan(&agent, &model, &effort, &responses, &launches); err != nil {
					t.Fatal(err)
				}
				got = append(got, []any{agent, model, effort, responses, launches})
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			want := [][]any{
				{"orchestrator / unknown", "codex · openai/gpt-5 · response", "high → unavailable", int64(9), nil},
				{"subagent / unknown", "codex · openai/gpt-5 · selected", "high → unavailable", nil, int64(7)},
				{"unknown / unknown", "codex · openai/gpt-5 · response", "high → unavailable", int64(2), int64(3)},
				{"zero / unknown", "codex · openai/gpt-5 · response", "high → unavailable", int64(0), nil},
				{"zero-launch / unknown", "codex · openai/gpt-5 · selected", "high → unavailable", nil, int64(0)},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("usage rows = %#v; want %#v", got, want)
			}
		}
	})
	t.Run("unavailable-only range", func(t *testing.T) {
		if _, err := s.db.Exec(`UPDATE runtime_rows SET row_json = json_set(row_json,
			'$.responses', NULL, '$.launches', NULL,
			'$.duration.kind', 'unavailable', '$.duration.measured_count', 0, '$.duration.sum_ms', NULL,
			'$.total_tokens.reported', 0, '$.total_tokens.sum', NULL)`); err != nil {
			t.Fatal(err)
		}
		for _, panel := range dashboard.Panels {
			if panel.ID != 15 && panel.ID != 16 && panel.ID != 18 && panel.ID != 20 && panel.ID != 21 {
				continue
			}
			query := strings.NewReplacer("${__from}", fmt.Sprint(from), "${__to}", fmt.Sprint(to)).Replace(panel.Targets[0].QueryText)
			rows, err := s.db.Query(query)
			if err != nil {
				t.Fatal(err)
			}
			if panel.ID == 20 {
				if !rows.Next() {
					t.Fatal("missing received observations")
				}
				var observations, errors int64
				var responses, launches, tokens any
				if err := rows.Scan(&observations, &responses, &launches, &tokens, &errors); err != nil {
					t.Fatal(err)
				}
				if observations != 3 || errors != 2 || responses != nil || launches != nil || tokens != nil {
					t.Fatal("unavailable reports must not become zero or erase observations")
				}
			} else if panel.ID == 21 {
				count := 0
				for rows.Next() {
					var model, effort string
					var input, output, read, creation, reasoning, total any
					if err := rows.Scan(&model, &effort, &input, &output, &read, &creation, &reasoning, &total); err != nil {
						t.Fatal(err)
					}
					if total != nil {
						t.Fatal("unreported total must stay null even when categories are reported")
					}
					if model == "codex · openai/gpt-5 · response" && (input != int64(0) || output != int64(1) || reasoning != int64(4)) {
						t.Fatal("missing total must not erase independently reported categories")
					}
					count++
				}
				if count != 3 {
					t.Fatalf("got %d combinations; want 3", count)
				}
			} else if rows.Next() {
				t.Fatalf("%s invented a distribution or zero latency", panel.Title)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
		}
	})
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
