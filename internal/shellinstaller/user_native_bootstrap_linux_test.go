//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestUserNativeInstallerBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("authentic bootstrap prerequisites require the bounded offline Guest")
	}
	if os.Getuid() != 1000 {
		t.Fatal("bootstrap qualification requires Guest UID1000")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := installStagePath(t)
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	// Whole reviewed local source DATA, not the unmodified Main archive or a
	// published supplier. The Guest carrier is frozen outside this worker.
	data, err := os.ReadFile("/native-source.zip")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := userMainDescriptor("73960fcf003b37e612a7b18960d6acf75f33a517", "feed2eb9961c6ab3b3a3d55ea314bfca881a5d901c44f97493231d72e0326453")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	w, err := userNativeRuntimeFromSnapshot(ctx, root, artifact)
	if err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(root, "runtime/native-bootstrap")
	t.Run("no-consent-before-prerequisites", func(t *testing.T) {
		_, err := w.Prepare(ctx, false)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "refused" {
			t.Fatalf("consent refusal: %v", err)
		}
		if _, err := os.Lstat(tools); !os.IsNotExist(err) {
			t.Fatalf("unconsented tooling created: %v", err)
		}
	})
	t.Run("pre-canceled-before-prerequisites", func(t *testing.T) {
		stopped, stop := context.WithCancel(ctx)
		stop()
		_, err := w.Prepare(stopped, true)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation cause: %v", err)
		}
	})
	if err := os.MkdirAll(filepath.Dir(w.node), 0700); err != nil {
		t.Fatal(err)
	}
	cp := exec.CommandContext(ctx, "/usr/bin/cp", "--no-dereference", "/opt/node/bin/node", w.node)
	if _, err := userOwnedRun(ctx, cp, cancel); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(w.node, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	// Original bootstrap verifies the authentic held archive and creates its own
	// shim. Only byte transport is injected; no digest/process/installer adapter.
	seed := exec.CommandContext(ctx, w.node, "--liftoff-only", "--input-type=module", "-e", nativeBootstrapFixture, w.source, tools)
	seed.Dir = filepath.Join(root, "project")
	seed.Env = w.nativeEnvironment()
	if _, err := userOwnedRun(ctx, seed, cancel); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(tools, "pnpm/bin/pnpm")
	before, err := userTreeStamp(tools)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Prepare(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(plan.JSON()) || !slices.Contains(plan.bootstrapEnv, "GENTLE_BOOTSTRAP_TOOLS="+filepath.Join(root, "runtime")) {
		t.Fatal("prepared plan/environment or original bootstrap-only marker missing")
	}
	t.Run("tooling-remains-through-consumer-return", func(t *testing.T) {
		if _, err := os.Lstat(shim); err != nil {
			t.Fatal(err)
		}
		after, err := userTreeStamp(tools)
		if err != nil || before != after {
			t.Fatalf("owned tooling changed: %v", err)
		}
	})
	t.Run("prepared-run-requires-consent", func(t *testing.T) {
		_, _, err := w.RunWithLocation(ctx, plan, false)
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "refused" {
			t.Fatalf("prepared consent refusal: %v", err)
		}
	})
	t.Run("fresh-plan-change-blocks-installation", func(t *testing.T) {
		changed := plan
		changed.data = json.RawMessage(`{"different":true}`)
		result, _, err := w.RunWithLocation(ctx, changed, true)
		if err != nil || result.Outcome != "blocked" || result.Reason != "selection-changed" || len(result.Completed) != 0 {
			t.Fatalf("stale plan: %+v %v", result, err)
		}
		if _, err := os.Lstat(shim); err != nil {
			t.Fatal("bootstrap removed caller-owned tools", err)
		}
	})
	if t.Failed() {
		return
	}
	t.Log("NATIVE_BOOTSTRAP_READBACK original_bootstrap=true authentic_pnpm=true supplied_tools_retained=true stale_plan_blocked=true complete_local_source=true installed_acceptance=false")
}

const nativeBootstrapFixture = `
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
const { bootstrap } = await import(pathToFileURL(join(process.argv[1], "scripts/installer-downloads.mjs")).href);
await bootstrap(process.argv[1], process.argv[2], {
  env: process.env,
  adapters: { download: async () => readFileSync("/pnpm-11.1.1.tgz") },
  run: async () => { process.stdout.write("AUTHENTIC_PNPM_FIXTURE_PREPARED"); },
});
`
