package sddstatus

import (
	"strings"
	"testing"
)

// TestNormalizeFinishAttemptRequestRevisionRefusalsAreSelfDiagnosing is the RED
// reproduction for #2294: `sdd-attempt settle` (and `finish`) rejected every
// --evidence-revision value — a 40-hex git SHA, a bare 64-hex value, and a
// sha256:<64-hex>-prefixed value with uppercase hex or incidental whitespace —
// with the identical opaque "evidence_revision must be sha256" text. The
// validation RULE (require an exact sha256:<64-lowercase-hex> token) is
// correct and stays unchanged here; only the message's legibility is under
// test: it must name the expected shape, describe (without echoing) what was
// actually received, and name a runnable exit.
func TestNormalizeFinishAttemptRequestRevisionRefusalsAreSelfDiagnosing(t *testing.T) {
	uppercaseHex := "sha256:" + strings.Repeat("A", 64)
	base := func() FinishAttemptRequest {
		return FinishAttemptRequest{
			ExpectedRevision: runtimeTestHash('1'), RequestID: "legibility-check", Outcome: AttemptPassed,
			EvidenceRevision: runtimeTestHash('2'), Diagnosis: "diagnosis", HarnessDisposition: HarnessReused,
			CleanupEvidence: "cleanup", ProcessEvidence: "process",
		}
	}

	t.Run("evidence_revision", func(t *testing.T) {
		request := base()
		request.EvidenceRevision = uppercaseHex
		_, err := normalizeFinishAttemptRequest(request)
		assertSelfDiagnosingRevisionRefusal(t, err, uppercaseHex, "--evidence-revision")
	})

	t.Run("remediates_evidence_revision", func(t *testing.T) {
		request := base()
		request.RemediatesEvidenceRevision = uppercaseHex
		_, err := normalizeFinishAttemptRequest(request)
		assertSelfDiagnosingRevisionRefusal(t, err, uppercaseHex, "--remediates-evidence-revision")
	})
}

// assertSelfDiagnosingRevisionRefusal is the mutation-style proof for the
// message-shape fix: it asserts the concrete expected-shape literal, a
// redacted length observation, and a runnable exit naming the actual flag —
// and that the raw offending value never appears verbatim, since it may be
// sensitive or long. Reverting the message-quality fix collapses the refusal
// back to the bare "<field> must be sha256" text and fails every assertion
// here except the raw-value-absence one (which a bare message also passes
// vacuously, so the other three are what actually guard the fix).
func assertSelfDiagnosingRevisionRefusal(t *testing.T, err error, rawValue, wantFlag string) {
	t.Helper()
	if err == nil {
		t.Fatal("normalizeFinishAttemptRequest = nil error, want a refusal")
	}
	message := err.Error()
	if !strings.Contains(message, "sha256:<64-lowercase-hex>") {
		t.Fatalf("refusal %q does not name the expected shape sha256:<64-lowercase-hex>", message)
	}
	if !strings.Contains(message, "length=") {
		t.Fatalf("refusal %q does not include a redacted length observation", message)
	}
	if !strings.Contains(message, wantFlag) {
		t.Fatalf("refusal %q does not name a runnable exit with %s", message, wantFlag)
	}
	if strings.Contains(message, rawValue) {
		t.Fatalf("refusal %q echoes the raw revision value verbatim, which may be sensitive/long", message)
	}
}
