package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// damageReviewAuthorityTree makes this clone's review-transactions tree fail
// RAR path safety the way #2882's reporter hit it: the directory is a symlink
// pointing outside the repository, so every ancestor walk over it refuses.
func damageReviewAuthorityTree(t *testing.T, repo string) {
	t.Helper()
	gentle := filepath.Join(repo, ".git", "gentle-ai")
	if err := os.MkdirAll(gentle, 0o700); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	authority := filepath.Join(gentle, "review-transactions")
	if err := os.RemoveAll(authority); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, authority); err != nil {
		t.Skipf("this platform cannot stage the unsafe authority path: %v", err)
	}
}

// TestKillSwitchIsReachableWhileReviewAuthorityIsDamaged pins the invariant the
// orchestrator contract states verbatim: the clone-scoped kill switch is
// "reachable even while review authority is broken".
//
// It was not. cloneLocalRDDModeRoot derived the override directory INSIDE the
// review authority tree, deliberately, to reuse that tree's path-safety and
// private-IO helpers. The cost is that the switch inherits every failure mode
// of the thing it exists to turn off. #2882's reporter found the exact
// consequence: with authority failing RAR path safety, `review mode disable
// --scope clone` exited non-zero without persisting, and the mode stayed on.
//
// This matters beyond one command. `gentle-ai review mode disable --scope
// clone` is the continuation the stop-reason table names for most
// unrecoverable review states, including corrupted_or_unverifiable_authority.
// Those codes were pointing at an exit that did not work in precisely the
// situation they point at it from.
func TestKillSwitchIsReachableWhileReviewAuthorityIsDamaged(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	damageReviewAuthorityTree(t, repo)

	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("clone-scoped disable failed while authority was damaged, which is the exact case the stop-reason table sends callers here for: %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOff ||
		result.Status.Source != reviewtransaction.RDDModeSourceCloneLocal {
		t.Fatalf("disable reported %#v, want the clone-local source to decide off", result.Status)
	}

	// The write has to survive a fresh resolution, not just report success
	// once: the reporter's disable printed a projection and then persisted
	// nothing, so a following status still said on.
	output.Reset()
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("read-only status failed while authority was damaged: %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOff {
		t.Fatalf("status after disable = %#v, want off; the override did not persist", result.Status)
	}
}

// TestKillSwitchStatusIsReadableWhileReviewAuthorityIsDamaged covers the
// read-only surface on its own. status is documented as strictly read-only and
// is how an operator finds out what governs delivery; refusing to project the
// global source because an unrelated authority tree is damaged tells them
// nothing they can act on.
func TestKillSwitchStatusIsReadableWhileReviewAuthorityIsDamaged(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	damageReviewAuthorityTree(t, repo)

	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("status refused on a damaged authority tree: %v", err)
	}
	// No clone-local override was ever written, so the default decides and
	// reviews stay on. Failing closed is preserved; only reachability changed.
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("status = %#v, want the default to still decide on", result.Status)
	}
}
