package model_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestCodexEffortValid(t *testing.T) {
	tests := []struct {
		name  string
		input model.CodexEffort
		want  bool
	}{
		{"low", model.CodexEffortLow, true},
		{"medium", model.CodexEffortMedium, true},
		{"high", model.CodexEffortHigh, true},
		{"xhigh", model.CodexEffortXHigh, true},
		{"empty", model.CodexEffort(""), false},
		{"junk", model.CodexEffort("junk"), false},
		{"uppercase", model.CodexEffort("HIGH"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.Valid(); got != tc.want {
				t.Errorf("CodexEffort(%q).Valid() = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestCodexPresetsCoverAllPhases(t *testing.T) {
	presets := []struct {
		name string
		fn   func() map[string]model.CodexEffort
	}{
		{"Recommended", model.CodexModelPresetRecommended},
		{"Powerful", model.CodexModelPresetPowerful},
		{"LowCost", model.CodexModelPresetLowCost},
	}

	for _, tc := range presets {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.fn()
			if len(m) != 13 {
				t.Errorf("%s preset has %d keys, want 13", tc.name, len(m))
			}
			requiredKeys := []string{
				"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
				"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
				"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
			}
			for _, k := range requiredKeys {
				v, ok := m[k]
				if !ok {
					t.Errorf("%s preset missing key %q", tc.name, k)
					continue
				}
				if !v.Valid() {
					t.Errorf("%s preset[%q] = %q is not a valid CodexEffort", tc.name, k, v)
				}
			}
		})
	}
}

func TestRenderCodexPhaseEfforts_Deterministic(t *testing.T) {
	assignments := model.CodexModelPresetRecommended()
	out1 := model.RenderCodexPhaseEfforts(assignments, nil)
	out2 := model.RenderCodexPhaseEfforts(assignments, nil)
	if out1 != out2 {
		t.Error("RenderCodexPhaseEfforts() is not deterministic: two calls returned different results")
	}
}

func TestRenderCodexPhaseEfforts_NilFallsBackToRecommended(t *testing.T) {
	nilOut := model.RenderCodexPhaseEfforts(nil, nil)
	emptyOut := model.RenderCodexPhaseEfforts(map[string]model.CodexEffort{}, nil)
	recommended := model.RenderCodexPhaseEfforts(model.CodexModelPresetRecommended(), nil)
	if nilOut != recommended {
		t.Error("RenderCodexPhaseEfforts(nil) should equal Recommended output")
	}
	if emptyOut != recommended {
		t.Error("RenderCodexPhaseEfforts(empty) should equal Recommended output")
	}
}

func TestRenderCodexPhaseEfforts_LowCostTierValues(t *testing.T) {
	out := model.RenderCodexPhaseEfforts(model.CodexModelPresetLowCost(), nil)
	// Low-cost: sdd-strong=medium, sdd-mid=medium, sdd-cheap=low
	checkCarrilRow(t, out, "sdd-strong", model.CodexEffortMedium)
	checkCarrilRow(t, out, "sdd-mid", model.CodexEffortMedium)
	checkCarrilRow(t, out, "sdd-cheap", model.CodexEffortLow)
}

func TestRenderCodexPhaseEfforts_PowerfulTierValues(t *testing.T) {
	out := model.RenderCodexPhaseEfforts(model.CodexModelPresetPowerful(), nil)
	// Powerful: sdd-strong=xhigh, sdd-mid=high, sdd-cheap=low
	checkCarrilRow(t, out, "sdd-strong", model.CodexEffortXHigh)
	checkCarrilRow(t, out, "sdd-mid", model.CodexEffortHigh)
	checkCarrilRow(t, out, "sdd-cheap", model.CodexEffortLow)
}

// ─── Targeted fix: carril effort correctness per preset ──────────────────────

// TestRenderCodexPhaseEfforts_CorrectCarrilEfforts asserts that each preset
// renders the correct per-carril effort as determined by the carril intent
// (not by the historical per-phase max). Each row is checked by extracting the
// line that starts with "| `<profile>`" and verifying the effort cell.
func TestRenderCodexPhaseEfforts_CorrectCarrilEfforts(t *testing.T) {
	cases := []struct {
		name       string
		preset     map[string]model.CodexEffort
		wantStrong model.CodexEffort
		wantMid    model.CodexEffort
		wantCheap  model.CodexEffort
	}{
		{
			name:       "LowCost/Plus$20",
			preset:     model.CodexModelPresetLowCost(),
			wantStrong: model.CodexEffortMedium,
			wantMid:    model.CodexEffortMedium,
			wantCheap:  model.CodexEffortLow,
		},
		{
			name:       "Recommended/Pro$100",
			preset:     model.CodexModelPresetRecommended(),
			wantStrong: model.CodexEffortHigh,
			wantMid:    model.CodexEffortMedium,
			wantCheap:  model.CodexEffortLow,
		},
		{
			name:       "Powerful/Pro$200",
			preset:     model.CodexModelPresetPowerful(),
			wantStrong: model.CodexEffortXHigh,
			wantMid:    model.CodexEffortHigh,
			wantCheap:  model.CodexEffortLow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := model.RenderCodexPhaseEfforts(tc.preset, nil)
			checkCarrilRow(t, out, "sdd-strong", tc.wantStrong)
			checkCarrilRow(t, out, "sdd-mid", tc.wantMid)
			checkCarrilRow(t, out, "sdd-cheap", tc.wantCheap)
		})
	}
}

// checkCarrilRow verifies that the table row for profile contains wantEffort in
// the reasoning_effort cell. Format: "| `profile` | `model` | `effort` | phases |"
func checkCarrilRow(t *testing.T, table string, profile string, wantEffort model.CodexEffort) {
	t.Helper()
	needle := "| `" + profile + "`"
	if !strings.Contains(table, needle) {
		t.Errorf("table missing row for profile %q", profile)
		return
	}
	// Find the row text.
	rowStart := strings.Index(table, needle)
	rowEnd := len(table)
	for i := rowStart + 1; i < len(table); i++ {
		if table[i] == '\n' {
			rowEnd = i
			break
		}
	}
	row := table[rowStart:rowEnd]
	effortCell := "| `" + string(wantEffort) + "` |"
	if !strings.Contains(row, effortCell) {
		t.Errorf("profile %q row = %q: want effort cell %q", profile, row, effortCell)
	}
}

// ─── WU-1 RED: carril helpers and defaults ───────────────────────────────────

func TestCodexTierGroups_AllPhasesAssigned(t *testing.T) {
	// Validates that CodexTierGroups covers all 13 known phases exactly once
	// and maps each to one of the three valid carrils.
	tiers := model.CodexTierGroups()
	validCarrils := map[string]bool{
		"sdd-strong": true,
		"sdd-mid":    true,
		"sdd-cheap":  true,
	}
	seen := make(map[string]string) // phase → carril
	for _, g := range tiers {
		if !validCarrils[g.Profile] {
			t.Errorf("CodexTierGroups: unknown carril %q", g.Profile)
		}
		for _, phase := range g.Phases {
			if prev, dup := seen[phase]; dup {
				t.Errorf("phase %q appears in both %q and %q", phase, prev, g.Profile)
			}
			seen[phase] = g.Profile
		}
	}
	wantPhases := []string{
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
	}
	for _, phase := range wantPhases {
		if _, ok := seen[phase]; !ok {
			t.Errorf("CodexTierGroups: phase %q not covered by any carril", phase)
		}
	}
	if len(seen) != 13 {
		t.Errorf("expected 13 phases total, got %d", len(seen))
	}
}

func TestDefaultCarrilModels(t *testing.T) {
	m := model.DefaultCarrilModels()
	if m["sdd-strong"] != "gpt-5.5" {
		t.Errorf("sdd-strong = %q, want gpt-5.5", m["sdd-strong"])
	}
	if m["sdd-mid"] != "gpt-5.5" {
		t.Errorf("sdd-mid = %q, want gpt-5.5", m["sdd-mid"])
	}
	if m["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("sdd-cheap = %q, want gpt-5.4-mini", m["sdd-cheap"])
	}
	if len(m) != 3 {
		t.Errorf("DefaultCarrilModels() has %d entries, want 3", len(m))
	}
}

func TestPresetPlus_ModelEffortPerCarril(t *testing.T) {
	m := model.CodexModelPresetLowCost()
	// Plus $20: Razonamiento=gpt-5.5/medium, Código=gpt-5.5/medium, Liviano=gpt-5.4-mini/low
	// Check that propose/design (Razonamiento/sdd-strong) is medium
	if m["sdd-propose"] != model.CodexEffortMedium {
		t.Errorf("Plus preset sdd-propose = %q, want medium", m["sdd-propose"])
	}
	// apply (Código/sdd-mid) is medium
	if m["sdd-apply"] != model.CodexEffortMedium {
		t.Errorf("Plus preset sdd-apply = %q, want medium", m["sdd-apply"])
	}
	// explore (Liviano/sdd-cheap) is low
	if m["sdd-explore"] != model.CodexEffortLow {
		t.Errorf("Plus preset sdd-explore = %q, want low", m["sdd-explore"])
	}

	// Verify Plus preset carril models
	carrilModels := model.DefaultCarrilModels()
	if carrilModels["sdd-strong"] != "gpt-5.5" {
		t.Errorf("Plus preset sdd-strong model = %q, want gpt-5.5", carrilModels["sdd-strong"])
	}
	if carrilModels["sdd-mid"] != "gpt-5.5" {
		t.Errorf("Plus preset sdd-mid model = %q, want gpt-5.5", carrilModels["sdd-mid"])
	}
	if carrilModels["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("Plus preset sdd-cheap model = %q, want gpt-5.4-mini", carrilModels["sdd-cheap"])
	}
}

func TestPresetPro100_ModelEffortPerCarril(t *testing.T) {
	// Pro $100: Razonamiento=gpt-5.5/high, Código=gpt-5.5/medium, Liviano=gpt-5.4-mini/low
	m := model.CodexModelPresetRecommended()
	if m["sdd-propose"] != model.CodexEffortHigh {
		t.Errorf("Pro100 preset sdd-propose = %q, want high", m["sdd-propose"])
	}
	// sdd-apply belongs to Código (sdd-mid): must be medium in Pro $100, not high.
	if m["sdd-apply"] != model.CodexEffortMedium {
		t.Errorf("Pro100 preset sdd-apply = %q, want medium (Código carril)", m["sdd-apply"])
	}

	carrilModels := model.DefaultCarrilModels()
	if carrilModels["sdd-strong"] != "gpt-5.5" {
		t.Errorf("Pro100 preset sdd-strong model = %q, want gpt-5.5", carrilModels["sdd-strong"])
	}
	if carrilModels["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("Pro100 preset sdd-cheap model = %q, want gpt-5.4-mini", carrilModels["sdd-cheap"])
	}
}

func TestPresetPro200_ModelEffortPerCarril(t *testing.T) {
	// Pro $200: Razonamiento=gpt-5.5/xhigh, Código=gpt-5.5/high, Liviano=gpt-5.4-mini/low
	m := model.CodexModelPresetPowerful()
	if m["sdd-propose"] != model.CodexEffortXHigh {
		t.Errorf("Pro200 preset sdd-propose = %q, want xhigh", m["sdd-propose"])
	}
	if m["sdd-apply"] != model.CodexEffortHigh {
		t.Errorf("Pro200 preset sdd-apply = %q, want high", m["sdd-apply"])
	}

	carrilModels := model.DefaultCarrilModels()
	if carrilModels["sdd-strong"] != "gpt-5.5" {
		t.Errorf("Pro200 preset sdd-strong model = %q, want gpt-5.5", carrilModels["sdd-strong"])
	}
	if carrilModels["sdd-cheap"] != "gpt-5.4-mini" {
		t.Errorf("Pro200 preset sdd-cheap model = %q, want gpt-5.4-mini", carrilModels["sdd-cheap"])
	}
}

// ─── WU-2 RED: RenderCodexPhaseEfforts Model column ───────────────────────────

func TestRenderCodexPhaseEfforts_ModelColumn(t *testing.T) {
	assignments := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEfforts(assignments, nil)
	if !strings.Contains(out, "Model") {
		t.Errorf("RenderCodexPhaseEfforts: table header missing 'Model' column; got:\n%s", out)
	}
	// All rows should contain gpt-5.5 or gpt-5.4-mini.
	if !strings.Contains(out, "gpt-5.5") {
		t.Errorf("RenderCodexPhaseEfforts: expected gpt-5.5 in output; got:\n%s", out)
	}
	if !strings.Contains(out, "gpt-5.4-mini") {
		t.Errorf("RenderCodexPhaseEfforts: expected gpt-5.4-mini in output; got:\n%s", out)
	}
}

func TestRenderCodexPhaseEfforts_NilCarrilModels(t *testing.T) {
	assignments := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEfforts(assignments, nil)
	// nil carrilModels: defaults apply; sdd-cheap row must show gpt-5.4-mini
	if !strings.Contains(out, "gpt-5.4-mini") {
		t.Errorf("RenderCodexPhaseEfforts(nil): sdd-cheap should show gpt-5.4-mini; got:\n%s", out)
	}
}

func TestRenderCodexPhaseEfforts_NonDefaultModel(t *testing.T) {
	// Pass a carril override that differs from the defaults so the test will
	// FAIL if the carrilModels override path is removed from RenderCodexPhaseEfforts.
	assignments := model.CodexModelPresetRecommended()
	carrilModels := map[string]string{
		"sdd-strong": "gpt-5.4", // non-default model for sdd-strong
		"sdd-mid":    "gpt-5.5",
		"sdd-cheap":  "gpt-5.4-mini",
	}
	out := model.RenderCodexPhaseEfforts(assignments, carrilModels)
	// The sdd-strong row must show the overridden model, not the default gpt-5.5.
	checkCarrilRowModel(t, out, "sdd-strong", "gpt-5.4")
	// Other rows must still show their canonical models.
	checkCarrilRowModel(t, out, "sdd-mid", "gpt-5.5")
	checkCarrilRowModel(t, out, "sdd-cheap", "gpt-5.4-mini")
}

// ─── WU-1: CodexAvailableModels + FilterCodexModels ─────────────────────────

func TestCodexAvailableModels_Contents(t *testing.T) {
	models := model.CodexAvailableModels()
	want := []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.2-codex", "gpt-5.3-codex"}
	if len(models) != len(want) {
		t.Fatalf("CodexAvailableModels() len = %d, want %d", len(models), len(want))
	}
	for i, w := range want {
		if models[i] != w {
			t.Errorf("CodexAvailableModels()[%d] = %q, want %q", i, models[i], w)
		}
	}
}

func TestFilterCodexModels_EmptyQuery(t *testing.T) {
	// Empty query returns all models.
	result := model.FilterCodexModels("")
	all := model.CodexAvailableModels()
	if len(result) != len(all) {
		t.Errorf("FilterCodexModels(\"\") len = %d, want %d", len(result), len(all))
	}
	for i, m := range all {
		if result[i] != m {
			t.Errorf("FilterCodexModels(\"\")[%d] = %q, want %q", i, result[i], m)
		}
	}
}

func TestFilterCodexModels_Match(t *testing.T) {
	tests := []struct {
		query    string
		wantAny  []string
		wantNone []string
	}{
		{
			query:    "gpt-5.5",
			wantAny:  []string{"gpt-5.5"},
			wantNone: []string{"gpt-5.4-mini", "gpt-5.2-codex"},
		},
		{
			query:    "codex",
			wantAny:  []string{"gpt-5.2-codex", "gpt-5.3-codex"},
			wantNone: []string{"gpt-5.5", "gpt-5.4"},
		},
		{
			query:    "CODEX", // case-insensitive
			wantAny:  []string{"gpt-5.2-codex", "gpt-5.3-codex"},
			wantNone: []string{"gpt-5.5"},
		},
		{
			query:    "mini",
			wantAny:  []string{"gpt-5.4-mini"},
			wantNone: []string{"gpt-5.5", "gpt-5.4"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			result := model.FilterCodexModels(tc.query)
			resultSet := make(map[string]bool, len(result))
			for _, r := range result {
				resultSet[r] = true
			}
			for _, want := range tc.wantAny {
				if !resultSet[want] {
					t.Errorf("FilterCodexModels(%q): expected %q in result %v", tc.query, want, result)
				}
			}
			for _, noWant := range tc.wantNone {
				if resultSet[noWant] {
					t.Errorf("FilterCodexModels(%q): unexpected %q in result %v", tc.query, noWant, result)
				}
			}
		})
	}
}

func TestFilterCodexModels_NoMatch(t *testing.T) {
	result := model.FilterCodexModels("zzz-no-match")
	if len(result) != 0 {
		t.Errorf("FilterCodexModels(no match) = %v, want empty", result)
	}
}

// ─── WU-4 RED: RenderCodexPhaseEffortsByPhase ────────────────────────────────

// TestRenderCodexPhaseEffortsByPhase_AllPhasesPresent verifies that when a
// per-phase model map is provided, the output contains all 13 phases.
func TestRenderCodexPhaseEffortsByPhase_AllPhasesPresent(t *testing.T) {
	phaseModels := map[string]string{
		"sdd-propose": "gpt-5.5",
		"sdd-apply":   "gpt-5.4",
	}
	efforts := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEffortsByPhase(phaseModels, efforts)

	phases := []string{
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
	}
	for _, phase := range phases {
		if !strings.Contains(out, phase) {
			t.Errorf("RenderCodexPhaseEffortsByPhase missing phase %q; output:\n%s", phase, out)
		}
	}
}

// TestRenderCodexPhaseEffortsByPhase_CustomModelShown verifies that the specific
// phase row shows the exact custom model ID, not just a substring match that would
// pass trivially because gpt-5.4-mini contains "gpt-5.4".
func TestRenderCodexPhaseEffortsByPhase_CustomModelShown(t *testing.T) {
	phaseModels := map[string]string{
		"sdd-propose": "gpt-5.4",
	}
	efforts := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEffortsByPhase(phaseModels, efforts)

	// The sdd-propose row must contain exactly | `sdd-propose` | `gpt-5.4` |
	// (not gpt-5.4-mini or any other model that happens to contain "gpt-5.4").
	wantRow := "| `sdd-propose` | `gpt-5.4` |"
	if !strings.Contains(out, wantRow) {
		t.Errorf("RenderCodexPhaseEffortsByPhase: sdd-propose row missing exact custom model cell %q; output:\n%s", wantRow, out)
	}
}

// TestRenderCodexPhaseEffortsByPhase_UnassignedUsesDefaultModel verifies that
// phases without a custom model assignment fall back to DefaultCarrilModels.
func TestRenderCodexPhaseEffortsByPhase_UnassignedUsesDefaultModel(t *testing.T) {
	// No custom models — all phases should use carril defaults.
	efforts := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEffortsByPhase(nil, efforts)

	// sdd-explore is in sdd-cheap carril → gpt-5.4-mini.
	if !strings.Contains(out, "gpt-5.4-mini") {
		t.Errorf("RenderCodexPhaseEffortsByPhase(nil models): sdd-cheap phases should show gpt-5.4-mini; output:\n%s", out)
	}
}

// TestRenderCodexPhaseEffortsByPhase_HeaderPresent verifies the table has a
// Phase column header.
func TestRenderCodexPhaseEffortsByPhase_HeaderPresent(t *testing.T) {
	out := model.RenderCodexPhaseEffortsByPhase(nil, model.CodexModelPresetRecommended())
	if !strings.Contains(out, "Phase") {
		t.Errorf("RenderCodexPhaseEffortsByPhase: missing 'Phase' header; output:\n%s", out)
	}
}

// checkCarrilRowModel verifies that the table row for profile contains wantModel
// in the model cell. Format: "| `profile` | `model` | `effort` | phases |"
func checkCarrilRowModel(t *testing.T, table string, profile string, wantModel string) {
	t.Helper()
	needle := "| `" + profile + "`"
	rowStart := strings.Index(table, needle)
	if rowStart == -1 {
		t.Errorf("table missing row for profile %q", profile)
		return
	}
	rowEnd := len(table)
	for i := rowStart + 1; i < len(table); i++ {
		if table[i] == '\n' {
			rowEnd = i
			break
		}
	}
	row := table[rowStart:rowEnd]
	modelCell := "| `" + wantModel + "` |"
	if !strings.Contains(row, modelCell) {
		t.Errorf("profile %q row = %q: want model cell %q", profile, row, modelCell)
	}
}
