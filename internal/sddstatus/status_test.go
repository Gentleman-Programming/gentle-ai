package sddstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestListActiveOpenSpecChanges(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "openspec", "changes", "z-change"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "a-change"))
	mkdir(t, filepath.Join(root, "openspec", "changes", "archive", "2026-01-01-old"))

	changes, err := ListActiveOpenSpecChanges(root)
	if err != nil {
		t.Fatalf("ListActiveOpenSpecChanges() error = %v", err)
	}

	want := []string{"a-change", "z-change"}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("ListActiveOpenSpecChanges() = %v, want %v", changes, want)
	}
}

func TestResolveUsesEngramArtifactsWhenOpenSpecIsAbsent(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".engram"))
	write(t, filepath.Join(root, ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:Gentleman-Programming/gentle-ai.git\n")

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
		t.Fatalf("Verify dependency = %q, want %q", status.Dependencies.Verify, DependencyReady)
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
			wantApply:   ApplyBlocked,
			wantApplyD:  DependencyBlocked,
			wantVerify:  DependencyBlocked,
			wantArchive: DependencyBlocked,
			wantNext:    "propose",
			wantBlocked: "proposal.md is missing or partial.",
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
			name: "apply all done makes verify ready",
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
			name: "apply progress makes verify ready before all tasks complete",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n- [ ] 1.2 Remaining\n")
				write(t, filepath.Join(changeRoot, "apply-progress.md"), "# Apply\n")
			},
			wantApply:   ApplyReady,
			wantApplyD:  DependencyReady,
			wantVerify:  DependencyReady,
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
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nPASS\n")
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
				write(t, filepath.Join(changeRoot, "verify-report.md"), strings.Join([]string{
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
				write(t, filepath.Join(changeRoot, "verify-report.md"), strings.Join([]string{
					"## Verification Report",
					"**Tests**: ✅ 12 passed / ❌ 0 failed / ⚠️ 1 skipped",
					"**CRITICAL**: None",
					"**WARNING**: flaky integration was skipped",
					"### Verdict",
					"PASS WITH WARNINGS",
					"",
				}, "\n"))
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
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive blocked when verify report has nonzero failed count",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nVerdict: PASS\nfailed: 1\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
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
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
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
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive blocked when verify report has blockers present",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nVerdict: PASS\nBlockers: missing evidence\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive blocked when verify report has todo pending and blockers",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nPASS\nTODO: finish audit\nPENDING: test run\nVerification blocker: missing evidence\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive blocked when verify report says status not passed",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nStatus: not passed\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive blocked when verify report says pass no",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nPASS: no\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive blocked when verify report has success and failure",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nStatus: SUCCESS\nFailure: build broke\n")
			},
			wantApply:   ApplyAllDone,
			wantApplyD:  DependencyAllDone,
			wantVerify:  DependencyReady,
			wantArchive: DependencyBlocked,
			wantNext:    "verify",
			wantBlocked: "verify-report.md is not clearly passing.",
		},
		{
			name: "archive ready when verify report has status pass",
			seed: func(t *testing.T, root string) {
				changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), "# Verify\nStatus: PASS\n")
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
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), blockedProse) {
		t.Fatalf("BlockedReasons = %v, want containing %q", status.BlockedReasons, blockedProse)
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

	// Missing proposal now routes to "propose", not "resolve-blockers".
	// Blocked prose lives in blockedReasons, separate from nextRecommended.
	for _, want := range []string{
		"next_recommended: propose",
		"### Blocked Reasons",
		"proposal.md is missing or partial.",
		`"nextRecommended": "propose"`,
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("RenderDispatcherMarkdown() missing %q:\n%s", want, markdown)
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
		"proposal.md is missing or partial.",
		"Read proposal, specs, design, and tasks before editing.",
		"```json",
		`"schemaName": "gentle-ai.sdd-status"`,
		"```",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("RenderNativePhasePrompt() missing %q:\n%s", want, prompt)
		}
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
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "# Auth Spec\n")
	write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	write(t, filepath.Join(changeRoot, "tasks.md"), tasks)
	return changeRoot
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
