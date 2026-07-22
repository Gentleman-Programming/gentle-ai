package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/state"
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

func TestMigratePersistedPersonaAliasSkipsUnreadableState(t *testing.T) {
	persisted := state.InstallState{Persona: string(model.PersonaGentlemanNeutralArtifacts)}
	if err := migratePersistedPersonaAlias(t.TempDir(), &persisted, errStateUnreadableForTest); err != nil {
		t.Fatalf("migrate with read error must be a no-op, got %v", err)
	}
}
