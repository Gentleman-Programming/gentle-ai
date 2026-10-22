//go:build linux && amd64 && shell_main_sdk_consumer

package shellinstaller

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// No production behavior changes: actual Main registration/lifecycle is a
// qualification prerequisite, not admission, installation or native health.
func TestUserMainFrozenSDKConsumer(t *testing.T) {
	if testing.Short() {
		t.Skip("external retained Main/SDK qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	descriptor, err := os.ReadFile("/frozen-main/main.json")
	if err != nil {
		t.Fatal(err)
	}
	var artifact userSnapshotArtifact
	if err := json.Unmarshal(descriptor, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.SHA != "cbc39d46b0d5416435c22498515e5bdfef8b4368e6a8e9d591f265f2de99362e" ||
		artifact.Prefix != "gentle-shell-43541a0d30dfafacfcb2e99ef4135ab8b1693e99" {
		t.Fatal("retained authorized Main snapshot differs")
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
	// Test-only ancestor links expose original read-only dependency layouts.
	// Neither retained source tree receives a dependency or native overlay.
	if err := os.Symlink("/node_modules/gentle-pi/node_modules", filepath.Join(root, "runtime/node_modules")); err != nil {
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
	archive, err := os.ReadFile("/frozen-pi.tgz")
	if err != nil {
		t.Fatal(err)
	}
	pin := lock.Packages["node_modules/@earendil-works/pi-coding-agent"]
	hash := sha512.Sum512(archive)
	if pin.Version == "" ||
		pin.Resolved != "https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-"+pin.Version+".tgz" ||
		pin.Integrity != "sha512-"+base64.StdEncoding.EncodeToString(hash[:]) {
		t.Fatal("SDK fixture differs from committed npm SRI")
	}
	sdkParent := t.TempDir()
	sdk := filepath.Join(sdkParent, "sdk")
	if err := os.Mkdir(sdk, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", "/frozen-pi.tgz", "--strip-components=1", "-C", sdk)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	if err := os.Symlink("/node_modules/@earendil-works/pi-coding-agent/node_modules", filepath.Join(sdkParent, "node_modules")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"project", "agent", "config", "home"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	project := filepath.Join(root, "project")
	for _, name := range []string{"README.md", "AGENTS.md", "historical-openspec.md"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte("private project preimage: "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	personal := t.TempDir()
	for _, name := range []string{".pi/agent/settings.json", ".pi/gentle-ai/persona.json", "project/AGENTS.md", "project/openspec/specs/history.md"} {
		path := filepath.Join(personal, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("personal preservation fixture: "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd = exec.CommandContext(ctx, "/usr/bin/git", "init", "--quiet", "--template=", project)
	cmd.Env = []string{"HOME=" + filepath.Join(root, "home"), "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1"}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	protected := []string{sdk, personal, filepath.Join(project, ".git"), filepath.Join(project, "README.md"),
		filepath.Join(project, "AGENTS.md"), filepath.Join(project, "historical-openspec.md"),
		filepath.Join(root, "runtime/main"), filepath.Join(root, "runtime/main-source")}
	before := make(map[string]string)
	for _, path := range protected {
		stamp, err := userTreeStamp(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = stamp
	}
	cmd = exec.CommandContext(ctx, "/opt/node/bin/node", "/source/e2e/shell-main-sdk-consumer.mjs", root, sdk)
	cmd.Dir = project
	cmd.Env = []string{"HOME=" + filepath.Join(root, "home"), "PATH=/opt/node/bin:/usr/bin:/bin",
		"PI_CODING_AGENT_DIR=" + filepath.Join(root, "agent"), "GENTLE_PI_CONFIG_HOME=" + filepath.Join(root, "config"),
		"GIT_CONFIG_NOSYSTEM=1"}
	output, runErr := cmd.CombinedOutput()
	t.Log(string(output))
	// Readbacks also run on a refused consumer; a failed prerequisite must not
	// hide changed Git/personal preimages, archive, SDK or source. The actual
	// package's private project cache is checked explicitly by the SDK probe.
	for _, path := range protected {
		after, err := userTreeStamp(path)
		if err != nil || after != before[path] {
			t.Errorf("Main SDK changed protected tree %s: %v", path, err)
		}
	}
	if err := userCompareMainSource(ctx, root, artifact); err != nil {
		t.Error(err)
	}
	if err := ctx.Err(); err != nil {
		t.Error("Main SDK did not settle", err)
	}
	if runErr != nil {
		t.Fatal("actual retained Main SDK consumer refused", runErr)
	}
}
