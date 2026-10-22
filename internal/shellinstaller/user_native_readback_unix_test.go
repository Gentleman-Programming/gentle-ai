//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The native readback derives the bundle version from the stock installer's
// layout and manifest, never from a frozen release literal. A coherent stock
// bundle is admitted; foreign layouts, version drift and byte changes still
// fail closed.
func TestUserNativeReadbackAdmitsCurrentStockBundle(t *testing.T) {
	release := map[string]string{"linux": "linux_amd64", "darwin": "darwin_arm64"}[runtime.GOOS]
	version := "4.2.0"
	binary := []byte("stock gentle-ai binary fixture: " + version)
	manifest := fmt.Sprintf(`{"version":%q,"asset":"gentle-ai_%s_%s.tar.gz","assetSha256":"%s","binarySha256":"%s"}`+"\n",
		version, version, release, strings.Repeat("a", 64), fmt.Sprintf("%x", sha256.Sum256(binary)))
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, ".gentle-ai", "v"+version)
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "gentle-ai"), binary, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "integrity.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userNativeReadback(ctx, filepath.Join(root, ".gentle-ai")); err != nil {
		t.Fatalf("current stock bundle refused: %v", err)
	}
	for _, tt := range []struct {
		name         string
		directory    string
		manifestText string
		binaryData   []byte
		binaryMode   os.FileMode
		manifestMode os.FileMode
	}{
		{"version-drift", "v" + version, strings.Replace(manifest, `"version":"`+version+`"`, `"version":"`+version[:len(version)-1]+"1"+`"`, 1), binary, 0700, 0600},
		{"foreign-method", "v" + version, strings.Replace(manifest, `"asset"`, `"method":"unknown-method","asset"`, 1), binary, 0700, 0600},
		{"wrong-asset", "v" + version, strings.Replace(manifest, release, "other_platform", 1), binary, 0700, 0600},
		{"noncanonical-manifest", "v" + version, " " + manifest, binary, 0700, 0600},
		{"changed-binary", "v" + version, manifest, append([]byte{binary[0] ^ 1}, binary[1:]...), 0700, 0600},
		{"loose-binary-mode", "v" + version, manifest, binary, 0755, 0600},
		{"loose-manifest-mode", "v" + version, manifest, binary, 0700, 0644},
		{"foreign-directory", "not-a-version", manifest, binary, 0700, 0600},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := t.TempDir()
			if err := os.Chmod(fixture, 0700); err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(fixture, ".gentle-ai", tt.directory)
			if err := os.MkdirAll(nested, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(nested, "gentle-ai"), tt.binaryData, tt.binaryMode); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(nested, "integrity.json"), []byte(tt.manifestText), tt.manifestMode); err != nil {
				t.Fatal(err)
			}
			if err := userNativeReadback(ctx, filepath.Join(fixture, ".gentle-ai")); err == nil {
				t.Fatal("foreign or mutated native state accepted")
			}
		})
	}
	for _, name := range []string{"missing-integrity", "extra-member"} {
		t.Run(name, func(t *testing.T) {
			fixture := t.TempDir()
			if err := os.Chmod(fixture, 0700); err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(fixture, ".gentle-ai", "v"+version)
			if err := os.MkdirAll(nested, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(nested, "gentle-ai"), binary, 0700); err != nil {
				t.Fatal(err)
			}
			if name == "extra-member" {
				if err := os.WriteFile(filepath.Join(nested, "integrity.json"), []byte(manifest), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(nested, "extra"), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := userNativeReadback(ctx, filepath.Join(fixture, ".gentle-ai")); err == nil {
				t.Fatal("incomplete or extended bundle accepted")
			}
		})
	}
	duplicate := t.TempDir()
	if err := os.Chmod(duplicate, 0700); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"v" + version, "v3.7.0"} {
		nested := filepath.Join(duplicate, ".gentle-ai", directory)
		if err := os.MkdirAll(nested, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "gentle-ai"), binary, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "integrity.json"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := userNativeReadback(ctx, filepath.Join(duplicate, ".gentle-ai")); err == nil {
		t.Fatal("duplicate native bundles accepted")
	}
}
