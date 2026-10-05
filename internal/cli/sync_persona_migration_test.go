package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

var errStateUnreadableForTest = errors.New("state unreadable")

func TestMigratePersistedPersonaAliasRewritesStateOnce(t *testing.T) {
	var buf bytes.Buffer
	previous := personaNoticeWriter
	personaNoticeWriter = &buf
	defer func() { personaNoticeWriter = previous }()

	homeDir := t.TempDir()
	persisted := state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}
	if err := state.Write(homeDir, persisted); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	if err := migratePersistedPersonaAlias(homeDir, &persisted, nil); err != nil {
		t.Fatalf("migratePersistedPersonaAlias() error = %v", err)
	}

	reread, err := state.Read(homeDir)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if reread.Persona != string(model.PersonaNeutral) {
		t.Fatalf("persisted persona = %q, want %q", reread.Persona, model.PersonaNeutral)
	}
	if !strings.Contains(buf.String(), personaAliasRemapNotice) {
		t.Fatalf("notice not printed; got %q", buf.String())
	}

	// Second run: state already neutral — no notice, no rewrite.
	buf.Reset()
	if err := migratePersistedPersonaAlias(homeDir, &reread, nil); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("second run printed %q, want silence", buf.String())
	}
}

// TestMigratePersistedPersonaAliasReReadsLatestStateAfterLockContention is the
// #1809 preservation proof for the persona alias migration: the advisory
// snapshot is read before the lock, so the rewrite must re-read the latest
// state inside the canonical lock instead of writing the stale snapshot back.
// A concurrent writer's RDDMode must survive the remap, and a contended lock
// must fail fast without writing anything or printing the notice.
func TestMigratePersistedPersonaAliasReReadsLatestStateAfterLockContention(t *testing.T) {
	var buf bytes.Buffer
	previous := personaNoticeWriter
	personaNoticeWriter = &buf
	defer func() { personaNoticeWriter = previous }()

	home := t.TempDir()
	// Advisory snapshot taken before the lock exists: it still shows the
	// legacy alias, but is stale by the time the migration runs.
	persisted := state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}

	held, err := reviewtransaction.AcquireAuthorityFileLock(mustInstallStateLockPath(t, home))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{
		Persona: string(model.PersonaGentlemanNeutralArtifacts),
		RDDMode: string(reviewtransaction.RDDModeOn),
	}); err != nil {
		t.Fatal(err)
	}
	if err := migratePersistedPersonaAlias(home, &persisted, nil); err == nil || !strings.Contains(err.Error(), "acquire install state lock") {
		t.Fatalf("contended migration error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("contended run printed %q, want silence", buf.String())
	}
	contended, err := state.Read(home)
	if err != nil || contended.Persona != string(model.PersonaGentlemanNeutralArtifacts) || contended.RDDMode != string(reviewtransaction.RDDModeOn) {
		t.Fatalf("state after contended run = %#v, err = %v", contended, err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	// The advisory snapshot predates the concurrent RDDMode write. The
	// migration must preserve that concurrent field and still remap the alias.
	if err := migratePersistedPersonaAlias(home, &persisted, nil); err != nil {
		t.Fatalf("migratePersistedPersonaAlias() error = %v", err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if got.Persona != string(model.PersonaNeutral) {
		t.Fatalf("persisted persona = %q, want %q", got.Persona, model.PersonaNeutral)
	}
	if got.RDDMode != string(reviewtransaction.RDDModeOn) {
		t.Fatalf("concurrent RDDMode clobbered: got %q, want %q", got.RDDMode, reviewtransaction.RDDModeOn)
	}
	if !strings.Contains(buf.String(), personaAliasRemapNotice) {
		t.Fatalf("notice not printed; got %q", buf.String())
	}
}

func TestMigratePersistedPersonaAliasSkipsUnreadableState(t *testing.T) {
	persisted := state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}
	if err := migratePersistedPersonaAlias(t.TempDir(), &persisted, errStateUnreadableForTest); err != nil {
		t.Fatalf("migrate with read error must be a no-op, got %v", err)
	}
}

// TestApplyResolvedPersonaAliasResolution covers only what this change owns:
// a persisted legacy alias resolves to neutral, valid persisted personas are
// honored unchanged, an explicit selection wins over persisted state, and the
// documented missing-field compatibility default still applies to state files
// written before persona persistence existed.
//
// Resolution of unknown or unreadable persisted state is deliberately NOT
// asserted here. That policy belongs to issue #1677, which makes it fail
// closed; pinning today's fail-open behavior would codify a contract this
// change does not intend to define.
func TestApplyResolvedPersonaAliasResolution(t *testing.T) {
	cases := []struct {
		name      string
		selection model.Selection
		persisted string
		want      model.PersonaID
	}{
		{name: "persisted legacy alias resolves to neutral", persisted: string(model.PersonaGentlemanNeutralArtifacts), want: model.PersonaNeutral},
		{name: "persisted gentleman is honored", persisted: string(model.PersonaGentleman), want: model.PersonaGentleman},
		{name: "persisted neutral is honored", persisted: string(model.PersonaNeutral), want: model.PersonaNeutral},
		{name: "explicit selection wins over persisted alias", selection: model.Selection{Persona: model.PersonaGentleman}, persisted: string(model.PersonaGentlemanNeutralArtifacts), want: model.PersonaGentleman},
		{name: "missing persona field uses the documented compatibility default", persisted: "", want: model.PersonaNeutral},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selection := tc.selection
			applyResolvedPersona(&selection, tc.persisted)
			if selection.Persona != tc.want {
				t.Fatalf("applyResolvedPersona(persisted=%q) = %q, want %q", tc.persisted, selection.Persona, tc.want)
			}
		})
	}
}

// TestRunSyncWithSelectionMigratesAliasOnNoOpSync pins the early-return path:
// a sync that discovers zero agents must still migrate a persisted legacy
// alias, otherwise the one-time migration never fires for those users.
func TestRunSyncWithSelectionMigratesAliasOnNoOpSync(t *testing.T) {
	var buf bytes.Buffer
	previous := personaNoticeWriter
	personaNoticeWriter = &buf
	defer func() { personaNoticeWriter = previous }()

	homeDir := t.TempDir()
	if err := state.Write(homeDir, state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	result, err := RunSyncWithSelection(homeDir, model.Selection{})
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}
	if !result.NoOp {
		t.Fatal("zero-agent sync must report NoOp")
	}

	reread, err := state.Read(homeDir)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if reread.Persona != string(model.PersonaNeutral) {
		t.Fatalf("persisted persona = %q, want %q after no-op sync", reread.Persona, model.PersonaNeutral)
	}
	if !strings.Contains(buf.String(), personaAliasRemapNotice) {
		t.Fatalf("notice not printed on no-op sync; got %q", buf.String())
	}
}
