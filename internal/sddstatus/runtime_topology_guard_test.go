package sddstatus

import (
	"context"
	"os"
	"path/filepath"
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
			if initial.ApplyState != ApplyBlocked || initial.Consent == nil ||
				!strings.Contains(initialReasons, "blocked(edit_authority_missing)") ||
				strings.Contains(initialReasons, "cross_common_dir_runtime_target") {
				t.Fatalf("pre-grant status = %#v, want blocked edit-authority consent without topology blocker", initial)
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
