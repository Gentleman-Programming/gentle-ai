//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// userWindowsLaunchFixture builds a minimal owned installed tree that carries
// exactly what the launch trust boundary verifies. It deliberately contains no
// runtime/archives directory and no provision.mjs: if launch verification read
// the retained archives or ran provision verify, the accept cases below would
// fail instead of passing.
func userWindowsLaunchFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	userWindowsLaunchFixtureFile(t, root, "supervisor.exe", []byte("owned supervisor image"))
	userWindowsLaunchFixtureFile(t, root, "source/package-lock.json", []byte("{\"lockfileVersion\":3}"))
	userWindowsLaunchFixtureFile(t, root, "runtime/node/node.exe", []byte("fixture Node, never executed"))
	userWindowsLaunchFixtureFile(t, root, "prefix/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js", []byte("owned pi CLI entry"))
	userWindowsLaunchFixtureFile(t, root, "prefix/node_modules/gentle-pi/bin/gentle-shell.mjs", []byte("owned shell CLI entry"))
	userWindowsLaunchFixtureFile(t, root, "agent/settings.json", []byte("owned agent state"))
	for _, product := range []string{"pi", "gentle-shell"} {
		userWindowsLaunchFixtureFile(t, root, "bin/"+product+".cmd", userWindowsBinding(root, product))
	}
	step := func(name string, err error) {
		if err != nil {
			t.Fatalf("fixture step %s: %v", name, err)
		}
	}
	sid, err := userWindowsSID()
	step("sid", err)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := userWindowsIdentity(root, true)
	step("root identity", err)
	if err != nil {
		t.Fatal(err)
	}
	prefixIdentity, err := userWindowsIdentity(filepath.Join(root, "prefix"), true)
	step("prefix identity", err)
	if err != nil {
		t.Fatal(err)
	}
	agentIdentity, err := userWindowsIdentity(filepath.Join(root, "agent"), true)
	step("agent identity", err)
	if err != nil {
		t.Fatal(err)
	}
	selection := UserInstallRequest{Destination: root, Mode: "separate", Confirmation: "fixture-only", Channel: "stable"}
	selectionData, err := json.Marshal(selection)
	step("marshal selection", err)
	if err != nil {
		t.Fatal(err)
	}
	userWindowsLaunchFixtureFile(t, root, "selection.json", selectionData)
	lock, err := userWindowsRead(filepath.Join(root, "source/package-lock.json"), 32<<20)
	step("read lock", err)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userWindowsRead(filepath.Join(root, "supervisor.exe"), 256<<20)
	step("read supervisor", err)
	if err != nil {
		t.Fatal(err)
	}
	manifest := userWindowsManifest{
		Schema: userWindowsSchema, Destination: root, SID: sid, Identity: identity,
		SupervisorSHA: userWindowsSHA(supervisor), PrefixIdentity: prefixIdentity, AgentIdentity: agentIdentity,
		SelectionSHA: userWindowsSHA(selectionData), LockSHA: userWindowsSHA(lock),
	}
	manifestData, err := json.Marshal(manifest)
	step("marshal manifest", err)
	if err != nil {
		t.Fatal(err)
	}
	userWindowsLaunchFixtureFile(t, root, "manifest.json", append(manifestData, '\n'))
	return root
}

func userWindowsLaunchFixtureFile(t *testing.T, root, relative string, data []byte) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
}

// Launch verification passes on the owned tree even though the retained
// archives and provision source are absent: the fast boundary must not touch
// them. The complete verification still refuses the same tree, because it
// remains responsible for archive, per-file and npm-package integrity.
func TestUserWindowsLaunchBoundaryAcceptsOwnedTreeWithoutArchivesOrProvision(t *testing.T) {
	root := userWindowsLaunchFixture(t)
	if err := userWindowsVerifyLaunch(context.Background(), root); err != nil {
		t.Fatalf("launch trust boundary refused an owned tree: %v", err)
	}
	if err := userWindowsVerify(context.Background(), root, io.Discard, io.Discard); err == nil {
		t.Fatal("complete verification admitted a tree without retained archives")
	}
}

func TestUserWindowsLaunchBoundaryRejectsTamperedTrustObjects(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{"tampered supervisor image", func(t *testing.T, root string) {
			userWindowsLaunchFixtureFile(t, root, "supervisor.exe", []byte("tampered supervisor image"))
		}},
		{"tampered pi binding", func(t *testing.T, root string) {
			userWindowsLaunchFixtureFile(t, root, "bin/pi.cmd", []byte("@echo off\r\nrem tampered\r\n"))
		}},
		{"tampered gentle-shell binding", func(t *testing.T, root string) {
			userWindowsLaunchFixtureFile(t, root, "bin/gentle-shell.cmd", userWindowsBinding(root, "pi"))
		}},
		{"missing Node executable", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "runtime", "node", "node.exe")); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing pi CLI entry", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "prefix", "node_modules", "@earendil-works", "pi-coding-agent", "dist", "bundle", "cli.js")); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing shell CLI entry", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "prefix", "node_modules", "gentle-pi", "bin", "gentle-shell.mjs")); err != nil {
				t.Fatal(err)
			}
		}},
		{"prefix replaced by junction", func(t *testing.T, root string) {
			prefix := filepath.Join(root, "prefix")
			if err := os.RemoveAll(prefix); err != nil {
				t.Fatal(err)
			}
			userWindowsJunction(t, prefix, t.TempDir())
		}},
		{"agent foreign DACL", func(t *testing.T, root string) {
			userWindowsForeignACE(t, filepath.Join(root, "agent"))
		}},
		{"tampered manifest", func(t *testing.T, root string) {
			path := filepath.Join(root, "manifest.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			userWindowsLaunchFixtureFile(t, root, "manifest.json", append(data, ' '))
		}},
		{"tampered selection", func(t *testing.T, root string) {
			userWindowsLaunchFixtureFile(t, root, "selection.json", []byte(`{"Destination":"elsewhere","Mode":"separate","Confirmation":"fixture-only","Channel":"stable"}`))
		}},
		{"tampered lock", func(t *testing.T, root string) {
			userWindowsLaunchFixtureFile(t, root, "source/package-lock.json", []byte("{\"lockfileVersion\":9}"))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := userWindowsLaunchFixture(t)
			tt.mutate(t, root)
			if err := userWindowsVerifyLaunch(context.Background(), root); err == nil {
				t.Fatalf("launch trust boundary admitted %s", tt.name)
			}
		})
	}
}

// Documented launch tradeoff: the fast boundary pins Node and the owned CLI
// entries by owned identity and presence, not by byte content, so byte-level
// tampering of already-installed runtime content is only caught by the
// complete verification on install, idempotent install and explicit check.
// The supervisor image and every command binding stay byte-verified at launch.
func TestUserWindowsLaunchBoundaryAcceptsByteTamperedRuntimeContent(t *testing.T) {
	root := userWindowsLaunchFixture(t)
	userWindowsLaunchFixtureFile(t, root, "runtime/node/node.exe", []byte("byte-tampered Node"))
	userWindowsLaunchFixtureFile(t, root, "prefix/node_modules/gentle-pi/bin/gentle-shell.mjs", []byte("byte-tampered entry"))
	if err := userWindowsVerifyLaunch(context.Background(), root); err != nil {
		t.Fatalf("launch trust boundary rejected documented runtime content tradeoff: %v", err)
	}
}

// The launch case must verify through the fast launch boundary, never through
// the complete verification: the stub below is the only verifier the launch
// path consults, and it observes the exact root and product being launched.
func TestUserWindowsLaunchUsesFastBoundary(t *testing.T) {
	sentinel := errors.New("launch boundary stub")
	root := filepath.Join(t.TempDir(), "owned")
	previous := userWindowsLaunchVerify
	var observedRoot string
	userWindowsLaunchVerify = func(ctx context.Context, root string) error {
		observedRoot = root
		return sentinel
	}
	defer func() { userWindowsLaunchVerify = previous }()
	err := userWindowsLaunch(context.Background(), root, "pi", nil, nil, io.Discard, io.Discard)
	if !errors.Is(err, sentinel) {
		t.Fatalf("launch did not verify through the fast boundary: %v", err)
	}
	if observedRoot != root {
		t.Fatalf("fast boundary received root %q; want %q", observedRoot, root)
	}
}
