//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func userRecoveryFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir()) // darwin spells TMPDIR through /var.
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	prefix, agent := filepath.Join(root, "prefix"), filepath.Join(root, "agent")
	for _, name := range []string{"state", "prefix", "agent", "state/prefix.preimage", "state/agent.preimage"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(map[string]string{"mode": "shared", "prefix": prefix, "agent": agent, "finalPrefix": prefix, "finalRoot": root,
		"prefixSHA": strings.Repeat("a", 64), "agentSHA": strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state/selection.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return root, prefix, agent
}

func recoveryInspect(t *testing.T, root string) string {
	t.Helper()
	var output bytes.Buffer
	if err := userRecover(context.Background(), []string{root, "inspect"}, &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestUserSharedFailureRetainsOnlyAfterProvisioning(t *testing.T) {
	for _, started := range []bool{false, true} {
		workspace, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(workspace, 0700); err != nil {
			t.Fatal(err)
		}
		identity, err := privateDirectory(workspace)
		if err != nil {
			t.Fatal(err)
		}
		cause := privateError("source", errors.New("refused before provisioning"))
		req := UserInstallRequest{Mode: "shared", Destination: filepath.Join(workspace, "target")}
		_, err = userInstallFinish(context.Background(), UserInstallResult{}, cause, workspace, identity, req, started)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || !errors.Is(err, cause) {
			t.Fatalf("original failure lost: %v", err)
		}
		_, statErr := os.Stat(workspace)
		if started && (failure.Kind != "uncertain" || statErr != nil) {
			t.Fatalf("possible shared effects lost evidence: %v %v", err, statErr)
		}
		if !started && (failure.Kind != "source" || !os.IsNotExist(statErr)) {
			t.Fatalf("clean refusal became uncertain or retained staging: %v %v", err, statErr)
		}
	}
}

func TestUserRecoveryConfirmationIgnoresDamagedCurrentTree(t *testing.T) {
	root, prefix, agent := userRecoveryFixture(t)
	before := recoveryInspect(t, root)
	if err := os.Symlink("missing-package", filepath.Join(prefix, "dangling")); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(agent, "oversized"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate((32 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got := recoveryInspect(t, root); got != before {
		t.Fatal("untrusted current contents changed saved-preimage authority")
	}
}

func TestUserRecoveryConfirmationBindsSavedDataAndRootIdentity(t *testing.T) {
	root, prefix, _ := userRecoveryFixture(t)
	before := recoveryInspect(t, root)
	if err := os.WriteFile(filepath.Join(root, "state/agent.preimage/settings.json"), []byte("saved settings"), 0600); err != nil {
		t.Fatal(err)
	}
	after := recoveryInspect(t, root)
	if after == before {
		t.Fatal("saved preimage changes did not invalidate confirmation")
	}
	oldToken := strings.Fields(before)[2]
	if err := userRecover(context.Background(), []string{root, oldToken}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "fresh recovery confirmation differs") {
		t.Fatalf("stale snapshot confirmation reached restore: %v", err)
	}
	if err := os.Rename(prefix, prefix+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(prefix, 0700); err != nil {
		t.Fatal(err)
	}
	if recoveryInspect(t, root) == after {
		t.Fatal("selected root replacement did not invalidate confirmation")
	}
	if err := os.Chmod(prefix, 0770); err != nil {
		t.Fatal(err)
	}
	if err := userRecover(context.Background(), []string{root, "inspect"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unsafe selected root accepted for recovery")
	}
}
