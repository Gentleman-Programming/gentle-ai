//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallAgentRestoresUnchangedPreimages(t *testing.T) {
	home := userUnixTempRoot(t)
	target := userUnixTempRoot(t)
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
		// Legacy evidence records no mode: the restore stays owner-only.
		if info, err := os.Lstat(filepath.Join(agent, name)); err != nil || info.Mode() != 0600 {
			t.Fatalf("legacy restore of %s mode = %v %v", name, info.Mode(), err)
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
	home := userUnixTempRoot(t)
	target := userUnixTempRoot(t)
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

// agentToolsInstalled performs, at the helper level, every agent write a
// fresh agent install makes on a stock Pi agent holding Pi's own fd/rg:
// settings preimage + rewrite, the set-aside, the pinned bind and the
// post-image evidence. The target is published and registered.
func agentToolsInstalled(t *testing.T) (home string, req UserInstallRequest, bin, before string) {
	t.Helper()
	return agentToolsInstalledWith(t, func(tool string) []byte { return agentTargetBytes(4096, tool[0]-32) })
}

// agentToolsInstalledWith binds pinned(tool) as the pinned copy of each tool.
func agentToolsInstalledWith(t *testing.T, pinned func(tool string) []byte) (home string, req UserInstallRequest, bin, before string) {
	t.Helper()
	_, req = agentTargetFixture(t, true)
	agent := req.SharedAgent
	bin = filepath.Join(agent, "bin")
	before = agentTargetRecord(t, bin)
	target := req.Destination
	if err := os.MkdirAll(filepath.Join(target, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userSnapshotOwnedAgentSettings(context.Background(), agent, target); err != nil {
		t.Fatal(err)
	}
	agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"packages":["/x"],"npmCommand":["/n"]}`+"\n"), 0600)
	if err := userSetAsideAgentTools(context.Background(), agent, target); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"fd", "rg"} {
		if err := userToolWrite(filepath.Join(bin, tool), pinned(tool), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := userCaptureAgentPostImages(agent, target); err != nil {
		t.Fatal(err)
	}
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: agent, Mode: "agent"})
	home = userUnixTempRoot(t)
	if err := RecordUserShellInstall(home, UserShellInstall{Target: target, Mode: "agent", Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	return home, req, bin, before
}

// T16 rule 3: uninstall removes the pinned fd/rg and puts Pi's own back with
// their exact bytes and modes; no kept tool backup is left behind.
func TestUninstallAgentRestoresSetAsidePiTools(t *testing.T) {
	home, req, bin, before := agentToolsInstalled(t)
	backup, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"})
	if err != nil || backup == "" {
		t.Fatalf("agent uninstall failed: %v %q", err, backup)
	}
	if after := agentTargetRecord(t, bin); after != before {
		t.Fatalf("uninstall did not restore Pi's fd/rg:\n%s\n->\n%s", before, after)
	}
	if data, err := os.ReadFile(filepath.Join(req.SharedAgent, "settings.json")); err != nil || string(data) != agentTargetSettings {
		t.Fatalf("uninstall did not restore settings: %q %v", data, err)
	}
	if kept, _ := filepath.Glob(backup + userAgentToolsKeptSuffix + "*"); len(kept) != 0 {
		t.Fatalf("a restored uninstall kept a tool backup: %v", kept)
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
	if got := DiscoverUserShellInstalls(home); len(got) != 0 {
		t.Fatalf("agent uninstall left registry: %+v", got)
	}
}

// T16 rule 3: Pi 1.4.0 can download the very release the installer pins, so
// the pinned copy has Pi's bytes with the installer's 0700 mode. Uninstall
// still puts Pi's copy back with its own mode.
func TestUninstallAgentRestoresSetAsideToolWithPinnedBytes(t *testing.T) {
	home, req, bin, before := agentToolsInstalledWith(t, func(tool string) []byte {
		size := map[string]int{"fd": agentTargetStockFDSize, "rg": agentTargetStockRGSize}[tool]
		return agentTargetBytes(size, tool[0])
	})
	for _, tool := range []string{"fd", "rg"} {
		if info, err := os.Lstat(filepath.Join(bin, tool)); err != nil || info.Mode() != 0700 {
			t.Fatalf("fixture did not bind the pinned %s as 0700: %v %v", tool, info, err)
		}
	}
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"}); err != nil {
		t.Fatalf("agent uninstall failed: %v", err)
	}
	if after := agentTargetRecord(t, bin); after != before {
		t.Fatalf("uninstall did not restore Pi's fd/rg modes:\n%s\n->\n%s", before, after)
	}
}

// T16 rule 3: a pinned tool the operator deleted leaves an empty slot, which
// takes Pi's original back like an unchanged pinned one.
func TestUninstallAgentRestoresSetAsideToolIntoEmptySlot(t *testing.T) {
	home, req, bin, before := agentToolsInstalled(t)
	if err := os.Remove(filepath.Join(bin, "fd")); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"}); err != nil {
		t.Fatalf("agent uninstall failed: %v", err)
	}
	if after := agentTargetRecord(t, bin); after != before {
		t.Fatalf("uninstall did not restore Pi's fd/rg:\n%s\n->\n%s", before, after)
	}
}

// T16 rule 3: when Pi replaced a pinned tool after installation (for example
// it downloaded a newer fd), uninstall never overwrites it: the rest is
// restored, the original stays in a kept backup and the outcome says where.
func TestUninstallAgentKeepsSetAsideToolWhenPiReplacedIt(t *testing.T) {
	home, req, bin, before := agentToolsInstalled(t)
	newer := agentTargetBytes(8192, 'n')
	if err := os.Remove(filepath.Join(bin, "fd")); err != nil {
		t.Fatal(err)
	}
	agentTargetWrite(t, filepath.Join(bin, "fd"), newer, 0755)
	original := agentTargetBytes(agentTargetStockFDSize, 'f')
	backup, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"})
	if backup == "" || err == nil {
		t.Fatalf("uninstall with a replaced fd did not report it: %q %v", backup, err)
	}
	kept := backup + userAgentToolsKeptSuffix
	for _, phrase := range []string{filepath.Join(bin, "fd"), "changed after installation", kept, "installation moved to " + backup} {
		if !strings.Contains(err.Error(), phrase) {
			t.Fatalf("kept-tool report lacks %q: %v", phrase, err)
		}
	}
	var note interface{ UninstallNote() string }
	if !errors.As(err, &note) || !strings.Contains(note.UninstallNote(), kept) || strings.Contains(note.UninstallNote(), "installation moved to") {
		t.Fatalf("kept-tool outcome is not a completion note: %v", err)
	}
	if strings.Contains(err.Error(), filepath.Join(bin, "rg")) {
		t.Fatalf("kept-tool report names the restored rg: %v", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(bin, "fd")); readErr != nil || !bytes.Equal(data, newer) {
		t.Fatalf("uninstall overwrote Pi's newer fd: %v", readErr)
	}
	var wantBin []string
	for _, line := range strings.Split(before, "\n") {
		if !strings.HasPrefix(line, `"/fd":`) {
			wantBin = append(wantBin, line)
		}
	}
	if got := agentTargetRecordWithout(t, bin, "fd"); got != strings.Join(wantBin, "\n") {
		t.Fatalf("uninstall did not restore Pi's rg exactly:\n%s\n->\n%s", strings.Join(wantBin, "\n"), got)
	}
	if data, readErr := os.ReadFile(filepath.Join(kept, "fd")); readErr != nil || !bytes.Equal(data, original) {
		t.Fatalf("kept backup lost Pi's original fd: %v", readErr)
	}
	if _, statErr := os.Lstat(filepath.Join(kept, "rg")); !os.IsNotExist(statErr) {
		t.Fatalf("kept backup still holds the restored rg: %v", statErr)
	}
	record := agentToolsEvidence(t, filepath.Join(kept, userAgentToolsKeptEvidence))
	if image, ok := record.Files["fd"]; !ok || image.Mode != "0755" || image.SHA256 != fmt.Sprintf("%x", sha256.Sum256(original)) {
		t.Fatalf("kept backup evidence = %+v", record)
	}
	if data, readErr := os.ReadFile(filepath.Join(req.SharedAgent, "settings.json")); readErr != nil || string(data) != agentTargetSettings {
		t.Fatalf("uninstall did not restore settings: %q %v", data, readErr)
	}
	if _, statErr := os.Lstat(req.Destination); !os.IsNotExist(statErr) {
		t.Fatal("target still exists after agent uninstall")
	}
	if got := DiscoverUserShellInstalls(home); len(got) != 0 {
		t.Fatalf("agent uninstall left registry: %+v", got)
	}
}

// T16 rule 3: a damaged or missing tool backup aborts uninstall before any
// write: the agent and the target stay exactly as they are.
func TestUninstallAgentRefusesDamagedToolBackup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, backup string)
	}{
		{"changed bytes", func(t *testing.T, backup string) {
			if err := os.WriteFile(backup, []byte("tampered"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing", func(t *testing.T, backup string) {
			if err := os.Remove(backup); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, req, _, _ := agentToolsInstalled(t)
			tc.damage(t, filepath.Join(req.Destination, userAgentToolsBackup, "rg"))
			agentBefore, targetBefore := agentTargetRecord(t, req.SharedAgent), agentTargetRecord(t, req.Destination)
			_, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"})
			if err == nil || !strings.Contains(err.Error(), "rg") || !strings.Contains(err.Error(), "without touching") {
				t.Fatalf("damaged tool backup was not refused: %v", err)
			}
			if after := agentTargetRecord(t, req.SharedAgent); after != agentBefore {
				t.Fatalf("refused uninstall changed the agent:\n%s\n->\n%s", agentBefore, after)
			}
			if after := agentTargetRecord(t, req.Destination); after != targetBefore {
				t.Fatalf("refused uninstall changed the target:\n%s\n->\n%s", targetBefore, after)
			}
			if got := DiscoverUserShellInstalls(home); len(got) != 1 {
				t.Fatalf("refused uninstall dropped the registry entry: %+v", got)
			}
		})
	}
}

// agentInstalledWithContext7 performs every agent write of a fresh install
// with Context7 selected, in install order, and publishes and registers it.
func agentInstalledWithContext7(t *testing.T, req UserInstallRequest) (home string) {
	t.Helper()
	agent, target := req.SharedAgent, req.Destination
	if err := os.MkdirAll(filepath.Join(target, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userSnapshotOwnedAgentSettings(context.Background(), agent, target); err != nil {
		t.Fatal(err)
	}
	// provision.mjs publishes its settings declaration 0600.
	if err := os.Remove(filepath.Join(agent, "settings.json")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"packages":["/x"],"npmCommand":["/n"]}`+"\n"), 0600)
	if err := userStageOwnedAgentContext7(context.Background(), agent, "on", target); err != nil {
		t.Fatal(err)
	}
	if err := userSetAsideAgentTools(context.Background(), agent, target); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(agent, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"fd", "rg"} {
		if err := userToolWrite(filepath.Join(agent, "bin", tool), agentTargetBytes(4096, tool[0]-32), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := userCaptureAgentPostImages(agent, target); err != nil {
		t.Fatal(err)
	}
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: agent, Mode: "agent"})
	home = userUnixTempRoot(t)
	if err := RecordUserShellInstall(home, UserShellInstall{Target: target, Mode: "agent", Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	return home
}

// S1 data fidelity: uninstall returns the whole agent to its pre-install
// snapshot: settings.json and mcp.json with their original bytes and modes,
// an mcp.json that did not exist absent again, and Pi's own fd/rg back.
func TestUninstallAgentRestoresTheWholeAgentExactly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, agent string)
	}{
		{"stock Pi agent without mcp.json", func(*testing.T, string) {}},
		{"agent with its own 0640 mcp.json", func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "mcp.json"), []byte(`{"mcpServers":{"engram":{"command":"keep"}}}`+"\n"), 0640)
		}},
		{"agent without settings.json", func(t *testing.T, agent string) {
			if err := os.Remove(filepath.Join(agent, "settings.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"agent with a private 0600 settings.json", func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(agentTargetSettings), 0600)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, req := agentTargetFixture(t, true)
			tc.prepare(t, req.SharedAgent)
			before := agentTargetRecord(t, req.SharedAgent)
			home := agentInstalledWithContext7(t, req)
			if agentTargetRecord(t, req.SharedAgent) == before {
				t.Fatal("the simulated install did not change the agent")
			}
			if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"}); err != nil {
				t.Fatalf("agent uninstall failed: %v", err)
			}
			if after := agentTargetRecord(t, req.SharedAgent); after != before {
				t.Fatalf("uninstall did not restore the agent exactly:\n%s\n->\n%s", before, after)
			}
		})
	}
}

// S1 data fidelity: preimage evidence that would widen a restored file's
// permissions is refused before any write.
func TestUninstallAgentRefusesWideningPreimageMode(t *testing.T) {
	_, req := agentTargetFixture(t, true)
	home := agentInstalledWithContext7(t, req)
	evidence := filepath.Join(req.Destination, userAgentPreimageEvidence)
	data, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	widened := strings.Replace(string(data), `"0644"`, `"0666"`, 1)
	if widened == string(data) {
		t.Fatalf("preimage evidence does not record settings.json as 0644: %s", data)
	}
	if err := os.Remove(evidence); err != nil {
		t.Fatal(err)
	}
	agentTargetWrite(t, evidence, []byte(widened), 0600)
	agentBefore, targetBefore := agentTargetRecord(t, req.SharedAgent), agentTargetRecord(t, req.Destination)
	if _, err := UninstallUserShellInstall(home, UserShellInstall{Target: req.Destination, Mode: "agent"}); err == nil || !strings.Contains(err.Error(), "without touching") {
		t.Fatalf("widening preimage evidence was not refused: %v", err)
	}
	if after := agentTargetRecord(t, req.SharedAgent); after != agentBefore {
		t.Fatalf("refused uninstall changed the agent:\n%s\n->\n%s", agentBefore, after)
	}
	if after := agentTargetRecord(t, req.Destination); after != targetBefore {
		t.Fatalf("refused uninstall changed the target:\n%s\n->\n%s", targetBefore, after)
	}
}
