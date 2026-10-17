//go:build linux && shell_gga_canonical

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUserGGAArchiveReplacement(t *testing.T) {
	archive, err := os.ReadFile("/frozen-gga-source.tgz")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unchanged", "replacement", "oversized", "symlink", "FIFO", "canceled"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "source.tgz")
			if err := userToolWrite(path, archive, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			before := ""
			beforeMeta := make(map[string]string)
			data, readErr := userGGAReadArchive(ctx, path, func(path string) (*os.File, error) {
				// Controlled owned-fixture replacement after pathname validation,
				// before the real nofollow/nonblocking descriptor open. No producer,
				// network, external writer or physical runtime is fabricated.
				if name != "unchanged" && name != "canceled" {
					if err := os.Rename(path, path+".old"); err != nil {
						t.Fatal(err)
					}
				}
				var err error
				switch name {
				case "replacement":
					err = userToolWrite(path, archive, 0600)
				case "oversized":
					err = userToolWrite(path, append(append([]byte(nil), archive...), make([]byte, 1048576)...), 0600)
				case "symlink":
					err = os.Symlink("source.tgz.old", path)
				case "FIFO":
					err = syscall.Mkfifo(path, 0600)
				case "canceled":
					cancel()
				}
				if err != nil {
					t.Fatal(err)
				}
				// The normal selection stamp refuses FIFOs. Observe every owned
				// entry without opening nonregular objects instead of excluding it.
				before = userSnapshot(t, root)
				entries, err := os.ReadDir(root)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					info, err := os.Lstat(filepath.Join(root, entry.Name()))
					if err != nil {
						t.Fatal(err)
					}
					beforeMeta[entry.Name()] = privateStamp(info)
				}
				return userGGAOpenArchive(path)
			})
			if (readErr == nil) != (name == "unchanged") {
				t.Fatalf("replacement result = %v", readErr)
			}
			if name == "unchanged" && !bytes.Equal(data, archive) {
				t.Fatal("bounded descriptor changed authentic bytes")
			}
			if name == "canceled" && !errors.Is(readErr, context.Canceled) {
				t.Fatal("cancellation cause lost", readErr)
			}
			if before != userSnapshot(t, root) {
				t.Fatal("reader changed controlled fixture")
			}
			for name, stamp := range beforeMeta {
				info, err := os.Lstat(filepath.Join(root, name))
				if err != nil || stamp != privateStamp(info) {
					t.Fatal("reader changed controlled entry metadata", name, err)
				}
			}
		})
	}
}
