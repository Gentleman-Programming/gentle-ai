package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/sddstatus"
)

func TestRunSDDStatusPrintsMarkdownForBlockedStatus(t *testing.T) {
	root := t.TempDir()
	writeSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")

	var stdout bytes.Buffer
	err := RunSDDStatus([]string{"thin", "--cwd", root}, &stdout)
	if err != nil {
		t.Fatalf("RunSDDStatus() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"## SDD Status: thin",
		"### Blocked Reasons",
		"proposal.md is missing or partial.",
		"```json",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RunSDDStatus() output missing %q:\n%s", want, out)
		}
	}
}

func TestRunSDDStatusPrintsJSONWithInstructions(t *testing.T) {
	root := t.TempDir()
	seedSDDStatusReadyChange(t, root, "add-auth", "- [ ] 1.1 Wire routes\n")

	var stdout bytes.Buffer
	err := RunSDDStatus([]string{"add-auth", "--cwd", root, "--json", "--instructions"}, &stdout)
	if err != nil {
		t.Fatalf("RunSDDStatus() error = %v", err)
	}

	var status sddstatus.Status
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("RunSDDStatus() JSON decode error = %v\n%s", err, stdout.String())
	}
	if status.ChangeName == nil || *status.ChangeName != "add-auth" {
		t.Fatalf("ChangeName = %v, want add-auth", status.ChangeName)
	}
	if status.PhaseInstructions == nil {
		t.Fatal("PhaseInstructions = nil, want instructions included")
	}
	if status.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply", status.NextRecommended)
	}
}

func TestRunSDDContinuePrintsDispatcherMarkdownWithInstructions(t *testing.T) {
	root := t.TempDir()
	seedSDDStatusReadyChange(t, root, "add-auth", "- [ ] 1.1 Wire routes\n")

	var stdout bytes.Buffer
	err := RunSDDContinue([]string{"add-auth", "--cwd", root}, &stdout)
	if err != nil {
		t.Fatalf("RunSDDContinue() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"## Native SDD Dispatcher: add-auth",
		"next_recommended: apply",
		"### Next Phase Instructions: apply",
		"Read proposal, specs, design, and tasks before editing.",
		"```json",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RunSDDContinue() output missing %q:\n%s", want, out)
		}
	}
}

func TestRunSDDContinuePrintsJSONWithInstructionsByDefault(t *testing.T) {
	root := t.TempDir()
	seedSDDStatusReadyChange(t, root, "add-auth", "- [ ] 1.1 Wire routes\n")

	var stdout bytes.Buffer
	err := RunSDDContinue([]string{"add-auth", "--cwd", root, "--json"}, &stdout)
	if err != nil {
		t.Fatalf("RunSDDContinue() error = %v", err)
	}

	var status sddstatus.Status
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("RunSDDContinue() JSON decode error = %v\n%s", err, stdout.String())
	}
	if status.ChangeName == nil || *status.ChangeName != "add-auth" {
		t.Fatalf("ChangeName = %v, want add-auth", status.ChangeName)
	}
	if status.PhaseInstructions == nil {
		t.Fatal("PhaseInstructions = nil, want instructions included by default")
	}
	if status.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply", status.NextRecommended)
	}
}

func TestRunSDDStatusRejectsCWDWithoutNonFlagValue(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "cwd followed by json flag", args: []string{"--cwd", "--json"}},
		{name: "cwd followed by instructions flag", args: []string{"--cwd", "--instructions"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := RunSDDStatus(tt.args, &stdout); err == nil {
				t.Fatalf("RunSDDStatus(%v) expected error", tt.args)
			}
		})
	}
}

func TestSDDCommandsParseAndForwardExplicitLineage(t *testing.T) {
	parsed, err := sddstatus.ParseCommandArgs([]string{"thin", "--lineage", "compact-thin"})
	if err != nil || parsed.LineageID != "compact-thin" {
		t.Fatalf("ParseCommandArgs() = %#v, %v", parsed, err)
	}
	for _, run := range []struct {
		name string
		fn   func([]string, io.Writer) error
	}{{name: "status", fn: RunSDDStatus}, {name: "continue", fn: RunSDDContinue}} {
		t.Run(run.name, func(t *testing.T) {
			root := t.TempDir()
			seedSDDStatusReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
			var output bytes.Buffer
			err := run.fn([]string{"thin", "--cwd", root, "--lineage", "missing"}, &output)
			if err != nil || !strings.Contains(output.String(), "explicit lineage") {
				t.Fatalf("command error = %v, output = %s", err, output.String())
			}
		})
	}
}

func TestSDDCommandsRejectMissingLineageValue(t *testing.T) {
	for _, args := range [][]string{{"--lineage"}, {"--lineage", "--json"}} {
		if _, err := sddstatus.ParseCommandArgs(args); err == nil || !strings.Contains(err.Error(), "--lineage requires a value") {
			t.Fatalf("ParseCommandArgs(%v) error = %v", args, err)
		}
	}
}

func TestRunSDDStatusRejectsNonexistentCWD(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	var stdout bytes.Buffer
	if err := RunSDDStatus([]string{"--cwd", root}, &stdout); err == nil {
		t.Fatal("RunSDDStatus() expected error for nonexistent cwd")
	}
}

func seedSDDStatusReadyChange(t *testing.T, root string, name string, tasks string) string {
	t.Helper()
	changeRoot := filepath.Join(root, "openspec", "changes", name)
	writeSDDStatusFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "specs", "feature", "spec.md"), "# Spec\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "tasks.md"), tasks)
	return changeRoot
}

func writeSDDStatusFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
