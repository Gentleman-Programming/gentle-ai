//go:build linux

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPrivateNativeControls(t *testing.T) {
	privateGuest(t)
	// These literals are independent of production constants and local inventories.
	if privateNativeNodeSHA != "41a74efb34cbde5c7632cdac0cf8bd1a14d0b8d73dc1e82755014d9a9ce70f5c" || privateNativeNodeSize != 123655872 || privateNativeResolverSHA != "cbdf5deac8b7a85ab1253dbd049953aeb192a7d1f7987f9206916ab449c10a92" {
		t.Fatal("independent native/Node/source pins differ")
	}
	workspace := t.TempDir()
	if !strings.Contains(privateNativeInvoke, "installGentleAi({packageRoot})") || strings.Contains(privateNativeInvoke, "process.env") || strings.Contains(privateNativeInvoke, "install-gentle-ai.mjs") {
		t.Fatal("supplier invocation changed")
	}
	// Owned-filesystem hash controls only: no fixture executable is launched.
	path := filepath.Join(workspace, "data")
	data := []byte("owned bounded file DATA")
	pin := fmt.Sprintf("%x", sha256.Sum256(data))
	privateMust(t, os.WriteFile(path, data, 0600))
	before, err := privateNativeFile(context.Background(), path, 0600, int64(len(data)), pin)
	privateMust(t, err)
	cases := []struct {
		name string
		mode os.FileMode
		size int64
		pin  string
	}{
		{"mode", 0644, int64(len(data)), pin},
		{"size", 0600, int64(len(data)) + 1, pin},
		{"hash", 0600, int64(len(data)), strings.Repeat("0", 64)},
	}
	for _, tc := range cases {
		if _, err := privateNativeFile(context.Background(), path, tc.mode, tc.size, tc.pin); err == nil {
			t.Fatal("file DATA accepted", tc.name)
		}
	}
	privateMust(t, os.Chmod(path, 0600|os.ModeSticky))
	if _, err := privateNativeFile(context.Background(), path, 0600, int64(len(data)), pin); err == nil {
		t.Fatal("special mode accepted")
	}
	privateMust(t, os.Chmod(path, 0600))
	privateMust(t, os.Rename(path, path+".old"))
	privateMust(t, os.WriteFile(path, data, 0600))
	after, err := privateNativeFile(context.Background(), path, 0600, int64(len(data)), pin)
	privateMust(t, err)
	if before == after {
		t.Fatal("same-content replacement identity missed")
	}
	link := filepath.Join(workspace, "link")
	privateMust(t, os.Symlink(path, link))
	for _, candidate := range []string{link, workspace, filepath.Join(workspace, "missing"), "/cold-root-owned", "/node.tgz"} {
		if _, err := privateNativeFile(context.Background(), candidate, 0600, -1, ""); err == nil {
			t.Fatal("nonregular/foreign physical file accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := privateNativeFile(ctx, path, 0600, -1, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("file read ignored cancellation", err)
	}
}

func TestPrivateNativeStates(t *testing.T) {
	privateGuest(t)
	_, err := privateDirectory("/cold-root-owned")
	if err == nil {
		t.Fatal("actual foreign private directory accepted")
	}
	// Direct physical primitives, not the retired version-specific state classifier.
	base := t.TempDir()
	root := filepath.Join(base, "owned")
	privateMust(t, os.Mkdir(root, 0700))
	for _, mode := range []os.FileMode{0700, 0755} {
		privateMust(t, os.Chmod(root, mode))
		privateMust(t, privateNativeDirectory(root))
	}
	for _, mode := range []os.FileMode{0777, 0700 | os.ModeSticky} {
		privateMust(t, os.Chmod(root, mode))
		if err := privateNativeDirectory(root); err == nil {
			t.Fatal("native directory mode accepted", mode)
		}
	}
	privateMust(t, os.Chmod(root, 0700))
	link := filepath.Join(base, "link")
	privateMust(t, os.Symlink(root, link))
	file := filepath.Join(base, "file")
	privateMust(t, os.WriteFile(file, []byte("preserve"), 0600))
	before := privateFixture(t, base)
	for _, candidate := range []string{link, file, filepath.Join(base, "missing"), "/cold-root-owned"} {
		if err := privateNativeDirectory(candidate); err == nil {
			t.Fatal("nonphysical/foreign native directory accepted", candidate)
		}
	}
	if !reflect.DeepEqual(before, privateFixture(t, base)) {
		t.Fatal("native directory inspection changed preimage")
	}
	entries, err := privateNativeEntries(root)
	privateMust(t, err)
	if len(entries) != 0 {
		t.Fatal("empty directory inventory differs")
	}
	for i := 0; i < 18; i++ {
		privateMust(t, os.Mkdir(filepath.Join(root, fmt.Sprint(i)), 0700))
	}
	before = privateFixture(t, base)
	entries, err = privateNativeEntries(root)
	privateMust(t, err)
	if len(entries) != 17 || !reflect.DeepEqual(before, privateFixture(t, base)) {
		t.Fatal("native directory inventory exceeded bound or changed preimage")
	}
}

func TestPrivateNativeCleanup(t *testing.T) {
	privateGuest(t)
	// Cleanup faults are actual owned filesystem effects, without an installer or finalizer.
	for _, name := range []string{"clean absent", "empty runtime", "partial", "published", "permission", "replacement", "nil identity"} {
		parent := t.TempDir()
		root := filepath.Join(parent, ".gentle-ai")
		pi := filepath.Join(parent, "pi")
		privateMust(t, os.Mkdir(pi, 0700))
		privateMust(t, os.WriteFile(filepath.Join(pi, "settings.json"), []byte("unrelated Pi preimage"), 0600))
		privateMust(t, os.Symlink("settings.json", filepath.Join(pi, "link")))
		piBefore := privateFixture(t, pi)
		workspace, err := os.MkdirTemp(parent, ".gentle-native-")
		privateMust(t, err)
		identity, err := privateDirectory(workspace)
		privateMust(t, err)
		if name != "clean absent" {
			privateMust(t, os.Mkdir(root, 0700))
		}
		if name == "partial" || name == "published" {
			// Arbitrary sentinel DATA; no version-specific supplier state interpretation.
			entry := "partial"
			if name == "published" {
				entry = "published"
			}
			privateMust(t, os.Mkdir(filepath.Join(root, entry), 0700))
			privateMust(t, os.WriteFile(filepath.Join(root, entry, "sentinel"), []byte("never delete"), 0600))
		}
		blocked := filepath.Join(workspace, "blocked")
		if name == "permission" {
			privateMust(t, os.Mkdir(blocked, 0700))
			privateMust(t, os.WriteFile(filepath.Join(blocked, "child"), []byte("owned"), 0600))
			privateMust(t, os.Chmod(blocked, 0000))
		}
		if name == "replacement" {
			privateMust(t, os.Rename(workspace, workspace+".old"))
			privateMust(t, os.Mkdir(workspace, 0700))
			privateMust(t, os.WriteFile(filepath.Join(workspace, "foreign"), []byte("preserve"), 0600))
		}
		if name == "nil identity" {
			identity = nil
		}
		var runtimeBefore map[string]string
		if name != "clean absent" {
			runtimeBefore = privateFixture(t, root)
		}
		var workspaceBefore map[string]string
		if name == "replacement" || name == "nil identity" {
			workspaceBefore = privateFixture(t, workspace)
		}
		cleanupErr := privateCleanup(workspace, identity)
		if name == "permission" {
			privateMust(t, os.Chmod(blocked, 0700))
		}
		wantFailure := name == "permission" || name == "replacement" || name == "nil identity"
		var failure *PrivateRuntimeError
		permissionFailure := name == "permission" && os.IsPermission(cleanupErr)
		identityFailure := (name == "replacement" || name == "nil identity") && errors.As(cleanupErr, &failure) && failure.Kind == "uncertain"
		if (cleanupErr != nil) != wantFailure || (wantFailure && !permissionFailure && !identityFailure) {
			t.Fatal("native cleanup outcome differs", name, cleanupErr)
		}
		if runtimeBefore != nil && !reflect.DeepEqual(runtimeBefore, privateFixture(t, root)) {
			t.Fatal("native state deleted by wrapper finalizer")
		}
		if workspaceBefore != nil && !reflect.DeepEqual(workspaceBefore, privateFixture(t, workspace)) {
			t.Fatal("foreign workspace deleted")
		}
		if !reflect.DeepEqual(piBefore, privateFixture(t, pi)) {
			t.Fatal("Pi preimage changed")
		}
		if !wantFailure {
			if _, err := os.Lstat(workspace); !os.IsNotExist(err) {
				t.Fatal("wrapper workspace leaked")
			}
		}
	}
}
