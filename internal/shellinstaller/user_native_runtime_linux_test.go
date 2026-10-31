//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Actual factory/runner from the authenticated read-only DATA packet. This is
// transport qualification inside an already bounded Guest, not user-manager or
// installed-product acceptance. No fake approval or acquired dependencies.
func TestUserNativeInstallerRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("external retained native runtime transport")
	}
	if os.Getuid() != 1000 {
		t.Fatal("requires the bounded native UID1000 Guest")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("HOME")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := installStagePath(t)
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	nodeDir := filepath.Join(root, "runtime/node/bin")
	if err := os.MkdirAll(nodeDir, 0700); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(nodeDir, "node")
	cmd := exec.CommandContext(ctx, "/usr/bin/cp", "--no-dereference", "/opt/node/bin/node", node)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal("retained Node DATA", err, string(output))
	}
	if err := os.Chmod(node, 0700); err != nil {
		t.Fatal(err)
	}
	worker, err := NewNativeInstallerRuntime(ctx, root, "/alan-source")
	if err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(root, "project", "preserved")
	if err := os.WriteFile(canary, []byte("preserve\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// The consent-tree observer refuses this genuine 123MB Node. Keep its
	// existing bound; independently compare named writable trees and Node.
	stateStamp := func() string {
		var stamps []string
		for _, name := range []string{"home", "tmp", "config", "state", "agent", "project", "bin"} {
			stamp, err := userTreeStamp(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			stamps = append(stamps, stamp)
		}
		stamp, err := privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(append(stamps, stamp), "\n")
	}
	before := stateStamp()
	data, err := worker.invokeWithEnvironment(ctx, "collect", nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := NativeInstallerPlan{owner: worker, data: data}
	var decoded struct {
		Tools map[string]struct {
			Status string
		}
	}
	if err := json.Unmarshal(plan.JSON(), &decoded); err != nil || decoded.Tools["pnpm"].Status != "unavailable" {
		t.Fatal("fresh private runtime must disclose its missing pnpm", err, string(plan.JSON()))
	}
	for _, tt := range []struct {
		name    string
		consent bool
		reason  string
	}{
		{"no-consent", false, "consent-required"},
		{"confirmed-but-blocked", true, "preflight-blocked"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := worker.invokeWithEnvironment(ctx, "run", plan.JSON(), tt.consent, plan.bootstrapEnv)
			var result NativeInstallerResult
			if err == nil {
				err = json.Unmarshal(data, &result)
			}
			result.Details = append(json.RawMessage(nil), data...)
			if err != nil || result.Outcome != "blocked" || result.Reason != tt.reason || len(result.Completed) != 0 {
				t.Fatal(result, err)
			}
			located, installation, err := worker.RunWithLocation(ctx, plan, tt.consent)
			if err != nil || installation != nil || string(located.Details) != string(result.Details) {
				t.Fatal("opt-in changed blocked result", located, installation, err)
			}
		})
	}
	t.Run("stale-plan", func(t *testing.T) {
		stale := plan
		stale.data = json.RawMessage(`{}`)
		result, _, err := worker.RunWithLocation(ctx, stale, true)
		if err != nil || result.Outcome != "blocked" || result.Reason != "selection-changed" || len(result.Completed) != 0 {
			t.Fatal(result, err)
		}
	})
	t.Run("caller-cannot-mutate-reviewed-plan", func(t *testing.T) {
		data := plan.JSON()
		data[0] = '['
		if !json.Valid(plan.JSON()) {
			t.Fatal("reviewed plan exposed mutable storage")
		}
	})
	t.Run("canceled-before-start", func(t *testing.T) {
		stopped, stop := context.WithCancel(ctx)
		stop()
		_, _, err := worker.RunWithLocation(stopped, plan, true)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "canceled" || !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("deadline-refusal", func(t *testing.T) {
		bounded, stop := context.WithTimeout(ctx, time.Millisecond)
		defer stop()
		_, err := worker.invokeWithEnvironment(bounded, "collect", nil, false, nil)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "deadline" {
			t.Fatal(err)
		}
	})
	if now, err := os.Getwd(); err != nil || now != cwd || os.Getenv("HOME") != home {
		t.Fatal("caller cwd/environment changed", err)
	}
	if stateStamp() != before {
		t.Fatal("transport changed private writable trees or Node")
	}
	if _, err := os.Lstat(filepath.Join(root, "pnpm")); !os.IsNotExist(err) {
		t.Fatal("blocked transport created PNPM_HOME", err)
	}
	if got, err := os.ReadFile(canary); err != nil || string(got) != "preserve\n" {
		t.Fatal("caller project canary changed", err)
	}
	for _, name := range []string{"pnpm", "pnpm/store", "pnpm/store/v11", "config/user.npmrc"} {
		t.Run("refuse-escape-"+name, func(t *testing.T) {
			fixture := installStagePath(t)
			if err := userPrepareInstallStage(ctx, fixture, "separate", nil); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(fixture, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if name == "config/user.npmrc" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(t.TempDir(), path); err != nil {
				t.Fatal(err)
			}
			if nativeRuntimePaths(fixture) == nil {
				t.Fatal("escaped private path accepted")
			}
		})
	}
	t.Log("NATIVE_RUNTIME_TRANSPORT_READBACK source_factory=true source_runner=true blocked=true installed_acceptance=false")
}
