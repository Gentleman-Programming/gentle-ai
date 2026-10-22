//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestIndependentPNPMTreeGuards(t *testing.T) {
	for _, scenario := range []string{"root-before", "root-during", "intermediate-link", "fifo", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			parent := t.TempDir()
			root, outside := filepath.Join(parent, "stage"), filepath.Join(parent, "outside")
			for _, dir := range []string{root, outside, filepath.Join(root, "payload")} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{filepath.Join(root, "000"), filepath.Join(root, "payload/next"), filepath.Join(outside, "next")} {
				if err := os.WriteFile(path, []byte("unchanged preimage"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			preimage, err := os.ReadFile(filepath.Join(outside, "next"))
			if err != nil {
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
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			replace := func() {
				if err := os.Rename(root, root+"-held"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "root-before" {
				replace()
			}
			count := 0
			err = userPNPMSyncTree(ctx, reader, func(file userPNPMSyncFile) error {
				count++
				result := errors.Join(file.Sync(), file.Close())
				if count == 1 {
					switch scenario {
					case "root-during":
						replace()
					case "intermediate-link":
						if err := os.Rename(filepath.Join(root, "payload"), filepath.Join(parent, "held-payload")); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(outside, filepath.Join(root, "payload")); err != nil {
							t.Fatal(err)
						}
					case "fifo":
						path := filepath.Join(root, "payload/next")
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						if err := unix.Mkfifo(path, 0600); err != nil {
							t.Fatal(err)
						}
					case "cancel":
						cancel()
					}
				}
				return result
			})
			if err == nil {
				t.Fatal("unsafe traversal accepted")
			}
			if scenario == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			if count > 1 || (scenario == "root-before" && count != 0) {
				t.Fatal("replacement submitted", count)
			}
			after, err := os.ReadFile(filepath.Join(outside, "next"))
			if err != nil || sha256.Sum256(preimage) != sha256.Sum256(after) {
				t.Fatal("external preimage changed", err)
			}
		})
	}
}

func TestIndependentPNPMTreeCoverageAndDrain(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"home", "config", "pnpm"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	expected := map[string]bool{"home/.bashrc": true, "config/user.npmrc": true, "pnpm/vendor": true}
	for name := range expected {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/not-published/node", filepath.Join(root, "node")); err != nil {
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
	found := map[string]bool{}
	err = userPNPMSyncTree(context.Background(), reader, func(file userPNPMSyncFile) error {
		f := file.(*os.File)
		rel, err := filepath.Rel(root, f.Name())
		if err != nil {
			t.Fatal(err)
		}
		found[rel] = true
		return errors.Join(file.Sync(), file.Close())
	})
	if err != nil || !reflect.DeepEqual(found, expected) {
		t.Fatal("incomplete regular-file synchronization", found, err)
	}
	batch := newUserPNPMFileSyncBatch()
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = userPNPMSyncTree(ctx, reader, func(file userPNPMSyncFile) error {
		cancel()
		return batch.submit(pnpmSyncProbe{
			syncFile: func() error {
				close(entered)
				<-release
				return file.Sync()
			},
			closeFile: file.Close,
		})
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("walk cancellation lost", err)
	}
	<-entered
	done := make(chan error, 1)
	go func() { done <- batch.finish() }()
	select {
	case err := <-done:
		t.Fatal("pending flush escaped drain", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
