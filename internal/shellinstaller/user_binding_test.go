package shellinstaller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestUserBindingPreservesPathsAndArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("generated bindings require POSIX sh")
	}
	for _, name := range []string{"pi", "gentle-shell"} {
		for _, install := range []bool{false, true} {
			if name == "pi" && install {
				continue
			}
			t.Run(name+"/"+map[bool]string{false: "launch", true: "install"}[install], func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "o'brien shell;$HOME")
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				// Echo-only fixture: never invokes a real supervisor or installer.
				if err := os.WriteFile(filepath.Join(root, "supervisor"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
					t.Fatal(err)
				}
				binding := filepath.Join(root, name)
				if err := os.WriteFile(binding, []byte(userBinding(root, name)), 0700); err != nil {
					t.Fatal(err)
				}
				args := []string{"argument with spaces", "o'brien", "$(false)"}
				want := append([]string{"shell", "launch", root}, args...)
				if install {
					want = append([]string{"shell", "install"}, args...)
					args = append([]string{"install"}, args...)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "/bin/sh", append([]string{binding}, args...)...)
				cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
				output, err := cmd.CombinedOutput()
				if err != nil || string(output) != strings.Join(want, "\n")+"\n" {
					t.Fatalf("binding changed paths or arguments: %v %q, want %q", err, output, want)
				}
			})
		}
	}
}
