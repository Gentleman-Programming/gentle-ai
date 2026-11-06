//go:build linux || darwin

package shellinstaller

import (
	"context"
	"path/filepath"
)

// Qualification-only names stay in tests. The installed worker directly uses
// the real action boundary; no dummy production consumer exists for these aliases.
// Preflight unpublished Main against verified modern Separate stock. This is
// not API qualification, activation, or permission to skip later stock checks.
func userVerifyMainBindings(ctx context.Context, root, finalRoot string, artifact userSnapshotArtifact) error {
	return userVerifyMainBindingsAction(ctx, root, finalRoot, filepath.Join(root, "prefix"), filepath.Join(root, "agent"), artifact, "verify-main-source")
}

// Explicit SDK composition only: all three stock bindings stay authoritative.
// Neither this preflight nor the fixed plan activates source or installs Main.
func userVerifyMainSDKPreviewBindings(ctx context.Context, root, finalRoot string, artifact userSnapshotArtifact) error {
	return userVerifyMainBindingsAction(ctx, root, finalRoot, filepath.Join(root, "prefix"), filepath.Join(root, "agent"), artifact, "verify-main-sdk-source")
}
