package sddstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

func TestResolvePreservesNormalizedWorkspace(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes"))
	t.Chdir(root)
	status, err := Resolve(ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if status.ActionContext.WorkspaceRoot != root {
		t.Fatalf("status workspace = %q, want %q", status.ActionContext.WorkspaceRoot, root)
	}
}

func TestResolveUsesEngramArtifactsWhenOpenSpecIsAbsent(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".engram"))
	runRuntimeLedgerGit(t, root, "init", "-q")
	runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")

	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/add-auth/proposal", Content: "## Proposal\nAdd auth", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/add-auth/spec", Content: "## Requirements\n- SHALL work", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/add-auth/design", Content: "## Design\nUse middleware", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/add-auth/tasks", Content: "- [ ] 1.1 Wire routes\n", Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	status, err := Resolve(ResolveOptions{CWD: root, IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if status.ArtifactStore != ArtifactStoreEngram {
		t.Fatalf("ArtifactStore = %q, want %q", status.ArtifactStore, ArtifactStoreEngram)
	}
	if status.ChangeName == nil || *status.ChangeName != "add-auth" {
		t.Fatalf("ChangeName = %v, want add-auth", ptrValue(status.ChangeName))
	}
	if status.Dependencies.Apply != DependencyReady || status.NextRecommended != "apply" {
		t.Fatalf("apply dependency = %q next = %q, want ready/apply", status.Dependencies.Apply, status.NextRecommended)
	}
	if status.TaskProgress != (TaskProgress{Total: 1, Pending: 1, AllComplete: false}) {
		t.Fatalf("TaskProgress = %#v", status.TaskProgress)
	}
	if got := firstPath(status.ArtifactPaths.Tasks); got != "sdd/add-auth/tasks" {
		t.Fatalf("ArtifactPaths.Tasks[0] = %q, want topic key", got)
	}
	if status.PhaseInstructions == nil {
		t.Fatal("PhaseInstructions is nil")
	}
}

func TestResolveSelectionStates(t *testing.T) {
	tests := []struct {
		name          string
		seed          func(t *testing.T, root string)
		changeName    string
		wantChange    *string
		wantNext      string
		wantBlockedRx string
	}{
		{
			name:          "no active change blocks",
			seed:          func(t *testing.T, root string) { mkdir(t, filepath.Join(root, "openspec", "changes")) },
			wantNext:      "sdd-new",
			wantBlockedRx: "No active OpenSpec changes",
		},
		{
			name: "ambiguous active changes block",
			seed: func(t *testing.T, root string) {
				mkdir(t, filepath.Join(root, "openspec", "changes", "first"))
				mkdir(t, filepath.Join(root, "openspec", "changes", "second"))
			},
			wantNext:      "select-change",
			wantBlockedRx: "ambiguous: first, second",
		},
		{
			name: "explicit missing change blocks",
			seed: func(t *testing.T, root string) {
				mkdir(t, filepath.Join(root, "openspec", "changes", "real"))
			},
			changeName:    "missing",
			wantChange:    strPtr("missing"),
			wantNext:      "sdd-new",
			wantBlockedRx: "not found: missing",
		},
		{
			name: "single active change is inferred",
			seed: func(t *testing.T, root string) {
				seedReadyChange(t, root, "add-auth", "- [ ] 1.1 Wire routes\n")
			},
			wantChange: strPtr("add-auth"),
			wantNext:   "apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.seed(t, root)

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: tt.changeName})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if !equalStringPtr(status.ChangeName, tt.wantChange) {
				t.Fatalf("ChangeName = %v, want %v", ptrValue(status.ChangeName), ptrValue(tt.wantChange))
			}
			if status.NextRecommended != tt.wantNext {
				t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.wantNext)
			}
			if tt.wantBlockedRx != "" && !strings.Contains(strings.Join(status.BlockedReasons, "\n"), tt.wantBlockedRx) {
				t.Fatalf("BlockedReasons = %v, want containing %q", status.BlockedReasons, tt.wantBlockedRx)
			}
		})
	}
}

// TestAmbiguousChangeSelectionNamesARunnableCommandPerChange pins the machine
// surface, not the markdown one. #2117 step 5: the SDD task-failure envelope
// hands the caller `gentle-ai sdd-status --cwd <cwd> --json` as its
// continuation. With more than one active change that lands here, and the
// blocked reason used to be the entire guidance: it listed the change names and
// named no command, so an automated consumer following our own continuation had
// nowhere to go. RenderDispatcherMarkdown already spelled the commands out, but
// --json never reaches it.
func TestAmbiguousChangeSelectionNamesARunnableCommandPerChange(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "first"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "second"))

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "select-change" {
		t.Fatalf("NextRecommended = %q, want select-change", status.NextRecommended)
	}

	reasons := strings.Join(status.BlockedReasons, "\n")
	for _, change := range []string{"first", "second"} {
		// The selector is positional: ParseCommandArgs has no --change flag
		// (#3278, #2790), so this is the only runnable spelling.
		want := "gentle-ai sdd-status " + change + " --cwd " + root
		if !strings.Contains(reasons, want) {
			t.Fatalf("blocked reasons named no runnable command for %q; a refusal that lists options and no command is the shape this project does not ship.\ngot:\n%s", change, reasons)
		}
	}
}

func TestDispatcherMarkdownRendersSelectChangeInstructions(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "first"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "second"))

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "select-change" {
		t.Fatalf("NextRecommended = %q, want select-change", status.NextRecommended)
	}

	// next_recommended == "select-change" is a routing state, not a Phase, so
	// nextRecommendedPhase() does not recognize it. Without an explicit
	// continuation, the blocked reason ("Change selection is ambiguous: ...")
	// is the entire guidance and names no way out.
	dispatcher := RenderDispatcherMarkdown(status)
	for _, want := range []string{"### Next Selection Operation", "gentle-ai sdd-status --cwd", "gentle-ai sdd-continue --cwd", "<change-name>"} {
		if !strings.Contains(dispatcher, want) {
			t.Fatalf("dispatcher missing %q for select-change:\n%s", want, dispatcher)
		}
	}
}

func TestResolveBlockedStatusJSONUsesEmptyArraysForPathFields(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes"))

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	for _, section := range []string{"artifactPaths", "contextFiles"} {
		var paths map[string]json.RawMessage
		if err := json.Unmarshal(decoded[section], &paths); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", section, err)
		}
		for _, field := range []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"} {
			if got := string(paths[field]); got != "[]" {
				t.Fatalf("%s.%s JSON = %s, want [] in %s", section, field, got, payload)
			}
		}
	}
}

func TestResolveStatusJSONUsesEmptyBlockedReasonsArray(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "add-auth", "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := string(decoded["blockedReasons"]); got != "[]" {
		t.Fatalf("blockedReasons JSON = %s, want [] in %s", got, payload)
	}
}

func TestBlockerReasonsForRoute(t *testing.T) {
	expected := []string{
		"proposal.md is missing or partial.",
		"openspec/changes/thin/specs/<domain>/spec.md is missing or partial.",
		"design.md is missing or partial.",
		"tasks.md is missing or partial.",
	}
	genuine := []string{
		"tasks.md has no markdown task checkboxes.",
		"proposal.md is missing or partial.",
	}
	reasons := blockerReasons{expectedPlanning: expected, genuine: genuine}

	tests := []struct {
		name  string
		route string
		want  []string
	}{
		{name: "propose omits all expected planning blockers", route: "propose", want: genuine},
		{name: "spec omits expected planning blockers", route: "spec", want: genuine},
		{name: "design omits expected planning blockers", route: "design", want: genuine},
		{name: "tasks omits expected planning blockers", route: "tasks", want: genuine},
		{name: "apply preserves expected then genuine order", route: "apply", want: append(append([]string{}, expected...), genuine...)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reasons.forRoute(tt.route)
			if got == nil {
				t.Fatal("forRoute() returned a nil slice")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("forRoute(%q) = %v, want %v", tt.route, got, tt.want)
			}
		})
	}

	got := reasons.forRoute("propose")
	got[0] = "mutated"
	if next := reasons.forRoute("propose"); !reflect.DeepEqual(next, genuine) {
		t.Fatalf("forRoute() reused its backing slice: next = %v, want %v", next, genuine)
	}
}

func TestResolvePlanningRoutesOmitExpectedBlockersForBothStores(t *testing.T) {
	routes := []struct {
		name  string
		route string
	}{
		{name: "propose", route: "propose"},
		{name: "spec", route: "spec"},
		{name: "design", route: "design"},
		{name: "tasks", route: "tasks"},
	}

	for _, store := range []string{"openspec", "engram"} {
		for _, tt := range routes {
			t.Run(store+"/"+tt.name, func(t *testing.T) {
				root := t.TempDir()
				if store == "openspec" {
					seedPlanningRoute(t, root, "thin", tt.route)
				} else {
					mkdir(t, filepath.Join(root, ".engram"))
					runRuntimeLedgerGit(t, root, "init", "-q")
					runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
					restore := stubEngramExport(t, engramPlanningRoute("thin", tt.route))
					t.Cleanup(restore)
				}

				status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
				if err != nil {
					t.Fatalf("Resolve() error = %v", err)
				}
				if status.NextRecommended != tt.route {
					t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.route)
				}
				if want := []string{}; !reflect.DeepEqual(status.BlockedReasons, want) {
					t.Fatalf("BlockedReasons = %v, want %v", status.BlockedReasons, want)
				}
			})
		}
	}
}

func TestResolveMissingProposalStillRecommendsPropose(t *testing.T) {
	root := t.TempDir()
	seedPlanningRoute(t, root, "legacy-change", "propose")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "legacy-change"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "propose" {
		t.Fatalf("NextRecommended = %q, want propose", status.NextRecommended)
	}
	if status.Artifacts["proposal"] != ArtifactMissing {
		t.Fatalf("proposal state = %q, want missing", status.Artifacts["proposal"])
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want no planning-route blockers", status.BlockedReasons)
	}
}

func TestResolveTaskIntegrityBlockerSurvivesPlanningAndResolveBlockersRoutes(t *testing.T) {
	tests := []struct {
		name     string
		seed     func(t *testing.T, root string)
		wantNext string
	}{
		{
			name: "planning route",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "not a checklist\n")
			},
			wantNext: "propose",
		},
		{
			name: "all planning artifacts complete",
			seed: func(t *testing.T, root string) {
				seedReadyChange(t, root, "thin", "not a checklist\n")
			},
			wantNext: "resolve-blockers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.seed(t, root)

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if status.NextRecommended != tt.wantNext {
				t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.wantNext)
			}
			want := []string{"tasks.md has no markdown task checkboxes."}
			if !reflect.DeepEqual(status.BlockedReasons, want) {
				t.Fatalf("BlockedReasons = %v, want %v", status.BlockedReasons, want)
			}
		})
	}
}

func TestResolveEngramPlanningRouteRetainsGenuineBlocker(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".engram"))
	runRuntimeLedgerGit(t, root, "init", "-q")
	runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/thin/tasks", Content: "not a checklist\n", Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "propose" {
		t.Fatalf("NextRecommended = %q, want propose", status.NextRecommended)
	}
	want := []string{"tasks.md has no markdown task checkboxes."}
	if !reflect.DeepEqual(status.BlockedReasons, want) {
		t.Fatalf("BlockedReasons = %v, want %v", status.BlockedReasons, want)
	}
}

func TestResolveArtifactStatesAndTaskProgress(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "add-auth", strings.Join([]string{
		"# Tasks",
		"",
		"- [x] 1.1 Build foundation",
		"- [X] 1.2 Add API",
		"- [ ] 1.3 Wire routes",
		"plain [ ] note is ignored",
		"",
	}, "\n"))
	write(t, filepath.Join(changeRoot, "apply-progress.md"), "# Apply\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "add-auth"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	assertArtifact(t, status, "proposal", ArtifactDone)
	assertArtifact(t, status, "specs", ArtifactDone)
	assertArtifact(t, status, "design", ArtifactDone)
	assertArtifact(t, status, "tasks", ArtifactDone)
	assertArtifact(t, status, "applyProgress", ArtifactDone)
	assertArtifact(t, status, "verifyReport", ArtifactMissing)
	if status.TaskProgress != (TaskProgress{Total: 3, Completed: 2, Pending: 1, AllComplete: false}) {
		t.Fatalf("TaskProgress = %#v", status.TaskProgress)
	}
	if status.Dependencies.Verify != DependencyReady {
		t.Fatalf("Verify dependency = %q, want %q for optional partial diagnostics", status.Dependencies.Verify, DependencyReady)
	}
}

// #3311: status built Relationships.DependsOn as an unconditional empty list
// and never read openspec/changes/<change>/state.yaml, the file the OpenSpec
// convention names as the change's DAG state.
func TestResolveProjectsOpenSpecStateDependsOn(t *testing.T) {
	tests := []struct {
		name  string
		state string
		want  []string
	}{
		{"flow list", "dependsOn: [parent-change]\n", []string{"parent-change"}},
		{"block list", "phase: apply\ndependsOn:\n  - parent-change\n  - other-change\n", []string{"parent-change", "other-change"}},
		{"no file", "", []string{}},
		{"missing key", "phase: apply\n", []string{}},
		{"malformed", "dependsOn: [unclosed\n", []string{}},
		{"scalar instead of list", "dependsOn: parent-change\n", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedReadyChange(t, root, "add-auth", "- [ ] 1.1 Work\n")
			if tt.state != "" {
				write(t, filepath.Join(changeRoot, "state.yaml"), tt.state)
			}
			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "add-auth"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if status.Relationships.DependsOn == nil || !reflect.DeepEqual(status.Relationships.DependsOn, tt.want) {
				t.Fatalf("Relationships.DependsOn = %#v, want %#v", status.Relationships.DependsOn, tt.want)
			}
			if status.NextRecommended != "apply" {
				t.Fatalf("NextRecommended = %q, want apply: state.yaml must never block status", status.NextRecommended)
			}
		})
	}
}

func TestResolveNextRecommendedPlanningRouting(t *testing.T) {
	tests := []struct {
		name     string
		seed     func(t *testing.T, root string)
		wantNext string
	}{
		{
			name: "no artifacts routes to propose",
			seed: func(t *testing.T, root string) {
				mkdir(t, filepath.Join(root, "openspec", "changes", "thin"))
			},
			wantNext: "propose",
		},
		{
			name: "proposal only routes to spec",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "proposal.md"), "# Proposal\n")
			},
			wantNext: "spec",
		},
		{
			name: "proposal and specs but no design routes to design",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "proposal.md"), "# Proposal\n")
				write(t, filepath.Join(root, "openspec", "changes", "thin", "specs", "core", "spec.md"), "# Spec\n")
			},
			wantNext: "design",
		},
		{
			name: "proposal specs and design but no tasks routes to tasks",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "proposal.md"), "# Proposal\n")
				write(t, filepath.Join(root, "openspec", "changes", "thin", "specs", "core", "spec.md"), "# Spec\n")
				write(t, filepath.Join(root, "openspec", "changes", "thin", "design.md"), "# Design\n")
			},
			wantNext: "tasks",
		},
		{
			name: "all planning done with pending tasks routes to apply",
			seed: func(t *testing.T, root string) {
				seedReadyChange(t, root, "thin", "- [ ] 1.1 Work\n")
			},
			wantNext: "apply",
		},
		{
			name: "tasks only (no proposal) routes to propose not resolve-blockers",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")
			},
			wantNext: "propose",
		},
		{
			name: "design only (no proposal or specs) routes to propose",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "design.md"), "# Design\n")
			},
			wantNext: "propose",
		},
		{
			name: "proposal and design but no specs routes to spec",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "proposal.md"), "# Proposal\n")
				write(t, filepath.Join(root, "openspec", "changes", "thin", "design.md"), "# Design\n")
			},
			wantNext: "spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.seed(t, root)

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if status.NextRecommended != tt.wantNext {
				t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.wantNext)
			}
		})
	}
}

func TestResolveNextRecommendedUsesStableTokenForCoreArtifactBlockers(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Missing proposal routes to "propose", not "resolve-blockers".
	// Blocked prose must live in blockedReasons, never in nextRecommended.
	blockedProse := "proposal.md is missing or partial."
	if status.NextRecommended != "propose" {
		t.Fatalf("NextRecommended = %q, want propose", status.NextRecommended)
	}
	if status.NextRecommended == blockedProse || strings.Contains(status.NextRecommended, blockedProse) {
		t.Fatalf("NextRecommended = %q, must not contain blocked reason prose %q", status.NextRecommended, blockedProse)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("BlockedReasons = %v, want no expected planning blockers", status.BlockedReasons)
	}
}

func TestResolveIncludesInstructionsWhenRequested(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "add-auth", "- [x] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root, IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if status.PhaseInstructions == nil {
		t.Fatal("PhaseInstructions is nil")
	}
	// #3814: the archive gate names the resolved verify-report locator and the
	// active store's read verb instead of a hardcoded OpenSpec filename, so the
	// same instruction is correct under every artifact store.
	archive := strings.Join(status.PhaseInstructions.Archive, "\n")
	if !strings.Contains(archive, "Verify-report locator:") {
		t.Fatalf("Archive instructions omit the verify-report locator = %v", status.PhaseInstructions.Archive)
	}
	if !strings.Contains(archive, "neither a report nor task completion is an admission requirement") {
		t.Fatalf("Archive instructions omit optional verification semantics = %v", status.PhaseInstructions.Archive)
	}
}

func TestResolveRejectsNonexistentCWD(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	if _, err := Resolve(ResolveOptions{CWD: root}); err == nil {
		t.Fatal("Resolve() expected error for nonexistent cwd")
	}
}

func TestResolveExistingCWDWithoutOpenSpecChangesBlocks(t *testing.T) {
	root := t.TempDir()

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if status.NextRecommended != "sdd-new" {
		t.Fatalf("NextRecommended = %q, want sdd-new", status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "No active OpenSpec changes") {
		t.Fatalf("BlockedReasons = %v, want no active change block", status.BlockedReasons)
	}
}

func TestRenderMarkdownIncludesFencedJSON(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "add-auth", "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	markdown := RenderMarkdown(status)

	for _, want := range []string{
		"## SDD Status: add-auth",
		"next: apply",
		"```json",
		`"schemaName": "gentle-ai.sdd-status"`,
		"```",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("RenderMarkdown() missing %q:\n%s", want, markdown)
		}
	}
}

func TestRenderDispatcherMarkdownIncludesRoutingContext(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "add-auth", "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root, IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	markdown := RenderDispatcherMarkdown(status)

	for _, want := range []string{
		"## Native SDD Dispatcher: add-auth",
		"next_recommended: apply",
		"### Dependency States",
		"### Next Phase Instructions: apply",
		"Read proposal, specs, design, and tasks before editing.",
		"```json",
		`"schemaName": "gentle-ai.sdd-status"`,
		"```",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("RenderDispatcherMarkdown() missing %q:\n%s", want, markdown)
		}
	}
}

func TestRenderDispatcherMarkdownIncludesBlockedReasonsSeparately(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	markdown := RenderDispatcherMarkdown(status)

	for _, want := range []string{
		"next_recommended: propose",
		`"nextRecommended": "propose"`,
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("RenderDispatcherMarkdown() missing %q:\n%s", want, markdown)
		}
	}
	for _, unwanted := range []string{"### Blocked Reasons", "proposal.md is missing or partial."} {
		if strings.Contains(markdown, unwanted) {
			t.Fatalf("RenderDispatcherMarkdown() unexpectedly included %q:\n%s", unwanted, markdown)
		}
	}
}

func TestRenderNativePhasePromptIncludesAuthorityInstructionsJSONAndBlockedGuidance(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	prompt := RenderNativePhasePrompt(status, PhaseApply)

	for _, want := range []string{
		"## Native SDD Phase Prompt: apply",
		"Native status is authoritative over prompt inference.",
		"If this phase is blocked, return the blockers instead of acting.",
		"dependency_state: blocked",
		"Read proposal, specs, design, and tasks before editing.",
		"```json",
		`"schemaName": "gentle-ai.sdd-status"`,
		"```",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("RenderNativePhasePrompt() missing %q:\n%s", want, prompt)
		}
	}
	for _, unwanted := range []string{"### Blocked Reasons", "proposal.md is missing or partial."} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("RenderNativePhasePrompt() unexpectedly included %q:\n%s", unwanted, prompt)
		}
	}
}

func TestPlanningRouteRenderersOmitExpectedBlockersAndRetainGenuineBlockers(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "not a checklist\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "propose" {
		t.Fatalf("NextRecommended = %q, want propose", status.NextRecommended)
	}
	for name, rendered := range map[string]string{
		"status markdown":     RenderMarkdown(status),
		"dispatcher markdown": RenderDispatcherMarkdown(status),
		"native phase prompt": RenderNativePhasePrompt(status, PhasePropose),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(rendered, "tasks.md has no markdown task checkboxes.") {
				t.Fatalf("rendered output did not retain the genuine blocker:\n%s", rendered)
			}
			if strings.Contains(rendered, "proposal.md is missing or partial.") {
				t.Fatalf("rendered output retained an expected planning blocker:\n%s", rendered)
			}
		})
	}
}

func TestParseCommandArgs(t *testing.T) {
	got, err := ParseCommandArgs([]string{"add-auth", "--json", "--instructions", "--cwd", "/tmp/repo"})
	if err != nil {
		t.Fatalf("ParseCommandArgs() error = %v", err)
	}
	want := CommandArgs{ChangeName: "add-auth", CWD: "/tmp/repo", JSON: true, IncludeInstructions: true, Contract: "gentle-ai.sdd-status/v2"}
	if got != want {
		t.Fatalf("ParseCommandArgs() = %#v, want %#v", got, want)
	}
}

func TestParseCommandArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing cwd value", args: []string{"--cwd"}},
		{name: "cwd followed by json flag", args: []string{"--cwd", "--json"}},
		{name: "cwd followed by instructions flag", args: []string{"--cwd", "--instructions"}},
		{name: "unknown flag", args: []string{"--bogus"}},
		{name: "extra positional", args: []string{"first", "second"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCommandArgs(tt.args); err == nil {
				t.Fatalf("ParseCommandArgs(%v) expected error", tt.args)
			}
		})
	}
}

func seedPlanningRoute(t *testing.T, root string, name string, route string) {
	t.Helper()
	changeRoot := filepath.Join(root, "openspec", "changes", name)
	if route != "propose" {
		write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	}
	if route == "design" || route == "tasks" {
		write(t, filepath.Join(changeRoot, "specs", "core", "spec.md"), "# Spec\n")
	}
	if route == "tasks" {
		write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	}
	if route == "propose" {
		write(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")
	}
}

func assertArtifact(t *testing.T, status Status, key string, want ArtifactState) {
	t.Helper()
	if status.Artifacts[key] != want {
		t.Fatalf("Artifacts[%q] = %q, want %q", key, status.Artifacts[key], want)
	}
}

func strPtr(value string) *string {
	return &value
}

func equalStringPtr(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// #2212: a flat openspec/changes/<change>/specs/<cap>.md layout reports
// partial and routes to spec, but the spec route drops expected planning
// blockers, so the reporter saw an empty blockedReasons forever. The layout
// guidance is a genuine reason so it survives the spec route.
func TestResolveFlatSpecsLayoutNamesNestedLayoutOnSpecRoute(t *testing.T) {
	root := t.TempDir()
	changeRoot := filepath.Join(root, "openspec", "changes", "flat-specs")
	write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "specs", "example.md"), "# Spec\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "flat-specs"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertArtifact(t, status, "specs", ArtifactPartial)
	if status.NextRecommended != "spec" {
		t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "spec")
	}
	want := "openspec/changes/flat-specs/specs/ has files but no non-empty <domain>/spec.md; the spec phase writes every capability (new ones as full specs) at openspec/changes/flat-specs/specs/<domain>/spec.md, and sdd-archive promotes new ones to openspec/specs/<domain>/spec.md"
	if !reflect.DeepEqual(status.BlockedReasons, []string{want}) {
		t.Fatalf("BlockedReasons = %v, want [%q]", status.BlockedReasons, want)
	}
}

func TestResolveNestedSpecsLayoutReportsDoneWithoutLayoutGuidance(t *testing.T) {
	root := t.TempDir()
	changeRoot := filepath.Join(root, "openspec", "changes", "nested-specs")
	write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "specs", "example", "spec.md"), "# Spec\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "nested-specs"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertArtifact(t, status, "specs", ArtifactDone)
	if status.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, "apply")
	}
	for _, reason := range status.BlockedReasons {
		if strings.Contains(reason, "<domain>/spec.md") {
			t.Fatalf("BlockedReasons carries layout guidance for a nested layout: %v", status.BlockedReasons)
		}
	}
}

func TestArtifactBlockedReasonsNamesChangeLocalSpecPath(t *testing.T) {
	artifacts := map[string]ArtifactState{"proposal": ArtifactDone, "specs": ArtifactMissing, "design": ArtifactDone, "tasks": ArtifactDone}
	reasons := artifactBlockedReasons(artifacts, TaskProgress{Total: 1, Pending: 1}, "thin", nil)
	want := []string{"openspec/changes/thin/specs/<domain>/spec.md is missing or partial."}
	if !reflect.DeepEqual(reasons.expectedPlanning, want) {
		t.Fatalf("expectedPlanning = %v, want %v", reasons.expectedPlanning, want)
	}
	if len(reasons.genuine) != 0 {
		t.Fatalf("genuine = %v, want none for a missing specs directory", reasons.genuine)
	}

	reasons = artifactBlockedReasons(artifacts, TaskProgress{Total: 1, Pending: 1}, "", nil)
	if want := []string{"openspec/changes/<change>/specs/<domain>/spec.md is missing or partial."}; !reflect.DeepEqual(reasons.expectedPlanning, want) {
		t.Fatalf("expectedPlanning without a change name = %v, want %v", reasons.expectedPlanning, want)
	}
}

// An empty Engram spec artifact also reports partial, but the change-local
// filesystem layout guidance belongs to the OpenSpec store only.
func TestResolveEngramPartialSpecOmitsOpenSpecLayoutGuidance(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".engram"))
	runRuntimeLedgerGit(t, root, "init", "-q")
	runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/thin/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/thin/spec", Content: "   \n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/thin/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/thin/tasks", Content: "- [ ] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertArtifact(t, status, "specs", ArtifactPartial)
	if status.NextRecommended != "spec" {
		t.Fatalf("NextRecommended = %q, want spec", status.NextRecommended)
	}
	if want := []string{}; !reflect.DeepEqual(status.BlockedReasons, want) {
		t.Fatalf("BlockedReasons = %v, want %v", status.BlockedReasons, want)
	}
}

// --- INC-21 Phase 13 (P3c): governance wiring in the OpenSpec/hybrid
// resolver (design.md S4.3, D-05, D-09, D-13; T-10). Every test below is
// re-run, unmodified, by Phase 14: this section is the resolver-level proof
// that governance changes routing ONLY with a sealed, non-continuous
// kickoff, exactly as Phase 11's byte-for-byte control gate guards for the
// unsealed case.

// sealFullstackKickoff seals a single-role "fullstack" kickoff for
// changeRoot with the given execution style, reusing the same
// blocking/local/bug_only defaults sdd_kickoff.go's ToKickoff already
// applies for an explicit seal (args.go).
func sealFullstackKickoff(t *testing.T, changeRoot, changeName string, style kickoff.ExecutionStyle) {
	t.Helper()
	_, ok, err := kickoff.Seal(changeRoot, kickoff.Kickoff{
		Schema: kickoff.KickoffSchemaV1, Change: changeName, SealedBy: "test",
		Config: kickoff.FlowConfig{
			FlowMode: kickoff.FlowSDD, ExecutionStyle: style, ExecutionStyleSource: "explicit",
			HandoffPolicy: kickoff.HandoffNone,
			Roles:         []kickoff.KickoffRole{{Role: "fullstack", GatePolicy: "blocking", TasksFile: "tasks.md", VerifyFile: "verify-report.md"}},
		},
		Lifecycle: kickoff.Lifecycle{DeploymentTarget: "local", PostArchivePolicy: "bug_only"},
	})
	if err != nil || !ok {
		t.Fatalf("sealFullstackKickoff: kickoff.Seal() = (ok=%v, err=%v), want a successful preparation seal", ok, err)
	}
}

// TestResolveRoutesToAwaitGateWhenABlockReviewGateIsPending covers tasks.md
// 13.1's first two required cases together: a sealed, checkpointed change
// with every core artifact done and no gate decision recorded yet routes to
// "await-gate" (D-09) instead of "apply", and names the genuine
// pending-gate reason in BlockedReasons.
//
// Updated by Phase 21 (P6c): this test originally asserted
// dependencies.Archive == ready here, with a comment explicitly deferring
// "the full integration-gate condition" to Phase 21 — see git history for
// the original assertion. Now that archiveDependencyFromGovernance wires
// that condition in, a sealed change with no approved "integration" gate is
// correctly blocked from archiving regardless of what else is pending
// (REQ-21.16): this fixture has neither a favorable verify-report.md nor
// any integration-gate decision, so archive is doubly ungrounded here, not
// merely "not yet checked".
func TestResolveRoutesToAwaitGateWhenABlockReviewGateIsPending(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-pending"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "await-gate" {
		t.Fatalf("NextRecommended = %q, want await-gate", status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), `gate "spec"`) {
		t.Fatalf("BlockedReasons = %v, want a genuine reason naming the pending spec gate", status.BlockedReasons)
	}
	if status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("Dependencies.Archive = %q, want blocked: no integration gate is approved yet (REQ-21.16, Phase 21)", status.Dependencies.Archive)
	}
}

// TestResolveRoutesRejectedGateToOwningPhaseWithExactReason covers tasks.md
// 13.1's third required case: a rejected gate whose recorded digest still
// matches the current artifact routes back to that gate's owning phase
// (D-09) and surfaces the EXACT registered rejection reason, not a
// paraphrase of it.
func TestResolveRoutesRejectedGateToOwningPhaseWithExactReason(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-rejected"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	specPath := filepath.Join(changeRoot, "specs", "auth", "spec.md")
	digest, err := kickoff.ArtifactDigest([]string{specPath})
	if err != nil {
		t.Fatalf("kickoff.ArtifactDigest() error = %v", err)
	}
	const rejectionReason = "Falta cubrir el caso de sesion expirada"
	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.GateSpec, Decision: kickoff.DecisionRejected, Reason: rejectionReason,
		ArtifactDigest: digest, Actor: "reviewer",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "spec" {
		t.Fatalf("NextRecommended = %q, want spec (the rejected gate's owning phase)", status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), rejectionReason) {
		t.Fatalf("BlockedReasons = %v, want the exact registered rejection reason %q", status.BlockedReasons, rejectionReason)
	}
}

// TestResolveReturnsErrorForCorruptKickoffNeverSilentlyUnsealed is T-10's
// named case: an unreadable kickoff.yaml must fail the resolver with a
// named error, never degrade in silence to "no governance" (which would be
// indistinguishable from a change that never sealed one at all).
func TestResolveReturnsErrorForCorruptKickoffNeverSilentlyUnsealed(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-corrupt"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	writeFile(t, filepath.Join(changeRoot, kickoff.KickoffFileName), "{ not: valid: yaml")

	if _, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName}); err == nil {
		t.Fatal(`Resolve() = nil error, want a named error for a corrupt kickoff.yaml (T-10: never silently degrade to "unsealed")`)
	}
}

// TestResolveContinuousExecutionStyleNeverEvaluatesGates is D-05's
// end-to-end proof through the public resolver: a continuous-execution seal
// must never surface a Governance block, and routing stays exactly what it
// would be without any seal at all.
func TestResolveContinuousExecutionStyleNeverEvaluatesGates(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-continuous"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionContinuous)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Governance != nil {
		t.Fatalf("Status.Governance = %#v, want nil for a continuous-execution sealed change (D-05)", status.Governance)
	}
	if status.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply unchanged by an ungated continuous seal", status.NextRecommended)
	}
}

// --- INC-21 Phase 21 (P6c): archive precondition wired into
// dependencies.Archive (design.md S4.6, D-13; REQ-21.16). Every fixture
// below seals a checkpointed, single-role fullstack kickoff with every core
// artifact done, every task checked off, and a favorable local
// verify-report.md -- exactly the state
// TestResolveRoutesToAwaitGateWhenABlockReviewGateIsPending's own comment
// already promised dependencies.Archive would reach "exactly as today's
// logic computes it" before this phase wired the missing half in.

// writeFavorableVerifyReport seeds verify-report.md so machine.go's
// VerifyFound input opens the "integration" gate at all: EvaluateGates
// never surfaces it otherwise (machine.go, Phase 5). REQ-21.16 itself
// presupposes a favorable local verify report already exists before
// archive's own integration/deployment precondition is even asked about.
func writeFavorableVerifyReport(t *testing.T, changeRoot string) {
	t.Helper()
	write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify Report\n\nVerdict: pass\n")
}

// TestArchiveDependencyBlockedWithoutApprovedIntegrationGate is task 21.3's
// first case: a sealed, checkpointed change with every core artifact done,
// every task checked off, and a favorable local verify-report.md still
// reports dependencies.Archive blocked while no "integration" gate is
// approved -- naming the missing precondition and the exact invocation that
// satisfies it (REQ-21.16, second scenario).
func TestArchiveDependencyBlockedWithoutApprovedIntegrationGate(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-archive-pending"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	writeFavorableVerifyReport(t, changeRoot)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("Dependencies.Archive = %q, want blocked without an approved integration gate (REQ-21.16)", status.Dependencies.Archive)
	}
	joined := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(joined, `"integration"`) {
		t.Fatalf("BlockedReasons = %v, want a genuine reason naming the missing integration gate", status.BlockedReasons)
	}
	if !strings.Contains(joined, "axiom sdd gate record --gate integration --decision approved") {
		t.Fatalf("BlockedReasons = %v, want the exact executable invocation that satisfies the precondition", status.BlockedReasons)
	}
}

// TestArchiveDependencyReadyWithApprovedIntegrationGate is task 21.3's
// second case: once the "integration" gate is approved, dependencies.Archive
// becomes ready (REQ-21.16, first scenario).
func TestArchiveDependencyReadyWithApprovedIntegrationGate(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-archive-approved"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	writeFavorableVerifyReport(t, changeRoot)
	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.GateIntegration, Decision: kickoff.DecisionApproved,
		Reason: "PR #41 fusionado en main", EvidenceKind: kickoff.EvidencePRMerged,
		EvidenceRef: "abc123def", EvidenceBaseRef: "main", Verified: true, Actor: "maintainer",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Archive != DependencyReady {
		t.Fatalf("Dependencies.Archive = %q, want ready once the integration gate is approved", status.Dependencies.Archive)
	}
}

// TestArchiveDependencyUnsealedChangeUnaffectedByGovernance repeats Phase
// 11's byte-for-byte control gate against this specific new branch: an
// unsealed change keeps resolveDependencies' existing dependencies.Archive
// computation exactly as it was before this increment (D-13's own
// documented zero-cost boundary).
func TestArchiveDependencyUnsealedChangeUnaffectedByGovernance(t *testing.T) {
	root := t.TempDir()
	changeName := "unsealed-archive"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	writeFavorableVerifyReport(t, changeRoot)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Archive != DependencyReady {
		t.Fatalf("Dependencies.Archive = %q, want ready: an unsealed change must keep resolveDependencies' existing behavior byte for byte (D-13)", status.Dependencies.Archive)
	}
}

// TestArchivePreconditionIntegrationEndToEnd is task 21.5's integration
// test: over one full OpenSpec tree, resolving status is read-only either
// way, so a change with no approved "integration" gate stays exactly where
// it was -- never moved under openspec/changes/archive/, and
// openspec/specs/ is never touched -- while dependencies.Archive reports
// blocked (REQ-21.16, second scenario); once the gate is approved, the same
// tree, still untouched by Resolve itself, projects dependencies.Archive
// ready (REQ-21.16, first scenario).
func TestArchivePreconditionIntegrationEndToEnd(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-archive-e2e"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	writeFavorableVerifyReport(t, changeRoot)
	archivedRoot := filepath.Join(root, "openspec", "changes", "archive", changeName)
	liveSpecsRoot := filepath.Join(root, "openspec", "specs")

	before, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if before.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("Dependencies.Archive (before) = %q, want blocked without integration evidence", before.Dependencies.Archive)
	}
	if _, statErr := os.Stat(changeRoot); statErr != nil {
		t.Fatalf("change root %q no longer exists after a read-only status resolve: %v", changeRoot, statErr)
	}
	if _, statErr := os.Stat(archivedRoot); !os.IsNotExist(statErr) {
		t.Fatalf("change unexpectedly present under openspec/changes/archive/ before archive ran: stat error = %v", statErr)
	}
	if _, statErr := os.Stat(liveSpecsRoot); !os.IsNotExist(statErr) {
		t.Fatalf("openspec/specs/ unexpectedly exists before archive ran: stat error = %v", statErr)
	}

	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.GateIntegration, Decision: kickoff.DecisionApproved,
		Reason: "PR #41 fusionado en main", EvidenceKind: kickoff.EvidencePRMerged,
		EvidenceRef: "abc123def", EvidenceBaseRef: "main", Verified: true, Actor: "maintainer",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}

	after, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if after.Dependencies.Archive != DependencyReady {
		t.Fatalf("Dependencies.Archive (after) = %q, want ready once integration evidence is approved", after.Dependencies.Archive)
	}
}
