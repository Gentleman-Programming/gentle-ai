//go:build linux && amd64 && shell_main_supplier_protocol

package shellinstaller

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Actual retained Main imports inside the offline Guest, not activation or a
// source-hash exception. The existing exact-byte production gate stays closed.
func TestUserMainSupplierNativeResolverProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("external retained-source/runtime qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	descriptor, err := os.ReadFile("/frozen-main/main.json")
	if err != nil {
		t.Fatal(err)
	}
	var artifact userSnapshotArtifact
	if err := json.Unmarshal(descriptor, &artifact); err != nil {
		t.Fatal(err)
	}
	// Fixture identity is evidence, not a new production supplier/native pin.
	if artifact.SHA != "cbc39d46b0d5416435c22498515e5bdfef8b4368e6a8e9d591f265f2de99362e" ||
		artifact.Prefix != "gentle-shell-43541a0d30dfafacfcb2e99ef4135ab8b1693e99" {
		t.Fatal("retained authorized snapshot differs")
	}
	data, err := os.ReadFile("/frozen-main/main.zip")
	if err != nil {
		t.Fatal(err)
	}
	root := userMainCustodyStage(t)
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	lockData, err := os.ReadFile("/source/scripts/user-locks/modern/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Packages map[string]struct{ Version, Resolved, Integrity string }
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile("/frozen-gentle-pi.tgz")
	if err != nil {
		t.Fatal(err)
	}
	pin := lock.Packages["node_modules/gentle-pi"]
	hash := sha512.Sum512(archive)
	if pin.Version == "" || pin.Resolved != "https://registry.npmjs.org/gentle-pi/-/gentle-pi-"+pin.Version+".tgz" ||
		pin.Integrity != "sha512-"+base64.StdEncoding.EncodeToString(hash[:]) {
		t.Fatal("original supplier fixture differs from committed npm SRI")
	}
	stock := t.TempDir()
	cmd := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", "/frozen-gentle-pi.tgz", "--strip-components=1", "-C", stock)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	nativeRoot := "/node_modules/gentle-pi/.gentle-ai"
	// The retained distribution's bundle version is producer data, never a pin:
	// derive the single v<version> directory and authenticate the binary against
	// the manifest's own binarySha256, as the production readback does.
	bundles, err := os.ReadDir(nativeRoot)
	if err != nil || len(bundles) != 1 || !strings.HasPrefix(bundles[0].Name(), "v") {
		t.Fatal("original native bundle layout differs", err)
	}
	bundleVersion := bundles[0].Name()
	binaryPath := filepath.Join(nativeRoot, bundleVersion, "gentle-ai")
	manifestData, err := os.ReadFile(filepath.Join(nativeRoot, bundleVersion, "integrity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifestPin struct {
		Version      string `json:"version"`
		BinarySha256 string `json:"binarySha256"`
	}
	if json.Unmarshal(manifestData, &manifestPin) != nil || manifestPin.Version != bundleVersion[1:] || manifestPin.BinarySha256 == "" {
		t.Fatal("original native manifest differs")
	}
	if _, err := privateNativeFile(ctx, binaryPath, 0, -1, manifestPin.BinarySha256); err != nil {
		t.Fatal("original authenticated native binary differs", err)
	}
	if _, err := privateNativeFile(ctx, filepath.Join(nativeRoot, bundleVersion, "integrity.json"), 0,
		int64(len(manifestData)), ""); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	nativeBefore, err := userTreeStamp(nativeRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "changed binary", "changed manifest", "noncanonical manifest", "symlink binary"} {
		t.Run(scenario, func(t *testing.T) {
			warm := t.TempDir()
			bundle := filepath.Join(warm, ".gentle-ai", bundleVersion)
			if err := os.MkdirAll(bundle, 0700); err != nil {
				t.Fatal(err)
			}
			content := append([]byte(nil), binary...)
			manifest := append([]byte(nil), manifestData...)
			switch scenario {
			case "changed binary":
				content[0] ^= 1
			case "changed manifest":
				var record map[string]any
				if err := json.Unmarshal(manifest, &record); err != nil {
					t.Fatal(err)
				}
				record["assetSha256"] = "changed"
				manifest, err = json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				manifest = append(manifest, '\n')
			case "noncanonical manifest":
				manifest = append([]byte(" "), manifest...)
			}
			target := filepath.Join(bundle, "gentle-ai")
			if scenario == "symlink binary" {
				if err := os.Symlink(binaryPath, target); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(target, content, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(bundle, "integrity.json"), manifest, 0600); err != nil {
				t.Fatal(err)
			}
			// Node imports the original supplied Main modules, never patched copies.
			cmd := exec.CommandContext(ctx, "/opt/node/bin/node", "/source/e2e/shell-main-supplier-protocol.mjs",
				filepath.Join(root, "runtime/main-source"), stock, warm, scenario)
			cmd.Dir, cmd.Env = warm, []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatal(err, string(output))
			} else {
				t.Log(string(output))
			}
			if err := ctx.Err(); err != nil {
				t.Fatal("supplier/resolver probe did not settle", err)
			}
		})
	}
	if err := userCompareMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	after, err := userTreeStamp(root)
	if err != nil || after != before {
		t.Fatal("Main module imports changed custody/source", err)
	}
	nativeAfter, err := userTreeStamp(nativeRoot)
	if err != nil || nativeBefore != nativeAfter {
		t.Fatal("probe changed original native distribution", err)
	}
}
