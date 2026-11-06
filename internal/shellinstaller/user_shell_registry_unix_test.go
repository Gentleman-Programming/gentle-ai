//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// registryTestHome returns a canonical HOME whose ~/.gentle-ai already exists
// with mode, as gentle-ai's own state writers leave it (0755).
func registryTestHome(t *testing.T, mode os.FileMode) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	state := filepath.Join(home, ".gentle-ai")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, mode); err != nil {
		t.Fatal(err)
	}
	return home
}

func registryTestFile(t *testing.T, home string) (userShellRegistry, os.FileMode, bool) {
	t.Helper()
	path := userShellRegistryPath(home)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return userShellRegistry{}, 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var registry userShellRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatalf("registry is not JSON: %v\n%s", err, data)
	}
	return registry, info.Mode(), true
}

// gentle-ai creates ~/.gentle-ai 0755 (state.json, telemetry). A published
// installation must still be recorded for Manage in that directory.
func TestUserRecordPublishedShellRegistersInGentleAIStateDirectory(t *testing.T) {
	home := registryTestHome(t, 0755)
	t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
	target := filepath.Join(home, ".local", "share", "gentle-shell", "install")
	if err := userRecordPublishedShell(userManifest{Destination: target, Mode: "separate"}); err != nil {
		t.Fatalf("publication was not registered: %v", err)
	}
	registry, mode, ok := registryTestFile(t, home)
	if !ok {
		t.Fatal("published installation left no ~/.gentle-ai/shell-installs.json")
	}
	if mode != 0600 {
		t.Fatalf("registry mode = %v, want 0600", mode)
	}
	if registry.Schema != userShellRegistrySchema || len(registry.Installs) != 1 {
		t.Fatalf("registry = %+v, want one entry", registry)
	}
	if got := registry.Installs[0]; got.Target != target || got.Mode != "separate" || got.Channel != "stable" || got.UpdatedAt.IsZero() {
		t.Fatalf("registry entry = %+v", got)
	}
	if info, err := os.Lstat(filepath.Join(home, ".gentle-ai")); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("registration changed the shared state directory: %v %v", info, err)
	}
	if _, err := os.Lstat(userShellRegistryPath(home) + ".new"); !os.IsNotExist(err) {
		t.Fatalf("registration left its temporary file: %v", err)
	}
	if installs := ReadUserShellInstalls(home); len(installs) != 1 || installs[0].Target != target {
		t.Fatalf("discovery does not read the recorded entry: %+v", installs)
	}
}

func TestUserRecordPublishedShellRecordsMainPreviewChannel(t *testing.T) {
	home := registryTestHome(t, 0700)
	t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
	target := filepath.Join(home, "shell")
	if err := userRecordPublishedShell(userManifest{Destination: target, Mode: "separate", MainSDK: &userSnapshotArtifact{}}); err != nil {
		t.Fatal(err)
	}
	registry, _, ok := registryTestFile(t, home)
	if !ok || len(registry.Installs) != 1 || registry.Installs[0].Channel != "main-preview" {
		t.Fatalf("Main publication registry = %+v (present %v)", registry, ok)
	}
}

// Without the explicit seam, tests and embedders never write a registry.
func TestUserRecordPublishedShellWithoutRegistryHomeHasNoEffect(t *testing.T) {
	home := registryTestHome(t, 0755)
	t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", "")
	if err := userRecordPublishedShell(userManifest{Destination: filepath.Join(home, "shell"), Mode: "separate"}); err != nil {
		t.Fatalf("unset registry home must be inert: %v", err)
	}
	if _, _, ok := registryTestFile(t, home); ok {
		t.Fatal("registry written without GENTLE_AI_SHELL_REGISTRY_HOME")
	}
}

// Writable-by-others or aliased state directories are refused, and the
// refusal is returned instead of being dropped.
func TestUserRecordPublishedShellRefusesUnsafeStateDirectory(t *testing.T) {
	t.Run("group writable", func(t *testing.T) {
		home := registryTestHome(t, 0775)
		t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
		if err := userRecordPublishedShell(userManifest{Destination: filepath.Join(home, "shell"), Mode: "separate"}); err == nil {
			t.Fatal("group-writable ~/.gentle-ai accepted")
		}
		if _, _, ok := registryTestFile(t, home); ok {
			t.Fatal("registry written into a group-writable directory")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		home, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		elsewhere := filepath.Join(home, "elsewhere")
		if err := os.Mkdir(elsewhere, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, filepath.Join(home, ".gentle-ai")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
		if err := userRecordPublishedShell(userManifest{Destination: filepath.Join(home, "shell"), Mode: "separate"}); err == nil {
			t.Fatal("symlinked ~/.gentle-ai accepted")
		}
		if _, err := os.Lstat(filepath.Join(elsewhere, "shell-installs.json")); !os.IsNotExist(err) {
			t.Fatalf("registry followed the ~/.gentle-ai symlink: %v", err)
		}
	})
}

// The delegated unit rebuilds its environment from scratch (env -i), so the
// host's managed-install intents cross it only as revalidated entries.
func TestUserServiceIntentEnvironment(t *testing.T) {
	home := registryTestHome(t, 0755)
	target := filepath.Join(home, ".local", "share", "gentle-shell", "install")
	for _, test := range []struct {
		name                        string
		registry, reconfigure, main string
		want                        []string
	}{
		{"none", "", "", "", nil},
		{"all", home, target, "1", []string{"GENTLE_AI_SHELL_REGISTRY_HOME=" + home, "GENTLE_AI_RECONFIGURE_TARGET=" + target, "GENTLE_AI_MAIN_FALLBACK=1"}},
		{"registry home other than the unit HOME", filepath.Join(home, "other"), "", "", nil},
		{"relative reconfigure target", "", "relative/install", "", nil},
		{"uncleaned reconfigure target", "", target + "/../install", "", nil},
		{"fallback other than 1", "", "", "yes", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PATH", "/usr/bin:/bin")
			t.Setenv("GENTLE_AI_LAUNCHER_DIR_ON_PATH", "")
			t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", test.registry)
			t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", test.reconfigure)
			t.Setenv("GENTLE_AI_MAIN_FALLBACK", test.main)
			// The launcher intents have their own test.
			got := slices.DeleteFunc(userServiceIntentEnvironment(home), func(entry string) bool {
				return strings.HasPrefix(entry, "GENTLE_AI_LAUNCHER_")
			})
			if len(got) != len(test.want) {
				t.Fatalf("intents = %q, want %q", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("intents = %q, want %q", got, test.want)
				}
			}
		})
	}
}

// The launcher dir decision crosses the unit as a boolean, never as the
// caller's PATH; an already forwarded decision survives a nested unit.
func TestUserServiceIntentEnvironmentForwardsLauncherDirDecision(t *testing.T) {
	home := registryTestHome(t, 0755)
	launcherDir := filepath.Join(home, ".local", "bin")
	for _, test := range []struct {
		name, path, intent string
		want               []string
	}{
		{"terminal PATH holds the launcher dir", "/usr/local/bin:" + launcherDir, "", []string{"GENTLE_AI_LAUNCHER_DIR_ON_PATH=1"}},
		{"terminal PATH lacks the launcher dir", "/usr/local/bin", "", []string{"GENTLE_AI_LAUNCHER_PROFILE=" + filepath.Join(home, ".profile")}},
		{"nested unit keeps the forwarded decision", "/usr/bin:/bin", "1", []string{"GENTLE_AI_LAUNCHER_DIR_ON_PATH=1"}},
		{"invalid intent falls back to PATH", "/usr/bin:/bin", "yes", []string{"GENTLE_AI_LAUNCHER_PROFILE=" + filepath.Join(home, ".profile")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", "")
			t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", "")
			t.Setenv("GENTLE_AI_MAIN_FALLBACK", "")
			t.Setenv("SHELL", "")
			t.Setenv("GENTLE_AI_LAUNCHER_PROFILE", "")
			t.Setenv("PATH", test.path)
			t.Setenv("GENTLE_AI_LAUNCHER_DIR_ON_PATH", test.intent)
			got := userServiceIntentEnvironment(home)
			if strings.Join(got, "\n") != strings.Join(test.want, "\n") {
				t.Fatalf("intents = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUserRegistryNoteNamesTheRefusal(t *testing.T) {
	if note := userRegistryNote(nil); note != "" {
		t.Fatalf("successful registration produced a note: %q", note)
	}
	note := userRegistryNote(os.ErrPermission)
	if !strings.Contains(note, "not registered") || !strings.Contains(note, "shell-installs.json") || !strings.Contains(note, os.ErrPermission.Error()) {
		t.Fatalf("registration refusal note hides the cause: %q", note)
	}
	if joined := userJoinNote("Commands: gs", note); joined != "Commands: gs\n"+note {
		t.Fatalf("notes not joined line by line: %q", joined)
	}
}

// A HOME spelled through a symlink (the macOS /var -> /private/var temp root,
// or a linked home directory) still finds the install whose canonical
// manifest records the resolved destination, reported in canonical form.
func TestDiscoverUserShellInstallsThroughSymlinkedHome(t *testing.T) {
	parent := userUnixTempRoot(t)
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".local", "share", "gentle-shell", "install")
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"), Mode: "separate"})
	linked := filepath.Join(parent, "linked-home")
	if err := os.Symlink(home, linked); err != nil {
		t.Fatal(err)
	}
	for _, spelled := range []string{home, linked} {
		installs := DiscoverUserShellInstalls(spelled)
		if len(installs) != 1 || installs[0].Target != target || installs[0].Mode != "separate" || installs[0].Channel != "stable" || installs[0].Agent != filepath.Join(target, "agent") {
			t.Fatalf("HOME %q did not discover the canonical install: %+v", spelled, installs)
		}
	}
}
