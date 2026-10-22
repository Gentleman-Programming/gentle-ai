package pi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestCodeGraphManifestHonorsGentleAIHome(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "gentle-ai")
	t.Setenv(state.HomeEnv, override)

	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got, want := CodeGraphPaths(home).Manifest, filepath.Join(override, "pi-codegraph.json"); got != want {
		t.Fatalf("default agent dir Manifest = %q, want %q", got, want)
	}

	agentDir := filepath.Join(home, "isolated", "agent")
	t.Setenv("PI_CODING_AGENT_DIR", "~/isolated/agent")
	got := CodeGraphPaths(home).Manifest
	if filepath.Dir(got) != override || !strings.HasPrefix(filepath.Base(got), "pi-codegraph-") {
		t.Fatalf("isolated agent dir Manifest = %q, want pi-codegraph-<hash>.json under %q (agent dir %q)", got, override, agentDir)
	}
}

func TestGentlePiConfigDirRules(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(t.TempDir(), "config")

	t.Setenv(GentlePiConfigDirEnv, "")
	if got, want := GentlePiConfigDir(home), filepath.Join(home, ".pi", "gentle-ai"); got != want {
		t.Fatalf("default GentlePiConfigDir() = %q, want %q", got, want)
	}
	if err := ValidateGentlePiConfigDirEnv(); err != nil {
		t.Fatalf("ValidateGentlePiConfigDirEnv() unset = %v", err)
	}

	t.Setenv(GentlePiConfigDirEnv, dir)
	if got := GentlePiConfigDir(home); got != dir {
		t.Fatalf("GentlePiConfigDir() = %q, want %q", got, dir)
	}

	t.Setenv(GentlePiConfigDirEnv, "relative/config")
	err := ValidateGentlePiConfigDirEnv()
	if err == nil || !strings.Contains(err.Error(), GentlePiConfigDirEnv) {
		t.Fatalf("ValidateGentlePiConfigDirEnv() relative = %v, want an error naming %s", err, GentlePiConfigDirEnv)
	}
	if got, want := GentlePiConfigDir(home), filepath.Join(home, ".pi", "gentle-ai"); got != want {
		t.Fatalf("relative value must never be used: got %q, want %q", got, want)
	}
}
