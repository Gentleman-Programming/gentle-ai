//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Selection refusals name the offending path so the operator can fix it
// (#5854): the refusal itself is unchanged.
func TestUserTreeStampRefusalsNameThePath(t *testing.T) {
	cases := map[string]func(t *testing.T, root string) string{
		"symlink escape": func(t *testing.T, root string) string {
			path := filepath.Join(root, "escape")
			if err := os.Symlink(t.TempDir(), path); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"writable directory": func(t *testing.T, root string) string {
			path := filepath.Join(root, "shared")
			if err := os.Mkdir(path, 0700); err != nil || os.Chmod(path, 0770) != nil {
				t.Fatal(err)
			}
			return path
		},
		"writable file": func(t *testing.T, root string) string {
			path := filepath.Join(root, "shared.txt")
			if err := os.WriteFile(path, nil, 0600); err != nil || os.Chmod(path, 0660) != nil {
				t.Fatal(err)
			}
			return path
		},
		"fifo": func(t *testing.T, root string) string {
			path := filepath.Join(root, "pipe")
			if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			return path
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := prepare(t, root)
			if _, err := userTreeStamp(root); err == nil || !strings.Contains(err.Error(), `path="`+path+`"`) {
				t.Fatalf("refusal does not name %q: %v", path, err)
			}
		})
	}
}
