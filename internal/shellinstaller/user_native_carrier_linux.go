//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// StageNativeInstallerCarrier materializes the selected local original source in
// an already prepared private worker with an owned runtime directory. It does
// not acquire Node, invoke suppliers, choose SDKSource, or authorize execution.
// The caller must keep runtime/main and runtime/main-source immutable before
// executing the returned runtime, and owns cleanup and retained failure evidence.
func StageNativeInstallerCarrier(ctx context.Context, root string) (*NativeInstallerRuntime, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return nil, err
	}
	if err := nativeRuntimePaths(root); err != nil {
		return nil, err
	}
	if _, err := privateDirectory(filepath.Join(root, "runtime")); err != nil {
		return nil, err
	}
	for _, name := range []string{"main", "main-source"} {
		if _, err := os.Lstat(filepath.Join(root, "runtime", name)); !os.IsNotExist(err) {
			return nil, privateError("refused", err)
		}
	}
	artifact, err := userNativeCarrierDescriptor()
	if err != nil {
		return nil, err
	}
	data, err := assets.ReadNativeInstallerSource()
	if err != nil {
		return nil, err
	}
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		return nil, err
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		return nil, err
	}
	return userNativeRuntimeFromSnapshot(ctx, root, artifact)
}

func userNativeCarrierDescriptor() (userSnapshotArtifact, error) {
	return userMainDescriptor("8983b43bd52210a9242a056d291fb2bf30e73cc5", "f4b964eda29f2ff9b4fb6b8329a2f1eb60e7b5313984518cf6109159b18a71e9")
}
