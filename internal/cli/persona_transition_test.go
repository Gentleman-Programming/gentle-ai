package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// personaTransitionTestEnv holds the shared Gentleman-installed fixture for
// the CLI e2e rollback tests. Setup mirrors TestPersonaSyncOutputStyleSwitchIs
// Idempotent (sync_test.go:3988) plus the osUserHomeDir/backup.UserHomeDirFn
// overrides from sync_review_retirement_test.go:52-55.
type personaTransitionTestEnv struct {
	home           string
	gentlemanPath  string
	settingsPath   string
	neutralPath    string
	userFilePath   string
	gentlemanBytes []byte
	settingsBytes  []byte
	userFileBytes  []byte
	removeCalls    int
}

func setupPersonaTransitionTestEnv(t *testing.T) *personaTransitionTestEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	env := &personaTransitionTestEnv{home: home}
	env.gentlemanPath = filepath.Join(home, ".claude", "output-styles", "gentleman.md")
	env.settingsPath = filepath.Join(home, ".claude", "settings.json")
	env.neutralPath = filepath.Join(home, ".claude", "output-styles", "neutral.md")

	// Pre-existing settings with an unrelated user key must survive both the
	// transition and the rollback (SEN-USER-FILE-PRESERVED).
	mustWriteFile(t, env.settingsPath, []byte("{\"theme\":\"dark\"}\n"))

	// Install Gentleman with the default seam: writes gentleman.md and selects it.
	if _, err := persona.Inject(home, claude.NewAdapter(), model.PersonaGentleman); err != nil {
		t.Fatalf("persona.Inject(gentleman) error = %v", err)
	}

	var err error
	env.gentlemanBytes, err = os.ReadFile(env.gentlemanPath)
	if err != nil {
		t.Fatalf("precondition: ReadFile(gentleman.md) error = %v", err)
	}
	env.settingsBytes, err = os.ReadFile(env.settingsPath)
	if err != nil {
		t.Fatalf("precondition: ReadFile(settings.json) error = %v", err)
	}
	var settingsBefore map[string]any
	if err := json.Unmarshal(env.settingsBytes, &settingsBefore); err != nil {
		t.Fatalf("precondition: Unmarshal(settings.json) error = %v", err)
	}
	if settingsBefore["theme"] != "dark" {
		t.Fatalf("precondition: settings.json lost the user key; got:\n%s", env.settingsBytes)
	}
	if settingsBefore["outputStyle"] != "Gentleman" {
		t.Fatalf("precondition: settings.json does not select Gentleman; got:\n%s", env.settingsBytes)
	}

	// A user-owned style file next to the managed ones — never touched.
	env.userFilePath = filepath.Join(home, ".claude", "output-styles", "user-custom.md")
	env.userFileBytes = []byte("# user-authored style notes\n")
	mustWriteFile(t, env.userFilePath, env.userFileBytes)

	return env
}

// injectRemovalFailure overrides the exported persona removal seam with a
// failing func, counting invocations to pin REQ-NO-RETRY (at most one attempt).
func injectRemovalFailure(t *testing.T, env *personaTransitionTestEnv) {
	t.Helper()
	original := persona.RemoveFileFn
	persona.RemoveFileFn = func(string) error {
		env.removeCalls++
		return errors.New("simulated removal failure: file locked")
	}
	t.Cleanup(func() { persona.RemoveFileFn = original })
}

// assertRolledBackPersonaTransition verifies the observable post-rollback
// state required by REQ-ROLLBACK-PROPAGATION, REQ-EXIT-WARNING, REQ-NO-RETRY
// and REQ-USER-FILES: gentleman.md + settings.json byte-for-byte restored, no
// partial neutral.md, user file untouched, warning on stderr, exactly one
// removal attempt.
func assertRolledBackPersonaTransition(t *testing.T, env *personaTransitionTestEnv, stderr string) {
	t.Helper()

	got, err := os.ReadFile(env.gentlemanPath)
	if err != nil || !bytes.Equal(got, env.gentlemanBytes) {
		t.Errorf("gentleman.md not restored byte-for-byte: got=%q error=%v", got, err)
	}
	gotSettings, err := os.ReadFile(env.settingsPath)
	if err != nil || !bytes.Equal(gotSettings, env.settingsBytes) {
		t.Errorf("settings.json not restored byte-for-byte: got=%q error=%v", gotSettings, err)
	}
	if _, statErr := os.Stat(env.neutralPath); !os.IsNotExist(statErr) {
		t.Errorf("partial neutral.md remains after rollback: %v", statErr)
	}
	gotUser, err := os.ReadFile(env.userFilePath)
	if err != nil || !bytes.Equal(gotUser, env.userFileBytes) {
		t.Errorf("user-owned file not preserved: got=%q error=%v", gotUser, err)
	}
	if !strings.Contains(stderr, persona.MessageRolledBackOutputStyle) {
		t.Errorf("stderr missing rollback warning; got:\n%s", stderr)
	}
	if !strings.Contains(stderr, env.gentlemanPath) {
		t.Errorf("stderr missing restored retired style path %q; got:\n%s", env.gentlemanPath, stderr)
	}
	if !strings.Contains(stderr, env.settingsPath) {
		t.Errorf("stderr missing restored settings path %q; got:\n%s", env.settingsPath, stderr)
	}
	if env.removeCalls != 1 {
		t.Errorf("removal seam called %d times, want exactly 1 (REQ-NO-RETRY)", env.removeCalls)
	}
}

// TestPersonaOutputStyleTransitionRollbackInstall is the install-side e2e
// (SEN-ROLLBACK-RESTORES, SEN-NO-PARTIAL-STATE, SEN-EXIT-ZERO-ON-ROLLBACK,
// SEN-WARNING-EXPLAINS-ROLLBACK, SEN-NO-RETRY-LOOP, SEN-USER-FILE-PRESERVED):
// a failing removal seam during a Gentleman→Neutral install must roll back
// through the pipeline boundary and surface as warning + exit 0.
func TestPersonaOutputStyleTransitionRollbackInstall(t *testing.T) {
	env := setupPersonaTransitionTestEnv(t)
	injectRemovalFailure(t, env)

	var err error
	stderr := captureStderr(t, func() {
		_, err = RunInstall([]string{
			"--agent", "claude-code",
			"--component", "persona",
			"--persona", "neutral",
		}, system.DetectionResult{})
	})
	if err != nil {
		t.Fatalf("RunInstall() error = %v, want nil (rolled-back removal exits 0)", err)
	}
	assertRolledBackPersonaTransition(t, env, stderr)
}

// TestPersonaOutputStyleTransitionRollbackParity pins SEN-INSTALL-SYNC-PARITY:
// install and sync apply the identical transition contract — same removal
// seam, same rollback propagation, same warning + exit 0.
func TestPersonaOutputStyleTransitionRollbackParity(t *testing.T) {
	pipelines := []struct {
		name string
		run  func(t *testing.T, env *personaTransitionTestEnv) error
	}{
		{
			name: "install",
			run: func(t *testing.T, env *personaTransitionTestEnv) error {
				_, err := RunInstall([]string{
					"--agent", "claude-code",
					"--component", "persona",
					"--persona", "neutral",
				}, system.DetectionResult{})
				return err
			},
		},
		{
			name: "sync",
			run: func(t *testing.T, env *personaTransitionTestEnv) error {
				_, err := RunSyncWithSelection(env.home, model.Selection{
					Agents:     []model.AgentID{model.AgentClaudeCode},
					Components: []model.ComponentID{model.ComponentPersona},
					Persona:    model.PersonaNeutral,
				})
				return err
			},
		},
	}

	for _, pipeline := range pipelines {
		t.Run(pipeline.name, func(t *testing.T) {
			env := setupPersonaTransitionTestEnv(t)
			injectRemovalFailure(t, env)

			var err error
			stderr := captureStderr(t, func() {
				err = pipeline.run(t, env)
			})
			if err != nil {
				t.Fatalf("%s pipeline error = %v, want nil (rolled-back removal exits 0)", pipeline.name, err)
			}
			assertRolledBackPersonaTransition(t, env, stderr)
		})
	}
}
