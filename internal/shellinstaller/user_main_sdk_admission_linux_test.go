//go:build linux && amd64 && shell_main_sdk_admission

package shellinstaller

import (
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

// Actual authorized Main DATA and authenticated original stock qualify only
// the internal composition binding plan, not activation or installed effects.
func TestUserMainSDKPreviewAdmission(t *testing.T) {
	if testing.Short() {
		t.Skip("external retained Main/stock binding qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	artifact, err := userMainDescriptor("43541a0d30dfafacfcb2e99ef4135ab8b1693e99", "cbc39d46b0d5416435c22498515e5bdfef8b4368e6a8e9d591f265f2de99362e")
	if err != nil {
		t.Fatal(err)
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
	// These are fixture identities, not changes to production supplier pins.
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
	pin := lock.Packages["node_modules/gentle-pi"]
	if pin.Version == "" || pin.Resolved != "https://registry.npmjs.org/gentle-pi/-/gentle-pi-"+pin.Version+".tgz" {
		t.Fatal("stock acquisition tuple differs")
	}
	archive, err := os.ReadFile("/frozen-gentle-pi.tgz")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512(archive)
	if pin.Integrity != "sha512-"+base64.StdEncoding.EncodeToString(hash[:]) {
		t.Fatal("stock archive differs from committed SRI")
	}
	stock := t.TempDir()
	cmd := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", "/frozen-gentle-pi.tgz", "--strip-components=1", "-C", stock)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	for name, expected := range map[string]string{
		"scripts/gentle-ai-installer.mjs": userInstallerSHA,
		"runtime/gentle-ai-binary.mjs":    privateNativeResolverSHA,
	} {
		bytes, err := os.ReadFile(filepath.Join(stock, name))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(bytes)
		if hex.EncodeToString(hash[:]) != expected {
			t.Fatal("authenticated stock supplier pin differs", name)
		}
	}
	protected := []string{root, stock}
	before := map[string]string{}
	for _, path := range protected {
		before[path], err = userTreeStamp(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	cmd = exec.CommandContext(ctx, "/opt/node/bin/node", "/source/e2e/shell-main-sdk-admission.mjs", filepath.Join(root, "runtime/main-source"), stock)
	cmd.Dir, cmd.Env = root, []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
	output, runErr := cmd.CombinedOutput()
	t.Log(string(output))
	for _, path := range protected {
		after, err := userTreeStamp(path)
		if err != nil || after != before[path] {
			t.Errorf("composition preflight changed %s: %v", path, err)
		}
	}
	if err := userCompareMainSource(ctx, root, artifact); err != nil {
		t.Error(err)
	}
	if err := ctx.Err(); err != nil {
		t.Error("composition preflight did not settle", err)
	}
	if runErr != nil {
		t.Fatal("Main SDK composition binding plan refused", runErr)
	}
}
