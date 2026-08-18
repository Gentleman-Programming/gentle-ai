package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestManagedReviewerAssetProvenanceAuthorityBoundary(t *testing.T) {
	for name, run := range map[string]func(*testing.T, string, string) (bool, error){
		"read-only recovery": func(t *testing.T, home, repo string) (bool, error) {
			staleManagedReviewerAssets(t, home)
			return false, errors.Join(RunReview([]string{"status", "--cwd", repo}, &bytes.Buffer{}), RunReview([]string{"capabilities"}, &bytes.Buffer{}), RunReview([]string{"start", "--help"}, &bytes.Buffer{}), RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &bytes.Buffer{}))
		},
		"capture evidence repository context": func(t *testing.T, home, repo string) (bool, error) {
			writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)
			started := runNegotiatedReviewStart(t, repo, "provenance-capture")
			input := filepath.Join(t.TempDir(), "evidence.txt")
			requireManagedAssetProvenanceNoError(t, os.WriteFile(input, []byte("passed\n"), 0o600))
			staleManagedReviewerAssets(t, home)
			return true, RunReviewCaptureEvidence([]string{"--repository-context", started.RepositoryContext.Handle, "--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity, "--expected-revision", started.RepositoryContext.Revision, "--outcome", "passed", "--input", input}, &bytes.Buffer{})
		},
		"abandon": func(t *testing.T, home, repo string) (bool, error) {
			staleManagedReviewerAssets(t, home)
			return true, RunReviewAbandon([]string{"--cwd", repo, "--lineage", "lineage", "--expected-revision", "revision", "--reason", "reason", "--actor", "actor", "--maintainer-authorization", "authorization"}, &bytes.Buffer{})
		},
		"bundle import": func(t *testing.T, home, repo string) (bool, error) {
			staleManagedReviewerAssets(t, home)
			return true, RunReviewBundleImport([]string{"--cwd", repo, "--bundle", filepath.Join(t.TempDir(), "bundle.json")}, &bytes.Buffer{})
		},
		"receipt-backed delivery": func(t *testing.T, home, repo string) (bool, error) {
			approveDiscoveryMarkdown(t, repo, "provenance-delivery", "docs/review.md", "reviewed\n")
			staleManagedReviewerAssets(t, home)
			return true, RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &bytes.Buffer{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			home, repo := reviewEnabledHome(t), initReviewCLIRepo(t)
			refuse, err := run(t, home, repo)
			if refuse && (err == nil || !bytes.Contains([]byte(err.Error()), []byte(managedAssetProvenanceRefusal))) {
				t.Fatalf("authority bypass error = %v, want %q", err, managedAssetProvenanceRefusal)
			}
			if !refuse && err != nil {
				t.Fatalf("recovery surface refused: %v", err)
			}
		})
	}
	home := t.TempDir()
	requireManagedAssetProvenanceNoError(t, state.Write(home, state.InstallState{ManagedAssetDigest: "sha256:previous-writer"}))
	decoy := filepath.Join(home, ".config", "opencode", "plugins", "opencode-review-transport.ts")
	requireManagedAssetProvenanceNoError(t, os.MkdirAll(filepath.Dir(decoy), 0o755))
	requireManagedAssetProvenanceNoError(t, os.WriteFile(decoy, []byte(assets.MustRead("opencode/plugins/opencode-review-transport.ts")), 0o644))
	requireManagedAssetProvenanceNoError(t, os.WriteFile(filepath.Join(home, ".config", "opencode", "opencode.json"), []byte("{\n"), 0o644))
	result, err := RunSyncWithSelection(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentGGA, model.ComponentSDD}, SDDMode: model.SDDModeSingle})
	if err == nil || len(result.Execution.Apply.Steps) < 3 || result.Execution.Apply.Steps[1].Status != "succeeded" {
		t.Fatalf("partial sync result = %#v, %v", result, err)
	}
	persisted, readErr := state.Read(home)
	if _, err := os.Stat(decoy); err != nil || readErr != nil || persisted.ManagedAssetDigest != "sha256:previous-writer" {
		t.Fatalf("decoy plugin bypassed provenance: stat=%v state=%#v read=%v", err, persisted, readErr)
	}
}
func TestManagedReviewerAssetProvenanceReceiptOrdering(t *testing.T) {
	t.Run("corrupt compact receipt", func(t *testing.T) {
		home, repo := reviewEnabledHome(t), initReviewCLIRepo(t)
		_, store := approveDiscoveryMarkdown(t, repo, "provenance-corrupt", "docs/review.md", "reviewed\n")
		requireManagedAssetProvenanceNoError(t, os.WriteFile(store.ReceiptPath(), []byte("{"), 0o644))
		staleManagedReviewerAssets(t, home)
		requireManagedAssetProvenanceError(t, RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &bytes.Buffer{}), "complete review authority inventory is unavailable or corrupted")
	})
	t.Run("escalated compact receipt", func(t *testing.T) {
		home := reviewEnabledHome(t)
		repo, _, record := escalatedCurrentChangesRecoveryFixture(t, "provenance-escalated")
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
		requireManagedAssetProvenanceNoError(t, err)
		receipt, err := record.State.Receipt()
		requireManagedAssetProvenanceNoError(t, err)
		requireManagedAssetProvenanceNoError(t, reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt))
		runReviewCLIGit(t, repo, "add", "tracked.txt")
		staleManagedReviewerAssets(t, home)
		requireManagedAssetProvenanceError(t, RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &bytes.Buffer{}), "compact review authority is escalated (budget_exceeded)")
	})
	t.Run("valid legacy receipt", func(t *testing.T) {
		home := reviewEnabledHome(t)
		fixture := newLegacyCLIFixture(t, "provenance-legacy")
		runReviewCLIGit(t, fixture.repo, "add", "tracked.txt")
		staleManagedReviewerAssets(t, home)
		requireManagedAssetProvenanceError(t, RunReviewFacadeValidate([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--gate", "pre-commit"}, &bytes.Buffer{}), managedAssetProvenanceRefusal)
	})
	t.Run("in-flight compact receipt", func(t *testing.T) {
		home, repo := reviewEnabledHome(t), initReviewCLIRepo(t)
		startNewLineageForFinalizeTest(t, repo, "provenance-inflight")
		staleManagedReviewerAssets(t, home)
		requireManagedAssetProvenanceError(t, RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", "provenance-inflight", "--gate", "pre-commit"}, &bytes.Buffer{}), reviewFacadeReceiptNotAvailableReason("provenance-inflight"))
	})
}

// TestManagedReviewerAssetProvenanceRefusesOnlyRecordedSkew pins the two
// shapes the refusal must NOT take. Only a recorded digest that disagrees is
// stale; a home that never installed anything has no managed assets to be
// stale, and refusing it would block every `go install` user from reviewing
// while telling them to run a sync that would fix nothing.
func TestManagedReviewerAssetProvenanceRefusesOnlyRecordedSkew(t *testing.T) {
	digest, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)

	for name, persist := range map[string]func(string){
		"no state file at all": func(string) {},
		"state without a recorded digest": func(home string) {
			requireManagedAssetProvenanceNoError(t, state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}}))
		},
		"digest matching this binary's assets": func(home string) {
			requireManagedAssetProvenanceNoError(t, state.Write(home, state.InstallState{ManagedAssetDigest: digest}))
		},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			persist(home)
			if err := authorizeManagedReviewerAssets(); err != nil {
				t.Fatalf("authorize = %v, want no refusal", err)
			}
		})
	}

	t.Run("recorded digest that disagrees", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		staleManagedReviewerAssets(t, home)
		requireManagedAssetProvenanceError(t, authorizeManagedReviewerAssets(), managedAssetProvenanceRefusal)
	})
}

func TestNegotiatedReviewStartClassifiesStaleManagedAssetsBeforeAuthority(t *testing.T) {
	home, repo := reviewEnabledHome(t), initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/stale-assets.md", "# Candidate\n", 0o644)
	staleManagedReviewerAssets(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--agent", "opencode", "--consent", "granted",
	}), &output)
	if err == nil {
		t.Fatalf("stale managed assets START succeeded: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != "review.start" || failure.Phase != "preflight" ||
		failure.Code != "managed_assets_outdated" || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.NextAction != "stop" {
		t.Fatalf("stale managed assets failure = %#v", failure)
	}
	if failure.Cause != managedAssetProvenanceRefusal {
		t.Fatalf("stale managed assets cause = %q, want %q", failure.Cause, managedAssetProvenanceRefusal)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("stale managed assets failure does not satisfy its published contract: %v", err)
	}
}

func TestNegotiatedReviewStartWithCurrentManagedAssetsStillStarts(t *testing.T) {
	home, repo := reviewEnabledHome(t), initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/current-assets.md", "# Candidate\n", 0o644)
	digest, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)
	recordManagedAssetDigest(t, home, digest)

	var output bytes.Buffer
	err = RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--agent", "opencode", "--consent", "granted",
	}), &output)
	if err != nil {
		t.Fatalf("current managed assets START refused: %v\n%s", err, output.String())
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if err := started.Validate(); err != nil || started.LineageID == "" {
		t.Fatalf("current managed assets START = %#v, validate = %v", started, err)
	}
}

func TestManagedAssetsPreflightDoesNotClassifyUnrelatedRuntimeRefusal(t *testing.T) {
	failure := newReviewIntegrationFailure("review.start", nil, errors.New("unrelated runtime refusal"))
	if failure.Code != "operation_outcome_unknown" || failure.Phase != "native_running" ||
		failure.MutationOutcome != ReviewMutationUnknown || failure.NextAction != "review.status" ||
		!strings.Contains(failure.Cause, "unrelated runtime refusal") {
		t.Fatalf("unrelated runtime failure = %#v", failure)
	}
}

// TestManagedAssetDigestIsStableAndAssetBound proves the digest is a property
// of the embedded assets rather than of the build, which is the whole reason
// it replaced the capabilities build identity: a rebuild that changes no asset
// must not invalidate an installation, and a test binary must be able to agree
// with a released one.
func TestManagedAssetDigestIsStableAndAssetBound(t *testing.T) {
	first, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)
	second, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)
	if first != second || first == "" {
		t.Fatalf("digest is not stable: %q then %q", first, second)
	}
	build, err := reviewCapabilitiesBuildIdentity(AppVersion)
	requireManagedAssetProvenanceNoError(t, err)
	if first == build.ID {
		t.Fatal("digest equals the build identity, so it still carries build metadata")
	}
}

// staleManagedReviewerAssets records an asset digest that disagrees with this
// binary's, which is the only skew the provenance guard refuses on.
//
// It reads the existing user state and rewrites only that one field. A blind
// state.Write would also erase the explicit global "on" these fixtures depend
// on -- receipt-driven development is opt-in, so wiping it would turn every
// following gate into a disabled/unmanaged report and the provenance refusal
// under test would never be reached.
func staleManagedReviewerAssets(t *testing.T, home string) {
	t.Helper()
	recordManagedAssetDigest(t, home, "sha256:stale")
}

// recordManagedAssetDigest rewrites only the recorded managed-asset digest,
// preserving every other opinion already persisted in the user's state.
func recordManagedAssetDigest(t *testing.T, home, digest string) {
	t.Helper()
	// A home with nothing persisted yet is an ordinary starting point here, so
	// an absent state file seeds an empty one rather than failing the fixture.
	persisted, err := state.Read(home)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	persisted.ManagedAssetDigest = digest
	requireManagedAssetProvenanceNoError(t, state.Write(home, persisted))
}
func requireManagedAssetProvenanceNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func requireManagedAssetProvenanceError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte(want)) {
		t.Fatalf("delivery error = %v, want %q", err, want)
	}
}
