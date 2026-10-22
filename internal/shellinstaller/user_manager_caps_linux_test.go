package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUserManagerClearsCapabilitiesBeforeSupervisor(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		t.Run(map[bool]string{false: "pipe", true: "pty"}[interactive], func(t *testing.T) {
			self := "/owned/supervisor"
			values := []string{"internal-launch", "/owned/target", "--property=NoNewPrivileges=no", "literal argument"}
			args := userServiceArgs("gentle-shell-0123456789abcdef", interactive, self, "/owned/project", values, []string{"HOME=/owned/home"})
			start := -1
			for index, value := range args {
				if value == "/usr/bin/env" {
					start = index
					break
				}
			}
			want := append([]string{"/usr/bin/env", "-i", "HOME=/owned/home", "/usr/bin/setpriv", "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs", "--", self, "shell"}, values...)
			if start < 0 || !reflect.DeepEqual(args[start:], want) {
				t.Fatalf("manager supervisor can inherit capabilities: %q", args)
			}
		})
	}
}

func TestUserManagerCapabilityHelperUsesExecutableCustody(t *testing.T) {
	ctx := context.Background()
	if _, err := userSupervisorSHA(ctx, "/usr/bin/setpriv"); err != nil {
		t.Fatalf("stock helper cannot be qualified: %v", err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "setpriv")
	if err := os.WriteFile(path, []byte("owned helper fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := userSupervisorSHA(ctx, path); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0600, 0770, 0777} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := userSupervisorSHA(ctx, path); err == nil {
			t.Fatalf("unsafe helper mode %#o accepted", mode)
		}
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := userSupervisorSHA(ctx, alias); err == nil {
		t.Fatal("aliased helper accepted")
	}
	if err := os.Chmod(root, 0770); err != nil {
		t.Fatal(err)
	}
	if _, err := userSupervisorSHA(ctx, path); err == nil || !strings.Contains(err.Error(), root) {
		t.Fatalf("unsafe helper ancestor not refused: %v", err)
	}
}
