package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// TestSDDAttemptBoundFinishNamesTheExitOutOfAStrandedSuccessor is the
// discoverability proof for the block the extended benchmark corpus reported
// as journey j39-sdd-remediation-stranded-successor, and the closest surviving
// relative of the deadlock a community tester reported.
//
// The state: the bound lineage was superseded by a real recovery successor,
// and then the widened scope that successor froze was taken back out. The
// successor's frozen target can never be the live candidate again, so it can
// never be finalized — while it exists, the bound lineage is not the compact
// recovery leaf and its post-apply gate does not allow.
//
// The refusal used to route to the review router, whose printed transition
// runs, exits 0 and changes nothing; and the remediation finish it then asked
// for refused with "compact post-apply gate is not allow", which names nothing
// runnable at all. The one operation that clears this is abandoning the
// stranded successor, and no message on this path named it.
//
// This test requires the refusal to name that exit and proves the naming by
// running it: the command is lifted out of the message the product printed,
// the maintainer authorization is assembled from the template the same message
// prints, only the operator's own run facts are substituted, and the block must
// then clear all the way to a completed objective.
func TestSDDAttemptBoundFinishNamesTheExitOutOfAStrandedSuccessor(t *testing.T) {
	fixture := newBoundRemediationFixture(t, "cli-stranded-successor")
	successor := fixture.lineage + "-successor"

	// A later scope change hands the bound lineage a real recovery successor,
	// and the widened path is then taken back out. The successor's frozen
	// target survives that revert, so it is stranded: its live projection will
	// never match its own authority again.
	widened := filepath.Join(fixture.changeRoot, "widened.md")
	writeCLIAttemptFile(t, widened, "# widened\n")
	runReviewCLIGit(t, fixture.repo, "add", widened)
	createCLIRecoverySuccessor(t, fixture.repo, fixture.lineage, successor)
	runReviewCLIGit(t, fixture.repo, "rm", "-q", "--cached", widened)
	removeCLIAttemptFile(t, widened)

	message := boundPassingFinishRefusal(t, fixture, "stranded-finish-2")
	if !strings.Contains(message, successor) {
		t.Fatalf("refusal never names the stranded successor that is in the way:\n%s", message)
	}

	arguments := namedRunnableGentleCommand(t, message)
	if len(arguments) < 3 || arguments[0] != "gentle-ai" || arguments[1] != "review" || arguments[2] != "abandon" {
		t.Fatalf("refusal names %v, not the abandonment that clears the strand:\n%s", arguments, message)
	}
	arguments = arguments[2:]

	const actor = "cli-stranded-exit-test"
	const reason = "the stranded successor can never be finalized"
	want := map[string]string{
		"<actor>":                    actor,
		"<why-it-is-abandoned>":      reason,
		"<maintainer-authorization>": namedAbandonAuthorization(t, message, actor, reason),
	}
	for _, placeholder := range namedCommandPlaceholders(arguments) {
		value, known := want[placeholder]
		if !known {
			t.Fatalf("refusal leaves %s for the operator to guess; it is not one of the run's own facts:\n%s", placeholder, message)
		}
		arguments = fillNamedCommandPlaceholder(arguments, placeholder, value)
	}

	var abandoned bytes.Buffer
	if abandonErr := RunReview(arguments, &abandoned); abandonErr != nil {
		t.Fatalf("the exit the refusal named does not work: review %v: %v\nrefusal:\n%s\noutput:\n%s",
			arguments, abandonErr, message, abandoned.String())
	}

	// The block has to be gone, not merely differently worded. The same plain
	// passing finish now reaches the self-successor branch, and running the
	// exit THAT branch names has to complete the objective.
	followUp := boundPassingFinishRefusal(t, fixture, "stranded-finish-3")
	finish := namedRunnableGentleCommand(t, followUp)
	if len(finish) < 3 || finish[1] != "sdd-attempt" || finish[2] != "finish" {
		t.Fatalf("after the abandonment the refusal still does not name the self-successor finish:\n%s", followUp)
	}
	finish = finish[2:]
	completions := map[string]string{
		"<unique-request-id>":         "stranded-finish-4",
		"<corrected-evidence-sha256>": cliAttemptHash('b'),
		"<proven-diagnosis>":          "bounded self remediation passed corrected verification",
		"<reused|invalidated>":        "reused",
		"<cleanup-evidence>":          "self remediation cleanup completed",
		"<process-evidence>":          "self remediation process scan found no descendants",
	}
	for _, placeholder := range namedCommandPlaceholders(finish) {
		value, known := completions[placeholder]
		if !known {
			t.Fatalf("the self-successor finish leaves %s to guess:\n%s", placeholder, followUp)
		}
		finish = fillNamedCommandPlaceholder(finish, placeholder, value)
	}
	var completed bytes.Buffer
	if finishErr := RunSDDAttempt(finish, &completed); finishErr != nil {
		t.Fatalf("the chain the refusal started does not terminate: sdd-attempt %v: %v\noutput:\n%s",
			finish, finishErr, completed.String())
	}
	var status sddstatus.RuntimeStatus
	decodeStrictReviewJSON(t, completed.Bytes(), &status)
	if !status.Complete || status.ActiveAttempt != nil || status.NextAction != sddstatus.RuntimeActionComplete {
		t.Fatalf("the chain the refusal started did not clear the block: %#v", status)
	}
}

// TestSDDAttemptBoundFinishStillRoutesUnstrandedSuccessorsToTheRouter keeps the
// abandonment from becoming the answer to every non-leaf state. A successor
// that governs the candidate actually charged is not stranded: it can be
// reviewed and approved, and it is then a legal --successor-lineage. Naming an
// abandonment there would destroy a live review the operator still wants.
func TestSDDAttemptBoundFinishStillRoutesUnstrandedSuccessorsToTheRouter(t *testing.T) {
	fixture := newBoundRemediationFixture(t, "cli-unstranded-successor")

	writeCLIAttemptFile(t, filepath.Join(fixture.changeRoot, "tasks.md"),
		"- [x] 1.1 Done\n# bounded self remediation\n# a later scope change\n")
	createCLIRecoverySuccessor(t, fixture.repo, fixture.lineage, fixture.lineage+"-successor")

	message := boundPassingFinishRefusal(t, fixture, "unstranded-finish-2")
	if strings.Contains(message, "gentle-ai review abandon") {
		t.Fatalf("refusal names an abandonment for a successor that still governs the candidate:\n%s", message)
	}
	arguments := namedRunnableGentleCommand(t, message)
	if len(arguments) < 3 || arguments[1] != "review" || arguments[2] != "status" {
		t.Fatalf("refusal names %v, not the review router:\n%s", arguments, message)
	}
}

// namedAbandonAuthorization assembles the maintainer authorization out of the
// template the refusal itself printed, substituting only the actor and reason
// the operator chose. Building it any other way would test this file's idea of
// the binding rather than the product's.
func namedAbandonAuthorization(t *testing.T, message, actor, reason string) string {
	t.Helper()
	const marker = reviewtransaction.CompactAbandonAuthorizationSchema
	start := strings.Index(message, marker)
	if start < 0 {
		t.Fatalf("refusal names an abandonment without the authorization template:\n%s", message)
	}
	lines := strings.Split(message[start:], "\n")
	if len(lines) < 6 {
		t.Fatalf("authorization template is not six lines:\n%s", message)
	}
	template := strings.Join(lines[:6], "\n")
	template = strings.ReplaceAll(template, "<--actor>", actor)
	template = strings.ReplaceAll(template, "<--reason>", reason)
	if strings.Contains(template, "<") {
		t.Fatalf("authorization template still carries a value the product knows:\n%s", template)
	}
	return template
}

func removeCLIAttemptFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
