//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"path/filepath"
)

var errNativeSourceSnapshot = errors.New("native source snapshot differs")

// Bind the complete staged source to the caller-authorized archive, not to an
// arbitrary path or entry stamp. Existing custody owns materialization. A
// descriptor/hash is not publisher authentication: the caller must authorize
// these exact snapshot bytes and keep them immutable throughout execution.
func userNativeRuntimeFromSnapshot(ctx context.Context, root string, artifact userSnapshotArtifact) (*NativeInstallerRuntime, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return nil, err
	}
	if err := userCompareMainSource(ctx, root, artifact); err != nil {
		return nil, nativeRuntimeError(ctx, privateError("refused", errors.Join(errNativeSourceSnapshot, err)))
	}
	w, err := NewNativeInstallerRuntime(ctx, root, filepath.Join(root, "runtime/main-source"))
	if err != nil {
		return nil, err
	}
	w.sourceSnapshot = &artifact // Private value copy; caller edits cannot rebind it.
	return w, nil
}

func (w *NativeInstallerRuntime) nativeSourceCheck(ctx context.Context) error {
	if w.sourceSnapshot == nil {
		return nil // Existing low-level client still requires caller-owned custody.
	}
	if err := userCompareMainSource(ctx, w.root, *w.sourceSnapshot); err != nil {
		return nativeRuntimeError(ctx, privateError("refused", errors.Join(errNativeSourceSnapshot, err)))
	}
	return nil
}
