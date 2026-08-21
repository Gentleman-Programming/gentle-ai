package pi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func must(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
func writeManifest(t *testing.T, root, manifest string) {
	must(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o644))
}
func writeTarget(t *testing.T, root, relative string, mode os.FileMode) string {
	path := filepath.Join(root, filepath.FromSlash(relative))
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte("#!/bin/sh\n"), mode))
	return path
}
func assertKind(t *testing.T, err error, domain, kind string) {
	var got string
	switch e := err.(type) {
	case *PackageError:
		got = "package:" + e.Kind
	case *ManifestError:
		got = "manifest:" + e.Kind
	case *BinError:
		got = "bin:" + e.Kind
	}
	if got != domain+":"+kind {
		t.Fatalf("error = %T %v; want %s error %q", err, err, domain, kind)
	}
}
func TestResolvePackageBinForms(t *testing.T) {
	const valid, malformed, manifestBound = `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, `{"bin":`, MaxPackageManifestBytes
	snapshot := func(path string) []byte {
		data, err := os.ReadFile(path)
		must(t, err)
		return data
	}
	cases := []struct{ name, manifest, target, link, kind string }{
		{"string", `{"name":"gentle-pi-models","bin":"bin/gentle-pi-models"}`, "bin/gentle-pi-models", "", ""}, {"object and canonical symlink", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "real/gentle-pi-models", "symlink", ""},
		{"exact bound", valid + strings.Repeat(" ", manifestBound-len(valid)), "bin/gentle-pi-models", "", ""}, {"bound plus one", valid + strings.Repeat(" ", manifestBound+1-len(valid)), "bin/gentle-pi-models", "", "manifest-too-large"}, {"malformed within bound", malformed + strings.Repeat(" ", manifestBound-len(malformed)), "bin/gentle-pi-models", "", "malformed-manifest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.link != "" && runtime.GOOS == "windows" {
				t.Skip("symlink permissions vary on Windows")
			}
			root := t.TempDir()
			want := writeTarget(t, root, tc.target, 0o755)
			if tc.link != "" {
				bin := filepath.Join(root, "bin", "gentle-pi-models")
				must(t, os.MkdirAll(filepath.Dir(bin), 0o755))
				must(t, os.Symlink(filepath.Join("..", tc.target), bin))
			}
			writeManifest(t, root, tc.manifest)
			beforeManifest, beforeTarget := snapshot(filepath.Join(root, "package.json")), snapshot(want)
			got, err := ResolvePackageBin(root)
			if tc.kind != "" {
				assertKind(t, err, "manifest", tc.kind)
				return
			}
			if err != nil || got != want {
				t.Fatalf("ResolvePackageBin() = %q, %v; want %q", got, err, want)
			}
			afterManifest, afterTarget := snapshot(filepath.Join(root, "package.json")), snapshot(want)
			if string(beforeManifest) != string(afterManifest) || string(beforeTarget) != string(afterTarget) {
				t.Fatal("resolution mutated the manifest or executable")
			}
		})
	}
}
func TestResolvePackageBinErrors(t *testing.T) {
	cases := []struct{ name, manifest, domain, kind, setup string }{
		{"missing package", "", "package", "missing-package", "package-missing"}, {"missing manifest", "", "manifest", "missing-manifest", ""},
		{"string bin with another package name", `{"name":"other-package","bin":"bin/gentle-pi-models"}`, "bin", "absent-bin", ""}, {"malformed manifest", `{"bin":`, "manifest", "malformed-manifest", ""},
		{"malformed bin", `{"bin":true}`, "bin", "malformed-bin", ""}, {"absent bin", `{"name":"gentle-pi-models"}`, "bin", "absent-bin", ""},
		{"absent object bin", `{"bin":{"other":"bin/other"}}`, "bin", "absent-bin", ""}, {"missing target", `{"bin":{"gentle-pi-models":"bin/missing"}}`, "bin", "missing-bin-target", ""},
		{"non-regular target", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "bin", "non-regular-bin-target", "directory"}, {"non-executable target", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "bin", "non-executable-bin-target", "nonexec"},
		{"absolute target", `{"bin":{"gentle-pi-models":"/outside"}}`, "bin", "unsafe-bin", ""}, {"lexical escape", `{"bin":{"gentle-pi-models":"../outside"}}`, "bin", "unsafe-bin", ""},
		{"duplicate top-level bin", `{"bin":"bin/x","bin":"bin/y"}`, "bin", "malformed-bin", ""}, {"duplicate selected bin", `{"bin":{"gentle-pi-models":"bin/x","gentle-pi-models":"bin/y"}}`, "bin", "malformed-bin", ""}, {"symlink escape", `{"bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`, "bin", "unsafe-bin", "symlink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.setup == "nonexec" || tc.setup == "symlink") && runtime.GOOS == "windows" {
				t.Skip("Windows does not use executable permission bits")
			}
			root := t.TempDir()
			switch tc.setup {
			case "package-missing":
				root = filepath.Join(root, "missing")
			case "directory":
				must(t, os.MkdirAll(filepath.Join(root, "bin", "gentle-pi-models"), 0o755))
			case "nonexec":
				writeTarget(t, root, "bin/gentle-pi-models", 0o644)
			case "symlink":
				outside := writeTarget(t, t.TempDir(), "outside", 0o755)
				bin := filepath.Join(root, "bin", "gentle-pi-models")
				must(t, os.MkdirAll(filepath.Dir(bin), 0o755))
				must(t, os.Symlink(outside, bin))
			}
			if tc.manifest != "" {
				writeManifest(t, root, tc.manifest)
			}
			_, err := ResolvePackageBin(root)
			assertKind(t, err, tc.domain, tc.kind)
			if tc.setup == "symlink" && !errors.As(err, new(*UnsafeBinError)) {
				t.Fatalf("error = %T %v; want UnsafeBinError cause", err, err)
			}
			if (tc.kind == "missing-package" || tc.kind == "missing-manifest" || tc.kind == "missing-bin-target") && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("error = %v; want os.ErrNotExist cause", err)
			}
		})
	}
}
