package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type personaRollbackFixture struct {
	home, gentleman, neutral, settings, unrelated    string
	gentlemanBefore, settingsBefore, unrelatedBefore []byte
}

func newPersonaRollbackFixture(t *testing.T) personaRollbackFixture {
	t.Helper()
	f := personaRollbackFixture{
		home:            t.TempDir(),
		gentleman:       "",
		neutral:         "",
		settings:        "",
		unrelated:       "",
		gentlemanBefore: []byte("original gentleman style\n"),
		settingsBefore:  []byte("{\n  \"outputStyle\": \"Gentleman\",\n  \"permissions\": {\"allow\": [\"Read\"]}\n}\n"),
		unrelatedBefore: []byte("user-owned file; do not touch\n"),
	}
	styleDir := filepath.Join(f.home, ".claude", "output-styles")
	f.gentleman = filepath.Join(styleDir, "gentleman.md")
	f.neutral = filepath.Join(styleDir, "neutral.md")
	f.settings = filepath.Join(f.home, ".claude", "settings.json")
	f.unrelated = filepath.Join(f.home, ".claude", "user-owned.txt")
	if err := os.MkdirAll(styleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		f.gentleman: f.gentlemanBefore,
		f.settings:  f.settingsBefore,
		f.unrelated: f.unrelatedBefore,
	} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("seed %q: %v", path, err)
		}
	}
	return f
}

func failRetiredOutputStyleRemoval(t *testing.T, retiredPath string) error {
	t.Helper()
	cause := errors.New("forced retired output style removal failure")
	t.Cleanup(persona.SetRetiredOutputStyleRemoverForTest(func(path string) (bool, error) {
		if path != retiredPath {
			t.Fatalf("retired output style path = %q, want %q", path, retiredPath)
		}
		return false, cause
	}))
	return cause
}

func assertPersonaRollback(t *testing.T, f personaRollbackFixture, execution pipeline.ExecutionResult, cause error) {
	t.Helper()
	if execution.Err == nil {
		t.Fatal("persona transition unexpectedly succeeded")
	}
	if !execution.Rollback.Success {
		t.Fatalf("persona transition rollback = %#v, want success", execution.Rollback)
	}
	var removalErr *persona.RetiredOutputStyleRemovalError
	if !errors.As(execution.Err, &removalErr) {
		t.Fatalf("pipeline error = %v, want retired output style error", execution.Err)
	}
	if removalErr.Path != f.gentleman || !errors.Is(execution.Err, cause) {
		t.Fatalf("pipeline error = %v, want path %q and cause %v", execution.Err, f.gentleman, cause)
	}
	for _, file := range []struct {
		path string
		want []byte
	}{{f.gentleman, f.gentlemanBefore}, {f.settings, f.settingsBefore}, {f.unrelated, f.unrelatedBefore}} {
		got, err := os.ReadFile(file.path)
		if err != nil || !bytes.Equal(got, file.want) {
			t.Fatalf("rollback path %q = %q, read error %v; want exact %q", file.path, got, err, file.want)
		}
	}
	if _, err := os.Stat(f.neutral); !os.IsNotExist(err) {
		t.Fatalf("rollback left newly created neutral style: %v", err)
	}
}

func TestInstallPersonaTransitionRollbackRestoresOutputStyleAndSettings(t *testing.T) {
	f := newPersonaRollbackFixture(t)
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentPersona},
		Persona:    model.PersonaNeutral,
	}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	runtime, err := newInstallRuntime(f.home, ScopeGlobal, ChannelStable, selection, resolved, system.PlatformProfile{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.state.cleanupCompatibilityTransaction)
	cause := failRetiredOutputStyleRemoval(t, f.gentleman)
	plan := runtime.stagePlan()
	// The dependency preflight is unrelated to this transaction boundary.
	plan.Prepare = plan.Prepare[1:]
	execution := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	assertPersonaRollback(t, f, execution, cause)
}

func TestSyncPersonaTransitionRollbackRestoresOutputStyleAndSettings(t *testing.T) {
	f := newPersonaRollbackFixture(t)
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentPersona},
		Persona:    model.PersonaNeutral,
	}
	runtime, err := newSyncRuntime(f.home, selection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.state.cleanupCompatibilityTransaction)
	cause := failRetiredOutputStyleRemoval(t, f.gentleman)
	execution := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(runtime.stagePlan())
	assertPersonaRollback(t, f, execution, cause)
}
