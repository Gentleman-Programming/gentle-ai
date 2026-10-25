//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// userUnixOverlaySeams stubs the overlay's verifier, acquisition and
// provisioner actions. The stubs record their calls; the fixture tree never
// executes Node and carries no real supplier archives. The acquisition stub
// materializes the runtime entries the real staging writes so the committed
// cleanup and the rollback have physical state to manage.
func userUnixOverlaySeams(t *testing.T, artifact *userSnapshotArtifact, verifies *int, actions *[]string, snapshotCalls *int) {
	t.Helper()
	previousVerify, previousAcquire, previousCompose := userUnixOverlayVerify, userUnixOverlayAcquire, userUnixOverlayCompose
	userUnixOverlayVerify = func(ctx context.Context, root string, manifest userManifest) error {
		*verifies++
		return nil
	}
	userUnixOverlayAcquire = func(ctx context.Context, root string) (userSnapshotArtifact, error) {
		*snapshotCalls++
		if artifact == nil {
			return userSnapshotArtifact{}, errors.New("no Main snapshot expected")
		}
		for _, relative := range []string{"runtime/main/main.json", "runtime/main/main.zip", "runtime/main-source/package.json", "runtime/main-sdk/package.json"} {
			name := filepath.Join(root, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte("main fixture\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return *artifact, nil
	}
	userUnixOverlayCompose = func(ctx context.Context, root string, manifest userManifest, artifact userSnapshotArtifact) error {
		*actions = append(*actions, "compose")
		return nil
	}
	t.Cleanup(func() {
		userUnixOverlayVerify, userUnixOverlayAcquire, userUnixOverlayCompose = previousVerify, previousAcquire, previousCompose
	})
}

func userUnixOverlayRequest(root, source string) UserInstallRequest {
	return UserInstallRequest{Destination: root, Mode: "separate", Experience: &UserExperience{Persona: "gentleman", BackgroundSubagents: "on", SDKSource: source}}
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

// The stable -> Main overlay composes the fresh Main snapshot, flips the SDK
// declaration in place and rebinds the manifest; every other component,
// preference file and binding stays byte-identical.
func TestUserUnixChannelOverlayStableToMain(t *testing.T) {
	root, _ := userUnixOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	verifies, snapshotCalls := 0, 0
	var actions []string
	userUnixOverlaySeams(t, &artifact, &verifies, &actions, &snapshotCalls)

	result, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, "main-preview"))
	if err != nil || !applied {
		t.Fatalf("stable -> Main overlay failed: applied=%v err=%v", applied, err)
	}
	if result.State != "channel-overlay" || result.Destination != root || result.Prefix != filepath.Join(root, "prefix") {
		t.Fatalf("overlay result differs: %+v", result)
	}
	if verifies != 2 || snapshotCalls != 1 || len(actions) != 1 {
		t.Fatalf("overlay seam sequence differs: verifies=%d snapshotCalls=%d actions=%v", verifies, snapshotCalls, actions)
	}
	packages := userUnixOverlayPackages(t, root)
	if len(packages) != 1 || packages[0] != filepath.Join(root, "runtime/main-sdk") {
		t.Fatalf("SDK declaration was not flipped to Main: %v", packages)
	}
	// Authority rebind: the manifest names the fresh Main snapshot.
	data, err := os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rebound userManifest
	if err := json.Unmarshal(data, &rebound); err != nil {
		t.Fatal(err)
	}
	if rebound.MainSDK == nil || rebound.MainSDK.Prefix != artifact.Prefix || rebound.MainSDK.SHA != artifact.SHA {
		t.Fatalf("manifest was not rebound to Main: %+v", rebound.MainSDK)
	}
	if _, err := os.Lstat(filepath.Join(root, "installation.json.overlay-new")); !os.IsNotExist(err) {
		t.Fatalf("manifest stage file survived the rebind: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "agent", ".gentle-shell-settings-overlay")); !os.IsNotExist(err) {
		t.Fatalf("settings stage file survived the flip: %v", err)
	}
	// Every other component stays byte-identical.
	if got, err := os.ReadFile(filepath.Join(root, "runtime", "node", "bin", "node")); err != nil || string(got) != "fixture Node, never executed\n" {
		t.Fatalf("Node runtime was touched: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "prefix/lib/node_modules/gentle-pi/package.json")); err != nil || string(got) != "stock package\n" {
		t.Fatalf("stock package was touched: %q %v", got, err)
	}
	settings := userUnixOverlaySettings(t, root)
	if _, ok := settings["npmCommand"]; !ok {
		t.Fatal("npmCommand declaration was lost")
	}
	if _, ok := settings["packages"]; !ok {
		t.Fatal("packages declaration was lost")
	}
}

// The Main -> Stable overlay flips the declaration back to the stock package
// root, clears the manifest authority and removes the Main composition only
// after the rebound tree verifies.
func TestUserUnixChannelOverlayMainToStable(t *testing.T) {
	root, manifest := userUnixOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	// The installation is Main: the manifest names the snapshot and the
	// settings declare the composed package.
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	manifest.MainSDK = &artifact
	rebound, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "installation.json"), rebound, 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(root, "agent", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["packages"] = []string{filepath.Join(root, "runtime/main-sdk")}
	selected, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, append(selected, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"runtime/main/main.json", "runtime/main/main.zip", "runtime/main-source/package.json", "runtime/main-sdk/package.json"} {
		name := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("main fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	verifies, snapshotCalls := 0, 0
	var actions []string
	userUnixOverlaySeams(t, nil, &verifies, &actions, &snapshotCalls)

	result, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, "stable"))
	if err != nil || !applied {
		t.Fatalf("Main -> Stable overlay failed: applied=%v err=%v", applied, err)
	}
	if result.State != "channel-overlay" {
		t.Fatalf("overlay result differs: %+v", result)
	}
	if verifies != 2 || snapshotCalls != 0 || len(actions) != 0 {
		t.Fatalf("overlay seam sequence differs: verifies=%d snapshotCalls=%d actions=%v", verifies, snapshotCalls, actions)
	}
	packages := userUnixOverlayPackages(t, root)
	if len(packages) != 1 || packages[0] != filepath.Join(root, "prefix/lib/node_modules/gentle-pi") {
		t.Fatalf("SDK declaration was not restored to stock: %v", packages)
	}
	data, err = os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var restored userManifest
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.MainSDK != nil {
		t.Fatalf("manifest was not restored to stock: %+v", restored.MainSDK)
	}
	// The Main composition leaves only after the restored tree verifies.
	for _, relative := range []string{"runtime/main", "runtime/main-source", "runtime/main-sdk"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("Main composition survived the committed cleanup: %s %v", relative, err)
		}
	}
}

// A same-channel reconfigure, an external target, a non-Separate mode, a
// foreign platform and a fresh destination never route to the overlay; the
// full retire-and-reinstall keeps handling them.
func TestUserUnixChannelOverlayNotApplied(t *testing.T) {
	root, _ := userUnixOverlayFixture(t)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	verifies, snapshotCalls := 0, 0
	var actions []string
	userUnixOverlaySeams(t, &artifact, &verifies, &actions, &snapshotCalls)

	// Same channel.
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	if _, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, "stable")); err != nil || applied {
		t.Fatalf("same-channel reconfigure routed to the overlay: applied=%v err=%v", applied, err)
	}
	// No reconfigure target.
	os.Unsetenv("GENTLE_AI_RECONFIGURE_TARGET")
	if _, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, "main-preview")); err != nil || applied {
		t.Fatalf("external reconfigure routed to the overlay: applied=%v err=%v", applied, err)
	}
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	// Non-Separate mode.
	shared, sharedManifest := userUnixOverlayFixture(t)
	sharedManifest.Mode = "agent"
	sharedManifest.Agent = filepath.Join(shared, "agent")
	encoded, err := json.Marshal(sharedManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "installation.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(shared, "main-preview")); err != nil || applied {
		t.Fatalf("agent mode routed to the overlay: applied=%v err=%v", applied, err)
	}
	if verifies != 0 || snapshotCalls != 0 || len(actions) != 0 {
		t.Fatalf("not-applied paths still called the seams: verifies=%d snapshotCalls=%d actions=%v", verifies, snapshotCalls, actions)
	}
}

// Any doubt unwinds the whole overlay: a rebound verification failure
// restores both the settings declaration and the manifest bytes and leaves
// the previous valid state in place.
func TestUserUnixChannelOverlayRollback(t *testing.T) {
	root, _ := userUnixOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	verifies, snapshotCalls := 0, 0
	var actions []string
	userUnixOverlaySeams(t, &artifact, &verifies, &actions, &snapshotCalls)
	previousVerify := userUnixOverlayVerify
	defer func() { userUnixOverlayVerify = previousVerify }()
	calls := 0
	userUnixOverlayVerify = func(ctx context.Context, root string, manifest userManifest) error {
		calls++
		if calls == 1 {
			return nil // The installed-tree gate passes.
		}
		return errors.New("rebound verification refused")
	}

	_, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, "main-preview"))
	if err == nil || !applied {
		t.Fatalf("rollback expected an applied failure: applied=%v err=%v", applied, err)
	}
	if !strings.Contains(err.Error(), "rebound verification refused") {
		t.Fatalf("rollback lost the verification cause: %v", err)
	}
	packages := userUnixOverlayPackages(t, root)
	if len(packages) != 1 || packages[0] != filepath.Join(root, "prefix/lib/node_modules/gentle-pi") {
		t.Fatalf("settings declaration was not restored: %v", packages)
	}
	data, err := os.ReadFile(filepath.Join(root, "installation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var restored userManifest
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.MainSDK != nil {
		t.Fatalf("manifest was not restored: %+v", restored.MainSDK)
	}
	// The acquired Main state is rolled back with the authority: the staging
	// entries are removed as part of the unwind so the tree is exactly as it
	// was before the overlay started.
	for _, relative := range []string{"runtime/main", "runtime/main-source", "runtime/main-sdk"} {
		if _, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); statErr == nil {
			t.Fatalf("Main staging survived the rollback: %s", relative)
		}
	}
}

// An occupied Main composition path refuses before a single byte moves.
func TestUserUnixChannelOverlayRefusesOccupiedComposition(t *testing.T) {
	root, _ := userUnixOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	verifies, snapshotCalls := 0, 0
	var actions []string
	userUnixOverlaySeams(t, &artifact, &verifies, &actions, &snapshotCalls)
	if err := os.MkdirAll(filepath.Join(root, "runtime", "main-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, "main-preview"))
	if err == nil || !applied {
		t.Fatalf("occupied composition must refuse: applied=%v err=%v", applied, err)
	}
	if !strings.Contains(err.Error(), "already occupied") {
		t.Fatalf("refusal lost the occupied path: %v", err)
	}
	if snapshotCalls != 0 || len(actions) != 0 {
		t.Fatalf("refusal ran acquisition or composition: snapshotCalls=%d actions=%v", snapshotCalls, actions)
	}
	if packages := userUnixOverlayPackages(t, root); len(packages) != 1 || packages[0] != filepath.Join(root, "prefix/lib/node_modules/gentle-pi") {
		t.Fatalf("refusal changed the settings declaration: %v", packages)
	}
}
