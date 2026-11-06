//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// The in-place channel overlay swaps only the installed SDK declaration when
// a managed reconfigure changes nothing but the SDK update channel. On Unix
// the stock gentle-pi package stays installed and never mutates: Main lives
// as a composed private package under runtime/main-sdk and activation is a
// settings declaration flip, so Node, Go, fd, rg, CodeGraph, every other
// prefix component, preferences, the agent home and the supervisor stay
// untouched. Seams keep the overlay testable in-process: fixture trees carry
// no real supplier archives and never execute Node. Production values are the
// complete installed-tree verifier, the owned Main acquisition and the owned
// provisioner actions.
var (
	userUnixOverlayVerify = func(ctx context.Context, root string, manifest userManifest) error {
		return userVerifyInstalledGlobal(ctx, root, manifest)
	}
	userUnixOverlayAcquire = func(ctx context.Context, root string) (userSnapshotArtifact, error) {
		artifact, data, err := userMainSnapshot(ctx)
		if err != nil {
			return userSnapshotArtifact{}, err
		}
		// The Main snapshot is validated exactly like the fresh-install path
		// before anything is staged into the published tree.
		if _, err := userMainSourceFiles(ctx, data, artifact); err != nil {
			return userSnapshotArtifact{}, errors.Join(err, errors.New("fresh Main snapshot validation refused"))
		}
		if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
			return userSnapshotArtifact{}, err
		}
		if err := userStageMainSource(ctx, root, artifact); err != nil {
			return userSnapshotArtifact{}, err
		}
		return artifact, nil
	}
	// Composition only: the provisioner's selection refuses a published tree,
	// so the overlay flips the declaration itself and the commit verification
	// (userVerifyInstalledGlobal) runs the same selected-SDK action as a fresh
	// Main install.
	userUnixOverlayCompose = func(ctx context.Context, root string, manifest userManifest, artifact userSnapshotArtifact) error {
		return userMaterializeMainSDK(ctx, root, root, manifest.Prefix, manifest.Agent, artifact)
	}
)

// userUnixOverlayMainPaths is every path the Main composition adds to a
// Stable tree, in creation order: the acquired snapshot, its source, the
// composed package and the runtime/node_modules dependency link that
// materialize-main-sdk-runtime creates. The occupancy gate, the stable -> Main
// unwind and the committed Main -> Stable cleanup all use this one list.
var userUnixOverlayMainPaths = []string{"runtime/main", "runtime/main-source", "runtime/main-sdk", "runtime/node_modules"}

// userUnixOverlayRemoveMain removes the Main composition paths in reverse
// creation order. os.RemoveAll never follows the dependency symlink.
func userUnixOverlayRemoveMain(root string) error {
	var cause error
	for index := len(userUnixOverlayMainPaths) - 1; index >= 0; index-- {
		cause = errors.Join(cause, os.RemoveAll(filepath.Join(root, filepath.FromSlash(userUnixOverlayMainPaths[index]))))
	}
	return cause
}

// userUnixOverlayRollback unwinds compensated overlay mutations in reverse
// order. Any undo failure is joined and reported; nothing is silently dropped.
type userUnixOverlayRollback struct{ undos []func() error }

func (r *userUnixOverlayRollback) push(undo func() error) {
	r.undos = append(r.undos, undo)
}

func (r *userUnixOverlayRollback) unwind() error {
	var cause error
	for index := len(r.undos) - 1; index >= 0; index-- {
		cause = errors.Join(cause, r.undos[index]())
	}
	r.undos = nil
	return cause
}

// userUnixChannelOverlay performs the in-place SDK channel swap when the
// managed reconfigure request differs from the installed composition only in
// the channel. applied=false with a nil error means the channels match, the
// platform lacks the Main machinery, or the mode keeps the full
// retire-and-reinstall reconfigure. The request's remaining experience and
// preferences are deliberately ignored here: the overlay touches only the SDK
// activation and the manifest authority, never configuration.
func userUnixChannelOverlay(ctx context.Context, req UserInstallRequest) (result UserInstallResult, applied bool, err error) {
	root := req.Destination
	if req.Experience == nil || userReconfigureTarget() != root {
		return UserInstallResult{}, false, nil
	}
	// The Main composition runtime is qualified on Linux amd64 and macOS
	// arm64; every other platform keeps its existing full reconfigure flow.
	if !userMainPlatform() {
		return UserInstallResult{}, false, nil
	}
	if _, statErr := os.Lstat(root); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return UserInstallResult{}, false, nil
		}
		return UserInstallResult{}, false, statErr
	}
	manifest, err := userReadInstallationManifest(root)
	if err != nil {
		// An installed but unreadable target is overlay-scope evidence, not a
		// reason to retire it behind the caller's back.
		return UserInstallResult{}, true, err
	}
	if manifest.Mode != "separate" {
		return UserInstallResult{}, false, nil
	}
	installed := "stable"
	if manifest.MainSDK != nil {
		installed = "main-preview"
	}
	requested := req.Experience.SDKSource
	if requested != "main-preview" {
		requested = "stable"
	}
	if installed == requested {
		return UserInstallResult{}, false, nil // Same-channel reconfigure stays full.
	}
	// Integrity gate: refuse on any doubt about the installed tree before a
	// single byte moves.
	if err := userUnixOverlayVerify(ctx, root, manifest); err != nil {
		return UserInstallResult{}, true, fmt.Errorf("Unix channel overlay refused: installed tree verification failed: %w", err)
	}
	rollback := &userUnixOverlayRollback{}
	rebound := manifest
	committed := false
	cleanup := false
	defer func() {
		if committed {
			return
		}
		if cause := rollback.unwind(); cause != nil {
			err = errors.Join(err, cause, fmt.Errorf("Unix channel overlay rollback incomplete; preserved %q for recovery", root))
		}
	}()
	switch installed {
	case "stable": // stable -> main
		fmt.Fprintln(os.Stderr, "Unix install phase: channel-overlay stable -> main")
		if rebound, err = userUnixOverlayStableToMain(ctx, root, manifest, rollback); err != nil {
			return UserInstallResult{}, true, err
		}
	case "main-preview": // main -> stable
		fmt.Fprintln(os.Stderr, "Unix install phase: channel-overlay main -> stable")
		if selectErr := userUnixOverlaySelectSDK(root, manifest, false, rollback); selectErr != nil {
			return UserInstallResult{}, true, selectErr
		}
		rebound.MainSDK = nil
		if rebindErr := userUnixOverlayRebindManifest(root, rebound, rollback); rebindErr != nil {
			return UserInstallResult{}, true, rebindErr
		}
		cleanup = true
	default:
		return UserInstallResult{}, true, fmt.Errorf("Unix channel overlay refused: unknown installed channel %q", installed)
	}
	// Commit confirmation: the rebound tree must pass the complete installed
	// verification; any doubt unwinds the whole overlay including the
	// declaration flip and the manifest rebind, leaving the previous valid
	// state in place.
	if err := userUnixOverlayVerify(ctx, root, rebound); err != nil {
		return UserInstallResult{}, true, fmt.Errorf("Unix channel overlay unwound: rebound tree verification failed: %w", err)
	}
	committed = true
	if cleanup {
		if removeErr := userUnixOverlayRemoveMain(root); removeErr != nil {
			fmt.Fprintf(os.Stderr, "Unix channel overlay committed, but Main cleanup failed; overlay evidence remains in %q: %v\n", root, removeErr)
		}
	}
	userRecordPublishedShell(rebound)
	return UserInstallResult{root, manifest.Prefix, manifest.Agent, "channel-overlay", ""}, true, nil
}

// userUnixOverlayStableToMain stages and composes the fresh Main snapshot,
// flips the SDK declaration and rebinds the manifest. Every mutation is
// registered on the rollback stack before the next step runs; the caller owns
// the final verification and the unwind.
func userUnixOverlayStableToMain(ctx context.Context, root string, manifest userManifest, rollback *userUnixOverlayRollback) (userManifest, error) {
	for _, relative := range userUnixOverlayMainPaths {
		if _, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !errors.Is(statErr, os.ErrNotExist) {
			return manifest, errors.Join(statErr, fmt.Errorf("Unix channel overlay refused: %s is already occupied", relative))
		}
	}
	// The gate proved these paths absent, so the unwind removes exactly
	// what this overlay stages; the installed tree never held them.
	rollback.push(func() error { return userUnixOverlayRemoveMain(root) })
	artifact, err := userUnixOverlayAcquire(ctx, root)
	if err != nil {
		return manifest, err
	}
	if err := userUnixOverlayCompose(ctx, root, manifest, artifact); err != nil {
		return manifest, err
	}
	if err := userUnixOverlaySelectSDK(root, manifest, true, rollback); err != nil {
		return manifest, err
	}
	rebound := manifest
	rebound.MainSDK = &artifact
	if err := userUnixOverlayRebindManifest(root, rebound, rollback); err != nil {
		return manifest, err
	}
	return rebound, nil
}

// userUnixOverlaySelectSDK flips the agent's SDK declaration between the
// installed stock package root and the composed Main package root. Only the
// plain string entry naming the source root is rewritten, exactly like the
// provisioner's own selection: every other package declaration, preference
// and setting passes through byte-identical. The write is atomic and the
// previous bytes are restored through the rollback stack on any failure.
func userUnixOverlaySelectSDK(root string, manifest userManifest, main bool, rollback *userUnixOverlayRollback) error {
	agent := manifest.Agent
	info, err := os.Lstat(agent)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return fmt.Errorf("Unix channel overlay refused: agent custody differs: %s", agent)
	}
	settingsPath := filepath.Join(agent, "settings.json")
	previous, err := os.ReadFile(settingsPath)
	if err != nil {
		return err
	}
	info, err = os.Lstat(settingsPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return fmt.Errorf("Unix channel overlay refused: settings custody differs: %s", settingsPath)
	}
	var document map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(previous, &document); unmarshalErr != nil || document == nil {
		return errors.Join(unmarshalErr, errors.New("Unix channel overlay refused: agent settings are not an object"))
	}
	var entries []json.RawMessage
	if raw, ok := document["packages"]; ok {
		if unmarshalErr := json.Unmarshal(raw, &entries); unmarshalErr != nil {
			return errors.Join(unmarshalErr, errors.New("Unix channel overlay refused: agent settings packages are not an array"))
		}
	}
	stockRoot := filepath.Join(manifest.Prefix, "lib/node_modules/gentle-pi")
	mainRoot := filepath.Join(root, "runtime/main-sdk")
	source, target := stockRoot, mainRoot
	if !main {
		source, target = mainRoot, stockRoot
	}
	var declarations []string
	for _, entry := range entries {
		var value any
		if unmarshalErr := json.Unmarshal(entry, &value); unmarshalErr != nil {
			return errors.Join(unmarshalErr, errors.New("Unix channel overlay refused: malformed agent settings package declaration"))
		}
		if text, ok := value.(string); ok {
			declarations = append(declarations, text)
		}
	}
	sourceCount, targetCount := 0, 0
	for _, declaration := range declarations {
		switch declaration {
		case source:
			sourceCount++
		case target:
			targetCount++
		}
	}
	if sourceCount != 1 || targetCount != 0 {
		return fmt.Errorf("Unix channel overlay refused: agent settings declare %d source and %d target SDK entries", sourceCount, targetCount)
	}
	for index, entry := range entries {
		var value any
		if unmarshalErr := json.Unmarshal(entry, &value); unmarshalErr != nil {
			return errors.Join(unmarshalErr, errors.New("Unix channel overlay refused: malformed agent settings package declaration"))
		}
		if text, ok := value.(string); ok && text == source {
			entries[index], err = json.Marshal(target)
			if err != nil {
				return err
			}
		}
	}
	document["packages"], err = json.Marshal(entries)
	if err != nil {
		return err
	}
	selected, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	selected = append(selected, '\n')
	stage := filepath.Join(agent, ".gentle-shell-settings-overlay")
	if _, statErr := os.Lstat(stage); !errors.Is(statErr, os.ErrNotExist) {
		return errors.Join(statErr, errors.New("Unix channel overlay refused: settings stage path is already occupied"))
	}
	if writeErr := userToolWrite(stage, selected, 0o600); writeErr != nil {
		return writeErr
	}
	restore := func() error {
		if restoreErr := userToolWrite(stage, previous, 0o600); restoreErr != nil {
			return restoreErr
		}
		if renameErr := os.Rename(stage, settingsPath); renameErr != nil {
			return renameErr
		}
		return userDirectorySync(agent)
	}
	if renameErr := os.Rename(stage, settingsPath); renameErr != nil {
		rollback.push(restore)
		return renameErr
	}
	if syncErr := userDirectorySync(agent); syncErr != nil {
		rollback.push(restore)
		return syncErr
	}
	if readback, readErr := os.ReadFile(settingsPath); readErr != nil || !bytes.Equal(readback, selected) {
		rollback.push(restore)
		return errors.Join(readErr, errors.New("Unix channel overlay refused: selected settings readback differs"))
	}
	if info, statErr := os.Lstat(settingsPath); statErr != nil || info.Mode().Perm() != 0o600 || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		rollback.push(restore)
		return errors.Join(statErr, errors.New("Unix channel overlay refused: selected settings custody differs"))
	}
	rollback.push(restore)
	return nil
}

// userUnixOverlayRebindManifest rebinds the channel authority: the rebound
// manifest (MainSDK set for Main, cleared for Stable) is staged exclusively
// and renamed into place, with the previous bytes restored on any failure.
func userUnixOverlayRebindManifest(root string, manifest userManifest, rollback *userUnixOverlayRollback) error {
	path := filepath.Join(root, "installation.json")
	previous, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return fmt.Errorf("Unix channel overlay refused: manifest custody differs: %s", path)
	}
	if len(previous) > 4096 {
		return errors.New("Unix channel overlay refused: manifest exceeds the canonical bound")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	stage := filepath.Join(root, "installation.json.overlay-new")
	if _, statErr := os.Lstat(stage); !errors.Is(statErr, os.ErrNotExist) {
		return errors.Join(statErr, errors.New("Unix channel overlay refused: manifest stage path is already occupied"))
	}
	if writeErr := userToolWrite(stage, data, 0o600); writeErr != nil {
		return writeErr
	}
	restore := func() error {
		if restoreErr := userToolWrite(stage, previous, 0o600); restoreErr != nil {
			return restoreErr
		}
		if renameErr := os.Rename(stage, path); renameErr != nil {
			return renameErr
		}
		return userDirectorySync(root)
	}
	if renameErr := os.Rename(stage, path); renameErr != nil {
		rollback.push(restore)
		return renameErr
	}
	if syncErr := userDirectorySync(root); syncErr != nil {
		rollback.push(restore)
		return syncErr
	}
	if readback, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(readback, data) {
		rollback.push(restore)
		return errors.Join(readErr, errors.New("Unix channel overlay refused: rebound manifest readback differs"))
	}
	rollback.push(restore)
	return nil
}
