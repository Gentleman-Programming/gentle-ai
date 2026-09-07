package cli

import (
	"errors"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

const managedAssetProvenanceRefusal = "managed reviewer assets are outdated; run `gentle-ai sync`"

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

// managedAssetProvenance is the raw digest comparison behind
// authorizeManagedReviewerAssets, exposed separately so a stale refusal can
// name which recorded asset is stale (#3299, #4170) without duplicating the
// comparison. diagnosed is false in exactly the shapes
// authorizeManagedReviewerAssets already treats as "nothing to refuse": no
// resolvable home directory, no persisted state, or no recorded digest.
type managedAssetProvenance struct {
	installedDigest string
	expectedDigest  string
	diagnosed       bool
}

// stale reports the same "recorded digest disagrees" condition
// authorizeManagedReviewerAssets has always refused on. An expected digest
// this binary could not derive counts as disagreeing too, exactly as the
// original inline check treated a digest error: there is nothing to compare
// against, so the recorded one cannot be vouched for.
func (p managedAssetProvenance) stale() bool {
	return p.diagnosed && (p.expectedDigest == "" || p.installedDigest != p.expectedDigest)
}

// staleAssetIdentities names the one asset identity known to be stale: the
// recorded digest that no longer matches this binary's embedded assets. It
// returns nil when nothing is stale, or when the stale installed digest
// itself is unknown.
func (p managedAssetProvenance) staleAssetIdentities() []string {
	if !p.stale() || p.installedDigest == "" {
		return nil
	}
	return []string{p.installedDigest}
}

// checkManagedReviewerAssets resolves the installed/expected digest pair
// authorizeManagedReviewerAssets compares. Callers that must explain *which*
// asset is stale -- a STATUS preflight or a START refusal's sync continuation
// -- use this directly instead of re-deriving the comparison from an error
// string.
func checkManagedReviewerAssets() managedAssetProvenance {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return managedAssetProvenance{}
	}
	persisted, err := state.Read(homeDir)
	if err != nil || persisted.ManagedAssetDigest == "" {
		return managedAssetProvenance{}
	}
	digest, digestErr := managedAssetDigest()
	if digestErr != nil {
		return managedAssetProvenance{installedDigest: persisted.ManagedAssetDigest, diagnosed: true}
	}
	return managedAssetProvenance{installedDigest: persisted.ManagedAssetDigest, expectedDigest: digest, diagnosed: true}
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
	if checkManagedReviewerAssets().stale() {
		return errors.New(managedAssetProvenanceRefusal)
	}
	return nil
}
