package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestPaperReviewTemplateHasExpectedPhases(t *testing.T) {
	def, ok := Template("paper-review")
	if !ok {
		t.Fatal("Template(paper-review) returned false")
	}

	wantIDs := []string{
		"identify-gap",
		"evaluate-writing",
		"verify-data",
		"review-figures",
		"check-style",
		"verify-code",
		"structural-review",
		"final-report",
	}

	gotIDs := make([]string, 0, len(def.Phases))
	for _, p := range def.Phases {
		gotIDs = append(gotIDs, p.ID)
	}

	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("phase ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestPaperReviewTemplateMatchesProposalExample(t *testing.T) {
	// The proposal example is the contract. We re-state it here as a
	// fixture and assert key properties hold (gate count, gate skill,
	// critical dependencies). Re-stating the whole JSON would be
	// brittle and is unnecessary.
	def, ok := Template("paper-review")
	if !ok {
		t.Fatal("Template(paper-review) returned false")
	}

	phasesByID := make(map[string]WorkflowPhase, len(def.Phases))
	for _, p := range def.Phases {
		phasesByID[p.ID] = p
	}

	// Every phase except identify-gap and the three parallel ones must
	// have at least one dep.
	for _, id := range []string{"check-style", "verify-code", "structural-review", "final-report"} {
		p := phasesByID[id]
		if len(p.DependsOn) == 0 {
			t.Errorf("phase %q: expected at least one depends_on", id)
		}
	}

	// structural-review must depend on evaluate-writing AND check-style.
	// (proposal says so explicitly.)
	sr := phasesByID["structural-review"]
	wantDeps := map[string]bool{"evaluate-writing": true, "check-style": true}
	for _, dep := range sr.DependsOn {
		if !wantDeps[dep] {
			t.Errorf("structural-review has unexpected dep %q", dep)
		}
	}

	// final-report must depend on all seven prior phases.
	fr := phasesByID["final-report"]
	if len(fr.DependsOn) != 7 {
		t.Errorf("final-report depends_on count = %d, want 7 (all prior phases)", len(fr.DependsOn))
	}

	// Each phase has at least one gate.
	for _, p := range def.Phases {
		if len(p.ValidationGates) == 0 {
			t.Errorf("phase %q: expected at least one validation gate", p.ID)
		}
	}

	// produces_code must be false (paper review is non-code by spec).
	if def.ProducesCode {
		t.Error("ProducesCode = true, want false for paper-review")
	}

	// strict_tdd must be explicitly false.
	if def.Validation.StrictTDD == nil {
		t.Error("StrictTDD = nil, want pointer to false")
	} else if *def.Validation.StrictTDD {
		t.Error("StrictTDD = true, want false")
	}
}

func TestTemplateReturnsCopy(t *testing.T) {
	// Mutating the returned definition must not affect subsequent calls.
	first, _ := Template("paper-review")
	first.Phases[0].ID = "mutated"

	second, _ := Template("paper-review")
	if second.Phases[0].ID == "mutated" {
		t.Error("Template() returned a shared slice; mutation leaked")
	}
}

func TestTemplateNamesAreSorted(t *testing.T) {
	names := TemplateNames()
	if !sort.StringsAreSorted(names) {
		t.Errorf("TemplateNames() = %v, want sorted", names)
	}
}

func TestTemplateUnknownReturnsFalse(t *testing.T) {
	_, ok := Template("does-not-exist")
	if ok {
		t.Error("Template(unknown) returned ok=true, want false")
	}
}

// ─── SDD built-in workflow template (PR 4) ─────────────────────────────────

func TestSDDTemplateHasExpectedPhases(t *testing.T) {
	def := SDDTemplate()

	wantIDs := []string{
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
	}

	gotIDs := make([]string, 0, len(def.Phases))
	for _, p := range def.Phases {
		gotIDs = append(gotIDs, p.ID)
	}

	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("SDD template: got %d phases, want %d\n  got:  %v\n  want: %v",
			len(gotIDs), len(wantIDs), gotIDs, wantIDs)
	}

	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Errorf("SDD template phase[%d] id = %q, want %q", i, gotIDs[i], want)
		}
	}
}

func TestSDDTemplateHasLinearDependencies(t *testing.T) {
	def := SDDTemplate()
	phasesByID := make(map[string]WorkflowPhase, len(def.Phases))
	for _, p := range def.Phases {
		phasesByID[p.ID] = p
	}

	// Each phase (except the first) must depend on the previous phase.
	expectedDeps := []struct {
		phase string
		depOn string
	}{
		{"sdd-explore", ""},
		{"sdd-propose", "sdd-explore"},
		{"sdd-spec", "sdd-propose"},
		{"sdd-design", "sdd-spec"},
		{"sdd-tasks", "sdd-design"},
		{"sdd-apply", "sdd-tasks"},
		{"sdd-verify", "sdd-apply"},
		{"sdd-archive", "sdd-verify"},
	}

	for _, ed := range expectedDeps {
		p := phasesByID[ed.phase]
		if ed.depOn == "" {
			if len(p.DependsOn) != 0 {
				t.Errorf("phase %q: expected no depends_on, got %v", ed.phase, p.DependsOn)
			}
		} else {
			if len(p.DependsOn) != 1 || p.DependsOn[0] != ed.depOn {
				t.Errorf("phase %q: expected depends_on [%q], got %v", ed.phase, ed.depOn, p.DependsOn)
			}
		}
	}
}

func TestSDDTemplateHasStrictTDDTrue(t *testing.T) {
	def := SDDTemplate()
	if def.Validation.StrictTDD == nil {
		t.Fatal("SDD template: Validation.StrictTDD is nil, want pointer to true")
	}
	if !*def.Validation.StrictTDD {
		t.Error("SDD template: *Validation.StrictTDD = false, want true")
	}
}

func TestSDDTemplateProducesCode(t *testing.T) {
	def := SDDTemplate()
	if !def.ProducesCode {
		t.Error("SDD template: ProducesCode = false, want true (SDD produces code)")
	}
}

func TestSDDTemplateHasCorrectName(t *testing.T) {
	def := SDDTemplate()
	if def.Name != "sdd" {
		t.Errorf("SDD template: Name = %q, want %q", def.Name, "sdd")
	}
}

func TestSDDTemplateHasSupportedVersion(t *testing.T) {
	def := SDDTemplate()
	if def.Version != SupportedSchemaVersion {
		t.Errorf("SDD template: Version = %d, want %d", def.Version, SupportedSchemaVersion)
	}
}

func TestSDDTemplateValidatesCleanly(t *testing.T) {
	def := SDDTemplate()
	result := Validate(&def, ValidationOptions{})
	if result.HasErrors() {
		t.Errorf("SDD template validation errors: %+v", result.Issues)
	}
}

func TestSDDTemplateIsNotInBuiltInTemplates(t *testing.T) {
	// SDD is a system workflow, not a user-initializable template.
	if _, ok := BuiltInTemplates["sdd"]; ok {
		t.Error("SDD template should NOT be in BuiltInTemplates (system workflow)")
	}
}

func TestSDDTemplateNoValidationGates(t *testing.T) {
	// SDD handles validation internally through its pipeline, not via
	// workflow-level validation gates.
	def := SDDTemplate()
	for i, p := range def.Phases {
		if len(p.ValidationGates) > 0 {
			t.Errorf("SDD template phase[%d] %q: unexpected validation gates: %d gates found",
				i, p.ID, len(p.ValidationGates))
		}
	}
}

func TestSDDTemplateNameConsistentAcrossCalls(t *testing.T) {
	// Multiple calls must return consistent data.
	def1 := SDDTemplate()
	def2 := SDDTemplate()
	if def1.Name != def2.Name {
		t.Errorf("SDDTemplate() name changed: %q vs %q", def1.Name, def2.Name)
	}
	if len(def1.Phases) != len(def2.Phases) {
		t.Errorf("SDDTemplate() phase count changed: %d vs %d", len(def1.Phases), len(def2.Phases))
	}
}

func TestTemplateUnknownDoesNotReturnSDD(t *testing.T) {
	// SDD is NOT a user template — Template("sdd") must return false.
	_, ok := Template("sdd")
	if ok {
		t.Error("Template(\"sdd\") returned ok=true — SDD should not be a user template")
	}
}

func TestTemplateListDoesNotIncludeSDD(t *testing.T) {
	names := TemplateNames()
	for _, name := range names {
		if name == "sdd" {
			t.Fatal("TemplateNames() includes \"sdd\" — SDD should not be a user template")
		}
	}
}

func TestSDDTemplateEnumeratesAllPhases(t *testing.T) {
	// Belt-and-suspenders: confirm no phase has empty ID or artifact.
	def := SDDTemplate()
	for i, p := range def.Phases {
		if p.ID == "" {
			t.Errorf("SDD template phase[%d]: empty ID", i)
		}
		if p.Name == "" {
			t.Errorf("SDD template phase[%d] %q: empty Name", i, p.ID)
		}
	}
}

// ─── Academic Article Review template (PR 1) ────────────────────────────────

func TestAcademicArticleReviewTemplate_Exists(t *testing.T) {
	def, ok := Template("academic-article-review")
	if !ok {
		t.Fatal("Template(academic-article-review) returned false")
	}

	// 11 phases matching the spec's 8 conceptual phases with sub-phases (3A, 3B, 5A, 5B, 5C).
	if len(def.Phases) != 11 {
		t.Fatalf("expected 11 phases, got %d", len(def.Phases))
	}
}

func TestAcademicArticleReviewTemplate_HasExpectedPhaseIDs(t *testing.T) {
	def, ok := Template("academic-article-review")
	if !ok {
		t.Fatal("Template(academic-article-review) returned false")
	}

	wantIDs := []string{
		"global-reading",
		"initial-diagnosis",
		"scientific-review",
		"narrative-review",
		"improvement-plan",
		"scientific-improvements",
		"experiment-improvements",
		"writing-improvements",
		"coherence-review",
		"reviewer-simulation",
		"submission-preparation",
	}

	gotIDs := make([]string, 0, len(def.Phases))
	for _, p := range def.Phases {
		gotIDs = append(gotIDs, p.ID)
	}

	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("phase count = %d, want %d\n  got:  %v\n  want: %v",
			len(gotIDs), len(wantIDs), gotIDs, wantIDs)
	}

	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Errorf("phase[%d] id = %q, want %q", i, gotIDs[i], want)
		}
	}
}

func TestAcademicArticleReviewTemplate_Dependencies(t *testing.T) {
	def, ok := Template("academic-article-review")
	if !ok {
		t.Fatal("Template(academic-article-review) returned false")
	}
	phasesByID := make(map[string]WorkflowPhase, len(def.Phases))
	for _, p := range def.Phases {
		phasesByID[p.ID] = p
	}

	tests := []struct {
		phase    string
		wantDeps []string
	}{
		{"global-reading", nil},
		{"initial-diagnosis", []string{"global-reading"}},
		{"scientific-review", []string{"initial-diagnosis"}},
		{"narrative-review", []string{"initial-diagnosis"}},
		{"improvement-plan", []string{"scientific-review", "narrative-review"}},
		{"scientific-improvements", []string{"improvement-plan"}},
		{"experiment-improvements", []string{"improvement-plan"}},
		{"writing-improvements", []string{"improvement-plan"}},
		{"coherence-review", []string{"scientific-improvements", "experiment-improvements", "writing-improvements"}},
		{"reviewer-simulation", []string{"coherence-review"}},
		{"submission-preparation", []string{"reviewer-simulation"}},
	}

	for _, tt := range tests {
		p, ok := phasesByID[tt.phase]
		if !ok {
			t.Fatalf("phase %q not found", tt.phase)
		}
		if tt.wantDeps == nil {
			if len(p.DependsOn) != 0 {
				t.Errorf("phase %q: expected no depends_on, got %v", tt.phase, p.DependsOn)
			}
			continue
		}
		if len(p.DependsOn) != len(tt.wantDeps) {
			t.Errorf("phase %q: depends_on = %v, want %v", tt.phase, p.DependsOn, tt.wantDeps)
			continue
		}
		depSet := make(map[string]bool, len(tt.wantDeps))
		for _, d := range tt.wantDeps {
			depSet[d] = true
		}
		for _, d := range p.DependsOn {
			if !depSet[d] {
				t.Errorf("phase %q: unexpected dep %q", tt.phase, d)
			}
		}
	}
}

func TestAcademicArticleReviewTemplate_ProducesCodeFalse(t *testing.T) {
	def, ok := Template("academic-article-review")
	if !ok {
		t.Fatal("Template(academic-article-review) returned false")
	}
	if def.ProducesCode {
		t.Error("ProducesCode = true, want false (article review is non-code)")
	}
}

func TestAcademicArticleReviewTemplate_StrictTDDExplicitFalse(t *testing.T) {
	def, ok := Template("academic-article-review")
	if !ok {
		t.Fatal("Template(academic-article-review) returned false")
	}
	if def.Validation.StrictTDD == nil {
		t.Fatal("StrictTDD is nil, want pointer to false")
	}
	if *def.Validation.StrictTDD {
		t.Error("StrictTDD = true, want false (article review is non-code)")
	}
}

func TestAcademicArticleReviewTemplateNamesIncludeBoth(t *testing.T) {
	names := TemplateNames()
	foundPaper := false
	foundAcademic := false
	for _, name := range names {
		if name == "paper-review" {
			foundPaper = true
		}
		if name == "academic-article-review" {
			foundAcademic = true
		}
	}
	if !foundPaper {
		t.Error("TemplateNames() missing paper-review")
	}
	if !foundAcademic {
		t.Error("TemplateNames() missing academic-article-review")
	}
}

func TestAcademicArticleReviewTemplate_IsRegisteredInBuiltInTemplates(t *testing.T) {
	_, ok := BuiltInTemplates["academic-article-review"]
	if !ok {
		t.Error("BuiltInTemplates missing academic-article-review")
	}
}

func TestAcademicArticleReviewTemplate_OnDiskJSONValidates(t *testing.T) {
	// Load the on-disk workflow.json and assert it passes validation.
	// We read the file directly rather than using Load() because Load() returns
	// the built-in template first (Template() → BuiltInTemplates), bypassing
	// the on-disk JSON.
	jsonPath := filepath.Join("../../openspec/workflows/academic-article-review/workflow.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", jsonPath, err)
	}
	result, parseErr := Parse(data)
	if parseErr != nil {
		t.Fatalf("Parse(workflow.json): %v", parseErr)
	}
	if result.Definition == nil {
		t.Fatal("Parse returned nil Definition")
	}

	// Create skill fixtures for validation.
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"academic-researcher"} {
		skillPath := filepath.Join(skillDir, name)
		if err := os.MkdirAll(skillPath, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\nbody\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	valResult := Validate(result.Definition, ValidationOptions{ExtraSkillDirs: []string{skillDir}})
	if valResult.HasErrors() {
		t.Errorf("Validate(on-disk academic-article-review JSON) unexpected errors: %+v", valResult.Issues)
	}
}

func TestAcademicArticleReviewTemplate_PhaseNamesAreSpanish(t *testing.T) {
	def, ok := Template("academic-article-review")
	if !ok {
		t.Fatal("Template(academic-article-review) returned false")
	}
	phasesByID := make(map[string]WorkflowPhase, len(def.Phases))
	for _, p := range def.Phases {
		phasesByID[p.ID] = p
	}

	// Spot-check a few Spanish display names from the spec (full titles).
	checks := []struct {
		id   string
		name string
	}{
		{"global-reading", "Lectura global y comprensión"},
		{"initial-diagnosis", "Diagnóstico inicial (fortalezas y debilidades)"},
		{"reviewer-simulation", "Simulación de reviewers (Reviewer 1/2/3)"},
		{"submission-preparation", "Preparación de envío (checklist de conferencia)"},
	}
	for _, c := range checks {
		p := phasesByID[c.id]
		if p.Name != c.name {
			t.Errorf("phase %q: Name = %q, want %q", c.id, p.Name, c.name)
		}
	}
}

// ─── Existing test ─────────────────────────────────────────────────────────

func TestTemplateValidatesCleanly(t *testing.T) {
	// Belt-and-suspenders: the template should also pass Validate(). This
	// guards against future edits to the template introducing typos that
	// validate.go would catch. We seed the academic-researcher and
	// latex-formatting skills in a temp dir because they are not
	// embedded; they are domain-specific and only meaningful to users
	// who adopt the workflow.
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"academic-researcher", "latex-formatting"} {
		skillPath := filepath.Join(skillDir, name)
		if err := os.MkdirAll(skillPath, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\nbody\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	def, _ := Template("paper-review")
	result := Validate(&def, ValidationOptions{ExtraSkillDirs: []string{skillDir}})
	if result.HasErrors() {
		t.Errorf("Validate(paper-review template) unexpected errors: %+v", result.Issues)
	}
}
