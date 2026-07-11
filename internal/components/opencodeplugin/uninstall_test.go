package opencodeplugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// ─── Shared fixtures ────────────────────────────────────────────────────────

func writeTUIConfig(t *testing.T, home string, plugins []string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"$schema": "https://opencode.ai/tui.json",
		"plugin":  plugins,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "tui.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTUIPlugins(t *testing.T, home string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root.Plugin
}

func writePackageJSON(t *testing.T, home string, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPackageJSON(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeCacheEntry creates the OpenCode cache directory for pkg at the
// canonical path ~/.cache/opencode/packages/<pkg>@latest/ with a sentinel
// file inside, mirroring the layout used by
// internal/update/upgrade/strategy.go:clearOpenCodePluginPackageCache.
func writeCacheEntry(t *testing.T, home, pkgName string) string {
	t.Helper()
	cacheDir := filepath.Join(home, ".cache", "opencode", "packages", pkgName+"@latest")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "data"), []byte(`{"version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return cacheDir
}

// ─── 1. Input validation ───────────────────────────────────────────────────

func TestUninstallRejectsUnknownID(t *testing.T) {
	home := t.TempDir()
	_, err := Uninstall(home, model.OpenCodeCommunityPluginID("not-a-real-plugin"))
	if err == nil {
		t.Fatal("Uninstall() error = nil, want error for unknown id")
	}
	if !strings.Contains(err.Error(), "unknown id") {
		t.Fatalf("error %q does not mention unknown id", err)
	}
}

func TestUninstallRejectsEmptyHomeDir(t *testing.T) {
	_, err := Uninstall("", model.OpenCodePluginSubAgentStatusline)
	if err == nil {
		t.Fatal("Uninstall() error = nil, want error for empty homeDir")
	}
	if !strings.Contains(err.Error(), "homeDir") {
		t.Fatalf("error %q does not mention homeDir", err)
	}
}

// ─── 2. Idempotency and no-op matrices ─────────────────────────────────────

func TestUninstallNonExistentPluginNoOp(t *testing.T) {
	home := t.TempDir()
	// Empty home: tui.json does not exist, package.json does not exist,
	// node_modules/<pkg> does not exist, ~/.cache/opencode does not exist.
	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v, want nil", err)
	}
	if res.PluginID != model.OpenCodePluginSubAgentStatusline {
		t.Fatalf("PluginID = %q, want %q", res.PluginID, model.OpenCodePluginSubAgentStatusline)
	}
	if res.ChangedTUI || res.ChangedPackageJSON || res.ChangedNodeModules {
		t.Fatalf("expected no-op flags: got %+v", res)
	}
	if res.CacheEntryRemoved != "" || res.NodeModulesPath != "" || res.TSXPath != "" {
		t.Fatalf("expected empty path fields: got %+v", res)
	}
}

// ─── 3. Layer 1: tui.json ──────────────────────────────────────────────────

func TestUninstallRemovesFromTUIConfig(t *testing.T) {
	home := t.TempDir()
	writeTUIConfig(t, home, []string{"unrelated-plugin", "opencode-subagent-statusline"})

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.ChangedTUI {
		t.Fatal("ChangedTUI = false, want true")
	}

	plugins := readTUIPlugins(t, home)
	if len(plugins) != 1 || plugins[0] != "unrelated-plugin" {
		t.Fatalf("plugin list = %#v, want only unrelated-plugin", plugins)
	}
}

func TestUninstallTUIConfigMissing(t *testing.T) {
	home := t.TempDir()
	// Create the dir so capture doesn't error on parent path; only tui.json
	// itself is absent.
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.ChangedTUI {
		t.Fatal("ChangedTUI = true, want false when tui.json is missing")
	}
}

func TestUninstallTUIConfigMalformedJSON(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tui.json"), []byte("{not-json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err == nil {
		t.Fatal("Uninstall() error = nil, want error for malformed tui.json")
	}
	// The error chain comes from removeTUIPlugin → wrapped as "uninstall layer 1 (tui.json): …".
	// Either tier of the message must reference the path.
	if !strings.Contains(err.Error(), "tui.json") {
		t.Fatalf("error %q does not reference tui.json", err)
	}
}

// ─── 4. Layer 2: package.json ──────────────────────────────────────────────

func TestUninstallPackageJSONDepsRemoval(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName})
	writePackageJSON(t, home, `{"dependencies":{"`+pkgName+`":"^1.0.0"},"devDependencies":{"`+pkgName+`":"^1.0.0","unrelated":"^2.0.0"}}`)

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.ChangedPackageJSON {
		t.Fatal("ChangedPackageJSON = false, want true")
	}

	raw := readPackageJSON(t, home)
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatalf("Unmarshal post-removal package.json: %v (raw=%s)", err, raw)
	}
	if _, present := manifest.Dependencies[pkgName]; present {
		t.Fatalf("pkg still in dependencies: %#v", manifest.Dependencies)
	}
	if _, present := manifest.DevDependencies[pkgName]; present {
		t.Fatalf("pkg still in devDependencies: %#v", manifest.DevDependencies)
	}
	if manifest.DevDependencies["unrelated"] != "^2.0.0" {
		t.Fatalf("unrelated devDep gone: %#v", manifest.DevDependencies)
	}
	// Spec: keep the keys present even when empty.
	if manifest.Dependencies == nil {
		t.Fatalf("dependencies key disappeared entirely (raw=%s)", raw)
	}
}

func TestUninstallPackageJSONMissing(t *testing.T) {
	home := t.TempDir()
	// tui.json for layer 1, no package.json for layer 2
	writeTUIConfig(t, home, []string{"opencode-subagent-statusline"})

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.ChangedPackageJSON {
		t.Fatal("ChangedPackageJSON = true, want false when package.json missing")
	}
	// Layer 1 should have succeeded, layer 3/4 no-op.
	if !res.ChangedTUI {
		t.Fatal("ChangedTUI = false, want true (pkg still present in tui.json)")
	}
}

// ─── 5. Layer 3: node_modules ──────────────────────────────────────────────

func TestUninstallNodeModulesRemoval(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName})

	nmDir := filepath.Join(home, ".config", "opencode", "node_modules", pkgName)
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dummy := filepath.Join(nmDir, "index.js")
	if err := os.WriteFile(dummy, []byte("module.exports = 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.ChangedNodeModules {
		t.Fatal("ChangedNodeModules = false, want true")
	}
	if res.NodeModulesPath != nmDir {
		t.Fatalf("NodeModulesPath = %q, want %q", res.NodeModulesPath, nmDir)
	}
	if _, err := os.Stat(nmDir); !os.IsNotExist(err) {
		t.Fatalf("expected node_modules dir gone, stat err = %v", err)
	}
}

func TestUninstallNodeModulesMissing(t *testing.T) {
	home := t.TempDir()
	writeTUIConfig(t, home, []string{"opencode-subagent-statusline"})

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.ChangedNodeModules {
		t.Fatal("ChangedNodeModules = true, want false when directory absent")
	}
	if res.NodeModulesPath != "" {
		t.Fatalf("NodeModulesPath = %q, want empty", res.NodeModulesPath)
	}
}

// ─── 6. Layer 4: cache entries ─────────────────────────────────────────────

func TestUninstallCacheEntryRemoval(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName})
	createdAt := writeCacheEntry(t, home, pkgName)

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.CacheEntryRemoved != createdAt {
		t.Fatalf("CacheEntryRemoved = %q, want %q", res.CacheEntryRemoved, createdAt)
	}
	if _, err := os.Stat(res.CacheEntryRemoved); !os.IsNotExist(err) {
		t.Fatalf("expected cache dir gone, stat err = %v", err)
	}
}

func TestUninstallCacheEntryNestedFilesRemoval(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName})
	createdAt := writeCacheEntry(t, home, pkgName)
	nested := filepath.Join(createdAt, "subdir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "leaf"), []byte("leaf"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.CacheEntryRemoved != createdAt {
		t.Fatalf("CacheEntryRemoved = %q, want %q", res.CacheEntryRemoved, createdAt)
	}
	if _, err := os.Stat(res.CacheEntryRemoved); !os.IsNotExist(err) {
		t.Fatalf("expected cache dir gone, stat err = %v", err)
	}
}

func TestUninstallCacheEntryMissing(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName})
	// ~/.cache/opencode/packages/<pkg>@latest does not exist: layer 4 must no-op silently.

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.CacheEntryRemoved != "" {
		t.Fatalf("CacheEntryRemoved = %q, want empty", res.CacheEntryRemoved)
	}
}

func TestUninstallCacheOnlyTouchesExactPath(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName})

	cacheRoot := filepath.Join(home, ".cache", "opencode", "packages")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// Sibling entries that share a name prefix or live next to <pkg>@latest
	// must survive — the engine only targets the exact <pkg>@latest path.
	siblings := []string{
		pkgName + "@0.5.2",
		"opencode-sdd-engram-manage@latest",
		pkgName + ".json",
	}
	for _, name := range siblings {
		if err := os.MkdirAll(filepath.Join(cacheRoot, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	for _, name := range siblings {
		path := filepath.Join(cacheRoot, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("sibling %s should survive, stat err = %v", path, err)
		}
	}
}

// ─── 7. GentleLogo ─────────────────────────────────────────────────────────

func TestUninstallGentleLogoRemovesTSXAndTUI(t *testing.T) {
	home := t.TempDir()
	// Install-equivalent setup: write tui.json with the absolute .tsx path,
	// and the .tsx file itself.
	pluginDir := filepath.Join(home, ".config", "opencode", "tui-plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsxPath := filepath.Join(pluginDir, gentleLogoPluginFile)
	if err := os.WriteFile(tsxPath, []byte("// fake plugin content"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTUIConfig(t, home, []string{tsxPath})

	res, err := Uninstall(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.ChangedTUI {
		t.Fatal("ChangedTUI = false, want true (entry should be removed)")
	}
	if res.TSXPath != tsxPath {
		t.Fatalf("TSXPath = %q, want %q", res.TSXPath, tsxPath)
	}
	if _, err := os.Stat(tsxPath); !os.IsNotExist(err) {
		t.Fatalf("expected .tsx file gone, stat err = %v", err)
	}

	// Layers 2/3/4 must NOT have run for GentleLogo.
	if res.ChangedPackageJSON {
		t.Fatal("ChangedPackageJSON = true, want false (GentleLogo skips layer 2)")
	}
	if res.ChangedNodeModules {
		t.Fatal("ChangedNodeModules = true, want false (GentleLogo skips layer 3)")
	}
	if res.CacheEntryRemoved != "" {
		t.Fatalf("CacheEntryRemoved = %q, want empty (GentleLogo skips layer 4)", res.CacheEntryRemoved)
	}
}

func TestUninstallGentleLogoMissingTSX(t *testing.T) {
	home := t.TempDir()
	// tui.json references the .tsx, but the file does not exist on disk.
	pluginDir := filepath.Join(home, ".config", "opencode", "tui-plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsxPath := filepath.Join(pluginDir, gentleLogoPluginFile)
	writeTUIConfig(t, home, []string{tsxPath})

	res, err := Uninstall(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.ChangedTUI {
		t.Fatal("ChangedTUI = false, want true (entry still gets removed even without the file)")
	}
	if res.TSXPath != "" {
		t.Fatalf("TSXPath = %q, want empty (no file existed to remove)", res.TSXPath)
	}
}

// ─── 8. Idempotency over the full setup ────────────────────────────────────

func TestUninstallIdempotent(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"

	writeTUIConfig(t, home, []string{pkgName, "unrelated-plugin"})
	writePackageJSON(t, home, `{"dependencies":{"`+pkgName+`":"^1.0.0"},"devDependencies":{"`+pkgName+`":"^1.0.0"}}`)
	nmDir := filepath.Join(home, ".config", "opencode", "node_modules", pkgName)
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".cache", "opencode", "packages", pkgName+"@latest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cache", "opencode", "packages", pkgName+"@latest", "data"), []byte(`{"version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("first Uninstall() error = %v", err)
	}
	if !first.ChangedTUI {
		t.Fatal("first run: ChangedTUI = false, want true")
	}
	if !first.ChangedPackageJSON {
		t.Fatal("first run: ChangedPackageJSON = false, want true")
	}
	if !first.ChangedNodeModules {
		t.Fatal("first run: ChangedNodeModules = false, want true")
	}
	if first.CacheEntryRemoved == "" {
		t.Fatal("first run: CacheEntryRemoved = '', want non-empty")
	}

	second, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("second Uninstall() error = %v", err)
	}
	if second.ChangedTUI {
		t.Fatal("second run: ChangedTUI = true, want false (idempotent)")
	}
	if second.ChangedPackageJSON {
		t.Fatal("second run: ChangedPackageJSON = true, want false (idempotent)")
	}
	if second.ChangedNodeModules {
		t.Fatal("second run: ChangedNodeModules = true, want false (idempotent)")
	}
	if second.CacheEntryRemoved != "" {
		t.Fatalf("second run: CacheEntryRemoved = %q, want empty", second.CacheEntryRemoved)
	}
	if second.NodeModulesPath != "" || second.TSXPath != "" {
		t.Fatalf("second run: leaked path fields = %+v", second)
	}

	// Unrelated plugin should survive both runs.
	if got := readTUIPlugins(t, home); len(got) != 1 || got[0] != "unrelated-plugin" {
		t.Fatalf("tui.json plugin list after two runs = %#v, want only unrelated-plugin", got)
	}
}

// TestUninstallRemovesAllDuplicateTUIEntries covers the F5 regression: if the
// same package appears twice in tui.json's plugin[] list, both occurrences
// must be removed, not just the first one.
func TestUninstallRemovesAllDuplicateTUIEntries(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName, "unrelated-plugin", pkgName})

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !res.ChangedTUI {
		t.Fatal("res.ChangedTUI = false, want true (duplicates were present)")
	}
	got := readTUIPlugins(t, home)
	want := []string{"unrelated-plugin"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("tui.json plugin list after uninstall = %#v, want %#v", got, want)
	}
}

// ─── 9. Rollback ───────────────────────────────────────────────────────────

// TestUninstallRollbackOnFailure forces layer 2 (package.json write) to fail
// and verifies that the engine restores layer 1's tui.json from the journal.
//
// Failure injection: we replace package.json's path with a DIRECTORY instead
// of a regular file. uninstallPackageJSON's first os.ReadFile then returns
// "is a directory" (EISDIR) on POSIX; on Windows the journal's Capture is
// also happy but filemerge.WriteFileAtomic's readComparableFile bails out
// when it cannot stat a directory as a regular file. Either way, layer 2
// errors out before any disk write and Uninstall invokes rollback.
//
// We deliberately do NOT rely on chmod-0500 on the file: filemerge.WriteFileAtomic
// uses os.CreateTemp + os.Rename, both of which bypass POSIX read-only bits
// (the rename overwrites unconditionally) and which on Windows the
// atomic-writer self-heals via Chmod on the parent directory.
func TestUninstallRollbackOnFailure(t *testing.T) {
	home := t.TempDir()
	pkgName := "opencode-subagent-statusline"
	writeTUIConfig(t, home, []string{pkgName, "unrelated-plugin"})
	pjPath := filepath.Join(home, ".config", "opencode", "package.json")
	// Put a directory where package.json should be: every read or write attempt
	// against it returns EISDIR/equivalent, so the journal captures tui.json
	// successfully but layer 2's Write fails before any disk mutation.
	if err := os.MkdirAll(pjPath, 0o755); err != nil {
		t.Fatal(err)
	}
	originalTUI := readTUIPlugins(t, home)

	res, err := Uninstall(home, model.OpenCodePluginSubAgentStatusline)
	if err == nil {
		t.Fatal("Uninstall() error = nil, want failure from non-file package.json")
	}

	if !strings.Contains(err.Error(), "layer 2") {
		t.Fatalf("error %q does not mention the failed stage (layer 2)", err)
	}
	if res.PluginID != model.OpenCodePluginSubAgentStatusline {
		t.Fatalf("res.PluginID = %q, want %q", res.PluginID, model.OpenCodePluginSubAgentStatusline)
	}
	if res.ChangedTUI {
		t.Fatal("res.ChangedTUI = true after rollback, want false (tui.json was restored)")
	}

	rolledBack := readTUIPlugins(t, home)
	if len(rolledBack) != len(originalTUI) {
		t.Fatalf("tui.json plugin list size mismatch after rollback: got=%v want=%v", rolledBack, originalTUI)
	}
	for i := range originalTUI {
		if rolledBack[i] != originalTUI[i] {
			t.Fatalf("tui.json not rolled back: got=%v want=%v", rolledBack, originalTUI)
		}
	}
}

// TestUninstallRollbackWrapsRestoreError verifies that when journal.Restore
// fails (because a captured file has been clobbered with a symlink between
// Capture and the failure), the rollback wraps the restore error so the
// caller can tell restoration was also broken. We exercise this directly by
// engineering a Restore failure against a journal that mirrors what
// Uninstall would have produced after layer 1 succeeded.
func TestUninstallRollbackWrapsRestoreError(t *testing.T) {
	home := t.TempDir()
	tuiPath := filepath.Join(home, ".config", "opencode", "tui.json")
	if err := os.MkdirAll(filepath.Dir(tuiPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tuiPath, []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["orig"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate the state Uninstall leaves the journal in after layer 1 wrote
	// successfully: <tuiPath, original bytes> is captured, the file on disk is
	// the rewritten version. Then we clobber the on-disk file with a symlink
	// to make a subsequent Restore fail (filemerge.WriteFileAtomic rejects
	// symlinked targets via readComparableFile).
	journal := mutationjournal.New(home)
	if err := journal.Capture(tuiPath); err != nil {
		t.Fatalf("journal.Capture() error = %v", err)
	}
	if err := os.WriteFile(tuiPath, []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Drop the file and replace it with a symlink that points nowhere;
	// readComparableFile then returns "refusing to read symlink" and
	// filemerge.WriteFileAtomic bails out, producing a non-nil Restore error.
	if err := os.Remove(tuiPath); err != nil {
		t.Fatal(err)
	}
	linkTarget := filepath.Join(t.TempDir(), "nowhere.txt")
	if err := os.Symlink(linkTarget, tuiPath); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tuiPath) })

	partial := UninstallResult{PluginID: "x", ChangedTUI: true}
	original := errors.New("layer 2 boom")
	res, wrapped := rollbackUninstall(journal, model.OpenCodeCommunityPluginID("x"), partial, original, "layer 2", false)

	if !errors.Is(wrapped, original) {
		t.Fatalf("rollback did not wrap original error: wrapped = %v", wrapped)
	}
	if res.PluginID != model.OpenCodeCommunityPluginID("x") {
		t.Fatalf("res.PluginID = %q, want %q (PluginID is always overridden on rollback)", res.PluginID, "x")
	}
	if !res.ChangedTUI {
		t.Fatal("res.ChangedTUI = false after rollback with Restore failure, want true (file modification persists on disk when Restore fails)")
	}
	if !strings.Contains(wrapped.Error(), "restore journal after layer 2") {
		t.Fatalf("wrapped error %q does not mention the failed stage", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "refusing to read symlink") {
		t.Fatalf("wrapped error %q does not wrap the restore failure", wrapped)
	}
}

// ─── 10. Journal path validation ───────────────────────────────────────────

// TestUninstallJournalRejectsOutsideRoot verifies the defense the
// mutationjournal provides against path traversal: the journal used by
// Uninstall rejects any path outside homeDir. We exercise this directly with
// the same construction Uninstall uses (mutationjournal.New(homeDir)). It is
// impossible to drive an out-of-root path through Uninstall's public API
// because every path Uninstall computes (tui.json, package.json, the .tsx
// file) is derived from homeDir and therefore within the journal's root —
// so this test pins down the contract that makes Uninstall safe by
// construction.
func TestUninstallJournalRejectsOutsideRoot(t *testing.T) {
	home := t.TempDir()

	insideFile := filepath.Join(home, ".config", "opencode", "tui.json")
	if err := os.MkdirAll(filepath.Dir(insideFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(insideFile, []byte(`{"$schema":"https://opencode.ai/tui.json"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "package.json")
	if err := os.WriteFile(outsideFile, []byte(`{"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	journal := mutationjournal.New(home)
	if err := journal.Capture(insideFile); err != nil {
		t.Fatalf("journal.Capture(inside) error = %v, want nil", err)
	}
	err := journal.Capture(outsideFile)
	if err == nil {
		t.Fatal("journal.Capture(outside) error = nil, want outside-roots rejection")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Fatalf("error %q does not mention outside-roots", err)
	}
}
