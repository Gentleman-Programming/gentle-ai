package engram

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/filecoord"
)

// ─── #1635 B: cooperative single-writer coordination lock ────────────────────

func TestInjectAntigravityHeldLeaseRefusesWithoutManagedWrites(t *testing.T) {
	home, globalPath, settingsPath, pluginPath, pluginMCPPath, hooksPath := antigravityWriteFixture(t)
	lease, lockErr := acquireAntigravityCoordinationLock(home)
	if lockErr != nil {
		t.Fatalf("acquireAntigravityCoordinationLock(%q) error = %v", home, lockErr)
	}
	before := readAntigravityFile(t, globalPath)

	result, err := Inject(home, antigravityAdapter())

	if err == nil || !errors.Is(err, filecoord.ErrBusy) {
		t.Fatalf("Inject(antigravity) error = %v, want it to wrap filecoord.ErrBusy", err)
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Fatalf("error must carry retry advice: %v", err)
	}
	// No managed writes: nothing beyond coordination metadata was touched.
	assertAntigravityLandedFiles(t, result, nil)
	if got := readAntigravityFile(t, globalPath); !bytes.Equal(got, before) {
		t.Fatalf("global MCP config must be untouched while the lease is held\nwant:\n%s\ngot:\n%s", before, got)
	}
	for _, path := range []string{settingsPath, pluginPath, pluginMCPPath, hooksPath} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("%q must not be created while the lease is held; stat err = %v", path, statErr)
		}
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("lease.Release() error = %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("second lease.Release() must be idempotent; error = %v", err)
	}

	retry, retryErr := Inject(home, antigravityAdapter())
	if retryErr != nil {
		t.Fatalf("Inject(antigravity) after release error = %v", retryErr)
	}
	for _, path := range []string{pluginPath, pluginMCPPath, hooksPath} {
		assertAntigravityCanonicalPluginAsset(t, path)
	}
	if raw := readAntigravityFile(t, settingsPath); strings.TrimSpace(string(raw)) != "{}" {
		t.Fatalf("settings on disk = %q, want empty JSON object", raw)
	}
	if !retry.Changed {
		t.Fatalf("retry injection must report its mutations")
	}
}

func TestInjectAntigravityCoordinationLockAliasesShareKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	home := t.TempDir()
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(home, alias); err != nil {
		t.Fatalf("Symlink(%q) error = %v", alias, err)
	}
	lease, err := acquireAntigravityCoordinationLock(home)
	if err != nil {
		t.Fatalf("acquireAntigravityCoordinationLock(%q) error = %v", home, err)
	}
	t.Cleanup(func() { _ = lease.Release() })

	if _, err := acquireAntigravityCoordinationLock(alias); err == nil || !errors.Is(err, filecoord.ErrBusy) {
		t.Fatalf("acquire through the symlink alias error = %v, want filecoord.ErrBusy (aliases must share one key)", err)
	}
}

func TestAntigravityCoordinationLockResolvesMissingLeafThroughExistingAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	home := t.TempDir()
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(home, alias); err != nil {
		t.Fatalf("Symlink(%q) error = %v", alias, err)
	}
	missing := filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram")
	missingAlias := filepath.Join(alias, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram")
	homeTarget, err := antigravityLockTarget(missing)
	if err != nil {
		t.Fatalf("antigravityLockTarget(existing home) error = %v", err)
	}
	aliasTarget, err := antigravityLockTarget(missingAlias)
	if err != nil {
		t.Fatalf("antigravityLockTarget(missing leaf under alias) error = %v", err)
	}
	if homeTarget != aliasTarget {
		t.Fatalf("lock targets must share one key across aliases\ngot  %q\nwant %q", aliasTarget, homeTarget)
	}
	physicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", home, err)
	}
	if !strings.HasPrefix(homeTarget, physicalHome+string(filepath.Separator)) {
		t.Fatalf("lock target %q must canonicalize under the physical home %q", homeTarget, physicalHome)
	}
}

func TestAntigravityCoordinationLockWorkspaceDistinctFromGlobal(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", workspace, err)
	}
	workspaceLease, err := acquireAntigravityCoordinationLock(workspace)
	if err != nil {
		t.Fatalf("acquireAntigravityCoordinationLock(workspace) error = %v", err)
	}
	t.Cleanup(func() { _ = workspaceLease.Release() })

	// Distinct physical config homes coordinate independently.
	homeLease, err := acquireAntigravityCoordinationLock(home)
	if err != nil {
		t.Fatalf("acquireAntigravityCoordinationLock(home) error = %v; workspace and global keys must be distinct", err)
	}
	t.Cleanup(func() { _ = homeLease.Release() })
}

func TestInjectAntigravityMalformedGlobalCreatesZeroPaths(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", cliDir, err)
	}
	mcpPath := filepath.Join(cliDir, "mcp_config.json")
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
	}

	_, err := Inject(home, antigravityAdapter())
	if err == nil || !strings.Contains(err.Error(), "Antigravity global MCP config") {
		t.Fatalf("Inject(antigravity) error = %v, want the global MCP config prevalidation error", err)
	}
	// Prevalidation is side-effect-free: not even the lock root may exist.
	want := map[string]bool{
		filepath.Join(".gemini"):                                       true,
		filepath.Join(".gemini", "antigravity-cli"):                    true,
		filepath.Join(".gemini", "antigravity-cli", "mcp_config.json"): true,
	}
	got := map[string]bool{}
	err = filepath.Walk(home, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == home {
			return nil
		}
		got[strings.TrimPrefix(path, home+string(filepath.Separator))] = true
		return nil
	})
	if err != nil {
		t.Fatalf("Walk(%q) error = %v", home, err)
	}
	for path := range got {
		if !want[path] {
			t.Fatalf("unexpected path %q created by injection; want exactly %v", path, want)
		}
	}
	for path := range want {
		if !got[path] {
			t.Fatalf("expected path %q missing; got %v", path, got)
		}
	}
}

func TestInjectAntigravityReleasesLockAfterFailure(t *testing.T) {
	home, globalPath, _, pluginPath, pluginMCPPath, hooksPath := antigravityWriteFixture(t)
	orig := writeAntigravityFileAtomic
	writeAntigravityFileAtomic = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
		result, writeErr := orig(path, content, perm)
		if writeErr == nil && filepath.Base(path) == "hooks.json" {
			writeErr = errAntigravityWriteFault
		}
		return result, writeErr
	}
	if _, err := Inject(home, antigravityAdapter()); err == nil || !errors.Is(err, errAntigravityWriteFault) {
		t.Fatalf("first Inject error = %v, want the injected write fault", err)
	}
	writeAntigravityFileAtomic = orig

	// The failed pass must have released the lease: the retry acquires and
	// completes the full ownership transfer.
	retry, retryErr := Inject(home, antigravityAdapter())
	if retryErr != nil {
		t.Fatalf("Inject(antigravity) after a failed pass error = %v; the coordination lock must be released on failure", retryErr)
	}
	for _, path := range []string{pluginPath, pluginMCPPath, hooksPath} {
		assertAntigravityCanonicalPluginAsset(t, path)
	}
	if raw := readAntigravityFile(t, globalPath); bytes.Contains(raw, []byte("engram")) || !bytes.Contains(raw, []byte("context7")) {
		t.Fatalf("completed transfer must remove the managed Engram duplicate and keep context7; got:\n%s", raw)
	}
	if !retry.Changed {
		t.Fatalf("retry injection must report its mutations")
	}
}
