package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDStatusAndContinueOmitExpectedPlanningBlockers(t *testing.T) {
	root := t.TempDir()
	writeSDDStatusFile(t, filepath.Join(root, "openspec", "changes", "thin", "tasks.md"), "- [ ] 1.1 Work\n")

	for name, run := range map[string]func([]string, io.Writer) error{
		"sdd-status":   RunSDDStatus,
		"sdd-continue": RunSDDContinue,
	} {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run([]string{"thin", "--cwd", root, "--json"}, &stdout); err != nil {
				t.Fatalf("command error = %v", err)
			}

			var status sddstatus.Status
			if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
				t.Fatalf("JSON decode error = %v\n%s", err, stdout.String())
			}
			if status.NextRecommended != "propose" {
				t.Fatalf("NextRecommended = %q, want propose", status.NextRecommended)
			}
			if len(status.BlockedReasons) != 0 {
				t.Fatalf("BlockedReasons = %v, want []", status.BlockedReasons)
			}
		})
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

func TestRunSDDStatusAndContinueReportFlatSpecConsistently(t *testing.T) {
	root := t.TempDir()
	changeRoot := filepath.Join(root, "openspec", "changes", "flat-spec")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "spec.md"), "### Requirement: flat\n#### Scenario: present\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")

	tests := []struct {
		name string
		run  func([]string, io.Writer) error
	}{
		{name: "sdd-status", run: RunSDDStatus},
		{name: "sdd-continue", run: RunSDDContinue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := tt.run([]string{"flat-spec", "--cwd", root, "--json"}, &stdout); err != nil {
				t.Fatalf("command error = %v", err)
			}

			var status sddstatus.Status
			if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
				t.Fatalf("JSON decode error = %v\n%s", err, stdout.String())
			}
			wantSpec := filepath.Join(changeRoot, "spec.md")
			if !reflect.DeepEqual(status.ArtifactPaths.Specs, []string{wantSpec}) {
				t.Fatalf("ArtifactPaths.Specs = %v, want [%s]", status.ArtifactPaths.Specs, wantSpec)
			}
			if status.Artifacts["specs"] != sddstatus.ArtifactDone {
				t.Fatalf("Artifacts[specs] = %q, want done", status.Artifacts["specs"])
			}
			if status.Dependencies.Specs != sddstatus.DependencyAllDone || status.Dependencies.Apply != sddstatus.DependencyReady {
				t.Fatalf("dependencies = %#v, want specs all_done and apply ready", status.Dependencies)
			}
			if status.NextRecommended != "apply" {
				t.Fatalf("NextRecommended = %q, want apply", status.NextRecommended)
			}
		})
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

func TestSDDJSONCommandsHideInternalRuntimeStatusInRealGitRepository(t *testing.T) {
	repo := initReviewCLIRepo(t)
	seedSDDStatusReadyChange(t, repo, "runtime-projection", "- [ ] 1.1 Work\n")
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, "runtime-projection")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(context.Background(), sddstatus.BeginAttemptRequest{
		ExpectedRevision: "",
		RequestID:        "begin-runtime-projection",
		WorkUnit:         "status-projection",
		EvidenceGoal:     "prove internal runtime status is not public",
		MaxAttempts:      1,
		MaxChangedLines:  20,
	}); err != nil {
		t.Fatal(err)
	}

	internalStatus, err := sddstatus.Resolve(sddstatus.ResolveOptions{
		CWD:        repo,
		ChangeName: "runtime-projection",
	})
	if err != nil {
		t.Fatal(err)
	}
	if internalStatus.RuntimeStatus == nil {
		t.Fatal("Resolve() RuntimeStatus = nil, want internal runtime authority")
	}

	for name, run := range map[string]func([]string, io.Writer) error{
		"sdd-status":   RunSDDStatus,
		"sdd-continue": RunSDDContinue,
	} {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run([]string{"runtime-projection", "--cwd", repo, "--json"}, &stdout); err != nil {
				t.Fatalf("%s error = %v", name, err)
			}

			var document map[string]json.RawMessage
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatalf("decode %s JSON: %v\n%s", name, err, stdout.String())
			}
			if _, leaked := document["runtimeStatus"]; leaked {
				t.Fatalf("%s leaked runtimeStatus:\n%s", name, stdout.String())
			}
			var remediation map[string]json.RawMessage
			if err := json.Unmarshal(document["remediationState"], &remediation); err != nil {
				t.Fatalf("decode remediationState: %v", err)
			}
			if _, leaked := remediation["correctionBudget"]; leaked {
				t.Fatalf("%s leaked remediationState.correctionBudget:\n%s", name, stdout.String())
			}
		})
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
