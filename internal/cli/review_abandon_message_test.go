package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestReviewAbandonRefusalTerminatesInASuccessfulAbandon consumes the refusal
// the product actually emits and nothing else. The command the refusal names
// is run verbatim through the real router and must abandon the lineage: no
// wording is asserted, the only claim under test is that a consumer who
// follows the message reaches a terminal state.
//
// The lineage is deliberately stale and partially captured, the case the
// abandon help advertises as still abandonable.
func TestReviewAbandonRefusalTerminatesInASuccessfulAbandon(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, _, _, _ := partiallyCapturedReview(t)

	var refused bytes.Buffer
	err := RunReview([]string{"abandon", "--cwd", repo}, &refused)
	if err == nil {
		t.Fatalf("review abandon without a lineage succeeded: %s", refused.String())
	}
	argv := reviewCommandNamedBy(t, err.Error())

	var abandoned bytes.Buffer
	if err := RunReview(argv[2:], &abandoned); err != nil {
		t.Fatalf("the refusal's own command does not abandon: %v\nmessage:\n%s", err, err.Error())
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, abandoned.Bytes(), &result)
	if result.Record.Status != reviewtransaction.CompactReclaimCommitted || result.Record.LineageID != lineage {
		t.Fatalf("abandon record = %#v, want committed %q", result.Record, lineage)
	}
}

// reviewCommandNamedBy requires the message to name exactly one runnable
// gentle-ai command and returns its argv. A message that names no command, or
// names more than one so the reader must guess, fails here.
func reviewCommandNamedBy(t *testing.T, message string) []string {
	t.Helper()
	named := []string{}
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "gentle-ai review ") {
			named = append(named, strings.TrimSpace(line))
		}
	}
	if len(named) != 1 {
		t.Fatalf("refusal names %d runnable commands, want exactly one:\n%s", len(named), message)
	}
	argv := strings.Fields(named[0])
	for index := range argv {
		argv[index] = strings.Trim(argv[index], `"`)
	}
	return argv
}
