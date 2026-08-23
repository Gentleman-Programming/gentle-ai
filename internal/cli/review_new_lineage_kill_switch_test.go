package cli

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is coverage closure for spec rdd-new-lineage-activation ->
// "Kill-Switch-Off Is Structurally Unfailable and Creates Nothing" -> "Kill
// switch off produces no side effect". Prior coverage proved this only for
// the unwired OfferReviewAfterVerify (review_offer_test.go); it was never
// proven for the facade at its five observed gate call sites. No production
// behavior changes here — this is a new test only.

// snapshotAuthorityTree returns a canonical, comparable representation of
// every regular file under root — relative path plus exact byte content —
// so two snapshots are identical if and only if nothing anywhere in the
// subtree changed. A directory-existence check alone would miss a write
// into an already-existing sibling record (e.g. the rdd-mode override this
// same fixture creates by disabling the kill switch); walking the whole
// tree and comparing bytes is the only assertion strong enough to prove
// "zero side effect" rather than "no new top-level entry".
func snapshotAuthorityTree(t *testing.T, root string) string {
	t.Helper()
	if _, statErr := os.Stat(root); os.IsNotExist(statErr) {
		return ""
	}
	type record struct {
		relative string
		payload  []byte
	}
	var records []record
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		records = append(records, record{relative: relative, payload: payload})
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].relative < records[j].relative })
	var buf bytes.Buffer
	for _, r := range records {
		buf.WriteString(r.relative)
		buf.WriteByte('\n')
		buf.Write(r.payload)
		buf.WriteString("\n---\n")
	}
	return buf.String()
}

// TestNewLineageKillSwitchOffProducesZeroSideEffectsAcrossEntrySurfaces
// drives every new-lineage-adjacent read surface — all five `review
// validate` gates plus OfferReviewAfterVerify's own guard path — with the
// kill switch off, twice against the identical fixture (same-fixture
// double-eval), and proves the entire
// .git/gentle-ai subtree is byte-identical before and after each pass.
func TestNewLineageKillSwitchOffProducesZeroSideEffectsAcrossEntrySurfaces(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("kill-switch-off fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	disableReviewForClone(t, repo)

	authorityRoot := filepath.Join(repo, ".git", "gentle-ai")
	runFullPass := func(pass string) {
		before := snapshotAuthorityTree(t, authorityRoot)
		for _, gate := range []reviewtransaction.GateKind{
			reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit, reviewtransaction.GatePrePush,
			reviewtransaction.GatePrePR, reviewtransaction.GateRelease,
		} {
			// No explicit --lineage: the ordinary hook-invoked shape every
			// one of the "observed call sites" the spec scenario names
			// actually uses (an explicit --lineage is a maintainer
			// diagnostic override with its own, unrelated raw-error
			// behavior for an unknown id, proven identical whether the kill
			// switch is on or off — not part of this scenario).
			var output bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output); err != nil {
				t.Fatalf("%s: gate %q produced an error while the kill switch is off: %v\n%s", pass, gate, err, output.String())
			}
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
				t.Fatalf("%s: gate %q delivery = %q, want %q\n%s", pass, gate, result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged, output.String())
			}
			if result.Allowed || result.Result == reviewtransaction.GateAllow {
				t.Fatalf("%s: gate %q fabricated an approval while disabled: %#v", pass, gate, result)
			}
		}

		offer, offerErr := reviewtransaction.OfferReviewAfterVerify(context.Background(), repo, reviewtransaction.OfferRequest{LineageID: "kill-switch-off-lineage"})
		if offerErr != nil {
			t.Fatalf("%s: OfferReviewAfterVerify produced an error while the kill switch is off: %v", pass, offerErr)
		}
		if offer.Available {
			t.Fatalf("%s: OfferReviewAfterVerify(kill switch off) = %#v, want Available=false", pass, offer)
		}

		after := snapshotAuthorityTree(t, authorityRoot)
		if before != after {
			t.Fatalf("%s: .git/gentle-ai changed while the kill switch was off:\nbefore:\n%s\nafter:\n%s", pass, before, after)
		}
	}

	runFullPass("first pass")
	runFullPass("second pass (same fixture, double-eval)")
}
