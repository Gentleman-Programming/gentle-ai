package reviewassets_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestNativeAgentOwnership(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "jd-judge-a.md"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("custom before first install"), 0644); err != nil {
		t.Fatal(err)
	}
	first, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "custom before first install" {
		t.Fatalf("unknown file overwritten: %q", got)
	}
	if !containsPath(first.Skipped, path) {
		t.Fatalf("unknown file not reported skipped: %+v", first)
	}
	ledger := readOwnershipLedger(t, filepath.Join(dir, reviewassets.OwnershipLedgerFilename))
	if _, ok := ledger.Files[name]; ok {
		t.Fatal("unknown file adopted")
	}
	owned := "jd-judge-b.md"
	ownedPath := filepath.Join(dir, owned)
	original, err := os.ReadFile(ownedPath)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Files[owned] != fmt.Sprintf("%x", sha256.Sum256(original)) {
		t.Fatal("ledger is not hash of installed bytes")
	}
	changedOpts := reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: "CodeGraph changed guidance"}
	updated, err := reviewassets.InstallNativeAgents(home, adapter, changedOpts)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(ownedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) == string(original) || !containsPath(updated.Files, ownedPath) {
		t.Fatalf("known owned file did not update: %+v", updated)
	}
	ledger = readOwnershipLedger(t, filepath.Join(dir, reviewassets.OwnershipLedgerFilename))
	if ledger.Files[owned] != fmt.Sprintf("%x", sha256.Sum256(actual)) {
		t.Fatal("ledger did not track rendered update")
	}
	if err := os.WriteFile(ownedPath, []byte("user modified"), 0644); err != nil {
		t.Fatal(err)
	}
	skipped, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(ownedPath); string(got) != "user modified" || !containsPath(skipped.Skipped, ownedPath) {
		t.Fatalf("modified file not preserved/reported: %+v %q", skipped, got)
	}
	if readOwnershipLedger(t, filepath.Join(dir, reviewassets.OwnershipLedgerFilename)).Files[owned] != ledger.Files[owned] {
		t.Fatal("modified file was adopted")
	}
}

func TestNativeAgentOwnedModelUpdates(t *testing.T) {
	for _, tc := range []struct {
		agent   model.AgentID
		name    string
		options reviewassets.InstallOptions
		want    string
	}{
		{model.AgentClaudeCode, "jd-judge-a.md", reviewassets.InstallOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{"jd-judge-a": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh}}}, "effort: high"},
		{model.AgentKiroIDE, "jd-judge-a.md", reviewassets.InstallOptions{KiroModelAssignments: map[string]model.KiroModelAlias{"jd-judge-a": model.KiroModelOpus}}, "model: claude-opus"},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			adapter, err := agents.NewAdapter(tc.agent)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(adapter.SubAgentsDir(home), tc.name)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := reviewassets.InstallNativeAgents(home, adapter, tc.options)
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) == string(after) || !strings.Contains(string(after), tc.want) || !containsPath(result.Files, path) {
				t.Fatalf("owned model update not applied: %+v", result)
			}
			ledger := readOwnershipLedger(t, filepath.Join(adapter.SubAgentsDir(home), reviewassets.OwnershipLedgerFilename))
			if ledger.Files[tc.name] != fmt.Sprintf("%x", sha256.Sum256(after)) {
				t.Fatal("model update hash does not match installed bytes")
			}
		})
	}
}

func TestNativeAgentNoLedgerDoesNotAdoptMatchingBytes(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCursor)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	name := "review-risk.md"
	path := filepath.Join(dir, name)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, reviewassets.OwnershipLedgerFilename)); err != nil {
		t.Fatal(err)
	}
	matching, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsPath(matching.Skipped, path) || len(matching.Files) != 0 {
		t.Fatalf("unknown matching bytes were adopted: %+v", matching)
	}
	result, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{CodeGraphGuidanceMarkdown: "new guidance"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(before) || !containsPath(result.Skipped, path) {
		t.Fatalf("matching unknown bytes adopted: %+v", result)
	}
	if _, err := os.Lstat(filepath.Join(dir, reviewassets.OwnershipLedgerFilename)); !os.IsNotExist(err) {
		t.Fatalf("all-skipped install created an ownership ledger: %v", err)
	}
}

func TestNativeAgentInvalidLedgerFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		link       bool
	}{
		{name: "malformed", body: "{"}, {name: "unsupported", body: `{"version":2,"files":{}}`}, {name: "symlink", link: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := agents.NewAdapter(model.AgentCursor)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			dir := adapter.SubAgentsDir(home)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			ledger := filepath.Join(dir, reviewassets.OwnershipLedgerFilename)
			if tc.link {
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("safe"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, ledger); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(ledger, []byte(tc.body), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err == nil {
				t.Fatal("invalid ledger accepted")
			}
			if _, err := os.Lstat(filepath.Join(dir, "review-risk.md")); !os.IsNotExist(err) {
				t.Fatalf("partial install despite invalid ledger: %v", err)
			}
		})
	}
}

func TestNativeAgentSymlinkFailsClosed(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCursor)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "user")
	if err := os.WriteFile(target, []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "review-risk.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err == nil {
		t.Fatal("symlink accepted")
	}
	if got, _ := os.ReadFile(target); string(got) != "safe" {
		t.Fatalf("symlink target modified: %q", got)
	}
}

type ownershipFixture struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

func readOwnershipLedger(t *testing.T, path string) ownershipFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ledger ownershipFixture
	if err := json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.Version != 1 {
		t.Fatalf("ledger version: %d", ledger.Version)
	}
	return ledger
}
func containsPath(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

func TestNativeAgentsRejectUnsupportedRuntimeWithoutWriting(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err == nil {
		t.Fatal("unsupported runtime accepted")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unsupported install wrote files: %v %v", entries, err)
	}
}

func TestNativeAgentModelsAndUnrelatedFiles(t *testing.T) {
	for _, tc := range []struct {
		agent      model.AgentID
		opts       reviewassets.InstallOptions
		name, want string
	}{
		{model.AgentClaudeCode, reviewassets.InstallOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{"jd-judge-a": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh}}}, "jd-judge-a.md", "model: opus"},
		{model.AgentKiroIDE, reviewassets.InstallOptions{KiroModelAssignments: map[string]model.KiroModelAlias{"jd-judge-a": model.KiroModelOpus}}, "jd-judge-a.md", "model: claude-opus"},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			adapter, err := agents.NewAdapter(tc.agent)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			dir := adapter.SubAgentsDir(home)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			custom := filepath.Join(dir, "custom.md")
			if err := os.WriteFile(custom, []byte("user content"), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := reviewassets.InstallNativeAgents(home, adapter, tc.opts); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(dir, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), tc.want) || strings.Contains(string(content), "{{KIRO_MODEL}}") || strings.Contains(string(content), "{{CLAUDE_MODEL}}") {
				t.Errorf("model placeholder not resolved: %s", content[:min(250, len(content))])
			}
			if tc.agent == model.AgentClaudeCode && !strings.Contains(string(content), "effort: high") {
				t.Error("Claude effort not rendered")
			}
			preserved, err := os.ReadFile(custom)
			if err != nil || string(preserved) != "user content" {
				t.Fatalf("custom file overwritten: %q %v", preserved, err)
			}
		})
	}
}
