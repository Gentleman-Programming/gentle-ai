//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// Pi 1.4.0 creates ~/.pi/agent 0755 on its first run and downloads its own
// fd (3.7 MB) and rg (5.4 MB) into agent/bin; both exceed every 1 MiB cap.
const (
	agentTargetStockFDSize = 3_700_000
	agentTargetStockRGSize = 5_400_000
	agentTargetSettings    = "{\n  \"lastChangelogVersion\": \"1.4.0\",\n  \"theme\": \"dark\"\n}\n"
)

// agentTargetFixture builds a private installer parent holding an absent
// Gentle Shell target and an operator Pi agent shaped like the one Pi 1.4.0
// leaves behind: 0755 agent, 0644 settings.json, 0755 bin. withTools adds
// Pi's own fd and rg.
func agentTargetFixture(t *testing.T, withTools bool) (parent string, req UserInstallRequest) {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(parent, "agent")
	for _, dir := range []string{agent, filepath.Join(agent, "bin"), filepath.Join(agent, "sessions")} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		// Make the stock 0755 mode real regardless of the test umask.
		if err := os.Chmod(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(agentTargetSettings), 0644)
	agentTargetWrite(t, filepath.Join(agent, "auth.json"), []byte("{}\n"), 0600)
	if withTools {
		agentTargetWrite(t, filepath.Join(agent, "bin/fd"), agentTargetBytes(agentTargetStockFDSize, 'f'), 0755)
		agentTargetWrite(t, filepath.Join(agent, "bin/rg"), agentTargetBytes(agentTargetStockRGSize, 'r'), 0755)
	}
	return parent, UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "agent", SharedAgent: agent}
}

func agentTargetWrite(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func agentTargetBytes(size int, seed byte) []byte {
	data := bytes.Repeat([]byte{seed}, size)
	for index := 0; index < size; index += 4093 {
		data[index] = byte(index)
	}
	return data
}

// agentTargetRecord lists every name, presence, mode, link target and byte
// hash below root without following links: the full snapshot a refused or
// rolled-back install must leave unchanged.
func agentTargetRecord(t *testing.T, root string) string {
	t.Helper()
	var records []string
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, walkErr error) error {
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
		records = append(records, fmt.Sprintf("%q:%v:%s", strings.TrimPrefix(path, root), info.Mode(), content))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(records, "\n")
}

// agentTargetRefusesEverywhere drives every public entry the TUI uses for a
// fresh agent install and requires one refusal carrying every wanted phrase
// and no change anywhere below the installer parent.
func agentTargetRefusesEverywhere(t *testing.T, parent string, req UserInstallRequest, want ...string) {
	t.Helper()
	before := agentTargetRecord(t, parent)
	for _, entry := range []struct {
		name string
		call func() error
	}{
		{"validate", func() error { return ValidateUserInstall(req) }},
		{"inspect", func() error { _, err := InspectUserInstall(req); return err }},
		{"run", func() error {
			result, err := RunUserInstall(context.Background(), req)
			if result != (UserInstallResult{}) {
				t.Fatalf("refused agent install returned a result: %+v", result)
			}
			return err
		}},
	} {
		err := entry.call()
		if err == nil {
			t.Fatalf("%s admitted a refused agent selection", entry.name)
		}
		// Linux test processes run outside the delegated user unit, so the
		// kernel gate refuses Run first; the precheck text is darwin-observable.
		if entry.name == "run" && runtime.GOOS != "darwin" {
			continue
		}
		for _, phrase := range want {
			if !strings.Contains(err.Error(), phrase) {
				t.Fatalf("%s refusal lacks %q: %v", entry.name, phrase, err)
			}
		}
	}
	if after := agentTargetRecord(t, parent); after != before {
		t.Fatalf("refused agent install changed the parent:\n%s\n->\n%s", before, after)
	}
}

// T16 rule 1: a stock Pi agent already holds Pi's own fd and rg. They are no
// longer refused: every preflight entry admits the agent for the set-aside,
// without any effect on the agent or the installer parent.
func TestAgentTargetStockPiToolsAreAdmittedForSetAside(t *testing.T) {
	parent, req := agentTargetFixture(t, true)
	before := agentTargetRecord(t, parent)
	if err := ValidateUserInstall(req); err != nil {
		t.Fatalf("stock agent with Pi's fd/rg refused: %v", err)
	}
	first, err := InspectUserInstall(req)
	if err != nil || first == "" {
		t.Fatalf("stock agent with Pi's fd/rg inspection refused: %q %v", first, err)
	}
	if again, err := InspectUserInstall(req); err != nil || again != first {
		t.Fatalf("unchanged agent changed its confirmation: %q %v", again, err)
	}
	if after := agentTargetRecord(t, parent); after != before {
		t.Fatalf("admission had effects:\n%s\n->\n%s", before, after)
	}
}

// T16 rule 1: only Pi's own regular fd/rg files are set aside. Any other
// entry in their place keeps the up-front refusal, with the path and the fix.
func TestAgentTargetToolSetAsideRefusesForeignEntries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, bin string)
		want    []string
	}{
		{name: "symlinked fd", prepare: func(t *testing.T, bin string) {
			outside := filepath.Join(filepath.Dir(filepath.Dir(bin)), "fd-elsewhere")
			agentTargetWrite(t, outside, []byte("#!/bin/sh\nexit 0\n"), 0755)
			if err := os.Symlink(outside, filepath.Join(bin, "fd")); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"{bin}/fd", "symbolic link", "regular"}},
		{name: "fd directory", prepare: func(t *testing.T, bin string) {
			if err := os.Mkdir(filepath.Join(bin, "fd"), 0755); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"{bin}/fd", "not a regular file"}},
		{name: "fd fifo", prepare: func(t *testing.T, bin string) {
			if err := syscall.Mkfifo(filepath.Join(bin, "fd"), 0600); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"{bin}/fd", "not a regular file"}},
		{name: "group-writable rg", prepare: func(t *testing.T, bin string) {
			agentTargetWrite(t, filepath.Join(bin, "rg"), agentTargetBytes(4096, 'r'), 0775)
		}, want: []string{"{bin}/rg", "writable by other users", "chmod go-w"}},
		{name: "hard-linked rg", prepare: func(t *testing.T, bin string) {
			agentTargetWrite(t, filepath.Join(bin, "rg"), agentTargetBytes(4096, 'r'), 0755)
			if err := os.Link(filepath.Join(bin, "rg"), filepath.Join(filepath.Dir(filepath.Dir(bin)), "rg-link")); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"{bin}/rg", "hard links"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, req := agentTargetFixture(t, false)
			bin := filepath.Join(req.SharedAgent, "bin")
			tc.prepare(t, bin)
			want := append([]string(nil), tc.want...)
			for index := range want {
				want[index] = strings.ReplaceAll(want[index], "{bin}", bin)
			}
			agentTargetRefusesEverywhere(t, parent, req, want...)
			if err := userSetAsideAgentTools(context.Background(), req.SharedAgent, filepath.Join(parent, "absent-stage")); err == nil {
				t.Fatal("set-aside admitted a foreign tool entry")
			}
			if _, err := os.Lstat(filepath.Join(parent, "absent-stage")); !os.IsNotExist(err) {
				t.Fatalf("refused set-aside wrote a stage: %v", err)
			}
		})
	}
}

// agentToolsEvidence reads the set-aside evidence the install records.
func agentToolsEvidence(t *testing.T, path string) userAgentToolsRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record userAgentToolsRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

// agentTargetRecordWithout drops the named agent-relative entries from a
// snapshot, so the rest of the agent can be compared exactly.
func agentTargetRecordWithout(t *testing.T, root string, names ...string) string {
	t.Helper()
	var kept []string
	for _, line := range strings.Split(agentTargetRecord(t, root), "\n") {
		drop := false
		for _, name := range names {
			drop = drop || strings.HasPrefix(line, fmt.Sprintf("%q:", "/"+name))
		}
		if !drop {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// T16 rule 1: the set-aside moves exactly Pi's fd/rg into the installation
// backup with their bytes, and records their digests, sizes and modes as the
// installation's evidence. Nothing else in the agent changes, and the bind
// precheck then finds free tool slots.
func TestAgentTargetSetAsideMovesPiToolsIntoTheInstallation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tools []string
	}{{"fd and rg", []string{"fd", "rg"}}, {"fd only", []string{"fd"}}, {"neither", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			parent, req := agentTargetFixture(t, false)
			agent, bin := req.SharedAgent, filepath.Join(req.SharedAgent, "bin")
			originals := map[string][]byte{}
			for _, name := range tc.tools {
				originals[name] = agentTargetBytes(agentTargetStockFDSize, name[0])
				agentTargetWrite(t, filepath.Join(bin, name), originals[name], 0755)
			}
			rest := agentTargetRecordWithout(t, agent, "bin/fd", "bin/rg")
			root := filepath.Join(parent, "stage")
			if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := userSetAsideAgentTools(context.Background(), agent, root); err != nil {
				t.Fatalf("set-aside refused Pi's own tools: %v", err)
			}
			if after := agentTargetRecord(t, agent); after != rest {
				t.Fatalf("set-aside changed more than Pi's fd/rg:\n%s\n->\n%s", rest, after)
			}
			if len(tc.tools) == 0 {
				for _, path := range []string{filepath.Join(root, userAgentToolsBackup), filepath.Join(root, userAgentToolsEvidence)} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("set-aside without tools wrote %q: %v", path, err)
					}
				}
				return
			}
			record := agentToolsEvidence(t, filepath.Join(root, userAgentToolsEvidence))
			if record.Schema != userAgentToolsSchema || record.Agent != agent || len(record.Files) != len(tc.tools) {
				t.Fatalf("set-aside evidence = %+v", record)
			}
			for _, name := range tc.tools {
				backup := filepath.Join(root, userAgentToolsBackup, name)
				data, err := os.ReadFile(backup)
				if err != nil || !bytes.Equal(data, originals[name]) {
					t.Fatalf("backup of %s differs: %v", name, err)
				}
				if info, err := os.Lstat(backup); err != nil || info.Mode() != 0600 {
					t.Fatalf("backup of %s mode = %v %v", name, info.Mode(), err)
				}
				image := record.Files[name]
				if image.SHA256 != fmt.Sprintf("%x", sha256.Sum256(originals[name])) || image.Size != int64(len(originals[name])) || image.Mode != "0755" {
					t.Fatalf("evidence of %s = %+v", name, image)
				}
			}
			if err := userToolBinAvailable(agent); err != nil {
				t.Fatalf("bind precheck still sees Pi's tools: %v", err)
			}
		})
	}
}

// The Shared refusal keeps its message contract ("already contains fd", the
// macOS qualification harness consumes it) and still names the path.
func TestAgentTargetSharedToolConflictKeepsMessageContract(t *testing.T) {
	_, req := agentTargetFixture(t, false)
	bin := filepath.Join(req.SharedAgent, "bin")
	agentTargetWrite(t, filepath.Join(bin, "fd"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	before := agentTargetRecord(t, req.SharedAgent)
	err := userToolBinAvailable(req.SharedAgent)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "refused" || !strings.Contains(err.Error(), "already contains fd") || !strings.Contains(err.Error(), filepath.Join(bin, "fd")) || strings.Contains(err.Error(), "and rg") {
		t.Fatalf("Shared tool conflict message = %v", err)
	}
	if after := agentTargetRecord(t, req.SharedAgent); after != before {
		t.Fatalf("tool conflict check changed the agent:\n%s\n->\n%s", before, after)
	}
}

// Rule 1: every precondition the agent target refuses on later is checked
// before the first write to the agent, with an actionable message.
func TestAgentTargetPreconditionsRefuseUpFront(t *testing.T) {
	for _, tc := range []struct {
		name      string
		linuxOnly bool
		profile   func() *UserExperience
		prepare   func(t *testing.T, agent string)
		want      []string
	}{
		{name: "npmCommand residue", prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"npmCommand":["/missing/node"],"packages":[]}`), 0644)
		}, want: []string{"settings.json", "npmCommand", "remove"}},
		{name: "gentle-pi declaration", prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"packages":["npm:gentle-pi@1.0.0"]}`), 0644)
		}, want: []string{"settings.json", "gentle-pi", "remove"}},
		{name: "malformed settings", prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`["not","an","object"]`), 0644)
		}, want: []string{"settings.json", "JSON object"}},
		{name: "malformed packages", prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"packages":"npm:other"}`), 0644)
		}, want: []string{"settings.json", "packages"}},
		{name: "group-writable settings", prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(agentTargetSettings), 0664)
		}, want: []string{"settings.json", "chmod"}},
		{name: "symlinked settings", prepare: func(t *testing.T, agent string) {
			outside := filepath.Join(filepath.Dir(agent), "outside.json")
			agentTargetWrite(t, outside, []byte(agentTargetSettings), 0600)
			if err := os.Remove(filepath.Join(agent, "settings.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(agent, "settings.json")); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"settings.json", "regular file"}},
		{name: "group-writable agent", prepare: func(t *testing.T, agent string) {
			if err := os.Chmod(agent, 0775); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"{agent}", "chmod go-w"}},
		{name: "agent bin is a file", prepare: func(t *testing.T, agent string) {
			if err := os.Remove(filepath.Join(agent, "bin")); err != nil {
				t.Fatal(err)
			}
			agentTargetWrite(t, filepath.Join(agent, "bin"), []byte("x"), 0644)
		}, want: []string{"agent bin", "0700/0755"}},
		{name: "malformed mcp.json with Context7", linuxOnly: true, profile: func() *UserExperience {
			profile := DefaultUserExperience()
			profile.Context7 = "on"
			return &profile
		}, prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "mcp.json"), []byte(`[]`), 0600)
		}, want: []string{"mcp.json", "JSON object"}},
		{name: "leftover mcp.json.new with Context7", linuxOnly: true, profile: func() *UserExperience {
			profile := DefaultUserExperience()
			profile.Context7 = "on"
			return &profile
		}, prepare: func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "mcp.json.new"), []byte(`{}`), 0600)
		}, want: []string{"mcp.json.new", "remove"}},
		{name: "Main preview on a 0755 agent", linuxOnly: true, profile: func() *UserExperience {
			profile := DefaultUserExperience()
			profile.SDKSource = "main-preview"
			return &profile
		}, prepare: func(*testing.T, string) {}, want: []string{"Main preview", "chmod 700"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.linuxOnly && (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") {
				t.Skip("this experience selection is qualified on Linux amd64 only")
			}
			parent, req := agentTargetFixture(t, false)
			if tc.profile != nil {
				req.Experience = tc.profile()
			}
			tc.prepare(t, req.SharedAgent)
			want := append([]string(nil), tc.want...)
			for index := range want {
				want[index] = strings.ReplaceAll(want[index], "{agent}", req.SharedAgent)
			}
			agentTargetRefusesEverywhere(t, parent, req, want...)
		})
	}
}

// Rule 3: once Pi's own fd/rg are moved aside, the stock 0755 agent is
// admitted by every preflight entry without any effect.
func TestAgentTargetStockAgentIsAdmittedWithoutEffects(t *testing.T) {
	parent, req := agentTargetFixture(t, false)
	before := agentTargetRecord(t, parent)
	if err := ValidateUserInstall(req); err != nil {
		t.Fatalf("stock 0755 agent refused: %v", err)
	}
	first, err := InspectUserInstall(req)
	if err != nil || first == "" {
		t.Fatalf("stock 0755 agent inspection refused: %q %v", first, err)
	}
	if again, err := InspectUserInstall(req); err != nil || again != first {
		t.Fatalf("unchanged agent changed its confirmation: %q %v", again, err)
	}
	if after := agentTargetRecord(t, parent); after != before {
		t.Fatalf("admission had effects:\n%s\n->\n%s", before, after)
	}
}

// Rule 3: the late custody checks accept what the installer itself leaves in
// a stock 0755 agent: the identity binding admits the 0755 agent (still
// refusing other-writable ones) and the uninstall evidence covers the pinned
// multi-megabyte fd/rg instead of refusing them with a 1 MiB cap.
func TestAgentTargetLateCustodyAcceptsStockAgent(t *testing.T) {
	parent, req := agentTargetFixture(t, false)
	agent := req.SharedAgent
	identity, err := userSelectedAgentIdentity("agent", agent)
	if err != nil || identity == "" {
		t.Fatalf("0755 agent identity refused: %q %v", identity, err)
	}
	if separate, err := userSelectedAgentIdentity("separate", agent); err == nil {
		t.Fatalf("a non-agent selection admitted a 0755 directory: %q", separate)
	}
	root := filepath.Join(parent, "stage")
	if err := os.MkdirAll(filepath.Join(root, "state/agent.preimage"), 0700); err != nil {
		t.Fatal(err)
	}
	agentTargetWrite(t, filepath.Join(root, "state/agent.preimage/settings.json"), []byte(agentTargetSettings), 0600)
	// What the install leaves: rewritten settings and the pinned tools.
	agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"packages":["/x"],"npmCommand":["/n"]}`+"\n"), 0600)
	agentTargetWrite(t, filepath.Join(agent, "bin/fd"), agentTargetBytes(agentTargetStockFDSize, 'F'), 0700)
	agentTargetWrite(t, filepath.Join(agent, "bin/rg"), agentTargetBytes(agentTargetStockRGSize, 'R'), 0700)
	if err := userCaptureAgentPostImages(agent, root); err != nil {
		t.Fatalf("post-image evidence refused the pinned tools: %v", err)
	}
	if kept, err := userRestoreAgentImages(root, agent); err != nil || len(kept) != 0 {
		t.Fatalf("uninstall restore refused its own evidence: %v %v", kept, err)
	}
	for _, name := range []string{"bin/fd", "bin/rg", "mcp.json"} {
		if _, err := os.Lstat(filepath.Join(agent, name)); !os.IsNotExist(err) {
			t.Fatalf("uninstall left %s: %v", name, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(agent, "settings.json")); err != nil || string(data) != agentTargetSettings {
		t.Fatalf("uninstall did not restore settings: %q %v", data, err)
	}
	if err := os.Chmod(agent, 0775); err != nil {
		t.Fatal(err)
	}
	if _, err := userSelectedAgentIdentity("agent", agent); err == nil {
		t.Fatal("agent identity admitted a group-writable agent")
	}
}

// Rule 3 through the publication boundary: an agent manifest bound to a 0755
// agent reaches the rename instead of refusing the identity as "preimage".
func TestAgentTargetPublishAdmitsStockAgentIdentity(t *testing.T) {
	parent, req := agentTargetFixture(t, false)
	stage := filepath.Join(parent, "stage")
	for _, dir := range []string{stage, stage + "/prefix", stage + "/bin"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := userManifest{Schema: userSchema, Destination: req.Destination, Prefix: stage + "/prefix", Agent: req.SharedAgent, Mode: "agent"}
	var err error
	if manifest.PrefixIdentity, err = userIdentity(stage + "/prefix"); err != nil {
		t.Fatal(err)
	}
	if manifest.AgentIdentity, err = userSelectedAgentIdentity("agent", req.SharedAgent); err != nil {
		t.Fatal(err)
	}
	original := userPublishEphemeralCleanup
	t.Cleanup(func() { userPublishEphemeralCleanup = original })
	userPublishEphemeralCleanup = func(string) error { return nil }
	_, publishErr := userPublishManifest(context.Background(), stage, stage+"/prefix", req.SharedAgent, manifest)
	var failure *PrivateRuntimeError
	if !errors.As(publishErr, &failure) || failure.Kind != "uncertain" {
		t.Fatalf("agent publish did not reach the rename readback: %v", publishErr)
	}
	if _, err := os.Lstat(filepath.Join(req.Destination, "installation.json")); err != nil {
		t.Fatalf("agent publish did not rename the stage: %v", err)
	}
}

// Rule 2: a failure after any agent write restores the agent byte-for-byte.
// Each step is the exact write the install performs, in install order, and
// every prefix of the sequence, including torn temporaries, is rolled back.
func TestAgentTargetRollbackRestoresEveryMutationStep(t *testing.T) {
	type step struct {
		name  string
		apply func(t *testing.T, agent, root string)
	}
	settingsWrite := func(t *testing.T, agent string, torn bool) {
		// provision.mjs: writeExclusive(.gentle-shell-settings-PID) + rename.
		stage := filepath.Join(agent, ".gentle-shell-settings-4242")
		agentTargetWrite(t, stage, []byte(`{"packages":["/missing"],"npmCommand":["/missing/node"]}`+"\n"), 0600)
		if !torn {
			if err := os.Rename(stage, filepath.Join(agent, "settings.json")); err != nil {
				t.Fatal(err)
			}
		}
	}
	steps := []step{
		{"provision torn settings stage", func(t *testing.T, agent, _ string) { settingsWrite(t, agent, true) }},
		{"provision settings published", func(t *testing.T, agent, _ string) { settingsWrite(t, agent, false) }},
		{"Main SDK settings selection", func(t *testing.T, agent, _ string) {
			if err := os.Remove(filepath.Join(agent, ".gentle-shell-settings-4242")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			settingsWrite(t, agent, false)
		}},
		{"Context7 torn temporary", func(t *testing.T, agent, _ string) {
			agentTargetWrite(t, filepath.Join(agent, "mcp.json.new"), []byte(`{"mcpServers":{}}`), 0600)
		}},
		{"Context7 mcp.json published", func(t *testing.T, agent, root string) {
			if err := os.Remove(filepath.Join(agent, "mcp.json.new")); err != nil {
				t.Fatal(err)
			}
			if err := userStageOwnedAgentContext7(context.Background(), agent, "on", root); err != nil {
				t.Fatal(err)
			}
		}},
		{"set aside Pi's fd and rg", func(t *testing.T, agent, root string) {
			if err := userSetAsideAgentTools(context.Background(), agent, root); err != nil {
				t.Fatal(err)
			}
		}},
		{"tool bind fd", func(t *testing.T, agent, _ string) {
			if err := userToolWrite(filepath.Join(agent, "bin/fd"), agentTargetBytes(agentTargetStockFDSize, 'F'), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"tool bind rg", func(t *testing.T, agent, _ string) {
			if err := userToolWrite(filepath.Join(agent, "bin/rg"), agentTargetBytes(agentTargetStockRGSize, 'R'), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"post-image evidence", func(t *testing.T, agent, root string) {
			if err := userCaptureAgentPostImages(agent, root); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, variant := range []struct {
		name    string
		prepare func(t *testing.T, agent string)
	}{
		{"stock agent with bin", func(*testing.T, string) {}},
		{"stock agent with Pi's fd and rg", func(t *testing.T, agent string) {
			agentTargetWrite(t, filepath.Join(agent, "bin/fd"), agentTargetBytes(agentTargetStockFDSize, 'f'), 0755)
			agentTargetWrite(t, filepath.Join(agent, "bin/rg"), agentTargetBytes(agentTargetStockRGSize, 'r'), 0755)
		}},
		{"agent without bin or settings, with mcp.json", func(t *testing.T, agent string) {
			for _, path := range []string{filepath.Join(agent, "bin"), filepath.Join(agent, "settings.json")} {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			agentTargetWrite(t, filepath.Join(agent, "mcp.json"), []byte(`{"mcpServers":{"engram":{"command":"keep"}}}`), 0640)
		}},
	} {
		for count := 1; count <= len(steps); count++ {
			t.Run(fmt.Sprintf("%s/fail after %s", variant.name, steps[count-1].name), func(t *testing.T) {
				parent, req := agentTargetFixture(t, false)
				agent := req.SharedAgent
				variant.prepare(t, agent)
				root := filepath.Join(parent, "stage")
				if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
					t.Fatal(err)
				}
				before := agentTargetRecord(t, agent)
				guard, err := userBeginAgentRollback(agent)
				if err != nil {
					t.Fatalf("rollback snapshot refused a stock agent: %v", err)
				}
				if err := os.MkdirAll(filepath.Join(agent, "bin"), 0700); err != nil {
					t.Fatal(err)
				}
				for _, current := range steps[:count] {
					current.apply(t, agent, root)
				}
				if agentTargetRecord(t, agent) == before {
					t.Fatal("the injected steps did not mutate the agent")
				}
				injected := errors.New("injected failure")
				err = guard.rollback(injected, parent)
				if !errors.Is(err, injected) || !strings.Contains(err.Error(), "restored") || !strings.Contains(err.Error(), agent) {
					t.Fatalf("rollback result = %v", err)
				}
				if after := agentTargetRecord(t, agent); after != before {
					t.Fatalf("rollback left the agent changed:\n%s\n->\n%s", before, after)
				}
			})
		}
	}
}

// Rule 2: a rollback never writes into an agent that was replaced while the
// install ran, and it keeps the saved preimages for the operator instead.
func TestAgentTargetRollbackRefusesReplacedAgent(t *testing.T) {
	parent, req := agentTargetFixture(t, false)
	agent := req.SharedAgent
	guard, err := userBeginAgentRollback(agent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(agent, agent+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(agent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(agent, 0755); err != nil {
		t.Fatal(err)
	}
	before := agentTargetRecord(t, agent)
	injected := errors.New("injected failure")
	err = guard.rollback(injected, parent)
	if !errors.Is(err, injected) || !strings.Contains(err.Error(), "not restored") {
		t.Fatalf("rollback into a replaced agent = %v", err)
	}
	if after := agentTargetRecord(t, agent); after != before {
		t.Fatalf("rollback wrote into a replaced agent:\n%s\n->\n%s", before, after)
	}
	saved, globErr := filepath.Glob(filepath.Join(parent, ".gentle-agent-preimage-*", "settings.json"))
	if globErr != nil || len(saved) != 1 || !strings.Contains(err.Error(), filepath.Dir(saved[0])) {
		t.Fatalf("rollback did not keep the preimage for the operator: %v %v", saved, globErr)
	}
	if data, err := os.ReadFile(saved[0]); err != nil || string(data) != agentTargetSettings {
		t.Fatalf("kept preimage differs: %q %v", data, err)
	}
}

// Separate is unchanged by the agent prechecks: a sibling agent holding Pi's
// own fd/rg and a foreign npmCommand is not part of a Separate selection.
func TestAgentTargetSeparateSelectionUnchanged(t *testing.T) {
	parent, req := agentTargetFixture(t, true)
	agentTargetWrite(t, filepath.Join(req.SharedAgent, "settings.json"), []byte(`{"npmCommand":["/missing/node"]}`), 0644)
	separate := UserInstallRequest{Destination: req.Destination, Mode: "separate"}
	before := agentTargetRecord(t, parent)
	if err := ValidateUserInstall(separate); err != nil {
		t.Fatalf("Separate validation changed: %v", err)
	}
	if token, err := InspectUserInstall(separate); err != nil || token == "" {
		t.Fatalf("Separate inspection changed: %q %v", token, err)
	}
	if after := agentTargetRecord(t, parent); after != before {
		t.Fatalf("Separate preflight had effects:\n%s\n->\n%s", before, after)
	}
}

// A2: an agent refused for custody gets the fix for its actual cause; mode
// advice (chmod go-w) is given only when the mode bits are the problem.
func TestAgentTargetAgentCustodyRefusalNamesTheCause(t *testing.T) {
	for _, tc := range []struct {
		name       string
		darwinOnly bool
		prepare    func(t *testing.T, req *UserInstallRequest)
		want       []string
		modeAdvice bool
	}{
		{name: "writable by others", modeAdvice: true, prepare: func(t *testing.T, req *UserInstallRequest) {
			if err := os.Chmod(req.SharedAgent, 0775); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"{agent}", "writable by other users", "chmod go-w"}},
		{name: "symbolic link", prepare: func(t *testing.T, req *UserInstallRequest) {
			link := filepath.Join(filepath.Dir(req.SharedAgent), "agent-link")
			if err := os.Symlink(req.SharedAgent, link); err != nil {
				t.Fatal(err)
			}
			req.SharedAgent = link
		}, want: []string{"{agent}", "symbolic link", "real path", filepath.Join("{parent}", "agent")}},
		{name: "not a directory", prepare: func(t *testing.T, req *UserInstallRequest) {
			file := filepath.Join(filepath.Dir(req.SharedAgent), "agent-file")
			agentTargetWrite(t, file, []byte("x"), 0600)
			req.SharedAgent = file
		}, want: []string{"{agent}", "not a directory"}},
		{name: "allow ACL", darwinOnly: true, prepare: func(t *testing.T, req *UserInstallRequest) {
			if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", req.SharedAgent).CombinedOutput(); err != nil {
				t.Fatalf("chmod +a: %v %s", err, output)
			}
		}, want: []string{"{agent}", "ACL", "chmod -N"}},
		{name: "immutable flag", darwinOnly: true, prepare: func(t *testing.T, req *UserInstallRequest) {
			if output, err := exec.Command("/usr/bin/chflags", "uchg", req.SharedAgent).CombinedOutput(); err != nil {
				t.Fatalf("chflags uchg: %v %s", err, output)
			}
			agent := req.SharedAgent
			t.Cleanup(func() { _ = exec.Command("/usr/bin/chflags", "nouchg", agent).Run() })
		}, want: []string{"{agent}", "file flags", "chflags nouchg"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.darwinOnly && runtime.GOOS != "darwin" {
				t.Skip("ACLs and BSD file flags are darwin metadata")
			}
			parent, req := agentTargetFixture(t, false)
			tc.prepare(t, &req)
			want := append([]string(nil), tc.want...)
			for index := range want {
				want[index] = strings.NewReplacer("{agent}", req.SharedAgent, "{parent}", parent).Replace(want[index])
			}
			agentTargetRefusesEverywhere(t, parent, req, want...)
			if err := ValidateUserInstall(req); err == nil || strings.Contains(err.Error(), "chmod go-w") != tc.modeAdvice {
				t.Fatalf("mode advice = %v, want %v: %v", err != nil && strings.Contains(err.Error(), "chmod go-w"), tc.modeAdvice, err)
			}
		})
	}
}

// agentTargetCommitFixture is an agent install at the publication boundary:
// the agent already carries the install's bindings, the stage holds the
// manifest evidence, and publish is reached with a commit marker.
func agentTargetCommitFixture(t *testing.T) (parent string, req UserInstallRequest, stage string, manifest userManifest, guard *userAgentRollback, before string) {
	t.Helper()
	parent, req = agentTargetFixture(t, false)
	agent := req.SharedAgent
	stage = filepath.Join(parent, "stage")
	for _, dir := range []string{stage, stage + "/prefix", stage + "/bin", stage + "/state", stage + "/state/agent.preimage"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	before = agentTargetRecord(t, agent)
	var err error
	if guard, err = userBeginAgentRollback(agent); err != nil {
		t.Fatal(err)
	}
	agentTargetWrite(t, filepath.Join(stage, "state/agent.preimage/settings.json"), []byte(agentTargetSettings), 0600)
	agentTargetWrite(t, filepath.Join(agent, "settings.json"), []byte(`{"packages":["/x"],"npmCommand":["/n"]}`+"\n"), 0600)
	for _, tool := range []struct {
		name string
		size int
	}{{"fd", agentTargetStockFDSize}, {"rg", agentTargetStockRGSize}} {
		if err := userToolWrite(filepath.Join(agent, "bin", tool.name), agentTargetBytes(tool.size, tool.name[0]), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := userCaptureAgentPostImages(agent, stage); err != nil {
		t.Fatal(err)
	}
	manifest = userManifest{Schema: userSchema, Destination: req.Destination, Prefix: stage + "/prefix", Agent: agent, Mode: "agent"}
	if manifest.PrefixIdentity, err = userIdentity(stage + "/prefix"); err != nil {
		t.Fatal(err)
	}
	if manifest.AgentIdentity, err = userSelectedAgentIdentity("agent", agent); err != nil {
		t.Fatal(err)
	}
	original := userPublishEphemeralCleanup
	t.Cleanup(func() { userPublishEphemeralCleanup = original })
	userPublishEphemeralCleanup = func(string) error { return nil }
	return parent, req, stage, manifest, guard, before
}

// A1, before the commit point: a publication refused before its rename is
// not committed, and the agent is restored byte-for-byte.
func TestAgentTargetFailureBeforePublicationCommitRestores(t *testing.T) {
	parent, req, stage, manifest, guard, before := agentTargetCommitFixture(t)
	manifest.AgentIdentity = "changed"
	ctx, committed := userWithPublicationCommit(context.Background())
	_, publishErr := userPublishManifest(ctx, stage, stage+"/prefix", req.SharedAgent, manifest)
	var failure *PrivateRuntimeError
	if !errors.As(publishErr, &failure) || failure.Kind != "preimage" || *committed {
		t.Fatalf("pre-rename refusal = %v committed=%v", publishErr, *committed)
	}
	if _, err := os.Lstat(req.Destination); !os.IsNotExist(err) {
		t.Fatalf("pre-rename refusal published the target: %v", err)
	}
	err := guard.settle(publishErr, *committed, parent)
	if !errors.Is(err, publishErr) || !strings.Contains(err.Error(), "restored to its pre-install state") {
		t.Fatalf("settle before commit = %v", err)
	}
	if after := agentTargetRecord(t, req.SharedAgent); after != before {
		t.Fatalf("failure before commit left the agent changed:\n%s\n->\n%s", before, after)
	}
}

// A1, after the commit point: once the rename published the manifest bound
// to the agent, later failures (uncertain readback, workspace cleanup, an
// expired context) keep the agent bindings, and the published install stays
// consistent: its uninstall evidence still matches and restores the agent.
func TestAgentTargetFailureAfterPublicationCommitKeepsBindings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause func(publishErr error) error
	}{
		{"uncertain readback", func(publishErr error) error { return publishErr }},
		{"workspace cleanup", func(error) error { return privateError("uncertain", errors.New("workspace cleanup failed")) }},
		{"context expired", func(error) error { return context.DeadlineExceeded }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, req, stage, manifest, guard, _ := agentTargetCommitFixture(t)
			installed := agentTargetRecord(t, req.SharedAgent)
			ctx, committed := userWithPublicationCommit(context.Background())
			// The incomplete stage fails the post-rename readback as uncertain.
			_, publishErr := userPublishManifest(ctx, stage, stage+"/prefix", req.SharedAgent, manifest)
			var failure *PrivateRuntimeError
			if !errors.As(publishErr, &failure) || failure.Kind != "uncertain" || !*committed {
				t.Fatalf("post-rename failure = %v committed=%v", publishErr, *committed)
			}
			cause := tc.cause(publishErr)
			err := guard.settle(cause, *committed, parent)
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), "keeps the published bindings") || strings.Contains(err.Error(), "restored to its pre-install state") {
				t.Fatalf("settle after commit = %v", err)
			}
			if after := agentTargetRecord(t, req.SharedAgent); after != installed {
				t.Fatalf("failure after commit changed the agent bindings:\n%s\n->\n%s", installed, after)
			}
			if _, err := os.Lstat(filepath.Join(req.Destination, "installation.json")); err != nil {
				t.Fatalf("published manifest missing: %v", err)
			}
			if kept, err := userRestoreAgentImages(req.Destination, req.SharedAgent); err != nil || len(kept) != 0 {
				t.Fatalf("published install is inconsistent with its agent: %v %v", kept, err)
			}
			// Uninstall evidence restores contents and presence.
			if data, err := os.ReadFile(filepath.Join(req.SharedAgent, "settings.json")); err != nil || string(data) != agentTargetSettings {
				t.Fatalf("uninstall did not restore settings: %q %v", data, err)
			}
			for _, name := range []string{"bin/fd", "bin/rg", "mcp.json"} {
				if _, err := os.Lstat(filepath.Join(req.SharedAgent, name)); !os.IsNotExist(err) {
					t.Fatalf("uninstall left %s: %v", name, err)
				}
			}
		})
	}
}
