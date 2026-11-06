//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// NativeInstallerRuntime is a process client, not installation authority or a
// sandbox. The caller must authenticate the whole source snapshot, keep it
// immutable, establish the existing bounded worker, restore the terminal and
// obtain physical consent before preparing dependencies or calling RunWithLocation.
// This transport does not replace the qualified npm backend or publish roots.
type NativeInstallerRuntime struct {
	root, source, node, sourceStamp string
	sourceSnapshot                  *userSnapshotArtifact
	publicationData                 bool
}

// NativeInstallerPlan retains the exact release plan for this private client.
// A copy for display cannot mutate the reviewed snapshot.
type NativeInstallerPlan struct {
	owner        *NativeInstallerRuntime
	data         json.RawMessage
	bootstrapEnv []string
}

func (p NativeInstallerPlan) JSON() []byte { return append([]byte(nil), p.data...) }

// NativeInstallerResult preserves the native verdict and full result; neither
// "ready" nor a successful process exit proves publication or installed health.
type NativeInstallerResult struct {
	Outcome   string          `json:"outcome"`
	Reason    string          `json:"reason"`
	Completed []string        `json:"completed"`
	Details   json.RawMessage `json:"-"`
}

func NewNativeInstallerRuntime(ctx context.Context, root, source string) (*NativeInstallerRuntime, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return nil, err
	}
	if !privateHierarchyPath(root) || !privateHierarchyPath(source) {
		return nil, privateError("refused", nil)
	}
	if err := nativeRuntimePaths(root); err != nil {
		return nil, err
	}
	// This is only a physical entry check; it does not authenticate imports.
	entry, err := privateForeignPhysical(filepath.Join(source, "scripts/installer-runtime.mjs"))
	if err != nil {
		return nil, err
	}
	node := filepath.Join(root, "runtime/node/bin/node")
	return &NativeInstallerRuntime{root: root, source: source, node: node, sourceStamp: privateStamp(entry)}, nil
}

func (w *NativeInstallerRuntime) invokeWithEnvironment(ctx context.Context, operation string, plan json.RawMessage, consent bool, env []string) (json.RawMessage, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return nil, err
	}
	// A snapshot-bound client refuses complete-tree drift before any process.
	// The caller still owns source authorization and the immutable mount.
	if err := w.nativeSourceCheck(ctx); err != nil {
		return nil, err
	}
	// Recheck owned paths/Node and the source entry before every process.
	if err := nativeRuntimePaths(w.root); err != nil {
		return nil, err
	}
	if err := UserKernelCheck(); err != nil {
		return nil, privateError("refused", err)
	}
	entry, err := privateForeignPhysical(filepath.Join(w.source, "scripts/installer-runtime.mjs"))
	if err != nil || privateStamp(entry) != w.sourceStamp {
		return nil, privateError("refused", err)
	}
	if _, err := privateNativeFile(ctx, w.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		return nil, nativeRuntimeError(ctx, err)
	}
	request := struct {
		Operation string          `json:"operation"`
		Plan      json.RawMessage `json:"plan,omitempty"`
		Consent   bool            `json:"consent"`
		Bootstrap bool            `json:"bootstrap,omitempty"`
	}{operation, plan, consent, len(env) != 0}
	input, err := json.Marshal(request)
	if err != nil || len(input) > 4096 {
		return nil, privateError("refused", err)
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, w.node, "--liftoff-only", "--input-type=module", "-e", nativeRuntimeClient, w.source)
	cmd.Dir = filepath.Join(w.root, "project")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = w.nativeEnvironment()
	if len(env) != 0 {
		cmd.Env = append([]string(nil), env...)
	}
	// Existing bounded output, process-group kill, wait/reap and error taxonomy.
	output, err := userOwnedRun(child, cmd, cancel)
	if err != nil {
		return nil, userNativePrepareDiagnostic(w.root, operation, output, nativeRuntimeError(ctx, err))
	}
	data := json.RawMessage(output)
	if !json.Valid(data) {
		return nil, privateError("output", nil)
	}
	return data, nil
}

func (w *NativeInstallerRuntime) nativeEnvironment() []string {
	dataHome := filepath.Join(w.root, "home/.local/share")
	if w.publicationData {
		dataHome = filepath.Join(w.root, "data")
	}
	return []string{
		"HOME=" + filepath.Join(w.root, "home"), "TMPDIR=" + filepath.Join(w.root, "tmp"),
		"XDG_CONFIG_HOME=" + filepath.Join(w.root, "config"), "XDG_STATE_HOME=" + filepath.Join(w.root, "state"),
		"XDG_DATA_HOME=" + dataHome, "PNPM_HOME=" + filepath.Join(w.root, "pnpm"),
		"PI_CODING_AGENT_DIR=" + filepath.Join(w.root, "agent"), "GENTLE_PI_CONFIG_ROOT=" + filepath.Join(w.root, "config"),
		"NPM_CONFIG_USERCONFIG=" + filepath.Join(w.root, "config/user.npmrc"), "NPM_CONFIG_GLOBALCONFIG=" + filepath.Join(w.root, "config/global.npmrc"),
		"NODE_USE_SYSTEM_CA=1", "SHELL=/usr/bin/bash",
		// Keep nested runtime pools small; never inherit caller runtime options.
		"GOMAXPROCS=1",
		"UV_THREADPOOL_SIZE=1",
		"NODE_OPTIONS=--v8-pool-size=1",
		// An explicit private store avoids pnpm probing cwd with a temporary file.
		"PNPM_CONFIG_STORE_DIR=" + filepath.Join(w.root, "pnpm/store"),
		"PATH=" + filepath.Dir(w.node) + ":" + filepath.Join(w.root, "pnpm") + ":/usr/bin:/bin",
	}
}

func nativeRuntimeError(ctx context.Context, err error) error {
	var failure *PrivateRuntimeError
	if errors.As(err, &failure) && failure.Kind == "uncertain" {
		return err // Context cannot downgrade unknown process-group settlement.
	}
	if ctx == nil {
		return privateError("canceled", err)
	}
	if ctx.Err() == nil {
		return err
	}
	kind := "canceled"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = "deadline"
	}
	return privateError(kind, errors.Join(err, ctx.Err()))
}

func nativeRuntimePaths(root string) error {
	for _, name := range []string{"", "home", "tmp", "config", "state", "agent", "project", "pnpm", "pnpm/store", "pnpm/store/v11", "home/.local", "home/.local/share", "runtime", "runtime/native-bootstrap", "runtime/native-bootstrap/pnpm", "runtime/native-bootstrap/pnpm/bin", "runtime/native-bootstrap/pnpm/package"} {
		path := filepath.Join(root, name)
		_, err := os.Lstat(path)
		if os.IsNotExist(err) && (name == "pnpm" || strings.HasPrefix(name, "pnpm/store") || strings.HasPrefix(name, "home/.local") || name == "runtime" || strings.HasPrefix(name, "runtime/native-bootstrap")) {
			continue
		}
		if _, err := privateDirectory(path); err != nil {
			return err
		}
	}
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		info, err := privatePhysical(filepath.Join(root, "config", name))
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		if info.Mode() != 0600 || st.Uid != uint32(os.Getuid()) || st.Nlink != 1 {
			return privateError("refused", nil)
		}
	}
	return nil
}

// Invocation glue only: no server, web entry, bootstrap reconstruction or
// installation actions here. The existing shared factory owns their logic.
const nativeRuntimeClient = `
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
const request = JSON.parse(readFileSync(0, "utf8"));
const { createInstallerRuntime } = await import(pathToFileURL(join(process.argv[1], "scripts/installer-runtime.mjs")).href);
async function consume(env) {
  const withLocation = request.operation === "run-with-location";
  let installation;
  let failureTail;
  const options = { env };
  if (withLocation) options.onInstallationVerified = (value) => {
    if (installation !== undefined || !Object.isFrozen(value)) throw new Error("Invalid installation handoff");
    installation = value;
  };
  if (withLocation) {
    const { hostAdapters } = await import(pathToFileURL(join(process.argv[1], "scripts/installer-probes.mjs")).href);
    const adapters = hostAdapters();
    options.adapters = { ...adapters, run: async (...args) => {
      const result = await adapters.run(...args);
      if (result.code !== 0 && typeof result.stderrTail === "string") {
        failureTail = Buffer.from(result.stderrTail).subarray(-4096);
      }
      return result;
    }};
  }
  const runtime = createInstallerRuntime(options);
  const respond = (result) => withLocation ? { result, installation: installation ?? null } : result;
  const fresh = await runtime.collectPlan("release");
  if (request.operation === "prepare") return { plan: fresh.plan, env };
  if (request.operation === "collect") return fresh.plan;
  if (request.operation !== "run" && !withLocation) throw new Error("Unsupported native runtime operation");
  if (JSON.stringify(request.plan) !== JSON.stringify(fresh.plan)) {
    return respond({ outcome: "blocked", reason: "selection-changed", completed: [] });
  }
  failureTail = undefined; // Planning probes are not installation diagnostics.
  const result = await runtime.runInstall({ plan: fresh.plan, consent: request.consent });
  if (withLocation && result.outcome === "failed" && failureTail?.length) {
    // Keep the original result unchanged; diagnostics remain in the private workspace.
    writeFileSync(join(process.env.HOME, "../native-setup.stderr"), failureTail, { flag: "wx", mode: 0o600 });
  }
  return respond(result);
}
let reply;
if (request.bootstrap) {
  if (request.consent !== true) throw new Error("Physical consent required before bootstrap");
  const { bootstrap } = await import(pathToFileURL(join(process.argv[1], "scripts/installer-downloads.mjs")).href);
  reply = await bootstrap(process.argv[1], join(process.env.HOME, "../runtime/native-bootstrap"), {
    env: process.env, run: async ({ env }) => await consume(env),
  });
} else {
  reply = await consume(process.env);
}
process.stdout.write(JSON.stringify(reply));
`
