//go:build windows

package shellinstaller

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeReuseBackup builds a retired installation with the reusable component
// trees and one retained archive per pinned artifact, returning the backup
// path and the archive bytes keyed by archive name.
func writeReuseBackup(t *testing.T, archives ...string) (string, map[string][]byte) {
	t.Helper()
	backup := filepath.Join(t.TempDir(), "installed")
	for _, dir := range []string{"runtime/node", "runtime/go/bin", "runtime/archives", "prefix", "agent/bin", "source", "config"} {
		if err := os.MkdirAll(filepath.Join(backup, filepath.FromSlash(dir)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(backup, "config", "gentle-shell.json"), []byte(`{"home":"isolated"}`), 0600); err != nil {
		t.Fatal(err)
	}
	retained := map[string][]byte{}
	for _, name := range archives {
		data := []byte("retained " + name)
		retained[name] = data
		if err := os.WriteFile(filepath.Join(backup, "runtime", "archives", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return backup, retained
}

func assertStagedArchives(t *testing.T, stage string, retained map[string][]byte, names ...string) {
	t.Helper()
	for _, name := range names {
		actual, err := os.ReadFile(filepath.Join(stage, "runtime", "archives", name))
		if err != nil {
			t.Fatalf("retained %s was not carried into the stage: %v", name, err)
		}
		if !bytes.Equal(actual, retained[name]) {
			t.Fatalf("staged %s differs from the retained archive", name)
		}
	}
}

// Issue #6236: verification re-reads runtime/archives for every reused
// runtime and helper, so a component update that reuses Node and the SDK
// (for example a supervisor-only update) must carry node.zip, fd.zip and
// rg.zip from the retired installation; none of them is acquired again.
func TestUserWindowsReuseComponentsCarriesRetainedArchives(t *testing.T) {
	backup, retained := writeReuseBackup(t, "node.zip", "fd.zip", "rg.zip")
	stage := filepath.Join(t.TempDir(), "stage")
	if err := os.MkdirAll(filepath.Join(stage, "runtime", "archives"), 0700); err != nil || os.MkdirAll(filepath.Join(stage, "config"), 0700) != nil {
		t.Fatal(err)
	}
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSupervisor})
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReuseComponents(context.Background(), stage, backup, &userWindowsManifest{}, UserInstallRequest{}, scope, false); err != nil {
		t.Fatal(err)
	}
	assertStagedArchives(t, stage, retained, "node.zip", "fd.zip", "rg.zip")
	binding, err := os.ReadFile(filepath.Join(stage, "config", "gentle-shell.json"))
	if err != nil || string(binding) != `{"home":"isolated"}` {
		t.Fatal("reused SDK provisioning binding config/gentle-shell.json was not carried:", err)
	}
	if _, err := os.Lstat(filepath.Join(stage, "runtime", "archives", "go.zip")); !os.IsNotExist(err) {
		t.Fatal("go.zip must not be carried without Go runtime reuse:", err)
	}
}

// Reusing the recorded pinned Go runtime also reuses its retained go.zip,
// which verification compares against runtime/go/bin/go.exe.
func TestUserWindowsReuseComponentsCarriesRetainedGoArchive(t *testing.T) {
	backup, retained := writeReuseBackup(t, "node.zip", "go.zip")
	stage := filepath.Join(t.TempDir(), "stage")
	if err := os.MkdirAll(filepath.Join(stage, "runtime", "archives"), 0700); err != nil || os.MkdirAll(filepath.Join(stage, "config"), 0700) != nil {
		t.Fatal(err)
	}
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSDK})
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReuseComponents(context.Background(), stage, backup, &userWindowsManifest{}, UserInstallRequest{}, scope, true); err != nil {
		t.Fatal(err)
	}
	assertStagedArchives(t, stage, retained, "node.zip", "go.zip")
}

// An SDK-scoped update re-acquires the helpers and re-provisions the SDK, so
// it never carries the retired fd.zip/rg.zip or the provisioning binding,
// which provisioning creates exclusively.
func TestUserWindowsReuseComponentsLeavesRequestedArchives(t *testing.T) {
	backup, _ := writeReuseBackup(t, "node.zip", "fd.zip", "rg.zip")
	stage := filepath.Join(t.TempDir(), "stage")
	if err := os.MkdirAll(filepath.Join(stage, "runtime", "archives"), 0700); err != nil || os.MkdirAll(filepath.Join(stage, "config"), 0700) != nil {
		t.Fatal(err)
	}
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSDK + "," + UserComponentNode})
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReuseComponents(context.Background(), stage, backup, &userWindowsManifest{}, UserInstallRequest{}, scope, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"node.zip", "fd.zip", "rg.zip"} {
		if _, err := os.Lstat(filepath.Join(stage, "runtime", "archives", name)); !os.IsNotExist(err) {
			t.Fatalf("requested component archive %s was carried from the retired installation: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(stage, "config", "gentle-shell.json")); !os.IsNotExist(err) {
		t.Fatal("re-provisioned SDK binding was carried from the retired installation:", err)
	}
}

func newReuseStage(t *testing.T) string {
	t.Helper()
	stage := filepath.Join(t.TempDir(), "stage")
	for _, dir := range []string{"runtime/archives", "config"} {
		if err := os.MkdirAll(filepath.Join(stage, filepath.FromSlash(dir)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return stage
}

// A Go reuse whose retired tree holds no go.zip keeps its prior outcome
// (#5959) and never invents the archive.
func TestUserWindowsReuseComponentsSkipsAbsentGoArchive(t *testing.T) {
	backup, retained := writeReuseBackup(t, "node.zip")
	stage := newReuseStage(t)
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSDK})
	if err != nil {
		t.Fatal(err)
	}
	if err := userWindowsReuseComponents(context.Background(), stage, backup, &userWindowsManifest{}, UserInstallRequest{}, scope, true); err != nil {
		t.Fatal(err)
	}
	assertStagedArchives(t, stage, retained, "node.zip")
	if _, err := os.Lstat(filepath.Join(stage, "runtime", "archives", "go.zip")); !os.IsNotExist(err) {
		t.Fatal("absent go.zip must not appear in the stage:", err)
	}
}

// A reused SDK without its provisioning binding fails at reuse, never later
// as an unqualified published tree.
func TestUserWindowsReuseComponentsRequiresBindingForReusedSDK(t *testing.T) {
	backup, _ := writeReuseBackup(t, "node.zip", "fd.zip", "rg.zip")
	if err := os.Remove(filepath.Join(backup, "config", "gentle-shell.json")); err != nil {
		t.Fatal(err)
	}
	stage := newReuseStage(t)
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSupervisor})
	if err != nil {
		t.Fatal(err)
	}
	err = userWindowsReuseComponents(context.Background(), stage, backup, &userWindowsManifest{}, UserInstallRequest{}, scope, false)
	if err == nil || !strings.Contains(err.Error(), "SDK provisioning binding reuse") {
		t.Fatalf("want the binding reuse error, got %v", err)
	}
}
