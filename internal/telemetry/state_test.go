package telemetry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingStateReturnsNotExist(t *testing.T) {
	home := t.TempDir()
	if _, err := Load(home); !os.IsNotExist(err) {
		t.Fatalf("Load() on a missing state file = %v, want os.ErrNotExist; Load must never mint or persist an id", err)
	}
	if _, err := os.Stat(Path(home)); !os.IsNotExist(err) {
		t.Fatal("Load must never create the state file as a side effect")
	}
}

func TestEnsureStateCreatesAndPersistsOnFirstCall(t *testing.T) {
	home := t.TempDir()
	s, err := EnsureState(home)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled {
		t.Fatal("a fresh state must default to enabled")
	}
	if s.InstallID == "" {
		t.Fatal("a fresh state must carry a generated install_id")
	}
	if _, err := os.Stat(Path(home)); err != nil {
		t.Fatalf("EnsureState must persist the fresh state: %v", err)
	}
}

func TestEnsureStateIsStableAcrossConsecutiveCalls(t *testing.T) {
	home := t.TempDir()
	first, err := EnsureState(home)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		again, err := EnsureState(home)
		if err != nil {
			t.Fatal(err)
		}
		if again.InstallID != first.InstallID {
			t.Fatalf("call %d: install_id = %q, want stable %q", i, again.InstallID, first.InstallID)
		}
	}
}

func TestEnsureStateFillsMissingIDOnCorruptLegacyFileWithoutLosingOtherFields(t *testing.T) {
	home := t.TempDir()
	path := Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"enabled":false,"notice_shown":true,"counters":{"syncs":5,"sdd_phase_runs":0,"reviews_approved":0,"reviews_correction":0,"reviews_escalated":0}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := EnsureState(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.InstallID == "" {
		t.Fatal("EnsureState must mint an id for a legacy file that carries none")
	}
	if s.Enabled || !s.NoticeShown || s.Counters.Syncs != 5 {
		t.Fatalf("EnsureState must preserve other fields while filling in the id: %+v", s)
	}
}

func TestLoadLegacyStateWithoutEnabledKeyDefaultsToEnabled(t *testing.T) {
	home := t.TempDir()
	path := Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"install_id":"legacy-id","notice_shown":true}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled {
		t.Fatal("a legacy state file with no \"enabled\" key must default to enabled")
	}
	if s.InstallID != "legacy-id" {
		t.Fatalf("install_id = %q, want preserved legacy-id", s.InstallID)
	}
}

func TestSaveThenLoadRoundTripsExplicitDisable(t *testing.T) {
	home := t.TempDir()
	if err := Save(home, State{InstallID: "id-1", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Enabled {
		t.Fatal("an explicit enabled:false must round-trip as disabled")
	}
}

func TestNewInstallIDsAreDistinctUUIDv4(t *testing.T) {
	a, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewState()
	if err != nil {
		t.Fatal(err)
	}
	if a.InstallID == b.InstallID {
		t.Fatal("two fresh states must not share an install_id")
	}
	if a.InstallID[14] != '4' {
		t.Fatalf("install_id %q is not a version-4 UUID", a.InstallID)
	}
}

func TestPathIsNextToGentleAIStateDir(t *testing.T) {
	home := t.TempDir()
	got := Path(home)
	want := filepath.Join(home, ".gentle-ai", "telemetry.json")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
