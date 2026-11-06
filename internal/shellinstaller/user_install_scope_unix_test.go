//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUserInstallScopeOf(t *testing.T) {
	full, err := userInstallScopeOf(UserInstallRequest{})
	if err != nil || !full.full || len(full.reused) != 0 {
		t.Fatalf("empty scope must be a full install: %+v %v", full, err)
	}
	scoped, err := userInstallScopeOf(UserInstallRequest{Scope: "sdk,codegraph"})
	if err != nil {
		t.Fatal(err)
	}
	if scoped.full || !scoped.requested[UserComponentSDK] || !scoped.requested[UserComponentCodeGraph] {
		t.Fatalf("requested components differ: %+v", scoped)
	}
	for _, name := range []string{UserComponentNode, UserComponentContext7} {
		if !scoped.reused[name] {
			t.Fatalf("component %q must be reused, got %+v", name, scoped)
		}
	}
	if _, err := userInstallScopeOf(UserInstallRequest{Scope: "codegraph,codegraph"}); err == nil {
		t.Fatal("duplicate scope components must be refused")
	}
	if _, err := userInstallScopeOf(UserInstallRequest{Scope: "kernel"}); err == nil {
		t.Fatal("unknown scope components must be refused")
	}
}

func TestUserReuseTree(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	from := filepath.Join(parent, "installed", "runtime", "node")
	if err := os.MkdirAll(filepath.Join(from, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "bin", "node"), []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "readme"), []byte("text"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("node", filepath.Join(from, "bin", "node-link")); err != nil {
		t.Fatal(err)
	}
	to := filepath.Join(parent, "stage", "runtime", "node")
	if err := userReuseTree(ctx, from, to); err != nil {
		t.Fatal(err)
	}
	// An empty pre-created stage directory receives the copy; a populated
	// target is still refused.
	merge := filepath.Join(parent, "stage2", "agent")
	if err := os.MkdirAll(merge, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(merge, "occupied"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userReuseTree(ctx, from, merge); err == nil {
		t.Fatal("reuse into a populated target must be refused")
	}
	if err := os.Remove(filepath.Join(merge, "occupied")); err != nil {
		t.Fatal(err)
	}
	if err := userReuseTree(ctx, from, merge); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(merge, "bin", "node")); err != nil {
		t.Fatalf("reuse into an empty target lost the copy: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(to, "bin", "node"))
	if err != nil || string(data) != "binary" {
		t.Fatalf("reused binary differs: %q %v", data, err)
	}
	info, err := os.Lstat(filepath.Join(to, "bin", "node"))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("reused binary mode differs: %v %v", info.Mode(), err)
	}
	info, err = os.Lstat(filepath.Join(to, "readme"))
	if err != nil || info.Mode().Perm() != 0400 {
		t.Fatalf("reused file mode differs: %v %v", info.Mode(), err)
	}
	link, err := os.Readlink(filepath.Join(to, "bin", "node-link"))
	if err != nil || link != "node" {
		t.Fatalf("reused symlink differs: %q %v", link, err)
	}
	if err := userReuseTree(ctx, from, to); err == nil {
		t.Fatal("reuse into an existing tree must be refused")
	}
	if err := userReuseTree(ctx, filepath.Join(parent, "absent"), filepath.Join(parent, "other")); err == nil {
		t.Fatal("reuse from an absent tree must be refused")
	}
}
