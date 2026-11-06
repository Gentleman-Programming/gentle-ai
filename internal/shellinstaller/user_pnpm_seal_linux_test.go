//go:build linux && amd64 && shell_pnpm_seal

package shellinstaller

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserPNPMCodeSeal(t *testing.T) {
	if testing.Short() {
		t.Skip("requires genuine installer DATA in the private sandbox")
	}
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	original, err := PNPMCodeSeal(ctx, "/out")
	check(err)
	copyRoot := t.TempDir()
	check(os.Chmod(copyRoot, 0700))
	dependency := ""
	check(filepath.WalkDir("/out/pnpm", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel("/out", path)
		dest := filepath.Join(copyRoot, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Mkdir(dest, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dest)
		}
		if strings.HasSuffix(rel, "/package.json") && strings.Contains(rel, "/node_modules/semver/") {
			dependency = dest
		}
		from, err := os.Open(path)
		if err != nil {
			return err
		}
		defer from.Close()
		to, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, err = io.Copy(to, from)
		closeErr := to.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Chmod(dest, info.Mode().Perm())
	}))
	if dependency == "" {
		t.Fatal("genuine transitive dependency missing")
	}
	assertSeal := func(want string) {
		t.Helper()
		got, err := PNPMCodeSeal(ctx, copyRoot)
		check(err)
		if got != want {
			t.Fatal("seal mismatch", got, want)
		}
	}
	assertSeal(original)
	data, err := os.ReadFile(dependency)
	check(err)
	info, err := os.Stat(dependency)
	check(err)
	for _, action := range []string{"modify", "remove", "add", "mode", "link"} {
		t.Run(action, func(t *testing.T) {
			path := dependency
			switch action {
			case "modify":
				check(os.WriteFile(path, []byte("changed"), 0600))
			case "remove":
				check(os.Remove(path))
			case "add":
				check(os.WriteFile(path+".extra", []byte("extra"), 0600))
			case "mode":
				check(os.Chmod(path, 0700))
			case "link":
				path = filepath.Join(copyRoot, "pnpm/bin/node")
				check(os.Remove(path))
				check(os.Symlink("/outside/node", path))
			}
			if got, err := PNPMCodeSeal(ctx, copyRoot); err == nil && got == original {
				t.Fatal("changed closure accepted", action)
			}
			switch action {
			case "add":
				check(os.Remove(path + ".extra"))
			case "mode":
				check(os.Chmod(path, info.Mode().Perm()))
			case "link":
				target, err := os.Readlink("/out/pnpm/bin/node")
				check(err)
				check(os.Remove(path))
				check(os.Symlink(target, path))
			default:
				check(os.WriteFile(path, data, info.Mode().Perm()))
			}
		})
	}
	assertSeal(original)
	check(os.Mkdir(filepath.Join(copyRoot, "agent"), 0700))
	check(os.WriteFile(filepath.Join(copyRoot, "agent/session.json"), []byte("{}"), 0600))
	assertSeal(original)
	if fresh, err := PNPMCodeSeal(ctx, "/out"); err != nil || fresh != original {
		t.Fatal("original DATA changed", err)
	}
}
