//go:build linux && amd64 && shell_main_owned_preflight

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUserMainSDKExperienceWire(t *testing.T) {
	legacy := `{"persona":"neutral","background_subagents":"off"}`
	base, err := DecodeUserExperience(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if encoded, err := base.Encode(); err != nil || encoded != legacy {
		t.Fatal("legacy experience bytes changed", encoded, err)
	}
	for _, source := range []string{"stable", "main-preview"} {
		t.Run(source, func(t *testing.T) {
			raw := strings.TrimSuffix(legacy, "}") + `,"sdk_source":"` + source + `"}`
			args := []string{"/fixture/target", "separate", "", "", "physical-token", "--experience", raw}
			req, err := UserInstallFromEntry(args)
			if err != nil || req.Experience == nil {
				t.Fatal("explicit SDK source did not reach worker request", err)
			}
			encoded, err := req.Experience.Encode()
			if err != nil || encoded != raw || req.Confirmation != "physical-token" {
				t.Fatal("SDK source or original arguments changed", encoded, err)
			}
			original := req
			original.Experience = &base
			if userConfirmation(req, "physical-identity") == userConfirmation(original, "physical-identity") {
				t.Fatal("SDK source edit kept stale physical confirmation")
			}
		})
	}
	for _, key := range []string{"codegraph", "context7", "gga", "global_rdd", "strict_tdd"} {
		profile, err := DecodeUserExperience(`{"persona":"neutral","background_subagents":"off","sdk_source":"main-preview","` + key + `":"off"}`)
		if err != nil || userExperienceSupported(&profile) == nil {
			t.Fatal("unqualified Main SDK combination", key, err)
		}
	}
	for _, choice := range []string{`null`, `""`, `"main"`, `"on"`, `true`, `[]`, `"main-preview","sdk_source":"stable"`, `"main-preview","sdk_\u0073ource":"stable"`} {
		t.Run("refuse-"+choice, func(t *testing.T) {
			raw := strings.TrimSuffix(legacy, "}") + `,"sdk_source":` + choice + `}`
			if _, err := DecodeUserExperience(raw); err == nil {
				t.Fatal("malformed or duplicate explicit SDK source admitted", raw)
			}
		})
	}
}

// Consume production placement, optionally through actual private selection.
// Runtime closure and preservation follow the consumer; no persistent install.
func userMainComposedSDKConsumer(t *testing.T, ctx context.Context, root string, artifact userSnapshotArtifact, selected, worker bool, stockSettings, expectedSettings []byte) {
	t.Helper()
	if worker {
		userMainSDKWorkerConsumer(t, ctx, root, artifact, expectedSettings)
		return
	}
	t.Cleanup(func() { userMainComposedAgentReadback(t, root) })
	main := filepath.Join(root, "runtime/main-sdk")
	modules := filepath.Join(root, "prefix/lib/node_modules")
	native := filepath.Join(main, ".gentle-ai")
	dependency := filepath.Join(root, "runtime/node_modules")
	for _, path := range []string{native, dependency} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("fixture overlay destination occupied", path, err)
		}
	}
	verificationAction := "verify-main-sdk-runtime"
	defer func() {
		// Independent verification survives consumer timeout. The production
		// DATA placement is retained, never removed to hide runtime drift.
		verifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := userVerifyMainBindingsAction(verifyCtx, root, root, artifact, verificationAction); err != nil {
			t.Error("production runtime readback after SDK", err)
		}
	}()
	if err := userVerifyMainBindingsAction(ctx, root, root, artifact, "materialize-main-sdk-runtime"); err != nil {
		// Read-only diagnostic of retained partial output. Never replay a
		// failed materializer, repair drift or treat this as success proof.
		diagnosticCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(diagnosticCtx, filepath.Join(root, "runtime/node/bin/node"), filepath.Join(root, "provision.mjs"), root, filepath.Join(root, "prefix"), filepath.Join(root, "agent"), filepath.Join(root, "prefix"), root, "separate", "verify-main-sdk-runtime")
		cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, filepath.Join(root, "prefix"), filepath.Join(root, "agent"))
		output, diagnosticErr := userOwnedRun(diagnosticCtx, cmd, cancel)
		t.Log("read-only runtime failure diagnostic", string(output), diagnosticErr)
		t.Fatal("production composed runtime placement", err)
	}
	if err := userNativeReadback(ctx, native); err != nil {
		t.Fatal("composed package-local native readback", err)
	}
	linkText := "../prefix/lib/node_modules"
	if target, err := os.Readlink(dependency); err != nil || target != linkText {
		t.Fatal("rename-safe dependency binding", target, err)
	}
	refuse := func(action string) {
		t.Helper()
		err := userVerifyMainBindingsAction(ctx, root, root, artifact, action)
		if failure, ok := err.(*PrivateRuntimeError); !ok || failure.Kind != "nonzero" {
			t.Fatal("runtime refusal did not settle nonzero", action, err)
		}
	}
	refuse("materialize-main-sdk-runtime") // Never overwrite occupied runtime.
	if err := os.Remove(dependency); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", dependency); err != nil {
		t.Fatal(err)
	}
	refuse("verify-main-sdk-runtime")
	if target, err := os.Readlink(dependency); err != nil || target != "." {
		t.Fatal("verifier repaired dependency drift", target, err)
	}
	if err := os.Remove(dependency); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkText, dependency); err != nil {
		t.Fatal(err)
	}
	if selected {
		settingsPath := filepath.Join(root, "agent/settings.json")
		// Selection is a separate production write, after exact runtime proof.
		refuse("verify-main-sdk-selected")
		for _, packages := range [][]string{{filepath.Join(modules, "gentle-pi"), filepath.Join(modules, "gentle-pi")}, {main}} {
			var values map[string]any
			if err := json.Unmarshal(stockSettings, &values); err != nil {
				t.Fatal(err)
			}
			values["packages"] = packages
			data, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settingsPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			refuse("select-main-sdk-package")
			if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, data) {
				t.Fatal("selection repaired or overwrote malformed declaration", err)
			}
		}
		if err := os.WriteFile(settingsPath, stockSettings, 0600); err != nil {
			t.Fatal(err)
		}
		agent := filepath.Join(root, "agent")
		if err := os.Chmod(agent, 0770); err != nil {
			t.Fatal(err)
		}
		refuse("select-main-sdk-package")
		if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, stockSettings) {
			t.Fatal("selection changed settings in a nonprivate agent", err)
		}
		if info, err := os.Stat(agent); err != nil || info.Mode().Perm() != 0770 {
			t.Fatal("selection repaired nonprivate agent mode", err)
		}
		if err := os.Chmod(agent, 0700); err != nil {
			t.Fatal(err)
		}
		published := filepath.Join(root, "installation.json")
		if err := os.WriteFile(published, []byte("{}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		refuse("select-main-sdk-package")
		if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, stockSettings) {
			t.Fatal("selection changed published fixture settings", err)
		}
		if err := os.Remove(published); err != nil {
			t.Fatal(err)
		}
		if err := userVerifyMainBindingsAction(ctx, root, root, artifact, "select-main-sdk-package"); err != nil {
			t.Fatal("production private SDK package selection", err)
		}
		verificationAction = "verify-main-sdk-selected"
		if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, expectedSettings) {
			t.Fatal("selection changed unrelated settings or package declaration", err)
		}
		refuse("select-main-sdk-package") // No repeated publication.
		refuse("verify")                  // Ordinary stock verification is not silently widened.
		if err := os.WriteFile(settingsPath, stockSettings, 0600); err != nil {
			t.Fatal(err)
		}
		refuse(verificationAction)
		if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, stockSettings) {
			t.Fatal("selected verifier repaired the package declaration", err)
		}
		if err := os.WriteFile(settingsPath, expectedSettings, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"project", "home", "config"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	project := filepath.Join(root, "project")
	for _, name := range []string{"README.md", "AGENTS.md", "historical-openspec.md"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte("private project preimage: "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/git", "init", "--quiet", "--template=", project)
	cmd.Env = []string{"HOME=" + filepath.Join(root, "home"), "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1"}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	protected := []string{main, filepath.Join(project, ".git"), filepath.Join(project, "README.md"),
		filepath.Join(project, "AGENTS.md"), filepath.Join(project, "historical-openspec.md")}
	before := map[string]string{}
	for _, path := range protected {
		var err error
		before[path], err = userTreeStamp(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	consumerArgs := []string{"/source/e2e/shell-main-sdk-composed.mjs", root, filepath.Join(modules, "@earendil-works/pi-coding-agent")}
	if selected {
		consumerArgs = append(consumerArgs, "selected")
	}
	cmd = exec.CommandContext(ctx, filepath.Join(root, "runtime/node/bin/node"), consumerArgs...)
	cmd.Dir = project
	cmd.Env = []string{"HOME=" + filepath.Join(root, "home"), "PATH=" + filepath.Join(root, "runtime/node/bin") + ":/usr/bin:/bin",
		"PI_CODING_AGENT_DIR=" + filepath.Join(root, "agent"), "GENTLE_PI_CONFIG_HOME=" + filepath.Join(root, "config"),
		"GIT_CONFIG_NOSYSTEM=1"}
	output, runErr := cmd.CombinedOutput()
	t.Log(string(output))
	for _, path := range protected {
		after, err := userTreeStamp(path)
		if err != nil || after != before[path] {
			t.Errorf("composed SDK changed protected DATA %s: %v", path, err)
		}
	}
	if err := ctx.Err(); err != nil {
		t.Fatal("composed SDK did not settle", err)
	}
	if runErr != nil {
		t.Fatal("actual composed SDK consumer refused", runErr)
	}
	// The literal dependency binding must survive a physical whole-root
	// rename. Keep the selected settings scope: no publication or activation.
	renamed := root + "-renamed"
	if _, err := os.Lstat(renamed); !os.IsNotExist(err) {
		t.Fatal("rename fixture destination occupied", err)
	}
	if err := os.Rename(root, renamed); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Rename(renamed, root); err != nil {
			t.Error("restore renamed qualification root", err)
		}
	}()
	if target, err := filepath.EvalSymlinks(filepath.Join(renamed, "runtime/node_modules")); err != nil || target != filepath.Join(renamed, "prefix/lib/node_modules") {
		t.Fatal("physical final-rename dependency resolution", target, err)
	}
	if err := userVerifyMainBindingsAction(ctx, renamed, root, artifact, verificationAction); err != nil {
		t.Fatal("renamed runtime full readback", err)
	}
	t.Logf("MAIN_SDK_RUNTIME_RENAME_READBACK stock_native=true dependency_relative=true private_package_selected=%t persistent_installation=false", selected)
}

// Readback also runs after setup refusal or a killed consumer and fails closed
// on the first drift; it does not promise an exhaustive failed-state inventory.
// Only known partial package-owned additions may survive a failed consumer.
func userMainComposedAgentReadback(t *testing.T, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files := map[string]string{}
	for _, name := range strings.Fields("gentle-ai-explore gentle-ai-verify gentle-ai-worker jd-fix-agent jd-judge-a jd-judge-b review-readability review-reliability review-resilience review-risk") {
		files["agents/"+name+".md"] = "assets/agents/" + name + ".md"
	}
	files["chains/4r-review.chain.md"] = "assets/chains/4r-review.chain.md"
	for _, name := range []string{"strict-tdd", "strict-tdd-verify"} {
		files["gentle-ai/support/"+name+".md"] = "assets/support/" + name + ".md"
	}
	dirs := map[string]bool{}
	for _, name := range strings.Fields(". agents chains gentle-agents gentle-agents/catalog gentle-agents/presence gentle-agents/transport gentle-agents/transport/presence gentle-ai gentle-ai/support") {
		dirs[name] = true
	}
	hashes := map[string]any{}
	contents := map[string][]byte{}
	read := func(path string) ([]byte, error) {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		return userRetainedSource(ctx, path, info.Size())
	}
	for name, source := range files {
		data, err := read(filepath.Join(root, "runtime/main-sdk", source))
		if err != nil {
			t.Error(err)
			return
		}
		contents[name] = data
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	err := filepath.WalkDir(filepath.Join(root, "agent"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := filepath.Rel(filepath.Join(root, "agent"), path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if entry.IsDir() && dirs[name] && info.Mode() == os.ModeDir|0700 && info.Sys().(*syscall.Stat_t).Uid == 1000 {
			return nil
		}
		data, err := read(path)
		if err != nil {
			return err
		}
		if name == "settings.json" || (contents[name] != nil && bytes.Equal(data, contents[name])) {
			return nil // Settings preimage is independently checked by the parent.
		}
		var manifest any
		if name == "gentle-ai/managed-assets.json" && json.Unmarshal(data, &manifest) == nil && reflect.DeepEqual(manifest, map[string]any{"schemaVersion": float64(1), "assets": hashes}) {
			return nil
		}
		return fmt.Errorf("unexpected or changed composed SDK agent entry %s", name)
	})
	if err != nil {
		t.Error("failure-safe agent readback", err)
	}
}

func userMainSDKWorkerConsumer(t *testing.T, ctx context.Context, root string, artifact userSnapshotArtifact, expectedSettings []byte) {
	t.Helper()
	defer userMainComposedAgentReadback(t, root)
	manifest := userManifest{Mode: "separate", Destination: root, Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent"), MainSDK: &artifact}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil {
		t.Fatal("persisted SDK selection metadata read", err)
	}
	var retained userManifest
	if err := json.Unmarshal(data, &retained); err != nil || retained.MainSDK == nil || *retained.MainSDK != artifact {
		t.Fatal("retained SDK selection metadata", err)
	}
	defer func() {
		verifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := userVerifyInstalledGlobal(verifyCtx, root, manifest); err != nil {
			t.Error("independent selected runtime readback", err)
		}
	}()
	if err := userVerifyInstalledGlobal(ctx, root, retained); err != nil {
		t.Fatal("published selected SDK readback", err)
	}
	retained.Mode = "shared"
	if err := userVerifyInstalledGlobal(ctx, root, retained); err == nil {
		t.Fatal("Main SDK Shared metadata admitted")
	}
	retained.Mode = "separate"
	retained.MainSDK.Bound++
	if err := userVerifyInstalledGlobal(ctx, root, retained); err == nil {
		t.Fatal("changed retained SDK descriptor admitted")
	}
	for _, name := range []string{"project", "home", "config"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	main := filepath.Join(root, "runtime/main-sdk")
	before, err := userTreeStamp(main)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/bin/node"), "/source/e2e/shell-main-sdk-composed.mjs", root, filepath.Join(root, "prefix/lib/node_modules/@earendil-works/pi-coding-agent"), "worker")
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, manifest.Prefix, manifest.Agent)
	output, runErr := cmd.CombinedOutput()
	t.Log(string(output))
	if runErr != nil || ctx.Err() != nil {
		t.Fatal("published fixture file-backed SDK consumer", runErr, ctx.Err())
	}
	if after, err := userTreeStamp(main); err != nil || after != before {
		t.Fatal("published SDK changed composition", err)
	}
	if after, err := os.ReadFile(filepath.Join(root, "agent/settings.json")); err != nil || !bytes.Equal(after, expectedSettings) {
		t.Fatal("worker or installed verifier changed unrelated settings", err)
	}
	t.Log("MAIN_SDK_WORKER_READBACK final_paths=true file_backed=true manifest_dispatch=true installed_acceptance=false")
}
