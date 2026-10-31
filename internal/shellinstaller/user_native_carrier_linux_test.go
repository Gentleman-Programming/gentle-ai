//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

func TestUserNativeCarrier(t *testing.T) {
	const commit = "8983b43bd52210a9242a056d291fb2bf30e73cc5"
	const digest = "f4b964eda29f2ff9b4fb6b8329a2f1eb60e7b5313984518cf6109159b18a71e9"
	data, err := assets.ReadNativeInstallerSource()
	if err != nil || len(data) != 11837894 || fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
		t.Fatal("selected local carrier differs", err)
	}
	expected, err := userMainDescriptor(commit, digest)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"exact", "canceled", "archive tamper", "source tamper", "existing source", "symlink runtime"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root := installStagePath(t)
			if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
				t.Fatal(err)
			}
			runtime := filepath.Join(root, "runtime")
			if scenario == "symlink runtime" {
				other := t.TempDir()
				if err := os.Symlink(other, runtime); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(runtime, 0700); err != nil {
				t.Fatal(err)
			}
			if scenario == "existing source" {
				if err := os.Mkdir(filepath.Join(runtime, "main-source"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			before, err := userTreeStamp(root)
			if err != nil && scenario != "symlink runtime" {
				t.Fatal(err)
			}
			if scenario == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			worker, err := StageNativeInstallerCarrier(ctx, root)
			refused := scenario == "canceled" || scenario == "existing source" || scenario == "symlink runtime"
			if refused {
				if err == nil || worker != nil {
					t.Fatal("unsafe carrier stage accepted", scenario)
				}
				if scenario != "symlink runtime" {
					after, err := userTreeStamp(root)
					if err != nil || before != after {
						t.Fatal("rejected carrier changed private stage", err)
					}
				}
				return
			}
			if err != nil || worker == nil {
				t.Fatal("local carrier materialization failed", err)
			}
			if worker.sourceSnapshot == nil || *worker.sourceSnapshot != expected || worker.root != root || worker.source != filepath.Join(runtime, "main-source") {
				t.Fatal("runtime is not bound to selected original source")
			}
			retained, err := userReadMainArchive(ctx, root, expected)
			if err != nil || !bytes.Equal(data, retained) {
				t.Fatal("retained local carrier differs", err)
			}
			if err := userCompareMainSource(ctx, root, expected); err != nil {
				t.Fatal("complete source differs", err)
			}
			// No supplied executable exists: materialization must not invoke Node or bootstrap.
			for _, name := range []string{"node", "native-bootstrap"} {
				if _, err := os.Lstat(filepath.Join(runtime, name)); !os.IsNotExist(err) {
					t.Fatal("carrier ran supplier or provisioned tooling", name, err)
				}
			}
			if scenario == "exact" {
				return
			}
			changed := filepath.Join(runtime, "main-source/scripts/installer-runtime.mjs")
			if scenario == "archive tamper" {
				changed = filepath.Join(runtime, "main/main.zip")
			}
			if err := os.WriteFile(changed, []byte("changed carrier bytes\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := worker.nativeSourceCheck(ctx); err == nil {
				t.Fatal("changed carrier reached native runtime")
			}
			after, err := os.ReadFile(changed)
			if err != nil || string(after) != "changed carrier bytes\n" {
				t.Fatal("rejected source check changed evidence", err)
			}
		})
	}
}

func TestUserNativeCarrierBytes(t *testing.T) {
	first, err := assets.ReadNativeInstallerSource()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 11837894 {
		t.Fatal("first carrier read has unexpected length", len(first))
	}
	first[0] ^= 255
	second, err := assets.ReadNativeInstallerSource()
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 11837894 || fmt.Sprintf("%x", sha256.Sum256(second)) != "f4b964eda29f2ff9b4fb6b8329a2f1eb60e7b5313984518cf6109159b18a71e9" {
		t.Fatal("carrier read aliases returned bytes")
	}
}
