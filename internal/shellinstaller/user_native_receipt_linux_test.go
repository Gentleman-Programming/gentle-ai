//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserNativeProducerReceipt(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	native := NativeInstallerResult{Details: json.RawMessage(`{"outcome":"terminal-action-required","action":"open-new-terminal","completed":["original-step"]}`)}
	location := NativeInstallationLocation{PackageRoot: root + "/pnpm/store/v11/links/package/node_modules/gentle-pi", PNPMHome: root + "/pnpm", GlobalBin: root + "/pnpm/bin", ShellVersion: "4.3.0", PiVersion: "1.3.0"}
	if err := userWriteNativeReceipt(root, native, &location); err != nil {
		t.Fatal(err)
	}
	got, loc, err := userReadNativeReceipt(context.Background(), root)
	if err != nil || loc == nil || *loc != location || got.Outcome != "terminal-action-required" || len(got.Completed) != 1 || string(got.Details) != string(native.Details) {
		t.Fatal("original success details or locator lost", got, loc, err)
	}
	before, err := os.ReadFile(filepath.Join(root, "native-installation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := userWriteNativeReceipt(root, native, &location); err == nil {
		t.Fatal("receipt overwritten")
	}
	after, err := os.ReadFile(filepath.Join(root, "native-installation.json"))
	if err != nil || string(after) != string(before) {
		t.Fatal("existing receipt changed", err)
	}
}

func TestUserNativeProducerReceiptRefuses(t *testing.T) {
	for _, name := range []string{"missing", "malformed", "oversized", "mode", "symlink", "blocked", "canceled"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "native-installation.json")
			data, err := json.Marshal(map[string]any{
				"result":       json.RawMessage(`{"outcome":"terminal-action-required","action":"open-new-terminal"}`),
				"installation": NativeInstallationLocation{PackageRoot: root + "/pnpm/store/v11/links/package/node_modules/gentle-pi", PNPMHome: root + "/pnpm", GlobalBin: root + "/pnpm/bin", ShellVersion: "4.3.0", PiVersion: "1.3.0"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if name == "blocked" {
				data = []byte(`{"result":{"outcome":"blocked"}}`)
			}
			if name == "malformed" {
				data = []byte("not json")
			}
			if name == "oversized" {
				data = []byte(strings.Repeat("x", 65537))
			}
			if name != "missing" && name != "symlink" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if name == "mode" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if name == "symlink" {
				outside := filepath.Join(root, "linked-receipt")
				if err := os.WriteFile(outside, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "canceled" {
				cancel()
			}
			if _, _, err := userReadNativeReceipt(ctx, root); err == nil {
				t.Fatal("unsafe receipt accepted")
			}
			if after, err := userTreeStamp(root); err != nil || after != before {
				t.Fatal("refusal changed producer", err)
			}
		})
	}
}

func TestUserNativeReceiptRefusesNonPrivateRoot(t *testing.T) {
	root := installStagePath(t)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	native := NativeInstallerResult{Details: json.RawMessage(`{"outcome":"terminal-action-required","action":"open-new-terminal","completed":["original-step"]}`)}
	location := NativeInstallationLocation{PackageRoot: root + "/pnpm/store/v11/links/package/node_modules/gentle-pi", PNPMHome: root + "/pnpm", GlobalBin: root + "/pnpm/bin", ShellVersion: "4.3.0", PiVersion: "1.3.0"}
	var refusal *PrivateRuntimeError
	if err := userWriteNativeReceipt(root, native, &location); !errors.As(err, &refusal) || refusal.Kind != "filesystem" {
		t.Fatal("nonprivate receipt root bypassed custody", err)
	}
	if reader, err := newUserPNPMInventoryReader(root); err == nil || reader != nil {
		t.Fatal("nonprivate inventory root acquired a descriptor", reader, err)
	}
	worker := &NativeInstallerRuntime{root: root}
	if prepared, err := worker.withPublicationData(); err == nil || prepared != nil {
		t.Fatal("nonprivate publication root acquired mutable data", prepared, err)
	}
	after, err := os.Lstat(root)
	if err != nil || privateStamp(before) != privateStamp(after) {
		t.Fatal("root refusal changed its custody or metadata", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("root refusal wrote before admission", entries, err)
	}
}
