//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUserPNPMInventoryReaderParity(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0644, 0700, 0755, 0750} {
		t.Run(mode.String(), func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "file")
			if err := os.WriteFile(path, []byte("retained content\n"), mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			reader, err := newUserPNPMInventoryReader(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.close()
			want, oldErr := privateNativeFile(context.Background(), path, 0, -1, "")
			got, err := reader.file(context.Background(), path)
			if (err == nil) != (oldErr == nil) || got != want {
				t.Fatal("native file contract differs", got, err, want, oldErr)
			}
			var fallback *userPNPMInventoryReader
			got, err = fallback.file(context.Background(), path)
			if (err == nil) != (oldErr == nil) || got != want {
				t.Fatal("unsupported-kernel fallback differs", got, err, want, oldErr)
			}
		})
	}
}

func TestUserPNPMInventoryReaderRefusal(t *testing.T) {
	for _, scenario := range []string{"leaf symlink", "parent symlink", "replacement", "canceled", "root replaced"} {
		t.Run(scenario, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "source")
			dir := filepath.Join(root, "nested")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "file")
			if err := os.WriteFile(path, []byte("retained content\n"), 0600); err != nil {
				t.Fatal(err)
			}
			reader, err := newUserPNPMInventoryReader(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.close()
			ctx := context.Background()
			switch scenario {
			case "leaf symlink":
				if err := os.Rename(path, path+"-old"); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(path+"-old", path)
			case "parent symlink":
				if err := os.Rename(dir, dir+"-old"); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(dir+"-old", dir)
			case "replacement":
				ctx = &inventoryMutationContext{Context: ctx, mutate: func() {
					if err := os.Rename(path, path+"-old"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("retained content\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "root replaced":
				if reader == nil {
					t.Skip("kernel lacks descriptor-rooted inventory support")
				}
				if err := os.Rename(root, root+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := reader.verify(); err == nil {
					t.Fatal("root replacement accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stamp, err := reader.file(ctx, path); err == nil || stamp != "" {
				t.Fatal("unsafe inventory file accepted", scenario, stamp, err)
			}
		})
	}
}

type inventoryMutationContext struct {
	context.Context
	calls  int
	mutate func()
}

func (c *inventoryMutationContext) Err() error {
	c.calls++
	if c.calls == 2 {
		c.mutate()
	}
	return c.Context.Err()
}
