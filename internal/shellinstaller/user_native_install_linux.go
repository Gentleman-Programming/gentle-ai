//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// produceForDestination preserves the original runner and pre-publication guards.
// The caller owns the qualified worker, immutable snapshot custody, temporary
// Node, terminal restoration and physical confirmation. It retains producer
// evidence on failure. This is not an ordinary source-carrier acquisition API.
// A blocked native result is returned unchanged, without a publication location.
// Native service callers publish only after this sealed process has terminated.
func (w *NativeInstallerRuntime) produceForDestination(ctx context.Context, req UserInstallRequest, stage string) (native NativeInstallerResult, location *NativeInstallationLocation, err error) {
	if err = nativeRuntimeError(ctx, nil); err != nil {
		return
	}
	if w == nil || w.sourceSnapshot == nil || req.Mode != "separate" {
		return native, location, privateError("refused", errors.New("native publication requires a snapshot-bound fresh Separate runtime"))
	}
	if err = userValidateStageExperience(req.Experience); err != nil {
		return
	}
	if err = UserKernelCheck(); err != nil {
		return
	}
	if req, err = userCanonicalRequest(req); err != nil {
		return
	}
	if !userSelectionPath(stage) || stage == req.Destination || filepath.Dir(stage) != filepath.Dir(req.Destination) || userPathsOverlap(w.root, stage) || userPathsOverlap(w.root, req.Destination) {
		return native, location, privateError("destination", nil)
	}
	for _, path := range []string{stage, req.Destination} {
		if err = privateDestination(path); err != nil {
			return
		}
	}
	confirmation, err := InspectUserInstall(req)
	if err != nil || req.Confirmation != confirmation {
		return native, location, privateError("refused", errors.Join(err, errors.New("physical selection confirmation differs")))
	}
	if err = w.nativeSourceCheck(ctx); err != nil {
		return
	}
	worker, err := w.withPublicationData()
	if err != nil {
		return
	}
	plan, err := worker.Prepare(ctx, true)
	if err != nil {
		return
	}
	native, location, err = worker.RunWithLocation(ctx, plan, true)
	if err != nil || location == nil {
		return // The original blocked/terminal result is not publication authority.
	}
	confirmation, err = InspectUserInstall(req)
	if err != nil || req.Confirmation != confirmation {
		return native, location, privateError("refused", errors.Join(err, errors.New("physical selection changed during original installation")))
	}
	return
}

// Keep the existing transport environment unchanged for other callers. Both
// original phases use this same copy because prepared plans bind their owner.
func (w *NativeInstallerRuntime) withPublicationData() (*NativeInstallerRuntime, error) {
	data := filepath.Join(w.root, "data")
	if _, err := privateDirectory(w.root); err != nil {
		return nil, err
	}
	if err := os.Mkdir(data, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	if _, err := privateDirectory(data); err != nil {
		return nil, err
	}
	worker := *w
	worker.publicationData = true
	return &worker, nil
}
