package model_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
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
		{"max deferred", model.CodexEffort("max"), false},
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
			if len(m) != 14 {
				t.Errorf("%s preset has %d keys, want 14", tc.name, len(m))
			}
			requiredKeys := []string{
				"sdd-explore", "sdd-research", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
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
	// Low-cost: sdd-strong=medium, sdd-mid=medium, sdd-cheap=high
	checkCarrilRow(t, out, "sdd-strong", model.CodexEffortMedium)
	checkCarrilRow(t, out, "sdd-mid", model.CodexEffortMedium)
	checkCarrilRow(t, out, "sdd-cheap", model.CodexEffortHigh)
}

func TestRenderCodexPhaseEfforts_PowerfulTierValues(t *testing.T) {
	out := model.RenderCodexPhaseEfforts(model.CodexModelPresetPowerful(), nil)
	// Powerful: sdd-strong=xhigh, sdd-mid=high, sdd-cheap=high
	checkCarrilRow(t, out, "sdd-strong", model.CodexEffortXHigh)
	checkCarrilRow(t, out, "sdd-mid", model.CodexEffortHigh)
	checkCarrilRow(t, out, "sdd-cheap", model.CodexEffortHigh)
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
			name:       "LowCost",
			preset:     model.CodexModelPresetLowCost(),
			wantStrong: model.CodexEffortMedium,
			wantMid:    model.CodexEffortMedium,
			wantCheap:  model.CodexEffortHigh,
		},
		{
			name:       "Recommended",
			preset:     model.CodexModelPresetRecommended(),
			wantStrong: model.CodexEffortMedium,
			wantMid:    model.CodexEffortHigh,
			wantCheap:  model.CodexEffortHigh,
		},
		{
			name:       "Powerful",
			preset:     model.CodexModelPresetPowerful(),
			wantStrong: model.CodexEffortXHigh,
			wantMid:    model.CodexEffortHigh,
			wantCheap:  model.CodexEffortHigh,
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
	// Validates that CodexTierGroups covers all 14 known phases exactly once
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
		"sdd-explore", "sdd-research", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
	}
	for _, phase := range wantPhases {
		if _, ok := seen[phase]; !ok {
			t.Errorf("CodexTierGroups: phase %q not covered by any carril", phase)
		}
	}
	if len(seen) != 14 {
		t.Errorf("expected 14 phases total, got %d", len(seen))
	}
}

func TestDefaultCarrilModels(t *testing.T) {
	m := model.DefaultCarrilModels()
	if m["sdd-strong"] != "gpt-6-sol" {
		t.Errorf("sdd-strong = %q, want gpt-6-sol", m["sdd-strong"])
	}
	if m["sdd-mid"] != "gpt-6-luna" {
		t.Errorf("sdd-mid = %q, want gpt-6-luna", m["sdd-mid"])
	}
	if m["sdd-cheap"] != "gpt-6-luna" {
		t.Errorf("sdd-cheap = %q, want gpt-6-luna", m["sdd-cheap"])
	}
	if len(m) != 3 {
		t.Errorf("DefaultCarrilModels() has %d entries, want 3", len(m))
	}
}

func TestMigrateLegacyCodexCarrilDefaults(t *testing.T) {
	legacy := map[string]string{
		"sdd-strong": "gpt-5.5",
		"sdd-mid":    "gpt-5.5",
		"sdd-cheap":  "gpt-5.4-mini",
	}
	tests := []struct {
		name  string
		input map[string]string
		want  map[string]string
	}{
		{name: "exact legacy tuple", input: legacy, want: model.DefaultCarrilModels()},
		{name: "different value", input: map[string]string{"sdd-cheap": "gpt-5.4", "sdd-mid": "gpt-5.5", "sdd-strong": "gpt-5.5"}, want: map[string]string{"sdd-cheap": "gpt-5.4", "sdd-mid": "gpt-5.5", "sdd-strong": "gpt-5.5"}},
		{name: "partial map", input: map[string]string{"sdd-strong": "gpt-5.5", "sdd-mid": "gpt-5.5"}, want: map[string]string{"sdd-strong": "gpt-5.5", "sdd-mid": "gpt-5.5"}},
		{name: "extra key", input: map[string]string{"sdd-strong": "gpt-5.5", "sdd-mid": "gpt-5.5", "sdd-cheap": "gpt-5.4-mini", "custom": "gpt-5.6-sol"}, want: map[string]string{"sdd-strong": "gpt-5.5", "sdd-mid": "gpt-5.5", "sdd-cheap": "gpt-5.4-mini", "custom": "gpt-5.6-sol"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.MigrateLegacyCodexCarrilDefaults(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MigrateLegacyCodexCarrilDefaults() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPresetLowCost_ModelEffortPerCarril(t *testing.T) {
	m := model.CodexModelPresetLowCost()
	// Low-cost: Razonamiento=gpt-6-sol/medium, Código=gpt-6-luna/medium, Liviano=gpt-6-luna/high
	// Check that propose/design (Razonamiento/sdd-strong) is medium
	if m["sdd-propose"] != model.CodexEffortMedium {
		t.Errorf("Low-cost preset sdd-propose = %q, want medium", m["sdd-propose"])
	}
	// explore reasons over delivered context, so it rides Razonamiento/sdd-strong.
	if m["sdd-explore"] != model.CodexEffortMedium {
		t.Errorf("Low-cost preset sdd-explore = %q, want medium", m["sdd-explore"])
	}
	// apply (Código/sdd-mid) is medium
	if m["sdd-apply"] != model.CodexEffortMedium {
		t.Errorf("Low-cost preset sdd-apply = %q, want medium", m["sdd-apply"])
	}
	// spec (Liviano/sdd-cheap) is high: transcription is cheap per token, so
	// effort buys accuracy without buying an expensive model.
	if m["sdd-spec"] != model.CodexEffortHigh {
		t.Errorf("Low-cost preset sdd-spec = %q, want high", m["sdd-spec"])
	}

	// Verify Low-cost preset carril models
	carrilModels := model.CodexCarrilModelsForPreset(string(model.CodexPresetLowCost))
	if carrilModels["sdd-strong"] != "gpt-6-sol" {
		t.Errorf("Low-cost preset sdd-strong model = %q, want gpt-6-sol", carrilModels["sdd-strong"])
	}
	if carrilModels["sdd-mid"] != "gpt-6-luna" {
		t.Errorf("Low-cost preset sdd-mid model = %q, want gpt-6-luna", carrilModels["sdd-mid"])
	}
	if carrilModels["sdd-cheap"] != "gpt-6-luna" {
		t.Errorf("Low-cost preset sdd-cheap model = %q, want gpt-6-luna", carrilModels["sdd-cheap"])
	}
}

func TestPresetRecommended_ModelEffortPerCarril(t *testing.T) {
	// Recommended: Razonamiento=gpt-6-sol/medium, Código=gpt-6-luna/high, Liviano=gpt-6-luna/high
	m := model.CodexModelPresetRecommended()
	if m["sdd-propose"] != model.CodexEffortMedium {
		t.Errorf("Recommended preset sdd-propose = %q, want medium", m["sdd-propose"])
	}
	// sdd-apply belongs to Código (sdd-mid): writing code in an agentic loop is
	// effort-sensitive, so the balanced workload policy buys high effort on the
	// cheaper writing model rather than a stronger model at medium.
	if m["sdd-apply"] != model.CodexEffortHigh {
		t.Errorf("Recommended preset sdd-apply = %q, want high (Código carril)", m["sdd-apply"])
	}

	carrilModels := model.CodexCarrilModelsForPreset(string(model.CodexPresetRecommended))
	if carrilModels["sdd-strong"] != "gpt-6-sol" {
		t.Errorf("Recommended preset sdd-strong model = %q, want gpt-6-sol", carrilModels["sdd-strong"])
	}
	if carrilModels["sdd-mid"] != "gpt-6-luna" {
		t.Errorf("Recommended preset sdd-mid model = %q, want gpt-6-luna", carrilModels["sdd-mid"])
	}
	if carrilModels["sdd-cheap"] != "gpt-6-luna" {
		t.Errorf("Recommended preset sdd-cheap model = %q, want gpt-6-luna", carrilModels["sdd-cheap"])
	}
}

func TestPresetPowerful_ModelEffortPerCarril(t *testing.T) {
	// Powerful: Razonamiento=gpt-6-astra/xhigh, Código=gpt-6-sol/high, Liviano=gpt-6-luna/high
	m := model.CodexModelPresetPowerful()
	if m["sdd-propose"] != model.CodexEffortXHigh {
		t.Errorf("Powerful preset sdd-propose = %q, want xhigh", m["sdd-propose"])
	}
	if m["sdd-apply"] != model.CodexEffortHigh {
		t.Errorf("Powerful preset sdd-apply = %q, want high", m["sdd-apply"])
	}

	carrilModels := model.CodexCarrilModelsForPreset(string(model.CodexPresetPowerful))
	if carrilModels["sdd-strong"] != "gpt-6-astra" {
		t.Errorf("Powerful preset sdd-strong model = %q, want gpt-6-astra", carrilModels["sdd-strong"])
	}
	if carrilModels["sdd-mid"] != "gpt-6-sol" {
		t.Errorf("Powerful preset sdd-mid model = %q, want gpt-6-sol", carrilModels["sdd-mid"])
	}
	if carrilModels["sdd-cheap"] != "gpt-6-luna" {
		t.Errorf("Powerful preset sdd-cheap model = %q, want gpt-6-luna", carrilModels["sdd-cheap"])
	}
}

func TestCodexPresetCarrilDefaults_UnknownPresetFallsBackToRecommended(t *testing.T) {
	got := model.CodexPresetCarrilDefaults("unknown-preset")
	want := model.CodexPresetCarrilDefaults(string(model.CodexPresetRecommended))

	for carril, wantDefault := range want {
		if got[carril] != wantDefault {
			t.Errorf("CodexPresetCarrilDefaults(unknown)[%q] = %+v, want recommended %+v", carril, got[carril], wantDefault)
		}
	}
	if len(got) != len(want) {
		t.Errorf("CodexPresetCarrilDefaults(unknown) len = %d, want %d", len(got), len(want))
	}
}

func TestCodexPresetConstantsRemainStringCompatible(t *testing.T) {
	tests := []struct {
		name string
		key  model.CodexPresetKey
		want string
	}{
		{"low cost", model.CodexPresetLowCost, "low-cost"},
		{"recommended", model.CodexPresetRecommended, "recommended"},
		{"powerful", model.CodexPresetPowerful, "powerful"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.key) != tc.want {
				t.Errorf("string(%s) = %q, want %q", tc.name, tc.key, tc.want)
			}
		})
	}
}

// ─── WU-2 RED: RenderCodexPhaseEfforts Model column ───────────────────────────

func TestRenderCodexPhaseEfforts_ModelColumn(t *testing.T) {
	assignments := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEfforts(assignments, nil)
	if !strings.Contains(out, "Model") {
		t.Errorf("RenderCodexPhaseEfforts: table header missing 'Model' column; got:\n%s", out)
	}
	for _, want := range []string{"gpt-6-sol", "gpt-6-luna"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderCodexPhaseEfforts: expected %s in output; got:\n%s", want, out)
		}
	}
}

func TestRenderCodexPhaseEfforts_NilCarrilModels(t *testing.T) {
	assignments := model.CodexModelPresetRecommended()
	out := model.RenderCodexPhaseEfforts(assignments, nil)
	// nil carrilModels: defaults apply; sdd-cheap row must show gpt-6-luna
	if !strings.Contains(out, "gpt-6-luna") {
		t.Errorf("RenderCodexPhaseEfforts(nil): sdd-cheap should show gpt-6-luna; got:\n%s", out)
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
	// The sdd-strong row must show the overridden model, not the default GPT-5.6 model.
	checkCarrilRowModel(t, out, "sdd-strong", "gpt-5.4")
	// Other rows must still show their canonical models.
	checkCarrilRowModel(t, out, "sdd-mid", "gpt-5.5")
	checkCarrilRowModel(t, out, "sdd-cheap", "gpt-5.4-mini")
}

// ─── WU-1: CodexAvailableModels + FilterCodexModelList ─────────────────────

func TestCodexODDPresetsRetainEffortPolicyWithoutSDDPhaseKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		preset func() map[string]model.CodexEffort
		rows   []string
	}{
		{"low cost", model.CodexModelPresetLowCost, []string{
			"| `odd-explorer` | `gpt-6-luna` | `high` |",
			"| `odd-worker` | `gpt-6-luna` | `medium` |",
			"| `odd-verify` | `gpt-6-sol` | `medium` |",
		}},
		{"powerful", model.CodexModelPresetPowerful, []string{
			"| `odd-explorer` | `gpt-6-luna` | `high` |",
			"| `odd-worker` | `gpt-6-sol` | `high` |",
			"| `odd-verify` | `gpt-6-astra` | `xhigh` |",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models := map[string]string{"odd-verify": "gpt-6-sol"}
			if tc.name == "powerful" {
				models["odd-verify"] = "gpt-6-astra"
			}
			preset := map[string]string{"low cost": "low-cost", "powerful": "powerful"}[tc.name]
			for _, efforts := range []map[string]model.CodexEffort{
				tc.preset(),
				model.CodexODDEffortsForPreset(preset),
			} {
				out := model.RenderCodexODDAssignments(models, efforts, model.CodexCarrilModelsForPreset(preset))
				for _, row := range tc.rows {
					if !strings.Contains(out, row) {
						t.Errorf("missing %q in %s", row, out)
					}
				}
			}
		})
	}
}

func TestCodexODDPresetCarrilModelAndEffortMatrix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		preset model.CodexPresetKey
		rows   []string
	}{
		{"low cost", model.CodexPresetLowCost, []string{
			"| `odd-explorer` | `gpt-6-luna` | `high` |",
			"| `odd-worker` | `gpt-6-luna` | `medium` |",
			"| `odd-verify` | `gpt-6-sol` | `medium` |",
		}},
		{"powerful", model.CodexPresetPowerful, []string{
			"| `odd-explorer` | `gpt-6-luna` | `high` |",
			"| `odd-worker` | `gpt-6-sol` | `high` |",
			"| `odd-verify` | `gpt-6-astra` | `xhigh` |",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := model.RenderCodexODDAssignments(nil, model.CodexODDEffortsForPreset(string(tc.preset)), model.CodexCarrilModelsForPreset(string(tc.preset)))
			for _, row := range tc.rows {
				if !strings.Contains(out, row) {
					t.Errorf("missing %q in %s", row, out)
				}
			}
		})
	}
}

func TestCodexODDEffortFallbackIgnoresUnrelatedSDDEffort(t *testing.T) {
	for _, effort := range []model.CodexEffort{"", "invalid"} {
		efforts := model.CodexModelPresetRecommended()
		efforts["sdd-apply"] = model.CodexEffortXHigh
		if effort != "" {
			efforts["odd-worker"] = effort
		}
		out := model.RenderCodexODDAssignments(nil, efforts, nil)
		if !strings.Contains(out, "| `odd-worker` | `gpt-6-luna` | `high` |") {
			t.Errorf("missing/invalid role effort %q inherited unrelated SDD effort: %s", effort, out)
		}
	}
}

func TestCodexODDAssignmentsDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name    string
		models  map[string]string
		efforts map[string]model.CodexEffort
		carrils map[string]string
		rows    []string
	}{
		{"recommended fallback", nil, nil, nil, []string{
			"| `odd-explorer` | `gpt-6-luna` | `high` |",
			"| `odd-worker` | `gpt-6-luna` | `high` |",
			"| `odd-verify` | `gpt-6-sol` | `medium` |",
		}},
		{"custom overrides", map[string]string{"odd-worker": "gpt-6-astra", "rdd-risk": "gpt-6-astra"},
			map[string]model.CodexEffort{"odd-worker": model.CodexEffortXHigh},
			map[string]string{"sdd-cheap": "gpt-6-luna", "sdd-mid": "gpt-6-sol", "sdd-strong": "gpt-6-astra"}, []string{
				"| `odd-explorer` | `gpt-6-luna` | `high` |",
				"| `odd-worker` | `gpt-6-astra` | `xhigh` |",
				"| `odd-verify` | `gpt-6-astra` | `medium` |",
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := model.RenderCodexODDAssignments(tc.models, tc.efforts, tc.carrils)
			for _, row := range tc.rows {
				if !strings.Contains(got, row) {
					t.Errorf("missing %q in %q", row, got)
				}
			}
			if strings.Contains(got, "rdd-risk") {
				t.Errorf("RDD role leaked into ODD table: %q", got)
			}
		})
	}
}

func TestCodexAvailableModels_Contents(t *testing.T) {
	models := model.CodexAvailableModels()
	want := []string{"gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.2-codex"}
	if len(models) != len(want) {
		t.Fatalf("CodexAvailableModels() len = %d, want %d", len(models), len(want))
	}
	for i, w := range want {
		if models[i] != w {
			t.Errorf("CodexAvailableModels()[%d] = %q, want %q", i, models[i], w)
		}
	}
}

func TestFilterCodexModelList_EmptyQuery(t *testing.T) {
	// Empty query returns all models.
	all := model.CodexAvailableModels()
	result := model.FilterCodexModelList(all, "")
	if len(result) != len(all) {
		t.Errorf("FilterCodexModelList(\"\") len = %d, want %d", len(result), len(all))
	}
	for i, m := range all {
		if result[i] != m {
			t.Errorf("FilterCodexModelList(\"\")[%d] = %q, want %q", i, result[i], m)
		}
	}
}

func TestFilterCodexModelList_Match(t *testing.T) {
	tests := []struct {
		query    string
		wantAny  []string
		wantNone []string
	}{
		{
			query:    "sol",
			wantAny:  []string{"gpt-5.6-sol"},
			wantNone: []string{"gpt-5.6-terra", "gpt-5.6-luna"},
		},
		{
			query:    "terra",
			wantAny:  []string{"gpt-5.6-terra"},
			wantNone: []string{"gpt-5.6-sol", "gpt-5.6-luna"},
		},
		{
			query:    "luna",
			wantAny:  []string{"gpt-5.6-luna"},
			wantNone: []string{"gpt-5.6-sol", "gpt-5.6-terra"},
		},
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
	all := model.CodexAvailableModels()
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			result := model.FilterCodexModelList(all, tc.query)
			resultSet := make(map[string]bool, len(result))
			for _, r := range result {
				resultSet[r] = true
			}
			for _, want := range tc.wantAny {
				if !resultSet[want] {
					t.Errorf("FilterCodexModelList(%q): expected %q in result %v", tc.query, want, result)
				}
			}
			for _, noWant := range tc.wantNone {
				if resultSet[noWant] {
					t.Errorf("FilterCodexModelList(%q): unexpected %q in result %v", tc.query, noWant, result)
				}
			}
		})
	}
}

func TestFilterCodexModelList_NoMatch(t *testing.T) {
	result := model.FilterCodexModelList(model.CodexAvailableModels(), "zzz-no-match")
	if len(result) != 0 {
		t.Errorf("FilterCodexModelList(no match) = %v, want empty", result)
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

// TestCodexPresetOrchestratorAssignment_MediumEffortWithPerPresetModel pins the
// two halves of the orchestrator policy separately. The effort is one shared
// rule: the orchestrator plans, routes and adjudicates rather than doing the
// delegated work, so every preset runs it at medium. The model is per-preset:
// low-cost runs the main session on Luna so a Plus plan can still afford Sol
// in the strong lanes, where the reasoning actually pays.
func TestCodexPresetOrchestratorAssignment_MediumEffortWithPerPresetModel(t *testing.T) {
	wantModel := map[model.CodexPresetKey]string{
		model.CodexPresetLowCost:     "gpt-6-luna",
		model.CodexPresetRecommended: "gpt-6-sol",
		model.CodexPresetPowerful:    "gpt-6-astra",
	}
	for preset, want := range wantModel {
		a := model.CodexPresetOrchestratorAssignment(string(preset))
		if a.Model != want || a.Effort != model.CodexEffortMedium {
			t.Errorf("preset %q orchestrator = %s/%s, want %s/medium", preset, a.Model, a.Effort, want)
		}
	}
}
