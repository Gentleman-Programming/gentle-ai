//go:build linux || darwin

package shellinstaller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallAgentRestoresUnchangedPreimages(t *testing.T) {
	home := t.TempDir()
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(target, 0700)
	agent := filepath.Join(home, "agent")
	if err := os.Mkdir(agent, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"settings.json": `{"stock":true}`, "mcp.json": `{"mcpServers":{"context7":{"enabled":true}}}`}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(agent, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: agent, Mode: "agent"})
	preimage := map[string]string{"agent.preimage/settings.json": `{"old":true}`, "agent.preimage/mcp.json": `{"mcpServers":{}}`}
	post := map[string]userAgentPostFile{}
	for name, data := range files {
		post[name] = userAgentPostFile{Exists: true, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(data)))}
	}
	post["bin/fd"], post["bin/rg"] = userAgentPostFile{}, userAgentPostFile{}
	for relative, data := range preimage {
		path := filepath.Join(target, "state", relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	evidence, _ := json.Marshal(struct {
		Schema string                       `json:"schema"`
		Files  map[string]userAgentPostFile `json:"files"`
	}{userAgentPostSchema, post})
	if err := os.WriteFile(filepath.Join(target, "state/agent.post.json"), evidence, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecordUserShellInstall(home, UserShellInstall{Target: target, Mode: "agent", Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"changed":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: target, Mode: "agent"}); err == nil {
		t.Fatal("agent uninstall overwrote a changed Pi")
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatal("aborted agent uninstall moved the target")
	}
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(files["settings.json"]), 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := UninstallUserShellInstall(home, UserShellInstall{Target: target, Mode: "agent"})
	if err != nil || backup == "" {
		t.Fatalf("agent uninstall failed: %v %q", err, backup)
	}
	for name, want := range map[string]string{"settings.json": `{"old":true}`, "mcp.json": `{"mcpServers":{}}`} {
		data, err := os.ReadFile(filepath.Join(agent, name))
		if err != nil || string(data) != want {
			t.Fatalf("%s was not restored: %s %v", name, data, err)
		}
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("target still exists after agent uninstall")
	}
	entries, err := os.ReadDir(backup)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !userUnixBackupKeep[entry.Name()] {
			t.Fatalf("agent uninstall backup retains unpruned entry %q", entry.Name())
		}
	}
	if _, err := os.Lstat(filepath.Join(backup, "installation.json")); err != nil {
		t.Fatalf("agent uninstall backup lost the manifest: %v", err)
	}
	if got := DiscoverUserShellInstalls(home); len(got) != 0 {
		t.Fatalf("agent uninstall left registry: %+v", got)
	}
}

func TestUninstallSeparateLeavesMinimalBackup(t *testing.T) {
	home := t.TempDir()
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(target, 0700)
	for _, relative := range []string{
		"config", "agent", "agent/bin", "home", "home/.gentle-ai", "home/go", "home/go/pkg/mod", "prefix", "prefix/lib", "runtime/node", "runtime/node/bin", "runtime/cache",
		"runtime/tools", "state", "tmp", "bin", "pnpm", "source",
	} {
		if err := os.MkdirAll(filepath.Join(target, relative), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{
		"config/user.npmrc": "x", "agent/mcp.json": "x", "agent/bin/fd": "x",
		"home/.gentle-ai/state.json": "x", "prefix/lib/x.js": "x", "runtime/node/bin/node": "x",
		"runtime/cache/x": "x", "runtime/tools/fd.tgz": "x", "state/old": "x", "tmp/scratch": "x",
		"bin/gentle-shell": "x", "pnpm/store": "x", "source/package-lock.json": "x",
		"provision.mjs": "x", "supervisor": "x", "main.json": "x",
		"complete-generated-lock-sri.mjs": "x", "home/go/pkg/mod/m": "x",
	} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"), Mode: "separate"})
	if err := RecordUserShellInstall(home, UserShellInstall{Target: target, Mode: "separate", Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	backup, err := UninstallUserShellInstall(home, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil || backup == "" {
		t.Fatalf("separate uninstall failed: %v %q", err, backup)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("target still exists after separate uninstall")
	}
	entries, readErr := os.ReadDir(backup)
	if readErr != nil {
		t.Fatal(readErr)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !userUnixBackupKeep[entry.Name()] {
			t.Fatalf("separate uninstall backup retains unpruned entry %q", entry.Name())
		}
		seen[entry.Name()] = true
	}
	for _, name := range []string{"config", "agent", "home", "installation.json"} {
		if !seen[name] {
			t.Fatalf("minimal backup lost %q", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(backup, "home", "go")); !os.IsNotExist(err) {
		t.Fatalf("backup retained the Go module cache inside home: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(backup, "home", ".gentle-ai", "state.json")); err != nil {
		t.Fatalf("uninstall lost the retained home state: %v", err)
	}
	if got := DiscoverUserShellInstalls(home); len(got) != 0 {
		t.Fatalf("separate uninstall left registry: %+v", got)
	}
}
