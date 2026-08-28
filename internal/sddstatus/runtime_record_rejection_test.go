package sddstatus

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

// #3816 change 3 / #2834. The ledger carried a bespoke refusal for every way a
// record could fail to be what the authority wrote: distinct message, distinct
// category, and a justifying paragraph each. Reaching any of them requires a
// record this package built, digested and CAS-chained to disagree with itself,
// and the response to all of them is the same. One assertion carries the same
// information; fifty-nine carry it fifty-nine times, and each is a place a
// future change can drift out of agreement with the writer.
//
// The decision recorded on #3816 is that the collapsed refusal carries the
// failed condition as a token plus the revision of the offending record, and
// nothing else. Expected/actual pairs are excluded on purpose: that is where
// the bespoke messages came from and is the drift surface this removes.

// TestRuntimeRecordRejectionCarriesConditionAndRevision pins the shape.
func TestRuntimeRecordRejectionCarriesConditionAndRevision(t *testing.T) {
	err := rejectRuntimeRecord("objective_rescope_widens_current")

	var rejected *RuntimeRecordRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("rejectRuntimeRecord returned %T, want *RuntimeRecordRejectedError", err)
	}
	if rejected.Condition != "objective_rescope_widens_current" {
		t.Errorf("Condition = %q", rejected.Condition)
	}
	if rejected.Revision != "" {
		t.Errorf("Revision = %q, want empty until the record is stamped", rejected.Revision)
	}

	stamped := withRuntimeRecordRevision(err, "sha256:abc")
	if !errors.As(stamped, &rejected) || rejected.Revision != "sha256:abc" {
		t.Fatalf("withRuntimeRecordRevision did not stamp the revision: %v", stamped)
	}
	for _, want := range []string{"objective_rescope_widens_current", "sha256:abc"} {
		if !strings.Contains(stamped.Error(), want) {
			t.Errorf("message %q omits %q", stamped.Error(), want)
		}
	}
}

// TestWithRuntimeRecordRevisionLeavesOtherErrorsAlone pins that the stamping
// helper never rewrites an unrelated error.
func TestWithRuntimeRecordRevisionLeavesOtherErrorsAlone(t *testing.T) {
	original := errors.New("some other failure")
	if got := withRuntimeRecordRevision(original, "sha256:abc"); got != original {
		t.Errorf("unrelated error was rewritten: %v", got)
	}
	if withRuntimeRecordRevision(nil, "sha256:abc") != nil {
		t.Error("nil was turned into an error")
	}
}

// TestReplayPathHasNoBespokeRefusals is the guard that keeps the taxonomy
// collapsed. Every rejection on the replay path must go through the single
// constructor, so a future change cannot reintroduce a one-off message.
func TestReplayPathHasNoBespokeRefusals(t *testing.T) {
	content, err := os.ReadFile("runtime_ledger.go")
	if err != nil {
		t.Fatalf("read ledger source: %v", err)
	}
	lines := strings.Split(string(content), "\n")

	replayOwned := regexp.MustCompile(`^func (applyRuntime\w+|validateRuntimeRecordShape|validateRuntimeBeginEvent)\(`)
	bespoke := regexp.MustCompile(`errors\.New\("`)

	inReplay, offenders := false, 0
	for index, line := range lines {
		if strings.HasPrefix(line, "func ") {
			inReplay = replayOwned.MatchString(line)
		}
		if inReplay && bespoke.MatchString(line) {
			offenders++
			t.Errorf("runtime_ledger.go:%d carries a bespoke replay refusal; use rejectRuntimeRecord: %s", index+1, strings.TrimSpace(line))
		}
	}
	if offenders == 0 {
		t.Log("every replay-path rejection goes through the single typed refusal")
	}
}

// isRuntimeRecordRejection reports whether err is the collapsed refusal for the
// named condition. Tests assert the condition rather than message text, which
// is the whole point of carrying it as data.
func isRuntimeRecordRejection(err error, condition string) bool {
	var rejected *RuntimeRecordRejectedError
	return errors.As(err, &rejected) && rejected.Condition == condition
}
