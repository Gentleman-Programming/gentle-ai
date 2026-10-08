package opencodeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

func TestExternalPluginsRefusedWithoutWrites(t *testing.T) {
	for _, id := range []model.OpenCodeCommunityPluginID{model.OpenCodePluginSubAgentStatusline, model.OpenCodePluginSDDEngramManage} {
		t.Run(string(id), func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				home := t.TempDir()
				path := filepath.Join(home, ".config", "opencode", "tui.json")
				original := []byte(`{"plugin":["user-plugin","opencode-subagent-statusline","opencode-sdd-engram-manage"]}`)
				if existing {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, original, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				result, err := Install(home, id)
				if err == nil || result.Changed || len(result.Files) != 0 {
					t.Fatalf("retired Install = %+v, %v", result, err)
				}
				if existing {
					data, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(data, original) {
						t.Fatalf("configuration changed: %q, %v", data, err)
					}
				} else if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
					t.Fatalf("refusal wrote files: %v, %v", entries, err)
				}
			}
		})
	}
}

func TestLegacyLookupRemainsUninstallOnly(t *testing.T) {
	for id, pkg := range map[model.OpenCodeCommunityPluginID]string{
		model.OpenCodePluginSDDEngramManage:    "opencode-sdd-engram-manage",
		model.OpenCodePluginSubAgentStatusline: "opencode-subagent-statusline",
	} {
		t.Run(string(id), func(t *testing.T) {
			legacy, ok := DefinitionFor(id)
			if !ok || legacy.PackageName != pkg {
				t.Fatalf("legacy lookup = (%+v, %v)", legacy, ok)
			}
		})
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

// TestEmbeddedGentleLogoBundleIsAvailable pins the T2 packaging contract
// (issue #5364): OpenCode never transpiles TSX, so the installer ships the
// committed pre-built ESM bundle and it must carry both slot paths.
func TestEmbeddedGentleLogoBundleIsAvailable(t *testing.T) {
	data, err := EmbeddedBundle()
	if err != nil {
		t.Fatalf("EmbeddedBundle() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded bundle is empty")
	}
	content := string(data)
	for _, marker := range []string{"gentle-logo", "home.footer", "home_logo", "createRoot", "as default"} {
		if !strings.Contains(content, marker) {
			t.Fatalf("embedded bundle missing marker %q", marker)
		}
	}
}

func TestInstallDoesNotRunPackageManager(t *testing.T) {
	home := t.TempDir()

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("Install() should not create node_modules; stat err = %v", err)
	}
}

// TestInstallOnV2WritesBundleBridgeAndRegistrations pins the V2 install
// contract (issue #5364): the pre-built bundle plus the bridge directory
// package, the relative cli.json registration, the managed opencode.jsonc
// union, and no tui.json.
func TestInstallOnV2WritesBundleBridgeAndRegistrations(t *testing.T) {
	home := t.TempDir()
	setProbe(t, "2.0.4")

	result, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Install() changed = false, want true")
	}

	configDir := filepath.Join(home, ".config", "opencode")
	bundlePath := filepath.Join(configDir, "tui-plugins", "gentle-logo.js")
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("ReadFile(bundle) error = %v", err)
	}
	embedded, err := EmbeddedBundle()
	if err != nil {
		t.Fatalf("EmbeddedBundle() error = %v", err)
	}
	if !bytes.Equal(data, embedded) {
		t.Fatal("installed bundle must be byte-identical to the embedded artifact")
	}

	bridgeData, err := os.ReadFile(filepath.Join(configDir, "tui-plugins", "gentle-logo", "tui.js"))
	if err != nil {
		t.Fatalf("ReadFile(bridge tui.js) error = %v", err)
	}
	if string(bridgeData) != bridgeTUIFileContent {
		t.Fatalf("bridge tui.js = %q, want the exact re-export content", bridgeData)
	}
	manifestData, err := os.ReadFile(filepath.Join(configDir, "tui-plugins", "gentle-logo", "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(bridge package.json) error = %v", err)
	}
	if string(manifestData) != bridgeManifestContent {
		t.Fatalf("bridge package.json = %q, want the exact manifest content", manifestData)
	}

	cliPath := filepath.Join(configDir, "cli.json")
	var cliConfig struct {
		Schema  string   `json:"$schema"`
		Plugins []string `json:"plugins"`
	}
	decodeFile(t, cliPath, &cliConfig)
	if cliConfig.Schema != cliSchemaV2 {
		t.Fatalf("cli.json schema = %q, want %q", cliConfig.Schema, cliSchemaV2)
	}
	if len(cliConfig.Plugins) != 1 || cliConfig.Plugins[0] != gentleLogoV2Entry {
		t.Fatalf("cli.json plugins = %#v, want [%s]", cliConfig.Plugins, gentleLogoV2Entry)
	}

	var opencodeConfig struct {
		Schema  string   `json:"$schema"`
		Plugins []string `json:"plugins"`
	}
	decodeFile(t, filepath.Join(configDir, "opencode.jsonc"), &opencodeConfig)
	if opencodeConfig.Schema != opencodeSchemaV2 {
		t.Fatalf("opencode.jsonc schema = %q, want %q", opencodeConfig.Schema, opencodeSchemaV2)
	}
	if len(opencodeConfig.Plugins) != 1 || opencodeConfig.Plugins[0] != gentleLogoV2Entry {
		t.Fatalf("opencode.jsonc plugins = %#v, want [%s]", opencodeConfig.Plugins, gentleLogoV2Entry)
	}

	if _, err := os.Stat(filepath.Join(configDir, "tui.json")); !os.IsNotExist(err) {
		t.Fatalf("V2 install must not create tui.json; stat err = %v", err)
	}
	assertFilesMatchManagedLogoFiles(t, home, result.Files)
}

func TestInstallOnV2IsIdempotent(t *testing.T) {
	home := t.TempDir()
	setProbe(t, "2.0.4")

	first, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	second, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Install() changed = false, want true")
	}
	if second.Changed {
		t.Fatal("second Install() changed = true, want false")
	}
}

// TestInstallOnV2RetiresStaleT1RegistrationAndPreservesUserEntries pins the
// migration from the T1-era installation: the absolute .tsx registration in
// tui.json is removed, the user's other entries survive everywhere, and the
// V2 registration lands exactly once in cli.json and opencode.jsonc.
func TestInstallOnV2RetiresStaleT1RegistrationAndPreservesUserEntries(t *testing.T) {
	home := t.TempDir()
	setProbe(t, "2.0.4")

	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleT1Path := filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx")
	staleEntry := filepath.ToSlash(staleT1Path)
	if err := os.WriteFile(filepath.Join(configDir, "cli.json"), []byte(`{"$schema":"https://opencode.ai/v2/cli.json","plugins":["other-v2-plugin"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["user-plugin","`+staleEntry+`"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "opencode.jsonc"), []byte("{\n  // user comment\n  \"$schema\": \""+opencodeSchemaV2+"\",\n  \"plugins\": [\"legacy-plugin\"]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	var cliConfig struct {
		Plugins []string `json:"plugins"`
	}
	decodeFile(t, filepath.Join(configDir, "cli.json"), &cliConfig)
	if len(cliConfig.Plugins) != 2 || cliConfig.Plugins[0] != "other-v2-plugin" || cliConfig.Plugins[1] != gentleLogoV2Entry {
		t.Fatalf("cli.json plugins = %#v, want [other-v2-plugin %s]", cliConfig.Plugins, gentleLogoV2Entry)
	}

	var tuiConfig struct {
		Plugin []string `json:"plugin"`
	}
	decodeFile(t, filepath.Join(configDir, "tui.json"), &tuiConfig)
	if len(tuiConfig.Plugin) != 1 || tuiConfig.Plugin[0] != "user-plugin" {
		t.Fatalf("tui.json plugin = %#v, want only user-plugin (stale T1 entry removed)", tuiConfig.Plugin)
	}

	raw, err := os.ReadFile(filepath.Join(configDir, "opencode.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "// user comment") {
		t.Fatal("opencode.jsonc user comment was not preserved")
	}
	var opencodeConfig struct {
		Plugins []string `json:"plugins"`
	}
	decodeFile(t, filepath.Join(configDir, "opencode.jsonc"), &opencodeConfig)
	if len(opencodeConfig.Plugins) != 2 || opencodeConfig.Plugins[0] != "legacy-plugin" || opencodeConfig.Plugins[1] != gentleLogoV2Entry {
		t.Fatalf("opencode.jsonc plugins = %#v, want [legacy-plugin %s]", opencodeConfig.Plugins, gentleLogoV2Entry)
	}
}

// TestInstallOnV2PrefersCLIJSONArtifactOverV1Probe pins the durable-artifact
// detection: cli.json means V2 even when the version probe reports V1.
func TestInstallOnV2PrefersCLIJSONArtifactOverV1Probe(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "cli.json"), []byte(`{"$schema":"https://opencode.ai/v2/cli.json","plugins":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tui-plugins", "gentle-logo", "tui.js")); err != nil {
		t.Fatalf("cli.json artifact must select the V2 install path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "cli.json")); err != nil {
		t.Fatal(err)
	}
}

// TestInstallOnV1WritesBundleAndRegistersRelativePath pins the V1 contract:
// the bundle plus a relative tui.jsonc registration; no V2 artifacts.
func TestInstallOnV1WritesBundleAndRegistersRelativePath(t *testing.T) {
	home := t.TempDir()

	result, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Install() changed = false, want true")
	}

	configDir := filepath.Join(home, ".config", "opencode")
	if _, err := os.ReadFile(filepath.Join(configDir, "tui-plugins", "gentle-logo.js")); err != nil {
		t.Fatalf("ReadFile(bundle) error = %v", err)
	}
	var tuiConfig struct {
		Schema  string   `json:"$schema"`
		Plugins []string `json:"plugin"`
	}
	decodeFile(t, filepath.Join(configDir, "tui.jsonc"), &tuiConfig)
	if tuiConfig.Schema != tuiSchemaV1 {
		t.Fatalf("tui.jsonc schema = %q, want %q", tuiConfig.Schema, tuiSchemaV1)
	}
	if len(tuiConfig.Plugins) != 1 || tuiConfig.Plugins[0] != gentleLogoV1Entry {
		t.Fatalf("tui.jsonc plugin = %#v, want [%s]", tuiConfig.Plugins, gentleLogoV1Entry)
	}
	if _, err := os.Stat(filepath.Join(configDir, "cli.json")); !os.IsNotExist(err) {
		t.Fatalf("V1 install must not create cli.json; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tui-plugins", "gentle-logo")); !os.IsNotExist(err) {
		t.Fatalf("V1 install must not create the bridge directory; stat err = %v", err)
	}
	assertFilesMatchManagedLogoFiles(t, home, result.Files)
}

// TestInstallOnV1PrefersExistingTUIJSON pins the existing-file-wins cascade:
// a tui.json already present stays the primary target and receives the
// relative entry while T1-era stale entries are retired.
func TestInstallOnV1PrefersExistingTUIJSON(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleEntry := filepath.ToSlash(filepath.Join(configDir, "tui-plugins", "gentle-logo.tsx"))
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["user-plugin","`+staleEntry+`"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	second, err := Install(home, model.OpenCodePluginGentleLogo)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Install() changed = true, want false")
	}
	_ = first

	var tuiConfig struct {
		Plugin []string `json:"plugin"`
	}
	decodeFile(t, filepath.Join(configDir, "tui.json"), &tuiConfig)
	if len(tuiConfig.Plugin) != 2 || tuiConfig.Plugin[0] != "user-plugin" || tuiConfig.Plugin[1] != gentleLogoV1Entry {
		t.Fatalf("tui.json plugin = %#v, want [user-plugin %s]", tuiConfig.Plugin, gentleLogoV1Entry)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tui.jsonc")); !os.IsNotExist(err) {
		t.Fatalf("existing tui.json must stay the primary target; tui.jsonc stat err = %v", err)
	}
}

// TestInstallOnV1KeepsJSONCPrimaryAndJSONSecondary pins the cortex-ia
// secondary write: when both tui.jsonc and tui.json exist, both receive the
// relative registration.
func TestInstallOnV1KeepsJSONCPrimaryAndJSONSecondary(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.jsonc"), []byte("{\"$schema\":\"https://opencode.ai/tui.json\",\"plugin\":[\"jsonc-user\"]}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), []byte(`{"$schema":"https://opencode.ai/tui.json","plugin":["json-user"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for name, user := range map[string]string{"tui.jsonc": "jsonc-user", "tui.json": "json-user"} {
		var config struct {
			Plugin []string `json:"plugin"`
		}
		decodeFile(t, filepath.Join(configDir, name), &config)
		if len(config.Plugin) != 2 || config.Plugin[0] != user || config.Plugin[1] != gentleLogoV1Entry {
			t.Fatalf("%s plugin = %#v, want [%s %s]", name, config.Plugin, user, gentleLogoV1Entry)
		}
	}
}

// TestInstallOnV1RollsBackBundleWhenRegistrationFails pins the compensation
// contract: a registration failure must leave the bundle unwritten and the
// user's config byte-exact.
func TestInstallOnV1RollsBackBundleWhenRegistrationFails(t *testing.T) {
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

	if _, err := os.Stat(filepath.Join(configDir, "tui-plugins", "gentle-logo.js")); !os.IsNotExist(err) {
		t.Fatalf("bundle should not exist after failed registration; stat err = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(configDir, "tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(malformed) {
		t.Fatalf("tui.json = %q, want unchanged %q", data, malformed)
	}
}

// TestInstallOnV1RestoresPreExistingBundleWhenRegistrationFails pins that the
// rollback restores a pre-existing bundle byte-exactly, including its mode.
func TestInstallOnV1RestoresPreExistingBundleWhenRegistrationFails(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	pluginDir := filepath.Join(configDir, "tui-plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(pluginDir, "gentle-logo.js")
	original := []byte("// pre-existing gentle logo bundle\nexport default {}\n")
	if err := os.WriteFile(bundlePath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tui.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(home, model.OpenCodePluginGentleLogo); err == nil {
		t.Fatal("Install() error = nil, want registration failure")
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("bundle = %q, want restored %q", data, original)
	}
	assertRestoredSourceMode(t, bundlePath, originalInfo.Mode())
}

func TestPriorFileRestoreReportsRemovalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not deny file removal on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "gentle-logo.js")
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

var errInjectedRegistration = errors.New("injected registration failure")
var errInjectedWrite = errors.New("injected write failure")

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

// TestInstallOnV1RollsBackSourceWhenItsWriteLandsWithError covers the
// landed-with-error window of WriteFileAtomic (#1676) on the bundle write:
// the seam reports the replacement as changed AND returns an error, so
// installGentleLogo must compensate the bundle instead of trusting
// err != nil as "nothing happened".
func TestInstallOnV1RollsBackSourceWhenItsWriteLandsWithError(t *testing.T) {
	t.Run("removes a newly created bundle", func(t *testing.T) {
		home := t.TempDir()
		bundlePath := filepath.Join(home, ".config", "opencode", "tui-plugins", "gentle-logo.js")

		origWrite := writeFileAtomicFn
		t.Cleanup(func() { writeFileAtomicFn = origWrite })
		writeFileAtomicFn = landPluginSourceWrite

		_, err := Install(home, model.OpenCodePluginGentleLogo)
		if err == nil {
			t.Fatal("Install() error = nil, want the injected bundle-write failure")
		}
		if !errors.Is(err, errInjectedWrite) {
			t.Fatalf("Install() error %v does not wrap the injected bundle-write failure", err)
		}
		if _, statErr := os.Stat(bundlePath); !os.IsNotExist(statErr) {
			t.Fatalf("newly created bundle still exists after rollback; stat err = %v", statErr)
		}
	})

	t.Run("restores a pre-existing bundle byte-exactly", func(t *testing.T) {
		home := t.TempDir()
		pluginDir := filepath.Join(home, ".config", "opencode", "tui-plugins")
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			t.Fatal(err)
		}
		bundlePath := filepath.Join(pluginDir, "gentle-logo.js")
		original := []byte("// pre-existing gentle logo bundle\nexport default {}\n")
		if err := os.WriteFile(bundlePath, original, 0o600); err != nil {
			t.Fatal(err)
		}
		originalInfo, err := os.Stat(bundlePath)
		if err != nil {
			t.Fatal(err)
		}

		origWrite := writeFileAtomicFn
		t.Cleanup(func() { writeFileAtomicFn = origWrite })
		writeFileAtomicFn = landPluginSourceWrite

		_, err = Install(home, model.OpenCodePluginGentleLogo)
		if err == nil {
			t.Fatal("Install() error = nil, want the injected bundle-write failure")
		}
		if !errors.Is(err, errInjectedWrite) {
			t.Fatalf("Install() error %v does not wrap the injected bundle-write failure", err)
		}

		data, err := os.ReadFile(bundlePath)
		if err != nil {
			t.Fatalf("ReadFile(bundle) error = %v", err)
		}
		if !bytes.Equal(data, original) {
			t.Fatalf("bundle = %q, want restored %q", data, original)
		}
		assertRestoredSourceMode(t, bundlePath, originalInfo.Mode())
	})
}

// TestInstallOnV2RollsBackEverythingWhenCLIJSONWriteLandsWithError covers the
// landed-with-error window of WriteFileAtomic (#1676) on the cli.json
// registration: the bundle and bridge writes already landed, so the
// compensation must remove them and leave no V2 registration behind.
func TestInstallOnV2RollsBackEverythingWhenCLIJSONWriteLandsWithError(t *testing.T) {
	home := t.TempDir()
	setProbe(t, "2.0.4")
	configDir := filepath.Join(home, ".config", "opencode")

	origWrite := writeFileAtomicFn
	t.Cleanup(func() { writeFileAtomicFn = origWrite })
	writeFileAtomicFn = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
		if filepath.Base(filepath.Dir(path)) == gentleLogoBridgeDir || filepath.Base(path) == gentleLogoBundleFile {
			landed, err := filemerge.WriteFileAtomic(path, content, perm)
			return landed, err
		}
		// Simulate the landed-with-error window on the registration write:
		// the replacement is already on disk when the failure is reported.
		landed, err := filemerge.WriteFileAtomic(path, content, perm)
		if err != nil {
			return landed, err
		}
		return filemerge.WriteResult{Changed: true}, errInjectedRegistration
	}

	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("Install() error = nil, want the injected registration failure")
	}
	if !errors.Is(err, errInjectedRegistration) {
		t.Fatalf("Install() error %v does not wrap the injected registration failure", err)
	}

	for _, path := range []string{
		filepath.Join(configDir, "tui-plugins", "gentle-logo.js"),
		filepath.Join(configDir, "tui-plugins", "gentle-logo", "tui.js"),
		filepath.Join(configDir, "tui-plugins", "gentle-logo", "package.json"),
		filepath.Join(configDir, "cli.json"),
		filepath.Join(configDir, "opencode.jsonc"),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("%s still exists after rollback; stat err = %v", path, statErr)
		}
	}
}

// TestInstallOnV2ReportsRollbackFailureInErrorChain verifies that when a
// restore fails, the returned error chain retains BOTH the registration
// failure and the rollback failure via errors.Join, and states that the
// previous state could not be restored.
func TestInstallOnV2ReportsRollbackFailureInErrorChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not deny file removal on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	home := t.TempDir()
	setProbe(t, "2.0.4")
	configDir := filepath.Join(home, ".config", "opencode")
	bundlePath := filepath.Join(configDir, "tui-plugins", "gentle-logo.js")

	origWrite := writeFileAtomicFn
	t.Cleanup(func() { writeFileAtomicFn = origWrite })
	writeFileAtomicFn = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
		landed, err := filemerge.WriteFileAtomic(path, content, perm)
		if err != nil {
			return landed, err
		}
		if filepath.Base(path) == cliConfigName {
			// Then make the bundle restore fail: an unwritable tui-plugins
			// dir blocks the os.Remove inside priorFile.restore.
			if err := os.Chmod(filepath.Dir(bundlePath), 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.Chmod(filepath.Dir(bundlePath), 0o755)
			})
			return filemerge.WriteResult{Changed: true}, errInjectedRegistration
		}
		return landed, nil
	}

	_, err := Install(home, model.OpenCodePluginGentleLogo)
	if err == nil {
		t.Fatal("Install() error = nil, want a joined failure")
	}
	if !errors.Is(err, errInjectedRegistration) {
		t.Fatalf("error %v does not retain the registration failure", err)
	}
	var removeErr *fs.PathError
	if !errors.As(err, &removeErr) || removeErr.Op != "remove" || removeErr.Path != bundlePath {
		t.Fatalf("error %v does not retain the bundle rollback failure", err)
	}
	if !strings.Contains(err.Error(), "the previous state could not be restored") {
		t.Fatalf("error %q does not state that the previous state could not be restored", err)
	}
}

// setProbe substitutes the OpenCode version probe for the duration of the
// test, mirroring the runtime detection seam used across this package.
func setProbe(t *testing.T, version string) {
	t.Helper()
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte(version)}, nil
	}
}

// assertFilesMatchManagedLogoFiles pins that Install reports exactly the
// managed file set ManagedLogoFiles advertises for backup/verification.
func assertFilesMatchManagedLogoFiles(t *testing.T, home string, files []string) {
	t.Helper()
	managed, err := ManagedLogoFiles(home)
	if err != nil {
		t.Fatalf("ManagedLogoFiles() error = %v", err)
	}
	if len(managed) != len(files) {
		t.Fatalf("Install files = %#v, want the managed set %#v", files, managed)
	}
	for i := range managed {
		if managed[i] != files[i] {
			t.Fatalf("Install files = %#v, want the managed set %#v", files, managed)
		}
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

func decodeFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	// JSONC files may legitimately carry comments; normalize before decoding.
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		t.Fatalf("decode(%s) error = %v", path, err)
	}
	normalized, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(normalized, target); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
}
