package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

const managedAssetProvenanceRefusal = "managed reviewer assets are outdated; run `gentle-ai sync`"

// managedAssetWriterIdentity reuses the review capabilities build identity so
// managed assets and their review consumer compare one exact binary identity.
func managedAssetWriterIdentity() (string, error) {
	version := strings.TrimSpace(AppVersion)
	if version == "" {
		return "", errors.New("gentle-ai package version is unavailable")
	}
	build, err := reviewCapabilitiesBuildIdentity(version)
	if err != nil {
		return "", fmt.Errorf("derive review capabilities build identity: %w", err)
	}
	return build.ID, nil
}

func authorizeManagedReviewerAssets() error {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return errors.New(managedAssetProvenanceRefusal)
	}
	persisted, err := state.Read(homeDir)
	if err != nil || persisted.ManagedAssetWriter == "" {
		return errors.New(managedAssetProvenanceRefusal)
	}
	writer, err := managedAssetWriterIdentity()
	if err != nil || persisted.ManagedAssetWriter != writer {
		return errors.New(managedAssetProvenanceRefusal)
	}
	return nil
}
