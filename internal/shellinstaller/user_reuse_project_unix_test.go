//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A component-scoped update that reuses the SDK (e.g. supervisor-only, the
// scope offered right after a gentle-ai upgrade) must carry every SDK-owned
// path the post-native readback re-reads into the stage: the readback verifies
// the reused global tree against runtime/project and the recorded graph seal,
// and a Main preview tree additionally against the retained Main composition.
// A stage without them failed every SDK-reusing update with
// "ENOENT: lstat .../installed/runtime/project" and, on Main preview, with the
// Main composition verification refusing the missing members.
func TestUserReuseComponentsCarriesSDKProject(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	destination := filepath.Join(parent, "install")
	files := map[string]string{
		"prefix/lib/node_modules/gentle-pi/package.json": `{"name":"gentle-pi"}`,
		"agent/settings.json":                            `{}`,
		"runtime/node/bin/node":                          "node",
		"runtime/project/package.json":                   `{"name":"gentle-shell-owned-seed"}`,
		"runtime/project/package-lock.json":              `{"lockfileVersion":3}`,
		"state/global-graph.json":                        `[{"directory":"lib/gentle-pi"}]`,
	}
	userReuseSDKFixture(t, destination, files, nil)
	scope := userReuseSDKScope(t)
	root := userReuseSDKStage(t, parent)
	req := UserInstallRequest{Destination: destination, Mode: "separate", Scope: UserComponentSupervisor}
	if err := userReuseComponents(ctx, root, req, userReuseSDKManifest(destination), scope); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"runtime/project/package.json",
		"runtime/project/package-lock.json",
		"state/global-graph.json",
	} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("reused SDK stage lacks %s: %v", name, err)
		}
		if !bytes.Equal(got, []byte(files[name])) {
			t.Fatalf("%s differs after reuse: %q", name, got)
		}
	}
	userReuseSDKAssertModes(t, root, files)
}

// A prior-cohort installation (pi-coding-agent 0.99.2) keeps its acquisition
// under runtime/prior and its graph seal in state/prior-global-graph.json;
// both are read by the verify action whenever the recorded tree is prior.
func TestUserReuseComponentsCarriesSDKPriorCohort(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	destination := filepath.Join(parent, "install")
	files := map[string]string{
		"prefix/lib/node_modules/@earendil-works/pi-coding-agent/package.json": `{"name":"@earendil-works/pi-coding-agent","version":"0.99.2"}`,
		"agent/settings.json":             `{}`,
		"runtime/node/bin/node":           "node",
		"runtime/prior/package.json":      `{"name":"gentle-shell-prior-seed"}`,
		"runtime/prior/package-lock.json": `{"lockfileVersion":3}`,
		"state/prior-global-graph.json":   `[{"directory":"lib/@earendil-works/pi-coding-agent"}]`,
	}
	userReuseSDKFixture(t, destination, files, nil)
	scope := userReuseSDKScope(t)
	root := userReuseSDKStage(t, parent)
	req := UserInstallRequest{Destination: destination, Mode: "agent", Scope: UserComponentSupervisor}
	if err := userReuseComponents(ctx, root, req, userReuseSDKManifest(destination), scope); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"runtime/prior/package.json",
		"runtime/prior/package-lock.json",
		"state/prior-global-graph.json",
	} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("reused SDK stage lacks %s: %v", name, err)
		}
		if !bytes.Equal(got, []byte(files[name])) {
			t.Fatalf("%s differs after reuse: %q", name, got)
		}
	}
	userReuseSDKAssertModes(t, root, files)
}

// A Main preview installation retains the snapshot under runtime/main, its
// extracted source and composed package under runtime/main-source and
// runtime/main-sdk, and the runtime/node_modules dependency link. The reused
// SDK keeps the Main declaration, so the committed verification re-runs
// verify-main-sdk-selected against all four; the dependency link must stay a
// symlink with the identical target and never be followed into a copy.
func TestUserReuseComponentsCarriesSDKMainPreview(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	destination := filepath.Join(parent, "install")
	files := map[string]string{
		"prefix/lib/node_modules/gentle-pi/package.json": `{"name":"gentle-pi"}`,
		"agent/settings.json":                            `{}`,
		"runtime/node/bin/node":                          "node",
		"runtime/main/main.json":                         `{"sha":"abc"}`,
		"runtime/main/main.zip":                          "archive-bytes",
		"runtime/main-source/src/index.mjs":              `export {}`,
		"runtime/main-sdk/package.json":                  `{"name":"gentle-shell-main-sdk"}`,
	}
	links := map[string]string{
		"runtime/node_modules": "../prefix/lib/node_modules",
	}
	userReuseSDKFixture(t, destination, files, links)
	scope := userReuseSDKScope(t)
	root := userReuseSDKStage(t, parent)
	req := UserInstallRequest{Destination: destination, Mode: "separate", Scope: UserComponentSupervisor}
	if err := userReuseComponents(ctx, root, req, userReuseSDKManifest(destination), scope); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"runtime/main/main.json",
		"runtime/main/main.zip",
		"runtime/main-source/src/index.mjs",
		"runtime/main-sdk/package.json",
	} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("reused SDK stage lacks %s: %v", name, err)
		}
		if !bytes.Equal(got, []byte(files[name])) {
			t.Fatalf("%s differs after reuse: %q", name, got)
		}
	}
	info, err := os.Lstat(filepath.Join(root, "runtime/node_modules"))
	if err != nil {
		t.Fatalf("reused SDK stage lacks runtime/node_modules: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("runtime/node_modules must stay a symlink, got mode %#x", info.Mode())
	}
	link, err := os.Readlink(filepath.Join(root, "runtime/node_modules"))
	if err != nil || link != links["runtime/node_modules"] {
		t.Fatalf("runtime/node_modules target differs after reuse: %q %v", link, err)
	}
	userReuseSDKAssertModes(t, root, files)
}

// Installations recorded before a path existed have nothing to carry: absent
// optional paths are skipped without creating empty stand-ins in the stage.
func TestUserReuseComponentsSkipsAbsentSDKPaths(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	destination := filepath.Join(parent, "install")
	files := map[string]string{
		"prefix/lib/node_modules/gentle-pi/package.json": `{"name":"gentle-pi"}`,
		"agent/settings.json":                            `{}`,
		"runtime/node/bin/node":                          "node",
	}
	userReuseSDKFixture(t, destination, files, nil)
	scope := userReuseSDKScope(t)
	root := userReuseSDKStage(t, parent)
	req := UserInstallRequest{Destination: destination, Mode: "separate", Scope: UserComponentSupervisor}
	if err := userReuseComponents(ctx, root, req, userReuseSDKManifest(destination), scope); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"runtime/project",
		"runtime/prior",
		"state/global-graph.json",
		"state/prior-global-graph.json",
		"runtime/main",
		"runtime/main-source",
		"runtime/main-sdk",
		"runtime/node_modules",
	} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("absent %s must not be staged, got %v", name, err)
		}
	}
}

// An unexpected entry type at a carried path is refused exactly like an
// unexpected entry inside userReuseTree: a partial reuse never publishes.
func TestUserReuseComponentsRefusesForeignSDKState(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	destination := filepath.Join(parent, "install")
	userReuseSDKFixture(t, destination, map[string]string{
		"prefix/lib/node_modules/gentle-pi/package.json": `{"name":"gentle-pi"}`,
		"agent/settings.json":                            `{}`,
		"runtime/node/bin/node":                          "node",
	}, nil)
	if err := os.MkdirAll(filepath.Join(destination, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(destination, "state", "global-graph.json"), 0600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	scope := userReuseSDKScope(t)
	root := userReuseSDKStage(t, parent)
	req := UserInstallRequest{Destination: destination, Mode: "separate", Scope: UserComponentSupervisor}
	if err := userReuseComponents(ctx, root, req, userReuseSDKManifest(destination), scope); err == nil {
		t.Fatal("a foreign entry at a carried SDK path must be refused")
	}
}

func userReuseSDKFixture(t *testing.T, destination string, files map[string]string, links map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range links {
		path := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
}

func userReuseSDKScope(t *testing.T) userInstallScope {
	t.Helper()
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentSupervisor})
	if err != nil || !scope.reused[UserComponentSDK] {
		t.Fatalf("supervisor-only scope must reuse the SDK: %+v %v", scope, err)
	}
	return scope
}

func userReuseSDKStage(t *testing.T, parent string) string {
	t.Helper()
	root := filepath.Join(parent, "stage", "installed")
	if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func userReuseSDKManifest(destination string) *userManifest {
	return &userManifest{Prefix: filepath.Join(destination, "prefix"), Agent: filepath.Join(destination, "agent")}
}

func userReuseSDKAssertModes(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for _, name := range []string{"state/global-graph.json", "state/prior-global-graph.json", "runtime/main/main.zip"} {
		if _, ok := files[name]; !ok {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			t.Fatalf("%s must stay a private 0600 regular file, got %#x", name, info.Mode())
		}
	}
}

// A Main preview update that reuses the SDK (e.g. Manage's CodeGraph OFF->ON)
// carries an agent that already selects runtime/main-sdk, so the post-native
// readback must verify that selection rather than the stock package; the stock
// verify refused it with "selected settings bindings changed".
func TestUserPostNativeReadbackVerifiesReusedMainSelection(t *testing.T) {
	previous := userMainSDKAction
	t.Cleanup(func() { userMainSDKAction = previous })
	var calls []string
	userMainSDKAction = func(_ context.Context, root, finalRoot, prefix, agent string, artifact userSnapshotArtifact, action string) error {
		calls = append(calls, action+"|"+root+"|"+finalRoot+"|"+prefix+"|"+agent+"|"+artifact.SHA)
		return nil
	}
	scope, err := userInstallScopeOf(UserInstallRequest{Scope: UserComponentCodeGraph})
	if err != nil {
		t.Fatal(err)
	}
	existing := &userManifest{MainSDK: &userSnapshotArtifact{SHA: "main-sha"}}
	if err := userPostNativeReadback(context.Background(), "/stage", "/stage/prefix", "/stage/agent", "/final/prefix", "/final", "separate", scope, existing); err != nil {
		t.Fatal(err)
	}
	want := userMainSDKSelectedAction + "|/stage|/final|/stage/prefix|/stage/agent|main-sha"
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("reused Main SDK readback = %q, want [%q]", calls, want)
	}
}
