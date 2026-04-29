package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

func TestEngramPathGuidanceFish(t *testing.T) {
	msg := engramPathGuidance("/usr/bin/fish", "/home/user/.local/bin")
	if want := "fish_user_paths"; !strings.Contains(msg, want) {
		t.Fatalf("engramPathGuidance(fish) missing %q: %s", want, msg)
	}
}

func TestEngramPathGuidanceZsh(t *testing.T) {
	msg := engramPathGuidance("/bin/zsh", "/home/user/.local/bin")
	if want := ".zshrc"; !strings.Contains(msg, want) {
		t.Fatalf("engramPathGuidance(zsh) missing %q: %s", want, msg)
	}
}

func TestEngramPathGuidanceDefault(t *testing.T) {
	msg := engramPathGuidance("", "/home/user/.local/bin")
	if want := "Add "; !strings.Contains(msg, want) {
		t.Fatalf("engramPathGuidance(default) missing %q: %s", want, msg)
	}
}

func TestEngramDownloadBinDirsIncludesLocalBin(t *testing.T) {
	dirs := engramDownloadBinDirs()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "bin")
	if len(dirs) != 2 || dirs[0] != "/usr/local/bin" || dirs[1] != want {
		t.Fatalf("engramDownloadBinDirs() = %v, want [/usr/local/bin %s]", dirs, want)
	}
}

func TestIsInPATH(t *testing.T) {
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+"/home/user/.local/bin")
	if !isInPATH("/home/user/.local/bin") {
		t.Fatal("isInPATH should return true for entry in PATH")
	}
	if isInPATH("/not/in/path") {
		t.Fatal("isInPATH should return false for entry not in PATH")
	}
}

func TestAllInPATH(t *testing.T) {
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+"/home/user/.local/bin")
	if !allInPATH([]string{"/usr/bin", "/home/user/.local/bin"}) {
		t.Fatal("allInPATH should return true when all entries are in PATH")
	}
	if allInPATH([]string{"/usr/bin", "/not/in/path"}) {
		t.Fatal("allInPATH should return false when an entry is missing")
	}
}

func TestWithEngramPathNoteAddsNoteWhenNotInPATH(t *testing.T) {
	// Set PATH to something that does NOT contain the direct-download install dirs.
	t.Setenv("PATH", "/usr/bin:/usr/local/bin")

	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{
		OrderedComponents: []model.ComponentID{model.ComponentEngram},
		PlatformDecision:  planner.PlatformDecision{PackageManager: "apt"},
	}

	updated := withEngramPathNote(report, resolved)
	if !strings.Contains(updated.FinalNote, "pre-built binary") {
		t.Fatalf("FinalNote should contain direct binary guidance, got: %q", updated.FinalNote)
	}
	if strings.Contains(updated.FinalNote, "go install") {
		t.Fatalf("FinalNote should not mention go install, got: %q", updated.FinalNote)
	}
	if !strings.Contains(updated.FinalNote, ".local/bin") {
		t.Fatalf("FinalNote should reference .local/bin dir, got: %q", updated.FinalNote)
	}
}

func TestWithEngramPathNoteSkipsWhenBrew(t *testing.T) {
	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{
		OrderedComponents: []model.ComponentID{model.ComponentEngram},
		PlatformDecision:  planner.PlatformDecision{PackageManager: "brew"},
	}

	updated := withEngramPathNote(report, resolved)
	if updated.FinalNote != report.FinalNote {
		t.Fatalf("FinalNote should be unchanged for brew, got: %q", updated.FinalNote)
	}
}

func TestWithEngramPathNoteSkipsWhenInPATH(t *testing.T) {
	home, _ := os.UserHomeDir()
	localBin := filepath.Join(home, ".local", "bin")
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+"/usr/local/bin"+string(os.PathListSeparator)+localBin)

	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{
		OrderedComponents: []model.ComponentID{model.ComponentEngram},
		PlatformDecision:  planner.PlatformDecision{PackageManager: "apt"},
	}

	updated := withEngramPathNote(report, resolved)
	if updated.FinalNote != report.FinalNote {
		t.Fatalf("FinalNote should be unchanged when engram install dirs are in PATH, got: %q", updated.FinalNote)
	}
}

func TestWithEngramPathNoteSkipsWithoutEngram(t *testing.T) {
	report := verify.Report{Ready: true, FinalNote: "You're ready."}
	resolved := planner.ResolvedPlan{
		OrderedComponents: []model.ComponentID{model.ComponentGGA},
		PlatformDecision:  planner.PlatformDecision{PackageManager: "apt"},
	}

	updated := withEngramPathNote(report, resolved)
	if updated.FinalNote != report.FinalNote {
		t.Fatalf("FinalNote should be unchanged without engram, got: %q", updated.FinalNote)
	}
}
