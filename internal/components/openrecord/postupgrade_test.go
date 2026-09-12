package openrecord

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// installFakeOpenRecordCLI puts a fake "openrecord" script on PATH that
// answers the one invocation PostUpgrade makes (`openrecord skills --emit
// <staging> --with-qmd`) by writing a single skill file plus its manifest
// into the staging dir it was given — enough for the real fanOut to run
// against, without a real openrecord binary in this sandbox.
func installFakeOpenRecordCLI(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"[ \"$1\" = skills ] && [ \"$2\" = --emit ] && [ \"$4\" = --with-qmd ] || exit 64\n" +
		"staging=\"$3\"\n" +
		"mkdir -p \"$staging/openrecord-consult\"\n" +
		"printf 'consult skill body' > \"$staging/openrecord-consult/SKILL.md\"\n" +
		"printf '%s' '{\"version\":1,\"emitter\":\"openrecord@test\",\"with_qmd\":true,\"entries\":[{\"path\":\"openrecord-consult/SKILL.md\",\"hash\":\"irrelevant\"}]}' > \"$staging/.openrecord-emitted.json\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "openrecord"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeInstallState(t *testing.T, home string, communityTools []string, installedAgents []string) {
	t.Helper()
	path := state.Path(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"installed_agents":[`
	for i, agent := range installedAgents {
		if i > 0 {
			content += ","
		}
		content += `"` + agent + `"`
	}
	content += `],"community_tools":[`
	for i, tool := range communityTools {
		if i > 0 {
			content += ","
		}
		content += `"` + tool + `"`
	}
	content += `]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPostUpgradeSelectionGate pins that PostUpgrade is a no-op — no writes
// at all — unless the persisted install state records openrecord as selected.
func TestPostUpgradeSelectionGate(t *testing.T) {
	t.Run("no persisted state is a no-op", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		if err := PostUpgrade(context.Background()); err != nil {
			t.Fatalf("PostUpgrade() error = %v", err)
		}
		if entries, _ := os.ReadDir(home); len(entries) != 0 {
			t.Fatalf("PostUpgrade() wrote into an unconfigured home: %v", entries)
		}
	})

	t.Run("openrecord not among the selected community tools is a no-op", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeInstallState(t, home, []string{"codegraph"}, []string{"claude-code"})

		if err := PostUpgrade(context.Background()); err != nil {
			t.Fatalf("PostUpgrade() error = %v", err)
		}
		skillsDir := filepath.Join(home, ".claude", "skills")
		if _, err := os.Stat(skillsDir); !os.IsNotExist(err) {
			t.Fatalf("PostUpgrade() wrote skills despite openrecord not being selected: err = %v", err)
		}
	})

	t.Run("openrecord selected re-runs emit, fan-out and prune", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		installFakeOpenRecordCLI(t)
		writeInstallState(t, home, []string{"codegraph", "openrecord"}, []string{"claude-code"})

		if err := PostUpgrade(context.Background()); err != nil {
			t.Fatalf("PostUpgrade() error = %v", err)
		}

		skillPath := filepath.Join(home, ".claude", "skills", "openrecord-consult", "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("expected %s to exist after PostUpgrade(): %v", skillPath, err)
		}
		if string(content) != "consult skill body" {
			t.Fatalf("%s content = %q, want %q", skillPath, content, "consult skill body")
		}
		manifestPath := filepath.Join(home, ".claude", "skills", ManifestName)
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("expected manifest at %s after PostUpgrade(): %v", manifestPath, err)
		}
	})
}

func TestSelectedFor(t *testing.T) {
	if selectedFor(nil) {
		t.Error("selectedFor(nil) = true, want false")
	}
	if !selectedFor([]string{"codegraph", "openrecord"}) {
		t.Error("selectedFor([codegraph openrecord]) = false, want true")
	}
}
