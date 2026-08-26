package sddstatus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveNativeAttemptIdentityRepositorySelection(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	nested := filepath.Join(repo, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(wd, repo)
	if err != nil {
		t.Fatal(err)
	}
	before := nativeAttemptTreeSnapshot(t, repo)
	first, err := ResolveNativeAttemptIdentity(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []string{relative, filepath.Join(repo, "."), nested} { // git -C accepts all three spellings.
		got, err := ResolveNativeAttemptIdentity(t.Context(), selection)
		if err != nil || got.Root != first.Root || got.RepositoryRef != first.RepositoryRef {
			t.Fatalf("ResolveNativeAttemptIdentity(%q) = %#v, %v", selection, got, err)
		}
	}
	if got := nativeAttemptTreeSnapshot(t, repo); got != before {
		t.Fatalf("read-only identity resolution mutated repository: %q != %q", got, before)
	}
	if err := os.Rename(filepath.Join(repo, ".git"), filepath.Join(repo, "git-before-swap")); err != nil {
		t.Fatal(err)
	}
	if err := first.Validate(context.Background()); err == nil {
		t.Fatal("Validate accepted repository identity drift")
	}
}

func TestNativeAttemptInputIdentities(t *testing.T) {
	t.Run("capture and validation are non-mutating", func(t *testing.T) {
		repo := nativeAttemptIdentityFixture(t)
		if err := os.Symlink("static.json", filepath.Join(repo, "snapshot-link")); err != nil {
			t.Fatal(err)
		}
		identity, err := ResolveNativeAttemptIdentity(t.Context(), repo)
		if err != nil {
			t.Fatal(err)
		}
		beforeCapture := nativeAttemptTreeSnapshot(t, repo)
		identity, err = nativeAttemptInputIdentities(t.Context(), identity, "node", "static.json", "src")
		if err != nil {
			t.Fatal(err)
		}
		if got := nativeAttemptTreeSnapshot(t, repo); got != beforeCapture {
			t.Fatalf("successful capture mutated repository: %q != %q", got, beforeCapture)
		}
		beforeValidate := nativeAttemptTreeSnapshot(t, repo)
		if err := identity.Validate(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := nativeAttemptTreeSnapshot(t, repo); got != beforeValidate {
			t.Fatalf("successful validation mutated repository: %q != %q", got, beforeValidate)
		}
	})

	for name, mutate := range map[string]func(*testing.T, string){
		"content drift": func(t *testing.T, root string) { write(t, filepath.Join(root, "static.json"), "changed\n") },
		"metadata drift": func(t *testing.T, root string) {
			if err := os.Chtimes(filepath.Join(root, "static.json"), time.Now(), time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		},
		"executable drift": func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "node"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"replacement TOCTOU": func(t *testing.T, root string) {
			write(t, filepath.Join(root, "node.replacement"), "new\n")
			if err := os.Rename(filepath.Join(root, "node.replacement"), filepath.Join(root, "node")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink TOCTOU": func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "static.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "node"), filepath.Join(root, "static.json")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		},
		"type drift": func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "static.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "static.json"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"recursive directory drift": func(t *testing.T, root string) {
			write(t, filepath.Join(root, "src", "migrations", "added.ts"), "new\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := nativeAttemptIdentityFixture(t)
			identity, err := ResolveNativeAttemptIdentity(t.Context(), repo)
			if err != nil {
				t.Fatal(err)
			}
			identity, err = nativeAttemptInputIdentities(t.Context(), identity, "node", "static.json", "src")
			if err != nil {
				t.Fatal(err)
			}
			before := nativeAttemptTreeSnapshot(t, repo)
			mutate(t, repo)
			if err := identity.Validate(t.Context()); err == nil {
				t.Fatal("Validate accepted input drift")
			}
			if name == "recursive directory drift" && before == nativeAttemptTreeSnapshot(t, repo) {
				t.Fatal("mutation fixture did not change the recursive snapshot")
			}
		})
	}
}

func TestNativeAttemptInputIdentitiesRejectsUnsafePaths(t *testing.T) {
	repo := nativeAttemptIdentityFixture(t)
	identity, err := ResolveNativeAttemptIdentity(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "static.json"), filepath.Join(repo, "static-link")); err == nil {
		before := nativeAttemptTreeSnapshot(t, repo)
		if _, err := nativeAttemptInputIdentities(t.Context(), identity, "static-link"); err == nil {
			t.Fatal("accepted symlink input")
		}
		if got := nativeAttemptTreeSnapshot(t, repo); got != before {
			t.Fatalf("rejection mutated repository: %q != %q", got, before)
		}
	} else {
		t.Skipf("symlinks unavailable: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "existing-outside")
	write(t, outside, "outside\n")
	before := nativeAttemptTreeSnapshot(t, repo)
	if _, err := nativeAttemptInputIdentities(t.Context(), identity, outside); err == nil {
		t.Fatal("accepted existing absolute input outside repository")
	}
	if got := nativeAttemptTreeSnapshot(t, repo); got != before {
		t.Fatalf("outside-path rejection mutated repository: %q != %q", got, before)
	}
	if _, err := nativeAttemptInputIdentities(t.Context(), identity, "missing"); err == nil {
		t.Fatal("accepted missing input")
	}
	if _, err := nativeAttemptCanonicalPath(repo, ".GIT"); err == nil {
		t.Fatal("accepted unsupported repository control directory")
	}
}

func nativeAttemptIdentityFixture(t *testing.T) string {
	t.Helper()
	repo := initRuntimeLedgerRepo(t)
	write(t, filepath.Join(repo, "node"), "node\n")
	if err := os.Chmod(filepath.Join(repo, "node"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, "static.json"), "{}\n")
	write(t, filepath.Join(repo, "src", "migrations", "one.ts"), "export {}\n")
	return repo
}

func nativeAttemptTreeSnapshot(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%q %v %d\n", filepath.ToSlash(relative), info.Mode(), info.ModTime().UnixNano())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(hash, "link %q\n", target)
		case info.Mode().IsRegular():
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(hash, "file %d\n", len(payload))
			_, _ = hash.Write(payload)
		case info.IsDir():
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				_, _ = fmt.Fprintf(hash, "entry %q %v\n", entry.Name(), entry.Type())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(hash.Sum(nil))
}
