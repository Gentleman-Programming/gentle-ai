package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
)

// codexCLIHome isolates home-related state and restores the CLI home resolver.
// The initial override is intentionally missing until a test supplies a root.
func codexCLIHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "localappdata"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "missing"))
	original := osUserHomeDir
	osUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { osUserHomeDir = original })
	return home
}

// externalCodexHome selects an existing canonical root outside the isolated home.
func externalCodexHome(t *testing.T) (string, string) {
	t.Helper()
	home := codexCLIHome(t)
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", canonical)
	return home, canonical
}

// TestCodexHomeStandaloneRestoreExternalRoot checks public restore output and bytes.
func TestCodexHomeStandaloneRestoreExternalRoot(t *testing.T) {
	for _, scenario := range []string{"canonical", "symlink", "relative-symlink"} {
		t.Run(scenario, func(t *testing.T) {
			home, root := externalCodexHome(t)
			if scenario != "canonical" {
				alias := filepath.Join(t.TempDir(), "codex-link")
				if err := os.Symlink(root, alias); err != nil {
					t.Skipf("directory symlinks unavailable: %v", err)
				}
				t.Setenv("CODEX_HOME", alias)
				root = alias
				if scenario == "relative-symlink" {
					t.Chdir(home)
					relative, err := filepath.Rel(home, alias)
					if err != nil {
						t.Fatal(err)
					}
					t.Setenv("CODEX_HOME", relative)
				}
			}
			path := filepath.Join(root, "config.toml")
			if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			manifest, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "codex-root"), []string{path})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := RunRestore([]string{"codex-root", "--yes"}, &output); err != nil {
				t.Fatalf("restore: %v; output=%s", err, output.String())
			}
			wantOutput := fmt.Sprintf("restore complete — restored backup %s (%s)\n", manifest.ID, manifest.DisplayLabel())
			if output.String() != wantOutput {
				t.Fatalf("restore output = %q, want %q", output.String(), wantOutput)
			}
			if raw, err := os.ReadFile(path); err != nil || string(raw) != "before\n" {
				t.Fatalf("restored config = %q, %v", raw, err)
			}
		})
	}
}

// TestCodexHomeRestoreStillRejectsUnrelatedRoot checks refusal without file mutation.
func TestCodexHomeRestoreStillRejectsUnrelatedRoot(t *testing.T) {
	for _, scenario := range []string{"unrelated", "symlink-escape", "isolated-symlink"} {
		t.Run(scenario, func(t *testing.T) {
			home, root := externalCodexHome(t)
			path := filepath.Join(t.TempDir(), "outside.txt")
			allowed := home
			if scenario != "unrelated" {
				alias := filepath.Join(t.TempDir(), "codex-link")
				if err := os.Symlink(root, alias); err != nil {
					t.Skipf("directory symlinks unavailable: %v", err)
				}
				t.Setenv("CODEX_HOME", alias)
				if scenario == "symlink-escape" {
					if err := os.Symlink(filepath.Dir(path), filepath.Join(root, "escape")); err != nil {
						t.Fatal(err)
					}
					path = filepath.Join(alias, "escape", "outside.txt")
					allowed = fmt.Sprintf("%s, %s, %s", home, root, alias)
				} else {
					home = t.TempDir()
					original := backup.UserHomeDirFn
					backup.UserHomeDirFn = func() (string, error) { return home, nil }
					t.Cleanup(func() { backup.UserHomeDirFn = original })
					osUserHomeDir = backup.UserHomeDirFn
					path = filepath.Join(alias, "outside.txt")
					allowed = home
				}
			}
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "unrelated"), []string{path})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			err = RunRestore([]string{"unrelated", "--yes"}, &output)
			wantError := fmt.Sprintf("restore failed: manifest entry has invalid OriginalPath %q: must be an absolute path under an allowed root (%s)", path, allowed)
			if err == nil || err.Error() != wantError {
				t.Fatalf("restore error = %v, want %q", err, wantError)
			}
			if output.Len() != 0 {
				t.Fatalf("rejected restore output = %q, want empty", output.String())
			}
			if raw, err := os.ReadFile(path); err != nil || string(raw) != "after" {
				t.Fatalf("unrelated file changed = %q, %v", raw, err)
			}
		})
	}
}

// TestCodexHomeRestorePreservesOrdinaryHomeScope checks the existing home restore scope.
func TestCodexHomeRestorePreservesOrdinaryHomeScope(t *testing.T) {
	home, _ := externalCodexHome(t)
	path := filepath.Join(home, "ordinary.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "ordinary"), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunRestore([]string{"ordinary", "--yes"}, &output); err != nil {
		t.Fatalf("ordinary restore: %v", err)
	}
	wantOutput := fmt.Sprintf("restore complete — restored backup %s (%s)\n", manifest.ID, manifest.DisplayLabel())
	if output.String() != wantOutput {
		t.Fatalf("restore output = %q, want %q", output.String(), wantOutput)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "before" {
		t.Fatalf("ordinary restored file = %q, %v", raw, err)
	}
}
