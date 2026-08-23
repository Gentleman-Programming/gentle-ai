//go:build legacy_compact_receipt

package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestReviewerContextLevelIsClosedOnInputAndOpenOnRead pins the asymmetry the
// whole forward-compatibility design rests on.
func TestReviewerContextLevelIsClosedOnInputAndOpenOnRead(t *testing.T) {
	tests := []struct {
		name              string
		level             ReviewerContextLevel
		accepted          bool
		wellFormed        bool
		declaredNowReason string
	}{
		{name: "provider command", level: ReviewerContextLevelProviderCommand, accepted: true, wellFormed: true},
		{name: "runtime interception", level: ReviewerContextLevelRuntimeInterception, accepted: true, wellFormed: true},
		{name: "unknown future mechanism", level: "signed_attestation", accepted: false, wellFormed: true,
			declaredNowReason: "a level this release cannot produce must not be declarable, but must stay readable"},
		{name: "empty", level: "", accepted: false, wellFormed: false},
		{name: "path shaped", level: "a/b", accepted: false, wellFormed: false},
		{name: "newline", level: "provider_command\nx", accepted: false, wellFormed: false},
		{name: "uppercase", level: "Provider_Command", accepted: false, wellFormed: false},
		{name: "trailing underscore", level: "provider_", accepted: false, wellFormed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ReviewerContextLevelAccepted(test.level); got != test.accepted {
				t.Fatalf("accepted(%q) = %v, want %v (%s)", test.level, got, test.accepted, test.declaredNowReason)
			}
			if got := ReviewerContextLevelWellFormed(test.level); got != test.wellFormed {
				t.Fatalf("wellFormed(%q) = %v, want %v", test.level, got, test.wellFormed)
			}
		})
	}
}

// TestCompactReceiptOmitsUnrecordedReviewerContextLevel proves the field is
// absent — not defaulted — when nothing recorded it. Absence is what every
// receipt issued before this field existed carries, so a re-derived receipt for
// such a lineage stays byte-identical and its immutable publication converges.
func TestCompactReceiptOmitsUnrecordedReviewerContextLevel(t *testing.T) {
	receipt := reviewerContextLevelReceipt(t, "")
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("reviewer_context_level")) {
		t.Fatalf("unrecorded level was serialized: %s", payload)
	}
	if receipt.ReviewerContextLevel != "" {
		t.Fatalf("unrecorded level = %q", receipt.ReviewerContextLevel)
	}
}

// TestCompactReceiptPreservesUnknownReviewerContextLevel is the forward
// compatibility contract: a receipt written by a later release that records a
// mechanism this binary has never heard of still parses, still validates, and
// round-trips byte-identically. Every other closed vocabulary on this receipt
// rejects its default case; this one must not, because a receipt is issued once
// and read for the rest of its life.
func TestCompactReceiptPreservesUnknownReviewerContextLevel(t *testing.T) {
	for _, level := range []ReviewerContextLevel{
		ReviewerContextLevelProviderCommand,
		ReviewerContextLevelRuntimeInterception,
		"a_mechanism_this_release_has_never_heard_of",
	} {
		t.Run(string(level), func(t *testing.T) {
			payload, err := json.Marshal(reviewerContextLevelReceipt(t, level))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(payload), `"reviewer_context_level":"`+string(level)+`"`) {
				t.Fatalf("receipt did not record the level verbatim: %s", payload)
			}
			parsed, err := ParseCompactReceipt(payload)
			if err != nil {
				t.Fatalf("older-reader parse of level %q failed: %v", level, err)
			}
			if parsed.ReviewerContextLevel != level {
				t.Fatalf("round-trip level = %q, want %q", parsed.ReviewerContextLevel, level)
			}
			again, err := json.Marshal(parsed)
			if err != nil {
				t.Fatal(err)
			}
			reparsed, err := ParseCompactReceipt(again)
			if err != nil {
				t.Fatalf("re-parse of level %q failed: %v", level, err)
			}
			if !reflect.DeepEqual(parsed, reparsed) {
				t.Fatalf("round-trip is not stable:\n%+v\n%+v", parsed, reparsed)
			}
		})
	}
}

// TestCompactReceiptRejectsMalformedReviewerContextLevel proves the open
// vocabulary is still a bounded one: unknown is readable, malformed is not.
func TestCompactReceiptRejectsMalformedReviewerContextLevel(t *testing.T) {
	receipt := reviewerContextLevelReceipt(t, "")
	receipt.ReviewerContextLevel = "not a level"
	if err := receipt.Validate(); err == nil {
		t.Fatal("malformed reviewer context level validated")
	}
}

func reviewerContextLevelReceipt(t *testing.T, level ReviewerContextLevel) CompactReceipt {
	t.Helper()
	receipt := CompactReceipt{
		Schema: CompactReceiptSchema, LineageID: "lens-context-receipt", Generation: 1,
		BaseTree:           strings.Repeat("a", 40),
		InitialReviewTree:  strings.Repeat("b", 40),
		FinalCandidateTree: strings.Repeat("b", 40),
		PathsDigest:        "sha256:" + strings.Repeat("1", 64),
		FixDeltaHash:       EmptyFixDeltaHash,
		PolicyHash:         "sha256:" + strings.Repeat("2", 64),
		EvidenceHash:       "sha256:" + strings.Repeat("3", 64),
		RiskLevel:          RiskLow, SelectedLenses: []string{}, ResolvedFindingIDs: []string{},
		TerminalState:        TerminalApproved,
		ReviewerContextLevel: level,
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("fixture receipt is invalid: %v", err)
	}
	return receipt
}
