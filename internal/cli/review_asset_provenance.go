package cli

import (
	"fmt"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

const managedAssetProvenanceRefusal = "managed reviewer assets are outdated; run `gentle-ai sync`"

// managedAssetProvenanceError carries the halves of the provenance refusal a
// caller needs to actually converge (#3848): the digest this binary expects,
// the digest state.json recorded, and which executable is doing the comparing.
// Two different gentle-ai binaries share the one home-scoped digest, and each
// compares against its OWN embedded assets, so a sync run by any other binary
// can never satisfy this one's preflight — the remediation must bind the sync
// to this exact executable, and the bare refusal text used to hide that.
type managedAssetProvenanceError struct {
	// ExpectedDigest is this binary's assets.ManagedDigest(); empty when the
	// digest itself could not be derived.
	ExpectedDigest string
	// PersistedDigest is the managed_asset_digest state.json recorded. The
	// refusal only fires on a recorded digest that disagrees, so it is never
	// empty here.
	PersistedDigest string
	// BinaryVersion is the running binary's own version (`gentle-ai --version`).
	BinaryVersion string
	// BinaryPath is os.Executable() for the running binary; empty when the
	// path could not be resolved.
	BinaryPath string
}

// managedAssetSyncInvocation is the exact sync command that converges THIS
// binary's preflight: the same executable that is refusing, not whichever
// gentle-ai happens to be first on PATH.
func (err *managedAssetProvenanceError) managedAssetSyncInvocation() string {
	if err.BinaryPath == "" {
		return "gentle-ai sync"
	}
	return err.BinaryPath + " sync"
}

// remediation names the same-binary requirement as one runnable sentence. It
// is the additive envelope field and the tail of the operator error line.
func (err *managedAssetProvenanceError) remediation() string {
	return "the sync must be run by this same binary: " + err.managedAssetSyncInvocation()
}

// Error keeps the historical refusal sentence as its stable prefix (existing
// callers and clients match on it) and appends the convergence diagnostics,
// single-line so the bounded envelope `cause` never truncates it at a newline.
// Paths sit inside backticks so the envelope's path-redacting privacy gate
// replaces exactly the path token without swallowing surrounding punctuation.
func (err *managedAssetProvenanceError) Error() string {
	expected := err.ExpectedDigest
	if expected == "" {
		expected = "underivable"
	}
	path := err.BinaryPath
	if path == "" {
		path = "an unresolvable executable path"
	}
	return fmt.Sprintf("%s (expected digest %s for this binary, persisted digest %s in state.json; this binary is gentle-ai %s at `%s`); the sync must be run by this same binary: `%s`",
		managedAssetProvenanceRefusal, expected, err.PersistedDigest, err.BinaryVersion, path, err.managedAssetSyncInvocation())
}

// managedAssetDigest returns a content digest of the managed assets this
// binary embeds. It deliberately does NOT use the review capabilities build
// identity: that identity carries vcs.revision, module version, and ldflags,
// so two binaries built from the same source disagree even when they embed
// byte-identical assets. Comparing build identities would therefore declare
// perfectly current assets outdated after any rebuild, and a test binary
// (which Go does not stamp with VCS settings) could never agree with a
// released one. The digest compares the thing #2685 is actually about.
func managedAssetDigest() (string, error) {
	digest, err := assets.ManagedDigest()
	if err != nil {
		return "", fmt.Errorf("derive managed asset digest: %w", err)
	}
	return digest, nil
}

// authorizeManagedReviewerAssets refuses review work when installed managed
// assets were written by a different set of embedded assets than this binary
// carries.
//
// It refuses ONLY on a recorded digest that disagrees. An absent state file,
// or one with no digest recorded, means no managed assets were ever installed
// here, so there is nothing stale to protect against and nothing this refusal
// could tell the caller to repair. Refusing that shape would block every user
// who never ran `gentle-ai install` from reviewing at all.
func authorizeManagedReviewerAssets() error {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return nil
	}
	persisted, err := state.Read(homeDir)
	if err != nil || persisted.ManagedAssetDigest == "" {
		return nil
	}
	// The scalar names the binary that last wrote the asset files; the set
	// names every binary a zero-agent sync additionally converged (#3848).
	// Either grants this binary's preflight, so two binaries sharing one
	// home stop revoking each other. The refusal keeps reporting the scalar
	// as persisted_digest — it is the writer identity the diagnostics name.
	digest, digestErr := managedAssetDigest()
	if digestErr != nil || (persisted.ManagedAssetDigest != digest && !slices.Contains(persisted.ManagedAssetDigests, digest)) {
		refusal := &managedAssetProvenanceError{
			PersistedDigest: persisted.ManagedAssetDigest,
		}
		if digestErr == nil {
			refusal.ExpectedDigest = digest
		}
		refusal.BinaryVersion, _ = reviewGentleAIVersionAndCommit()
		if path, pathErr := reviewCapabilitiesExecutablePath(); pathErr == nil {
			refusal.BinaryPath = path
		}
		return refusal
	}
	return nil
}
