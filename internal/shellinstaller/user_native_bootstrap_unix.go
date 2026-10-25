//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Prepare requires the caller's existing physical consent BEFORE prerequisites.
// Original bootstrap owns acquisition/validation. Supplied tooling belongs to
// the private worker, survives callback return, and is cleaned by its caller
// only after settlement. This is neither a new TUI nor publication authority.
func (w *NativeInstallerRuntime) Prepare(ctx context.Context, consent bool) (NativeInstallerPlan, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return NativeInstallerPlan{}, err
	}
	if !consent {
		return NativeInstallerPlan{}, privateError("refused", nil)
	}
	if err := w.nativeSourceCheck(ctx); err != nil {
		return NativeInstallerPlan{}, err
	}
	if err := nativeRuntimePaths(w.root); err != nil {
		return NativeInstallerPlan{}, err
	}
	if _, err := privateDirectory(filepath.Join(w.root, "runtime")); err != nil {
		return NativeInstallerPlan{}, err
	}
	tools := filepath.Join(w.root, "runtime/native-bootstrap")
	if err := os.Mkdir(tools, 0700); err != nil && !os.IsExist(err) {
		return NativeInstallerPlan{}, privateError("filesystem", err)
	}
	if _, err := privateDirectory(tools); err != nil {
		return NativeInstallerPlan{}, err
	}
	// Existing probe policy must recognize BOTH private Node and PNPM as
	// bootstrap-only, not installed/persistent runtime.
	env := append(w.nativeEnvironment(), "GENTLE_BOOTSTRAP_TOOLS="+filepath.Join(w.root, "runtime"))
	bin := filepath.Join(tools, "pnpm/bin")
	for i, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			env[i] = "PATH=" + bin + ":" + strings.TrimPrefix(entry, "PATH=")
		}
	}
	data, err := w.invokeWithEnvironment(ctx, "prepare", nil, true, env)
	if err != nil {
		return NativeInstallerPlan{}, err
	}
	var prepared struct {
		Plan json.RawMessage   `json:"plan"`
		Env  map[string]string `json:"env"`
	}
	if json.Unmarshal(data, &prepared) != nil || len(prepared.Plan) == 0 || prepared.Plan[0] != '{' || len(prepared.Env) != len(env) {
		return NativeInstallerPlan{}, privateError("output", nil)
	}
	// Capture the entire original callback environment; don't reconstruct the
	// helper's PATH handoff. This source only changes PATH; other private values
	// must remain exactly those supplied by the worker.
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if prepared.Env[key] != value && (key != "PATH" || prepared.Env[key] != bin+":"+value) {
			return NativeInstallerPlan{}, privateError("output", nil)
		}
	}
	captured := make([]string, 0, len(prepared.Env))
	for key, value := range prepared.Env {
		captured = append(captured, key+"="+value)
	}
	sort.Strings(captured)
	return NativeInstallerPlan{owner: w, data: append(json.RawMessage(nil), prepared.Plan...), bootstrapEnv: captured}, nil
}
