package cli

import (
	"bytes"
	"testing"
)

// This file accumulates the threat-matrix "PR commands" RED tests Wave 7's
// consumer-first deletion slices each require (design.md Threat Matrix:
// "CLI verb dispatch loses cases... An unknown verb must return the
// existing `unknown review command %q`, never a panic or silent no-op").
// One test per retired verb, added in the same slice that retires it.

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
