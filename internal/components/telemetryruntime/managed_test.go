package telemetryruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/mutationjournal"
)

func TestOpenCodeTelemetryApprovedPriorAssetUpgrade(t *testing.T) {
	dir := t.TempDir()
	prior, err := os.ReadFile("testdata/telemetry-runtime-a9cab7dd.ts")
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(prior)); digest != priorPluginDigestA9cab7dd {
		t.Fatalf("prior asset digest = %s, want %s", digest, priorPluginDigestA9cab7dd)
	}
	writeManagedTelemetryFixture(t, dir, prior)

	changed, err := Reconcile(dir)
	if err != nil {
		t.Fatalf("upgrade approved prior asset: %v", err)
	}
	if len(changed) != 2 {
		t.Fatalf("upgrade changed %d files, want plugin and manifest", len(changed))
	}
	paths := ManagedPaths(dir)
	current := assets.MustRead("opencode/plugins/telemetry-runtime.ts")
	if plugin, err := os.ReadFile(paths[0]); err != nil || string(plugin) != current {
		t.Fatalf("plugin was not upgraded: %v", err)
	}
	manifestBytes, err := os.ReadFile(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	var manifest managedManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.File.After != current || manifest.File.AfterHash != fmt.Sprintf("%x", sha256.Sum256([]byte(current))) {
		t.Fatal("ownership manifest was not refreshed to the current asset")
	}
	if err := CheckManaged(dir); err != nil {
		t.Fatalf("upgraded asset is not currently owned: %v", err)
	}
	customPath := filepath.Join(dir, "plugins", "custom.ts")
	if err := os.WriteFile(customPath, []byte("custom"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveManaged(dir)
	if err != nil || len(removed) != 2 {
		t.Fatalf("remove upgraded owned asset: %v, paths=%v", err, removed)
	}
	for _, path := range paths {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("owned path remains after uninstall: %s: %v", path, err)
		}
	}
	if custom, err := os.ReadFile(customPath); err != nil || string(custom) != "custom" {
		t.Fatalf("uninstall changed unrelated file: %q, %v", custom, err)
	}
}

func TestOpenCodeTelemetryUnapprovedPriorAssetConflicts(t *testing.T) {
	dir := t.TempDir()
	unapproved := []byte(ownershipMarker + "// unapproved historical content\n")
	writeManagedTelemetryFixture(t, dir, unapproved)
	paths := ManagedPaths(dir)

	if err := CheckManaged(dir); err == nil {
		t.Fatal("unapproved prior asset passed ownership validation")
	}
	if _, err := Reconcile(dir); err == nil {
		t.Fatal("unapproved prior asset was upgraded")
	}
	if _, err := RemoveManaged(dir); err == nil {
		t.Fatal("unapproved prior asset was removed")
	}
	if plugin, err := os.ReadFile(paths[0]); err != nil || !bytes.Equal(plugin, unapproved) {
		t.Fatalf("unapproved plugin was not preserved: %v", err)
	}
}

func TestOpenCodeTelemetryCurrentAssetRemainsOwned(t *testing.T) {
	dir := t.TempDir()
	if changed, err := Reconcile(dir); err != nil || len(changed) != 2 {
		t.Fatalf("install current asset: changed=%v err=%v", changed, err)
	}
	if changed, err := Reconcile(dir); err != nil || len(changed) != 0 {
		t.Fatalf("current asset is not idempotent: changed=%v err=%v", changed, err)
	}
	if err := CheckManaged(dir); err != nil {
		t.Fatalf("current asset is not owned: %v", err)
	}
}

func writeManagedTelemetryFixture(t *testing.T, dir string, content []byte) {
	t.Helper()
	paths := ManagedPaths(dir)
	if err := os.MkdirAll(filepath.Dir(paths[0]), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[0], content, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := managedManifest{Schema: ownershipSchema, File: mutationjournal.OwnedFile{
		After: string(content), AfterHash: fmt.Sprintf("%x", sha256.Sum256(content)), Overlay: false, Mode: 0o644,
	}}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[1], append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Exercise the retained rollback guard with a synthetic changed prior image,
// independently of the approved-digest upgrade path.
func TestOpenCodeTelemetryOwnedUpdateRollback(t *testing.T) {
	for _, edited := range []int{0, 1} {
		t.Run(fmt.Sprint(edited), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Reconcile(dir); err != nil {
				t.Fatal(err)
			}
			paths := ManagedPaths(dir)
			files := make([]guardedFile, 2)
			before := make([][]byte, 2)
			for i, path := range paths {
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[i] = []byte(fmt.Sprintf("previous approved fixture bytes %d", i))
				if err := os.WriteFile(path, before[i], 0600); err != nil {
					t.Fatal(err)
				}
				info, _ := os.Lstat(path)
				files[i] = guardedFile{configDir: dir, path: path, journal: mutationjournal.New(dir), expected: after, mode: info.Mode()}
				if _, err := files[i].journal.WriteWithMode(path, after, info.Mode()); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(paths[edited], []byte("concurrent custom edit"), 0600); err != nil {
				t.Fatal(err)
			}
			for i := range files {
				err := files[i].restore()
				if i == edited {
					if err == nil {
						t.Fatal("changed prior image was not protected")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				data, _ := os.ReadFile(paths[i])
				want := before[i]
				if i == edited {
					want = []byte("concurrent custom edit")
				}
				if !bytes.Equal(data, want) {
					t.Fatal("wrong rollback content")
				}
			}
		})
	}
}

func TestOpenCodeTelemetryManagedModeEquivalence(t *testing.T) {
	for _, tt := range []struct {
		name     string
		goos     string
		expected os.FileMode
		observed os.FileMode
		want     bool
	}{
		{"exact writable on Unix", "linux", 0644, 0644, true},
		{"read-only drift on Unix", "linux", 0644, 0444, false},
		{"different writable mode on Unix", "linux", 0644, 0640, false},
		{"widened writable plugin on Windows", "windows", 0644, 0666, true},
		{"widened writable manifest on Windows", "windows", 0600, 0666, true},
		{"read-only expected file remains distinct on Windows", "windows", 0444, 0666, false},
		{"other writable modes remain distinct on Windows", "windows", 0644, 0664, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := managedModeMatchesForOS(tt.goos, tt.expected, tt.observed); got != tt.want {
				t.Errorf("managedModeMatchesForOS(%q, %#o, %#o) = %t, want %t", tt.goos, tt.expected, tt.observed, got, tt.want)
			}
		})
	}
}

func TestOpenCodeTelemetryManagedModeLifecycle(t *testing.T) {
	dir := t.TempDir()
	changed, rollback, err := ReconcileWithRollback(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Fatalf("first reconcile changed %d files, want 2", len(changed))
	}
	if err := CheckManaged(dir); err != nil {
		t.Fatalf("validate reconciled files: %v", err)
	}
	if err := rollback(); err != nil {
		t.Fatalf("rollback reconciled files: %v", err)
	}
	for _, path := range ManagedPaths(dir) {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("rollback left managed file %s: %v", path, err)
		}
	}
}

func TestOpenCodeTelemetryManifestStrictness(t *testing.T) {
	for _, kind := range []string{"alias", "nested-alias", "duplicate", "nested-duplicate", "missing", "null", "schema-null", "mode-null", "mode-type", "mode-value", "mode-drift", "metadata-mode", "unknown-pair", "oversized", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Reconcile(dir); err != nil {
				t.Fatal(err)
			}
			paths := ManagedPaths(dir)
			raw, _ := os.ReadFile(paths[1])
			text := string(raw)
			switch kind {
			case "alias":
				text = strings.Replace(text, `"schema"`, `"Schema"`, 1)
			case "nested-alias":
				text = strings.Replace(text, `"afterHash"`, `"AfterHash"`, 1)
			case "nested-duplicate":
				text = strings.Replace(text, `"overlay":`, `"overlay":true,"overlay":`, 1)
			case "schema-null":
				text = strings.Replace(text, `"schema": "`+ownershipSchema+`"`, `"schema":null`, 1)
			case "mode-null":
				text = strings.Replace(text, `"mode": 420`, `"mode":null`, 1)
			case "trailing":
				text += `{}`
			case "duplicate":
				text = strings.Replace(text, `"schema":`, `"schema":"discarded","schema":`, 1)
			case "missing":
				text = strings.Replace(text, `"overlay": false,`, "", 1)
			case "null":
				text = strings.Replace(text, `"overlay": false`, `"overlay": null`, 1)
			case "mode-type":
				text = strings.Replace(text, `"mode": 420`, `"mode": "420"`, 1)
			case "mode-value":
				text = strings.Replace(text, `"mode": 420`, `"mode": 511`, 1)
			case "mode-drift":
				mode := os.FileMode(0600)
				if runtime.GOOS == "windows" {
					mode = 0444
				}
				if err := os.Chmod(paths[0], mode); err != nil {
					t.Fatal(err)
				}
			case "metadata-mode":
				mode := os.FileMode(0644)
				if runtime.GOOS == "windows" {
					mode = 0444
				}
				if err := os.Chmod(paths[1], mode); err != nil {
					t.Fatal(err)
				}
			case "unknown-pair":
				custom := ownershipMarker + "// custom data, not a package asset\n"
				var m managedManifest
				if err := json.Unmarshal(raw, &m); err != nil {
					t.Fatal(err)
				}
				m.File.After = custom
				m.File.AfterHash = fmt.Sprintf("%x", sha256.Sum256([]byte(custom)))
				raw, _ = json.Marshal(m)
				text = string(raw)
				if err := os.WriteFile(paths[0], []byte(custom), 0644); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				text += strings.Repeat(" ", 65537)
			}
			if err := os.WriteFile(paths[1], []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(paths[0])
			if err := CheckManaged(dir); err == nil {
				t.Error("manifest accepted")
			}
			if _, err := Reconcile(dir); err == nil {
				t.Error("unsafe reconcile accepted")
			}
			if _, err := RemoveManaged(dir); err == nil {
				t.Error("unsafe uninstall accepted")
			}
			if after, err := os.ReadFile(paths[0]); err != nil || !bytes.Equal(before, after) {
				t.Error("custom pair not preserved")
			}
		})
	}
}

func TestOpenCodeTelemetryManifestRecordsActualMode(t *testing.T) {
	dir := t.TempDir()
	if _, err := Reconcile(dir); err != nil {
		t.Fatal(err)
	}
	paths := ManagedPaths(dir)
	raw, _ := os.ReadFile(paths[1])
	var m managedManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.File.Mode = 0600
	raw, _ = json.Marshal(m)
	if err := os.Chmod(paths[0], 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[1], raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(paths[1])
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(paths[0])
	if uint32(info.Mode().Perm()) != m.File.Mode {
		t.Fatal("recorded mode differs from actual preserved mode")
	}
	if err := CheckManaged(dir); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeTelemetryManagedLifecycle(t *testing.T) {
	dir := t.TempDir()
	if _, err := Reconcile(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(ManagedPaths(dir)[1])
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ManagedPaths(dir)[1], compact.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if changed, err := Reconcile(dir); err != nil || len(changed) != 1 {
		t.Fatal("metadata refresh", changed, err)
	}
	if changed, err := Reconcile(dir); err != nil || len(changed) != 0 {
		t.Fatal("not idempotent", changed, err)
	}
	paths := ManagedPaths(dir)
	if err := os.Rename(paths[0], filepath.Join(dir, "removed-plugin")); err != nil {
		t.Fatal(err)
	}
	if changed, err := Reconcile(dir); err != nil || len(changed) != 1 {
		t.Fatal("missing asset not repaired", changed, err)
	}
	if removed, err := RemoveManaged(dir); err != nil || len(removed) != 2 {
		t.Fatal("uninstall", removed, err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("owned file remains", path, err)
		}
	}
}

func TestOpenCodeTelemetryManagedPreservesConflicts(t *testing.T) {
	for _, kind := range []string{"unowned", "modified", "symlink", "metadata"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			paths := ManagedPaths(dir)
			if kind == "modified" || kind == "metadata" {
				if _, err := Reconcile(dir); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Dir(paths[0]), 0700); err != nil {
				t.Fatal(err)
			}
			target := paths[0]
			if kind == "metadata" {
				target = paths[1]
			}
			if kind == "symlink" {
				target = filepath.Join(dir, "custom.ts")
				if err := os.WriteFile(target, []byte("personal"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, paths[0]); err != nil {
					t.Skip(err)
				}
			} else if err := os.WriteFile(target, []byte("personal"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Reconcile(dir); err == nil {
				t.Fatal("conflict silently overwritten")
			}
			if _, err := RemoveManaged(dir); err == nil {
				t.Fatal("conflict not reported during uninstall")
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "personal" {
				t.Fatal("custom content changed", err)
			}
		})
	}
}
