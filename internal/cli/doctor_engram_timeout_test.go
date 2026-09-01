package cli

import (
	"context"
	"strings"
	"testing"
)

// TestEngramDeadlineIsReportedAsADeadline is #3068's second half.
//
// A deadline that elapsed is not evidence that the transport is broken, but
// the check reported `initialize handshake failed` and a remedy telling the
// operator to check their Engram command and arguments. #3068's reporter had
// correct arguments and a store that answered in about five seconds; they were
// sent to inspect the one thing the evidence did not implicate, while
// `Status: unhealthy` is what fail-closed consumers act on.
//
// Failing stays: a probe that did not answer within its budget is not a pass.
// What changes is that the refusal describes what actually happened and points
// at the timeout it exceeded.
func TestEngramDeadlineIsReportedAsADeadline(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, context.DeadlineExceeded)
	homeDir, installedAgents := doctorStdioConfig(t)

	got := checkEngramReachable(context.Background(), homeDir, installedAgents)

	if got.Status != CheckStatusFail {
		t.Fatalf("status = %s, want fail: a probe that never answered is not a pass", got.Status)
	}
	if strings.Contains(got.Detail, "handshake failed") {
		t.Fatalf("a deadline is still reported as a failed handshake: %s", got.Detail)
	}
	if !strings.Contains(got.Detail, "did not answer within") {
		t.Fatalf("detail does not say the probe ran out of time: %s", got.Detail)
	}
	if got.Remedy == nil {
		t.Fatal("expected a remedy")
	}
	remedy := got.Remedy.Description
	if strings.Contains(remedy, "command and arguments") {
		t.Fatalf("remedy still sends the operator to inspect command and arguments, which a deadline does not implicate: %s", remedy)
	}
	if !strings.Contains(remedy, "timeout") {
		t.Fatalf("remedy does not name the timeout the operator can raise: %s", remedy)
	}
}

// TestEngramNonDeadlineFailureKeepsItsRemedy is the guard on the other side.
// A handshake that genuinely answered wrong, or a process that exited, still
// implicates the configured command, and that remedy must survive.
func TestEngramNonDeadlineFailureKeepsItsRemedy(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, errStdioProbeTest)
	homeDir, installedAgents := doctorStdioConfig(t)

	got := checkEngramReachable(context.Background(), homeDir, installedAgents)

	if got.Status != CheckStatusFail {
		t.Fatalf("status = %s, want fail", got.Status)
	}
	if !strings.Contains(got.Detail, "handshake failed") {
		t.Fatalf("a real handshake failure lost its wording: %s", got.Detail)
	}
	if got.Remedy == nil || !strings.Contains(got.Remedy.Description, "command and arguments") {
		t.Fatalf("a real handshake failure lost the remedy that does implicate the config: %#v", got.Remedy)
	}
}

// errStdioProbeTest is an ordinary transport failure: the process answered
// wrongly rather than not at all.
var errStdioProbeTest = errProbeAnsweredWrong{}

type errProbeAnsweredWrong struct{}

func (errProbeAnsweredWrong) Error() string {
	return "engram mcp exited without answering initialize"
}
