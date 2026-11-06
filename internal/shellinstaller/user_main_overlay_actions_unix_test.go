//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// userUnixOverlayFixture extends the canonical install fixture with the
// published Separate layout the overlay reads: the stock gentle-pi package,
// the agent settings declaration and the retained runtime entries the
// verifier seam stands in for.
func userUnixOverlayFixture(t *testing.T) (string, userManifest) {
	t.Helper()
	root := t.TempDir()
	manifest := userManifest{Schema: userSchema, Destination: root, Prefix: filepath.Join(root, "prefix"), Agent: filepath.Join(root, "agent"), Mode: "separate"}
	writeInstallFixture(t, root, manifest)
	file := func(relative, body string, mode os.FileMode) {
		t.Helper()
		name := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	file("prefix/lib/node_modules/gentle-pi/package.json", "stock package\n", 0o600)
	file("prefix/lib/node_modules/gentle-pi/bin/gentle-shell.mjs", "stock shell entry\n", 0o600)
	file("runtime/node/bin/node", "fixture Node, never executed\n", 0o700)
	file("runtime/archives/fd", "fixture fd, never executed\n", 0o700)
	settings := map[string]any{
		"packages": []string{filepath.Join(root, "prefix/lib/node_modules/gentle-pi")},
		"npmCommand": []string{
			filepath.Join(root, "runtime/node/bin/node"),
			filepath.Join(root, "runtime/node/lib/node_modules/npm/bin/npm-cli.js"),
			"--prefix", filepath.Join(root, "prefix"),
		},
	}
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file("agent/settings.json", string(encoded)+"\n", 0o600)
	if err := os.Mkdir(filepath.Join(root, "home"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root, manifest
}

// userUnixOverlayMainArtifact builds a frozen Main descriptor fixture.
func userUnixOverlayMainArtifact(t *testing.T, commit string) userSnapshotArtifact {
	t.Helper()
	artifact, err := userMainDescriptor(commit, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func userUnixOverlaySettings(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "agent", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func userUnixOverlayPackages(t *testing.T, root string) []string {
	t.Helper()
	packages, ok := userUnixOverlaySettings(t, root)["packages"].([]any)
	if !ok {
		t.Fatalf("settings packages are not an array: %+v", userUnixOverlaySettings(t, root))
	}
	result := make([]string, 0, len(packages))
	for _, entry := range packages {
		text, ok := entry.(string)
		if !ok {
			t.Fatalf("settings package entry is not a plain string: %+v", entry)
		}
		result = append(result, text)
	}
	return result
}

// userMainProvisionContract is the action vocabulary declared by the owned
// provisioner, parsed from the script itself so the Go callers cannot drift
// from it: accepted actions, Main actions and the runtime-aware subset that
// admits the materialized native member and dependency link.
type userMainProvisionContract struct {
	accepted, main, runtime []string
}

func userMainProvisionActions(t *testing.T) userMainProvisionContract {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "provision-gentle-shell-private-global.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile(`'([a-z-]+)'`)
	list := func(pattern string) []string {
		t.Helper()
		match := regexp.MustCompile(pattern).FindSubmatch(data)
		if match == nil {
			t.Fatalf("provision script no longer declares %s", pattern)
		}
		var names []string
		for _, name := range quoted.FindAllSubmatch(match[1], -1) {
			names = append(names, string(name[1]))
		}
		return names
	}
	runtime := list(`const mainRuntimeActions = \[([^\]]*)\];`)
	main := append(list(`const mainActions = \[([^\]]*)\.\.\.mainRuntimeActions\];`), runtime...)
	accepted := append(list(`!\[([^\]]*)\.\.\.mainActions\]\.includes\(requestedAction\)`), main...)
	if len(runtime) == 0 || len(main) <= len(runtime) || len(accepted) <= len(main) {
		t.Fatalf("provision action vocabulary parsed incompletely: runtime=%v main=%v accepted=%v", runtime, main, accepted)
	}
	return userMainProvisionContract{accepted: accepted, main: main, runtime: runtime}
}

// userMainProvisionEmulator stands in for the owned provisioner's Main
// actions with their real filesystem effects and refusals, keyed by the
// action vocabulary parsed from the script: the source composition creates
// runtime/main-sdk, the runtime materialization adds the stock native member
// and the runtime/node_modules dependency link, a non-runtime composition
// check refuses that native member as extra, selection refuses a published
// tree, and the selected verification requires the Main declaration plus the
// dependency link. Node never runs.
func userMainProvisionEmulator(t *testing.T, contract userMainProvisionContract, calls *[]string) func(context.Context, string, string, string, string, userSnapshotArtifact, string) error {
	t.Helper()
	return func(ctx context.Context, root, finalRoot, prefix, agent string, artifact userSnapshotArtifact, action string) error {
		*calls = append(*calls, action)
		refuse := func(message string) error { return fmt.Errorf("global provision refused: %s", message) }
		occupied := func(path string) bool {
			_, err := os.Lstat(path)
			return err == nil
		}
		composed := filepath.Join(root, "runtime/main-sdk")
		native := filepath.Join(composed, ".gentle-ai")
		dependency := filepath.Join(root, "runtime/node_modules")
		if !slices.Contains(contract.accepted, action) {
			return refuse("arguments or UID")
		}
		if !slices.Contains(contract.main, action) {
			return refuse("emulator covers only Main actions")
		}
		runtime := slices.Contains(contract.runtime, action)
		composing := action != "verify-main-source" && action != "verify-main-sdk-source"
		if composing && !runtime && occupied(native) {
			return refuse("Main composition extra member")
		}
		switch action {
		case "materialize-main-sdk-source":
			if occupied(composed) {
				return refuse("occupied path")
			}
			if err := os.Mkdir(composed, 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(composed, "package.json"), []byte("composed Main\n"), 0o600)
		case "materialize-main-sdk-runtime":
			if !occupied(composed) {
				return refuse("Main composition directory differs")
			}
			if occupied(native) || occupied(dependency) {
				return refuse("occupied path")
			}
			if err := os.Mkdir(native, 0o700); err != nil {
				return err
			}
			return os.Symlink("../prefix/lib/node_modules", dependency)
		case "select-main-sdk-package":
			if occupied(filepath.Join(root, "installation.json")) {
				return refuse("occupied path")
			}
			settings := filepath.Join(agent, "settings.json")
			data, err := os.ReadFile(settings)
			if err != nil {
				return err
			}
			selected := strings.ReplaceAll(string(data), filepath.Join(prefix, "lib/node_modules/gentle-pi"), filepath.Join(finalRoot, "runtime/main-sdk"))
			if err := os.WriteFile(settings, []byte(selected), 0o600); err != nil {
				return err
			}
		case "verify-main-sdk-selected":
			data, err := os.ReadFile(filepath.Join(agent, "settings.json"))
			if err != nil {
				return err
			}
			mainRoot, stockRoot := filepath.Join(finalRoot, "runtime/main-sdk"), filepath.Join(prefix, "lib/node_modules/gentle-pi")
			if !strings.Contains(string(data), mainRoot) || strings.Contains(string(data), stockRoot) {
				return refuse("Main SDK selected declaration differs")
			}
		}
		if runtime && (!occupied(native) || !occupied(dependency)) {
			return refuse("Main SDK dependency binding differs")
		}
		return nil
	}
}

// userUnixOverlayStubAcquire stages the files the real acquisition writes
// (runtime/main and runtime/main-source); runtime/main-sdk is left to the
// composition actions exactly like production.
func userUnixOverlayStubAcquire(t *testing.T, artifact userSnapshotArtifact) {
	t.Helper()
	previous := userUnixOverlayAcquire
	userUnixOverlayAcquire = func(ctx context.Context, root string) (userSnapshotArtifact, error) {
		for _, relative := range []string{"runtime/main/main.json", "runtime/main/main.zip", "runtime/main-source/package.json"} {
			name := filepath.Join(root, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
				return userSnapshotArtifact{}, err
			}
			if err := os.WriteFile(name, []byte("main fixture\n"), 0o600); err != nil {
				return userSnapshotArtifact{}, err
			}
		}
		return artifact, nil
	}
	t.Cleanup(func() { userUnixOverlayAcquire = previous })
}

func userUnixOverlayRuntimeEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// The overlay composes the published tree with exactly the actions the fresh
// Main install uses before its selection; the provisioner must accept each
// one for the runtime-materialized tree (no "Main composition extra member").
func TestUserUnixOverlayComposeMatchesFreshMainActions(t *testing.T) {
	contract := userMainProvisionActions(t)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	previous := userMainSDKAction
	t.Cleanup(func() { userMainSDKAction = previous })

	// Fresh install: an unpublished stage root, selection by the provisioner.
	stage, stageManifest := userUnixOverlayFixture(t)
	if err := os.Remove(filepath.Join(stage, "installation.json")); err != nil {
		t.Fatal(err)
	}
	var fresh []string
	userMainSDKAction = userMainProvisionEmulator(t, contract, &fresh)
	if err := userComposeMainSDK(context.Background(), stage, stage, stageManifest.Prefix, stageManifest.Agent, artifact); err != nil {
		t.Fatalf("fresh Main composition refused: %v (actions %v)", err, fresh)
	}
	if fresh[len(fresh)-2] != "select-main-sdk-package" || fresh[len(fresh)-1] != "verify-main-sdk-selected" {
		t.Fatalf("fresh Main composition no longer ends with provisioner selection: %v", fresh)
	}

	// Overlay: the published tree; Go flips the declaration afterwards.
	root, manifest := userUnixOverlayFixture(t)
	var overlay []string
	userMainSDKAction = userMainProvisionEmulator(t, contract, &overlay)
	if err := userUnixOverlayCompose(context.Background(), root, manifest, artifact); err != nil {
		t.Fatalf("overlay Main composition refused: %v (actions %v)", err, overlay)
	}
	if want := fresh[:len(fresh)-2]; !slices.Equal(overlay, want) {
		t.Fatalf("overlay composition actions diverge from the fresh Main install: overlay=%v fresh=%v", overlay, want)
	}
	for _, action := range append(slices.Clone(overlay), fresh...) {
		if !slices.Contains(contract.accepted, action) {
			t.Fatalf("Go requests %q, which the provisioner does not accept", action)
		}
	}
}

// Any stable -> Main failure after the runtime materialization unwinds every
// overlay path, including the runtime/node_modules dependency link, so the
// next attempt is not refused with "occupied path".
func TestUserUnixOverlayStableToMainUnwindsCompletely(t *testing.T) {
	contract := userMainProvisionActions(t)
	for _, name := range []string{"selection refused", "manifest rebind refused"} {
		t.Run(name, func(t *testing.T) {
			root, manifest := userUnixOverlayFixture(t)
			artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
			userUnixOverlayStubAcquire(t, artifact)
			before := userUnixOverlayRuntimeEntries(t, root)
			settingsBefore, err := os.ReadFile(filepath.Join(root, "agent", "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			manifestBefore, err := os.ReadFile(filepath.Join(root, "installation.json"))
			if err != nil {
				t.Fatal(err)
			}
			previous := userMainSDKAction
			t.Cleanup(func() { userMainSDKAction = previous })
			var calls []string
			emulate := userMainProvisionEmulator(t, contract, &calls)
			// Inject the failure right after the dependency link exists.
			userMainSDKAction = func(ctx context.Context, root, finalRoot, prefix, agent string, artifact userSnapshotArtifact, action string) error {
				if err := emulate(ctx, root, finalRoot, prefix, agent, artifact, action); err != nil || action != "materialize-main-sdk-runtime" {
					return err
				}
				target := filepath.Join(root, "agent", "settings.json")
				if name == "manifest rebind refused" {
					target = filepath.Join(root, "installation.json")
				}
				return os.Chmod(target, 0o640)
			}

			rollback := &userUnixOverlayRollback{}
			_, err = userUnixOverlayStableToMain(context.Background(), root, manifest, rollback)
			if err == nil {
				t.Fatalf("injected failure was not reported (actions %v)", calls)
			}
			if !strings.Contains(err.Error(), "custody differs") {
				t.Fatalf("overlay failed before the injected step: %v (actions %v)", err, calls)
			}
			if cause := rollback.unwind(); cause != nil {
				t.Fatalf("rollback incomplete: %v", cause)
			}
			for _, relative := range []string{"runtime/main", "runtime/main-source", "runtime/main-sdk", "runtime/node_modules"} {
				if _, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("overlay residue survived the unwind: %s (%v)", relative, statErr)
				}
			}
			if after := userUnixOverlayRuntimeEntries(t, root); !slices.Equal(after, before) {
				t.Fatalf("runtime entries differ after the unwind: before=%v after=%v", before, after)
			}
			if got, _ := os.ReadFile(filepath.Join(root, "agent", "settings.json")); string(got) != string(settingsBefore) {
				t.Fatalf("settings bytes changed after the unwind:\n%s", got)
			}
			if got, _ := os.ReadFile(filepath.Join(root, "installation.json")); string(got) != string(manifestBefore) {
				t.Fatalf("manifest bytes changed after the unwind:\n%s", got)
			}
		})
	}
}
