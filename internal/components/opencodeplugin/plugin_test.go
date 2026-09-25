package opencodeplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestActiveDefinitionsExcludeRetiredSDDPluginButPreserveLegacyLookup(t *testing.T) {
	for _, def := range Definitions() {
		if def.ID == model.OpenCodePluginSDDEngramManage {
			t.Fatal("retired SDD plugin is still offered for installation")
		}
	}
	legacy, ok := DefinitionFor(model.OpenCodePluginSDDEngramManage)
	if !ok || legacy.PackageName != "opencode-sdd-engram-manage" {
		t.Fatalf("legacy uninstall lookup = (%+v, %v), want owned package", legacy, ok)
	}
}

func TestRetiredSDDPluginCanStillBeUninstalledWithoutRemovingOtherRegistrations(t *testing.T) {
	home := t.TempDir()
	writeTUIConfig(t, home, []string{"opencode-sdd-engram-manage", "user-plugin"})

	result, err := Uninstall(home, model.OpenCodePluginSDDEngramManage)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if !result.ChangedTUI {
		t.Fatal("legacy registration was not removed")
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Plugin) != 1 || config.Plugin[0] != "user-plugin" {
		t.Fatalf("remaining registrations = %v, want only user-plugin", config.Plugin)
	}
}

func TestInstallAddsCommunityPluginToTUIConfig(t *testing.T) {
	home := t.TempDir()

	result, err := Install(home, model.OpenCodePluginSubAgentStatusline)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Install() changed = false, want true")
	}

	configPath := filepath.Join(home, ".config", "opencode", "tui.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}

	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(tui.json) error = %v", err)
	}
	if len(root.Plugin) != 1 || root.Plugin[0] != "opencode-subagent-statusline" {
		t.Fatalf("plugin list = %#v, want opencode-subagent-statusline", root.Plugin)
	}
}

func TestInstallPreservesExistingTUIPluginsAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initial := []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["existing-plugin"]}`)
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), initial, 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Install(home, model.OpenCodePluginSDDEngramManage)
	if err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	second, err := Install(home, model.OpenCodePluginSDDEngramManage)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Install() changed = false, want true")
	}
	if second.Changed {
		t.Fatal("second Install() changed = true, want false")
	}

	data, err := os.ReadFile(filepath.Join(configDir, "tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	want := []string{"existing-plugin", "opencode-sdd-engram-manage"}
	if len(root.Plugin) != len(want) {
		t.Fatalf("plugin list = %#v, want %#v", root.Plugin, want)
	}
	for i := range want {
		if root.Plugin[i] != want[i] {
			t.Fatalf("plugin list = %#v, want %#v", root.Plugin, want)
		}
	}
}

func TestInstallGentleLogoRollsBackSourceWhenRegistrationFails(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	malformed := []byte("{ not json")
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), malformed, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err == nil {
		t.Fatal("Install() error = nil, want registration failure")
	}

	pluginPath := filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx")
	if _, err := os.Stat(pluginPath); !os.IsNotExist(err) {
		t.Fatalf("plugin source should not exist after failed registration; stat err = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(configDir, "tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(malformed) {
		t.Fatalf("tui.json = %q, want unchanged %q", data, malformed)
	}
}

func TestInstallGentleLogoRestoresPreExistingSourceWhenRegistrationFails(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	pluginDir := filepath.Join(configDir, "tui-plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginPath := filepath.Join(pluginDir, "gentle-logo.tsx")
	original := []byte("// pre-existing gentle logo plugin\nexport default {}\n")
	if err := os.WriteFile(pluginPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	// Record the pre-existing mode so the restore can be compared against it
	// on Windows, where os reports 0666 for every regular file (#4843).
	originalInfo, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err == nil {
		t.Fatal("Install() error = nil, want registration failure")
	}

	data, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("plugin source = %q, want restored %q", data, original)
	}
	assertRestoredSourceMode(t, pluginPath, originalInfo.Mode())
}

func TestPriorFileRestoreReportsRemovalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not deny file removal on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "gentle-logo.tsx")
	if err := os.WriteFile(target, []byte("created by a failed install"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	prior := priorFile{}
	if err := prior.restore(target); err == nil {
		t.Fatal("restore() error = nil, want removal failure")
	}
}

func TestInstallDoesNotRunPackageManager(t *testing.T) {
	home := t.TempDir()

	if _, err := Install(home, model.OpenCodePluginSubAgentStatusline); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("Install() should not create node_modules; stat err = %v", err)
	}
}

// assertRestoredSourceMode verifies the rollback preserved the plugin source
// mode. Go's os package reports 0666 for every regular file on Windows — there
// are no Unix permission bits to enforce — so on Windows it asserts the restore
// kept the mode recorded before the install instead of the Unix-only 0600
// (#4843).
func assertRestoredSourceMode(t *testing.T, path string, original os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if got := info.Mode().Perm(); got != original.Perm() {
			t.Fatalf("plugin source mode = %o, want preserved %o", got, original.Perm())
		}
		return
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("plugin source mode = %o, want 600", got)
	}
}

var errInjectedRegistration = errors.New("injected registration failure")
var errInjectedWrite = errors.New("injected source-write failure")

// landPluginSourceWrite simulates the landed-with-error window of
// WriteFileAtomic (#1676): the replacement is already on disk when the
// failure is reported.
func landPluginSourceWrite(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return filemerge.WriteResult{}, err
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return filemerge.WriteResult{}, err
	}
	if err := os.Chmod(path, perm); err != nil {
		return filemerge.WriteResult{}, err
	}
	return filemerge.WriteResult{Changed: true}, errInjectedWrite
}

// TestInstallGentleLogoRollsBackSourceWhenItsWriteLandsWithError covers the
// landed-with-error window of WriteFileAtomic (#1676) on the plugin-source
// write: the seam reports the replacement as changed AND returns an error, so
// installGentleLogo must compensate the source instead of trusting
// err != nil as "nothing happened".
func TestInstallGentleLogoRollsBackSourceWhenItsWriteLandsWithError(t *testing.T) {
	t.Run("removes a newly created source", func(t *testing.T) {
		home := t.TempDir()
		pluginPath := filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")

		origWrite := writeFileAtomicFn
		t.Cleanup(func() { writeFileAtomicFn = origWrite })
		writeFileAtomicFn = landPluginSourceWrite

		_, err := Install(home, model.OpenCodePluginGentleLogo)
		if err == nil {
			t.Fatal("Install() error = nil, want the injected source-write failure")
		}
		if !errors.Is(err, errInjectedWrite) {
			t.Fatalf("Install() error %v does not wrap the injected source-write failure", err)
		}
		if _, statErr := os.Stat(pluginPath); !os.IsNotExist(statErr) {
			t.Fatalf("newly created plugin source still exists after rollback; stat err = %v", statErr)
		}
	})

	t.Run("restores a pre-existing source byte-exactly", func(t *testing.T) {
		home := t.TempDir()
		pluginDir := filepath.Join(home, ".config", "opencode", "tui-plugins")
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			t.Fatal(err)
		}
		pluginPath := filepath.Join(pluginDir, "gentle-logo.tsx")
		original := []byte("// pre-existing gentle logo plugin\nexport default {}\n")
		if err := os.WriteFile(pluginPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
		// Record the pre-existing mode so the restore can be compared against
		// it on Windows, where os reports 0666 for every regular file (#4843).
		originalInfo, err := os.Stat(pluginPath)
		if err != nil {
			t.Fatal(err)
		}

		origWrite := writeFileAtomicFn
		t.Cleanup(func() { writeFileAtomicFn = origWrite })
		writeFileAtomicFn = landPluginSourceWrite

		_, err = Install(home, model.OpenCodePluginGentleLogo)
		if err == nil {
			t.Fatal("Install() error = nil, want the injected source-write failure")
		}
		if !errors.Is(err, errInjectedWrite) {
			t.Fatalf("Install() error %v does not wrap the injected source-write failure", err)
		}

		data, err := os.ReadFile(pluginPath)
		if err != nil {
			t.Fatalf("ReadFile(plugin) error = %v", err)
		}
		if !bytes.Equal(data, original) {
			t.Fatalf("plugin source = %q, want restored %q", data, original)
		}
		assertRestoredSourceMode(t, pluginPath, originalInfo.Mode())
	})
}

// TestInstallGentleLogoRollsBackTUIRegistrationWhenItLandsWithError covers the
// landed-with-error window of WriteFileAtomic (#1676): the seam reports the
// tui.json replacement as changed AND returns an error, so installGentleLogo
// must compensate both files instead of trusting err != nil as "nothing
// happened".
func TestInstallGentleLogoRollsBackTUIRegistrationWhenItLandsWithError(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tuiPath := filepath.Join(configDir, "tui.json")
	original := []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["existing-plugin"]}`)
	if err := os.WriteFile(tuiPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	origSeam := ensureTUIPluginFn
	t.Cleanup(func() { ensureTUIPluginFn = origSeam })
	ensureTUIPluginFn = func(path, pkg string) (bool, error) {
		// Simulate the landed-with-error window: the registration replacement
		// is already on disk when the failure is reported.
		landed, err := ensureTUIPlugin(path, pkg)
		if err != nil {
			return landed, err
		}
		return true, errInjectedRegistration
	}

	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("Install() error = nil, want the injected registration failure")
	}
	if !errors.Is(err, errInjectedRegistration) {
		t.Fatalf("Install() error %v does not wrap the injected registration failure", err)
	}

	data, err := os.ReadFile(tuiPath)
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}
	if !bytes.Equal(data, original) {
		t.Fatalf("tui.json was not restored byte-identically: got %q, want %q", data, original)
	}

	pluginPath := filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx")
	if _, statErr := os.Stat(pluginPath); !os.IsNotExist(statErr) {
		t.Fatalf("plugin source still exists after rollback; stat err = %v", statErr)
	}
}

// TestInstallGentleLogoReportsRollbackFailureInErrorChain verifies that when
// a restore fails, the returned error chain retains BOTH the registration
// failure and the rollback failure via errors.Join, and states that the
// previous state could not be restored.
func TestInstallGentleLogoReportsRollbackFailureInErrorChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not deny file removal on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tuiPath := filepath.Join(configDir, "tui.json")
	original := []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["existing-plugin"]}`)
	if err := os.WriteFile(tuiPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	pluginPath := filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx")

	origSeam := ensureTUIPluginFn
	t.Cleanup(func() { ensureTUIPluginFn = origSeam })
	ensureTUIPluginFn = func(path, pkg string) (bool, error) {
		// Simulate the landed-with-error window (#1676): the registration
		// replacement is already on disk when the failure is reported.
		landed, err := ensureTUIPlugin(path, pkg)
		if err != nil {
			return landed, err
		}
		// Then make the plugin-source restore fail: an unwritable tui-plugins
		// dir blocks the os.Remove inside priorFile.restore.
		pluginsDir := filepath.Dir(pluginPath)
		if err := os.Chmod(pluginsDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(pluginsDir, 0o755)
		})
		return true, errInjectedRegistration
	}

	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("Install() error = nil, want a joined failure")
	}
	if !errors.Is(err, errInjectedRegistration) {
		t.Fatalf("error %v does not retain the registration failure", err)
	}
	var removeErr *fs.PathError
	if !errors.As(err, &removeErr) || removeErr.Op != "remove" || removeErr.Path != pluginPath {
		t.Fatalf("error %v does not retain the plugin-source rollback failure", err)
	}
	if !strings.Contains(err.Error(), "the previous state could not be restored") {
		t.Fatalf("error %q does not state that the previous state could not be restored", err)
	}
}

func TestInstallGentleLogoWritesLocalTUIPluginAndRegistersAbsolutePath(t *testing.T) {
	home := t.TempDir()

	result, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Install() changed = false, want true")
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	configPath := filepath.Join(home, ".config", "opencode", "tui.json")
	wantFiles := map[string]bool{pluginPath: true, configPath: true}
	for _, file := range result.Files {
		delete(wantFiles, file)
	}
	if len(wantFiles) != 0 {
		t.Fatalf("Install() files = %#v, missing %#v", result.Files, wantFiles)
	}

	pluginData, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(plugin) error = %v", err)
	}
	pluginContent := string(pluginData)
	for _, snippet := range []string{
		`id = "gentle-logo"`,
		`home_logo`,
		`const plugin = { id: "gentle-logo", tui }`,
		`export default plugin`,
	} {
		if !strings.Contains(pluginContent, snippet) {
			t.Fatalf("plugin missing snippet %q", snippet)
		}
	}
	if strings.Contains(pluginContent, "server") {
		t.Fatalf("plugin must not export or define server shape")
	}
	for _, forbidden := range []string{"TuiThemeCurrent", "ctx.theme", "props.theme"} {
		if strings.Contains(pluginContent, forbidden) {
			t.Fatalf("plugin must not subscribe to theme state (%q) because OpenCode can destroy TextBuffer during theme swaps", forbidden)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(tui.json) error = %v", err)
	}
	var root struct {
		Plugin []string `json:"plugin"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(tui.json) error = %v", err)
	}
	if len(root.Plugin) != 1 || root.Plugin[0] != pluginPath || !filepath.IsAbs(root.Plugin[0]) {
		t.Fatalf("plugin registration = %#v, want absolute %q", root.Plugin, pluginPath)
	}
}
