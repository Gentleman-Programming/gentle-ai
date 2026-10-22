//go:build linux && amd64 && shell_main_bindings

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Authenticate the original package, then test the production binding comparator
// over actual private Main materialization. ZIP DATA is not publisher TLS proof.
func TestUserMainDeclaredStockBindings(t *testing.T) {
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
		t.Fatal("stock fixture differs from committed SRI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	stock := t.TempDir()
	cmd := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", "/frozen-gentle-pi.tgz", "--strip-components=1", "-C", stock)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	names := []string{"package.json", "scripts/gentle-ai-installer.mjs", "runtime/gentle-ai-binary.mjs"}
	original := map[string][]byte{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(stock, name))
		if err != nil {
			t.Fatal(err)
		}
		original[name] = data
	}
	if installer := sha256.Sum256(original[names[1]]); hex.EncodeToString(installer[:]) != userInstallerSHA {
		t.Fatal("stock installer pin differs")
	}
	if resolver := sha256.Sum256(original[names[2]]); hex.EncodeToString(resolver[:]) != privateNativeResolverSHA {
		t.Fatal("stock resolver pin differs")
	}
	for _, field := range []string{"unchanged", "dependencies", "exports", "pi", "engines", "scripts", "unknown", names[1], names[2]} {
		t.Run(field, func(t *testing.T) {
			files := map[string][]byte{}
			for name, data := range original {
				files[name] = append([]byte(nil), data...)
			}
			expected := "accept"
			if field == names[1] || field == names[2] {
				files[field] = append(files[field], []byte("\n// changed supplier\n")...)
				expected = field
			} else if field != "unchanged" {
				var metadata map[string]json.RawMessage
				if err := json.Unmarshal(files[names[0]], &metadata); err != nil {
					t.Fatal(err)
				}
				metadata[field] = json.RawMessage(`{"unexpected":"changed"}`)
				files[names[0]], err = json.Marshal(metadata)
				if err != nil {
					t.Fatal(err)
				}
				expected = names[0]
			}
			files["README.md"] = []byte("Changed Main source DATA: not executed or API-qualified.")
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			for name, data := range files {
				member, err := writer.Create("gentle-shell-" + commit + "/" + name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := member.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			data := buffer.Bytes()
			digest := sha256.Sum256(data)
			artifact := userSnapshotArtifact{URL: "https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/" + commit,
				SHA: hex.EncodeToString(digest[:]), Archive: "main.zip", Prefix: "gentle-shell-" + commit, Bound: 32 << 20}
			root := userMainCustodyStage(t)
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			if err := userStageMainSource(ctx, root, artifact); err != nil {
				t.Fatal(err)
			}
			preimage, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, "/opt/node/bin/node", "/source/e2e/shell-main-bindings.mjs",
				filepath.Join(root, "runtime/main-source"), stock, expected)
			cmd.Dir, cmd.Env = root, []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatal(err, string(output))
			} else {
				t.Log(string(output))
			}
			if err := ctx.Err(); err != nil {
				t.Fatal("binding process did not settle", err)
			}
			if err := userCompareMainSource(ctx, root, artifact); err != nil {
				t.Fatal(err)
			}
			after, err := userTreeStamp(root)
			if err != nil || after != preimage {
				t.Fatal("binding preflight mutated stage", err)
			}
		})
	}
}
