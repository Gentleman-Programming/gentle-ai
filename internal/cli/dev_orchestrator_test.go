package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/devjournal"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunDevOrchestratorStatus(t *testing.T) {
	root := t.TempDir()
	seedSDDStatusReadyChange(t, root, "add-auth", "- [ ] 1.1 Wire routes\n")

	var stdout bytes.Buffer
	if err := RunDevOrchestrator([]string{"status", "--cwd", root, "--change", "add-auth"}, &stdout); err != nil {
		t.Fatalf("RunDevOrchestrator(status) error = %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if _, ok := document["artifactStore"]; !ok {
		t.Fatalf("status output missing artifactStore:\n%s", stdout.String())
	}
	if _, ok := document["journal"]; !ok {
		t.Fatalf("status output missing journal:\n%s", stdout.String())
	}
}

func TestRunDevOrchestratorRoute(t *testing.T) {
	root := t.TempDir()

	var stdout bytes.Buffer
	err := RunDevOrchestrator([]string{"route", "--cwd", root, "--intent", "Add a payments export job", "--source", "issue-42"}, &stdout)
	if err != nil {
		t.Fatalf("RunDevOrchestrator(route) error = %v", err)
	}
	var result struct {
		ChangeID     string
		ArtifactPath string
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.ChangeID != "issue-42" {
		t.Fatalf("ChangeID = %q, want issue-42", result.ChangeID)
	}
	if _, statErr := os.Stat(filepath.Join(root, result.ArtifactPath)); statErr != nil {
		t.Fatalf("expected artifact at %s: %v", result.ArtifactPath, statErr)
	}
}

func TestRunDevOrchestratorContext(t *testing.T) {
	root := t.TempDir()
	// explore.md (not proposal.md): dev-explorer's AllowedArtifactTypes is
	// {requirement, bug, feature, exploration} (H-08), and explore.md is the
	// canonical filename that derives "exploration".
	artifactPath := filepath.Join(root, "openspec", "changes", "feature", "explore.md")
	writeSDDStatusFile(t, artifactPath, "---\nid: feature-1\n---\n# Proposal\n")

	var stdout bytes.Buffer
	err := RunDevOrchestrator([]string{
		"context", "--cwd", root, "--agent", "dev-explorer",
		"--artifact", "openspec/changes/feature/explore.md",
	}, &stdout)
	if err != nil {
		t.Fatalf("RunDevOrchestrator(context) error = %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatal("RunDevOrchestrator(context) produced no output")
	}
}

func TestRunDevOrchestratorDispatch(t *testing.T) {
	root := t.TempDir()
	seedSDDStatusReadyChange(t, root, "add-auth", "- [ ] 1.1 Wire routes\n")

	var stdout bytes.Buffer
	err := RunDevOrchestrator([]string{
		"dispatch", "--cwd", root, "--change", "add-auth", "--agent", "backend-implementer",
	}, &stdout)
	if err != nil {
		t.Fatalf("RunDevOrchestrator(dispatch) error = %v", err)
	}
	// The planned-dispatch prompt lines precede the final JSON result on
	// stdout; decode only the trailing JSON object.
	jsonStart := bytes.IndexByte(stdout.Bytes(), '{')
	if jsonStart < 0 {
		t.Fatalf("no JSON object found in dispatch output:\n%s", stdout.String())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes()[jsonStart:], &document); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if _, ok := document["journal"]; !ok {
		t.Fatalf("dispatch output missing journal:\n%s", stdout.String())
	}

	// The journal write must survive a second, independent Open+Load.
	store, err := devjournal.Open(root, "add-auth")
	if err != nil {
		t.Fatalf("re-open journal: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("re-load journal: %v", err)
	}
	if loaded.Record.Change != "add-auth" {
		t.Fatalf("journal Change = %q, want add-auth", loaded.Record.Change)
	}
	if len(loaded.Record.Dispatches) == 0 {
		t.Fatal("journal recorded zero dispatches")
	}
}

func TestRunDevOrchestratorRejectsUnknownOperation(t *testing.T) {
	var stdout bytes.Buffer
	if err := RunDevOrchestrator([]string{"bogus"}, &stdout); err == nil {
		t.Fatal("RunDevOrchestrator() expected error for unknown operation")
	}
}

func TestRunDevOrchestratorRejectsNonexistentCWD(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	tests := [][]string{
		{"status", "--cwd", missing, "--change", "x"},
		{"route", "--cwd", missing, "--intent", "add a feature"},
		{"dispatch", "--cwd", missing, "--change", "x", "--agent", "backend-implementer"},
	}
	for _, args := range tests {
		var stdout bytes.Buffer
		if err := RunDevOrchestrator(args, &stdout); err == nil {
			t.Fatalf("RunDevOrchestrator(%v) expected error for nonexistent cwd", args)
		}
	}
}

func TestRunDevOrchestratorRejectsCWDWithoutValue(t *testing.T) {
	var stdout bytes.Buffer
	if err := RunDevOrchestrator([]string{"status", "--change", "x", "--cwd"}, &stdout); err == nil {
		t.Fatal("RunDevOrchestrator(status) expected error for --cwd missing its value")
	}
}

// TestRunDevOrchestratorRefusesTraversal covers T1 at the CLI boundary: a
// caller-supplied --source/--change containing traversal segments must be
// refused with zero filesystem side effects.
func TestRunDevOrchestratorRefusesTraversal(t *testing.T) {
	t.Run("route --source traversal", func(t *testing.T) {
		root := t.TempDir()
		before, _ := os.ReadDir(root)
		var stdout bytes.Buffer
		err := RunDevOrchestrator([]string{"route", "--cwd", root, "--intent", "x", "--source", "../../../secret.md"}, &stdout)
		if err == nil {
			t.Fatal("RunDevOrchestrator(route) expected containment error")
		}
		after, _ := os.ReadDir(root)
		if len(after) != len(before) {
			t.Fatalf("RunDevOrchestrator(route) created filesystem entries on refusal: before=%v after=%v", before, after)
		}
	})

	t.Run("dispatch --change traversal", func(t *testing.T) {
		root := t.TempDir()
		var stdout bytes.Buffer
		err := RunDevOrchestrator([]string{"dispatch", "--cwd", root, "--change", "../../etc", "--agent", "backend-implementer"}, &stdout)
		if err == nil {
			t.Fatal("RunDevOrchestrator(dispatch) expected containment error")
		}
	})
}

// TestRefuseEngramWrite is a pure unit test for the write-path refusal
// decision (spec "Write operations refuse in Engram-mode repos" /
// "Read-only status remains allowed in Engram-mode repos"). Exercising this
// end-to-end would require a real `engram export` binary, so the decision
// function is tested directly against constructed projections.
func TestRefuseEngramWrite(t *testing.T) {
	if err := refuseEngramWrite(sddstatus.StatusV1Projection{ArtifactStore: sddstatus.ArtifactStoreEngram}); err == nil {
		t.Fatal("refuseEngramWrite() expected ErrEngramArtifactStoreUnsupported for engram store")
	} else if err.Error() == "" {
		t.Fatal("refuseEngramWrite() returned an empty error message")
	}
	if err := refuseEngramWrite(sddstatus.StatusV1Projection{ArtifactStore: sddstatus.ArtifactStoreOpenSpec}); err != nil {
		t.Fatalf("refuseEngramWrite() error = %v, want nil for openspec store", err)
	}
}
