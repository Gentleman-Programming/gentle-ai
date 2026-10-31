//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The in-place channel overlay swaps only the installed SDK component when a
// managed reconfigure changes nothing but the SDK update channel. Node, Go,
// fd, rg, CodeGraph, every other prefix package, preferences, the agent home
// and the supervisor stay untouched. Seams keep the overlay testable
// in-process: fixture trees carry no real supplier archives and never execute
// Node. Production values are the complete verifier, the owned provisioner and
// the canonical Main acquisition and the exclusive authority writer.
var (
	userWindowsOverlayVerify       = userWindowsVerify
	userWindowsOverlayProvision    = userWindowsProvision
	userWindowsOverlayMainSnapshot = userWindowsMainSnapshot
	userWindowsOverlayWrite        = userWindowsWrite
)

// userWindowsOverlayStockName is the tree-root sibling that retains the stock
// gentle-pi package while Main owns prefix/node_modules/gentle-pi. It is the
// only restore source for a later Main -> Stable overlay; a Main installation
// therefore keeps it, exactly like the fresh-install overlay stage did.
const userWindowsOverlayStockName = "stock-gentle-pi"

// userWindowsOverlayRollback unwinds compensated overlay mutations in reverse
// order. Any undo failure is joined and reported; nothing is silently dropped.
type userWindowsOverlayRollback struct{ undos []func() error }

func (r *userWindowsOverlayRollback) push(undo func() error) {
	r.undos = append(r.undos, undo)
}

func (r *userWindowsOverlayRollback) unwind() error {
	var cause error
	for index := len(r.undos) - 1; index >= 0; index-- {
		cause = errors.Join(cause, r.undos[index]())
	}
	r.undos = nil
	return cause
}

func userWindowsOverlayPaths(root string) (gentle, stock string) {
	return filepath.Join(root, "prefix", "node_modules", "gentle-pi"), filepath.Join(root, userWindowsOverlayStockName)
}

func userWindowsOverlayRemoveIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// userWindowsChannelOverlay performs the in-place SDK channel swap when the
// managed reconfigure request differs from the installed selection only in the
// channel. applied=false with a nil error means the channels match and the
// caller must keep the full retire-and-reinstall reconfigure. The request's
// experience/preferences are deliberately ignored here: the overlay touches
// only the SDK component and the channel authority, never configuration.
func userWindowsChannelOverlay(ctx context.Context, req UserInstallRequest) (result UserInstallResult, applied bool, err error) {
	root := req.Destination
	if userWindowsReconfigureTarget() != root {
		return UserInstallResult{}, false, nil
	}
	selection, err := userWindowsRead(filepath.Join(root, "selection.json"), 4096)
	if err != nil {
		return UserInstallResult{}, false, err
	}
	installed, err := userWindowsSelection(selection, root)
	if err != nil {
		return UserInstallResult{}, false, err
	}
	if installed.Channel == req.Channel {
		return UserInstallResult{}, false, nil // Same-channel reconfigure stays full.
	}
	manifest, err := userWindowsManifestRead(root)
	if err != nil {
		return UserInstallResult{}, true, err
	}
	manifestBytes, err := userWindowsRead(filepath.Join(root, "manifest.json"), 4096)
	if err != nil {
		return UserInstallResult{}, true, err
	}
	// Integrity gate: refuse on any doubt about the installed tree before a
	// single byte moves.
	if err := userWindowsOverlayVerify(ctx, root, io.Discard, os.Stderr); err != nil {
		return UserInstallResult{}, true, fmt.Errorf("Windows channel overlay refused: installed tree verification failed: %w", err)
	}
	rollback := &userWindowsOverlayRollback{}
	committed := false
	defer func() {
		if committed {
			return
		}
		if cause := rollback.unwind(); cause != nil {
			err = errors.Join(err, cause, fmt.Errorf("Windows channel overlay rollback incomplete; preserved %q for recovery", root))
		}
	}()
	var cleanup func()
	switch {
	case installed.Channel == userChannelStable && req.Channel == userChannelMain:
		fmt.Fprintln(os.Stderr, "Windows install phase: channel-overlay stable -> main")
		snapshot, overlayErr := userWindowsOverlayToMain(ctx, root, rollback)
		if overlayErr != nil {
			return UserInstallResult{}, true, overlayErr
		}
		if rebindErr := userWindowsChannelRebind(root, selection, manifestBytes, installed, manifest, userChannelMain, &snapshot, rollback); rebindErr != nil {
			return UserInstallResult{}, true, rebindErr
		}
	case installed.Channel == userChannelMain && req.Channel == userChannelStable:
		fmt.Fprintln(os.Stderr, "Windows install phase: channel-overlay main -> stable")
		quarantine, overlayErr := userWindowsOverlayToStable(ctx, root, rollback)
		if overlayErr != nil {
			return UserInstallResult{}, true, overlayErr
		}
		if rebindErr := userWindowsChannelRebind(root, selection, manifestBytes, installed, manifest, userChannelStable, nil, rollback); rebindErr != nil {
			return UserInstallResult{}, true, rebindErr
		}
		cleanup = func() {
			cause := errors.Join(
				userWindowsOverlayRemoveIfPresent(filepath.Join(root, "main.json")),
				userWindowsOverlayRemoveIfPresent(filepath.Join(root, "runtime", "archives", "main.zip")),
				os.RemoveAll(quarantine))
			if cause != nil {
				fmt.Fprintf(os.Stderr, "Windows channel overlay committed, but Main cleanup failed; overlay evidence remains in %q: %v\n", root, cause)
			}
		}
	default:
		return UserInstallResult{}, true, fmt.Errorf("Windows channel overlay refused: unknown channel transition %q -> %q", installed.Channel, req.Channel)
	}
	// Commit confirmation: the rebound tree must pass the complete
	// verification; any doubt unwinds the whole overlay including the
	// authority rebind, leaving the previous valid state in place.
	if err := userWindowsOverlayVerify(ctx, root, io.Discard, os.Stderr); err != nil {
		return UserInstallResult{}, true, fmt.Errorf("Windows channel overlay unwound: rebound tree verification failed: %w", err)
	}
	committed = true
	if cleanup != nil {
		cleanup()
	}
	return UserInstallResult{root, filepath.Join(root, "prefix"), filepath.Join(root, "agent"), "channel-overlay", ""}, true, nil
}

// userWindowsOverlayToMain swaps the installed Stable SDK for a freshly
// acquired Main snapshot. Every mutation is compensated through the rollback
// stack, so a failure anywhere leaves the previous Stable installation intact.
func userWindowsOverlayToMain(ctx context.Context, root string, rollback *userWindowsOverlayRollback) (userWindowsArtifact, error) {
	gentle, stock := userWindowsOverlayPaths(root)
	snapshot, data, err := userWindowsOverlayMainSnapshot(ctx)
	if err != nil {
		return userWindowsArtifact{}, err
	}
	// The main snapshot is validated exactly like the fresh-install path:
	// descriptor authority, member set and package.json are checked before
	// anything is unpacked.
	if _, err := userWindowsMainFiles(ctx, data, snapshot); err != nil {
		return userWindowsArtifact{}, errors.Join(err, errors.New("fresh Main snapshot validation refused"))
	}
	// Refuse every occupied authority path before the first mutation.
	for _, relative := range []string{"main.json", "runtime/archives/main.zip", "selection.json.overlay-new", "manifest.json.overlay-new", userWindowsOverlayStockName} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			return userWindowsArtifact{}, errors.Join(err, fmt.Errorf("Windows channel overlay refused: %s is already occupied", relative))
		}
	}
	// First retain the installed stock package as the tree-root restore source
	// for a later Main -> Stable overlay: a stable installation has no copy.
	if err := os.Rename(gentle, stock); err != nil {
		return userWindowsArtifact{}, err
	}
	rollback.push(func() error {
		if err := os.RemoveAll(gentle); err != nil {
			return err
		}
		return os.Rename(stock, gentle)
	})
	// Unpack the fresh Main snapshot into the SDK location, mirroring the
	// fresh-install overlay block.
	if _, err := userWindowsZIP(ctx, data, snapshot, gentle, ""); err != nil {
		return userWindowsArtifact{}, err
	}
	for _, name := range []string{".gentle-ai", "node_modules"} {
		original, target := filepath.Join(stock, name), filepath.Join(gentle, name)
		if _, err := os.Lstat(original); errors.Is(err, os.ErrNotExist) && name == "node_modules" {
			continue // The stock package may carry no dependencies of its own.
		} else if err != nil {
			return userWindowsArtifact{}, err
		}
		if err := os.Rename(original, target); err != nil {
			return userWindowsArtifact{}, err
		}
		rollback.push(func() error { return os.Rename(target, original) })
	}
	// Inert Main authority: the manifest names nothing yet, so these writes
	// change no verification outcome until the authority rebind below. Each
	// undo is registered before its exclusive write (both paths were proven
	// absent above), so a failed or partial write never outlives the unwind.
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return userWindowsArtifact{}, err
	}
	for _, file := range []struct {
		path string
		data []byte
	}{{filepath.Join(root, "main.json"), encoded}, {filepath.Join(root, "runtime", "archives", "main.zip"), data}} {
		path := file.path
		rollback.push(func() error { return userWindowsOverlayRemoveIfPresent(path) })
		if err := userWindowsOverlayWrite(path, file.data); err != nil {
			return userWindowsArtifact{}, err
		}
	}
	if err := userWindowsCompareMain(ctx, data, snapshot, gentle); err != nil {
		return userWindowsArtifact{}, err
	}
	if err := userWindowsOverlayProvision(ctx, root, "overlay", io.Discard, os.Stderr); err != nil {
		return userWindowsArtifact{}, err
	}
	return snapshot, nil
}

// userWindowsOverlayToStable restores the retained tree-root stock package
// into prefix/node_modules/gentle-pi and returns the quarantine path holding
// the displaced Main package. The Main copy is never removed here: it only
// leaves the tree after the restored tree fully verifies.
func userWindowsOverlayToStable(ctx context.Context, root string, rollback *userWindowsOverlayRollback) (string, error) {
	gentle, stock := userWindowsOverlayPaths(root)
	quarantine := filepath.Join(root, "replaced-gentle-pi-"+time.Now().UTC().Format("20060102T150405Z"))
	for _, relative := range []string{"selection.json.overlay-new", "manifest.json.overlay-new"} {
		if _, err := os.Lstat(filepath.Join(root, relative)); !os.IsNotExist(err) {
			return "", errors.Join(err, fmt.Errorf("Windows channel overlay refused: %s is already occupied", relative))
		}
	}
	if _, err := os.Lstat(quarantine); !os.IsNotExist(err) {
		return "", errors.Join(err, errors.New("Windows channel overlay refused: Main quarantine path is already occupied"))
	}
	if _, err := os.Lstat(gentle); err != nil {
		return "", err
	}
	if _, err := os.Lstat(stock); err != nil {
		return "", errors.Join(err, errors.New("Main installation lost its retained stock package; no in-place restore source"))
	}
	// Native and dependency custody return to the stock copy first, exactly
	// reversing the overlay move.
	for _, name := range []string{".gentle-ai", "node_modules"} {
		mainPath, stockPath := filepath.Join(gentle, name), filepath.Join(stock, name)
		_, mainErr := os.Lstat(mainPath)
		_, stockErr := os.Lstat(stockPath)
		mainMissing, stockMissing := errors.Is(mainErr, os.ErrNotExist), errors.Is(stockErr, os.ErrNotExist)
		if mainErr != nil && !mainMissing {
			return "", mainErr
		}
		if stockErr != nil && !stockMissing {
			return "", stockErr
		}
		switch {
		case !mainMissing && stockMissing:
			if err := os.Rename(mainPath, stockPath); err != nil {
				return "", err
			}
			rollback.push(func() error { return os.Rename(stockPath, mainPath) })
		case mainMissing && stockMissing && name == "node_modules":
			// Neither side carries dependency custody.
		default:
			return "", fmt.Errorf("Windows channel overlay refused: unexpected %s custody layout", name)
		}
	}
	// Rename-exchange semantics: the Main package moves aside to quarantine,
	// then the retained stock copy is promoted. The last known-good copy is
	// only dropped after the restored tree fully verifies.
	if err := os.Rename(gentle, quarantine); err != nil {
		return "", err
	}
	rollback.push(func() error { return os.Rename(quarantine, gentle) })
	if err := os.Rename(stock, gentle); err != nil {
		return "", err
	}
	rollback.push(func() error {
		if err := os.Rename(gentle, stock); err != nil {
			return err
		}
		return os.Rename(quarantine, gentle)
	})
	// The selection still names Main here, so the provisioner re-checks the
	// composition of the restored package before any authority changes.
	return quarantine, userWindowsOverlayProvision(ctx, root, "verify", io.Discard, os.Stderr)
}

// userWindowsChannelRebind rebinds the channel authority: the new selection
// (channel field only, canonical JSON) and the new manifest (SelectionSHA;
// MainCommit/MainArchiveSHA set for Main, cleared for Stable) are staged
// exclusively and then renamed into place, the manifest last, with the
// previous bytes restored on any failure.
func userWindowsChannelRebind(root string, selection, manifestBytes []byte, installed UserInstallRequest, manifest userWindowsManifest, channel string, artifact *userWindowsArtifact, rollback *userWindowsOverlayRollback) error {
	installed.Channel = channel
	rebound, err := json.Marshal(installed)
	if err != nil {
		return err
	}
	if _, err := userWindowsSelection(rebound, root); err != nil {
		return err
	}
	reboundManifest := manifest
	reboundManifest.SelectionSHA = userWindowsSHA(rebound)
	if artifact != nil {
		reboundManifest.MainCommit = strings.TrimPrefix(artifact.Prefix, "gentle-shell-")
		reboundManifest.MainArchiveSHA = artifact.SHA
	} else {
		reboundManifest.MainCommit, reboundManifest.MainArchiveSHA = "", ""
	}
	encoded, err := json.Marshal(reboundManifest)
	if err != nil {
		return err
	}
	selectionStaged := filepath.Join(root, "selection.json.overlay-new")
	manifestStaged := filepath.Join(root, "manifest.json.overlay-new")
	if err := userWindowsWrite(selectionStaged, rebound); err != nil {
		return err
	}
	if err := userWindowsWrite(manifestStaged, append(encoded, '\n')); err != nil {
		return errors.Join(err, os.Remove(selectionStaged))
	}
	restore := func() error {
		return errors.Join(
			userWindowsOverlayRemoveIfPresent(selectionStaged),
			userWindowsOverlayRemoveIfPresent(manifestStaged),
			userWindowsReplaceFile(filepath.Join(root, "selection.json"), selection),
			userWindowsReplaceFile(filepath.Join(root, "manifest.json"), manifestBytes))
	}
	if err := os.Rename(selectionStaged, filepath.Join(root, "selection.json")); err != nil {
		rollback.push(restore)
		return err
	}
	if err := os.Rename(manifestStaged, filepath.Join(root, "manifest.json")); err != nil {
		rollback.push(restore)
		return err
	}
	rollback.push(restore)
	return nil
}
