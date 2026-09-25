package system

import (
	"path/filepath"
	"testing"
)

func TestUserPathsDefault(t *testing.T) {
	home := t.TempDir()

	wantAxiom := filepath.Join(home, ".axiom")
	if got := AxiomDir(home); got != wantAxiom {
		t.Errorf("AxiomDir() = %q, want %q", got, wantAxiom)
	}

	wantState := filepath.Join(wantAxiom, "state.json")
	if got := StatePath(home); got != wantState {
		t.Errorf("StatePath() = %q, want %q", got, wantState)
	}

	wantCache := filepath.Join(wantAxiom, "cache")
	if got := CacheDir(home); got != wantCache {
		t.Errorf("CacheDir() = %q, want %q", got, wantCache)
	}

	wantBin := filepath.Join(wantAxiom, "bin")
	if got := BinDir(home); got != wantBin {
		t.Errorf("BinDir() = %q, want %q", got, wantBin)
	}

	wantLegacy := filepath.Join(home, ".gentle-ai")
	if got := LegacyDir(home); got != wantLegacy {
		t.Errorf("LegacyDir() = %q, want %q", got, wantLegacy)
	}

	wantLegacyState := filepath.Join(wantLegacy, "state.json")
	if got := LegacyStatePath(home); got != wantLegacyState {
		t.Errorf("LegacyStatePath() = %q, want %q", got, wantLegacyState)
	}

	wantLegacyCache := filepath.Join(wantLegacy, "cache")
	if got := LegacyCacheDir(home); got != wantLegacyCache {
		t.Errorf("LegacyCacheDir() = %q, want %q", got, wantLegacyCache)
	}

	wantLegacyBin := filepath.Join(wantLegacy, "bin")
	if got := LegacyBinDir(home); got != wantLegacyBin {
		t.Errorf("LegacyBinDir() = %q, want %q", got, wantLegacyBin)
	}
}

func TestUserPathsCustomStateDir(t *testing.T) {
	home := t.TempDir()
	customDir := filepath.Join(home, "custom-axiom")

	t.Run("AXIOM_STATE_DIR takes precedence", func(t *testing.T) {
		t.Setenv("AXIOM_STATE_DIR", customDir)
		t.Setenv("GENTLE_AI_STATE_DIR", filepath.Join(home, "other-dir"))

		if got := AxiomDir(home); got != customDir {
			t.Errorf("AxiomDir() = %q, want %q", got, customDir)
		}
		if got := StatePath(home); got != filepath.Join(customDir, "state.json") {
			t.Errorf("StatePath() = %q, want %q", got, filepath.Join(customDir, "state.json"))
		}
		if got := CacheDir(home); got != filepath.Join(customDir, "cache") {
			t.Errorf("CacheDir() = %q, want %q", got, filepath.Join(customDir, "cache"))
		}
		if got := BinDir(home); got != filepath.Join(customDir, "bin") {
			t.Errorf("BinDir() = %q, want %q", got, filepath.Join(customDir, "bin"))
		}
	})

	t.Run("GENTLE_AI_STATE_DIR is used as fallback", func(t *testing.T) {
		t.Setenv("AXIOM_STATE_DIR", "")
		t.Setenv("GENTLE_AI_STATE_DIR", customDir)

		if got := AxiomDir(home); got != customDir {
			t.Errorf("AxiomDir() = %q, want %q", got, customDir)
		}
	})
}
