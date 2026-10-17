//go:build linux && shell_context7_registration

package shellinstaller

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Actual published SDK/resource-loader registration, not a fake ExtensionAPI.
// Direct MCP tool callbacks do not certify the agent permission/model pipeline.
func TestUserContext7FrozenRegistration(t *testing.T) {
	data, err := os.ReadFile("/source/scripts/user-locks/modern/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Packages map[string]struct{ Version, Resolved, Integrity string }
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	pin := lock.Packages["node_modules/@earendil-works/pi-coding-agent"]
	if pin.Version != "1.0.0" || pin.Resolved != "https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-1.0.0.tgz" {
		t.Fatal("fixture must use the committed frozen Pi archive")
	}
	archive, err := os.ReadFile("/frozen-pi.tgz")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512(archive)
	if "sha512-"+base64.StdEncoding.EncodeToString(digest[:]) != pin.Integrity {
		t.Fatal("archive differs from frozen lock; do not execute")
	}
	pkg, root := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := context7ProofCommand(ctx, "/usr/bin/tar", "-xzf", "/frozen-pi.tgz", "--strip-components=1", "-C", pkg)
	if output, err := context7ProofOutput(cmd); err != nil {
		t.Fatalf("authenticated extraction: %v\n%s", err, output)
	}
	if err := os.Symlink("/node_modules/@earendil-works/pi-coding-agent/node_modules", filepath.Join(pkg, "node_modules")); err != nil {
		t.Fatal(err)
	}
	script, marker, cases := "/source/e2e/shell-context7-registration.mjs", "FROZEN_PI_REGISTRATION_PASS", 4
	if os.Getenv("GENTLE_EXPERIENCE_PRIVATE_CONTEXT7_PROBE") == "1" {
		script, marker, cases = "/source/e2e/shell-context7-private.mjs", "FROZEN_PI_PRIVATE_POLICY_PASS", 7
	}
	if os.Getenv("GENTLE_EXPERIENCE_PRIVATE_CONTEXT7_CLI_PROBE") == "1" {
		script, marker, cases = "/source/e2e/shell-context7-private-cli.mjs", "FROZEN_PI_PRIVATE_CLI_PASS", 3
	}
	cmd = context7ProofCommand(ctx, "/opt/node/bin/node", script, pkg, root)
	cmd.Dir = root
	cmd.Env = []string{"HOME=" + root, "PATH=/opt/node/bin:/usr/bin:/bin", "GENTLE_AI_TELEMETRY_DISABLE=1"}
	output, err := context7ProofOutput(cmd)
	if err != nil {
		t.Fatalf("actual SDK registration: %v\n%s", err, output)
	}
	if strings.Count(string(output), marker) != cases {
		t.Fatalf("incomplete actual consumer controls: %s", output)
	}
	t.Logf("%s", output)
}
