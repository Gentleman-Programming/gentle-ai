//go:build linux && amd64 && shell_main_owned_preflight

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Qualify the actual owned stock helper, not the isolated binding comparator.
// Retained inputs are offline DATA. The composed SDK consumer runs only after
// authenticated placement; the Main installer is never executed.
func TestUserMainSDKOwnedStockPreflight(t *testing.T) {
	userMainSDKOwnedStockPreflight(t, false, false)
}

func TestUserMainSDKSelectedPackage(t *testing.T) {
	userMainSDKOwnedStockPreflight(t, true, false)
}

func TestUserMainSDKWorker(t *testing.T) {
	userMainSDKOwnedStockPreflight(t, true, true)
}

func userMainSDKOwnedStockPreflight(t *testing.T, selected, worker bool) {
	if testing.Short() {
		t.Skip("external retained full stock preflight qualification")
	}
	if os.Getuid() != 1000 {
		t.Fatal("qualification requires the bounded native UID1000 guest")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 210*time.Second)
	defer cancel()
	root := userMainCustodyStage(t)
	finalRoot := root
	if worker {
		finalRoot = root + "-published"
	}
	if err := userProvisionAssets(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"runtime/node", "runtime/project", "prefix/lib/node_modules", "prefix/bin"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	copyTree := func(from, to string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "/usr/bin/cp", "-R", "--preserve=mode", "--no-dereference", from+"/.", to)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatal("retained DATA copy", err, string(output))
		}
	}
	copyTree("/opt/node", filepath.Join(root, "runtime/node"))
	copyTree("/node_modules", filepath.Join(root, "prefix/lib/node_modules"))
	// Original modes are part of the retained optional inventory. Extract the
	// authenticated original DATA archive, not a permission-normalized copy.
	cmd := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", "/stock-source.tgz", "--strip-components=1", "--no-same-owner", "--same-permissions", "-C", filepath.Join(root, "runtime/project"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal("retained original producer DATA", err, string(output))
	}
	write := func(name string, data []byte, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(name, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	node := filepath.Join(root, "runtime/node/bin/node")
	if err := os.Chmod(node, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Fatal("original Shell Node pin", err)
	}
	prefix, agent := filepath.Join(root, "prefix"), filepath.Join(root, "agent")
	modules := filepath.Join(prefix, "lib/node_modules")
	for _, name := range []string{"gentle-pi", "@earendil-works/pi-coding-agent", "@earendil-works/pi-tui", "@heyhuynhgiabuu/pi-pretty", "typebox"} {
		var metadata struct {
			Bin json.RawMessage
		}
		if err := json.Unmarshal(read(filepath.Join(modules, name, "package.json")), &metadata); err != nil {
			t.Fatal(err)
		}
		bins := map[string]string{}
		if len(metadata.Bin) != 0 {
			if err := json.Unmarshal(metadata.Bin, &bins); err != nil {
				t.Fatal("retained stock root bin shape", err)
			}
		}
		for command, relative := range bins {
			// Match stock npm's relative bins so physical whole-root rename is
			// not obstructed by an absolute test-only stage pointer.
			target, err := filepath.Rel(filepath.Join(prefix, "bin"), filepath.Join(modules, name, relative))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(prefix, "bin", command)); err != nil {
				t.Fatal(err)
			}
		}
	}
	native := filepath.Join(modules, "gentle-pi/.gentle-ai")
	// The retained stock bundle's version is producer data; derive the single
	// v<version> directory from the layout instead of a frozen literal.
	bundles, err := os.ReadDir(native)
	if err != nil || len(bundles) != 1 || !strings.HasPrefix(bundles[0].Name(), "v") {
		t.Fatal("authentic native bundle layout required", err)
	}
	bundleVersion := bundles[0].Name()
	for _, name := range []string{native, filepath.Join(native, bundleVersion)} {
		if err := os.Chmod(name, 0700); err != nil {
			t.Fatal("authentic native DATA required", err)
		}
	}
	for name, mode := range map[string]os.FileMode{"gentle-ai": 0700, "integrity.json": 0600} {
		if err := os.Chmod(filepath.Join(native, bundleVersion, name), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := userNativeReadback(ctx, native); err != nil {
		t.Fatal(err)
	}
	settingsValues := map[string]any{
		"packages":   []string{filepath.Join(finalRoot, "prefix/lib/node_modules/gentle-pi")},
		"npmCommand": []string{filepath.Join(finalRoot, "runtime/node/bin/node"), filepath.Join(finalRoot, "runtime/node/lib/node_modules/npm/bin/npm-cli.js"), "--prefix", filepath.Join(finalRoot, "prefix")},
	}
	if selected {
		settingsValues["extensions"] = []string{"-builtin:mcp"}
		settingsValues["compaction"] = map[string]any{"enabled": false}
	}
	settings, err := json.MarshalIndent(settingsValues, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	settings = append(settings, '\n')
	settingsPath := filepath.Join(agent, "settings.json")
	write(settingsPath, settings, 0600)
	expectedSettings := settings
	graph := filepath.Join(root, "state/global-graph.json")
	witness := read("/stock-global-graph.json")
	write(graph, witness, 0600)
	artifact, err := userMainDescriptor("43541a0d30dfafacfcb2e99ef4135ab8b1693e99", "cbc39d46b0d5416435c22498515e5bdfef8b4368e6a8e9d591f265f2de99362e")
	if err != nil {
		t.Fatal(err)
	}
	if err := userStageMainArchive(ctx, root, artifact, read("/frozen-main/main.zip")); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	// Main legitimately installs private package assets into agent. The selected
	// variant permits only its explicit package replacement; other settings,
	// prior entries and independently enumerated additions remain checked.
	protected := []string{prefix, filepath.Join(root, "runtime/main"), filepath.Join(root, "runtime/main-source"), filepath.Join(root, "runtime/project")}
	if !selected {
		protected = append(protected, settingsPath)
	}
	before := map[string]string{}
	for _, directory := range protected {
		before[directory], err = userTreeStamp(directory)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, directory := range protected {
			after, err := userTreeStamp(directory)
			if err != nil || after != before[directory] {
				t.Errorf("preflight changed protected DATA %s: %v", directory, err)
			}
		}
		verifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := privateNativeFile(verifyCtx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
			t.Error("preflight changed pinned Shell Node", err)
		}
		if selected {
			if _, err := privateNativeFile(verifyCtx, settingsPath, 0600, int64(len(expectedSettings)), fmt.Sprintf("%x", sha256.Sum256(expectedSettings))); err != nil {
				t.Error("private selected settings exact readback", err)
			}
		}
		if !t.Failed() {
			t.Logf("MAIN_SDK_OWNED_PREFLIGHT_PASS original_stock_checks=true full_main_refused=true graph_bypass_refused=true private_package_selected=%t persistent_installation=false", selected)
		}
	})
	if err := userVerifyMainSDKPreviewBindings(ctx, root, finalRoot, artifact); err != nil {
		t.Fatal("actual owned SDK composition preflight refused", err)
	}
	refused := func(err error, label string) {
		t.Helper()
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Kind != "nonzero" {
			t.Fatal("expected settled owned stock refusal", label, err)
		}
	}
	refused(userVerifyMainBindings(ctx, root, finalRoot, artifact), "full Main changed installer")
	write(graph, []byte("[]\n"), 0600)
	refused(userVerifyMainSDKPreviewBindings(ctx, root, finalRoot, artifact), "original stock graph witness")
	write(graph, witness, 0600)
	if selected {
		settingsValues["packages"] = []string{filepath.Join(finalRoot, "runtime/main-sdk")}
		expectedSettings, err = json.MarshalIndent(settingsValues, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		expectedSettings = append(expectedSettings, '\n')
	}
	if worker {
		if err := userComposeMainSDK(ctx, root, finalRoot, artifact); err != nil {
			t.Fatal("worker SDK composition/selection", err)
		}
		if err := os.Rename(root, finalRoot); err != nil {
			t.Fatal("publish worker fixture", err)
		}
		defer func() {
			if err := os.Rename(finalRoot, root); err != nil {
				t.Error("restore published fixture for parent readbacks", err)
			}
		}()
		userMainComposedSDKConsumer(t, ctx, finalRoot, artifact, selected, worker, settings, expectedSettings)
		if err := ctx.Err(); err != nil {
			t.Fatal("worker did not settle", err)
		}
		return
	}
	// Explicit composition is unpublished DATA, never a prefix replacement.
	composition := filepath.Join(root, "runtime/main-sdk")
	if _, err := os.Lstat(composition); !os.IsNotExist(err) {
		t.Fatal("preflight unexpectedly materialized a composition", err)
	}
	if err := userVerifyMainBindingsAction(ctx, root, root, artifact, "materialize-main-sdk-source"); err != nil {
		t.Fatal("owned stock composition materialization refused", err)
	}
	for _, name := range []string{"package.json", "scripts/gentle-ai-installer.mjs", "runtime/gentle-ai-binary.mjs"} {
		if string(read(filepath.Join(composition, name))) != string(read(filepath.Join(modules, "gentle-pi", name))) {
			t.Fatal("composition did not preserve exact stock binding", name)
		}
	}
	verifyComposition := func() error {
		return userVerifyMainBindingsAction(ctx, root, root, artifact, "verify-main-sdk-composition")
	}
	if err := verifyComposition(); err != nil {
		t.Fatal("complete composition readback refused", err)
	}
	refused(userVerifyMainBindingsAction(ctx, root, root, artifact, "materialize-main-sdk-source"), "occupied composition")
	extra := filepath.Join(composition, "unexpected")
	write(extra, nil, 0600)
	refused(verifyComposition(), "extra composition member")
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(composition, "scripts/gentle-ai-installer.mjs")
	stockInstaller := read(installer)
	write(installer, read(filepath.Join(root, "runtime/main-source/scripts/gentle-ai-installer.mjs")), 0600)
	refused(verifyComposition(), "changed Main installer in composition")
	write(installer, stockInstaller, 0600)
	if err := verifyComposition(); err != nil {
		t.Fatal("restored composition readback refused", err)
	}
	userMainComposedSDKConsumer(t, ctx, root, artifact, selected, worker, settings, expectedSettings)
	if err := ctx.Err(); err != nil {
		t.Fatal("owned preflight did not settle", err)
	}
}
