package persona

import (
	"os"
	"path/filepath"
	"testing"

	piagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestPiGlobalPersonaConfigPathRules(t *testing.T) {
	home := t.TempDir()

	t.Setenv(piagent.GentlePiConfigDirEnv, "")
	if got, want := PiGlobalPersonaConfigPath(home), PiPersonaConfigPath(home); got != want {
		t.Fatalf("default PiGlobalPersonaConfigPath() = %q, want %q", got, want)
	}

	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv(piagent.GentlePiConfigDirEnv, dir)
	want := filepath.Join(dir, "persona.json")
	if got := PiGlobalPersonaConfigPath(home); got != want {
		t.Fatalf("PiGlobalPersonaConfigPath() = %q, want %q", got, want)
	}
	result, err := InjectPiPersonaAt(want, model.PersonaNeutral)
	if err != nil {
		t.Fatalf("InjectPiPersonaAt() error = %v", err)
	}
	if len(result.Files) != 1 || result.Files[0] != want {
		t.Fatalf("InjectPiPersonaAt() files = %v, want [%q]", result.Files, want)
	}
	if raw, err := os.ReadFile(want); err != nil || string(raw) != "{\n  \"mode\": \"neutral\"\n}\n" {
		t.Fatalf("persona.json = %q, %v", raw, err)
	}
}
