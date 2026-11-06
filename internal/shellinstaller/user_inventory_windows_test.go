//go:build windows

package shellinstaller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// userWindowsInventoryFixture builds a private installed-artefact tree plus the
// runtime state an owned launch legitimately leaves behind.
func userWindowsInventoryFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"runtime/node/node.exe", "prefix/node_modules/gentle-pi/package.json", "agent/settings.json", "config/gentle-shell.json"} {
		userWindowsInventoryFile(t, root, file)
	}
	return root
}

func userWindowsInventoryFile(t *testing.T, root, relative string) string {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("fixture-only, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func userWindowsForeignACE(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	sid, err := userWindowsSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func userWindowsJunction(t *testing.T, link, target string) {
	t.Helper()
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("unelevated junction unavailable: %v %s", err, output)
	}
}

func TestUserWindowsInventoryLaunchAcceptsMutableState(t *testing.T) {
	root := userWindowsInventoryFixture(t)
	// Go escapes uppercase module paths with "!"; Pi names sessions after the caller CWD.
	for _, state := range []string{
		"home/go/pkg/mod/github.com/!burnt!sushi/toml@v1.4.0/decode.go",
		"state/go-build/0a/0a1b-d",
		"tmp/pi-bash-output.txt",
		"runtime/cache/_cacache/index-v5/aa/bb",
		"agent/sessions/--C--R&D-proj!x--/2026-10-08_session.jsonl",
	} {
		userWindowsInventoryFile(t, root, state)
	}
	userWindowsInventoryFile(t, root, "agent/sessions/--C--100%^done--/session.jsonl")
	if err := userWindowsInventory(root, userWindowsInventoryLaunch); err != nil {
		t.Fatalf("legitimate runtime state refused at launch: %v", err)
	}
	if err := userWindowsInventory(root, userWindowsInventoryInstalled); err == nil {
		t.Fatal("pre-publication inventory admitted selection-unsafe names")
	}
}

func TestUserWindowsInventoryLaunchKeepsArtefactGuards(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, root string)
	}{
		{"bang artefact directory", func(t *testing.T, root string) {
			userWindowsInventoryFile(t, root, "prefix/node_modules/x!y/index.js")
		}},
		{"state hard link to artefact", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, "home"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(filepath.Join(root, "runtime/node/node.exe"), filepath.Join(root, "home/alias.exe")); err != nil {
				t.Fatal(err)
			}
		}},
		{"state root junction", func(t *testing.T, root string) {
			userWindowsJunction(t, filepath.Join(root, "tmp"), t.TempDir())
		}},
		{"artefact junction", func(t *testing.T, root string) {
			userWindowsJunction(t, filepath.Join(root, "prefix/node_modules/alias"), t.TempDir())
		}},
		{"state root foreign ACE", func(t *testing.T, root string) {
			userWindowsForeignACE(t, filepath.Join(root, "state"))
		}},
		{"state root replaced by file", func(t *testing.T, root string) {
			userWindowsInventoryFile(t, root, "home")
		}},
		{"metacharacter artefact directory", func(t *testing.T, root string) {
			userWindowsInventoryFile(t, root, "prefix/node_modules/a&b/index.js")
		}},
		// Descendants of mutable state keep every per-entry guard.
		{"nested junction in module cache", func(t *testing.T, root string) {
			parent := filepath.Join(root, filepath.FromSlash("home/go/pkg/mod/github.com/!burnt!sushi"))
			if err := os.MkdirAll(parent, 0700); err != nil {
				t.Fatal(err)
			}
			userWindowsJunction(t, filepath.Join(parent, "toml@v1.4.0"), t.TempDir())
		}},
		{"nested symlink in build cache", func(t *testing.T, root string) {
			target := userWindowsInventoryFile(t, root, "state/go-build/real")
			if err := os.Symlink(target, filepath.Join(root, "state/go-build/alias")); err != nil {
				t.Skipf("unelevated symlink unavailable: %v", err)
			}
		}},
		{"nested foreign ACE in session", func(t *testing.T, root string) {
			userWindowsForeignACE(t, filepath.Join(root, filepath.FromSlash("agent/sessions/--C--R&D!x--")))
		}},
		{"nested hard link within state", func(t *testing.T, root string) {
			first := userWindowsInventoryFile(t, root, "state/go-build/0a/first")
			if err := os.Link(first, filepath.Join(root, "state/go-build/0a/second")); err != nil {
				t.Fatal(err)
			}
		}},
		{"nested hard link in session", func(t *testing.T, root string) {
			foreign := filepath.Join(t.TempDir(), "foreign.jsonl")
			if err := os.WriteFile(foreign, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			userWindowsInventoryFile(t, root, "agent/sessions/--C--x--/keep")
			if err := os.Link(foreign, filepath.Join(root, "agent/sessions/--C--x--/linked.jsonl")); err != nil {
				t.Skipf("cross-directory hard link unavailable: %v", err)
			}
		}},
		{"oversized nested cache file", func(t *testing.T, root string) {
			if err := os.Truncate(userWindowsInventoryFile(t, root, "runtime/cache/_cacache/content-v2/big"), 33<<20); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := userWindowsInventoryFixture(t)
			tt.mutate(t, root)
			if err := userWindowsInventory(root, userWindowsInventoryLaunch); err == nil {
				t.Fatalf("%s admitted at launch", tt.name)
			}
		})
	}
}

// Installed executable bytes are compared with the retained archive on every
// launch; runtime state beside them does not participate in that comparison.
func TestUserWindowsLaunchArtefactBytesStayVerified(t *testing.T) {
	ctx := context.Background()
	root := userWindowsInventoryFixture(t)
	data, artifact := userWindowsZipFixture(t, "fixture/node.exe", "fixture/lib/module.js")
	if err := os.MkdirAll(filepath.Join(root, "runtime/archives"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(root, "runtime/archives", artifact.Archive), data); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "runtime/fixture")
	if _, err := userWindowsZIP(ctx, data, artifact, destination, ""); err != nil {
		t.Fatal(err)
	}
	userWindowsInventoryFile(t, root, "home/go/pkg/mod/cache/download/github.com/!burnt!sushi/toml/@v/v1.4.0.zip")
	userWindowsInventoryFile(t, root, "state/go-build/trim.txt")
	if err := userWindowsVerifyArchive(ctx, root, artifact, destination, "node.exe"); err != nil {
		t.Fatalf("untampered artefact refused beside runtime state: %v", err)
	}
	if err := userWindowsInventory(root, userWindowsInventoryLaunch); err != nil {
		t.Fatalf("runtime state refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destination, "node.exe"), []byte("tampered executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsVerifyArchive(ctx, root, artifact, destination, "node.exe"); err == nil {
		t.Fatal("tampered installed executable admitted")
	}
}
