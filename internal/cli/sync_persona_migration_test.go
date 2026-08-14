package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestReadSyncStateSnapshotResolvesOnlyValidStateAndSelection(t *testing.T) {
	const alias = string(model.PersonaGentlemanNeutralArtifacts)
	tests := []struct {
		name          string
		stateJSON     string
		explicit      model.PersonaID
		want          model.PersonaID
		wantMigration bool
		wantErr       string
	}{
		{name: "missing state defaults neutral", want: model.PersonaNeutral},
		{name: "omitted persona defaults neutral", stateJSON: `{}`, want: model.PersonaNeutral},
		{name: "canonical gentleman", stateJSON: `{"persona":"gentleman"}`, want: model.PersonaGentleman},
		{name: "canonical neutral case variant", stateJSON: `{"Persona":"neutral"}`, want: model.PersonaNeutral},
		{name: "canonical custom", stateJSON: `{"persona":"custom"}`, want: model.PersonaCustom},
		{name: "legacy alias", stateJSON: `{"persona":"` + alias + `"}`, want: model.PersonaNeutral, wantMigration: true},
		{name: "explicit wins over valid state", stateJSON: `{"persona":"gentleman"}`, explicit: model.PersonaCustom, want: model.PersonaCustom},
		{name: "explicit wins with missing state", explicit: model.PersonaGentleman, want: model.PersonaGentleman},
		{name: "invalid state rejects despite explicit", stateJSON: `{"persona":"unknown"}`, explicit: model.PersonaNeutral, wantErr: `unsupported persona "unknown"`},
		{name: "unsupported explicit rejects", stateJSON: `{}`, explicit: model.PersonaID("unknown"), wantErr: `unsupported persona "unknown"`},
		{name: "whitespace explicit rejects", stateJSON: `{}`, explicit: model.PersonaID("  "), wantErr: "whitespace-only persona"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.stateJSON != "" {
				mustWriteFile(t, state.Path(home), []byte(tt.stateJSON))
			}
			got, err := readSyncStateSnapshot(home, tt.explicit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readSyncStateSnapshot() error = %v, want %q", err, tt.wantErr)
				}
				if err != nil && (!strings.Contains(err.Error(), state.Path(home)) || !strings.Contains(err.Error(), "gentle-ai sync")) {
					t.Fatalf("error is not actionable: %v", err)
				}
				return
			}
			if err != nil || got.persona != tt.want || got.migratePersonaAlias != tt.wantMigration {
				t.Fatalf("snapshot = %#v, error = %v; want persona %q, migration %t", got, err, tt.want, tt.wantMigration)
			}
		})
	}
}

func TestMigratePersistedPersonaAliasRewritesLatestStateOnce(t *testing.T) {
	var notices bytes.Buffer
	previous := personaNoticeWriter
	personaNoticeWriter = &notices
	t.Cleanup(func() { personaNoticeWriter = previous })

	home := t.TempDir()
	seed := state.InstallState{
		InstalledAgents: []string{"opencode"},
		Persona:         string(model.PersonaGentlemanNeutralArtifacts),
		RDDMode:         "off",
	}
	if err := state.Write(home, seed); err != nil {
		t.Fatal(err)
	}

	if err := migratePersistedPersonaAlias(home, true); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Persona != string(model.PersonaNeutral) || got.RDDMode != "off" || len(got.InstalledAgents) != 1 {
		t.Fatalf("migrated state lost data: %#v", got)
	}
	if strings.Count(notices.String(), personaAliasRemapNotice) != 1 {
		t.Fatalf("migration notices = %q, want one", notices.String())
	}

	if err := migratePersistedPersonaAlias(home, true); err != nil {
		t.Fatal(err)
	}
	if strings.Count(notices.String(), personaAliasRemapNotice) != 1 {
		t.Fatalf("second migration notice = %q, want once-only", notices.String())
	}
}

func TestMigratePersistedPersonaAliasCompareAndSetPreservesConcurrentState(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}); err != nil {
		t.Fatal(err)
	}
	lock, err := reviewtransaction.AcquireAuthorityFileLock(installStateLockPath(home))
	if err != nil {
		t.Fatal(err)
	}

	if err := migratePersistedPersonaAlias(home, true); err == nil || !strings.Contains(err.Error(), "install state lock") {
		_ = lock.Release()
		t.Fatalf("migration during lock contention error = %v, want safe lock refusal", err)
	}
	stillAlias, err := state.Read(home)
	if err != nil || stillAlias.Persona != string(model.PersonaGentlemanNeutralArtifacts) {
		_ = lock.Release()
		t.Fatalf("contended migration changed state: %#v, error = %v", stillAlias, err)
	}

	concurrent := state.InstallState{InstalledAgents: []string{"codex"}, Persona: string(model.PersonaGentleman), RDDMode: "off"}
	if err := state.Write(home, concurrent); err != nil {
		_ = lock.Release()
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := migratePersistedPersonaAlias(home, true); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Persona != concurrent.Persona || got.RDDMode != concurrent.RDDMode || len(got.InstalledAgents) != 1 || got.InstalledAgents[0] != "codex" {
		t.Fatalf("migration overwrote concurrent state: got %#v, want %#v", got, concurrent)
	}
}

func TestMigratePersistedPersonaAliasPreservesConcurrentUnrelatedUpdate(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		Persona: string(model.PersonaGentlemanNeutralArtifacts),
		RDDMode: "on",
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readSyncStateSnapshot(home, "")
	if err != nil || !snapshot.migratePersonaAlias {
		t.Fatalf("initial migration intent = %t, error = %v", snapshot.migratePersonaAlias, err)
	}

	concurrent := state.InstallState{
		InstalledAgents: []string{"codex"},
		Persona:         string(model.PersonaGentlemanNeutralArtifacts),
		RDDMode:         "off",
		PendingSync:     true,
	}
	if err := state.Write(home, concurrent); err != nil {
		t.Fatal(err)
	}
	if err := migratePersistedPersonaAlias(home, snapshot.migratePersonaAlias); err != nil {
		t.Fatal(err)
	}

	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Persona != string(model.PersonaNeutral) || got.RDDMode != concurrent.RDDMode || got.PendingSync != concurrent.PendingSync || len(got.InstalledAgents) != 1 || got.InstalledAgents[0] != "codex" {
		t.Fatalf("migration did not preserve concurrent unrelated update: got %#v, want alias-only change to %#v", got, concurrent)
	}
}

func TestRunSyncWithSelectionMigratesAliasOnNoAgentSync(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}); err != nil {
		t.Fatal(err)
	}
	result, err := RunSyncWithSelection(home, model.Selection{})
	if err != nil || !result.NoOp {
		t.Fatalf("no-agent sync = %#v, error = %v", result, err)
	}
	got, err := state.Read(home)
	if err != nil || got.Persona != string(model.PersonaNeutral) {
		t.Fatalf("persisted state = %#v, error = %v", got, err)
	}
}

func TestRunSyncMigratesAliasOnNoAgentNormalSync(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	if err := state.Write(home, state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}); err != nil {
		t.Fatal(err)
	}
	result, err := RunSync(nil)
	if err != nil || !result.NoOp || result.DryRun {
		t.Fatalf("normal no-agent sync = %#v, error = %v", result, err)
	}
	got, err := state.Read(home)
	if err != nil || got.Persona != string(model.PersonaNeutral) {
		t.Fatalf("persisted state = %#v, error = %v", got, err)
	}
}

func TestRunSyncDryRunNeverMigratesPersonaAlias(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	if err := os.MkdirAll(filepath.Dir(state.Path(home)), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"persona":"gentleman-neutral-artifacts","unrelated":"preserved"}`)
	if err := os.WriteFile(state.Path(home), original, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSync([]string{"--dry-run"})
	if err != nil || !result.DryRun {
		t.Fatalf("dry-run result = %#v, error = %v", result, err)
	}
	got, err := os.ReadFile(state.Path(home))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("dry-run state = %q, error = %v; want original %q", got, err, original)
	}
}
