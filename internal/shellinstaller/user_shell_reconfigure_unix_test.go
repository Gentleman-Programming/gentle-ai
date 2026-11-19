//go:build linux && amd64

package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellRegistryRoundTrip(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	if errs := RecordUserShellInstall(home, UserShellInstall{Target: "/a", Mode: "separate", Channel: "stable"}); errs != nil {
		t.Fatal(errs)
	}
	if err := RecordUserShellInstall(home, UserShellInstall{Target: "/b", Mode: "shared", Channel: "main-preview"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordUserShellInstall(home, UserShellInstall{Target: "/a", Mode: "shared", Channel: "main-preview"}); err != nil {
		t.Fatal(err)
	}
	installs := ReadUserShellInstalls(home)
	if len(installs) != 2 {
		t.Fatalf("registry upsert lost entries: %+v", installs)
	}
	for _, install := range installs {
		if install.Target == "/a" && (install.Mode != "shared" || install.Channel != "main-preview") {
			t.Fatalf("registry did not replace the target: %+v", install)
		}
	}
	if err := os.WriteFile(userShellRegistryPath(home), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ReadUserShellInstalls(home); got != nil {
		t.Fatalf("malformed registry was accepted: %+v", got)
	}
}

func TestDiscoverUserShellInstalls(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	registered := filepath.Join(home, "registered")
	writeInstallFixture(t, registered, userManifest{Schema: userSchema, Destination: registered, Prefix: registered + "/prefix", Agent: registered + "/agent", Mode: "separate", MainSDK: &userSnapshotArtifact{URL: "x"}})
	conventional := filepath.Join(home, ".gentle-shell")
	writeInstallFixture(t, conventional, userManifest{})
	writeInstallFixture(t, conventional+".replaced-20260101T000000Z", userManifest{})
	if err := RecordUserShellInstall(home, UserShellInstall{Target: registered, Mode: "separate", Channel: "main-preview"}); err != nil {
		t.Fatal(err)
	}
	installs := DiscoverUserShellInstalls(home)
	if len(installs) != 2 {
		t.Fatalf("discovery differs: %+v", installs)
	}
	channels := map[string]string{}
	for _, install := range installs {
		channels[install.Target] = install.Channel
	}
	if channels[registered] != "main-preview" || channels[conventional] != "stable" {
		t.Fatalf("channel projection differs: %+v", channels)
	}
}

func TestLoadUserShellExperiencePrefill(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "installed")
	agent := filepath.Join(target, "agent")
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: target + "/prefix", Agent: agent, Mode: "separate", CodeGraph: true, MainSDK: &userSnapshotArtifact{URL: "x"}})
	for path, data := range map[string]string{
		"config/persona.json":              `{"mode":"neutral"}`,
		"config/background-subagents.json": `{"policy":"off"}`,
		"config/strict-tdd.json":           `{"policy":"on"}`,
		"home/.gentle-ai/state.json":       `{"rdd_mode":"off"}`,
		"agent/mcp.json":                   `{"mcpServers":{"context7":{"enabled":true}}}`,
	} {
		full := filepath.Join(target, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	profile, err := LoadUserShellExperience(UserShellInstall{Target: target, Mode: "separate", Channel: "main-preview"})
	if err != nil {
		t.Fatal(err)
	}
	want := UserExperience{Persona: "neutral", BackgroundSubagents: "off", StrictTDD: "on", GlobalRDD: "off", CodeGraph: "on", Context7: "on", GGA: "off", SDKSource: "main-preview"}
	if profile != want {
		t.Fatalf("prefill differs: got %+v want %+v", profile, want)
	}
}

func TestShellReplacementTransaction(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "installed")
	writeInstallFixture(t, target, userManifest{})
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", target)
	backup, replaced, err := userBeginShellReplacement(target)
	if err != nil || !replaced || backup == "" {
		t.Fatalf("replacement did not start: %v %v %q", replaced, err, backup)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("target was not moved aside")
	}
	if err := userRestoreShellReplacement(target, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(target, "installation.json")); err != nil {
		t.Fatalf("restore lost the install: %v", err)
	}
	// A different intent target never replaces anything.
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", filepath.Join(home, "other"))
	if _, replaced, err := userBeginShellReplacement(target); err != nil || replaced {
		t.Fatalf("mismatched intent replaced: %v %v", replaced, err)
	}
}

func TestShellReplacementRoutingByNewSelection(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "installed")
	writeInstallFixture(t, target, userManifest{})
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", target)
	stock := UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	encoded, err := stock.Encode()
	if err != nil {
		t.Fatal(err)
	}
	token, err := InspectUserInstall(UserInstallRequest{Destination: target, Mode: "separate", Experience: &stock})
	if err != nil {
		t.Fatal(err)
	}
	args := []string{target, "separate", "", "", token, "--experience", encoded}
	if _, fresh, err := userNativeFreshRequest(args); err != nil || !fresh {
		t.Fatalf("stock reconfigure did not route to the PNPM path: fresh=%v err=%v", fresh, err)
	}
	main := stock
	main.SDKSource = "main-preview"
	encoded, err = main.Encode()
	if err != nil {
		t.Fatal(err)
	}
	token, err = InspectUserInstall(UserInstallRequest{Destination: target, Mode: "separate", Experience: &main})
	if err != nil {
		t.Fatal(err)
	}
	args = []string{target, "separate", "", "", token, "--experience", encoded}
	if _, fresh, err := userNativeFreshRequest(args); err != nil || fresh {
		t.Fatalf("Main reconfigure did not route to the legacy path: fresh=%v err=%v", fresh, err)
	}
}

// After a successful publish only the newest replaced backup is retained
// (#6182), mirroring the Windows retention policy: owned, validated sibling
// trees only, symlinks never followed, uncertain trees preserved and named.
func TestPruneReplacedBackupsKeepsOnlyTheNewest(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "installed")
	writeInstallFixture(t, target, userManifest{})
	backup := func(stamp string) string { return target + ".replaced-" + stamp }
	owned := func(path string) {
		writeInstallFixture(t, path, userManifest{Schema: userSchema, Destination: target, Prefix: target + "/prefix", Agent: target + "/agent", Mode: "separate"})
		if err := os.MkdirAll(filepath.Join(path, "runtime", "node"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	keep := backup("20260301T000000Z")
	older := []string{backup("20260101T000000Z"), backup("20260201T000000Z"), backup("20980101T000000Z")}
	skewed := backup("20990101T000000Z")
	for _, path := range append([]string{keep, skewed}, older...) {
		owned(path)
	}
	foreign := backup("20250101T000000Z")
	writeInstallFixture(t, foreign, userManifest{Schema: userSchema, Destination: filepath.Join(home, "elsewhere"), Mode: "separate"})
	victim := filepath.Join(home, "victim")
	owned(victim)
	link := backup("20250201T000000Z")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	unrelated := []string{target + ".replaced-junk", filepath.Join(home, "other.replaced-20200101T000000Z")}
	for _, path := range unrelated {
		owned(path)
	}

	// A missing keep refuses before any candidate is touched.
	if err := userPruneReplacedBackups(target, backup("20260401T000000Z")); err == nil {
		t.Fatal("prune ran without its keep on disk")
	}
	if _, err := os.Lstat(older[0]); err != nil {
		t.Fatalf("missing keep still removed a candidate: %v", err)
	}

	err := userPruneReplacedBackups(target, keep)
	if err == nil || !strings.Contains(err.Error(), foreign) || !strings.Contains(err.Error(), link) {
		t.Fatalf("uncertain candidates were not reported: %v", err)
	}
	for _, path := range older {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("older backup %s survived: %v", path, statErr)
		}
	}
	for _, path := range append([]string{target, keep, skewed, foreign, link, filepath.Join(victim, "installation.json")}, unrelated...) {
		if _, statErr := os.Lstat(path); statErr != nil {
			t.Fatalf("retained path %s was removed: %v", path, statErr)
		}
	}
	// A second prune with nothing left to remove is a clean no-op apart from
	// the preserved uncertain trees.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(foreign); err != nil {
		t.Fatal(err)
	}
	if err := userPruneReplacedBackups(target, keep); err != nil {
		t.Fatalf("clean prune failed: %v", err)
	}
}
