package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestPathDefaultsToDotGentleAIUnderHome(t *testing.T) {
	t.Setenv(state.HomeEnv, "")
	home := t.TempDir()

	if got, want := Path(home), filepath.Join(home, ".gentle-ai", "telemetry.json"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestHomeEnvRoutesTelemetryStateAndLock(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "gentle-ai")
	t.Setenv(state.HomeEnv, override)

	if err := Update(home, func(s *State) { s.Counters.Syncs++ }); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	for _, name := range []string{"telemetry.json", "telemetry.json.lock"} {
		if _, err := os.Stat(filepath.Join(override, name)); err != nil {
			t.Fatalf("%s not written under %s: %v", name, state.HomeEnv, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".gentle-ai")); !os.IsNotExist(err) {
		t.Fatalf("~/.gentle-ai must not be created when %s is set (stat err = %v)", state.HomeEnv, err)
	}
}
