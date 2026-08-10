package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// WU2 (Wave 7 S6, design decision 4): byte-equivalence exit evidence, Commit
// A. Before the GENTLE_AI_RDD_NEW_LINEAGE switch and its legacy start branch
// are deleted (WU18, Commit B), the wave must prove a switch-ON build and a
// switch-free build produce byte-identical goldens/envelopes/receipts across
// the full journey set. This file records Commit A's baseline: the switch-ON
// bytes for the unnegotiated `review start` -> `review finalize` ->
// `review status` -> all-five-gates journey, using a fully deterministic
// fixture (fixed tracked content, fixed lineage name) so the resulting Git
// tree/blob hashes -- and therefore every downstream digest except the
// path-bound authority revision, which the comparison normalizes (see
// byteEquivalenceCommitARevisionPlaceholder) -- are byte-stable across
// machines and runs. WU18 re-runs the identical fixture switch-free and
// diffs against these exact golden files under the same normalization; any
// divergence is a defect signal (design decision 4), never a golden-update
// task.
//
// Scope note (disclosed deviation from the design's full "every entry
// surface" list): this Commit A only records the unnegotiated start form.
// The negotiated form (--contract/--target/--projection) and the
// capture-result surface are exercised at the unit level elsewhere
// (new_lineage_capture_test.go) and by WU18's own `bench --axis all`
// journey run, which is the stronger, real-binary proof for those surfaces;
// recording a second, harder-to-construct golden here for the same
// underlying code path would not add exit-evidence value proportional to
// the add-only budget.
var updateReviewByteEquivalenceCommitAGolden = flag.Bool("update-byte-equivalence-commit-a", false, "update Wave 7 S6 Commit A byte-equivalence golden files")

const byteEquivalenceCommitALineage = "byte-equivalence-commit-a-lineage"

// byteEquivalenceCommitARevisionPlaceholder replaces the one path-dependent
// digest in the recorded goldens. RepositoryIdentity (repository_locator.go)
// deliberately binds to the repository's absolute filesystem path (its CAS
// locking identity), so authority_revision -- surfaced as
// `authority_revision` by finalize and echoed as `store_revision` by every
// gate -- can never be byte-stable across machines (issue #2476: os.TempDir()
// differs per OS) and a shared fixed path makes concurrent test runs delete
// each other's fixture (issue #2483). Every OTHER byte of every golden is
// path-independent and stays under exact byte comparison. The excluded digest
// keeps two real assertions instead of the byte diff: it must be a
// well-formed sha256 digest, and only the exact value captured from THIS
// run's finalize receipt is substituted, so status and all five gates still
// fail loudly unless they echo that same revision (the receipt-binding
// property). WU18's Commit B re-evaluation diffs the identical fixture
// switch-free under this same normalization; divergence in any
// path-independent byte remains a defect signal, never a golden-update task.
const byteEquivalenceCommitARevisionPlaceholder = "sha256:{{path-dependent-authority-revision}}"

var byteEquivalenceCommitARevisionPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// byteEquivalenceCommitAFixturePath is a fresh per-test directory. The
// repository's absolute path feeds only the authority revision digest, which
// the golden comparison normalizes (see
// byteEquivalenceCommitARevisionPlaceholder), so a randomized path no longer
// breaks byte stability -- and it removes the shared fixed path that let
// concurrent runs destroy each other's fixture.
func byteEquivalenceCommitAFixturePath(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// byteEquivalenceCommitAFixture creates the exact deterministic candidate
// (same tracked content and commit identity every run) that this file's
// goldens are derived from, and finalizes it to an approved, receipted
// tier-0 (RiskLow) new-lineage authority -- the terminal state every one of
// the five gates requires to ever allow. It returns the repository path and
// the authority revision digest captured from the finalize receipt; every
// later surface must echo exactly that digest for its golden to match.
func byteEquivalenceCommitAFixture(t *testing.T) (string, string) {
	t.Helper()
	reviewModeHome(t)
	repo := byteEquivalenceCommitAFixturePath(t)
	runReviewCLIGit(t, repo, "init", "-q")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
	runReviewCLIGit(t, repo, "config", "core.autocrlf", "false")
	// A fixed GIT_AUTHOR_DATE/GIT_COMMITTER_DATE keeps the genesis commit --
	// and everything RepositoryIdentity derives from it -- byte-identical
	// across runs; git commit's default (wall-clock "now") would otherwise
	// make every downstream hash non-reproducible.
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "base")

	path := filepath.Join(repo, "docs", "byte-equivalence-commit-a.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("byte-equivalence commit A fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/byte-equivalence-commit-a.md")

	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")
	t.Cleanup(func() { os.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "") })

	var startOut bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", byteEquivalenceCommitALineage}, &startOut); err != nil {
		t.Fatalf("byte-equivalence fixture start: %v\n%s", err, startOut.String())
	}
	assertByteEquivalenceCommitAGolden(t, filepath.Join("testdata", "byte_equivalence_commit_a", "start.golden.json"), startOut.Bytes(), "")

	var finalizeOut bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", byteEquivalenceCommitALineage}, &finalizeOut); err != nil {
		t.Fatalf("byte-equivalence fixture finalize: %v\n%s", err, finalizeOut.String())
	}
	revision := byteEquivalenceCommitAReceiptRevision(t, finalizeOut.Bytes())
	assertByteEquivalenceCommitAGolden(t, filepath.Join("testdata", "byte_equivalence_commit_a", "finalize.golden.json"), finalizeOut.Bytes(), revision)

	return repo, revision
}

// byteEquivalenceCommitAReceiptRevision extracts and validates the authority
// revision digest from the finalize response. This is the one value the
// golden comparison normalizes, so it must at least prove it is a
// well-formed sha256 digest before it earns the placeholder.
func byteEquivalenceCommitAReceiptRevision(t *testing.T, finalizeResponse []byte) string {
	t.Helper()
	var response struct {
		Receipt struct {
			AuthorityRevision string `json:"authority_revision"`
		} `json:"receipt"`
	}
	if err := json.Unmarshal(finalizeResponse, &response); err != nil {
		t.Fatalf("decode finalize response for authority revision: %v\n%s", err, finalizeResponse)
	}
	revision := response.Receipt.AuthorityRevision
	if !byteEquivalenceCommitARevisionPattern.MatchString(revision) {
		t.Fatalf("finalize receipt authority_revision = %q, want sha256:<64 hex>", revision)
	}
	return revision
}

// TestReviewByteEquivalenceCommitAUnnegotiatedStartAndFinalize proves the
// start/finalize response bytes are byte-stable now, at Commit A time.
func TestReviewByteEquivalenceCommitAUnnegotiatedStartAndFinalize(t *testing.T) {
	_, _ = byteEquivalenceCommitAFixture(t)
}

// TestReviewByteEquivalenceCommitAStatus proves `review status`'s response
// bytes for the terminal (approved, receipted) lineage are byte-stable.
func TestReviewByteEquivalenceCommitAStatus(t *testing.T) {
	repo, revision := byteEquivalenceCommitAFixture(t)

	var statusOut bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--lineage", byteEquivalenceCommitALineage}, &statusOut); err != nil {
		t.Fatalf("byte-equivalence fixture status: %v\n%s", err, statusOut.String())
	}
	assertByteEquivalenceCommitAGolden(t, filepath.Join("testdata", "byte_equivalence_commit_a", "status.golden.json"), statusOut.Bytes(), revision)
}

func TestReviewByteEquivalenceCommitAGatePostApply(t *testing.T) {
	assertReviewByteEquivalenceCommitAGate(t, reviewtransaction.GatePostApply)
}

func TestReviewByteEquivalenceCommitAGatePreCommit(t *testing.T) {
	assertReviewByteEquivalenceCommitAGate(t, reviewtransaction.GatePreCommit)
}

func TestReviewByteEquivalenceCommitAGatePrePush(t *testing.T) {
	assertReviewByteEquivalenceCommitAGate(t, reviewtransaction.GatePrePush)
}

func TestReviewByteEquivalenceCommitAGatePrePR(t *testing.T) {
	assertReviewByteEquivalenceCommitAGate(t, reviewtransaction.GatePrePR)
}

func TestReviewByteEquivalenceCommitAGateRelease(t *testing.T) {
	assertReviewByteEquivalenceCommitAGate(t, reviewtransaction.GateRelease)
}

func assertReviewByteEquivalenceCommitAGate(t *testing.T, gate reviewtransaction.GateKind) {
	t.Helper()
	repo, revision := byteEquivalenceCommitAFixture(t)

	args := []string{"--cwd", repo, "--lineage", byteEquivalenceCommitALineage, "--gate", string(gate)}
	if gate == reviewtransaction.GateRelease {
		// The release gate additionally requires the five release-evidence
		// artifacts; their content (never their absolute path) is what
		// feeds any downstream digest, so fixed content keeps the golden
		// byte-stable regardless of t.TempDir()'s random path.
		dir := t.TempDir()
		releaseArtifact := func(name, content string) string {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}
		args = append(args,
			"--release-configuration", releaseArtifact("configuration.txt", "byte-equivalence commit A release configuration\n"),
			"--release-generated", releaseArtifact("generated.txt", "byte-equivalence commit A release generated\n"),
			"--release-provenance", releaseArtifact("provenance.txt", "byte-equivalence commit A release provenance\n"),
			"--release-publication-boundary", releaseArtifact("publication-boundary.txt", "byte-equivalence commit A release publication boundary\n"),
			"--release-evidence-freshness", releaseArtifact("evidence-freshness.txt", "byte-equivalence commit A release evidence freshness\n"),
		)
	}

	var gateOut bytes.Buffer
	if err := RunReviewFacadeValidate(args, &gateOut); err != nil {
		t.Fatalf("byte-equivalence fixture gate %s: %v\n%s", gate, err, gateOut.String())
	}
	assertReviewGateResult(t, gateOut.Bytes(), reviewtransaction.GateAllow)
	assertByteEquivalenceCommitAGolden(t, filepath.Join("testdata", "byte_equivalence_commit_a", "gate-"+string(gate)+".golden.json"), gateOut.Bytes(), revision)
}

// assertByteEquivalenceCommitAGolden is the identical -update convention
// review_new_lineage_switch_off_golden_test.go already established
// (`reviewNewLineageSwitchOffGolden`), reused under a distinct flag so
// regenerating Commit A's baseline is a deliberate, separately-named action
// from regenerating the switch-off goldens. A non-empty revision is the
// digest captured from this run's finalize receipt: exactly that value is
// replaced with byteEquivalenceCommitARevisionPlaceholder before comparison,
// so a surface that emits any OTHER revision still diverges byte-for-byte.
func assertByteEquivalenceCommitAGolden(t *testing.T, path string, got []byte, revision string) {
	t.Helper()
	normalized := append(bytes.TrimRight(got, "\n"), '\n')
	if revision != "" {
		normalized = bytes.ReplaceAll(normalized, []byte(revision), []byte(byteEquivalenceCommitARevisionPlaceholder))
	}
	if *updateReviewByteEquivalenceCommitAGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, normalized, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Commit A byte-equivalence golden %s: %v (regenerate with -update-byte-equivalence-commit-a)", path, err)
	}
	if !bytes.Equal(normalized, want) {
		t.Fatalf("Commit A byte-equivalence output diverged from %s:\ngot:\n%s\nwant:\n%s", path, normalized, want)
	}
}
