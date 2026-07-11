package opencodeplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// UninstallResult summarizes what the 4-layer engine touched.
type UninstallResult struct {
	PluginID           model.OpenCodeCommunityPluginID
	ChangedTUI         bool
	ChangedPackageJSON bool
	ChangedNodeModules bool
	CacheEntryRemoved  string // absolute path of removed cache entry, or "" if none
	NodeModulesPath    string // absolute path of node_modules/<pkg> removed, or ""
	TSXPath            string // absolute path of .tsx file removed (GentleLogo only), or ""
	JournalManifest    []mutationjournal.OwnedFile
}

// Uninstall removes a community plugin across up to 4 layers: tui.json,
// package.json, node_modules/<pkg>, and the matching cache entry under
// ~/.cache/opencode/packages/<pkg>@latest. Built-in GentleLogo swaps the NPM
// layers for a single .tsx removal. On failure, captured layers are restored
// from the journal and the partial result returned reflects what was actually
// performed.
//
// Layer 3 (node_modules) and the cache entry are not journalable — both are
// directory trees — so callers that need a guaranteed revert must snapshot
// them out of band before calling Uninstall.
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

	rollback := func(err error, stage string, partial UninstallResult) (UninstallResult, error) {
		return rollbackUninstall(journal, id, partial, err, stage, pjWritten)
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
		if info, statErr := os.Stat(nodeModulesPath); statErr == nil && info.IsDir() {
			if rmErr := os.RemoveAll(nodeModulesPath); rmErr != nil {
				return rollback(fmt.Errorf("uninstall layer 3 (node_modules): %w", rmErr), "layer 3", result)
			}
			result.ChangedNodeModules = true
			result.NodeModulesPath = nodeModulesPath
			result.JournalManifest = append(result.JournalManifest, removalRecord())
		}
	}

	// ── Layer 4: optional cache entries (skipped for built-in GentleLogo) ─
	if !isGentleLogo {
		removed, err := uninstallCacheEntry(homeDir, def.PackageName)
		if err != nil {
			return rollback(fmt.Errorf("uninstall layer 4 (cache): %w", err), "layer 4", result)
		}
		if removed != "" {
			result.CacheEntryRemoved = removed
			result.JournalManifest = append(result.JournalManifest, removalRecord())
		}
	}

	// ── Layer TSX: built-in GentleLogo local plugin file ───────────────────
	if isGentleLogo {
		tsxPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", gentleLogoPluginFile)
		info, statErr := os.Stat(tsxPath)
		if statErr == nil && !info.IsDir() {
			if err := journal.Capture(tsxPath); err != nil {
				return rollback(fmt.Errorf("uninstall tsx: capture %s: %w", tsxPath, err), "layer tsx", result)
			}
			if rmErr := os.Remove(tsxPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return rollback(fmt.Errorf("uninstall tsx: remove %s: %w", tsxPath, rmErr), "layer tsx", result)
			}
			result.TSXPath = tsxPath
			result.JournalManifest = append(result.JournalManifest, removalRecord())
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

// uninstallCacheEntry removes the OpenCode plugin cache for pkg. Must mirror
// internal/update/upgrade/strategy.go:clearOpenCodePluginPackageCache: the path
// is always ~/.cache/opencode/packages/<pkg>@latest, never a prefix-match
// across the cache root. If the path does not exist, this is a silent no-op
// and the returned path is "".
func uninstallCacheEntry(homeDir, pkg string) (string, error) {
	path := filepath.Join(homeDir, ".cache", "opencode", "packages", pkg+"@latest")
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat cache entry %q: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return "", fmt.Errorf("remove cache entry %q: %w", path, err)
	}
	return path, nil
}

// rollbackUninstall is the testable seam for Uninstall's rollback path. It
// restores the journal and returns the partial UninstallResult, flipping
// ChangedTUI/ChangedPackageJSON back to false because Restore rewinds those
// files to their before-images. Other "we did remove something" flags
// (ChangedNodeModules, CacheEntryRemoved, NodeModulesPath, TSXPath) stay
// because directories aren't journal-restorable. The original err is wrapped
// with the stage label on Restore failure.
func rollbackUninstall(journal *mutationjournal.Journal, id model.OpenCodeCommunityPluginID, partial UninstallResult, err error, stage string, pjWritten bool) (UninstallResult, error) {
	final := partial
	final.PluginID = id
	rerr := journal.Restore()
	if rerr == nil {
		final.ChangedTUI = false
		if pjWritten {
			final.ChangedPackageJSON = false
		}
		return final, err
	}
	return final, errors.Join(err, fmt.Errorf("restore journal after %s: %w", stage, rerr))
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
