package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
)

// standaloneRestoreHome isolates both home resolvers a standalone restore uses:
// the backup package's root resolver and the CLI's own. The home directory is
// resolved physically, so a case that also enters it as the working directory
// compares equal on hosts where the temp root is a symlink.
func standaloneRestoreHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "missing"))
	originalBackupHome := backup.UserHomeDirFn
	originalCLIHome := osUserHomeDir
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	osUserHomeDir = backup.UserHomeDirFn
	t.Cleanup(func() {
		backup.UserHomeDirFn = originalBackupHome
		osUserHomeDir = originalCLIHome
	})
	return home
}

// canonicalDir returns a temp directory in the physical spelling the process
// reports for its working directory, so entry containment compares exactly.
func canonicalDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// A standalone restore must restore an uninstall snapshot whose entry lives in a
// workspace outside the home root, exactly like the uninstaller's own restore
// does with the roots it kept in its Result. The workspace root can only come
// from this process, so the running directory is what authorizes it.
func TestStandaloneRestoreAuthorizesWorkspaceEntryInsideWorkingDirectory(t *testing.T) {
	home := standaloneRestoreHome(t)
	workspace := canonicalDir(t)
	t.Chdir(workspace)

	path := filepath.Join(workspace, "settings.json")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "workspace-root"), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunRestore([]string{"workspace-root", "--yes"}, &output); err != nil {
		t.Fatalf("restore workspace snapshot: %v; output=%s", err, output.String())
	}
	wantOutput := fmt.Sprintf("restore complete — restored backup %s (%s)\n", manifest.ID, manifest.DisplayLabel())
	if output.String() != wantOutput {
		t.Fatalf("restore output = %q, want %q", output.String(), wantOutput)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "before\n" {
		t.Fatalf("restored workspace entry = %q, %v", raw, err)
	}
}

// The workspace root widens only what the running directory already covers: an
// entry outside both the working directory and the home root is still refused
// with the guard's own message, before anything is written.
func TestStandaloneRestoreRefusesEntryOutsideWorkspaceAndHome(t *testing.T) {
	home := standaloneRestoreHome(t)
	workspace := canonicalDir(t)
	outside := canonicalDir(t)
	t.Chdir(workspace)

	path := filepath.Join(outside, "outside.json")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "outside-root"), []string{path})
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	wantError := fmt.Sprintf("restore failed: manifest entry has invalid OriginalPath %q: must be an absolute path under an allowed root (%s)", path, home)
	if err := RunRestore([]string{"outside-root", "--yes"}, &output); err == nil || err.Error() != wantError {
		t.Fatalf("restore error = %v, want %q", err, wantError)
	}
	if output.Len() != 0 {
		t.Fatalf("rejected restore output = %q, want empty", output.String())
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "before" {
		t.Fatalf("refused entry changed = %q, %v", raw, err)
	}
}

// When the working directory is the home root, nothing is added: the same
// single home root authorizes and refuses exactly what it did before.
func TestStandaloneRestoreKeepsHomeOnlyRootsWhenWorkingDirectoryIsHome(t *testing.T) {
	home := standaloneRestoreHome(t)
	t.Chdir(home)
	outside := canonicalDir(t)

	path := filepath.Join(outside, "outside.json")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := backup.NewSnapshotter().Create(filepath.Join(home, ".gentle-ai", "backups", "home-root"), []string{path})
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	wantError := fmt.Sprintf("restore failed: manifest entry has invalid OriginalPath %q: must be an absolute path under an allowed root (%s)", path, home)
	if err := RunRestore([]string{"home-root", "--yes"}, &output); err == nil || err.Error() != wantError {
		t.Fatalf("restore error = %v, want %q", err, wantError)
	}
	if output.Len() != 0 {
		t.Fatalf("rejected restore output = %q, want empty", output.String())
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "before" {
		t.Fatalf("refused entry changed = %q, %v", raw, err)
	}
}
