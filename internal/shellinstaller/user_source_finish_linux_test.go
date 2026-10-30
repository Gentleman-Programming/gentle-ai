package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the current supplier-source boundary with inert owned data, not the
// retired composite installer or package JavaScript. Installation still applies
// independently pinned hashes before invoking the supplier.
func TestUserSupplierSourceAuthentication(t *testing.T) {
	for _, name := range []string{"scripts/gentle-ai-installer.mjs", "runtime/gentle-ai-binary.mjs", "runtime/node/bin/node"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, name)
			privateMust(t, os.MkdirAll(filepath.Dir(path), 0700))
			data := []byte("owned source fixture: never execute")
			pin := fmt.Sprintf("%x", sha256.Sum256(data))
			privateMust(t, os.WriteFile(path, data, 0600))
			ctx := context.Background()
			before, err := userSourceFile(ctx, path, int64(len(data)), pin)
			privateMust(t, err)
			for _, tc := range []struct {
				name string
				size int64
				pin  string
			}{{"wrong size", int64(len(data)) + 1, pin}, {"wrong hash", int64(len(data)), strings.Repeat("0", 64)}} {
				t.Run(tc.name, func(t *testing.T) {
					if _, err := userSourceFile(ctx, path, tc.size, tc.pin); err == nil {
						t.Fatal("supplier identity mismatch accepted")
					}
				})
			}
			changed := append([]byte(nil), data...)
			changed[0] ^= 1
			privateMust(t, os.WriteFile(path, changed, 0600))
			if _, err := userSourceFile(ctx, path, int64(len(data)), pin); err == nil {
				t.Fatal("same-sized modified source accepted")
			}
			privateMust(t, os.WriteFile(path, data, 0600))
			for _, mode := range []os.FileMode{0660, 0777, 0600 | os.ModeSticky} {
				privateMust(t, os.Chmod(path, mode))
				if _, err := userSourceFile(ctx, path, -1, pin); err == nil {
					t.Fatalf("unsafe source mode %#o accepted", mode)
				}
			}
			privateMust(t, os.Chmod(path, 0600))
			privateMust(t, os.Rename(path, path+".original"))
			privateMust(t, os.WriteFile(path, data, 0600))
			after, err := userSourceFile(ctx, path, int64(len(data)), pin)
			privateMust(t, err)
			if after == before {
				t.Fatal("same-content replacement lost physical identity")
			}
			alias := filepath.Join(root, "alias")
			privateMust(t, os.Symlink(path, alias))
			for _, candidate := range []string{alias, root, filepath.Join(root, "missing")} {
				if _, err := userSourceFile(ctx, candidate, -1, pin); err == nil {
					t.Fatalf("nonphysical or absent source accepted: %s", candidate)
				}
			}
			if os.Getuid() != 0 {
				foreign := "/usr/bin/setpriv"
				bytes, readErr := os.ReadFile(foreign)
				privateMust(t, readErr)
				matchingHash := fmt.Sprintf("%x", sha256.Sum256(bytes))
				if _, err := userSourceFile(ctx, foreign, int64(len(bytes)), matchingHash); err == nil {
					t.Fatal("foreign ownership accepted despite exact bytes and size")
				}
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := userSourceFile(canceled, path, -1, pin); !errors.Is(err, context.Canceled) {
				t.Fatalf("source read lost cancellation: %v", err)
			}
		})
	}
}

func TestUserInstallFinishCanceledAfterPublication(t *testing.T) {
	for _, mode := range []string{"separate", "shared"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			workspace, target := filepath.Join(root, "workspace"), filepath.Join(root, "published")
			privateMust(t, os.Mkdir(workspace, 0700))
			privateMust(t, os.Mkdir(target, 0700))
			identity, err := privateDirectory(workspace)
			privateMust(t, err)
			preimage := filepath.Join(target, "saved-preimage")
			privateMust(t, os.WriteFile(preimage, []byte("preserve published recovery evidence"), 0600))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := UserInstallRequest{Destination: target, Mode: mode}
			result, err := userInstallFinish(ctx, UserInstallResult{Destination: target, State: "ComponentInstalled"}, nil, workspace, identity, req, mode == "shared")
			var failure *PrivateRuntimeError
			if !errors.As(err, &failure) || failure.Kind != "uncertain" || !errors.Is(err, context.Canceled) || failure.Destination != target || result != (UserInstallResult{}) {
				t.Fatalf("publication cancellation became success or lost evidence/uncertainty: result=%+v err=%v", result, err)
			}
			data, readErr := os.ReadFile(preimage)
			if readErr != nil || string(data) != "preserve published recovery evidence" {
				t.Fatalf("published recovery evidence lost: %v", readErr)
			}
		})
	}
}

func TestUserInstallFinishCanceledDuringSharedProvisioning(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	privateMust(t, os.Mkdir(workspace, 0700))
	identity, err := privateDirectory(workspace)
	privateMust(t, err)
	preimage := filepath.Join(workspace, "saved-preimage")
	privateMust(t, os.WriteFile(preimage, []byte("preserve unpublished shared preimage"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := UserInstallRequest{Destination: filepath.Join(root, "unpublished"), Mode: "shared"}
	_, err = userInstallFinish(ctx, UserInstallResult{}, context.Canceled, workspace, identity, req, true)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "uncertain" || !errors.Is(err, context.Canceled) || failure.Workspace != workspace {
		t.Fatalf("shared cancellation discarded its uncertain workspace: %v", err)
	}
	data, readErr := os.ReadFile(preimage)
	if readErr != nil || string(data) != "preserve unpublished shared preimage" {
		t.Fatalf("shared preimage removed: %v", readErr)
	}
	if _, err := os.Lstat(req.Destination); !os.IsNotExist(err) {
		t.Fatal("finalizer synthesized a published installation", err)
	}
}
