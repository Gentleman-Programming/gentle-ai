package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// lensEmissionMigrationFixture separates one in-flight review from another at
// the same lens slot. Every field the reader enforces (lineage, target,
// revision, lens, order, subject) is part of the binding, so a test can prove
// the migration-window guard keys on binding presence rather than on the level
// alone.
type lensEmissionMigrationFixture struct {
	lineage string
	subject string
}

func lensEmissionMigrationForTest(level ReviewerContextLevel, revision string, fixture lensEmissionMigrationFixture) LensContextEmission {
	return LensContextEmission{
		Schema: LensContextEmissionSchema, LineageID: fixture.lineage,
		TargetIdentity:    "sha256:" + strings.Repeat("a", 64),
		AuthorityRevision: revision,
		Lens:              LensReliability, SelectedOrder: 0,
		SubjectHash: fixture.subject, Level: level,
	}
}

// TestProviderContractEmissionIsAdmittedAndRecorded pins SEN-RPC-11: a review
// admitted via the shared contract records provider_contract on emission, on
// discovery, and on the receipt. This is the D2 descriptor's happy path.
func TestProviderContractEmissionIsAdmittedAndRecorded(t *testing.T) {
	if !ReviewerContextLevelAccepted(ReviewerContextLevelProviderContract) {
		t.Fatal("provider_contract must be declarable once the shared contract implements it")
	}
	dir := t.TempDir()
	revision := "sha256:" + strings.Repeat("1", 64)
	review := lensEmissionMigrationFixture{lineage: "review-0123456789abcdef", subject: "sha256:" + strings.Repeat("b", 64)}
	emission := lensEmissionMigrationForTest(ReviewerContextLevelProviderContract, revision, review)
	if err := PublishLensContextEmission(dir, emission); err != nil {
		t.Fatalf("publish provider_contract emission: %v", err)
	}
	got, found := ReadLensContextEmission(dir, emission.LineageID, emission.TargetIdentity,
		revision, emission.Lens, emission.SelectedOrder, emission.SubjectHash)
	if !found || got.Level != ReviewerContextLevelProviderContract {
		t.Fatalf("recorded emission = %#v, found=%v; want level provider_contract", got, found)
	}
	if level := DiscoverReviewerContextLevel(dir, emission.LineageID, emission.TargetIdentity,
		revision, []string{emission.Lens}, []string{emission.SubjectHash}); level != ReviewerContextLevelProviderContract {
		t.Fatalf("discovered level = %q, want provider_contract", level)
	}
	// Cross-level: a receipt minted for this review records the descriptor and
	// stays valid and stable through a re-read.
	receipt := reviewerContextLevelReceipt(t, ReviewerContextLevelProviderContract)
	if err := receipt.Validate(); err != nil {
		t.Fatalf("receipt with provider_contract is invalid: %v", err)
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCompactReceipt(payload)
	if err != nil {
		t.Fatalf("parse receipt with provider_contract: %v", err)
	}
	if parsed.ReviewerContextLevel != ReviewerContextLevelProviderContract {
		t.Fatalf("receipt round-trip level = %q, want provider_contract", parsed.ReviewerContextLevel)
	}
}

// TestLegacyLevelsRemainReadableAndNeverRewritten pins SEN-RPC-12: historical
// provider_command/runtime_interception descriptors stay readable, stay valid
// on receipts, and are never rewritten — reading an emission must not touch its
// bytes or its modification time.
func TestLegacyLevelsRemainReadableAndNeverRewritten(t *testing.T) {
	dir := t.TempDir()
	review := lensEmissionMigrationFixture{lineage: "review-0123456789abcdef", subject: "sha256:" + strings.Repeat("b", 64)}
	for index, legacy := range []ReviewerContextLevel{
		ReviewerContextLevelProviderCommand, ReviewerContextLevelRuntimeInterception,
	} {
		t.Run(string(legacy), func(t *testing.T) {
			revision := "sha256:" + strings.Repeat(string(rune('0'+index)), 64)
			emission := lensEmissionMigrationForTest(legacy, revision, review)
			if err := PublishLensContextEmission(dir, emission); err != nil {
				t.Fatalf("publish legacy emission: %v", err)
			}
			path, err := lensContextEmissionPath(dir, revision, emission.Lens, emission.SelectedOrder)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			infoBefore, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			got, found := ReadLensContextEmission(dir, emission.LineageID, emission.TargetIdentity,
				revision, emission.Lens, emission.SelectedOrder, emission.SubjectHash)
			if !found || got.Level != legacy {
				t.Fatalf("legacy emission = %#v, found=%v; want level %q", got, found, legacy)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			infoAfter, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
				t.Fatalf("reading rewrote the legacy record: bytes equal=%v, mtime equal=%v", bytes.Equal(before, after), infoAfter.ModTime().Equal(infoBefore.ModTime()))
			}
			// Cross-level: the legacy descriptor survives on the receipt too.
			receipt := reviewerContextLevelReceipt(t, legacy)
			if err := receipt.Validate(); err != nil {
				t.Fatalf("receipt with legacy level %q is invalid: %v", legacy, err)
			}
			payload, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseCompactReceipt(payload)
			if err != nil {
				t.Fatalf("parse receipt with legacy level %q: %v", legacy, err)
			}
			if parsed.ReviewerContextLevel != legacy {
				t.Fatalf("receipt round-trip level = %q, want %q", parsed.ReviewerContextLevel, legacy)
			}
		})
	}
}

// TestMigrationWindowAcceptsProviderContractForLegacyNegotiatedReview pins
// SEN-RPC-13: a review negotiated pre-shim (a legacy level) that is still in
// flight is accepted when the shared contract emits at provider_contract — no
// conflict refusal — and the frozen legacy audit note is not rewritten.
func TestMigrationWindowAcceptsProviderContractForLegacyNegotiatedReview(t *testing.T) {
	review := lensEmissionMigrationFixture{lineage: "review-0123456789abcdef", subject: "sha256:" + strings.Repeat("b", 64)}
	for index, legacy := range []ReviewerContextLevel{
		ReviewerContextLevelProviderCommand, ReviewerContextLevelRuntimeInterception,
	} {
		t.Run(string(legacy), func(t *testing.T) {
			dir := t.TempDir()
			revision := "sha256:" + strings.Repeat(string(rune('0'+index)), 64)
			negotiated := lensEmissionMigrationForTest(legacy, revision, review)
			if err := PublishLensContextEmission(dir, negotiated); err != nil {
				t.Fatalf("legacy-negotiated emission: %v", err)
			}
			path, err := lensContextEmissionPath(dir, revision, negotiated.Lens, negotiated.SelectedOrder)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			infoBefore, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			// The shim emits at provider_contract for the same in-flight review.
			shared := lensEmissionMigrationForTest(ReviewerContextLevelProviderContract, revision, review)
			if err := PublishLensContextEmission(dir, shared); err != nil {
				t.Fatalf("migration-window emission was refused: %v", err)
			}
			// Admission proceeded without rewriting the audit note: the slot
			// still names the negotiated mechanism, byte for byte.
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			infoAfter, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
				t.Fatalf("migration-window acceptance rewrote the legacy record: bytes equal=%v, mtime equal=%v", bytes.Equal(before, after), infoAfter.ModTime().Equal(infoBefore.ModTime()))
			}
			got, found := ReadLensContextEmission(dir, review.lineage, negotiated.TargetIdentity,
				revision, negotiated.Lens, negotiated.SelectedOrder, review.subject)
			if !found || got.Level != legacy {
				t.Fatalf("slot record after migration = %#v, found=%v; want %q preserved", got, found, legacy)
			}
			// The receipt for this in-flight review reflects the audit slot: the
			// negotiated level is discovered, never invented.
			if level := DiscoverReviewerContextLevel(dir, review.lineage, negotiated.TargetIdentity,
				revision, []string{negotiated.Lens}, []string{review.subject}); level != legacy {
				t.Fatalf("discovered level after migration = %q, want %q", level, legacy)
			}
		})
	}
}

// TestRefusalStillFiresOnlyOnGenuinelyConflictingEmission pins SEN-RPC-14: the
// guard refuses on genuinely absent or conflicting emission — the reverse
// migration, and a different review occupying the same slot — never on level
// mismatch alone for a mid-flight review (which is the SEN-RPC-13 case above).
func TestRefusalStillFiresOnlyOnGenuinelyConflictingEmission(t *testing.T) {
	t.Run("reverse migration stays a conflict", func(t *testing.T) {
		dir := t.TempDir()
		revision := "sha256:" + strings.Repeat("4", 64)
		review := lensEmissionMigrationFixture{lineage: "review-0123456789abcdef", subject: "sha256:" + strings.Repeat("b", 64)}
		if err := PublishLensContextEmission(dir, lensEmissionMigrationForTest(ReviewerContextLevelProviderContract, revision, review)); err != nil {
			t.Fatal(err)
		}
		err := PublishLensContextEmission(dir, lensEmissionMigrationForTest(ReviewerContextLevelProviderCommand, revision, review))
		if !errors.Is(err, ErrLensContextEmissionConflict) {
			t.Fatalf("legacy emission over provider_contract = %v, want conflict", err)
		}
	})
	t.Run("different review at the same slot stays a conflict", func(t *testing.T) {
		dir := t.TempDir()
		revision := "sha256:" + strings.Repeat("5", 64)
		first := lensEmissionMigrationFixture{lineage: "review-0123456789abcdef", subject: "sha256:" + strings.Repeat("b", 64)}
		other := lensEmissionMigrationFixture{lineage: "review-fedcba9876543210", subject: "sha256:" + strings.Repeat("c", 64)}
		if err := PublishLensContextEmission(dir, lensEmissionMigrationForTest(ReviewerContextLevelProviderCommand, revision, first)); err != nil {
			t.Fatal(err)
		}
		err := PublishLensContextEmission(dir, lensEmissionMigrationForTest(ReviewerContextLevelProviderContract, revision, other))
		if !errors.Is(err, ErrLensContextEmissionConflict) {
			t.Fatalf("provider_contract emission for a different review = %v, want conflict", err)
		}
	})
	t.Run("absent emission records nothing", func(t *testing.T) {
		dir := t.TempDir()
		revision := "sha256:" + strings.Repeat("6", 64)
		review := lensEmissionMigrationFixture{lineage: "review-0123456789abcdef", subject: "sha256:" + strings.Repeat("b", 64)}
		if level := DiscoverReviewerContextLevel(dir, review.lineage, "sha256:"+strings.Repeat("a", 64),
			revision, []string{LensReliability}, []string{review.subject}); level != "" {
			t.Fatalf("absent emission recorded level %q; absence must mean not established", level)
		}
		// Absence is not a refusal on its own: a fresh slot records the shared
		// contract level normally.
		emission := lensEmissionMigrationForTest(ReviewerContextLevelProviderContract, revision, review)
		if err := PublishLensContextEmission(dir, emission); err != nil {
			t.Fatalf("fresh provider_contract emission on an absent slot: %v", err)
		}
	})
}
