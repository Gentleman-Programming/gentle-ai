package testenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// RunBrewProbeFixture dispatches only copied brew test binaries before m.Run.
func RunBrewProbeFixture() {
	name := filepath.Base(os.Args[0])
	mode := os.Getenv("GENTLE_AI_TEST_BREW_PROBE")
	if mode == "" || (name != "brew" && name != "brew.exe") {
		return
	}
	if len(os.Args) != 3 || os.Args[1] != "help" || os.Args[2] != "trust" {
		os.Exit(2)
	}
	if mode == "modern" {
		os.Exit(0)
	}
	os.Exit(1)
}

// BrewProbe shadows real Homebrew with a native, capability-only test fixture.
func BrewProbe(t *testing.T, supported bool) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "brew"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), data, 0o755); err != nil {
		t.Fatal(err)
	}
	mode := "legacy"
	if supported {
		mode = "modern"
	}
	t.Setenv("GENTLE_AI_TEST_BREW_PROBE", mode)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
