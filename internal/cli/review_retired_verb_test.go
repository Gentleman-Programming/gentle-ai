package cli

import (
	"bytes"
	"reflect"
	"slices"
	"testing"
)

// This file accumulates the threat-matrix "PR commands" RED tests Wave 7's
// consumer-first deletion slices each require (design.md Threat Matrix:
// "CLI verb dispatch loses cases... An unknown verb must return the
// existing `unknown review command %q`, never a panic or silent no-op").
// One test per retired verb, added in the same slice that retires it.

// TestReviewRetiredVerbAdvisoryIsUnknownCommand verifies both former advisory
// command shapes refuse before they can alter the fixture's source or shared
// Git-common-dir authority.
func TestReviewRetiredVerbAdvisoryIsUnknownCommand(t *testing.T) {
	reviewEnabledHome(t)
	repo, contextArgs, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := contextArgs[slices.Index(contextArgs, "--repository-context")+1]
	lens := contextArgs[slices.Index(contextArgs, "--lens")+1]
	sourceBefore := [3]string{
		runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"),
		runReviewCLIGit(t, repo, "rev-parse", "HEAD"),
		runReviewCLIGit(t, repo, "diff", "--binary", "--full-index", "--no-color", "--no-ext-diff", "--no-textconv", "HEAD"),
	}
	authorityBefore := cliReviewAuthoritySnapshot(t, repo)

	tests := []struct {
		name string
		args []string
	}{
		{name: "prompt", args: []string{"advisory", "prompt", "--repository-context", handle, "--lens", lens, "--runtime", "claude-code"}},
		{name: "validate", args: []string{"advisory", "validate", "--repository-context", handle, "--lens", lens, "--input", "result.json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(test.args, &output)
			if err == nil {
				t.Fatal("retired verb advisory was accepted, want unknown-command refusal")
			}
			if want := `unknown review command "advisory"`; err.Error() != want {
				t.Fatalf("retired verb advisory error = %q, want %q", err.Error(), want)
			}
			if output.Len() != 0 {
				t.Fatalf("retired verb advisory wrote output before refusing: %q", output.String())
			}
			if sourceAfter := [3]string{
				runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"),
				runReviewCLIGit(t, repo, "rev-parse", "HEAD"),
				runReviewCLIGit(t, repo, "diff", "--binary", "--full-index", "--no-color", "--no-ext-diff", "--no-textconv", "HEAD"),
			}; sourceAfter != sourceBefore {
				t.Fatalf("retired verb advisory mutated tracked source: before=%q after=%q", sourceBefore, sourceAfter)
			}
			if authorityAfter := cliReviewAuthoritySnapshot(t, repo); !reflect.DeepEqual(authorityAfter, authorityBefore) {
				t.Fatalf("retired verb advisory mutated review authority: before=%#v after=%#v", authorityBefore, authorityAfter)
			}
		})
	}
}

// TestReviewRetiredVerbReconcileAuthorityIsUnknownCommand is WU7's (S3a)
// threat-matrix proof: once RunReviewReconcileAuthority and its dispatch
// case are gone, "review reconcile-authority" must refuse with the exact
// unknown-command message, never a panic or a silent no-op.
func TestReviewRetiredVerbReconcileAuthorityIsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"reconcile-authority", "--cwd", "."}, &output)
	if err == nil {
		t.Fatal("retired verb reconcile-authority was accepted, want unknown-command refusal")
	}
	if want := `unknown review command "reconcile-authority"`; err.Error() != want {
		t.Fatalf("retired verb reconcile-authority error = %q, want %q", err.Error(), want)
	}
	if output.Len() != 0 {
		t.Fatalf("retired verb reconcile-authority wrote output before refusing: %q", output.String())
	}
}

// TestReviewRetiredVerbReconcileAuthorityBatchIsUnknownCommand is WU9's
// (S4a) threat-matrix proof. This verb had TWO dispatch cases --
// runReviewCommandContext's own (which otherwise falls through to
// runReviewCommand by default) and runReviewCommand's -- both must be gone
// for the verb to become genuinely unknown rather than merely dropping to
// the second dispatcher's still-live case.
func TestReviewRetiredVerbReconcileAuthorityBatchIsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"reconcile-authority-batch", "--cwd", "."}, &output)
	if err == nil {
		t.Fatal("retired verb reconcile-authority-batch was accepted, want unknown-command refusal")
	}
	if want := `unknown review command "reconcile-authority-batch"`; err.Error() != want {
		t.Fatalf("retired verb reconcile-authority-batch error = %q, want %q", err.Error(), want)
	}
	if output.Len() != 0 {
		t.Fatalf("retired verb reconcile-authority-batch wrote output before refusing: %q", output.String())
	}
}

// TestReviewRetiredVerbQuarantineLegacyIsUnknownCommand,
// TestReviewRetiredVerbQuarantineLegacyFixScopeIsUnknownCommand, and
// TestReviewRetiredVerbRepairLegacyAliasIsUnknownCommand are WU14's (S5a)
// threat-matrix proof for the quarantine/repair legacy verb cluster. Each
// verb has exactly one dispatch case (in runReviewCommand; unlike
// reconcile-authority-batch none of these three are also matched earlier in
// runReviewCommandContext), so removing that one case is sufficient.
func TestReviewRetiredVerbQuarantineLegacyIsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"quarantine-legacy", "--cwd", "."}, &output)
	if err == nil {
		t.Fatal("retired verb quarantine-legacy was accepted, want unknown-command refusal")
	}
	if want := `unknown review command "quarantine-legacy"`; err.Error() != want {
		t.Fatalf("retired verb quarantine-legacy error = %q, want %q", err.Error(), want)
	}
	if output.Len() != 0 {
		t.Fatalf("retired verb quarantine-legacy wrote output before refusing: %q", output.String())
	}
}

func TestReviewRetiredVerbQuarantineLegacyFixScopeIsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"quarantine-legacy-fix-scope", "--cwd", "."}, &output)
	if err == nil {
		t.Fatal("retired verb quarantine-legacy-fix-scope was accepted, want unknown-command refusal")
	}
	if want := `unknown review command "quarantine-legacy-fix-scope"`; err.Error() != want {
		t.Fatalf("retired verb quarantine-legacy-fix-scope error = %q, want %q", err.Error(), want)
	}
	if output.Len() != 0 {
		t.Fatalf("retired verb quarantine-legacy-fix-scope wrote output before refusing: %q", output.String())
	}
}

func TestReviewRetiredVerbRepairLegacyAliasIsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"repair-legacy-alias", "--cwd", "."}, &output)
	if err == nil {
		t.Fatal("retired verb repair-legacy-alias was accepted, want unknown-command refusal")
	}
	if want := `unknown review command "repair-legacy-alias"`; err.Error() != want {
		t.Fatalf("retired verb repair-legacy-alias error = %q, want %q", err.Error(), want)
	}
	if output.Len() != 0 {
		t.Fatalf("retired verb repair-legacy-alias wrote output before refusing: %q", output.String())
	}
}
