//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUserPNPMSyncTree(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	reader, err := newUserPNPMInventoryReader(root)
	if err != nil {
		t.Fatal(err)
	}
	if reader == nil {
		t.Skip("requires openat2")
	}
	defer reader.close()
	// Creating payload directories legitimately changes root metadata after anchoring.
	if err := os.Mkdir(filepath.Join(root, "payload"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(root, "payload", name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/missing-final/node", filepath.Join(root, "node")); err != nil {
		t.Fatal(err)
	}
	count := 0
	err = userPNPMSyncTree(context.Background(), reader, func(file userPNPMSyncFile) error {
		count++
		return errors.Join(file.Sync(), file.Close())
	})
	if err != nil || count != 2 {
		t.Fatalf("regular files not synchronized: count=%d error=%v", count, err)
	}
	sentinel := errors.New("flush failed")
	err = userPNPMSyncTree(context.Background(), reader, func(file userPNPMSyncFile) error {
		return errors.Join(sentinel, file.Close())
	})
	if !errors.Is(err, sentinel) {
		t.Fatal("flush failure lost", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := userPNPMSyncTree(ctx, reader, func(file userPNPMSyncFile) error { t.Error("submitted after cancellation"); return file.Close() }); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}

func TestUserPNPMSyncTreeRefusesRootReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "stage")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	reader, err := newUserPNPMInventoryReader(root)
	if err != nil {
		t.Fatal(err)
	}
	if reader == nil {
		t.Skip("requires openat2")
	}
	defer reader.close()
	if err := os.Rename(root, root+"-held"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userPNPMSyncTree(context.Background(), reader, func(file userPNPMSyncFile) error { t.Error("submitted replacement file"); return file.Close() }); err == nil {
		t.Fatal("replacement root accepted")
	}
}
