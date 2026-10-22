package engram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/hermes"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/openclaw"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/qwen"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// ─── #1635 Antigravity prevalidation and write accounting tests ─────────────

var errAntigravityWriteFault = errors.New("injected antigravity write fault")

// antigravityWriteFixture seeds a home with a global Antigravity MCP config
// holding an unrelated server plus a managed Engram duplicate, and returns the
// five paths the Antigravity injection pass touches, in write order.
func antigravityWriteFixture(t *testing.T) (home, globalPath, settingsPath, pluginPath, pluginMCPPath, hooksPath string) {
	t.Helper()
	home = t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	pluginDir := filepath.Join(cliDir, "plugins", "gentle-ai-engram")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", pluginDir, err)
	}
	globalPath = filepath.Join(cliDir, "mcp_config.json")
	global := `{
  "mcpServers": {
    "context7": {"command": "npx", "args": ["-y", "@upstash/context7-mcp"]},
    "engram": {"command": "/usr/local/bin/engram", "args": ["mcp"]}
  }
}
`
	if err := os.WriteFile(globalPath, []byte(global), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", globalPath, err)
	}
	settingsPath = filepath.Join(cliDir, "settings.json")
	pluginPath = filepath.Join(pluginDir, "plugin.json")
	pluginMCPPath = filepath.Join(pluginDir, "mcp_config.json")
	hooksPath = filepath.Join(pluginDir, "hooks.json")
	return home, globalPath, settingsPath, pluginPath, pluginMCPPath, hooksPath
}

// failAntigravityWrite routes every Antigravity write through the real writer
// on real disk, except writes whose path ends in suffix: when land is true the
// real bytes still land and only the returned result carries the fault; when
// land is false the real writer is never called, simulating a failure before
// any replacement.
func failAntigravityWrite(t *testing.T, suffix string, land bool) {
	t.Helper()
	orig := writeAntigravityFileAtomic
	writeAntigravityFileAtomic = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
		if !strings.HasSuffix(path, suffix) {
			return orig(path, content, perm)
		}
		if !land {
			return filemerge.WriteResult{}, errAntigravityWriteFault
		}
		result, err := orig(path, content, perm)
		if err == nil {
			err = errAntigravityWriteFault
		}
		return result, err
	}
	t.Cleanup(func() { writeAntigravityFileAtomic = orig })
}

// assertAntigravityCanonicalPluginAsset asserts that one Antigravity plugin
// asset on disk holds exactly the canonical bytes the injection installs.
// The expected MCP command is resolved from the fixtures, which all seed a
// managed global entry carrying /usr/local/bin/engram.
func assertAntigravityCanonicalPluginAsset(t *testing.T, path string) {
	t.Helper()
	got := readAntigravityFile(t, path)
	var want []byte
	switch filepath.Base(path) {
	case "plugin.json":
		want = []byte(antigravityEngramPluginJSON)
	case "mcp_config.json":
		want = engramOverlayJSON(model.AgentAntigravity, "/usr/local/bin/engram")
	case "hooks.json":
		want = antigravityEngramHooksJSON()
	default:
		t.Fatalf("assertAntigravityCanonicalPluginAsset called on unknown asset %q", path)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("plugin asset %q must hold the canonical installed bytes\nwant:\n%s\ngot:\n%s", path, want, got)
	}
}

func TestInjectAntigravityPrevalidatesGlobalMCPConfigBeforeWrites(t *testing.T) {
	globalContent := func(s string) *string { return &s }
	for _, tc := range []struct {
		name    string
		content *string // nil leaves the global config missing
		wantErr bool
		locked  bool
	}{
		{name: "missing file is accepted"},
		{name: "empty file is accepted without normalization", content: globalContent("")},
		{name: "whitespace-only file is accepted without normalization", content: globalContent(" \t\r\n")},
		{name: "object file is accepted", content: globalContent(`{"mcpServers":{"context7":{"command":"npx"}}}` + "\n")},
		{name: "malformed JSON is rejected", content: globalContent(`{"mcpServers":`), wantErr: true},
		{name: "JSON null is rejected", content: globalContent("null"), wantErr: true},
		{name: "JSON array is rejected", content: globalContent("[]"), wantErr: true},
		{name: "JSON string is rejected", content: globalContent(`"engram"`), wantErr: true},
		{name: "JSON number is rejected", content: globalContent("42"), wantErr: true},
		{name: "JSON bool is rejected", content: globalContent("true"), wantErr: true},
		{name: "unreadable file is rejected", content: globalContent("{}\n"), wantErr: true, locked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.locked {
				if runtime.GOOS == "windows" {
					t.Skip("file permission bits are not supported on Windows")
				}
				if os.Geteuid() == 0 {
					t.Skip("root ignores file permission bits")
				}
			}
			home := t.TempDir()
			cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
			mcpPath := filepath.Join(cliDir, "mcp_config.json")
			if tc.content != nil {
				if err := os.MkdirAll(cliDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) error = %v", cliDir, err)
				}
				if err := os.WriteFile(mcpPath, []byte(*tc.content), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
				}
				if tc.locked {
					if err := os.Chmod(mcpPath, 0o000); err != nil {
						t.Fatalf("Chmod(%q) error = %v", mcpPath, err)
					}
				}
			}

			_, err := Inject(home, antigravityAdapter())

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Inject(antigravity) succeeded; want Antigravity global MCP config prevalidation error")
				}
				if !strings.Contains(err.Error(), "Antigravity global MCP config") {
					t.Fatalf("error = %v, want it to name the Antigravity global MCP config", err)
				}
				// No mutation and no plugin activation next to unclassifiable input.
				if _, statErr := os.Stat(filepath.Join(cliDir, "plugins", "gentle-ai-engram")); !os.IsNotExist(statErr) {
					t.Fatalf("plugin must not be activated; stat err = %v", statErr)
				}
				if _, statErr := os.Stat(filepath.Join(cliDir, "settings.json")); !os.IsNotExist(statErr) {
					t.Fatalf("settings must not be created; stat err = %v", statErr)
				}
				if tc.content != nil && !tc.locked {
					got, readErr := os.ReadFile(mcpPath)
					if readErr != nil {
						t.Fatalf("ReadFile(%q) error = %v", mcpPath, readErr)
					}
					if string(got) != *tc.content {
						t.Fatalf("global MCP config mutated\nwant:\n%s\ngot:\n%s", *tc.content, got)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Inject(antigravity) error = %v", err)
			}
			// Accepted input must be left byte-identical, never normalized.
			if tc.content != nil {
				got, readErr := os.ReadFile(mcpPath)
				if readErr != nil {
					t.Fatalf("ReadFile(%q) error = %v", mcpPath, readErr)
				}
				if string(got) != *tc.content {
					t.Fatalf("accepted global MCP config was rewritten\nwant:\n%s\ngot:\n%s", *tc.content, got)
				}
			}
			pluginMCPPath := filepath.Join(cliDir, "plugins", "gentle-ai-engram", "mcp_config.json")
			if _, statErr := os.Stat(pluginMCPPath); statErr != nil {
				t.Fatalf("plugin must be activated for accepted input; stat err = %v", statErr)
			}
		})
	}
}

func TestInjectAntigravityAccountsEachWriteFailure(t *testing.T) {
	cliMCPSuffix := filepath.Join(".gemini", "antigravity-cli", "mcp_config.json")
	for _, tc := range []struct {
		name   string
		suffix string
		land   bool
	}{
		{name: "settings bootstrap failing before replacement", suffix: filepath.Join(".gemini", "antigravity-cli", "settings.json")},
		{name: "settings bootstrap failing after landing", suffix: filepath.Join(".gemini", "antigravity-cli", "settings.json"), land: true},
		{name: "plugin manifest failing before replacement", suffix: filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "plugin.json")},
		{name: "plugin manifest failing after landing", suffix: filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "plugin.json"), land: true},
		{name: "plugin MCP config failing before replacement", suffix: filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json")},
		{name: "plugin MCP config failing after landing", suffix: filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json"), land: true},
		{name: "plugin hooks failing before replacement", suffix: filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "hooks.json")},
		{name: "plugin hooks failing after landing", suffix: filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "hooks.json"), land: true},
		{name: "global rewrite failing before replacement", suffix: cliMCPSuffix},
		{name: "global rewrite failing after landing", suffix: cliMCPSuffix, land: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, globalPath, settingsPath, pluginPath, pluginMCPPath, hooksPath := antigravityWriteFixture(t)
			// #1635 UNIT3 write order: settings first, then the plugin install,
			// and only then the managed global registration removal.
			order := []string{settingsPath, pluginPath, pluginMCPPath, hooksPath, globalPath}
			target := -1
			for i, path := range order {
				if strings.HasSuffix(path, tc.suffix) {
					target = i
					break
				}
			}
			if target < 0 {
				t.Fatalf("fault suffix %q matched no fixture path", tc.suffix)
			}
			failAntigravityWrite(t, tc.suffix, tc.land)
			wantCount := target
			if tc.land {
				wantCount++
			}
			wantFiles := order[:wantCount]
			// When the global rewrite landed, ownership transferred despite the error.
			postlandGlobal := target == len(order)-1 && tc.land

			result, err := Inject(home, antigravityAdapter())

			assertAntigravityFaults(t, err, errAntigravityWriteFault)
			// A landed replacement is accounted even alongside the error.
			assertAntigravityLandedFiles(t, result, wantFiles)
			for i, path := range order {
				if i >= wantCount {
					if path == globalPath {
						// The global config pre-exists; a failure before the rewrite
						// must leave its original bytes on disk.
						raw := readAntigravityFile(t, path)
						if !strings.Contains(string(raw), "/usr/local/bin/engram") || !strings.Contains(string(raw), "context7") {
							t.Fatalf("untouched global MCP config must keep its original bytes:\n%s", raw)
						}
						continue
					}
					if path == settingsPath {
						if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
							t.Fatalf("%q must not be created; stat err = %v", path, statErr)
						}
						continue
					}
					// A plugin asset only survives on disk when the global rewrite
					// landed (ownership transferred); otherwise restored to absent.
					if !postlandGlobal {
						if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
							t.Fatalf("%q must not be touched; stat err = %v", path, statErr)
						}
						continue
					}
					assertAntigravityCanonicalPluginAsset(t, path)
					continue
				}
				if path == settingsPath { // settings bootstrap landed: empty JSON object
					raw := readAntigravityFile(t, path)
					if strings.TrimSpace(string(raw)) != "{}" {
						t.Fatalf("settings on disk = %q, want empty JSON object", raw)
					}
					continue
				}
				if path == globalPath { // global rewrite landed: managed duplicate gone, unrelated server kept
					raw := readAntigravityFile(t, path)
					if strings.Contains(string(raw), "/usr/local/bin/engram") || !strings.Contains(string(raw), "context7") {
						t.Fatalf("global MCP config on disk wrong after landed rewrite:\n%s", raw)
					}
					continue
				}
				if !postlandGlobal {
					// #1635 UNIT2: the plugin asset landed but recovery restored
					// it to its absent before-image; the write stays accounted.
					if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
						t.Fatalf("%q was newly created and must be restored to absent; stat err = %v", path, statErr)
					}
					continue
				}
				assertAntigravityCanonicalPluginAsset(t, path)
			}
		})
	}
}

// ─── #1635 UNIT2 Antigravity plugin before-image and recovery tests ──────────

// antigravityRecoveryFixture seeds a home whose plugin directory already holds
// user-owned plugin assets (custom bytes, non-default modes), one absent
// plugin asset, and an unrelated third-party file. It returns the paths the
// recovery tests assert against plus the before-image of every seeded asset.
func antigravityRecoveryFixture(t *testing.T) (home, settingsPath, pluginDir, pluginPath, pluginMCPPath, hooksPath, unrelatedPath string, before map[string]antigravityAssetBackup) {
	t.Helper()
	home = t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	pluginDir = filepath.Join(cliDir, "plugins", "gentle-ai-engram")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", pluginDir, err)
	}
	settingsPath = filepath.Join(cliDir, "settings.json")
	pluginPath = filepath.Join(pluginDir, "plugin.json")
	pluginMCPPath = filepath.Join(pluginDir, "mcp_config.json")
	hooksPath = filepath.Join(pluginDir, "hooks.json")
	unrelatedPath = filepath.Join(pluginDir, "user-notes.txt")

	before = make(map[string]antigravityAssetBackup)
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(path, mode); err != nil {
				t.Fatalf("Chmod(%q) error = %v", path, err)
			}
		}
		snapshot, err := captureAntigravityPluginAsset(path)
		if err != nil {
			t.Fatalf("captureAntigravityPluginAsset(%q) error = %v", path, err)
		}
		before[path] = snapshot
	}
	write(pluginPath, "{\n  \"name\": \"gentle-ai-engram\",\n  \"user\": true\n}\n", 0o640)
	write(pluginMCPPath, `{"mcpServers":{"engram":{"command":"/custom/engram","args":["mcp"]}}}`+"\n", 0o600)
	write(unrelatedPath, "user notes\n", 0o644)
	// hooks.json is deliberately absent so recovery exercises both preexisting
	// and newly created assets.
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Fatalf("hooks.json must be absent in this fixture; stat err = %v", err)
	}
	return home, settingsPath, pluginDir, pluginPath, pluginMCPPath, hooksPath, unrelatedPath, before
}

// assertAntigravityAssetState asserts the actual on-disk state of one asset
// against its recorded before-image: existence, type, exact bytes, and mode.
func assertAntigravityAssetState(t *testing.T, label, path string, want antigravityAssetBackup) {
	t.Helper()
	info, err := os.Lstat(path)
	if !want.existed {
		if !os.IsNotExist(err) {
			t.Fatalf("%s: %q must not exist; stat err = %v", label, path, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %q missing; stat err = %v", label, path, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s: %q is not a regular file: %v", label, path, info.Mode())
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("%s: ReadFile(%q) error = %v", label, path, readErr)
	}
	if !bytes.Equal(got, want.content) {
		t.Fatalf("%s: %q bytes changed\nwant:\n%s\ngot:\n%s", label, path, want.content, got)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != want.mode {
		t.Fatalf("%s: %q mode = %04o, want %04o", label, path, info.Mode().Perm(), want.mode)
	}
}

func TestInjectAntigravityRejectsUnclassifiablePluginAssetBeforeWrites(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, hooksPath, pluginPath string)
		skip  string
	}{
		{
			name: "directory where hooks.json belongs",
			plant: func(t *testing.T, hooksPath, _ string) {
				if err := os.Mkdir(hooksPath, 0o755); err != nil {
					t.Fatalf("Mkdir(%q) error = %v", hooksPath, err)
				}
			},
		},
		{
			name: "symlink where hooks.json belongs",
			plant: func(t *testing.T, hooksPath, pluginPath string) {
				if err := os.Symlink(pluginPath, hooksPath); err != nil {
					t.Fatalf("Symlink(%q) error = %v", hooksPath, err)
				}
			},
		},
		{
			name: "unreadable hooks.json",
			skip: "permission bits",
			plant: func(t *testing.T, hooksPath, _ string) {
				if err := os.WriteFile(hooksPath, []byte("{}\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) error = %v", hooksPath, err)
				}
				if err := os.Chmod(hooksPath, 0o000); err != nil {
					t.Fatalf("Chmod(%q) error = %v", hooksPath, err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" {
				if runtime.GOOS == "windows" {
					t.Skip("file permission bits are not supported on Windows")
				}
				if os.Geteuid() == 0 {
					t.Skip("root ignores file permission bits")
				}
			}
			home, _, _, pluginPath, pluginMCPPath, hooksPath, unrelatedPath, before := antigravityRecoveryFixture(t)
			tc.plant(t, hooksPath, pluginPath)

			_, err := Inject(home, antigravityAdapter())
			if err == nil {
				t.Fatalf("Inject(antigravity) succeeded; want a plugin asset classification error")
			}
			// The error quotes the path with %q, which escapes Windows backslashes,
			// so match the quoted form rather than the raw path.
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", hooksPath)) || !strings.Contains(err.Error(), "plugin asset") {
				t.Fatalf("error = %v, want it to name the unclassifiable plugin asset %q", err, hooksPath)
			}
			// The rejection happens before any plugin write: every preexisting
			// asset keeps its exact bytes and mode, and the unsupported asset is
			// never written through or removed.
			assertAntigravityAssetState(t, "preserved", pluginPath, before[pluginPath])
			assertAntigravityAssetState(t, "preserved", pluginMCPPath, before[pluginMCPPath])
			assertAntigravityAssetState(t, "preserved", unrelatedPath, before[unrelatedPath])
			info, statErr := os.Lstat(hooksPath)
			if statErr != nil {
				t.Fatalf("Lstat(%q) error = %v", hooksPath, statErr)
			}
			if tc.name == "directory where hooks.json belongs" && !info.IsDir() {
				t.Fatalf("hooks.json directory was replaced: %v", info.Mode())
			}
			if tc.name == "symlink where hooks.json belongs" && info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("hooks.json symlink was replaced: %v", info.Mode())
			}
		})
	}
}

func TestInjectAntigravityRestoresTouchedPluginAssetsToExactBeforeImage(t *testing.T) {
	pluginAssetOrder := []string{"plugin.json", "mcp_config.json", "hooks.json"}
	for _, tc := range []struct {
		name   string
		suffix string
		land   bool
	}{
		{name: "manifest failing before replacement", suffix: pluginAssetOrder[0]},
		{name: "manifest failing after landing", suffix: pluginAssetOrder[0], land: true},
		{name: "MCP config failing before replacement", suffix: pluginAssetOrder[1]},
		{name: "MCP config failing after landing", suffix: pluginAssetOrder[1], land: true},
		{name: "hooks failing before replacement", suffix: pluginAssetOrder[2]},
		{name: "hooks failing after landing", suffix: pluginAssetOrder[2], land: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, settingsPath, pluginDir, pluginPath, pluginMCPPath, hooksPath, unrelatedPath, before := antigravityRecoveryFixture(t)
			paths := map[string]string{
				pluginAssetOrder[0]: pluginPath,
				pluginAssetOrder[1]: pluginMCPPath,
				pluginAssetOrder[2]: hooksPath,
			}
			failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", tc.suffix), tc.land)

			result, err := Inject(home, antigravityAdapter())

			if err == nil {
				t.Fatalf("Inject(antigravity) succeeded; want the injected write fault")
			}
			if !errors.Is(err, errAntigravityWriteFault) {
				t.Fatalf("error = %v, want errors.Is(err, errAntigravityWriteFault)", err)
			}
			// Only the plugin write fails, so the settings bootstrap before it
			// landed, and every plugin write up to and including the faulted one
			// occurred — recovery restores the assets but must not erase the
			// accounting of mutations that happened.
			wantFiles := []string{settingsPath}
			for _, name := range pluginAssetOrder {
				wantFiles = append(wantFiles, paths[name])
				if name == tc.suffix {
					if !tc.land {
						wantFiles = wantFiles[:len(wantFiles)-1]
					}
					break
				}
			}
			if !result.Changed {
				t.Fatalf("result.Changed = false; the settings creation and landed plugin writes are real mutations")
			}
			if len(result.Files) != len(wantFiles) {
				t.Fatalf("result.Files = %v, want %v", result.Files, wantFiles)
			}
			for i, want := range wantFiles {
				if result.Files[i] != want {
					t.Fatalf("result.Files[%d] = %q, want %q (all: %v)", i, result.Files[i], want, result.Files)
				}
			}
			// Every touched plugin asset is back to its exact before-image:
			// preexisting bytes and mode, absent for newly created files.
			for _, name := range pluginAssetOrder {
				assertAntigravityAssetState(t, "restored", paths[name], before[paths[name]])
			}
			// The unrelated third-party asset and the plugin directory itself
			// are never part of any restore.
			assertAntigravityAssetState(t, "unrelated", unrelatedPath, before[unrelatedPath])
			info, statErr := os.Stat(pluginDir)
			if statErr != nil || !info.IsDir() {
				t.Fatalf("plugin directory must survive recovery; stat err = %v", statErr)
			}
		})
	}
}

func TestInjectAntigravityRecoveryFailureIsExplicit(t *testing.T) {
	t.Run("restoration write failure", func(t *testing.T) {
		home, settingsPath, _, pluginPath, pluginMCPPath, hooksPath, _, _ := antigravityRecoveryFixture(t)
		failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "hooks.json"), true)
		restoreFault := errors.New("injected antigravity restore fault")
		origRestore := restoreAntigravityFileAtomic
		restoreAntigravityFileAtomic = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
			return filemerge.WriteResult{}, restoreFault
		}
		t.Cleanup(func() { restoreAntigravityFileAtomic = origRestore })

		result, err := Inject(home, antigravityAdapter())

		if err == nil {
			t.Fatalf("Inject(antigravity) succeeded; want the injected write fault")
		}
		// Both the original write error and the recovery error must survive.
		if !errors.Is(err, errAntigravityWriteFault) {
			t.Fatalf("error must preserve the original write fault: %v", err)
		}
		if !errors.Is(err, restoreFault) {
			t.Fatalf("error must preserve the restoration failure: %v", err)
		}
		if !strings.Contains(err.Error(), "could not be confirmed") {
			t.Fatalf("error must state the final plugin state could not be confirmed: %v", err)
		}
		// The landed writes are still accounted even though recovery failed.
		wantFiles := []string{settingsPath, pluginPath, pluginMCPPath, hooksPath}
		if !result.Changed || len(result.Files) != len(wantFiles) {
			t.Fatalf("result = %+v, want Changed with Files %v", result, wantFiles)
		}
		for i, want := range wantFiles {
			if result.Files[i] != want {
				t.Fatalf("result.Files[%d] = %q, want %q (all: %v)", i, result.Files[i], want, result.Files)
			}
		}
		// Restoration was refused for the preexisting assets, so they still
		// hold the injected canonical bytes; the newly created hooks file
		// was still removable. This is reported, never claimed as success.
		if got, readErr := os.ReadFile(pluginPath); readErr != nil || string(got) != antigravityEngramPluginJSON {
			t.Fatalf("plugin manifest must still hold the landed canonical bytes while restoration is broken; got %q, err %v", got, readErr)
		}
		if _, statErr := os.Stat(hooksPath); !os.IsNotExist(statErr) {
			t.Fatalf("newly created hooks.json must still be removed by recovery; stat err = %v", statErr)
		}
	})

	t.Run("recovery read failure", func(t *testing.T) {
		home, _, _, pluginPath, pluginMCPPath, hooksPath, _, _ := antigravityRecoveryFixture(t)
		failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "hooks.json"), true)
		readbackFault := errors.New("injected antigravity readback fault")
		origRead := readAntigravityPluginAsset
		readAntigravityPluginAsset = func(path string) (antigravityAssetBackup, error) {
			return antigravityAssetBackup{}, readbackFault
		}
		t.Cleanup(func() { readAntigravityPluginAsset = origRead })

		result, err := Inject(home, antigravityAdapter())

		if err == nil {
			t.Fatalf("Inject(antigravity) succeeded; want the injected write fault")
		}
		if !errors.Is(err, errAntigravityWriteFault) {
			t.Fatalf("error must preserve the original write fault: %v", err)
		}
		if !errors.Is(err, readbackFault) {
			t.Fatalf("error must preserve the readback failure: %v", err)
		}
		if !strings.Contains(err.Error(), "could not be confirmed") {
			t.Fatalf("error must state the final plugin state could not be confirmed: %v", err)
		}
		if !result.Changed {
			t.Fatalf("result.Changed = false; the landed writes are real mutations")
		}
		// A faulted read cannot prove the disk still holds what this pass
		// wrote, so recovery must refuse to restore anything rather than
		// risk clobbering newer bytes: every asset keeps its landed state,
		// and the uncertainty is reported, never claimed as success.
		// (No global config is seeded here, so the installed command preserves
		// the /custom/engram command the fixture plugin already carried.)
		want := map[string][]byte{
			pluginPath:    []byte(antigravityEngramPluginJSON),
			pluginMCPPath: engramOverlayJSON(model.AgentAntigravity, "/custom/engram"),
			hooksPath:     antigravityEngramHooksJSON(),
		}
		for path, content := range want {
			if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, content) {
				t.Fatalf("plugin asset %q must keep its landed canonical bytes while recovery is refused; got %q, err %v", path, got, readErr)
			}
		}
	})
}

var errAntigravityRemoveFault = errors.New("injected antigravity remove fault")

// failAntigravityRemove routes the sole-entry removal through the real
// os.Remove boundary: mode "before" faults without unlinking; mode "after"
// unlinks for real, then faults — the cleanup must classify a gone config.
func failAntigravityRemove(t *testing.T, mode string) {
	t.Helper()
	orig := removeAntigravityGlobalFile
	removeAntigravityGlobalFile = func(path string) error {
		if mode == "after" {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return errAntigravityRemoveFault
	}
	t.Cleanup(func() { removeAntigravityGlobalFile = orig })
}

// seedAntigravityGlobalManagedDuplicate writes the shared global Antigravity
// MCP config (context7 + managed Engram duplicate); returns path and bytes.
func seedAntigravityGlobalManagedDuplicate(t *testing.T, home string) (string, string) {
	t.Helper()
	globalPath := filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json")
	global := `{
  "mcpServers": {
    "context7": {"command": "npx", "args": ["-y", "@upstash/context7-mcp"]},
    "engram": {"command": "/usr/local/bin/engram", "args": ["mcp"]}
  }
}
`
	if err := os.WriteFile(globalPath, []byte(global), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", globalPath, err)
	}
	return globalPath, global
}

// assertAntigravityLandedFiles asserts mutations reported iff want is non-empty, exactly want, in order.
func assertAntigravityLandedFiles(t *testing.T, result InjectionResult, want []string) {
	t.Helper()
	if result.Changed != (len(want) > 0) {
		t.Fatalf("result.Changed = %v, want %v (mutations %v)", result.Changed, len(want) > 0, want)
	}
	if len(result.Files) != len(want) {
		t.Fatalf("result.Files = %v, want %v", result.Files, want)
	}
	for i, path := range want {
		if result.Files[i] != path {
			t.Fatalf("result.Files[%d] = %q, want %q (all: %v)", i, result.Files[i], path, result.Files)
		}
	}
}

// assertAntigravityFaults asserts the injection failed and preserved every error identity in wants.
func assertAntigravityFaults(t *testing.T, err error, wants ...error) {
	t.Helper()
	if err == nil {
		t.Fatal("Inject(antigravity) succeeded; want an injected fault")
	}
	for _, want := range wants {
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want errors.Is(err, %v)", err, want)
		}
	}
}

// readAntigravityFile reads a fixture or result file, failing on IO errors.
func readAntigravityFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return raw
}

// seedAntigravitySoleEntry writes a sole managed-entry global config; returns dir, path, bytes.
func seedAntigravitySoleEntry(t *testing.T, home string) (string, string, string) {
	t.Helper()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	mcpPath := filepath.Join(cliDir, "mcp_config.json")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", cliDir, err)
	}
	sole := `{"mcpServers":{"engram":{"command":"/usr/local/bin/engram","args":["mcp"]}}}` + "\n"
	if err := os.WriteFile(mcpPath, []byte(sole), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
	}
	return cliDir, mcpPath, sole
}

func TestInjectAntigravitySoleEntryRemovalFailureClassifiesOwnership(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
	}{
		{name: "removal failing before the real unlink", mode: "before"},
		{name: "removal faulting only after the real unlink", mode: "after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			cliDir, mcpPath, sole := seedAntigravitySoleEntry(t, home)
			failAntigravityRemove(t, tc.mode)

			result, err := Inject(home, antigravityAdapter())
			assertAntigravityFaults(t, err, errAntigravityRemoveFault)
			pluginAssets := []string{
				filepath.Join(cliDir, "plugins", "gentle-ai-engram", "plugin.json"),
				filepath.Join(cliDir, "plugins", "gentle-ai-engram", "mcp_config.json"),
				filepath.Join(cliDir, "plugins", "gentle-ai-engram", "hooks.json")}
			landed := append([]string{filepath.Join(cliDir, "settings.json")}, pluginAssets...)
			if tc.mode == "after" {
				// The real unlink happened before the fault: a landed mutation.
				landed = append(landed, mcpPath)
			}
			assertAntigravityLandedFiles(t, result, landed)
			if tc.mode == "before" {
				if !strings.Contains(err.Error(), "still in place") || !strings.Contains(err.Error(), "restored") {
					t.Fatalf("error must report the retained global ownership and the plugin restoration: %v", err)
				}
				raw := readAntigravityFile(t, mcpPath)
				if string(raw) != sole {
					t.Fatalf("global sole-entry config must be untouched before the real unlink\nwant:\n%s\ngot:\n%s", sole, raw)
				}
				// The global still owns Engram: no plugin asset may survive.
				for _, path := range pluginAssets {
					if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
						t.Fatalf("%q must be restored to absent while the global config still owns Engram; stat err = %v", path, statErr)
					}
				}
				return
			}
			// The real unlink happened before the fault: verify by readback.
			if !strings.Contains(err.Error(), "verified on disk to still provide the Engram registration") {
				t.Fatalf("error must report the verified plugin-side Engram availability: %v", err)
			}
			if _, statErr := os.Stat(mcpPath); !os.IsNotExist(statErr) {
				t.Fatalf("global sole-entry config must really be gone after the real unlink; stat err = %v", statErr)
			}
			for _, path := range pluginAssets {
				assertAntigravityCanonicalPluginAsset(t, path)
			}
		})
	}
}

// A present-but-unproven Engram value in the global config (null, scalar,
// array, or a non-managed object) is never classified as absent: nothing is
// restored — the plugin may hold the only registration — no availability claim.
func TestInjectAntigravityCleanupNeverRestoresForeignGlobalRegistration(t *testing.T) {
	for _, tc := range []struct{ name, foreign string }{
		{"null entry", `{"mcpServers":{"engram":null}}` + "\n"},
		{"scalar entry", `{"mcpServers":{"engram":"engram"}}` + "\n"},
		{"array entry", `{"mcpServers":{"engram":["engram"]}}` + "\n"},
		{"non-managed object entry", `{"mcpServers":{"engram":{"type":"remote"}}}` + "\n"},
		{"user wrapper entry", `{"mcpServers":{"engram":{"command":"/opt/user/engram-wrapper","args":["mcp"]}}}` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			cliDir, mcpPath, _ := seedAntigravitySoleEntry(t, home)
			orig := removeAntigravityGlobalFile
			removeAntigravityGlobalFile = func(path string) error {
				// Simulate a user rewrite between parse and failed removal.
				if err := os.WriteFile(path, []byte(tc.foreign), 0o644); err != nil {
					return err
				}
				return errAntigravityRemoveFault
			}
			t.Cleanup(func() { removeAntigravityGlobalFile = orig })
			_, err := Inject(home, antigravityAdapter())
			assertAntigravityFaults(t, err, errAntigravityRemoveFault)
			if !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), "no plugin asset was restored") ||
				strings.Contains(err.Error(), "remains available globally") || strings.Contains(err.Error(), "solely") {
				t.Fatalf("error must report uncertainty without restoration, availability, or exclusivity: %v", err)
			}
			if raw := readAntigravityFile(t, mcpPath); string(raw) != tc.foreign {
				t.Fatalf("foreign registration must be preserved byte-for-byte\nwant:\n%s\ngot:\n%s", tc.foreign, raw)
			}
			for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json"} {
				assertAntigravityCanonicalPluginAsset(t, filepath.Join(cliDir, "plugins", "gentle-ai-engram", name))
			}
		})
	}
}

func TestInjectAntigravityGlobalRemovalPrelandRestoresPluginBeforeImages(t *testing.T) {
	home, settingsPath, pluginDir, pluginPath, pluginMCPPath, hooksPath, unrelatedPath, before := antigravityRecoveryFixture(t)
	globalPath, global := seedAntigravityGlobalManagedDuplicate(t, home)
	failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "mcp_config.json"), false)
	result, err := Inject(home, antigravityAdapter())

	assertAntigravityFaults(t, err, errAntigravityWriteFault)
	if !strings.Contains(err.Error(), "still in place") || !strings.Contains(err.Error(), "restored") {
		t.Fatalf("error must report the retained global ownership and the plugin restoration: %v", err)
	}
	// The rewrite failed before replacement: the global is byte-identical.
	raw := readAntigravityFile(t, globalPath)
	if string(raw) != global {
		t.Fatalf("global MCP config must be untouched before the rewrite lands\nwant:\n%s\ngot:\n%s", global, raw)
	}
	// The plugin writes landed and were then restored to their exact
	// before-images: preexisting bytes and modes, absent for hooks.json.
	for _, path := range []string{pluginPath, pluginMCPPath, hooksPath, unrelatedPath} {
		assertAntigravityAssetState(t, "restored", path, before[path])
	}
	if _, statErr := os.Stat(pluginDir); statErr != nil {
		t.Fatalf("plugin directory must survive recovery; stat err = %v", statErr)
	} // Accounting: settings + plugin writes landed; the failed rewrite is not.
	assertAntigravityLandedFiles(t, result, []string{settingsPath, pluginPath, pluginMCPPath, hooksPath})
}

func TestInjectAntigravityGlobalRemovalPostlandTransfersOwnershipToPlugin(t *testing.T) {
	home, settingsPath, _, pluginPath, pluginMCPPath, hooksPath, _, before := antigravityRecoveryFixture(t)
	globalPath, _ := seedAntigravityGlobalManagedDuplicate(t, home)
	failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "mcp_config.json"), true)
	result, err := Inject(home, antigravityAdapter())

	assertAntigravityFaults(t, err, errAntigravityWriteFault)
	if !strings.Contains(err.Error(), "verified on disk to still provide the Engram registration") {
		t.Fatalf("error must report the verified plugin-side Engram availability: %v", err)
	}
	// The rewrite landed: the managed duplicate is gone and context7 is kept.
	raw := readAntigravityFile(t, globalPath)
	var cfg map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", globalPath, err)
	}
	if _, ok := cfg["mcpServers"]["engram"]; ok {
		t.Fatalf("landed rewrite must have removed the managed Engram entry; got:\n%s", raw)
	}
	if _, ok := cfg["mcpServers"]["context7"]; !ok {
		t.Fatalf("landed rewrite must preserve the unrelated context7 server; got:\n%s", raw)
	}
	// The transfer landed: the plugin keeps the canonical installed bytes —
	// restoring here would delete the only remaining registration.
	for _, path := range []string{pluginPath, pluginMCPPath, hooksPath} {
		assertAntigravityCanonicalPluginAsset(t, path)
	}
	if bytes.Equal(before[pluginPath].content, []byte(antigravityEngramPluginJSON)) {
		t.Fatalf("fixture guard: the preexisting manifest must differ from the canonical bytes for this proof")
	}
	// Accounting: settings, plugin writes, and the landed global rewrite.
	assertAntigravityLandedFiles(t, result, []string{settingsPath, pluginPath, pluginMCPPath, hooksPath, globalPath})
}

func TestInjectAntigravityOwnershipTransferUncertaintyIsExplicit(t *testing.T) {
	t.Run("unclassifiable global config blocks any ownership claim", func(t *testing.T) {
		home, settingsPath, _, pluginPath, pluginMCPPath, hooksPath, _, _ := antigravityRecoveryFixture(t)
		globalPath, _ := seedAntigravityGlobalManagedDuplicate(t, home)
		failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "mcp_config.json"), true)
		classifyFault := errors.New("injected antigravity classify fault")
		origClassify := classifyAntigravityGlobalManagedEntry
		classifyAntigravityGlobalManagedEntry = func(path string) (antigravityGlobalManagedState, error) {
			return antigravityGlobalManagedAbsent, classifyFault
		}
		t.Cleanup(func() { classifyAntigravityGlobalManagedEntry = origClassify })
		result, err := Inject(home, antigravityAdapter())
		assertAntigravityFaults(t, err, errAntigravityWriteFault, classifyFault)
		if !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), "no plugin asset was restored") {
			t.Fatalf("error must state the ownership outcome is uncertain and that nothing was restored: %v", err)
		}
		// The rewrite landed: nothing is touched — restoring here could
		// delete the only remaining registration.
		raw := readAntigravityFile(t, globalPath)
		if strings.Contains(string(raw), "/usr/local/bin/engram") || !strings.Contains(string(raw), "context7") {
			t.Fatalf("landed global rewrite must stay in place under uncertainty:\n%s", raw)
		}
		for _, path := range []string{pluginPath, pluginMCPPath, hooksPath} {
			assertAntigravityCanonicalPluginAsset(t, path)
		}
		assertAntigravityLandedFiles(t, result, []string{settingsPath, pluginPath, pluginMCPPath, hooksPath, globalPath})
	})

	t.Run("unverifiable plugin assets block a plugin-only claim", func(t *testing.T) {
		home, _, _, pluginPath, pluginMCPPath, hooksPath, _, _ := antigravityRecoveryFixture(t)
		globalPath, _ := seedAntigravityGlobalManagedDuplicate(t, home)
		failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "mcp_config.json"), true)
		readbackFault := errors.New("injected antigravity transfer readback fault")
		origRead := readAntigravityPluginAsset
		readAntigravityPluginAsset = func(path string) (antigravityAssetBackup, error) {
			return antigravityAssetBackup{}, readbackFault
		}
		t.Cleanup(func() { readAntigravityPluginAsset = origRead })
		_, err := Inject(home, antigravityAdapter())
		assertAntigravityFaults(t, err, errAntigravityWriteFault, readbackFault)
		if !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), "no plugin asset was restored") {
			t.Fatalf("error must state the ownership outcome is uncertain and that nothing was restored: %v", err)
		}
		// No restore under uncertainty: the plugin keeps the installed bytes.
		for _, path := range []string{pluginPath, pluginMCPPath, hooksPath} {
			assertAntigravityCanonicalPluginAsset(t, path)
		}
		raw := readAntigravityFile(t, globalPath)
		if strings.Contains(string(raw), "/usr/local/bin/engram") {
			t.Fatalf("landed global rewrite must stay in place under uncertainty:\n%s", raw)
		}
	})

	t.Run("failed restoration with the global entry retained is uncertain", func(t *testing.T) {
		home, _, _, pluginPath, pluginMCPPath, hooksPath, _, _ := antigravityRecoveryFixture(t)
		globalPath, global := seedAntigravityGlobalManagedDuplicate(t, home)
		failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "mcp_config.json"), false)
		restoreFault := errors.New("injected antigravity cleanup restore fault")
		origRestore := restoreAntigravityFileAtomic
		restoreAntigravityFileAtomic = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
			return filemerge.WriteResult{}, restoreFault
		}
		t.Cleanup(func() { restoreAntigravityFileAtomic = origRestore })
		_, err := Inject(home, antigravityAdapter())
		assertAntigravityFaults(t, err, errAntigravityWriteFault, restoreFault)
		if !strings.Contains(err.Error(), "could not be confirmed") {
			t.Fatalf("error must state the final ownership state could not be confirmed: %v", err)
		}
		// Restoration was refused: preexisting assets keep the canonical
		// bytes, hooks was still removable, the global keeps its ownership.
		for _, path := range []string{pluginPath, pluginMCPPath} {
			assertAntigravityCanonicalPluginAsset(t, path)
		}
		if _, statErr := os.Stat(hooksPath); !os.IsNotExist(statErr) {
			t.Fatalf("newly created hooks.json must still be removed by the partial restoration; stat err = %v", statErr)
		}
		raw := readAntigravityFile(t, globalPath)
		if string(raw) != global {
			t.Fatalf("global MCP config must stay untouched\nwant:\n%s\ngot:\n%s", global, raw)
		}
	})
}

// ─── #1635 A: concurrency drift guards on classify and rollback ─────────────

func TestInjectAntigravityClassifyDriftBlocksDestructiveRestore(t *testing.T) {
	home := t.TempDir()
	cliDir, _, _ := seedAntigravitySoleEntry(t, home)
	pluginMCPPath := filepath.Join(cliDir, "plugins", "gentle-ai-engram", "mcp_config.json")
	drift := []byte(`{"mcpServers":{"engram":{"command":"/external/engram","args":["mcp"]}}}` + "\n")
	classifyCalls := 0
	orig := classifyAntigravityGlobalManagedEntry
	classifyAntigravityGlobalManagedEntry = func(path string) (antigravityGlobalManagedState, error) {
		classifyCalls++
		state, err := orig(path)
		if classifyCalls == 1 && err == nil {
			// A concurrent writer replaces a freshly installed plugin asset
			// between the install and the ownership classification.
			if writeErr := os.WriteFile(pluginMCPPath, drift, 0o644); writeErr != nil {
				return state, errors.Join(err, writeErr)
			}
		}
		return state, err
	}
	t.Cleanup(func() { classifyAntigravityGlobalManagedEntry = orig })
	failAntigravityRemove(t, "before")

	_, err := Inject(home, antigravityAdapter())

	assertAntigravityFaults(t, err, errAntigravityRemoveFault)
	if !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), "no plugin asset was restored") {
		t.Fatalf("error must report uncertainty without restoration: %v", err)
	}
	if got := readAntigravityFile(t, pluginMCPPath); !bytes.Equal(got, drift) {
		t.Fatalf("drifted plugin asset must be preserved byte-for-byte, not clobbered by a stale before-image\nwant:\n%s\ngot:\n%s", drift, got)
	}
	// Nothing was restored: the untouched plugin assets keep their installed bytes.
	assertAntigravityCanonicalPluginAsset(t, filepath.Join(cliDir, "plugins", "gentle-ai-engram", "plugin.json"))
	assertAntigravityCanonicalPluginAsset(t, filepath.Join(cliDir, "plugins", "gentle-ai-engram", "hooks.json"))
}

func TestInjectAntigravityStaleGlobalRetirementPreventsPluginRestore(t *testing.T) {
	home := t.TempDir()
	cliDir, mcpPath, _ := seedAntigravitySoleEntry(t, home)
	retired := []byte(`{"mcpServers":{"context7":{"command":"npx"}}}` + "\n")
	classifyCalls := 0
	orig := classifyAntigravityGlobalManagedEntry
	classifyAntigravityGlobalManagedEntry = func(path string) (antigravityGlobalManagedState, error) {
		classifyCalls++
		if classifyCalls == 1 {
			return orig(path)
		}
		// A concurrent writer retires the managed global registration between
		// the classification and the pre-restore reread.
		if writeErr := os.WriteFile(path, retired, 0o644); writeErr != nil {
			return antigravityGlobalManagedAbsent, writeErr
		}
		return orig(path)
	}
	t.Cleanup(func() { classifyAntigravityGlobalManagedEntry = orig })
	failAntigravityRemove(t, "before")

	_, err := Inject(home, antigravityAdapter())

	assertAntigravityFaults(t, err, errAntigravityRemoveFault)
	if !strings.Contains(err.Error(), "uncertain") || !strings.Contains(err.Error(), "no plugin asset was restored") {
		t.Fatalf("error must report the concurrent retirement as uncertain without restoring: %v", err)
	}
	if got := readAntigravityFile(t, mcpPath); !bytes.Equal(got, retired) {
		t.Fatalf("concurrent global retirement must stay in place\nwant:\n%s\ngot:\n%s", retired, got)
	}
	// The stale before-images (absent) must NOT be restored over the plugin:
	// that would leave zero Engram registrations anywhere.
	for _, name := range []string{"plugin.json", "mcp_config.json", "hooks.json"} {
		assertAntigravityCanonicalPluginAsset(t, filepath.Join(cliDir, "plugins", "gentle-ai-engram", name))
	}
}

func TestInjectAntigravityRollbackPreservesDriftedPluginAsset(t *testing.T) {
	home, _, _, pluginPath, pluginMCPPath, hooksPath, _, before := antigravityRecoveryFixture(t)
	drift := []byte(`{"external":"drift"}` + "\n")
	drifted := false
	origRead := readAntigravityPluginAsset
	readAntigravityPluginAsset = func(path string) (antigravityAssetBackup, error) {
		if !drifted && path == pluginMCPPath {
			drifted = true
			if writeErr := os.WriteFile(pluginMCPPath, drift, 0o600); writeErr != nil {
				return antigravityAssetBackup{}, writeErr
			}
		}
		return origRead(path)
	}
	t.Cleanup(func() { readAntigravityPluginAsset = origRead })
	restored := map[string]bool{}
	origRestore := restoreAntigravityFileAtomic
	restoreAntigravityFileAtomic = func(path string, content []byte, perm fs.FileMode) (filemerge.WriteResult, error) {
		restored[path] = true
		return origRestore(path, content, perm)
	}
	t.Cleanup(func() { restoreAntigravityFileAtomic = origRestore })
	failAntigravityWrite(t, filepath.Join(".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "hooks.json"), true)

	_, err := Inject(home, antigravityAdapter())

	assertAntigravityFaults(t, err, errAntigravityWriteFault)
	if !strings.Contains(err.Error(), "uncertain") {
		t.Fatalf("error must report the drifted asset as uncertain: %v", err)
	}
	// The drifted asset keeps the concurrent writer's bytes and is never
	// restored from the stale before-image.
	if got := readAntigravityFile(t, pluginMCPPath); !bytes.Equal(got, drift) {
		t.Fatalf("drifted plugin asset must be preserved byte-for-byte\nwant:\n%s\ngot:\n%s", drift, got)
	}
	if restored[pluginMCPPath] {
		t.Fatalf("the drifted plugin asset must not be restored from its stale before-image")
	}
	// Assets that still match what this pass wrote are restored normally.
	assertAntigravityAssetState(t, "restored", pluginPath, before[pluginPath])
	if _, statErr := os.Stat(hooksPath); !os.IsNotExist(statErr) {
		t.Fatalf("hooks.json must be restored to its absent before-image; stat err = %v", statErr)
	}
}

func TestEngramSelectedSettingsRefuseNestedCommentsAndLockedMode(t *testing.T) {

	for _, tc := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{"nested comments", "{\"mcp\":{\"other\":{/* keep */\"type\":\"remote\"}}}\n", 0o600},
		{"locked mode", "{\"mcp\":{}}\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.mode != 0o600 {
				t.Skip("file permission bits are not supported on Windows")
			}
			path := filepath.Join(t.TempDir(), "opencode.jsonc")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := InjectWithOptions(t.TempDir(), opencodeAdapter(), InjectOptions{OpenCodeSettingsPath: path})
			if err == nil || !strings.Contains(err.Error(), "refuse") {
				t.Fatalf("want actionable refusal, got %v", err)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != tc.mode {
				t.Fatalf("settings mode changed: %04o", info.Mode().Perm())
			}
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != tc.content {
				t.Fatalf("settings bytes changed: %q", got)
			}
		})
	}
}

func TestEngramSelectedSettingsPreservePrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.jsonc")
	if err := os.WriteFile(path, []byte("{\"mcp\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeJSONFile(path, []byte(`{"mcp":{"engram":{"type":"local"}}}`)); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return // POSIX permission bits are not preserved on Windows.
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode = %v, error = %v; want 0600", info, err)
	}
}

// The selected-settings refusal is OpenCode-only; the shared merge helper keeps
// the base writer behavior for other agents' dotfiles-managed (symlinked) files.
func TestSharedMergeKeepsBaseWriterBehaviorForSymlinkedSettings(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles.json")
	settings := filepath.Join(dir, "settings.json")
	original := []byte("{\"mcpServers\":{}}\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, settings); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := mergeJSONFile(settings, []byte(`{"mcpServers":{"engram":{"command":"engram"}}}`))
	if err == nil || !strings.Contains(err.Error(), "refusing to read symlink") || strings.Contains(err.Error(), "select a regular settings file") {
		t.Fatalf("mergeJSONFile() error = %v; want base writer symlink error, not the OpenCode refusal", err)
	}
	if link, err := os.Readlink(settings); err != nil || link != target {
		t.Fatalf("settings symlink changed: %q, %v", link, err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("settings target changed: %q, %v", got, err)
	}
}

func TestEngramSelectedSettingsRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "user.jsonc")
	selected := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(target, []byte("{\"mcp\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, selected); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := InjectWithOptions(t.TempDir(), opencodeAdapter(), InjectOptions{OpenCodeSettingsPath: selected})
	if err == nil || !strings.Contains(err.Error(), "select a regular settings file") {
		t.Fatalf("InjectWithOptions() error = %v; want OpenCode symlink refusal", err)
	}
}

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }
func codexAdapter() agents.Adapter    { return codex.NewAdapter() }

func validCodexRuntime(t *testing.T) {
	t.Helper()
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil))
}
func geminiAdapter() agents.Adapter   { return gemini.NewAdapter() }
func hermesAdapter() agents.Adapter   { return hermes.NewAdapter() }
func qwenAdapter() agents.Adapter     { return qwen.NewAdapter() }
func openclawAdapter() agents.Adapter { return openclaw.NewAdapter() }
func antigravityAdapter() agents.Adapter {
	return antigravity.NewAdapter()
}

func piAdapter() agents.Adapter { return pi.NewAdapter() }

// assertArgsHaveToolsAgent is a shared helper that validates a JSON file
// contains the MCP "engram" entry with --tools=agent in args.
func assertArgsHaveToolsAgent(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	text := string(content)
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatalf("file %q missing --tools=agent in args; got:\n%s", path, text)
	}
}

func TestInjectClaudeWritesUserRegistryPreservingUnrelatedData(t *testing.T) {
	home := t.TempDir()
	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")
	registryPath := claude.UserConfigPath(home)
	seed := []byte(`{"oauthAccount":{"emailAddress":"user@example.com"},"projects":{"/repo":{"allowedTools":[]}},"mcpServers":{"codegraph":{"command":"codegraph"}}}`)
	if err := os.WriteFile(registryPath, seed, 0o644); err != nil {
		t.Fatalf("WriteFile(user registry) error = %v", err)
	}

	result, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	registry := readJSONFile(t, registryPath)
	assertNestedString(t, registry, "user@example.com", "oauthAccount", "emailAddress")
	assertNestedString(t, registry, "codegraph", "mcpServers", "codegraph", "command")
	assertNestedString(t, registry, "/opt/homebrew/bin/engram", "mcpServers", "engram", "command")
	assertNestedStrings(t, registry, []string{"mcp", "--tools=agent"}, "mcpServers", "engram", "args")
	if info, statErr := os.Stat(registryPath); statErr != nil {
		t.Fatalf("Stat(user registry) error = %v", statErr)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("user registry mode = %o; want 0600", info.Mode().Perm())
	}
	legacyPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if _, statErr := os.Stat(legacyPath); !os.IsNotExist(statErr) {
		t.Fatalf("fresh injection must not create legacy config %q; stat error = %v", legacyPath, statErr)
	}
}

func TestInjectClaudeRefusesCorruptUserRegistryWithoutMutation(t *testing.T) {
	home := t.TempDir()
	registryPath := claude.UserConfigPath(home)
	corrupt := []byte("{ not json")
	if err := os.WriteFile(registryPath, corrupt, 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt registry) error = %v", err)
	}

	if _, err := Inject(home, claudeAdapter()); err == nil {
		t.Fatal("Inject() error = nil; want corrupt registry refusal")
	}
	after, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(corrupt registry) error = %v", err)
	}
	if !bytes.Equal(after, corrupt) {
		t.Fatalf("corrupt registry mutated: got %q, want %q", after, corrupt)
	}
}

func TestInjectClaudeWritesProtocolSection(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatal("CLAUDE.md missing open marker for engram-protocol")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:engram-protocol -->") {
		t.Fatal("CLAUDE.md missing close marker for engram-protocol")
	}
	// Real content check.
	if !strings.Contains(text, "mem_save") {
		t.Fatal("CLAUDE.md missing real engram protocol content (expected 'mem_save')")
	}
	if !strings.Contains(text, "needs_review") {
		t.Fatal("CLAUDE.md missing memory lifecycle stale-context rule (expected 'needs_review')")
	}
}

func TestInjectClaudeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}
	registryPath := claude.UserConfigPath(home)
	afterFirst, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(user registry after first injection) error = %v", err)
	}

	second, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
	afterSecond, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(user registry after second injection) error = %v", err)
	}
	if !bytes.Equal(afterFirst, afterSecond) {
		t.Fatal("second injection must leave the user registry byte-identical")
	}
}

func TestInjectOpenCodeMergesEngramToSettings(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	// Should include opencode.json and AGENTS.md (fallback protocol injection).
	if len(result.Files) != 2 {
		t.Fatalf("Inject() files = %v, want exactly 2 (opencode.json + AGENTS.md)", result.Files)
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(config)
	if !strings.Contains(text, `"engram"`) {
		t.Fatal("opencode.json missing engram server entry")
	}
	if !strings.Contains(text, `"mcp"`) {
		t.Fatal("opencode.json missing mcp key")
	}
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatal("opencode.json should use 'mcp' key, not 'mcpServers'")
	}
	if !strings.Contains(text, `"type": "local"`) {
		t.Fatal("opencode.json engram missing type: local")
	}
	// OpenCode 1.3.3+: command must be an array, no separate "args" field.
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatal("opencode.json missing --tools=agent in command array")
	}
	if strings.Contains(text, `"args"`) {
		t.Fatal("opencode.json must NOT have a separate args field — command must be an array")
	}

	// Verify NO plugin files or plugin arrays exist.
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "engram.ts")
	if _, err := os.Stat(pluginPath); err == nil {
		t.Fatal("plugin file should NOT exist — old approach removed")
	}
	if strings.Contains(text, `"plugins"`) {
		t.Fatal("opencode.json should NOT contain plugins key")
	}

	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	agentsText := string(agentsContent)
	if !strings.Contains(agentsText, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatal("AGENTS.md missing engram protocol section marker")
	}
	if !strings.Contains(agentsText, "mem_save") {
		t.Fatal("AGENTS.md missing engram protocol content (expected 'mem_save')")
	}
}

func TestInjectOpenCodeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
}

func TestInjectPiProvisioningWritesNothingOnFreshHome(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, piAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("Inject() = (changed %v, files %v), want no writes (Pi Engram is native-only, not MCP)", result.Changed, result.Files)
	}
	for _, path := range []string{
		filepath.Join(home, ".pi", "agent", "settings.json"),
		filepath.Join(home, ".pi", "agent", "npm", "package.json"),
		filepath.Join(home, ".pi", "agent", "mcp.json"),
		filepath.Join(home, ".pi", "agent", "mcp-adapter.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stat %q err = %v, want IsNotExist", path, err)
		}
	}
}

func TestInjectPiProvisioningMigratesMCPAdapterServersWithoutEngram(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	writeFile(t, filepath.Join(home, ".pi", "agent", "mcp-adapter.json"), `{"mcpServers":{"context7":{"command":"npx"}}}`)

	result, err := Inject(home, piAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed || len(result.Files) != 1 || result.Files[0] != mcpPath {
		t.Fatalf("Inject() = (changed %v, files %v), want only %q written", result.Changed, result.Files, mcpPath)
	}
	config := readJSONFile(t, mcpPath)
	assertNestedString(t, config, "npx", "mcpServers", "context7", "command")
	if servers, _ := config["mcpServers"].(map[string]any); servers["engram"] != nil {
		t.Fatalf("mcp.json servers = %#v, want no engram server (Pi Engram is native-only)", servers)
	}
}

func TestInjectPiProvisioningRetiresMCPAdapterAndPreservesUnrelatedContent(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{"theme":"kanagawa","packages":["npm:other@1.0.0","npm:pi-mcp-adapter@2.0.0"]}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"), `{"name":"pi-user","dependencies":{"left-pad":"^1.0.0","pi-mcp-adapter":"^2.6.0"},"devDependencies":{"vitest":"^1.0.0"}}`)

	first, err := Inject(home, piAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() changed = false, want the adapter retired")
	}

	settings := readJSONFile(t, filepath.Join(home, ".pi", "agent", "settings.json"))
	assertNestedString(t, settings, "kanagawa", "theme")
	assertNestedStrings(t, settings, []string{"npm:other@1.0.0"}, "packages")

	npmPackage := readJSONFile(t, filepath.Join(home, ".pi", "agent", "npm", "package.json"))
	assertNestedString(t, npmPackage, "pi-user", "name")
	assertNestedString(t, npmPackage, "^1.0.0", "dependencies", "left-pad")
	assertNestedMissing(t, npmPackage, "dependencies", "pi-mcp-adapter")
	assertNestedString(t, npmPackage, "^1.0.0", "devDependencies", "vitest")

	second, err := Inject(home, piAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true, want idempotent no-op")
	}
}

func TestInjectPiProvisioningMigratesLegacyObjectPackages(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{"theme":"kanagawa","packages":{"npm:other":"1.0.0","npm:pi-mcp-adapter":"2.0.0"}}`)

	_, err := Inject(home, piAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settings := readJSONFile(t, filepath.Join(home, ".pi", "agent", "settings.json"))
	assertNestedString(t, settings, "kanagawa", "theme")
	assertNestedStrings(t, settings, []string{"npm:other@1.0.0"}, "packages")
}

// TestInjectOpenCodeMigratesFromOldFormat verifies that when a user's
// opencode.json contains the old v1.11.3 format (separate "args" key),
// Inject() replaces mcp.engram atomically so that "args" is absent and
// "command" is an array — the format required by OpenCode 1.3.3+.
func TestInjectOpenCodeMigratesFromOldFormat(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")

	adapter := opencodeAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	// Pre-seed with the old v1.11.3 format.
	oldFormat := `{"mcp": {"engram": {"command": "/opt/homebrew/bin/engram", "args": ["mcp","--tools=agent"], "type": "local"}}}`
	if err := os.WriteFile(configPath, []byte(oldFormat), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false; expected migration to produce a change")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	// (1) "args" key must be absent from mcp.engram.
	if strings.Contains(string(content), `"args"`) {
		t.Fatalf("mcp.engram still contains 'args' key after migration; got:\n%s", content)
	}

	// (2) command must be a []any containing the engram binary.
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}
	mcpMap, _ := parsed["mcp"].(map[string]any)
	engramMap, _ := mcpMap["engram"].(map[string]any)
	cmdRaw, ok := engramMap["command"]
	if !ok {
		t.Fatalf("mcp.engram missing command key; got:\n%s", content)
	}
	cmdArr, ok := cmdRaw.([]any)
	if !ok {
		t.Fatalf("mcp.engram.command must be []any after migration, got %T; got:\n%s", cmdRaw, content)
	}
	if len(cmdArr) == 0 {
		t.Fatalf("mcp.engram.command array is empty; got:\n%s", content)
	}
	firstElem, _ := cmdArr[0].(string)
	if firstElem == "" {
		t.Fatalf("mcp.engram.command[0] is empty or not a string; got:\n%s", content)
	}
	// Must end with "engram".
	if filepath.Base(firstElem) != "engram" {
		t.Fatalf("mcp.engram.command[0] = %q does not end with 'engram'; got:\n%s", firstElem, content)
	}

	// (3) Second Inject() call must be idempotent (changed=false).
	second, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true; expected idempotent (no change)")
	}
}

func TestInjectOpenCodeMigratesCellarEngramCommandToStablePath(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")

	adapter := opencodeAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	oldFormat := `{"mcp": {"engram": {"command": ["/opt/homebrew/Cellar/engram/1.14.1/bin/engram", "mcp", "--tools=agent"], "type": "local"}}}`
	if err := os.WriteFile(configPath, []byte(oldFormat), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false; expected Cellar command migration")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(content)
	if strings.Contains(text, "/Cellar/") {
		t.Fatalf("opencode.json still contains versioned Homebrew Cellar path; got:\n%s", text)
	}
	if !strings.Contains(text, "/opt/homebrew/bin/engram") {
		t.Fatalf("opencode.json did not migrate to stable Homebrew symlink; got:\n%s", text)
	}

	second, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true; expected idempotent Cellar migration")
	}
}

func TestInjectCursorMergesEngramToSettings(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(cursor) error = %v", injectErr)
	}

	// Cursor uses MCPConfigFile strategy — engram gets merged into mcp.json.
	if !result.Changed {
		t.Fatalf("Inject(cursor) changed = false")
	}
}

func TestInjectCursorWithMalformedMCPJsonRecovery(t *testing.T) {
	// Real Windows users may have a ~/.cursor/mcp.json that starts with non-JSON
	// content (e.g. "allow: all" or just "a"). The installer should recover by
	// treating the broken file as {} and proceeding with the overlay merge.
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	// Pre-create ~/.cursor/mcp.json with invalid (non-JSON) content.
	mcpPath := cursorAdapter.MCPConfigPath(home, "engram")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(mcpPath, []byte("allow: all"), 0o644); err != nil {
		t.Fatalf("WriteFile(malformed mcp.json) error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(cursor) with malformed mcp.json error = %v; want nil (should recover)", injectErr)
	}
	if !result.Changed {
		t.Fatalf("Inject(cursor) changed = false; want true")
	}

	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(mcp.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"mcpServers"`) {
		t.Fatalf("mcp.json missing mcpServers key after recovery; got:\n%s", text)
	}
	if !strings.Contains(text, `"engram"`) {
		t.Fatalf("mcp.json missing engram server after recovery; got:\n%s", text)
	}
}

func TestInjectVSCodeMergesEngramToMCPConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	adapter := vscode.NewAdapter()

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject(vscode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(vscode) changed = false")
	}

	mcpPath := adapter.MCPConfigPath(home, "engram")
	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(mcp.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"servers"`) {
		t.Fatal("mcp.json missing servers key")
	}
	if !strings.Contains(text, `"engram"`) {
		t.Fatal("mcp.json missing engram server")
	}
	if !strings.Contains(text, `"mcp"`) {
		t.Fatal("mcp.json missing engram args mcp")
	}
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatal("mcp.json should use 'servers' key, not 'mcpServers'")
	}
	// RED: VS Code overlay must include --tools=agent
	assertArgsHaveToolsAgent(t, mcpPath)
}

// ─── Gemini tests ─────────────────────────────────────────────────────────────

func TestInjectGeminiToolsFlagPresent(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, geminiAdapter())
	if err != nil {
		t.Fatalf("Inject(gemini) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(gemini) changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `"mcpServers"`) {
		t.Fatal("settings.json missing mcpServers key")
	}
	if !strings.Contains(text, `"engram"`) {
		t.Fatal("settings.json missing engram entry")
	}
	// RED: Gemini overlay must use --tools=agent
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatal("settings.json missing --tools=agent in args")
	}
}

func TestInjectAntigravityRegistersEngramViaPluginOnly(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(antigravity) changed = false")
	}

	// #797: Engram registration is plugin-owned only; the global
	// ~/.gemini/antigravity-cli/mcp_config.json is never written for Engram.
	cliMCPPath := filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json")
	if _, err := os.Stat(cliMCPPath); !os.IsNotExist(err) {
		t.Fatalf("global Antigravity MCP config %q must not be written for Engram; stat err = %v", cliMCPPath, err)
	}

	pluginPath := filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "plugin.json")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("Antigravity Engram plugin manifest missing: %v", err)
	}

	// #797: the plugin MCP config must use the canonical Engram agent tool
	// profile (args ["mcp", "--tools=agent"]).
	pluginMCPPath := filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json")
	pluginMCPContent, err := os.ReadFile(pluginMCPPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", pluginMCPPath, err)
	}
	var pluginCfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(pluginMCPContent, &pluginCfg); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", pluginMCPPath, err)
	}
	server, ok := pluginCfg.MCPServers["engram"]
	if !ok || server.Command == "" {
		t.Fatalf("Antigravity Engram plugin MCP config must register the engram server; got:\n%s", pluginMCPContent)
	}
	if len(server.Args) != 2 || server.Args[0] != "mcp" || server.Args[1] != "--tools=agent" {
		t.Fatalf("Antigravity Engram plugin MCP config args = %v, want [mcp --tools=agent]; got:\n%s", server.Args, pluginMCPContent)
	}

	hooksPath := filepath.Join(home, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "hooks.json")
	hooksContent, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", hooksPath, err)
	}
	hooksText := string(hooksContent)
	for _, want := range []string{
		"PreInvocation",
		"injectSteps",
		"mem_save",
		"mem_search",
		"mem_context",
		"mem_session_summary",
		"mem_get_observation",
		"mem_current_project",
		"mem_judge",
		"optional mem_review",
		"if mem_review is unavailable",
		// #1903: tools still load first, but the initial read is project-gated.
		"CRITICAL FIRST ACTION",
		"before any initial mem_context, follow the Engram startup protocol",
		"pass only the resolved project to mem_context",
	} {
		if !strings.Contains(hooksText, want) {
			t.Fatalf("Antigravity Engram hook missing %q; got:\n%s", want, hooksText)
		}
	}
	if strings.Contains(hooksText, "Then call mem_context when") {
		t.Fatalf("Antigravity Engram hook still requests an ungated initial mem_context; got:\n%s", hooksText)
	}

	desktopMCPPath := filepath.Join(home, ".gemini", "antigravity", "mcp_config.json")
	if _, err := os.Stat(desktopMCPPath); !os.IsNotExist(err) {
		t.Fatalf("legacy desktop MCP path %q should not be written for antigravity; stat err = %v", desktopMCPPath, err)
	}
}

func TestInjectAntigravityRemovesManagedGlobalEngramDuplicate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		args    string
	}{{
		name:    "default invocation",
		command: "/custom/bin/engram",
		args:    `["mcp"]`,
	}, {
		name:    "legacy agent tool profile",
		command: "/usr/local/bin/engram",
		args:    `["mcp", "--tools=agent"]`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
			mcpPath := filepath.Join(cliDir, "mcp_config.json")
			if err := os.MkdirAll(cliDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) error = %v", cliDir, err)
			}
			global := fmt.Sprintf(`{
  "mcpServers": {
    "context7": {"command": "npx", "args": ["-y", "@upstash/context7-mcp"]},
    "engram": {"command": %q, "args": %s}
  }
}
`, tc.command, tc.args)
			if err := os.WriteFile(mcpPath, []byte(global), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
			}

			result, err := Inject(home, antigravityAdapter())
			if err != nil {
				t.Fatalf("Inject(antigravity) error = %v", err)
			}
			if !result.Changed {
				t.Fatalf("Inject(antigravity) changed = false")
			}

			// The duplicate global Engram owner is removed; unrelated servers
			// in the same file are preserved untouched.
			raw, err := os.ReadFile(mcpPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", mcpPath, err)
			}
			var cfg struct {
				MCPServers map[string]json.RawMessage `json:"mcpServers"`
			}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				t.Fatalf("Unmarshal(%q) error = %v", mcpPath, err)
			}
			if _, ok := cfg.MCPServers["engram"]; ok {
				t.Fatalf("duplicate global Engram entry must be removed; got:\n%s", raw)
			}
			if _, ok := cfg.MCPServers["context7"]; !ok {
				t.Fatalf("unrelated context7 server must be preserved; got:\n%s", raw)
			}

			// The plugin carries the canonical agent tool profile.
			pluginMCPPath := filepath.Join(cliDir, "plugins", "gentle-ai-engram", "mcp_config.json")
			pluginRaw, err := os.ReadFile(pluginMCPPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", pluginMCPPath, err)
			}
			if !strings.Contains(string(pluginRaw), "--tools=agent") {
				t.Fatalf("plugin MCP config must use --tools=agent; got:\n%s", pluginRaw)
			}
			if !strings.Contains(string(pluginRaw), tc.command) {
				t.Fatalf("plugin MCP config must preserve selected command %q; got:\n%s", tc.command, pluginRaw)
			}

			second, err := Inject(home, antigravityAdapter())
			if err != nil {
				t.Fatalf("second Inject(antigravity) error = %v", err)
			}
			if second.Changed {
				t.Fatalf("second Inject(antigravity) changed = true, want false")
			}
			secondRaw, err := os.ReadFile(mcpPath)
			if err != nil {
				t.Fatalf("second ReadFile(%q) error = %v", mcpPath, err)
			}
			if string(secondRaw) != string(raw) {
				t.Fatalf("global MCP config changed on second injection\nfirst:\n%s\nsecond:\n%s", raw, secondRaw)
			}
			secondPluginRaw, err := os.ReadFile(pluginMCPPath)
			if err != nil {
				t.Fatalf("second ReadFile(%q) error = %v", pluginMCPPath, err)
			}
			if string(secondPluginRaw) != string(pluginRaw) {
				t.Fatalf("plugin MCP config changed on second injection\nfirst:\n%s\nsecond:\n%s", pluginRaw, secondPluginRaw)
			}
		})
	}
}

func TestInjectAntigravityIgnoresNonEngramGlobalCommandWhenPreservingPlugin(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	mcpPath := filepath.Join(cliDir, "mcp_config.json")
	pluginDir := filepath.Join(cliDir, "plugins", "gentle-ai-engram")
	pluginMCPPath := filepath.Join(pluginDir, "mcp_config.json")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", pluginDir, err)
	}
	global := `{
  "mcpServers": {
    "engram": {"command": "npx", "args": ["-y", "not-engram"]}
  }
}
`
	if err := os.WriteFile(mcpPath, []byte(global), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
	}
	if err := os.WriteFile(filepath.Join(cliDir, "settings.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(settings.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(antigravityEngramPluginJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json) error = %v", err)
	}
	pluginRaw := engramOverlayJSON(model.AgentAntigravity, "/custom/bin/engram")
	if err := os.WriteFile(pluginMCPPath, pluginRaw, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", pluginMCPPath, err)
	}
	hooksRaw := antigravityEngramHooksJSON()
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks.json"), hooksRaw, 0o644); err != nil {
		t.Fatalf("WriteFile(hooks.json) error = %v", err)
	}

	if _, err := Inject(home, antigravityAdapter()); err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}
	gotGlobal, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", mcpPath, err)
	}
	if string(gotGlobal) != global {
		t.Fatalf("non-Engram global MCP entry changed\nwant:\n%s\ngot:\n%s", global, gotGlobal)
	}
	gotPlugin, err := os.ReadFile(pluginMCPPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", pluginMCPPath, err)
	}
	if string(gotPlugin) != string(pluginRaw) {
		t.Fatalf("plugin MCP config should preserve existing Engram command when global command is not Engram\nwant:\n%s\ngot:\n%s", pluginRaw, gotPlugin)
	}
}

func TestInjectAntigravityRemovesFileContainingOnlyManagedEngram(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	mcpPath := filepath.Join(cliDir, "mcp_config.json")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", cliDir, err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"engram":{"command":"/usr/local/bin/engram","args":["mcp"]}}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
	}

	if _, err := Inject(home, antigravityAdapter()); err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}

	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Fatalf("global MCP config holding only the managed Engram entry should be removed; stat err = %v", err)
	}
}

func TestInjectAntigravityLeavesUserModifiedGlobalEngramUntouched(t *testing.T) {
	home := t.TempDir()
	cliDir := filepath.Join(home, ".gemini", "antigravity-cli")
	mcpPath := filepath.Join(cliDir, "mcp_config.json")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", cliDir, err)
	}
	global := `{
  "mcpServers": {
    "engram": {"command": "/opt/engram/bin/engram", "args": ["mcp", "--profile=custom"]}
  }
}
`
	if err := os.WriteFile(mcpPath, []byte(global), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", mcpPath, err)
	}

	if _, err := Inject(home, antigravityAdapter()); err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}

	raw, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", mcpPath, err)
	}
	if string(raw) != global {
		t.Fatalf("user-modified global Engram entry must be preserved untouched; got:\n%s", raw)
	}
}

func TestStableEngramCommandRecognizesLinuxbrewPath(t *testing.T) {
	SetLookPathForTest(t, "/home/linuxbrew/.linuxbrew/bin/engram", "")

	if !isStableHomebrewEngramPath("/home/linuxbrew/.linuxbrew/bin/engram") {
		t.Fatalf("Linuxbrew stable path must be recognized as a stable Homebrew engram path")
	}
	if got := preferredStableEngramCommand(); got != "/home/linuxbrew/.linuxbrew/bin/engram" {
		t.Fatalf("preferredStableEngramCommand() = %q, want the Linuxbrew stable path", got)
	}

	// Standard agents (including Antigravity plugin install) resolve the
	// stable command through stableEngramCommandForMergedConfig when no prior
	// config exists — the Linuxbrew stable path must be preserved there too.
	home := t.TempDir()
	got := stableEngramCommandForMergedConfig(filepath.Join(home, "missing", "mcp_config.json"), model.AgentAntigravity)
	if got != "/home/linuxbrew/.linuxbrew/bin/engram" {
		t.Fatalf("stableEngramCommandForMergedConfig(antigravity) = %q, want the Linuxbrew stable path", got)
	}
}

func TestInjectAntigravityInitializesEmptySettingsWhenGeminiMissing(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(antigravity) first changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", settingsPath, err)
	}
	if strings.TrimSpace(string(got)) != "{}" {
		t.Fatalf("antigravity settings = %q, want empty JSON object", got)
	}

	second, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(antigravity) second changed = true; want false")
	}
}

// ─── Codex tests ──────────────────────────────────────────────────────────────

func TestInjectCodexWritesTOMLMCP(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	result, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(codex) changed = false")
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[mcp_servers.engram]") {
		t.Fatalf("config.toml missing [mcp_servers.engram] block; got:\n%s", text)
	}
	// command must reference the engram binary — either relative ("engram") or an
	// absolute path (when engram is on PATH). Both are valid.
	if !strings.Contains(text, "command = ") {
		t.Fatalf("config.toml missing command field; got:\n%s", text)
	}
	cmdLine := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "command = ") {
			cmdLine = strings.TrimSpace(line)
			break
		}
	}
	if cmdLine == "" {
		t.Fatalf("config.toml missing command line; got:\n%s", text)
	}
	// The command value must end with "engram" or "engram.exe".
	cmdVal := strings.TrimPrefix(cmdLine, "command = ")
	cmdVal = strings.Trim(cmdVal, `"`)
	base := filepath.Base(cmdVal)
	if base != "engram" && base != "engram.exe" {
		t.Fatalf("config.toml command %q does not reference engram binary; got:\n%s", cmdVal, text)
	}
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatalf("config.toml missing --tools=agent; got:\n%s", text)
	}
}

func TestInjectCodexWritesInstructionFiles(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	_, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	instructionsPath := filepath.Join(home, ".codex", "engram-instructions.md")
	content, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("ReadFile(engram-instructions.md) error = %v", err)
	}
	if !strings.Contains(string(content), "mem_save") {
		t.Fatal("engram-instructions.md missing expected content (mem_save)")
	}
	if !strings.Contains(string(content), "needs_review") {
		t.Fatal("engram-instructions.md missing memory lifecycle stale-context rule (needs_review)")
	}

	compactPath := filepath.Join(home, ".codex", "engram-compact-prompt.md")
	compactContent, err := os.ReadFile(compactPath)
	if err != nil {
		t.Fatalf("ReadFile(engram-compact-prompt.md) error = %v", err)
	}
	if !strings.Contains(string(compactContent), "FIRST ACTION REQUIRED") {
		t.Fatal("engram-compact-prompt.md missing expected content (FIRST ACTION REQUIRED)")
	}
}

func TestInjectCodexInjectsTOMLKeys(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	_, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)

	instructionsPath := filepath.Join(home, ".codex", "engram-instructions.md")
	if !strings.Contains(text, `model_instructions_file`) {
		t.Fatalf("config.toml missing model_instructions_file key; got:\n%s", text)
	}
	normText := strings.ReplaceAll(strings.ReplaceAll(text, "\\\\", "/"), "\\", "/")
	normInstrPath := filepath.ToSlash(instructionsPath)
	if !strings.Contains(normText, normInstrPath) {
		t.Fatalf("config.toml model_instructions_file does not reference %q; got:\n%s", instructionsPath, text)
	}

	compactPath := filepath.Join(home, ".codex", "engram-compact-prompt.md")
	if !strings.Contains(text, `experimental_compact_prompt_file`) {
		t.Fatalf("config.toml missing experimental_compact_prompt_file key; got:\n%s", text)
	}
	normCompactPath := filepath.ToSlash(compactPath)
	if !strings.Contains(normText, normCompactPath) {
		t.Fatalf("config.toml experimental_compact_prompt_file does not reference %q; got:\n%s", compactPath, text)
	}
}

// ─── Engram setup absolute path preservation tests ────────────────────────────

// TestInjectClaudePreservesAbsoluteCommandFromEngramSetup verifies that when
// `engram setup claude-code` has already written an absolute-path command to
// the managed legacy file, Inject() migrates it into Claude's user registry.
func TestInjectClaudePreservesAbsoluteCommandFromEngramSetup(t *testing.T) {
	home := t.TempDir()

	// Simulate what `engram setup claude-code` writes on v1.10.3+:
	// an absolute path as the command value.
	absPath := "/opt/homebrew/bin/engram"
	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	setupContent := []byte(`{
  "command": "/opt/homebrew/bin/engram",
  "args": ["mcp", "--tools=agent"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	// Now run Inject — should NOT overwrite the absolute command.
	_, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	registryPath := claude.UserConfigPath(home)
	content, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(user registry) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, absPath) {
		t.Fatalf("Inject() overwrote absolute command path; want %q preserved, got:\n%s", absPath, text)
	}
	assertArgsHaveToolsAgent(t, registryPath)
	if _, statErr := os.Stat(mcpPath); !os.IsNotExist(statErr) {
		t.Fatalf("managed legacy file must be removed after migration; stat error = %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Dir(mcpPath)); !os.IsNotExist(statErr) {
		t.Fatalf("empty managed legacy directory must be removed; stat error = %v", statErr)
	}
}

// TestInjectClaudeSkipsMCPServersEngramWhenPluginEnabled reproduces issue
// #4188: gentle-ai sync must not add mcpServers.engram to ~/.claude.json
// when the Engram plugin is already enabled via
// ~/.claude/settings.json's enabledPlugins["engram@engram"], because the
// plugin already exposes the same 18 tools under a different prefix.
func TestInjectClaudeSkipsMCPServersEngramWhenPluginEnabled(t *testing.T) {
	home := t.TempDir()
	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")
	writeClaudeEngramPluginEnabled(t, home)

	registryPath := claude.UserConfigPath(home)
	registrySeed := []byte(`{"mcpServers":{"context7":{"command":"npx"}}}`)
	if err := os.WriteFile(registryPath, registrySeed, 0o644); err != nil {
		t.Fatal(err)
	}

	// Repeated syncs must leave the user registry byte-for-byte unchanged.
	for run := 1; run <= 2; run++ {
		if _, err := Inject(home, claudeAdapter()); err != nil {
			t.Fatalf("Inject() run %d error = %v", run, err)
		}
		if got, err := os.ReadFile(registryPath); err != nil || !bytes.Equal(got, registrySeed) {
			t.Fatalf("Inject() run %d changed registry: got=%q error=%v", run, got, err)
		}
	}

	registry := readJSONFile(t, registryPath)
	mcpServers, _ := registry["mcpServers"].(map[string]any)
	if _, exists := mcpServers["engram"]; exists {
		t.Fatalf("mcpServers.engram must not be added when the Engram plugin is enabled; registry = %#v", registry)
	}
	assertNestedString(t, registry, "npx", "mcpServers", "context7", "command")
}

// TestInjectClaudePreservesIdenticalManualRegistrationsWhenPluginEnabled
// verifies that sync never infers ownership from an Engram registration's
// shape. A user can author exactly the same registry and legacy entries that
// gentle-ai would write, so plugin detection must only suppress new writes.
func TestInjectClaudePreservesIdenticalManualRegistrationsWhenPluginEnabled(t *testing.T) {
	home := t.TempDir()
	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")
	writeClaudeEngramPluginEnabled(t, home)

	registryPath := claude.UserConfigPath(home)
	registrySeed := []byte(`{"mcpServers":{"context7":{"command":"npx"},"engram":{"command":"/opt/homebrew/bin/engram","args":["mcp","--tools=agent"]}}}`)
	if err := os.WriteFile(registryPath, registrySeed, 0o644); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	legacySeed := []byte(`{"command":"/opt/homebrew/bin/engram","args":["mcp","--tools=agent"]}`)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, legacySeed, 0o644); err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if _, err := Inject(home, claudeAdapter()); err != nil {
			t.Fatalf("Inject() run %d error = %v", run, err)
		}
	}
	if got, err := os.ReadFile(registryPath); err != nil || !bytes.Equal(got, registrySeed) {
		t.Fatalf("manual registry changed: got=%q error=%v", got, err)
	}
	if got, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(got, legacySeed) {
		t.Fatalf("manual legacy config changed: got=%q error=%v", got, err)
	}
}

// TestInjectClaudePreservesUserAuthoredMCPServersEngramWhenPluginEnabled
// verifies that plugin detection only suppresses new direct registration and
// never alters an existing customized mcpServers.engram entry.
func TestInjectClaudePreservesUserAuthoredMCPServersEngramWhenPluginEnabled(t *testing.T) {
	home := t.TempDir()
	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")
	writeClaudeEngramPluginEnabled(t, home)

	registryPath := claude.UserConfigPath(home)
	seed := `{"mcpServers":{"engram":{"command":"/custom/path/engram","args":["mcp","--tools=agent"],"env":{"FOO":"bar"}}}}`
	if err := os.WriteFile(registryPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	// Inject() also bootstraps CLAUDE.md's protocol section on a fresh
	// tempdir, so overall Changed being true here is expected and not the
	// behavior under test; only the registry content matters (issue #4188).
	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	registry := readJSONFile(t, registryPath)
	assertNestedString(t, registry, "/custom/path/engram", "mcpServers", "engram", "command")
	assertNestedString(t, registry, "bar", "mcpServers", "engram", "env", "FOO")
}

func TestInjectClaudeUsesDirectMCPWhenPluginIsNotEnabled(t *testing.T) {
	for _, tt := range []struct {
		name     string
		settings string
	}{
		{name: "plugin disabled", settings: `{"enabledPlugins":{"engram@engram":false}}`},
		{name: "malformed settings", settings: `{not-json`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")
			settingsPath := filepath.Join(home, ".claude", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settingsPath, []byte(tt.settings), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := Inject(home, claudeAdapter()); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}
			registry := readJSONFile(t, claude.UserConfigPath(home))
			assertNestedString(t, registry, "/opt/homebrew/bin/engram", "mcpServers", "engram", "command")
		})
	}
}

func TestInjectClaudePreservesMalformedRegistryWhenPluginEnabled(t *testing.T) {
	home := t.TempDir()
	writeClaudeEngramPluginEnabled(t, home)
	registryPath := claude.UserConfigPath(home)
	registrySeed := []byte(`{not-json`)
	if err := os.WriteFile(registryPath, registrySeed, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if got, err := os.ReadFile(registryPath); err != nil || !bytes.Equal(got, registrySeed) {
		t.Fatalf("malformed user registry changed: got=%q error=%v", got, err)
	}
}

func writeClaudeEngramPluginEnabled(t *testing.T, home string) {
	t.Helper()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"enabledPlugins":{"engram@engram":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInjectClaudePreservesManagedLegacyParentLayouts(t *testing.T) {
	for _, symlinkParent := range []bool{true, false} {
		name := "non-empty real parent"
		if symlinkParent {
			name = "symlink parent"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			parent := filepath.Join(home, ".claude", "mcp")
			actualParent := parent
			if symlinkParent {
				actualParent = t.TempDir()
				if err := os.MkdirAll(filepath.Dir(parent), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(actualParent, parent); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(parent, 0o755); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(actualParent, "engram.json")
			custom := filepath.Join(actualParent, "custom.json")
			customBytes := []byte(`{"command":"custom"}`)
			if err := os.WriteFile(legacy, []byte(`{"command":"/usr/local/bin/engram","args":["mcp","--tools=agent"]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(custom, customBytes, 0o640); err != nil {
				t.Fatal(err)
			}

			if _, err := Inject(home, claudeAdapter()); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}
			info, err := os.Lstat(parent)
			if err != nil || symlinkParent != (info.Mode()&os.ModeSymlink != 0) {
				t.Fatalf("legacy parent changed unsafely: info=%v error=%v", info, err)
			}
			if got, err := os.ReadFile(custom); err != nil || !bytes.Equal(got, customBytes) {
				t.Fatalf("custom sibling changed: got=%q error=%v", got, err)
			}
		})
	}
}

// TestInjectClaudePreservesAbsoluteCommandIsIdempotent verifies that calling
// Inject() twice when an absolute-path engram.json already exists does not
// cause repeated writes (idempotency).
func TestInjectClaudePreservesAbsoluteCommandIsIdempotent(t *testing.T) {
	home := t.TempDir()

	absPath := "/usr/local/bin/engram"
	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	setupContent := []byte(`{
  "command": "/usr/local/bin/engram",
  "args": ["mcp", "--tools=agent"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	first, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}

	second, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true after absolute-path setup; want idempotent (no change)")
	}

	// Absolute path must still be present in the supported registry.
	content, err := os.ReadFile(claude.UserConfigPath(home))
	if err != nil {
		t.Fatalf("ReadFile(user registry) error = %v", err)
	}
	if !strings.Contains(string(content), absPath) {
		t.Fatalf("absolute command path %q was lost after second Inject(); got:\n%s", absPath, string(content))
	}
	_ = first // first result not the focus of this test
}

func TestInjectClaudePreservesUnmanagedLegacyShapes(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{"command array", []byte(`{"command":["/usr/local/bin/engram"],"args":["mcp","--tools=agent"]}`)},
		{"bare args", []byte(`{"command":"/usr/local/bin/engram","args":["mcp"]}`)},
		{"extra semantics", []byte(`{"command":"/usr/local/bin/engram","args":["mcp","--tools=agent"],"env":{"CUSTOM":"1"}}`)},
		{"custom command", []byte(`{"command":"custom-engram","args":["mcp","--tools=agent"]}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			legacyPath := filepath.Join(home, ".claude", "mcp", "engram.json")
			if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacyPath, tt.content, 0o640); err != nil {
				t.Fatal(err)
			}
			if _, err := Inject(home, claudeAdapter()); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}
			got, err := os.ReadFile(legacyPath)
			if err != nil || !bytes.Equal(got, tt.content) {
				t.Fatalf("unmanaged legacy config changed: got=%q error=%v", got, err)
			}
		})
	}
}

func TestInjectClaudeMigratesCellarCommandToStablePath(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "/usr/local/bin/engram", "")

	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	setupContent := []byte(`{
  "command": "/usr/local/Cellar/engram/1.14.1/bin/engram",
  "args": ["mcp", "--tools=agent"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	result, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false; expected Cellar command migration")
	}

	registryPath := claude.UserConfigPath(home)
	content, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("ReadFile(user registry) error = %v", err)
	}
	text := string(content)
	if strings.Contains(text, "/Cellar/") {
		t.Fatalf("engram.json still contains versioned Homebrew Cellar path; got:\n%s", text)
	}
	if !strings.Contains(text, "/usr/local/bin/engram") {
		t.Fatalf("engram.json did not migrate to stable Homebrew symlink; got:\n%s", text)
	}
	assertArgsHaveToolsAgent(t, registryPath)
}

func TestInjectCodexIsIdempotent(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	first, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(codex) first changed = false")
	}

	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(codex) second changed = true (should be idempotent)")
	}

	// Verify only one [mcp_servers.engram] block.
	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	count := strings.Count(string(content), "[mcp_servers.engram]")
	if count != 1 {
		t.Fatalf("config.toml has %d [mcp_servers.engram] blocks, want exactly 1; got:\n%s", count, string(content))
	}
}

func TestInjectCodexPreservesEngramHeaderInsideMultilineString(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	instructions := `developer_instructions = """
Example config:
[mcp_servers.engram]
command = "fake"
Keep this text.
"""`
	if err := os.WriteFile(configPath, []byte(instructions+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject(codex) first error = %v", err)
	}
	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(codex) second changed = true (should be idempotent)")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, instructions) {
		t.Fatalf("developer_instructions was not preserved byte-for-byte; got:\n%s", text)
	}
	// One header lives in the preserved string, the other is the managed block.
	if count := strings.Count(text, "[mcp_servers.engram]"); count != 2 {
		t.Fatalf("config.toml has %d [mcp_servers.engram] lines, want 2; got:\n%s", count, text)
	}
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatalf("config.toml missing managed engram block; got:\n%s", text)
	}
}

// ─── Codex profile injection tests ───────────────────────────────────────────

func TestInjectCodexPreservesLegacyProfilesWithInstalledCLI(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	profileDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sdd-strong.config.toml", "sdd-mid.config.toml", "sdd-cheap.config.toml"} {
		path := filepath.Join(profileDir, name)
		if err := os.WriteFile(path, []byte("user-owned profile\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := InjectWithOptions(home, codexAdapter(), InjectOptions{CodexCarrilModelAssignments: map[string]string{"sdd-strong": "gpt-6.1-astra"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sdd-strong.config.toml", "sdd-mid.config.toml", "sdd-cheap.config.toml"} {
		path := filepath.Join(profileDir, name)
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "user-owned profile\n" {
			t.Errorf("%s altered: %q, %v", name, got, err)
		}
		for _, reported := range result.Files {
			if reported == path {
				t.Errorf("retired profile reported as written: %s", path)
			}
		}
	}
}

func TestInjectCodexDoesNotCreateLegacyProfilesWithInstalledCLI(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	result, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sdd-strong.config.toml", "sdd-mid.config.toml", "sdd-cheap.config.toml"} {
		path := filepath.Join(home, ".codex", name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("retired profile created: %s (%v)", path, err)
		}
		for _, reported := range result.Files {
			if reported == path {
				t.Errorf("retired profile reported: %s", path)
			}
		}
	}
}

func TestInjectCodexWithoutCLIUpdatesSharedConfigAndPreservesProfiles(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound)
	t.Cleanup(restore)

	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const configBefore = "custom = \"preserve\"\n[unrelated]\nkeep = true\n"
	if err := os.WriteFile(configPath, []byte(configBefore), 0o644); err != nil {
		t.Fatal(err)
	}
	profiles := map[string][]byte{
		"sdd-strong.config.toml": []byte("user strong profile\n"),
		"sdd-mid.config.toml":    []byte("user mid profile\n"),
		"sdd-cheap.config.toml":  []byte("user cheap profile\n"),
	}
	for name, before := range profiles {
		if err := os.WriteFile(filepath.Join(home, ".codex", name), before, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	first, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) with absent CLI error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(codex) with absent CLI changed = false")
	}
	configAfterFirst, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configAfterFirst), "custom = \"preserve\"") || !strings.Contains(string(configAfterFirst), "[mcp_servers.engram]") {
		t.Fatalf("shared config was not preserved and updated:\n%s", configAfterFirst)
	}
	for name, before := range profiles {
		got, readErr := os.ReadFile(filepath.Join(home, ".codex", name))
		if readErr != nil || !bytes.Equal(got, before) {
			t.Fatalf("profile %q changed with absent CLI: got=%q error=%v", name, got, readErr)
		}
	}

	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("second Inject(codex) with absent CLI error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject(codex) with absent CLI changed = true")
	}
	configAfterSecond, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(configAfterSecond, configAfterFirst) {
		t.Fatalf("shared config was not idempotent: got=%q error=%v", configAfterSecond, err)
	}
}

func TestInjectCodexReportsInstructionOnlyRepairsWithoutCLI(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound)
	t.Cleanup(restore)

	for _, name := range []string{"engram-instructions.md", "engram-compact-prompt.md"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			if _, err := Inject(home, codexAdapter()); err != nil {
				t.Fatalf("initial Inject(codex) error = %v", err)
			}

			path := filepath.Join(home, ".codex", name)
			if err := os.WriteFile(path, []byte("stale instruction\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			result, err := Inject(home, codexAdapter())
			if err != nil {
				t.Fatalf("instruction-only repair error = %v", err)
			}
			if !result.Changed {
				t.Fatal("instruction-only repair changed = false")
			}
			found := false
			for _, file := range result.Files {
				if file == path {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("instruction-only repair files = %v, want %q", result.Files, path)
			}

			second, err := Inject(home, codexAdapter())
			if err != nil {
				t.Fatalf("idempotent Inject(codex) error = %v", err)
			}
			if second.Changed {
				t.Fatal("idempotent instruction-only repair changed = true")
			}
		})
	}
}

func TestInjectCodexWithoutCLIWritesSharedConfigWithoutCreatingProfiles(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("", exec.ErrNotFound)
	t.Cleanup(restore)

	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	profiles := []string{
		filepath.Join(home, ".codex", "sdd-strong.config.toml"),
		filepath.Join(home, ".codex", "sdd-mid.config.toml"),
		filepath.Join(home, ".codex", "sdd-cheap.config.toml"),
	}
	assertProfilesAbsent := func(stage string) {
		t.Helper()
		for _, path := range profiles {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("profile %q %s: stat error = %v; want absent", path, stage, err)
			}
		}
	}
	assertProfilesAbsent("before injection")

	first, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) with absent CLI error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(codex) with absent CLI changed = false")
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	if !strings.Contains(string(config), "[mcp_servers.engram]") ||
		!strings.Contains(string(config), "model_instructions_file") ||
		!strings.Contains(string(config), "experimental_compact_prompt_file") {
		t.Fatalf("shared MCP/instruction config was not written:\n%s", config)
	}
	for _, path := range []string{
		filepath.Join(home, ".codex", "engram-instructions.md"),
		filepath.Join(home, ".codex", "engram-compact-prompt.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("shared instruction file %q was not written: %v", path, err)
		}
	}
	assertProfilesAbsent("after first injection")

	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("second Inject(codex) with absent CLI error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject(codex) with absent CLI changed = true")
	}
	assertProfilesAbsent("after repeated injection")
}

func TestInjectCodexInvalidRuntimeDoesNotMutateFiles(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
	}{
		{name: "broken executable", err: &exec.Error{Name: "codex", Err: os.ErrPermission}},
		{name: "malformed version", output: "codex-cli development"},
		{name: "old version", output: "codex-cli 0.143.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := codex.SetRuntimeVersionCommandForTest(tt.output, tt.err)
			t.Cleanup(restore)

			home := t.TempDir()
			configPath := filepath.Join(home, ".codex", "config.toml")
			profilePath := filepath.Join(home, ".codex", "sdd-strong.config.toml")
			const configBefore = "user_setting = \"keep\"\n"
			const profileBefore = "user profile bytes\n"
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, []byte(configBefore), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(profilePath, []byte(profileBefore), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := Inject(home, codexAdapter()); err == nil {
				t.Fatal("Inject(codex) error = nil")
			}
			if got, err := os.ReadFile(configPath); err != nil || string(got) != configBefore {
				t.Fatalf("config changed after validation failure: got=%q error=%v", got, err)
			}
			if got, err := os.ReadFile(profilePath); err != nil || string(got) != profileBefore {
				t.Fatalf("profile changed after validation failure: got=%q error=%v", got, err)
			}
		})
	}
}

// ─── Codex multi-agent config injection tests ────────────────────────────────

// TestInjectCodexMultiAgentDefaultOn asserts that after a plain Inject call,
// config.toml contains [features] with multi_agent = true. Codex SDD enables
// multi-agent delegation by default so the per-phase reasoning_effort table applies.
func TestInjectCodexMultiAgentDefaultOn(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[features]") {
		t.Fatalf("config.toml missing [features] section; got:\n%s", text)
	}
	if !strings.Contains(text, "multi_agent = true") {
		t.Fatalf("config.toml missing multi_agent = true (enabled by default); got:\n%s", text)
	}
	if strings.Contains(text, "multi_agent = false") {
		t.Fatalf("config.toml must NOT have multi_agent = false by default; got:\n%s", text)
	}
}

// TestInjectCodexMultiAgentOptIn asserts that InjectWithOptions with
// CodexMultiAgent=true writes multi_agent = true in [features].
func TestInjectCodexMultiAgentOptIn(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	opts := InjectOptions{CodexMultiAgent: true}
	if _, err := InjectWithOptions(home, codexAdapter(), opts); err != nil {
		t.Fatalf("InjectWithOptions(codex, multiAgent=true) error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "multi_agent = true") {
		t.Fatalf("config.toml missing multi_agent = true after opt-in; got:\n%s", text)
	}
}

// TestInjectCodexMultiAgentDefaults asserts that the [agents] section is always
// written with max_threads = 4 and max_depth = 2 regardless of the opt-in flag.
func TestInjectCodexMultiAgentDefaults(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[agents]") {
		t.Fatalf("config.toml missing [agents] section; got:\n%s", text)
	}
	if !strings.Contains(text, "max_threads = 4") {
		t.Fatalf("config.toml missing max_threads = 4; got:\n%s", text)
	}
	if !strings.Contains(text, "max_depth = 2") {
		t.Fatalf("config.toml missing max_depth = 2; got:\n%s", text)
	}
}

// TestInjectCodexMultiAgentIdempotent asserts that running Inject twice
// produces exactly one [features] section and one [agents] section with no
// duplicate keys, and that the engram and context7 blocks are not disturbed.
func TestInjectCodexMultiAgentIdempotent(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()

	if _, err := Inject(home, codexAdapter()); err != nil {
		t.Fatalf("first Inject(codex) error = %v", err)
	}
	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("second Inject(codex) error = %v", err)
	}
	if second.Changed {
		// Read content for diagnostics.
		content, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
		t.Fatalf("second Inject(codex) changed = true, want false (multi-agent keys are idempotent); config.toml:\n%s", string(content))
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)

	if count := strings.Count(text, "[features]"); count != 1 {
		t.Fatalf("expected 1 [features] section, got %d; config.toml:\n%s", count, text)
	}
	if count := strings.Count(text, "[agents]"); count != 1 {
		t.Fatalf("expected 1 [agents] section, got %d; config.toml:\n%s", count, text)
	}
	if count := strings.Count(text, "multi_agent"); count != 1 {
		t.Fatalf("expected 1 multi_agent key, got %d; config.toml:\n%s", count, text)
	}
	if count := strings.Count(text, "max_threads"); count != 1 {
		t.Fatalf("expected 1 max_threads key, got %d; config.toml:\n%s", count, text)
	}
	if count := strings.Count(text, "max_depth"); count != 1 {
		t.Fatalf("expected 1 max_depth key, got %d; config.toml:\n%s", count, text)
	}
	// Engram MCP block must still be present.
	if !strings.Contains(text, "[mcp_servers.engram]") {
		t.Fatalf("config.toml missing [mcp_servers.engram] after idempotency run; got:\n%s", text)
	}
}

// ─── Absolute path resolution tests ──────────────────────────────────────────

// mockEngramLookPath sets EngramLookPath to a mock and restores it after the test.
func mockEngramLookPath(t *testing.T, result string, errMsg string) {
	t.Helper()
	orig := EngramLookPath
	EngramLookPath = func(string) (string, error) {
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return result, nil
	}
	t.Cleanup(func() { EngramLookPath = orig })
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v; content:\n%s", path, err, raw)
	}
	return parsed
}

func nestedValue(t *testing.T, root map[string]any, path ...string) (any, bool) {
	t.Helper()
	var current any = root
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func assertNestedString(t *testing.T, root map[string]any, want string, path ...string) {
	t.Helper()
	got, ok := nestedValue(t, root, path...)
	if !ok {
		t.Fatalf("missing JSON path %v in %#v", path, root)
	}
	if got != want {
		t.Fatalf("JSON path %v = %#v, want %q", path, got, want)
	}
}

func assertNestedStrings(t *testing.T, root map[string]any, want []string, path ...string) {
	t.Helper()
	got, ok := nestedValue(t, root, path...)
	if !ok {
		t.Fatalf("missing JSON path %v in %#v", path, root)
	}
	items, ok := got.([]any)
	if !ok {
		t.Fatalf("JSON path %v = %#v, want string array", path, got)
	}
	if len(items) != len(want) {
		t.Fatalf("JSON path %v length = %d, want %d (%#v)", path, len(items), len(want), got)
	}
	for i, wantItem := range want {
		if items[i] != wantItem {
			t.Fatalf("JSON path %v[%d] = %#v, want %q", path, i, items[i], wantItem)
		}
	}
}

func assertNestedMissing(t *testing.T, root map[string]any, path ...string) {
	t.Helper()
	if got, ok := nestedValue(t, root, path...); ok {
		t.Fatalf("JSON path %v present = %#v, want missing", path, got)
	}
}

// TestEngramInjectUsesAbsolutePathWhenAvailable verifies that when engram is
// resolvable on PATH, its absolute path is written into the MCP config file
// for agents that use StrategyMCPConfigFile (e.g. Windsurf).
func TestEngramInjectUsesAbsolutePathWhenAvailable(t *testing.T) {
	home := t.TempDir()

	absPath := "/usr/local/bin/engram"
	mockEngramLookPath(t, absPath, "")

	windsurfAdapter, err := agents.NewAdapter("windsurf")
	if err != nil {
		t.Fatalf("NewAdapter(windsurf) error = %v", err)
	}

	result, injectErr := Inject(home, windsurfAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(windsurf) error = %v", injectErr)
	}
	if !result.Changed {
		t.Fatalf("Inject(windsurf) changed = false")
	}

	mcpPath := windsurfAdapter.MCPConfigPath(home, "engram")
	content, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", mcpPath, readErr)
	}

	// Parse and validate the command field contains the absolute path.
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", mcpPath, err)
	}

	mcpServersRaw, ok := parsed["mcpServers"]
	if !ok {
		t.Fatalf("mcp_config.json missing mcpServers key; got:\n%s", content)
	}
	mcpServers, ok := mcpServersRaw.(map[string]any)
	if !ok {
		t.Fatalf("mcpServers has unexpected type: %T", mcpServersRaw)
	}
	engramServerRaw, ok := mcpServers["engram"]
	if !ok {
		t.Fatalf("mcpServers missing engram entry; got:\n%s", content)
	}
	engramServer, ok := engramServerRaw.(map[string]any)
	if !ok {
		t.Fatalf("engram server has unexpected type: %T", engramServerRaw)
	}

	cmd, _ := engramServer["command"].(string)
	if cmd != absPath {
		t.Fatalf("mcp_config.json command = %q, want absolute path %q", cmd, absPath)
	}
}

// TestEngramInjectFallsBackToRelativeWhenNotFound verifies that when engram
// cannot be resolved on PATH, the config falls back to the relative "engram"
// command string.
func TestEngramInjectFallsBackToRelativeWhenNotFound(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "", "not found")

	windsurfAdapter, err := agents.NewAdapter("windsurf")
	if err != nil {
		t.Fatalf("NewAdapter(windsurf) error = %v", err)
	}

	result, injectErr := Inject(home, windsurfAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(windsurf) error = %v", injectErr)
	}
	if !result.Changed {
		t.Fatalf("Inject(windsurf) changed = false")
	}

	mcpPath := windsurfAdapter.MCPConfigPath(home, "engram")
	content, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", mcpPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, `"command": "engram"`) {
		t.Fatalf("mcp_config.json should use relative fallback 'engram'; got:\n%s", text)
	}
}

// TestEngramInjectAbsolutePathForOpenCodeMergeStrategy verifies that the
// absolute path is used when the StrategyMergeIntoSettings strategy is
// applied for OpenCode.
func TestEngramInjectAbsolutePathForOpenCodeMergeStrategy(t *testing.T) {
	home := t.TempDir()

	absPath := "/usr/local/bin/engram"
	mockEngramLookPath(t, absPath, "")

	adapter := opencodeAdapter()
	settingsDir := filepath.Dir(adapter.SettingsPath(home))
	os.MkdirAll(settingsDir, 0o755)
	os.WriteFile(adapter.SettingsPath(home), []byte("{}"), 0o644)

	_, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(adapter.SettingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(content)
	// For standard agents (OpenCode), prefer the stable Homebrew symlink when
	// available instead of a versioned Cellar path.
	if !strings.Contains(text, `"engram"`) {
		t.Fatalf("OpenCode settings missing stable engram command, got: %s", text)
	}
	// OpenCode 1.3.3+: command must be an array, no separate "args" field.
	if strings.Contains(text, `"args"`) {
		t.Fatalf("OpenCode settings must NOT have a separate args field; got: %s", text)
	}

	// Structurally verify command is a []any containing the stable path "engram".
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}
	mcpRaw, ok := parsed["mcp"]
	if !ok {
		t.Fatalf("opencode.json missing mcp key; got:\n%s", text)
	}
	mcpMap, ok := mcpRaw.(map[string]any)
	if !ok {
		t.Fatalf("mcp key has unexpected type %T; got:\n%s", mcpRaw, text)
	}
	engramRaw, ok := mcpMap["engram"]
	if !ok {
		t.Fatalf("mcp missing engram key; got:\n%s", text)
	}
	engramMap, ok := engramRaw.(map[string]any)
	if !ok {
		t.Fatalf("mcp.engram has unexpected type %T; got:\n%s", engramRaw, text)
	}
	cmdRaw, ok := engramMap["command"]
	if !ok {
		t.Fatalf("mcp.engram missing command key; got:\n%s", text)
	}
	cmdArr, ok := cmdRaw.([]any)
	if !ok {
		t.Fatalf("mcp.engram.command must be an array, got %T; value:\n%s", cmdRaw, text)
	}
	if len(cmdArr) == 0 {
		t.Fatalf("mcp.engram.command array is empty; got:\n%s", text)
	}
	firstElem, ok := cmdArr[0].(string)
	if !ok || firstElem != absPath {
		t.Fatalf("mcp.engram.command[0] = %v, want stable Homebrew symlink %q; got:\n%s", cmdArr[0], absPath, text)
	}
}

// TestEngramInjectAbsolutePathForGeminiMergeStrategy verifies that the
// absolute path is also used when the StrategyMergeIntoSettings strategy is
// applied (e.g. Gemini CLI).
func TestEngramInjectAbsolutePathForGeminiMergeStrategy(t *testing.T) {
	home := t.TempDir()

	absPath := "/opt/homebrew/bin/engram"
	mockEngramLookPath(t, absPath, "")

	result, err := Inject(home, geminiAdapter())
	if err != nil {
		t.Fatalf("Inject(gemini) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(gemini) changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	content, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", readErr)
	}

	text := string(content)
	// For standard agents (Gemini), we now prioritize a stable relative path
	// "engram" instead of a dynamic absolute path to ensure idempotency.
	if !strings.Contains(text, `"engram"`) {
		t.Fatalf("settings.json missing stable relative path 'engram'; got:\n%s", text)
	}
}

func TestQwenEngramIdempotency(t *testing.T) {
	orig := EngramLookPath
	t.Cleanup(func() { EngramLookPath = orig })

	homeDir := t.TempDir()
	adapter := qwenAdapter()
	settingsPath := adapter.SettingsPath(homeDir)

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}

	EngramLookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}

	_, err := Inject(homeDir, adapter)
	if err != nil {
		t.Fatalf("First injection failed: %v", err)
	}

	content1, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate engram being found later (e.g. after go install or manual install)
	absPath := "/usr/local/bin/engram"
	EngramLookPath = func(string) (string, error) {
		return absPath, nil
	}

	_, err = Inject(homeDir, adapter)
	if err != nil {
		t.Fatalf("Second injection failed: %v", err)
	}

	content2, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(content1) != string(content2) {
		t.Errorf("Idempotency failure! Settings changed between runs despite engram command being stable-relative.\nRun 1:\n%s\nRun 2:\n%s", string(content1), string(content2))
	}
}

func TestInjectOpenClawMergesEngramIntoMCPServersPreservingStdioAndRemoteFields(t *testing.T) {
	mockEngramLookPath(t, "engram", "")

	home := t.TempDir()
	adapter := openclawAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := `{
  "mcp": {
    "sessionIdleTtlMs": 120000,
    "servers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem"],
        "env": {"ROOT": "/workspace"},
        "unknownStdioField": true
      },
      "linear": {
        "url": "https://mcp.linear.app/sse",
        "transport": "sse",
        "headers": {"Authorization": "Bearer existing-token"},
        "unknownRemoteField": "preserve-me"
      }
    }
  },
  "theme": "kanagawa"
}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(openclaw.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(openclaw) changed = false")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	root := unmarshalObjectForTest(t, content)
	mcp := objectAtForTest(t, root, "mcp")
	if got := mcp["sessionIdleTtlMs"]; got != float64(120000) {
		t.Fatalf("mcp.sessionIdleTtlMs = %v, want preserved 120000", got)
	}
	servers := objectAtForTest(t, mcp, "servers")

	filesystem := objectAtForTest(t, servers, "filesystem")
	if got := filesystem["command"]; got != "npx" {
		t.Fatalf("filesystem.command = %v, want npx", got)
	}
	if got := filesystem["unknownStdioField"]; got != true {
		t.Fatalf("filesystem.unknownStdioField = %v, want true", got)
	}
	args, ok := filesystem["args"].([]any)
	if !ok || len(args) != 2 || args[1] != "@modelcontextprotocol/server-filesystem" {
		t.Fatalf("filesystem.args = %#v, want preserved stdio args", filesystem["args"])
	}

	linear := objectAtForTest(t, servers, "linear")
	if got := linear["url"]; got != "https://mcp.linear.app/sse" {
		t.Fatalf("linear.url = %v, want preserved remote url", got)
	}
	if got := linear["transport"]; got != "sse" {
		t.Fatalf("linear.transport = %v, want sse", got)
	}
	headers := objectAtForTest(t, linear, "headers")
	if got := headers["Authorization"]; got != "Bearer existing-token" {
		t.Fatalf("linear Authorization header = %v, want preserved token", got)
	}
	if got := linear["unknownRemoteField"]; got != "preserve-me" {
		t.Fatalf("linear.unknownRemoteField = %v, want preserve-me", got)
	}

	engram := objectAtForTest(t, servers, "engram")
	if got := engram["command"]; got != "engram" {
		t.Fatalf("engram.command = %v, want engram", got)
	}
	engramArgs, ok := engram["args"].([]any)
	if !ok || len(engramArgs) != 2 || engramArgs[0] != "mcp" || engramArgs[1] != "--tools=agent" {
		t.Fatalf("engram.args = %#v, want [mcp --tools=agent]", engram["args"])
	}
	if _, hasMCPServers := root["mcpServers"]; hasMCPServers {
		t.Fatal("OpenClaw config must use mcp.servers, not top-level mcpServers")
	}
}

func TestInjectOpenClawHandlesJSON5ConfigAndMissingConfigPath(t *testing.T) {
	t.Run("preserves JSON5 compatible config content as normalized JSON", func(t *testing.T) {
		home := t.TempDir()
		adapter := openclawAdapter()
		configPath := adapter.SettingsPath(home)
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		existing := `{
  // OpenClaw user config with JSON5-style comments.
  "ui": {
    "theme": "kanagawa", // trailing comma survives via normalization
  },
  "mcp": {
    "servers": {
      "remoteDocs": {
        "url": "https://docs.example/mcp",
        "transport": "http",
      },
    },
  },
}`
		if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
			t.Fatalf("WriteFile(openclaw.json) error = %v", err)
		}

		if _, err := Inject(home, adapter); err != nil {
			t.Fatalf("Inject(openclaw) error = %v", err)
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("ReadFile(openclaw.json) error = %v", err)
		}
		root := unmarshalObjectForTest(t, content)
		ui := objectAtForTest(t, root, "ui")
		if got := ui["theme"]; got != "kanagawa" {
			t.Fatalf("ui.theme = %v, want preserved kanagawa", got)
		}
		servers := objectAtForTest(t, objectAtForTest(t, root, "mcp"), "servers")
		remoteDocs := objectAtForTest(t, servers, "remoteDocs")
		if got := remoteDocs["transport"]; got != "http" {
			t.Fatalf("remoteDocs.transport = %v, want preserved http", got)
		}
		if _, ok := servers["engram"]; !ok {
			t.Fatal("mcp.servers.engram missing after JSON5 merge")
		}
	})

	t.Run("creates canonical OpenClaw config when missing", func(t *testing.T) {
		home := t.TempDir()
		adapter := openclawAdapter()
		configPath := filepath.Join(home, ".openclaw", "openclaw.json")

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("expected missing OpenClaw config before inject, got err=%v", err)
		}
		if _, err := Inject(home, adapter); err != nil {
			t.Fatalf("Inject(openclaw) error = %v", err)
		}
		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("ReadFile(created openclaw.json) error = %v", err)
		}
		servers := objectAtForTest(t, objectAtForTest(t, unmarshalObjectForTest(t, content), "mcp"), "servers")
		if _, ok := servers["engram"]; !ok {
			t.Fatal("created OpenClaw config missing mcp.servers.engram")
		}
	})
}

func TestInjectOpenClawWritesEngramProtocolToWorkspaceAgentsOnly(t *testing.T) {
	workspace := t.TempDir()
	adapter := openclawAdapter()
	toolsPath := filepath.Join(workspace, "TOOLS.md")
	if err := os.WriteFile(toolsPath, []byte("# Tool guidance\n\nUser-owned tool notes.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(TOOLS.md) error = %v", err)
	}

	first, err := Inject(workspace, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(openclaw) first changed = false")
	}

	agentsPath := filepath.Join(workspace, "AGENTS.md")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	agentsText := string(agentsContent)
	for _, want := range []string{
		"<!-- gentle-ai:engram-protocol -->",
		"<!-- /gentle-ai:engram-protocol -->",
		"mem_save",
	} {
		if !strings.Contains(agentsText, want) {
			t.Fatalf("OpenClaw AGENTS.md missing Engram protocol content %q; got:\n%s", want, agentsText)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".openclaw", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("OpenClaw Engram injection must not write global .openclaw/AGENTS.md; stat err=%v", err)
	}

	toolsContent, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("ReadFile(TOOLS.md) error = %v", err)
	}
	toolsText := string(toolsContent)
	if strings.Contains(toolsText, "gentle-ai:engram-protocol") || strings.Contains(toolsText, "mem_save") {
		t.Fatalf("TOOLS.md must not receive Engram protocol sections; got:\n%s", toolsText)
	}
	if !strings.Contains(toolsText, "User-owned tool notes.") {
		t.Fatalf("TOOLS.md user content was modified; got:\n%s", toolsText)
	}

	second, err := Inject(workspace, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("OpenClaw Engram injection should be idempotent")
	}
	updated, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) second error = %v", err)
	}
	if count := strings.Count(string(updated), "<!-- gentle-ai:engram-protocol -->"); count != 1 {
		t.Fatalf("AGENTS.md has %d Engram protocol markers, want exactly 1", count)
	}
}

func TestInjectOpenClawRejectsAmbiguousWorkspacePath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	result, err := Inject("", openclawAdapter())
	if err == nil {
		t.Fatalf("Inject(openclaw, empty workspace) error = nil, want deterministic ambiguity error; result=%+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, ".openclaw", "openclaw.json")); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous OpenClaw workspace must not create relative config; stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, "AGENTS.md")); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous OpenClaw workspace must not create relative AGENTS.md; stat err=%v", statErr)
	}
}

func unmarshalObjectForTest(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal JSON error = %v; content:\n%s", err, content)
	}
	return root
}

func objectAtForTest(t *testing.T, root map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := root[key]
	if !ok {
		t.Fatalf("missing object key %q in %#v", key, root)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("key %q has type %T, want object", key, value)
	}
	return object
}

// TestInjectEngramHermesYAMLOverlay verifies that Inject writes the engram MCP
// server block under mcp_servers: in ~/.hermes/config.yaml (StrategyMergeIntoYAML),
// and that a second call is idempotent (Changed=false).
func TestInjectEngramHermesYAMLOverlay(t *testing.T) {
	home := t.TempDir()
	SetLookPathForTest(t, "engram", "")

	result, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(hermes) first run: changed = false, want true")
	}

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "mcp_servers:") {
		t.Fatal("config.yaml missing mcp_servers: key")
	}
	if !strings.Contains(text, "  engram:") {
		t.Fatal("config.yaml missing engram: entry under mcp_servers:")
	}
	if !strings.Contains(text, "--tools=agent") {
		t.Fatal("config.yaml missing --tools=agent in engram args")
	}

	// Second call must be idempotent.
	second, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(hermes) second run changed = true (not idempotent)")
	}
}

// TestEngramYAMLCommandRecoveryCustomPath verifies that a custom absolute
// engram command already in config.yaml is preserved (not clobbered) on re-run.
func TestEngramYAMLCommandRecoveryCustomPath(t *testing.T) {
	home := t.TempDir()
	SetLookPathForTest(t, "engram", "")

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a config.yaml with a custom absolute engram command.
	prior := "mcp_servers:\n  engram:\n    command: /custom/path/engram\n    args:\n      - mcp\n      - --tools=agent\n"
	if err := os.WriteFile(configPath, []byte(prior), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "/custom/path/engram") {
		t.Fatalf("config.yaml clobbered custom engram command; got:\n%s", text)
	}
}

// TestEngramYAMLCommandRecoveryVersionedCellar verifies that a versioned Homebrew
// cellar path is stabilized to the bare "engram" command on re-run.
func TestEngramYAMLCommandRecoveryVersionedCellar(t *testing.T) {
	home := t.TempDir()
	SetLookPathForTest(t, "engram", "")

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a config.yaml with a versioned Cellar engram path.
	prior := "mcp_servers:\n  engram:\n    command: /opt/homebrew/Cellar/engram/1.2.3/bin/engram\n    args:\n      - mcp\n      - --tools=agent\n"
	if err := os.WriteFile(configPath, []byte(prior), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	// Versioned cellar path must be stabilized — the versioned path should not remain.
	if strings.Contains(text, "/Cellar/engram/") {
		t.Fatalf("config.yaml retained versioned Cellar path after stabilization; got:\n%s", text)
	}
	// And it must be stabilized to the bare "engram" command.
	if !strings.Contains(text, "command: engram") {
		t.Fatalf("config.yaml did not stabilize to bare \"engram\" command; got:\n%s", text)
	}
}

// TestEngramYAMLCommandRecoveryAbsent verifies that when no prior engram entry
// exists in config.yaml, the stable "engram" fallback is written.
func TestEngramYAMLCommandRecoveryAbsent(t *testing.T) {
	home := t.TempDir()
	SetLookPathForTest(t, "engram", "")

	result, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(hermes) changed = false")
	}

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "command: engram") {
		t.Fatalf("expected 'command: engram' fallback when no prior entry; got:\n%s", text)
	}
}

// TestEngramYAMLCommandRecoveryListShape verifies that a YAML list-shaped command
// (command: - /path/engram) has its first element recovered correctly.
func TestEngramYAMLCommandRecoveryListShape(t *testing.T) {
	home := t.TempDir()
	SetLookPathForTest(t, "engram", "")

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a config.yaml with command as a YAML list.
	prior := "mcp_servers:\n  engram:\n    command:\n      - /absolute/path/engram\n    args:\n      - mcp\n      - --tools=agent\n"
	if err := os.WriteFile(configPath, []byte(prior), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Inject(home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	// The list first element (/absolute/path/engram) should be recovered and preserved.
	if !strings.Contains(text, "/absolute/path/engram") {
		t.Fatalf("list-shaped command first element not recovered; got:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// Decision 1 per-adapter slim/full selection matrix (16 adapters).
// ---------------------------------------------------------------------------

// aboveFloorVersion is a version comfortably above the v1.4.0 gate (matches
// the live evidence cited in design.md Decision 1: engram 1.18.0).
const aboveFloorVersion = "1.18.0"

// TestProtocolForSelectsSlimOrFullPerDecision1Matrix is the 16-row table test
// required by task 1.2: Claude Code -> slim (gated on engram >= v1.4.0), all
// other adapters with a setup slug or system-prompt surface -> full. Pi is
// covered separately below since it never renders protocol text at all
// (existing MCP-only precedent, unchanged by this change).
func TestProtocolForSelectsSlimOrFullPerDecision1Matrix(t *testing.T) {
	tests := []struct {
		agent    model.AgentID
		wantSlim bool
	}{
		{model.AgentClaudeCode, true},
		{model.AgentCodex, false},
		{model.AgentOpenCode, false},
		{model.AgentKilocode, false},
		{model.AgentGeminiCLI, false},
		{model.AgentAntigravity, false},
		{model.AgentWindsurf, false},
		{model.AgentQwenCode, false},
		{model.AgentCursor, false},
		{model.AgentVSCodeCopilot, false},
		{model.AgentKiroIDE, false},
		{model.AgentKimi, false},
		{model.AgentTrae, false},
		{model.AgentHermes, false},
		{model.AgentGitHubCopilotCLI, false},
		{model.AgentOpenClaw, false},
	}

	if len(tests) != 16 {
		t.Fatalf("expected 16 non-Pi adapters in the Decision 1 matrix, got %d", len(tests))
	}

	slim := protocolSlim()
	full := protocolFull()

	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			got := protocolFor(tt.agent, InjectOptions{Version: aboveFloorVersion})
			want := full
			if tt.wantSlim {
				want = slim
			}
			if got != want {
				gotKind, wantKind := "full", "full"
				if got == slim {
					gotKind = "slim"
				}
				if tt.wantSlim {
					wantKind = "slim"
				}
				t.Fatalf("protocolFor(%s, above-floor version) = %s section, want %s section", tt.agent, gotKind, wantKind)
			}
		})
	}
}

// TestProtocolForPiRendersNoProtocolText asserts the existing MCP-only
// precedent: Pi never writes protocol text via Inject(), regardless of
// version, because piEngramProvisioner short-circuits before protocolFor is
// ever consulted.
func TestProtocolForPiRendersNoProtocolText(t *testing.T) {
	home := t.TempDir()

	result, err := InjectWithOptions(home, piAdapter(), InjectOptions{Version: aboveFloorVersion})
	if err != nil {
		t.Fatalf("InjectWithOptions(pi) error = %v", err)
	}

	for _, path := range result.Files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		if strings.Contains(string(content), "mem_save") || strings.Contains(string(content), "Engram Persistent Memory") {
			t.Fatalf("Pi file %q unexpectedly contains protocol text; got:\n%s", path, content)
		}
	}
}

// ---------------------------------------------------------------------------
// Decision 1 version-gate boundary (task 1.3).
// ---------------------------------------------------------------------------

func TestProtocolForVersionGateBoundary(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		wantSlim bool
	}{
		{"below floor", "1.3.9", false},
		{"unknown/unparseable version", "not-a-version", false},
		{"empty version (VerifyVersion failed)", "", false},
		{"exact floor v1.4.0 (inclusive boundary)", "1.4.0", true},
		{"above floor", aboveFloorVersion, true},
	}

	slim := protocolSlim()
	full := protocolFull()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protocolFor(model.AgentClaudeCode, InjectOptions{Version: tt.version})
			want := full
			if tt.wantSlim {
				want = slim
			}
			if got != want {
				t.Fatalf("protocolFor(claude-code, version=%q) did not match expected section (wantSlim=%v)", tt.version, tt.wantSlim)
			}
		})
	}
}

// TestInjectWithOptionsThreadsVersionIntoClaudeSlimSelection is the
// integration-level counterpart of the boundary test above: it exercises the
// full InjectWithOptions -> CLAUDE.md write path and asserts the rendered
// section flips from full to slim once InjectOptions.Version crosses the
// v1.4.0 floor.
func TestInjectWithOptionsThreadsVersionIntoClaudeSlimSelection(t *testing.T) {
	belowFloorHome := t.TempDir()
	if _, err := InjectWithOptions(belowFloorHome, claudeAdapter(), InjectOptions{Version: "1.3.9"}); err != nil {
		t.Fatalf("InjectWithOptions(claude, below floor) error = %v", err)
	}
	belowContent, err := os.ReadFile(filepath.Join(belowFloorHome, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if !strings.Contains(string(belowContent), "needs_review") {
		t.Fatal("below-floor version must render the FULL section (expected 'needs_review' from full text)")
	}

	aboveFloorHome := t.TempDir()
	if _, err := InjectWithOptions(aboveFloorHome, claudeAdapter(), InjectOptions{Version: aboveFloorVersion}); err != nil {
		t.Fatalf("InjectWithOptions(claude, above floor) error = %v", err)
	}
	aboveContent, err := os.ReadFile(filepath.Join(aboveFloorHome, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if strings.Contains(string(aboveContent), "needs_review") {
		t.Fatal("above-floor version must render the SLIM section (must not contain full-only 'needs_review' text)")
	}
	if !strings.Contains(string(aboveContent), "SessionStart hook") {
		t.Fatal("above-floor version must render the SLIM section with its pointer to the full protocol location")
	}
}

// TestInjectWithOptionsReInjectConvergesFullToSlimAndBack is the spec
// scenario "Re-inject converges to target state" (Idempotent injection and
// clean uninstall across upgrades): a previously-injected full section MUST
// converge to slim (and vice versa) via the existing marker-based mechanism
// when the target verdict changes across two Inject calls, without
// duplicating or corrupting markers.
func TestInjectWithOptionsReInjectConvergesFullToSlimAndBack(t *testing.T) {
	home := t.TempDir()
	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")

	// 1. Full (below-floor / unknown version — safe default).
	if _, err := InjectWithOptions(home, claudeAdapter(), InjectOptions{}); err != nil {
		t.Fatalf("InjectWithOptions(full) error = %v", err)
	}
	afterFull, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if !strings.Contains(string(afterFull), "needs_review") {
		t.Fatalf("expected FULL section after first inject; got:\n%s", afterFull)
	}
	if n := strings.Count(string(afterFull), "<!-- gentle-ai:engram-protocol -->"); n != 1 {
		t.Fatalf("expected exactly 1 open marker after first inject, got %d", n)
	}

	// 2. Re-inject with an above-floor version — MUST converge to slim, in
	// place, no duplicate markers.
	if _, err := InjectWithOptions(home, claudeAdapter(), InjectOptions{Version: aboveFloorVersion}); err != nil {
		t.Fatalf("InjectWithOptions(slim) error = %v", err)
	}
	afterSlim, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if strings.Contains(string(afterSlim), "needs_review") {
		t.Fatalf("expected SLIM section after re-inject with above-floor version; got:\n%s", afterSlim)
	}
	if !strings.Contains(string(afterSlim), "SessionStart hook") {
		t.Fatalf("expected SLIM pointer content after re-inject; got:\n%s", afterSlim)
	}
	if n := strings.Count(string(afterSlim), "<!-- gentle-ai:engram-protocol -->"); n != 1 {
		t.Fatalf("expected exactly 1 open marker after re-inject to slim (no duplication), got %d", n)
	}
	if n := strings.Count(string(afterSlim), "<!-- /gentle-ai:engram-protocol -->"); n != 1 {
		t.Fatalf("expected exactly 1 close marker after re-inject to slim (no duplication), got %d", n)
	}

	// 3. Re-inject back to full — MUST converge back, still no duplication.
	if _, err := InjectWithOptions(home, claudeAdapter(), InjectOptions{}); err != nil {
		t.Fatalf("InjectWithOptions(back to full) error = %v", err)
	}
	afterBackToFull, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if !strings.Contains(string(afterBackToFull), "needs_review") {
		t.Fatalf("expected FULL section after re-inject back to full; got:\n%s", afterBackToFull)
	}
	if n := strings.Count(string(afterBackToFull), "<!-- gentle-ai:engram-protocol -->"); n != 1 {
		t.Fatalf("expected exactly 1 open marker after re-inject back to full (no duplication), got %d", n)
	}
}

func TestInjectCodexOrchestratorAssignmentWritesTopLevelModel(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	opts := InjectOptions{CodexOrchestratorAssignment: model.CodexPresetOrchestratorAssignment(string(model.CodexPresetRecommended))}
	if _, err := InjectWithOptions(home, codexAdapter(), opts); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `model = "gpt-6.1-sol"`) || !strings.Contains(text, `model_reasoning_effort = "medium"`) {
		t.Fatalf("top-level orchestrator assignment missing:\n%s", text)
	}
}

func TestInjectCodexOrchestratorAssignmentPreservesNestedModelAssignments(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`model = "old-top-level-model"
model_reasoning_effort = "low"

[[profiles]] # user settings
model = "nested-model"
model_reasoning_effort = "nested-effort"
model_instructions_file = "nested-instructions.md"
experimental_compact_prompt_file = "nested-compact.md"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := InjectOptions{CodexOrchestratorAssignment: model.CodexPresetOrchestratorAssignment(string(model.CodexPresetRecommended))}
	if _, err := InjectWithOptions(home, codexAdapter(), opts); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `model = "gpt-6.1-sol"`) || !strings.Contains(text, `model_reasoning_effort = "medium"`) {
		t.Fatalf("top-level orchestrator assignment missing:\n%s", text)
	}
	if !strings.Contains(text, `[[profiles]] # user settings
model = "nested-model"
model_reasoning_effort = "nested-effort"
model_instructions_file = "nested-instructions.md"
experimental_compact_prompt_file = "nested-compact.md"`) {
		t.Fatalf("nested model assignments were not preserved:\n%s", text)
	}
}

func TestInjectCodexNilOrchestratorAssignmentPreservesTopLevelModel(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("model = \"user-model\"\nmodel_reasoning_effort = \"high\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InjectWithOptions(home, codexAdapter(), InjectOptions{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `model = "user-model"`) || !strings.Contains(string(content), `model_reasoning_effort = "high"`) {
		t.Fatalf("nil assignment clobbered top-level model:\n%s", content)
	}
}

func TestUpsertCodexTableKeyBeforeMCPServersKeepsMCPBlocksAtEOF(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{
			name:    "missing table goes before existing MCP block",
			content: "[mcp_servers.context7]\nurl = \"https://mcp.context7.com/mcp\"\n",
			want:    "[features]\nmulti_agent = true\n\n[mcp_servers.context7]\nurl = \"https://mcp.context7.com/mcp\"\n",
		},
		{
			name:    "missing table without MCP blocks is appended",
			content: "model = \"gpt\"\n",
			want:    "model = \"gpt\"\n\n[features]\nmulti_agent = true\n",
		},
		{
			name:    "existing table is updated in place",
			content: "[mcp_servers.context7]\nurl = \"u\"\n\n[features]\nmulti_agent = false\n",
			want:    "[mcp_servers.context7]\nurl = \"u\"\n\n[features]\nmulti_agent = true\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := upsertCodexTableKeyBeforeMCPServers(tt.content, "features", "multi_agent", "true"); got != tt.want {
				t.Fatalf("upsert =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestUpsertCodexTableKeyBeforeMCPServersIgnoresMultilineStrings pins that an
// MCP table header written inside a TOML multiline string is text, not the
// first MCP block: the new table must land after the string and before the
// real MCP block instead of splitting the string.
func TestUpsertCodexTableKeyBeforeMCPServersIgnoresMultilineStrings(t *testing.T) {
	for _, delimiter := range []string{`"""`, "'''"} {
		content := "developer_instructions = " + delimiter + "\nExample:\n[mcp_servers.docs]\n" + delimiter + "\n\n[mcp_servers.context7]\ncommand = \"npx\"\n"
		got := upsertCodexTableKeyBeforeMCPServers(content, "features", "multi_agent", "true")
		stringEnd := strings.LastIndex(got, delimiter)
		table := strings.Index(got, "[features]\n")
		mcp := strings.Index(got, "[mcp_servers.context7]")
		if table < 0 || table < stringEnd || table > mcp {
			t.Fatalf("delimiter %s: [features] must be inserted after the multiline string and before the real MCP block:\n%s", delimiter, got)
		}
		if !strings.Contains(got, "Example:\n[mcp_servers.docs]\n"+delimiter) {
			t.Fatalf("delimiter %s: multiline string text was altered:\n%s", delimiter, got)
		}
	}
}

// TestUpsertCodexTableKeyBeforeMCPServersIgnoresTargetHeaderInMultilineStrings
// pins #5022: a [features] line inside developer_instructions is text, so the
// key must go to a real [features] table created outside the string, with or
// without an MCP block after it.
func TestUpsertCodexTableKeyBeforeMCPServersIgnoresTargetHeaderInMultilineStrings(t *testing.T) {
	for _, delimiter := range []string{`"""`, "'''"} {
		instructions := "developer_instructions = " + delimiter + "\n[features]\nmulti_agent = false\n" + delimiter + "\n"
		for _, tail := range []string{"", "\n[mcp_servers.context7]\ncommand = \"npx\"\n"} {
			got := upsertCodexTableKeyBeforeMCPServers(instructions+tail, "features", "multi_agent", "true")
			if !strings.HasPrefix(got, instructions) {
				t.Fatalf("delimiter %s tail %q: multiline string text was altered:\n%s", delimiter, tail, got)
			}
			table := strings.Index(got, delimiter+"\n\n[features]\nmulti_agent = true\n")
			if table < 0 {
				t.Fatalf("delimiter %s tail %q: [features] must be created after the multiline string:\n%s", delimiter, tail, got)
			}
			if mcp := strings.Index(got, "[mcp_servers.context7]"); tail != "" && (mcp < 0 || mcp < table) {
				t.Fatalf("delimiter %s: [features] must stay before the MCP block:\n%s", delimiter, got)
			}
		}
	}
}

// TestUpsertCodexTableKeyBeforeMCPServersIgnoresDelimitersInStringsAndComments
// pins that a triple-quote sequence inside an ordinary string or a comment
// does not open a multiline string, so the real MCP block is still found.
func TestUpsertCodexTableKeyBeforeMCPServersIgnoresDelimitersInStringsAndComments(t *testing.T) {
	for _, prefix := range []string{
		"note = '\"\"\"'\n",
		"# a comment with \"\"\" in it\n",
	} {
		content := prefix + "\n[mcp_servers.context7]\ncommand = \"npx\"\n"
		got := upsertCodexTableKeyBeforeMCPServers(content, "features", "multi_agent", "true")
		table := strings.Index(got, "[features]\n")
		mcp := strings.Index(got, "[mcp_servers.context7]")
		if table < 0 || table > mcp {
			t.Fatalf("prefix %q: [features] must be inserted before the MCP block:\n%s", prefix, got)
		}
	}
}
