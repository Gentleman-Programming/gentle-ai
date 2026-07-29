//go:build windows

package cli

import (
	"context"
	"os"
)

func openFinalVerificationIncidentInput(ctx context.Context, path string) (*os.File, func() error, error) {
	return openFacadeArtifactManifestInput(ctx, path)
}
