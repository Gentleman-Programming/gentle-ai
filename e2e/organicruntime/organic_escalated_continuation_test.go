// Escalation continuation journeys. Both cases here are refusals that are
// CORRECT: an escalated lineage really is terminal, and recovering it into a
// successor whose target is byte-identical really would mint fresh authority
// for content that already failed its verification. What is under test is not
// whether the product refuses, but whether the refusal names something the
// operator can actually run — and whether running exactly that clears the
// block.
//
// Every assertion drives the real binary. The continuation is read out of the
// message the product itself printed, never out of knowledge this test has
// independently, so a message that names a dead end fails here.
package organicruntime_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// organicDottedOperation matches an internal operation identifier such as
// "review.status". Those are wire tokens for the negotiated contract; typing
// one into a shell does nothing.
var organicDottedOperation = regexp.MustCompile(`review\.[a-z_-]+`)

// organicNamedCommand extracts the single `gentle-ai ...` command a refusal
// names, and requires the refusal to name exactly one. Placeholders the
// operator must fill in are returned separately so the caller has to make a
// deliberate choice for each one instead of silently running a broken command.
func organicNamedCommand(t *testing.T, message string) ([]string, []string) {
	t.Helper()
	if match := organicDottedOperation.FindString(message); match != "" {
		t.Fatalf("refusal names the internal operation identifier %q, which is not runnable:\n%s", match, message)
	}
	index := strings.Index(message, "gentle-ai ")
	if index < 0 {
		t.Fatalf("refusal names no runnable command:\n%s", message)
	}
	tail := message[index:]
	if end := strings.IndexAny(tail, "\r\n"); end >= 0 {
		tail = tail[:end]
	}
	fields := strings.Fields(strings.TrimRight(tail, ".;,"))
	if len(fields) < 2 || fields[0] != "gentle-ai" {
		t.Fatalf("refusal named %q, which is not a gentle-ai command:\n%s", tail, message)
	}
	placeholders := []string{}
	for _, field := range fields {
		if strings.HasPrefix(field, "<") && strings.HasSuffix(field, ">") {
			placeholders = append(placeholders, field)
		}
	}
	return fields[1:], placeholders
}

// organicFillPlaceholder substitutes one operator-supplied token.
func organicFillPlaceholder(arguments []string, placeholder, value string) []string {
	filled := append([]string(nil), arguments...)
	for index := range filled {
		if filled[index] == placeholder {
			filled[index] = value
		}
	}
	return filled
}

// escalateOrganicCandidate builds the exact state the corpus builds for the
// escalate-then-recover journey: a passive tier-0 candidate whose final
// verification FAILED, which is the only deterministic route to a terminal
// escalated lineage.
func escalateOrganicCandidate(t *testing.T, harness *organicHarness, lineage string) {
	t.Helper()
	harness.writeFiles(map[string]string{
		"docs/escalated.md": "# escalated\n\nplain prose, no executable content.\n",
	})
	harness.git("add", "-A")
	started, _ := harness.startReview(lineage)
	if len(started.SelectedLenses) != 0 {
		t.Fatalf("passive prose selected lenses: %#v", started.SelectedLenses)
	}
	failed := filepath.Join(t.TempDir(), "failed-verification.txt")
	if err := os.WriteFile(failed, []byte("go build ./... ok\ngo test ./... FAIL\n2 packages failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := harness.finalize(lineage, "--evidence", failed, "--failed=true"); result.State != "escalated" {
		t.Fatalf("failed final verification did not escalate: %#v", result)
	}
}

// fixOrganicEscalatedCandidate applies the change the refusals ask for.
func fixOrganicEscalatedCandidate(harness *organicHarness) {
	harness.writeFiles(map[string]string{
		"docs/escalated.md": "# escalated\n\nplain prose, no executable content.\nthe failure the verification named is fixed.\n",
	})
	harness.git("add", "-A")
}

// TestOrganicEscalatedGateDenialNamesARunnableContinuation keeps the escalated
// safety boundary after gates became informational. An escalated authority may
// remain for diagnosis, but validate cannot authorize or veto delivery; changed
// work starts a distinct transaction instead of relying on gate routing.
func TestOrganicEscalatedGateDenialNamesARunnableContinuation(t *testing.T) {
	harness := newOrganicHarness(t)
	escalateOrganicCandidate(t, harness, "escalated-gate-denial")

	gate := harness.gate("pre-commit")
	harness.assertInvalidatedUnmanagedGate(gate)
	if gate.Context.Denial != nil {
		t.Fatalf("informational escalated gate retained deciding denial context: %#v", gate)
	}

	fixOrganicEscalatedCandidate(harness)
	fresh, _ := harness.startReview("escalated-gate-denial-fresh")
	harness.approveReview("escalated-gate-denial-fresh", fresh)
}

// TestOrganicEscalatedRecoveryRefusalNamesWhatToChange is block 2. Recovering
// an escalated lineage into a successor whose target has not moved is refused,
// and that refusal is correct: the successor would carry the exact bytes whose
// verification failed, so it would mint new authority for unchanged content.
//
// The refusal used to name nothing at all. It must say why the recovery is
// empty and name the command to run once the candidate really has changed.
func TestOrganicEscalatedRecoveryRefusalNamesWhatToChange(t *testing.T) {
	harness := newOrganicHarness(t)
	const lineage = "escalated-unchanged-recovery"
	escalateOrganicCandidate(t, harness, lineage)

	revision := harnessStatus(t, harness, lineage).Authority.Revision
	attempt := []string{
		"review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", lineage + "-successor",
		"--disposition", "escalated",
	}
	_, stderr, err := harness.gentleAllowFailure(attempt...)
	if err == nil {
		t.Fatalf("recovery into an unchanged target was allowed")
	}
	message := strings.TrimSpace(stderr)
	if !strings.Contains(message, "nothing to re-review") {
		t.Fatalf("unchanged-target refusal does not say why it refuses:\n%s", message)
	}
	if !strings.Contains(message, "change the candidate") {
		t.Fatalf("unchanged-target refusal does not say the candidate has to change:\n%s", message)
	}
	arguments, placeholders := organicNamedCommand(t, message)
	if len(placeholders) != 0 {
		t.Fatalf("unchanged-target refusal named placeholders it already knows the values for: %v\n%s", placeholders, message)
	}

	// Follow the message end to end: change the candidate, then run exactly
	// the command it printed, with no substitution at all.
	fixOrganicEscalatedCandidate(harness)
	if stdout, recoverStderr, recoverErr := harness.gentleAllowFailure(arguments...); recoverErr != nil {
		t.Fatalf("the command the unchanged-target refusal named failed: gentle-ai %v: %v\nstdout:\n%s\nstderr:\n%s",
			arguments, recoverErr, stdout, recoverStderr)
	}
	approved := harness.finalize(lineage + "-successor")
	harness.assertReviewBurned(lineage+"-successor", approved)
	harness.assertInvalidatedUnmanagedGate(harness.gate("pre-commit"))
}
