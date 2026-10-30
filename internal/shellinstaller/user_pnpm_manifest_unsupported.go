//go:build darwin || (linux && !amd64)

package shellinstaller

import (
	"context"
	"io"
)

func userReadPNPMManifest(_ context.Context, _ string, manifest userManifest) (userManifest, error) {
	return manifest, privateError("unsupported", nil)
}

func userLaunchPNPM(_ context.Context, _, _ string, _ []string, _ io.Reader, _, _ io.Writer) error {
	return privateError("unsupported", nil)
}
