//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// writeChannelManifest installs the canonical manifest needed by Load, Apply,
// Discover and Check; everything else stays a preference fixture.
func writeChannelManifest(t *testing.T, target string, mainSDK *userSnapshotArtifact) {
	t.Helper()
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"), Mode: "separate", MainSDK: mainSDK}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeChannelHome(t *testing.T) (home, target string) {
	t.Helper()
	home = filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	home = canonical
	target = filepath.Join(home, ".local", "share", "gentle-shell", "install")
	writeChannelManifest(t, target, nil)
	return home, target
}

func writeChannelPreference(t *testing.T, target, channel string) {
	t.Helper()
	path := userUpdateChannelPath(target)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, userUpdateChannelBytes(channel), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUserShellChannelPreferencePersistence(t *testing.T) {
	home, target := writeChannelHome(t)
	install := UserShellInstall{Target: target, Mode: "separate"}
	selected := UserExperience{Persona: "gentleman", BackgroundSubagents: "on", SDKSource: "main-preview"}
	if err := ApplyUserExperiencePreferences(install, selected); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(userUpdateChannelPath(target))
	if err != nil || string(data) != `{"channel":"main"}` {
		t.Fatalf("canonical Main bytes differ: %q %v", data, err)
	}
	if channel, ok := userReadUpdateChannel(target); !ok || channel != userChannelMain {
		t.Fatalf("Main preference readback differs: %q %v", channel, ok)
	}
	loaded, err := LoadUserShellExperience(install)
	if err != nil || loaded.SDKSource != "main-preview" {
		t.Fatalf("Load did not follow the Main preference: %+v %v", loaded, err)
	}
	installs := DiscoverUserShellInstalls(home)
	if len(installs) != 1 || installs[0].Channel != "main-preview" {
		t.Fatalf("Discover did not follow the Main preference: %+v", installs)
	}

	selected.SDKSource = "stable"
	if err := ApplyUserExperiencePreferences(install, selected); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(userUpdateChannelPath(target))
	if err != nil || string(data) != `{"channel":"stable"}` {
		t.Fatalf("canonical Stable bytes differ: %q %v", data, err)
	}
	if loaded, err = LoadUserShellExperience(install); err != nil || loaded.SDKSource != "stable" {
		t.Fatalf("Load did not follow the Stable preference: %+v %v", loaded, err)
	}
	if installs = DiscoverUserShellInstalls(home); len(installs) != 1 || installs[0].Channel != "stable" {
		t.Fatalf("Discover did not follow the Stable preference: %+v", installs)
	}
	if _, err := os.Stat(filepath.Join(target, "config", "persona.json")); err != nil {
		t.Fatalf("channel apply lost the other preference writes: %v", err)
	}
}

func TestUserShellChannelPreferenceRejectsUnknownBytes(t *testing.T) {
	_, target := writeChannelHome(t)
	writeChannelPreference(t, target, "canary")
	if channel, ok := userReadUpdateChannel(target); ok || channel != "" {
		t.Fatalf("non-canonical channel accepted: %q %v", channel, ok)
	}
	install := UserShellInstall{Target: target, Mode: "separate"}
	loaded, err := LoadUserShellExperience(install)
	if err != nil || loaded.SDKSource != "stable" {
		t.Fatalf("non-canonical preference must inherit the installed channel: %+v %v", loaded, err)
	}
}

func TestCheckUserShellUpdateFollowsPersistedChannel(t *testing.T) {
	ctx := context.Background()
	// The supervisor provenance guard refuses group-writable images and
	// ancestors; some environments build test trees under a 0002 umask. Tighten
	// the directories this test owns so the guard sees a production-shaped
	// ancestor chain, and leave everything else untouched.
	if self, err := os.Executable(); err != nil {
		t.Fatal(err)
	} else {
		_ = os.Chmod(self, 0o755)
		for current := filepath.Dir(self); current != "/" && current != userTrustedTmp; current = filepath.Dir(current) {
			info, err := os.Lstat(current)
			if err != nil || info.Mode().Perm()&0o022 == 0 || !info.IsDir() {
				continue // Absent, already strict enough, or not a directory.
			}
			if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Uid != uint32(os.Getuid()) {
				continue // Not ours to tighten.
			}
			_ = os.Chmod(current, 0o755)
		}
	}
	// A Stable installation routed to Main by the persisted preference checks
	// the Main snapshot; with no Main SDK installed the update is immediate.
	home, target := writeChannelHome(t)
	writeChannelPreference(t, target, userChannelMain)
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "main-preview" || !update.Available || len(update.Components) != 1 || update.Components[0] != "SDK Main snapshot" || !strings.Contains(update.Detail, "not installed") {
		t.Fatalf("Main routing differs: %+v", update)
	}

	// A Main installation routed to Stable by the persisted preference checks
	// the stable components against this build.
	mainTarget := filepath.Join(home, ".local", "share", "gentle-shell", "main")
	writeChannelManifest(t, mainTarget, &userSnapshotArtifact{SHA: strings.Repeat("a", 64)})
	writeChannelPreference(t, mainTarget, userChannelStable)
	update, err = CheckUserShellUpdate(ctx, UserShellInstall{Target: mainTarget, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "stable" || !update.Available || len(update.Components) == 0 {
		t.Fatalf("Stable routing differs: %+v", update)
	}

	// Without a persisted preference the installed composition routes the
	// check: a Stable install stays on the stable channel.
	_, stockTarget := writeChannelHome(t)
	update, err = CheckUserShellUpdate(ctx, UserShellInstall{Target: stockTarget, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Channel != "stable" || !update.Available {
		t.Fatalf("stock routing differs: %+v", update)
	}
}
