package sddstatus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveSharesOneNormalizedWorkspaceWithReviewMode(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes"))
	t.Chdir(root)

	modeLookups := 0
	status, err := Resolve(ResolveOptions{
		ReviewDisabledForWorkspace: func(workspaceRoot string) bool {
			modeLookups++
			if workspaceRoot != root {
				t.Fatalf("review mode workspace = %q, want %q", workspaceRoot, root)
			}
			return true
		},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if modeLookups != 1 {
		t.Fatalf("review mode lookups = %d, want 1", modeLookups)
	}
	if status.ActionContext.WorkspaceRoot != root {
		t.Fatalf("status workspace = %q, want shared root %q", status.ActionContext.WorkspaceRoot, root)
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
		"specs/**/spec.md is missing or partial.",
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

func TestResolveRuntimeOverrideRestoresExpectedPlanningBlockersForBothStores(t *testing.T) {
	for _, store := range []string{"openspec", "engram"} {
		t.Run(store, func(t *testing.T) {
			root := initRuntimeLedgerRepo(t)
			if store == "openspec" {
				seedPlanningRoute(t, root, "thin", "propose")
			} else {
				mkdir(t, filepath.Join(root, ".engram"))
				runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
				restore := stubEngramExport(t, engramPlanningRoute("thin", "propose"))
				t.Cleanup(restore)
			}
			// A maintainer decision is the runtime state that still overrides
			// routing to a final route. An active attempt stopped overriding in
			// #2463: compact acquire admits its own token holder, so status has
			// no standing to refuse that launch.
			store := mustRuntimeStore(t, root, "thin")
			active, err := store.Begin(context.Background(), BeginAttemptRequest{
				ExpectedRevision: "", RequestID: "begin-thin", WorkUnit: "apply",
				EvidenceGoal: "prove final-route blocker filtering", MaxAttempts: 1, MaxChangedLines: 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Finish(context.Background(), FinishAttemptRequest{
				ExpectedRevision: active.Revision, RequestID: "finish-thin", Outcome: AttemptFailed,
				EvidenceRevision: runtimeTestHash('4'), Diagnosis: "bounded runtime reproduced the failure",
				HarnessDisposition: HarnessReused, CleanupEvidence: "runtime process group exited",
				ProcessEvidence: "post-run scan found no descendants",
			}); err != nil {
				t.Fatal(err)
			}

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if status.NextRecommended != "resolve-blockers" {
				t.Fatalf("NextRecommended = %q, want resolve-blockers", status.NextRecommended)
			}
			reasons := strings.Join(status.BlockedReasons, "\n")
			for _, want := range []string{"proposal.md is missing or partial.", "blocked(maintainer_decision)"} {
				if !strings.Contains(reasons, want) {
					t.Fatalf("BlockedReasons = %v, want containing %q", status.BlockedReasons, want)
				}
			}
		})
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
	if status.Dependencies.Verify != DependencyBlocked {
		t.Fatalf("Verify dependency = %q, want %q until all tasks complete", status.Dependencies.Verify, DependencyBlocked)
	}
}

func TestResolveApplyVerifyArchiveGates(t *testing.T) {
	tests := []struct {
		name              string
		seed              func(t *testing.T, root string)
		wantApply         ApplyState
		wantApplyD        DependencyState
		wantVerify        DependencyState
		wantArchive       DependencyState
		wantNext          string
		wantBlocked       string
		wantBlockedAbsent string
	}{
		{
			name: "apply blocked when core artifacts are missing routes to propose",
			seed: func(t *testing.T, root string) {
				write(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")
			},
			wantApply:         ApplyBlocked,
			wantApplyD:        DependencyBlocked,
			wantVerify:        DependencyBlocked,
			wantArchive:       DependencyBlocked,
			wantNext:          "propose",
			wantBlockedAbsent: "proposal.md is missing or partial.",
		},
		{
			name: "apply ready when core artifacts are done and tasks are pending",
			seed: func(t *testing.T, root string) {
				seedReadyChange(t, root, "thin", "- [ ] 1.1 Work\n")
			},
			wantApply:   ApplyReady,
			wantApplyD:  DependencyReady,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "apply",
		},
		{
			// Wave 4 S3 (design.md decision 3, proposal.md's #1 success
			// criterion): RDD no longer supervises SDD before verify. Apply
			// done, no verify report yet => verify is immediately ready, no
			// review transaction required. The offer point is post-verify.
			name: "apply all done makes verify ready immediately with no pre-verify review supervision",
			seed: func(t *testing.T, root string) {
				seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
		},
		{
			name: "apply progress does not make final verify ready before all tasks complete",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n- [ ] 1.2 Remaining\n")
				write(t, filepath.Join(changeRoot, "apply-progress.md"), "# Apply\n")
			},
			wantApply:   ApplyReady,
			wantApplyD:  DependencyReady,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "apply",
		},
		{
			name: "apply ready ignores stale bad verify report blockers while tasks are pending",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n- [ ] 1.2 Remaining\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nVerdict: PASS\nfailed: 1\n")
			},
			wantApply:         ApplyReady,
			wantApplyD:        DependencyReady,
			wantVerify:        DependencyBlocked,
			wantArchive:       DependencyBlocked,
			wantNext:          "apply",
			wantBlockedAbsent: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive ready only when verify report exists and tasks are complete",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("1"), "pass"))
				writeApprovedReviewArtifacts(t, changeRoot)
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyAllDone,
			wantArchive: DependencyReady,
			wantNext:    "archive",
		},
		{
			name: "archive ready for canonical passing verify report",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("1"), "pass")+"\n"+strings.Join([]string{
					"## Verification Report",
					"### Build & Tests Execution",
					"**Tests**: ✅ 12 passed / ❌ 0 failed / ⚠️ 0 skipped",
					"failed: 0",
					"### Issues Found",
					"**CRITICAL**: None",
					"No blockers",
					"### Verdict",
					"Verdict: PASS",
					"",
				}, "\n"))
				writeApprovedReviewArtifacts(t, changeRoot)
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyAllDone,
			wantArchive: DependencyReady,
			wantNext:    "archive",
		},
		{
			name: "archive ready for canonical pass with warnings verdict",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("1"), "pass_with_warnings")+"\n"+strings.Join([]string{
					"## Verification Report",
					"**Tests**: ✅ 12 passed / ❌ 0 failed / ⚠️ 1 skipped",
					"**CRITICAL**: None",
					"**WARNING**: flaky integration was skipped",
					"### Verdict",
					"PASS WITH WARNINGS",
					"",
				}, "\n"))
				writeApprovedReviewArtifacts(t, changeRoot)
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyAllDone,
			wantArchive: DependencyReady,
			wantNext:    "archive",
		},
		{
			name: "archive blocked when verify report has critical findings",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\ncritical: archive blocker\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when verify report has nonzero failed count",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nVerdict: PASS\nfailed: 1\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when canonical matrix has untested result despite pass verdict",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), strings.Join([]string{
					"## Verification Report",
					"### Spec Compliance Matrix",
					"| Requirement | Scenario | Test | Result |",
					"|-------------|----------|------|--------|",
					"| REQ-01 | Covers auth | (none found) | ❌ UNTESTED |",
					"### Verdict",
					"Verdict: PASS",
					"",
				}, "\n"))
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when canonical matrix has failing result despite pass verdict",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), strings.Join([]string{
					"## Verification Report",
					"### Spec Compliance Matrix",
					"| Requirement | Scenario | Test | Result |",
					"|-------------|----------|------|--------|",
					"| REQ-01 | Covers auth | `auth_test.go > TestAuth` | ❌ FAILING |",
					"### Verdict",
					"Verdict: PASS",
					"",
				}, "\n"))
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when verify report has blockers present",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nVerdict: PASS\nBlockers: missing evidence\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when verify report has todo pending and blockers",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nPASS\nTODO: finish audit\nPENDING: test run\nVerification blocker: missing evidence\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when verify report says status not passed",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nStatus: not passed\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when verify report says pass no",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nPASS: no\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive blocked when verify report has success and failure",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nStatus: SUCCESS\nFailure: build broke\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "resolve-review",
			wantBlocked: "bounded review transaction is missing",
		},
		{
			name: "archive ready when verify report has status pass",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("1"), "pass")+"\n# Verify\nStatus: PASS\n")
				writeApprovedReviewArtifacts(t, changeRoot)
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyAllDone,
			wantArchive: DependencyReady,
			wantNext:    "archive",
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

			if status.ApplyState != tt.wantApply {
				t.Fatalf("ApplyState = %q, want %q", status.ApplyState, tt.wantApply)
			}
			if status.Dependencies.Apply != tt.wantApplyD {
				t.Fatalf("Dependencies.Apply = %q, want %q", status.Dependencies.Apply, tt.wantApplyD)
			}
			if status.Dependencies.Verify != tt.wantVerify {
				t.Fatalf("Dependencies.Verify = %q, want %q", status.Dependencies.Verify, tt.wantVerify)
			}
			if status.Dependencies.Archive != tt.wantArchive {
				t.Fatalf("Dependencies.Archive = %q, want %q", status.Dependencies.Archive, tt.wantArchive)
			}
			if status.NextRecommended != tt.wantNext {
				t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.wantNext)
			}
			if tt.wantBlocked != "" && !strings.Contains(strings.Join(status.BlockedReasons, "\n"), tt.wantBlocked) {
				t.Fatalf("BlockedReasons = %v, want containing %q", status.BlockedReasons, tt.wantBlocked)
			}
			if tt.wantBlockedAbsent != "" && strings.Contains(strings.Join(status.BlockedReasons, "\n"), tt.wantBlockedAbsent) {
				t.Fatalf("BlockedReasons = %v, want not containing %q", status.BlockedReasons, tt.wantBlockedAbsent)
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
	if !strings.Contains(strings.Join(status.PhaseInstructions.Archive, "\n"), "verify-report.md exists") {
		t.Fatalf("Archive instructions = %v", status.PhaseInstructions.Archive)
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
	want := CommandArgs{ChangeName: "add-auth", CWD: "/tmp/repo", JSON: true, IncludeInstructions: true}
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

func seedReadyChange(t *testing.T, root string, name string, tasks string) string {
	t.Helper()
	changeRoot := filepath.Join(root, "openspec", "changes", name)
	write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Expected behavior\n")
	write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	write(t, filepath.Join(changeRoot, "tasks.md"), tasks)
	return changeRoot
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

func engramPlanningRoute(name string, route string) []engramObservation {
	observations := []engramObservation{}
	if route != "propose" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"})
	}
	if route == "design" || route == "tasks" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/spec", Content: "# Spec\n", Project: "gentle-ai", Scope: "project"})
	}
	if route == "tasks" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"})
	}
	if route == "propose" {
		observations = append(observations, engramObservation{Title: "sdd/" + name + "/tasks", Content: "- [ ] 1.1 Work\n", Project: "gentle-ai", Scope: "project"})
	}
	return observations
}

func write(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func stubEngramExport(t *testing.T, observations []engramObservation) func() {
	t.Helper()
	original := engramExport
	engramExport = func(_ string) ([]engramObservation, error) {
		return observations, nil
	}
	return func() { engramExport = original }
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
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

func ptrValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}
