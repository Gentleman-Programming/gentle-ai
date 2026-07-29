package opencodeplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// UninstallResult summarizes what the 4-layer engine touched.
type UninstallResult struct {
	PluginID           model.OpenCodeCommunityPluginID
	ChangedTUI         bool
	ChangedPackageJSON bool
	ChangedNodeModules bool
	CacheEntryRemoved  string   // absolute path of removed cache entry, or "" if none
	NodeModulesPath    string   // absolute path of node_modules/<pkg> removed, or ""
	TSXPath            string   // absolute path of .tsx file removed (GentleLogo only), or ""
	CleanupPending     []string // staged paths left after a post-commit cleanup failure
	JournalManifest    []mutationjournal.OwnedFile
}

var removeAll = os.RemoveAll

// Uninstall removes a community plugin across up to 4 layers: tui.json,
// package.json, node_modules/<pkg>, and the matching cache entry under
// ~/.cache/opencode/packages/<pkg>@latest. Built-in GentleLogo swaps the NPM
// layers for a single .tsx removal. Files use compare-and-swap journal writes;
// directories are renamed to reversible staging paths until the operation
// commits. A post-commit staging cleanup failure is reported in CleanupPending.
func Uninstall(homeDir string, id model.OpenCodeCommunityPluginID) (UninstallResult, error) {
	result := UninstallResult{PluginID: id}

	if homeDir == "" {
		return result, fmt.Errorf("uninstall opencode plugin: homeDir must not be empty")
	}

	isGentleLogo := id == model.OpenCodePluginGentleLogo
	def, known := DefinitionFor(id)
	if !isGentleLogo && !known {
		return result, fmt.Errorf("uninstall opencode plugin: unknown id %q", id)
	}

	// targetInTUI is the literal string Install() registered inside tui.json's
	// plugin[] list. For NPM plugins this is the NPM package name; for the
	// built-in GentleLogo .tsx it is the absolute plugin path so it matches
	// the value Install() wrote.
	var targetInTUI string
	if isGentleLogo {
		targetInTUI = filepath.Join(homeDir, ".config", "opencode", "tui-plugins", gentleLogoPluginFile)
	} else {
		targetInTUI = def.PackageName
	}

	opencodeDir := filepath.Join(homeDir, ".config", "opencode")
	tuiPath := filepath.Join(opencodeDir, "tui.json")
	packagePath := filepath.Join(opencodeDir, "package.json")

	journal := mutationjournal.New(homeDir)
	pjWritten := false
	var staged []stagedRemoval

	rollback := func(err error, stage string, partial UninstallResult) (UninstallResult, error) {
		return rollbackUninstall(journal, staged, id, partial, err, stage, pjWritten)
	}

	// ── Layer 1: tui.json ──────────────────────────────────────────────────
	if err := journal.Capture(tuiPath); err != nil {
		return result, fmt.Errorf("uninstall layer 1 (tui.json capture): %w", err)
	}
	tuiChanged, tuiAfter, err := removeTUIPlugin(tuiPath, targetInTUI)
	if err != nil {
		return rollback(fmt.Errorf("uninstall layer 1 (tui.json): %w", err), "layer 1", result)
	}
	if tuiChanged {
		if _, err := journal.Write(tuiPath, tuiAfter); err != nil {
			return rollback(fmt.Errorf("uninstall layer 1 (tui.json write): %w", err), "layer 1", result)
		}
		result.ChangedTUI = true
		result.JournalManifest = append(result.JournalManifest, journal.OwnedFile(tuiPath, string(tuiAfter), false, false))
	}

	// ── Layer 2: package.json (skipped for built-in GentleLogo) ───────────
	if !isGentleLogo {
		pjChanged, pjOwned, err := uninstallPackageJSON(journal, packagePath, def.PackageName)
		if err != nil {
			return rollback(fmt.Errorf("uninstall layer 2 (package.json): %w", err), "layer 2", result)
		}
		if pjChanged {
			pjWritten = true
			result.ChangedPackageJSON = true
			result.JournalManifest = append(result.JournalManifest, pjOwned)
		}
	}

	// ── Layer 3: node_modules/<pkg>/ (skipped for built-in GentleLogo) ─────
	if !isGentleLogo {
		nodeModulesPath := filepath.Join(opencodeDir, "node_modules", def.PackageName)
		removal, err := stageDirectoryRemoval(journal, nodeModulesPath)
		if err != nil {
			return rollback(fmt.Errorf("uninstall layer 3 (node_modules): %w", err), "layer 3", result)
		}
		if removal != nil {
			staged = append(staged, *removal)
			result.ChangedNodeModules = true
			result.NodeModulesPath = nodeModulesPath
			result.JournalManifest = append(result.JournalManifest, removalRecord())
		}
	}

	// ── Layer 4: optional cache entries (skipped for built-in GentleLogo) ─
	if !isGentleLogo {
		cachePath := filepath.Join(homeDir, ".cache", "opencode", "packages", def.PackageName+"@latest")
		removal, err := stageDirectoryRemoval(journal, cachePath)
		if err != nil {
			return rollback(fmt.Errorf("uninstall layer 4 (cache): %w", err), "layer 4", result)
		}
		if removal != nil {
			staged = append(staged, *removal)
			result.CacheEntryRemoved = cachePath
			result.JournalManifest = append(result.JournalManifest, removalRecord())
		}
	}

	// ── Layer TSX: built-in GentleLogo local plugin file ───────────────────
	if isGentleLogo {
		tsxPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", gentleLogoPluginFile)
		info, statErr := os.Lstat(tsxPath)
		if statErr == nil && !info.IsDir() {
			removed, err := journal.Remove(tsxPath)
			if err != nil {
				return rollback(fmt.Errorf("uninstall tsx: remove %s: %w", tsxPath, err), "layer tsx", result)
			}
			if removed {
				result.TSXPath = tsxPath
				result.JournalManifest = append(result.JournalManifest, removalRecord())
			}
		}
	}

	for _, removal := range staged {
		if err := removeAll(removal.staged); err != nil {
			result.CleanupPending = append(result.CleanupPending, removal.staged)
		}
	}

	return result, nil
}

// uninstallPackageJSON removes pkg from dependencies + devDependencies in
// packagePath. Returns true and an OwnedFile manifest entry iff a write
// happened. If package.json is missing or empty, returns (false, "", nil) as
// a no-op. Keeps dependencies/devDependencies keys present (as empty maps)
// rather than deleting them, even when they end up empty.
//
// package.json is decoded with map[string]any (not the typed deps struct in
// internal/update/upgrade/strategy.go) so that unrelated keys — packageManager,
// scripts, workspaces, etc. — survive the rewrite verbatim.
func uninstallPackageJSON(journal *mutationjournal.Journal, packagePath, pkg string) (bool, mutationjournal.OwnedFile, error) {
	if err := journal.Capture(packagePath); err != nil {
		return false, mutationjournal.OwnedFile{}, fmt.Errorf("uninstall layer 2 (package.json capture): %w", err)
	}
	data, err := os.ReadFile(packagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, mutationjournal.OwnedFile{}, nil
		}
		return false, mutationjournal.OwnedFile{}, fmt.Errorf("read package.json %q: %w", packagePath, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return false, mutationjournal.OwnedFile{}, nil
	}

	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, mutationjournal.OwnedFile{}, fmt.Errorf("parse package.json %q: %w", packagePath, err)
	}

	hadChanges := false
	for _, key := range []string{"dependencies", "devDependencies"} {
		entry, ok := root[key].(map[string]any)
		if !ok {
			// Missing key or wrong shape: normalize to an empty map so the JSON
			// stays balanced after our rewrite, but don't pretend a change
			// happened if pkg was never there.
			root[key] = map[string]any{}
			continue
		}
		if _, present := entry[pkg]; present {
			delete(entry, pkg)
			hadChanges = true
		}
	}

	if !hadChanges {
		return false, mutationjournal.OwnedFile{}, nil
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, mutationjournal.OwnedFile{}, fmt.Errorf("marshal package.json: %w", err)
	}
	out = append(out, '\n')
	written, err := journal.Write(packagePath, out)
	if err != nil {
		return false, mutationjournal.OwnedFile{}, err
	}
	return true, written, nil
}

type stagedRemoval struct {
	original string
	staged   string
}

func stageDirectoryRemoval(journal *mutationjournal.Journal, path string) (*stagedRemoval, error) {
	if err := journal.Validate(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat removal target %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("refuse non-directory removal target %q", path)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".gentle-ai-uninstall-*")
	if err != nil {
		return nil, fmt.Errorf("reserve staging path for %q: %w", path, err)
	}
	staged := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(staged)
		return nil, fmt.Errorf("close staging placeholder for %q: %w", path, err)
	}
	if err := os.Remove(staged); err != nil {
		return nil, fmt.Errorf("remove staging placeholder for %q: %w", path, err)
	}
	if err := os.Rename(path, staged); err != nil {
		return nil, fmt.Errorf("stage removal target %q: %w", path, err)
	}
	return &stagedRemoval{original: path, staged: staged}, nil
}

// rollbackUninstall restores staged directories and journaled files. Result
// flags are cleared only when every restoration succeeds; otherwise the
// joined error identifies the state that requires manual recovery.
func rollbackUninstall(journal *mutationjournal.Journal, staged []stagedRemoval, id model.OpenCodeCommunityPluginID, partial UninstallResult, err error, stage string, pjWritten bool) (UninstallResult, error) {
	final := partial
	final.PluginID = id
	var restoreErrors []error
	for i := len(staged) - 1; i >= 0; i-- {
		if renameErr := os.Rename(staged[i].staged, staged[i].original); renameErr != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore staged directory %q: %w", staged[i].original, renameErr))
		}
	}
	if journalErr := journal.Restore(); journalErr != nil {
		restoreErrors = append(restoreErrors, journalErr)
	}
	if len(restoreErrors) == 0 {
		final.ChangedTUI = false
		if pjWritten {
			final.ChangedPackageJSON = false
		}
		final.ChangedNodeModules = false
		final.NodeModulesPath = ""
		final.CacheEntryRemoved = ""
		final.TSXPath = ""
		return final, err
	}
	return final, errors.Join(err, fmt.Errorf("restore after %s: %w", stage, errors.Join(restoreErrors...)))
}

// removalRecord documents a deletion in the manifest that the journal cannot
// snapshot directly (a directory tree, e.g. node_modules/<pkg>/ or the cache
// entry). After is empty, AfterHash is the canonical sha256 of the empty
// string, and Mode is 0 so the entry is omitted from JSON.
const emptySHA256Hex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func removalRecord() mutationjournal.OwnedFile {
	return mutationjournal.OwnedFile{
		After:     "",
		AfterHash: emptySHA256Hex,
	}
}
