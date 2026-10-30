//go:build linux

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic selected DATA only; no fixture is executed or installed.
func userSharedToolFixture(t *testing.T) (UserInstallRequest, string) {
	t.Helper()
	dest := userFixture(t)
	parent := filepath.Dir(dest)
	prefix, agent := filepath.Join(parent, "prefix"), filepath.Join(parent, "agent")
	cli := filepath.Join(prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
	for _, directory := range []string{filepath.Dir(cli), agent} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{cli: "selected Pi DATA", filepath.Join(agent, "settings.json"): "{\"theme\":\"dark\"}\n"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return UserInstallRequest{Destination: dest, Mode: "shared", SharedPrefix: prefix, SharedAgent: agent}, parent
}

// Records names, modes, link targets and byte hashes without following links.
func userSnapshot(t *testing.T, root string) string {
	t.Helper()
	var records []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		content := ""
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			content, err = os.Readlink(path)
		case info.Mode().IsRegular():
			var data []byte
			data, err = os.ReadFile(path)
			content = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		if err != nil {
			return err
		}
		records = append(records, fmt.Sprintf("%q:%v:%s", strings.TrimPrefix(path, root), info.Mode(), content))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(records, "\n")
}

func TestUserSharedAgentToolConflictRejectsBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, bin string)
	}{
		{"existing fd", func(t *testing.T, bin string) { userWriteFixture(t, filepath.Join(bin, "fd"), 0700) }},
		{"existing rg", func(t *testing.T, bin string) { userWriteFixture(t, filepath.Join(bin, "rg"), 0755) }},
		{"dangling rg link", func(t *testing.T, bin string) {
			if err := os.Symlink("missing-personal-rg", filepath.Join(bin, "rg")); err != nil {
				t.Fatal(err)
			}
		}},
		{"bin mode 0750", func(t *testing.T, bin string) {
			if err := os.Chmod(bin, 0750); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, parent := userSharedToolFixture(t)
			bin := filepath.Join(req.SharedAgent, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			tc.prepare(t, bin)
			prefixBefore, agentBefore := userSnapshot(t, req.SharedPrefix), userSnapshot(t, req.SharedAgent)
			for operation, call := range map[string]func() error{
				"validate": func() error { return ValidateUserInstall(req) },
				"inspect":  func() error { _, err := InspectUserInstall(req); return err },
			} {
				err := call()
				var failure *PrivateRuntimeError
				if !errors.As(err, &failure) || failure.Kind != "refused" || !strings.Contains(err.Error(), "agent bin") {
					t.Fatalf("%s admitted or misclassified agent tool conflict: %v", operation, err)
				}
			}
			if userSnapshot(t, req.SharedPrefix) != prefixBefore || userSnapshot(t, req.SharedAgent) != agentBefore {
				t.Fatal("rejected Shared selection changed selected prefix or agent")
			}
			if _, err := os.Lstat(req.Destination); !os.IsNotExist(err) {
				t.Fatalf("rejected Shared selection created destination: %v", err)
			}
			if stages, err := filepath.Glob(filepath.Join(parent, ".gentle-user-*")); err != nil || len(stages) != 0 {
				t.Fatalf("rejected Shared selection left owned staging: %v %v", stages, err)
			}
		})
	}
}

func TestUserSharedAgentToolBinAcceptedWhenFree(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
		file string
	}{
		{"absent bin", 0, ""},
		{"empty private bin", 0700, ""},
		{"readable bin with unrelated tool", 0755, "personal-helper"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := userSharedToolFixture(t)
			bin := filepath.Join(req.SharedAgent, "bin")
			if tc.mode != 0 {
				if err := os.Mkdir(bin, 0700); err != nil {
					t.Fatal(err)
				}
				if tc.file != "" {
					userWriteFixture(t, filepath.Join(bin, tc.file), 0700)
				}
				if err := os.Chmod(bin, tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			agentBefore := userSnapshot(t, req.SharedAgent)
			if err := ValidateUserInstall(req); err != nil {
				t.Fatalf("free agent bin refused: %v", err)
			}
			if _, err := InspectUserInstall(req); err != nil {
				t.Fatalf("free agent bin inspection refused: %v", err)
			}
			if userSnapshot(t, req.SharedAgent) != agentBefore {
				t.Fatal("accepted inspection changed selected agent")
			}
		})
	}
}

func TestUserToolsBindNeverReplacesPersonalTool(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()
	for _, directory := range []string{filepath.Join(root, "runtime/tools"), filepath.Join(agent, "bin")} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	userWriteFixture(t, filepath.Join(agent, "bin/rg"), 0700)
	before := userSnapshot(t, agent)
	err := userTools(context.Background(), root, agent, true)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "refused" {
		t.Fatalf("post-provisioning bind admitted personal tool: %v", err)
	}
	if userSnapshot(t, agent) != before {
		t.Fatal("refused bind changed selected agent bin")
	}
}

func TestUserToolsReadbackCreatesNoAgentBin(t *testing.T) {
	root, agent := t.TempDir(), t.TempDir()
	if err := userTools(context.Background(), root, agent, false); err == nil {
		t.Fatal("readback without retained archives admitted")
	}
	if _, err := os.Lstat(filepath.Join(agent, "bin")); !os.IsNotExist(err) {
		t.Fatalf("readback created agent bin: %v", err)
	}
}

func userWriteFixture(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("personal tool DATA, never executed"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
