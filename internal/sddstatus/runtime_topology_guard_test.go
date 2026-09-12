package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeTopologyGuardBlocksForeignRuntimeActors(t *testing.T) {
	tests := []struct {
		name               string
		linkedWorktree     bool
		aliasTarget        bool
		completeTasks      bool
		incompletePlanning bool
		remediationRoute   bool
		wantApplyState     ApplyState
		wantApply          DependencyState
		wantVerify         DependencyState
		wantNext           string
		wantTopologyHit    bool
	}{
		{
			name:               "incomplete planning preserves proposal route",
			incompletePlanning: true,
		},
		{
			name:            "independent repository blocks apply after edit grant",
			wantApplyState:  ApplyBlocked,
			wantApply:       DependencyBlocked,
			wantVerify:      DependencyBlocked,
			wantNext:        "resolve-blockers",
			wantTopologyHit: true,
		},
		{
			name:            "independent repository blocks verification after tasks complete",
			completeTasks:   true,
			wantApplyState:  ApplyAllDone,
			wantApply:       DependencyAllDone,
			wantVerify:      DependencyBlocked,
			wantNext:        "resolve-blockers",
			wantTopologyHit: true,
		},
		{
			name:             "independent repository blocks remediation after failed verification",
			completeTasks:    true,
			remediationRoute: true,
			wantApplyState:   ApplyAllDone,
			wantApply:        DependencyAllDone,
			wantVerify:       DependencyBlocked,
			wantNext:         "resolve-blockers",
			wantTopologyHit:  true,
		},
		{
			name:           "registered linked worktree shares candidate accounting",
			linkedWorktree: true,
			wantApplyState: ApplyReady,
			wantApply:      DependencyReady,
			wantVerify:     DependencyBlocked,
			wantNext:       "apply",
		},
		{
			name:           "canonical alias to linked worktree shares candidate accounting",
			linkedWorktree: true,
			aliasTarget:    true,
			wantApplyState: ApplyReady,
			wantApply:      DependencyReady,
			wantVerify:     DependencyBlocked,
			wantNext:       "apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			planning := filepath.Join(workspace, "planning")
			initEditAuthorityGitRepo(t, planning, true)
			target := filepath.Join(workspace, "target")
			if tt.linkedWorktree {
				target = filepath.Join(t.TempDir(), "linked-worktree")
				runRuntimeLedgerGit(t, planning, "worktree", "add", "-q", "-b", "topology-guard", target)
			} else {
				initEditAuthorityGitRepo(t, target, false)
			}
			planning = realPath(t, planning)
			target = realPath(t, target)
			taskTarget := target
			if tt.aliasTarget {
				aliasParent := filepath.Join(t.TempDir(), "linked-worktree-alias")
				if err := os.Symlink(filepath.Dir(target), aliasParent); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}
				taskTarget = filepath.Join(aliasParent, filepath.Base(target))
			}

			const change = "runtime-topology-guard"
			taskPath := filepath.Join(taskTarget, "internal", "api", "handler.go")
			tasks := "- [ ] 1.1 Update `" + taskPath + "`\n"
			changeRoot := filepath.Join(planning, "openspec", "changes", change)
			if tt.incompletePlanning {
				write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Expected behavior\n")
				write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
				write(t, filepath.Join(changeRoot, "tasks.md"), tasks)
			} else {
				changeRoot = seedReadyChange(t, planning, change, tasks)
			}
			initial, err := Resolve(ResolveOptions{CWD: planning, ChangeName: change})
			if err != nil {
				t.Fatal(err)
			}
			if tt.incompletePlanning {
				if initial.ApplyState != ApplyBlocked || initial.Dependencies.Proposal != DependencyBlocked ||
					initial.NextRecommended != "propose" || strings.Contains(strings.Join(initial.BlockedReasons, "\n"), "cross_common_dir_runtime_target") {
					t.Fatalf("incomplete-planning status = %#v, want blocked/propose without topology blocker", initial)
				}
				return
			}
			initialReasons := strings.Join(initial.BlockedReasons, "\n")
			if initial.ApplyState != ApplyBlocked ||
				!strings.Contains(initialReasons, "blocked(edit_authority_missing)") ||
				strings.Contains(initialReasons, "cross_common_dir_runtime_target") {
				t.Fatalf("pre-grant status = %#v, want blocked edit-authority status without topology blocker", initial)
			}
			if err := PrepareChangeInstanceConsent(initial); err != nil {
				t.Fatal(err)
			}

			instance, err := readChangeInstanceMarker(changeRoot)
			if err != nil || instance == "" {
				t.Fatalf("read change instance marker = %q, %v", instance, err)
			}
			opened, err := OpenRuntimeStore(context.Background(), planning, change)
			if err != nil {
				t.Fatal(err)
			}
			store, err := opened.ForInstance(instance)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Grant(context.Background(), GrantRootsRequest{
				RequestID: "grant-runtime-topology-guard", Roots: []string{target},
				Reason: "test the status topology guard", Actor: "test",
			}); err != nil {
				t.Fatal(err)
			}
			if tt.completeTasks {
				write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Update `"+taskPath+"`\n")
			}
			if tt.remediationRoute {
				write(t, filepath.Join(changeRoot, "verify-report.md"), "```yaml\nschema: gentle-ai.verify-result/v1\nevidence_revision: sha256:"+strings.Repeat("a", 64)+"\nverdict: fail\nblockers: 1\ncritical_findings: 0\nrequirements: 1/1\nscenarios: 1/1\ntest_command: go test ./...\ntest_exit_code: 0\ntest_output_hash: sha256:"+strings.Repeat("b", 64)+"\nbuild_command: go vet ./...\nbuild_exit_code: 0\nbuild_output_hash: sha256:"+strings.Repeat("c", 64)+"\n```")
			}
			beforeTask, err := os.ReadFile(filepath.Join(changeRoot, "tasks.md"))
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Status()
			if err != nil {
				t.Fatal(err)
			}

			status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: change})
			if err != nil {
				t.Fatal(err)
			}
			if status.ApplyState != tt.wantApplyState || status.Dependencies.Apply != tt.wantApply ||
				status.Dependencies.Verify != tt.wantVerify || status.NextRecommended != tt.wantNext ||
				(tt.remediationRoute && !status.RemediationState.Required) {
				t.Fatalf("post-grant status = %#v, want applyState=%q dependencies apply/verify=%q/%q next=%q", status, tt.wantApplyState, tt.wantApply, tt.wantVerify, tt.wantNext)
			}

			reasons := strings.Join(status.BlockedReasons, "\n")
			if tt.wantTopologyHit {
				if !strings.Contains(reasons, "blocked(cross_common_dir_runtime_target)") ||
					!strings.Contains(reasons, "shared linked worktree") ||
					!strings.Contains(reasons, "separately planned and runtime-accounted SDD changes") {
					t.Fatalf("topology blocker lacks its typed code or actionable exits: %s", reasons)
				}
			} else if strings.Contains(reasons, "cross_common_dir_runtime_target") {
				t.Fatalf("same-common-dir linked worktree was blocked: %s", reasons)
			}

			after, err := store.Status()
			if err != nil {
				t.Fatal(err)
			}
			afterTask, err := os.ReadFile(filepath.Join(changeRoot, "tasks.md"))
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision || len(after.Attempts) != len(before.Attempts) || string(afterTask) != string(beforeTask) {
				t.Fatalf("status topology guard mutated runtime or task state: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestGrantRefusesMarkerReplacementDuringMutation(t *testing.T) {
	for _, phase := range []string{"before", "before-replay", "after", "after-replay", "publication-error"} {
		t.Run(phase, func(t *testing.T) {
			repo := t.TempDir()
			initEditAuthorityGitRepo(t, repo, true)
			const change = "grant-marker-race"
			root := seedReadyChange(t, repo, change, "- [ ] Update source\n")
			marker := filepath.Join(root, changeInstanceMarkerFile)
			a, b := "sdd-"+strings.Repeat("a", 32), "sdd-"+strings.Repeat("b", 32)
			write(t, marker, a+"\n")
			opened, err := OpenRuntimeStore(t.Context(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			store, err := opened.ForCurrentChangeInstance(a)
			if err != nil {
				t.Fatal(err)
			}
			request := GrantRootsRequest{RequestID: "race-grant", Roots: []string{repo}, Actor: "test", Reason: "marker boundary"}
			replayed := strings.HasSuffix(phase, "replay")
			before, err := store.Status()
			if err != nil {
				t.Fatal(err)
			}
			if replayed {
				before, err = store.Grant(t.Context(), request)
				if err != nil || len(before.GrantedRoots) != 1 {
					t.Fatalf("valid grant: %#v %v", before, err)
				}
				valid, err := store.Grant(t.Context(), request)
				if err != nil || !reflect.DeepEqual(valid, before) {
					t.Fatalf("valid replay changed authority: %#v %v", valid, err)
				}
			}
			receiptRevision := before.Revision
			if replayed {
				later := request
				later.RequestID, later.ExpectedRevision = "later-grant", before.Revision
				before, err = store.Grant(t.Context(), later)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := store.ensureDirectories(); err != nil {
				t.Fatal(err)
			}
			clock, sync := runtimeGrantClock, runtimeSyncDirectory
			t.Cleanup(func() { runtimeGrantClock, runtimeSyncDirectory = clock, sync })
			injected := errors.New("injected directory sync failure")
			if strings.HasPrefix(phase, "before") {
				runtimeGrantClock = func() string { write(t, marker, b+"\n"); return clock() }
			} else {
				runtimeSyncDirectory = func(dir string) error {
					if err := sync(dir); err != nil {
						return err
					}
					if dir == store.Dir {
						write(t, marker, b+"\n")
						if phase == "publication-error" {
							return injected
						}
					}
					return nil
				}
			}
			result, err := store.Grant(t.Context(), request)
			if err == nil || len(result.GrantedRoots) != 0 {
				t.Errorf("stale result usable: %#v %v", result, err)
			}
			after, readErr := store.Status()
			if readErr != nil {
				t.Fatal(readErr)
			}
			var publication *RuntimePublicationError
			committed := !strings.HasPrefix(phase, "before")
			if !replayed {
				receiptRevision = after.Revision
			}
			if errors.As(err, &publication) != committed || (committed && (!publication.Committed || publication.Revision != receiptRevision)) {
				t.Errorf("incorrect committed state: %#v %v", publication, err)
			}
			if (replayed || !committed) && !reflect.DeepEqual(after, before) {
				t.Errorf("refusal/replay changed history: before=%#v after=%#v", before, after)
			}
			if committed && (after.Revision == "" || len(after.GrantedRoots) != 1) {
				t.Error("committed historical grant lost")
			}
			if committed && phase != "publication-error" && err != nil && strings.Contains(err.Error(), "requires exact replay") {
				t.Errorf("stale receipt encouraged unsafe replay: %v", err)
			}
			if phase == "publication-error" && !errors.Is(err, injected) {
				t.Errorf("publication failure masked: %v", err)
			}
			boundB, _ := opened.ForInstance(b)
			isolated, err := boundB.Status()
			if err != nil || len(isolated.GrantedRoots) != 0 {
				t.Fatalf("replacement inherited old roots: %#v %v", isolated, err)
			}
		})
	}
}
