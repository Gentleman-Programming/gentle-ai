//go:build linux

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallSharedInspectDisclosesSettingsWithoutEffects(t *testing.T) {
	for _, present := range []bool{true, false} {
		t.Run(fmt.Sprintf("settings-present-%v", present), func(t *testing.T) {
			req, beforePackages := shellSharedPreviewFixture(t, present)
			var output bytes.Buffer
			if err := RunShell([]string{"install", "--mode", "shared", "--prefix", req.SharedPrefix,
				"--agent", req.SharedAgent, "--target", req.Destination, "--inspect"}, &output); err != nil {
				t.Fatalf("inspection refused valid physical fixture: %v", err)
			}
			if !strings.Contains(output.String(), "Confirmation:") || !strings.Contains(output.String(), req.Destination+"/bin/pi") {
				t.Fatalf("existing confirmation/commands disappeared: %q", output.String())
			}
			assertShellSharedSettingsDisclosure(t, output.String(), req, beforePackages, present)
		})
	}
}

func TestShellInstallSharedTUIReviewDisclosesSettingsWithoutEffects(t *testing.T) {
	req, beforePackages := shellSharedPreviewFixture(t, true)
	m := shellInstallModel{req: req}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(shellInstallModel)
	if cmd != nil || m.err != nil || !m.review || m.confirmed || m.req.Confirmation == "" {
		t.Fatalf("Enter did not reach effect-free, unconfirmed review: %+v command=%v", m, cmd != nil)
	}
	assertShellSharedSettingsDisclosure(t, m.View(), req, beforePackages, true)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd != nil || next.(shellInstallModel).review || next.(shellInstallModel).confirmed {
		t.Fatal("declining shared review authorized installation")
	}
}

func TestShellInstallSharedInspectRefusesMalformedSettings(t *testing.T) {
	for name, settings := range map[string]string{
		"invalid-json": "{", "null": "null", "array": "[]",
		"null-packages": `{"packages":null}`, "non-array-packages": `{"packages":{}}`,
		"invalid-source": `{"packages":[{"source":3}]}`, "null-source": `{"packages":[{"source":null}]}`,
		"wrong-source-case": `{"packages":[{"Source":"npm:other"}]}`,
		"foreign-gentle":    `{"packages":["npm:gentle-pi"]}`, "foreign-command": `{"npmCommand":null}`,
		"byte-bound": `{"unused":"` + strings.Repeat("x", 64<<10) + `"}`, "settings-symlink": "symlink-fixture",
	} {
		t.Run(name, func(t *testing.T) {
			req, _ := shellSharedPreviewFixture(t, false, settings)
			var output bytes.Buffer
			err := RunShell([]string{"install", "--mode", "shared", "--prefix", req.SharedPrefix,
				"--agent", req.SharedAgent, "--target", req.Destination, "--inspect"}, &output)
			if err == nil || strings.Contains(output.String(), "Confirmation:") {
				t.Fatalf("unsupported settings received a confirmation: error=%v output=%q", err, output.String())
			}
		})
	}
}

// This fixture proves selection/review only, not an installed/authenticated graph.
// cli.js is an inert regular file and must never be executed by these tests.
func shellSharedPreviewFixture(t *testing.T, present bool, settingsOverride ...string) (shellinstaller.UserInstallRequest, []any) {
	t.Helper()
	if runtime.GOARCH != "amd64" {
		t.Skip("physical user selection is Linux amd64 only")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	req := shellinstaller.UserInstallRequest{Mode: "shared", Destination: filepath.Join(root, "shell"),
		SharedPrefix: filepath.Join(root, "prefix"), SharedAgent: filepath.Join(root, "agent")}
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(req.SharedPrefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js"), "// inert preview fixture; never execute\n")
	if err := os.MkdirAll(req.SharedAgent, 0700); err != nil {
		t.Fatal(err)
	}
	var packages []any
	if present {
		packages = []any{"npm:preview-existing", map[string]any{"source": "npm:preview-filtered", "extensions": []string{"keep.ts"}}}
		data, err := json.Marshal(map[string]any{"packages": packages, "theme": "UNRELATED_SETTING_CANARY"})
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(req.SharedAgent, "settings.json"), string(data))
	}
	// All canaries are synthetic, not operator settings or credentials.
	write(filepath.Join(req.SharedAgent, "private-note.txt"), "SECRET_CANARY_DO_NOT_DISCLOSE")
	write(filepath.Join(root, "other-agent/settings.json"), `{"packages":["npm:OTHER_SETTINGS_CANARY"],"npmCommand":["OTHER_COMMAND_CANARY"]}`)
	if len(settingsOverride) != 0 {
		settingsPath := filepath.Join(req.SharedAgent, "settings.json")
		if settingsOverride[0] == "symlink-fixture" {
			if err := os.Symlink(filepath.Join(root, "other-agent/settings.json"), settingsPath); err != nil {
				t.Fatal(err)
			}
		} else {
			write(settingsPath, settingsOverride[0])
		}
	}
	before := shellSharedPreviewTree(t, root)
	t.Cleanup(func() {
		if after := shellSharedPreviewTree(t, root); !reflect.DeepEqual(before, after) {
			t.Error("inspection/review changed fixture entries, bytes, modes, identities or modification times")
		}
	})
	return req, packages
}

func assertShellSharedSettingsDisclosure(t *testing.T, output string, req shellinstaller.UserInstallRequest, packages []any, present bool) {
	t.Helper()
	jsonText := func(value any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := "(absent)"
	if present {
		before = jsonText(packages)
	}
	after := append(append([]any{}, packages...), filepath.Join(req.SharedPrefix, "lib/node_modules/gentle-pi"))
	command := []string{filepath.Join(req.Destination, "runtime/node/bin/node"),
		filepath.Join(req.Destination, "runtime/node/lib/node_modules/npm/bin/npm-cli.js"), "--prefix", req.SharedPrefix}
	// Ignore layout whitespace but require complete, ordered before/after values.
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "")
	for _, want := range []string{
		filepath.Join(req.SharedAgent, "settings.json"),
		"packages before: " + before, "packages after: " + jsonText(after),
		"npmCommand before: (absent)", "npmCommand after: " + jsonText(command),
		"gentle-ai shell recover " + req.Destination + " inspect",
		"Recovery requires fresh confirmation", "leaves evidence and command bindings",
		"not uninstall or target deletion",
	} {
		if !strings.Contains(compact.Replace(output), compact.Replace(want)) {
			t.Errorf("shared review missing %q: %q", want, output)
		}
	}
	if !strings.Contains(strings.ToLower(output), "overwrite later changes") {
		t.Errorf("shared undo lacks warning that restoring preimages can overwrite later changes: %q", output)
	}
	for _, canary := range []string{"UNRELATED_SETTING_CANARY", "SECRET_CANARY_DO_NOT_DISCLOSE", "OTHER_SETTINGS_CANARY", "OTHER_COMMAND_CANARY"} {
		if strings.Contains(output, canary) {
			t.Errorf("shared review disclosed unrelated fixture data %q", canary)
		}
	}
}

func shellSharedPreviewTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var data []byte
		if !entry.IsDir() {
			data, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		stat := info.Sys().(*syscall.Stat_t)
		// Reading fixtures may update atime; inode/mode/mtime/content must not change.
		tree[path] = fmt.Sprintf("%d:%d:%v:%d:%s", stat.Dev, stat.Ino, info.Mode(), info.ModTime().UnixNano(), data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return tree
}
