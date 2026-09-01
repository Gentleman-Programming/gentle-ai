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
	digest, digestErr := managedAssetDigest()
	if digestErr != nil || persisted.ManagedAssetDigest != digest {
		return errors.New(managedAssetProvenanceRefusal)
	}
	return nil
}
