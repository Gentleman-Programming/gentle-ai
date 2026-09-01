package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestManagedReviewerAssetProvenanceAuthorityBoundary(t *testing.T) {
	for name, run := range map[string]func(*testing.T, string, string) (bool, error){
		"read-only recovery": func(t *testing.T, home, repo string) (bool, error) {
			staleManagedReviewerAssets(t, home)
			return false, errors.Join(RunReview([]string{"status", "--cwd", repo}, &bytes.Buffer{}), RunReview([]string{"capabilities"}, &bytes.Buffer{}), RunReview([]string{"start", "--help"}, &bytes.Buffer{}), RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &bytes.Buffer{}))
		},
		"abandon": func(t *testing.T, home, repo string) (bool, error) {
			staleManagedReviewerAssets(t, home)
			return true, RunReviewAbandon([]string{"--cwd", repo, "--lineage", "lineage", "--expected-revision", "revision", "--reason", "reason", "--actor", "actor", "--maintainer-authorization", "authorization"}, &bytes.Buffer{})
		},
		"bundle import": func(t *testing.T, home, repo string) (bool, error) {
			staleManagedReviewerAssets(t, home)
			return true, RunReviewBundleImport([]string{"--cwd", repo, "--bundle", filepath.Join(t.TempDir(), "bundle.json")}, &bytes.Buffer{})
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

// TestZeroAgentSyncConvergesManagedAssetProvenance pins issue #3848 hole 1: a
// sync that discovers zero agents used to return its no-op before the managed
// asset persistence step, so a stale recorded digest survived a successful
// `gentle-ai sync` and the START preflight refused forever with the exact
// remediation it had already run. The zero-agent no-op must converge this
// binary additively: its digest joins managed_asset_digests while the scalar
// stays whatever binary wrote it, so the very next preflight authorizes
// without revoking another binary's authorization.
func TestZeroAgentSyncConvergesManagedAssetProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	staleManagedReviewerAssets(t, home)

	result, err := RunSyncWithSelection(home, model.Selection{})
	if err != nil || !result.NoOp {
		t.Fatalf("zero-agent sync = NoOp %v, %v, want a clean no-op", result.NoOp, err)
	}
	digest, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)
	persisted, err := state.Read(home)
	requireManagedAssetProvenanceNoError(t, err)
	if persisted.ManagedAssetDigest != "sha256:stale" {
		t.Fatalf("zero-agent sync rewrote the scalar managed_asset_digest to %q, want the writer's %q preserved", persisted.ManagedAssetDigest, "sha256:stale")
	}
	if !slices.Contains(persisted.ManagedAssetDigests, digest) {
		t.Fatalf("zero-agent sync recorded set %v, want it to contain this binary's %q", persisted.ManagedAssetDigests, digest)
	}
	if err := authorizeManagedReviewerAssets(); err != nil {
		t.Fatalf("preflight after zero-agent sync = %v, want authorized", err)
	}
}

// TestZeroAgentSyncDoesNotClobberOtherBinaryDigest pins the two-binary
// regression: with binary B's digest in the scalar slot, a zero-agent sync by
// THIS binary must not overwrite it — otherwise B's next sync overwrites ours
// back and the two preflights revoke each other forever. The zero-agent sync
// converges additively: the scalar stays B's, this binary's digest joins the
// managed_asset_digests set (idempotently), and both preflights authorize.
func TestZeroAgentSyncDoesNotClobberOtherBinaryDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	recordManagedAssetDigest(t, home, "sha256:other-binary")

	for _, pass := range []string{"first", "idempotent second"} {
		result, err := RunSyncWithSelection(home, model.Selection{})
		if err != nil || !result.NoOp {
			t.Fatalf("%s zero-agent sync = NoOp %v, %v, want a clean no-op", pass, result.NoOp, err)
		}
	}
	digest, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)
	persisted, err := state.Read(home)
	requireManagedAssetProvenanceNoError(t, err)
	// The scalar is binary B's authorization: B's own preflight compares its
	// digest against exactly this field, so it must survive our sync untouched.
	if persisted.ManagedAssetDigest != "sha256:other-binary" {
		t.Fatalf("zero-agent sync clobbered the other binary's scalar digest: %q", persisted.ManagedAssetDigest)
	}
	if !slices.Equal(persisted.ManagedAssetDigests, []string{digest}) {
		t.Fatalf("zero-agent sync recorded set %v, want exactly this binary's %q", persisted.ManagedAssetDigests, digest)
	}
	if err := authorizeManagedReviewerAssets(); err != nil {
		t.Fatalf("this binary's preflight after zero-agent sync = %v, want authorized via set membership", err)
	}
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
	// #3848: the cause keeps its historical sentence as a stable prefix and now
	// names both digests; the executable path is redacted by the envelope's
	// privacy gate, so the runnable same-binary invocation travels in the
	// dedicated managed_assets context instead.
	expected, digestErr := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, digestErr)
	if !strings.HasPrefix(failure.Cause, managedAssetProvenanceRefusal) ||
		!strings.Contains(failure.Cause, expected) || !strings.Contains(failure.Cause, "sha256:stale") {
		t.Fatalf("stale managed assets cause = %q, want prefix %q naming both digests", failure.Cause, managedAssetProvenanceRefusal)
	}
	if failure.Context == nil || failure.Context.ManagedAssets == nil ||
		failure.Context.ManagedAssets.ExpectedDigest != expected ||
		failure.Context.ManagedAssets.PersistedDigest != "sha256:stale" {
		t.Fatalf("stale managed assets context = %#v", failure.Context)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("stale managed assets failure does not satisfy its published contract: %v", err)
	}
	schema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
	validatePublishedReviewSchema(t, schema, output.Bytes())
}

// TestManagedAssetProvenanceRefusalCarriesConvergenceDiagnostics pins issue
// #3848 hole 2: two different gentle-ai binaries share the one home-scoped
// digest, and each compares against its OWN embedded assets, so a sync run by
// binary A can never satisfy binary B's preflight. The refusal must therefore
// name both digests and the running binary's identity, and its remediation
// must bind the sync to this exact executable.
func TestManagedAssetProvenanceRefusalCarriesConvergenceDiagnostics(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	staleManagedReviewerAssets(t, home)
	expected, err := managedAssetDigest()
	requireManagedAssetProvenanceNoError(t, err)
	binaryPath, err := reviewCapabilitiesExecutablePath()
	requireManagedAssetProvenanceNoError(t, err)

	refusal := authorizeManagedReviewerAssets()
	var skew *managedAssetProvenanceError
	if !errors.As(refusal, &skew) {
		t.Fatalf("stale refusal = %T %v, want *managedAssetProvenanceError", refusal, refusal)
	}
	if skew.ExpectedDigest != expected || skew.PersistedDigest != "sha256:stale" ||
		skew.BinaryVersion == "" || skew.BinaryPath != binaryPath {
		t.Fatalf("refusal diagnostics = %#v, want expected %q, persisted %q, this binary", skew, expected, "sha256:stale")
	}
	for _, want := range []string{managedAssetProvenanceRefusal, expected, "sha256:stale", skew.BinaryVersion, binaryPath, binaryPath + " sync"} {
		if !strings.Contains(refusal.Error(), want) {
			t.Fatalf("refusal error %q does not name %q", refusal.Error(), want)
		}
	}
	if strings.ContainsAny(refusal.Error(), "\r\n") {
		t.Fatalf("refusal error is not single-line: %q", refusal.Error())
	}

	failure := newReviewIntegrationFailure("review.start", []string{"--contract", ReviewIntegrationContractV2},
		reviewPreflightRefusal(reviewPreflightManagedAssetsReason, refusal))
	if failure.Code != "managed_assets_outdated" || failure.Context == nil || failure.Context.ManagedAssets == nil {
		t.Fatalf("stale managed assets envelope = %#v, want managed_assets context", failure)
	}
	managed := failure.Context.ManagedAssets
	if managed.ExpectedDigest != expected || managed.PersistedDigest != "sha256:stale" ||
		managed.BinaryVersion != skew.BinaryVersion || managed.BinaryPath != binaryPath ||
		!strings.Contains(managed.Remediation, binaryPath+" sync") {
		t.Fatalf("managed assets context = %#v", managed)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("managed assets envelope does not satisfy its published contract: %v", err)
	}

	// The v1 contract directory is frozen byte-unchanged, so its gate_context
	// cannot admit the new diagnostics: a legacy envelope keeps the enriched
	// cause and carries no context.
	legacy := newReviewIntegrationFailure("review.start", nil,
		reviewPreflightRefusal(reviewPreflightManagedAssetsReason, refusal))
	if legacy.Contract != ReviewIntegrationContractV1 || legacy.Context != nil ||
		!strings.HasPrefix(legacy.Cause, managedAssetProvenanceRefusal) {
		t.Fatalf("legacy managed assets envelope = %#v", legacy)
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy managed assets envelope does not satisfy its published contract: %v", err)
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
