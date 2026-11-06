//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixCanonicalRequestKeepsUncheckablePathsVerbatim(t *testing.T) {
	parent := t.TempDir()
	canonical, err := userCanonicalPath(filepath.Join(parent, "shell"))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(parent)
	if err != nil || canonical != filepath.Join(physical, "shell") {
		t.Fatalf("canonical spelling = %q, want below %q (%v)", canonical, physical, err)
	}
	req := UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "separate", SharedAgent: "relative/agent", Confirmation: "token"}
	got, err := userCanonicalRequest(req)
	if err != nil || got.Destination != canonical || got.SharedPrefix != "" || got.SharedAgent != "relative/agent" || got.Mode != "separate" || got.Confirmation != "token" {
		t.Fatalf("canonical request = %+v %v", got, err)
	}
	if again, err := userCanonicalRequest(got); err != nil || again != got {
		t.Fatalf("canonicalization is not idempotent: %+v %v", again, err)
	}
	if _, err := os.Lstat(canonical); !os.IsNotExist(err) {
		t.Fatalf("canonicalization had effects: %v", err)
	}
}

// The publish tail runs the ephemeral cleanup only after a successful rename,
// readback and launcher publication; an uncertain publish must not prune the
// renamed evidence tree.
func TestUserPublishManifestSkipsEphemeralCleanupOnUncertainPublish(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	stage, root := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
	for _, dir := range []string{stage, stage + "/prefix", stage + "/agent", stage + "/bin"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := userManifest{Schema: userSchema, Destination: root, Prefix: stage + "/prefix", Agent: stage + "/agent", Mode: "separate"}
	if manifest.PrefixIdentity, err = userIdentity(stage + "/prefix"); err != nil {
		t.Fatal(err)
	}
	if manifest.AgentIdentity, err = userIdentity(stage + "/agent"); err != nil {
		t.Fatal(err)
	}
	original := userPublishEphemeralCleanup
	called := false
	userPublishEphemeralCleanup = func(string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { userPublishEphemeralCleanup = original })
	if _, publishErr := userPublishManifest(context.Background(), stage, stage+"/prefix", stage+"/agent", manifest); publishErr == nil {
		t.Fatal("incomplete staged tree published successfully")
	} else {
		var failure *PrivateRuntimeError
		if !errors.As(publishErr, &failure) || failure.Kind != "uncertain" {
			t.Fatalf("publish failure is not an uncertain rename outcome: %v", publishErr)
		}
	}
	if called {
		t.Fatal("ephemeral cleanup ran on an uncertain publish")
	}
	if _, err := os.Lstat(filepath.Join(root, "installation.json")); err != nil {
		t.Fatalf("uncertain publish lost the renamed evidence tree: %v", err)
	}
}

// The staged Unix install runs the network-bound provision at least twice
// (global install, then the verify readback) and the main channel adds
// further npm work, so the external install deadline must stay strictly above
// the npm budget of every provision invocation it contains. A bound at or
// below that product reproduces issue #5714: the deadline expires after the
// publish step and the installation is reported unqualified.
func TestUnixInstallBoundCoversNpmNetworkBudget(t *testing.T) {
	npm := userScriptsNpmNetworkTimeout(t)
	if userUnixNpmNetworkTimeout != npm {
		t.Fatalf("userUnixNpmNetworkTimeout %s differs from the scripts' NPM_NETWORK_TIMEOUT %s", userUnixNpmNetworkTimeout, npm)
	}
	if userUnixProvisionBound != userUnixNpmNetworkTimeout {
		t.Fatalf("userUnixProvisionBound %s differs from the npm budget %s", userUnixProvisionBound, userUnixNpmNetworkTimeout)
	}
	if userUnixInstallBound <= 3*userUnixNpmNetworkTimeout {
		t.Fatalf("userUnixInstallBound %s does not exceed three network-bound provision invocations at the npm budget %s each", userUnixInstallBound, userUnixNpmNetworkTimeout)
	}
}
