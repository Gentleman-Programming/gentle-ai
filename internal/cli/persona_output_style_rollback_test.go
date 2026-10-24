package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type personaInjectFunc func(string, agents.Adapter, model.PersonaID, string) (persona.InjectionResult, error)

var errInjectedRetiredStyleRemoval = errors.New("injected retired output style removal failure")

// outputStyleTransitionFixture is a Claude home with the Gentleman persona
// applied, plus a user-owned output style and an unrelated settings key.
type outputStyleTransitionFixture struct {
	home      string
	gentleman string
	neutral   string
	settings  string
	before    map[string][]byte
}

func newOutputStyleTransitionFixture(t *testing.T, home string) outputStyleTransitionFixture {
	t.Helper()
	claudeDir := filepath.Join(home, ".claude")
	f := outputStyleTransitionFixture{
		home:      home,
		gentleman: filepath.Join(claudeDir, "output-styles", "gentleman.md"),
		neutral:   filepath.Join(claudeDir, "output-styles", "neutral.md"),
		settings:  filepath.Join(claudeDir, "settings.json"),
	}
	userStyle := filepath.Join(claudeDir, "output-styles", "user-owned.md")
	mustWriteFile(t, f.settings, []byte(`{"permissions":{"allow":["Read"]}}`))
	mustWriteFile(t, userStyle, []byte("user-owned style\n"))

	if _, err := persona.Inject(home, claude.NewAdapter(), model.PersonaGentleman); err != nil {
		t.Fatalf("Inject(gentleman) error = %v", err)
	}

	f.before = map[string][]byte{}
	for _, path := range []string{f.gentleman, f.settings, userStyle, filepath.Join(claudeDir, "CLAUDE.md")} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("precondition: read %s: %v", path, err)
		}
		f.before[path] = content
	}
	if !strings.Contains(string(f.before[f.settings]), `"outputStyle": "Gentleman"`) {
		t.Fatalf("precondition: settings do not select Gentleman: %s", f.before[f.settings])
	}
	if _, err := os.Stat(f.neutral); !os.IsNotExist(err) {
		t.Fatalf("precondition: neutral output style should be absent, stat err=%v", err)
	}
	return f
}

// failRetiredGentlemanRemoval runs the real persona injection, then reproduces
// the observable state of a failed retired-style removal: the selected Neutral
// style and settings are already published, gentleman.md is still present, and
// the injection reports an error.
func (f outputStyleTransitionFixture) failRetiredGentlemanRemoval(t *testing.T, inject personaInjectFunc, reached *bool) personaInjectFunc {
	t.Helper()
	return func(home string, adapter agents.Adapter, id model.PersonaID, selectedSettingsPath string) (persona.InjectionResult, error) {
		if adapter.Agent() != model.AgentClaudeCode {
			return inject(home, adapter, id, selectedSettingsPath)
		}
		if _, err := inject(home, adapter, id, selectedSettingsPath); err != nil {
			return persona.InjectionResult{}, err
		}
		settings, err := os.ReadFile(f.settings)
		if err != nil {
			return persona.InjectionResult{}, err
		}
		if _, err := os.Stat(f.neutral); err == nil && strings.Contains(string(settings), `"outputStyle": "Neutral"`) {
			*reached = true
		}
		if err := os.WriteFile(f.gentleman, f.before[f.gentleman], 0o644); err != nil {
			return persona.InjectionResult{}, err
		}
		return persona.InjectionResult{}, fmt.Errorf("remove retired output style: %w", errInjectedRetiredStyleRemoval)
	}
}

func (f outputStyleTransitionFixture) assertRestored(t *testing.T) {
	t.Helper()
	for path, want := range f.before {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("after rollback: read %s: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("after rollback %s = %q, want exact before-image %q", path, got, want)
		}
	}
	if _, err := os.Stat(f.neutral); !os.IsNotExist(err) {
		t.Fatalf("after rollback a partially published neutral output style remains, stat err=%v", err)
	}
}

func TestInstallRollsBackOutputStyleSwitchWhenRetiredStyleRemovalFails(t *testing.T) {
	home := installTestHome(t)
	f := newOutputStyleTransitionFixture(t, home)

	reached := false
	original := injectInstallPersona
	injectInstallPersona = f.failRetiredGentlemanRemoval(t, original, &reached)
	t.Cleanup(func() { injectInstallPersona = original })

	_, err := RunInstall([]string{"--agent", "claude-code", "--component", "persona", "--persona", "neutral"}, system.DetectionResult{})
	if !errors.Is(err, errInjectedRetiredStyleRemoval) {
		t.Fatalf("RunInstall() error = %v, want injected retired style removal failure", err)
	}
	if !reached {
		t.Fatal("injection never published the Neutral style and settings before failing")
	}
	f.assertRestored(t)
}

func TestSyncRollsBackOutputStyleSwitchWhenRetiredStyleRemovalFails(t *testing.T) {
	f := newOutputStyleTransitionFixture(t, t.TempDir())
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentPersona},
		Persona:    model.PersonaNeutral,
	}

	reached := false
	original := injectSyncPersona
	injectSyncPersona = f.failRetiredGentlemanRemoval(t, original, &reached)
	t.Cleanup(func() { injectSyncPersona = original })

	if _, err := RunSyncWithSelection(f.home, selection); !errors.Is(err, errInjectedRetiredStyleRemoval) {
		t.Fatalf("RunSyncWithSelection() error = %v, want injected retired style removal failure", err)
	}
	if !reached {
		t.Fatal("injection never published the Neutral style and settings before failing")
	}
	f.assertRestored(t)

	// Once removal succeeds, the switch completes and a rerun is a no-op.
	injectSyncPersona = original
	if _, err := RunSyncWithSelection(f.home, selection); err != nil {
		t.Fatalf("recovery RunSyncWithSelection() error = %v", err)
	}
	if _, err := os.Stat(f.gentleman); !os.IsNotExist(err) {
		t.Fatalf("recovery sync left retired gentleman output style, stat err=%v", err)
	}
	if _, err := os.Stat(f.neutral); err != nil {
		t.Fatalf("recovery sync did not publish neutral output style: %v", err)
	}
	settled := map[string][]byte{}
	for path := range f.before {
		if path == f.gentleman {
			continue
		}
		settled[path] = nil
	}
	settled[f.neutral] = nil
	for path := range settled {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("after recovery: read %s: %v", path, err)
		}
		settled[path] = content
	}
	second, err := RunSyncWithSelection(f.home, selection)
	if err != nil {
		t.Fatalf("second RunSyncWithSelection() error = %v", err)
	}
	if second.FilesChanged != 0 || !second.NoOp {
		t.Fatalf("second sync files changed = %d, no-op = %t; want idempotent no-op", second.FilesChanged, second.NoOp)
	}
	if _, err := os.Stat(f.gentleman); !os.IsNotExist(err) {
		t.Fatalf("second sync restored retired gentleman output style, stat err=%v", err)
	}
	for path, want := range settled {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("after second sync: read %s: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("second sync changed %s = %q, want %q", path, got, want)
		}
	}
}
