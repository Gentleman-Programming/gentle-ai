package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectRuntimeMajorDoesNotBypassSymlinkedLauncher(t *testing.T) {
	home, external := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	name := resolveTargetCandidateName()
	wrapper := filepath.Join(external, name)
	content := launcherContent(runtime.GOOS, filepath.Join(home, "removed", name))[name]
	if err := os.WriteFile(wrapper, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(BinDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, wrapper, filepath.Join(BinDir(home), name))
	realDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(realDir, name), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := VersionRunnerOverride
	t.Cleanup(func() { VersionRunnerOverride = old })
	for _, selectedDir := range []string{BinDir(home), external} {
		t.Run(filepath.Base(selectedDir), func(t *testing.T) {
			t.Setenv("PATH", selectedDir+string(os.PathListSeparator)+realDir)
			calls := 0
			VersionRunnerOverride = func(context.Context, Command) (CommandOutput, error) {
				calls++
				if calls > 1 {
					return CommandOutput{Stdout: []byte("2.0.23")}, nil
				}
				return CommandOutput{}, &exec.ExitError{}
			}
			major, err := DetectRuntimeMajor(context.Background())
			if err == nil || major != RuntimeUnknown || calls != 1 {
				t.Fatalf("symlinked user launcher bypassed: major=%v err=%v probes=%d", major, err, calls)
			}
			got, err := os.ReadFile(wrapper)
			if err != nil || string(got) != content {
				t.Fatalf("external wrapper changed: %v", err)
			}
		})
	}
}
