package autoskill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeInboxSkill deposits one proposal in the manager's inbox, the shape
// Scan produces (SKILL.md plus metadata.json).
func writeInboxSkill(t *testing.T, workspace, name string) {
	t.Helper()
	folder := filepath.Join(workspace, ".axiom", "skills", "inbox", name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte("---\nname: "+name+"\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "metadata.json"), []byte(`{"name":"`+name+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestApproveRegeneratesIndexAfterPromotion covers REQ-22.13 «La promoción
// deja la skill indexada sin pasos manuales» at the hook boundary: a completed
// promotion runs the regenerator and reports no error.
func TestApproveRegeneratesIndexAfterPromotion(t *testing.T) {
	workspace := t.TempDir()
	writeInboxSkill(t, workspace, "go-testing")

	var calls int
	var gotCwd string
	manager := NewManager(workspace, nil, nil, nil)
	manager.RegenerateIndex = func(cwd, home string) error {
		calls++
		gotCwd = cwd
		return nil
	}

	outcome, err := manager.Approve("go-testing")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !outcome.Promoted || outcome.RegenerateError != nil {
		t.Fatalf("outcome = %+v, want promoted without regeneration error", outcome)
	}
	if calls != 1 {
		t.Fatalf("regenerator called %d times, want 1", calls)
	}
	if gotCwd != workspace {
		t.Fatalf("regenerator cwd = %q, want %q", gotCwd, workspace)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "skills", "go-testing", "SKILL.md")); statErr != nil {
		t.Fatalf("promoted skill missing: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, ".axiom", "skills", "inbox", "go-testing")); !os.IsNotExist(statErr) {
		t.Fatalf("inbox must be empty after promotion (stat err = %v)", statErr)
	}
}

// TestApproveKeepsPromotionWhenRegenerationFails covers REQ-22.13 «La
// promoción no se revierte si falla la regeneración» (D-12).
func TestApproveKeepsPromotionWhenRegenerationFails(t *testing.T) {
	workspace := t.TempDir()
	writeInboxSkill(t, workspace, "go-testing")

	manager := NewManager(workspace, nil, nil, nil)
	manager.RegenerateIndex = func(cwd, home string) error {
		return errors.New("index write failed")
	}

	outcome, err := manager.Approve("go-testing")
	if err != nil {
		t.Fatalf("Approve() error = %v, want nil: the promotion completed", err)
	}
	if !outcome.Promoted {
		t.Fatalf("outcome = %+v, want promoted", outcome)
	}
	if outcome.RegenerateError == nil || !strings.Contains(outcome.RegenerateError.Error(), "index write failed") {
		t.Fatalf("RegenerateError = %v, want the reported failure", outcome.RegenerateError)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "skills", "go-testing", "SKILL.md")); statErr != nil {
		t.Fatalf("promotion must not be reverted: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, ".axiom", "skills", "inbox", "go-testing")); !os.IsNotExist(statErr) {
		t.Fatalf("inbox must stay empty (stat err = %v)", statErr)
	}
}

// TestApprovePromotionFailureSkipsRegenerator covers the hard rule of D-12: a
// failed promotion returns an error and never runs the regenerator.
func TestApprovePromotionFailureSkipsRegenerator(t *testing.T) {
	workspace := t.TempDir()

	var calls int
	manager := NewManager(workspace, nil, nil, nil)
	manager.RegenerateIndex = func(cwd, home string) error {
		calls++
		return nil
	}

	outcome, err := manager.Approve("no-existe")
	if err == nil {
		t.Fatal("Approve() for a missing proposal must fail")
	}
	if outcome.Promoted {
		t.Fatalf("outcome = %+v, want nothing promoted", outcome)
	}
	if calls != 0 {
		t.Fatalf("regenerator called %d times, want 0 on promotion failure", calls)
	}
}

// TestApproveWithoutRegeneratorReportsTheFailure covers the nil seam of D-12:
// a missing regenerator is reported, never silently accepted.
func TestApproveWithoutRegeneratorReportsTheFailure(t *testing.T) {
	workspace := t.TempDir()
	writeInboxSkill(t, workspace, "go-testing")

	manager := NewManager(workspace, nil, nil, nil)
	outcome, err := manager.Approve("go-testing")
	if err != nil {
		t.Fatalf("Approve() error = %v, want nil", err)
	}
	if !outcome.Promoted {
		t.Fatalf("outcome = %+v, want promoted", outcome)
	}
	if outcome.RegenerateError == nil {
		t.Fatal("a nil regenerator must be reported as a regeneration failure")
	}
}

// TestRejectNeverRegenerates covers REQ-22.13 «Reject no dispara
// regeneración».
func TestRejectNeverRegenerates(t *testing.T) {
	workspace := t.TempDir()
	writeInboxSkill(t, workspace, "go-testing")
	before := filepath.Join(workspace, "skills")

	var calls int
	manager := NewManager(workspace, nil, nil, nil)
	manager.RegenerateIndex = func(cwd, home string) error {
		calls++
		return nil
	}

	if err := manager.Reject("go-testing"); err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("regenerator called %d times, want 0 for Reject", calls)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, ".axiom", "skills", "inbox", "go-testing")); !os.IsNotExist(statErr) {
		t.Fatalf("inbox proposal must be purged (stat err = %v)", statErr)
	}
	if _, statErr := os.Stat(before); statErr == nil {
		// skills/ must not appear: no promotion and no index write.
		t.Fatal("Reject must not create the skills directory")
	}
}
