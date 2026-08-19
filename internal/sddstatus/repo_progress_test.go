package sddstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func TestDeclaredRepoSlugsParsesMultipleDistinctRepositories(t *testing.T) {
	tasks := strings.Join([]string{
		"- [ ] 4a.1 repository: gentle-ai | depends_on: none | goal: create foo",
		"- [ ] 4a.2 repository: gentle-ai | depends_on: 4a.1 | goal: test foo",
		"- [ ] 4b.1 repository: SmartClic/MSPagos | depends_on: 4a.1 | goal: wire bar",
		"",
	}, "\n")

	got := declaredRepoSlugs(tasks)
	want := []string{"gentle-ai", "smartclic-mspagos"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declaredRepoSlugs() = %v, want %v", got, want)
	}
}

func TestDeclaredRepoSlugsSingleRepository(t *testing.T) {
	tasks := "- [ ] 1.1 repository: gentle-ai | depends_on: none | goal: work\n"
	got := declaredRepoSlugs(tasks)
	if !reflect.DeepEqual(got, []string{"gentle-ai"}) {
		t.Fatalf("declaredRepoSlugs() = %v, want [gentle-ai]", got)
	}
}

func TestDeclaredRepoSlugsNoRepositoryFieldReturnsNil(t *testing.T) {
	tasks := "- [ ] 1.1 Update `../service-a/internal/api/handler.go` to accept the new header\n"
	if got := declaredRepoSlugs(tasks); got != nil {
		t.Fatalf("declaredRepoSlugs() = %v, want nil", got)
	}
	if got := declaredRepoSlugs(""); got != nil {
		t.Fatalf("declaredRepoSlugs(\"\") = %v, want nil", got)
	}
}

func TestDeclaredRepoSlugsMalformedLineIsSkippedNotFatal(t *testing.T) {
	tasks := strings.Join([]string{
		"- [ ] 1.1 this line has no repository field at all",
		"- [ ] 1.2 repository:    | depends_on: none | goal: empty value",
		"- [ ] 1.3 repository: gentle-ai | depends_on: none | goal: real one",
		"",
	}, "\n")
	got := declaredRepoSlugs(tasks)
	if !reflect.DeepEqual(got, []string{"gentle-ai"}) {
		t.Fatalf("declaredRepoSlugs() = %v, want [gentle-ai] (malformed/empty repository fields skipped)", got)
	}
}

func TestBuildRepoProgressNilForFewerThanTwoDeclaredSlugs(t *testing.T) {
	if got := buildRepoProgress(nil, nil); got != nil {
		t.Fatalf("buildRepoProgress(nil) = %v, want nil (SPEC-002)", got)
	}
	if got := buildRepoProgress([]string{"gentle-ai"}, map[string]ArtifactState{"gentle-ai": ArtifactDone}); got != nil {
		t.Fatalf("buildRepoProgress(1 slug) = %v, want nil (single-repo stays legacy)", got)
	}
}

func TestBuildRepoProgressMultiRepoPartialAndComplete(t *testing.T) {
	partial := buildRepoProgress([]string{"gentle-ai", "smartclic-mspagos"}, map[string]ArtifactState{
		"gentle-ai": ArtifactDone,
	})
	if partial == nil || partial.AllComplete {
		t.Fatalf("buildRepoProgress() = %+v, want non-nil with AllComplete=false", partial)
	}
	if len(partial.Repos) != 2 {
		t.Fatalf("Repos = %v, want 2 entries", partial.Repos)
	}
	if partial.Repos[1].ApplyProgress != ArtifactMissing {
		t.Fatalf("missing slug state = %q, want %q", partial.Repos[1].ApplyProgress, ArtifactMissing)
	}

	complete := buildRepoProgress([]string{"gentle-ai", "smartclic-mspagos"}, map[string]ArtifactState{
		"gentle-ai":         ArtifactDone,
		"smartclic-mspagos": ArtifactDone,
	})
	if complete == nil || !complete.AllComplete {
		t.Fatalf("buildRepoProgress() = %+v, want non-nil with AllComplete=true", complete)
	}
}

func TestApplyMultiRepoApplyGateNoopWhenNilOrComplete(t *testing.T) {
	dependencies := Dependencies{Verify: DependencyReady, Archive: DependencyReady}
	next := "verify"
	reasons := &blockerReasons{}
	applyMultiRepoApplyGate(&dependencies, &next, reasons, nil)
	if dependencies.Verify != DependencyReady || next != "verify" || len(reasons.genuine) != 0 {
		t.Fatalf("nil progress mutated state: dependencies=%+v next=%q reasons=%v", dependencies, next, reasons.genuine)
	}

	complete := &RepoProgress{AllComplete: true, Repos: []RepoProgressEntry{{Slug: "a", ApplyProgress: ArtifactDone}}}
	applyMultiRepoApplyGate(&dependencies, &next, reasons, complete)
	if dependencies.Verify != DependencyReady || next != "verify" || len(reasons.genuine) != 0 {
		t.Fatalf("complete progress mutated state: dependencies=%+v next=%q reasons=%v", dependencies, next, reasons.genuine)
	}
}

func TestApplyMultiRepoApplyGateBlocksVerifyAndArchiveWhenIncomplete(t *testing.T) {
	dependencies := Dependencies{Verify: DependencyReady, Archive: DependencyReady}
	next := "verify"
	reasons := &blockerReasons{}
	incomplete := &RepoProgress{
		AllComplete: false,
		Repos: []RepoProgressEntry{
			{Slug: "gentle-ai", ApplyProgress: ArtifactDone},
			{Slug: "smartclic-mspagos", ApplyProgress: ArtifactPartial},
		},
	}
	applyMultiRepoApplyGate(&dependencies, &next, reasons, incomplete)
	if dependencies.Verify != DependencyBlocked || dependencies.Archive != DependencyBlocked {
		t.Fatalf("dependencies = %+v, want Verify/Archive blocked", dependencies)
	}
	if next != "apply" {
		t.Fatalf("nextRecommended = %q, want apply", next)
	}
	if len(reasons.genuine) != 1 || !strings.Contains(reasons.genuine[0], "smartclic-mspagos") {
		t.Fatalf("reasons = %v, want the incomplete slug named", reasons.genuine)
	}
	if strings.Contains(reasons.genuine[0], "gentle-ai") {
		t.Fatalf("reasons = %v, must not name the already-complete slug", reasons.genuine)
	}
}

func TestApplyProgressStateBySlugFromFilesAndEngramObservations(t *testing.T) {
	dir := t.TempDir()
	donePath := writeTempFile(t, dir, "gentle-ai.md", "done work\n")
	partialPath := writeTempFile(t, dir, "smartclic-mspagos.md", "")

	states := applyProgressStateBySlug([]string{donePath, partialPath})
	if states["gentle-ai"] != ArtifactDone {
		t.Fatalf("gentle-ai state = %q, want done", states["gentle-ai"])
	}
	if states["smartclic-mspagos"] != ArtifactPartial {
		t.Fatalf("smartclic-mspagos state = %q, want partial", states["smartclic-mspagos"])
	}

	engramStates := engramApplyProgressStateBySlug(map[string]engramObservation{
		"apply-progress":                   {Title: "sdd/x/apply-progress", Content: "bare, single-repo"},
		"apply-progress/gentle-ai":         {Title: "sdd/x/apply-progress/gentle-ai", Content: "done"},
		"apply-progress/smartclic-mspagos": {Title: "sdd/x/apply-progress/smartclic-mspagos", Content: ""},
		"tasks":                            {Title: "sdd/x/tasks", Content: "irrelevant"},
	})
	if _, ok := engramStates["gentle-ai"]; !ok || engramStates["gentle-ai"] != ArtifactDone {
		t.Fatalf("engram states = %v, want gentle-ai=done", engramStates)
	}
	if engramStates["smartclic-mspagos"] != ArtifactPartial {
		t.Fatalf("engram states = %v, want smartclic-mspagos=partial", engramStates)
	}
	if _, ok := engramStates[""]; ok {
		t.Fatalf("bare apply-progress key must not be treated as a slug: %v", engramStates)
	}
}

// TestEngramTitlePatternMatchesBareAndScopedApplyProgress is the regression
// test for the F1/F2 regex fix: both the bare (single-repo, legacy) and
// scoped (multi-repo) apply-progress titles must match, and the match count
// must stay 3 for BOTH shapes so the `len(matches) != 3` guards at
// collectEngramChanges and engramArtifactsForChange never blind themselves.
func TestEngramTitlePatternMatchesBareAndScopedApplyProgress(t *testing.T) {
	tests := []struct {
		title    string
		wantType string
	}{
		{"sdd/demo/apply-progress", "apply-progress"},
		{"sdd/demo/apply-progress/gentle-ai", "apply-progress/gentle-ai"},
		{"sdd/demo/apply-progress/smartclic-mspagos", "apply-progress/smartclic-mspagos"},
	}
	for _, tt := range tests {
		matches := engramTitlePattern.FindStringSubmatch(tt.title)
		if len(matches) != 3 {
			t.Fatalf("engramTitlePattern.FindStringSubmatch(%q) len = %d, want 3 (matches=%v)", tt.title, len(matches), matches)
		}
		if matches[1] != "demo" {
			t.Fatalf("matches[1] = %q, want demo", matches[1])
		}
		if matches[2] != tt.wantType {
			t.Fatalf("matches[2] = %q, want %q", matches[2], tt.wantType)
		}
	}
}

func TestCollectEngramChangesRecognizesScopedApplyProgressAsExistenceEvidence(t *testing.T) {
	observations := []engramObservation{
		engramArtifact("demo", "sdd/multi-repo-change/proposal"),
		engramArtifact("demo", "sdd/multi-repo-change/apply-progress/gentle-ai"),
		engramArtifact("demo", "sdd/multi-repo-change/apply-progress/smartclic-mspagos"),
	}
	changes := collectEngramChanges(observations, "demo")
	found := false
	for _, c := range changes {
		if c == "multi-repo-change" {
			found = true
		}
	}
	if !found {
		t.Fatalf("collectEngramChanges() = %v, want multi-repo-change discovered via scoped apply-progress", changes)
	}
}

func TestEngramArtifactPathsListsOnePathPerRepoSlugSorted(t *testing.T) {
	artifacts := engramArtifactsForChange([]engramObservation{
		engramArtifact("demo", "sdd/multi/apply-progress/smartclic-mspagos"),
		engramArtifact("demo", "sdd/multi/apply-progress/gentle-ai"),
	}, "demo", "multi")
	paths := engramArtifactPaths("multi", artifacts)
	want := []string{
		"sdd/multi/apply-progress/gentle-ai",
		"sdd/multi/apply-progress/smartclic-mspagos",
	}
	if !reflect.DeepEqual(paths.ApplyProgress, want) {
		t.Fatalf("ApplyProgress = %v, want %v", paths.ApplyProgress, want)
	}
}

// TestProjectStatusV1RepoProgressAppearsOnlyWhenPopulated extends the
// wire-freeze coverage without touching
// TestProjectStatusV1FreezesExactLegacyShape itself: a nil RepoProgress (the
// legacy default that test already pins) produces no "repoProgress" key, and
// a populated one produces exactly that one new key alongside the frozen
// set.
func TestProjectStatusV1RepoProgressAppearsOnlyWhenPopulated(t *testing.T) {
	base := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)

	projectedNil, err := ProjectStatusV1(base)
	if err != nil {
		t.Fatalf("ProjectStatusV1() error = %v", err)
	}
	payloadNil, err := json.Marshal(projectedNil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payloadNil), "repoProgress") {
		t.Fatalf("nil RepoProgress leaked a repoProgress key: %s", payloadNil)
	}

	populated := base
	populated.RepoProgress = &RepoProgress{
		AllComplete: false,
		Repos: []RepoProgressEntry{
			{Slug: "gentle-ai", ApplyProgress: ArtifactDone},
			{Slug: "smartclic-mspagos", ApplyProgress: ArtifactMissing},
		},
	}
	projectedPopulated, err := ProjectStatusV1(populated)
	if err != nil {
		t.Fatalf("ProjectStatusV1() error = %v", err)
	}
	payload, err := json.Marshal(projectedPopulated)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	raw, ok := document["repoProgress"]
	if !ok {
		t.Fatalf("populated RepoProgress produced no repoProgress key: %s", payload)
	}
	var decoded RepoProgress
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("repoProgress does not decode: %v", err)
	}
	if decoded.AllComplete || len(decoded.Repos) != 2 {
		t.Fatalf("decoded RepoProgress = %+v, want the populated fixture", decoded)
	}

	wantRootKeys := []string{
		"schemaName", "schemaVersion", "changeName", "artifactStore", "planningHome", "changeRoot",
		"artifactPaths", "contextFiles", "artifacts", "taskProgress", "dependencies", "applyState",
		"actionContext", "relationships", "remediationState", "nextRecommended", "blockedReasons",
		"repoProgress",
	}
	assertExactJSONKeys(t, document, wantRootKeys)
}

// TestResolveSingleRepoWithRepositoryFieldStaysByteIdentical is the explicit
// 1-repo byte-identical regression the apply step for this slice requires:
// a tasks artifact using the `repository:` field convention (dev-task-planner
// contract), declaring exactly one repo-slug, must produce a Status with a
// nil RepoProgress and a v1 projection carrying no "repoProgress" key at all
// -- identical to a change that never declared any repository field.
func TestResolveSingleRepoWithRepositoryFieldStaysByteIdentical(t *testing.T) {
	withField := t.TempDir()
	seedReadyChange(t, withField, "single-repo-field", "- [ ] 1.1 repository: gentle-ai | depends_on: none | goal: work\n")
	withFieldStatus, err := Resolve(ResolveOptions{CWD: withField, ChangeName: "single-repo-field"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if withFieldStatus.RepoProgress != nil {
		t.Fatalf("RepoProgress = %+v, want nil for a single declared repo-slug", withFieldStatus.RepoProgress)
	}

	without := t.TempDir()
	seedReadyChange(t, without, "single-repo-field", "- [ ] 1.1 Wire routes\n")
	withoutStatus, err := Resolve(ResolveOptions{CWD: without, ChangeName: "single-repo-field"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	withFieldStatus.ChangeRoot = withoutStatus.ChangeRoot
	withFieldStatus.PlanningHome = withoutStatus.PlanningHome
	withFieldStatus.ActionContext = withoutStatus.ActionContext

	projectedWith, err := ProjectStatusV1(withFieldStatus)
	if err != nil {
		t.Fatal(err)
	}
	projectedWithout, err := ProjectStatusV1(withoutStatus)
	if err != nil {
		t.Fatal(err)
	}
	encodedWith, err := json.Marshal(projectedWith)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedWith), "repoProgress") {
		t.Fatalf("single-repo status leaked a repoProgress key: %s", encodedWith)
	}
	encodedWithout, err := json.Marshal(projectedWithout)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedWithout), "repoProgress") {
		t.Fatalf("baseline single-repo status unexpectedly carries repoProgress: %s", encodedWithout)
	}
}

// TestResolveMultiRepoApplyGateBlocksVerifyUntilEveryRepoComplete is the
// SPEC-001/SPEC-004 integration scenario: 2 declared repo-slugs, apply-progress
// present for only one, must block Verify/Archive and keep nextRecommended at
// apply; once both are done, Verify/Archive route normally again.
func TestResolveMultiRepoApplyGateBlocksVerifyUntilEveryRepoComplete(t *testing.T) {
	root := t.TempDir()
	tasks := strings.Join([]string{
		"- [x] 1.1 repository: gentle-ai | depends_on: none | goal: work | status: done",
		"- [x] 1.2 repository: smartclic-mspagos | depends_on: none | goal: work | status: done",
		"",
	}, "\n")
	changeRoot := seedReadyChange(t, root, "multi-repo-progress", tasks)

	// Partial: only one repo's apply-progress file exists.
	write(t, filepath.Join(changeRoot, "apply-progress", "gentle-ai.md"), "done\n")
	partial, err := Resolve(ResolveOptions{CWD: root, ChangeName: "multi-repo-progress"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if partial.RepoProgress == nil || partial.RepoProgress.AllComplete {
		t.Fatalf("RepoProgress = %+v, want non-nil AllComplete=false", partial.RepoProgress)
	}
	if partial.Dependencies.Verify != DependencyBlocked || partial.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("Dependencies = %+v, want Verify/Archive blocked while a repo-slug lags", partial.Dependencies)
	}
	if partial.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply", partial.NextRecommended)
	}

	// Complete: both repos' apply-progress files exist.
	write(t, filepath.Join(changeRoot, "apply-progress", "smartclic-mspagos.md"), "done\n")
	complete, err := Resolve(ResolveOptions{CWD: root, ChangeName: "multi-repo-progress"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if complete.RepoProgress == nil || !complete.RepoProgress.AllComplete {
		t.Fatalf("RepoProgress = %+v, want non-nil AllComplete=true", complete.RepoProgress)
	}
}
