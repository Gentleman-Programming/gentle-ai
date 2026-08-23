package sddstatus

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestApplyNativeRuntimeErrorRoutingRendersWindowsPathVerbatim(t *testing.T) {
	change := "my-change"
	status := &Status{ChangeName: &change}
	status.ActionContext.WorkspaceRoot = `C:\Users\dev\repo`
	applyNativeRuntimeErrorRouting(status, errors.New("ledger unreadable"))
	got := strings.Join(status.BlockedReasons, "\n")
	want := `--cwd "C:\Users\dev\repo"`
	if !strings.Contains(got, want) {
		t.Fatalf("corrupt-authority diagnostic does not contain the path as the filesystem knows it:\nwant substring: %s\ngot: %s", want, got)
	}
}

func TestApplyNativeRuntimeRoutingRendersWindowsPathVerbatim(t *testing.T) {
	status := &Status{RuntimeStatus: &RuntimeStatus{Change: "my-change", DecisionRequired: true}}
	status.ActionContext.WorkspaceRoot = `C:\Users\dev\repo`
	applyNativeRuntimeRouting(status)
	got := strings.Join(status.BlockedReasons, "\n")
	want := `in "C:\Users\dev\repo"`
	if !strings.Contains(got, want) {
		t.Fatalf("blocked-runtime reason does not contain the path as the filesystem knows it:\nwant substring: %s\ngot: %s", want, got)
	}
}

func TestNativeRuntimeInstructionsRenderWindowsPathVerbatim(t *testing.T) {
	status := Status{
		RemediationState: RemediationState{Required: true, FailedEvidenceRevision: "sha256:aa"},
		RuntimeStatus: &RuntimeStatus{
			Objective: &RuntimeObjective{WorkUnit: "unit", EvidenceGoal: "goal", MaxAttempts: 2, MaxChangedLines: 200},
		},
	}
	status.ActionContext.WorkspaceRoot = `C:\Users\dev\repo`
	got := strings.Join(nativeRuntimeInstructions(status, "my-change"), "\n")
	want := `--cwd "C:\Users\dev\repo"`
	if occurrences := strings.Count(got, want); occurrences != 4 {
		t.Fatalf("runtime instructions must all carry the verbatim path:\nwant 4 occurrences of %s, got %d\ngot: %s", want, occurrences, got)
	}
}

func TestNonPhaseRoutingInstructionsRenderWindowsPathVerbatim(t *testing.T) {
	want := `--cwd "C:\Users\dev\repo"`
	tests := []struct {
		next        string
		occurrences int
	}{
		{next: "select-change", occurrences: 2},
	}
	for _, tt := range tests {
		t.Run(tt.next, func(t *testing.T) {
			status := Status{NextRecommended: tt.next}
			status.ActionContext.WorkspaceRoot = `C:\Users\dev\repo`
			instructions, ok := nonPhaseRoutingInstructions(status)
			if !ok {
				t.Fatalf("nonPhaseRoutingInstructions(%q) returned no instructions", tt.next)
			}
			got := strings.Join(instructions, "\n")
			if occurrences := strings.Count(got, want); occurrences != tt.occurrences {
				t.Fatalf("routing instructions must carry the verbatim path:\nwant %d occurrences of %s, got %d\ngot: %s", tt.occurrences, want, occurrences, got)
			}
		})
	}
}

// The stranded-successor refusal is gone with the gate it served. The Windows
// path-quoting rule it guarded stays covered by the reset and
// objective-change refusals above.

func TestRuntimeWorktreeMismatchRefusalRendersWindowsPathVerbatim(t *testing.T) {
	store := RuntimeStore{Workspace: `C:\Users\dev\elsewhere`}
	err := store.runtimeWorktreeMismatchRefusal(1, `C:\Users\dev\repo`)
	for _, want := range []string{
		`began in "C:\Users\dev\repo"`,
		`running from "C:\Users\dev\elsewhere"`,
		`--cwd "C:\Users\dev\repo"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("worktree-mismatch refusal does not contain the path as the filesystem knows it:\nwant substring: %s\ngot: %s", want, err)
		}
	}
}

func TestRuntimeObjectiveChangeRefusalRendersWindowsPathVerbatim(t *testing.T) {
	want := `--cwd "C:\Users\dev\repo"`
	objective := &RuntimeObjective{WorkUnit: "unit", EvidenceGoal: "goal", MaxAttempts: 2, MaxChangedLines: 200}
	tests := []struct {
		name        string
		status      RuntimeStatus
		occurrences int
	}{
		{
			// DecisionRequired makes reset structurally permitted and admissible
			// without touching Git, so the reset branch renders.
			name:        "reset branch",
			status:      RuntimeStatus{Revision: "rev-1", DecisionRequired: true, Objective: objective},
			occurrences: 2,
		},
		{
			// No attempts, not decision-required, not complete: reset and rescope
			// are both structurally refused, so the fail-closed default renders.
			name:        "fail-closed default branch",
			status:      RuntimeStatus{Revision: "rev-1", Objective: objective},
			occurrences: 1,
		},
	}
	store := RuntimeStore{Workspace: `C:\Users\dev\repo`, Change: "my-change"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.runtimeObjectiveChangeRefusal(context.Background(), tt.status)
			if occurrences := strings.Count(err.Error(), want); occurrences != tt.occurrences {
				t.Fatalf("objective-change refusal must carry the verbatim path:\nwant %d occurrences of %s, got %d\ngot: %s", tt.occurrences, want, occurrences, err)
			}
		})
	}
}
