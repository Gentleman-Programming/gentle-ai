package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
)

// setupRestoreHome creates a temporary home dir with N backup manifests.
// Returns the home dir path. Manifests are created with predictable IDs.
func setupRestoreHome(t *testing.T, count int) string {
	t.Helper()
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("backup-%03d", i)
		dir := filepath.Join(backupRoot, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
		m := backup.Manifest{
			ID:        id,
			CreatedAt: time.Date(2026, 3, 20+i, 10, 0, 0, 0, time.UTC),
			RootDir:   dir,
			Source:    backup.BackupSourceInstall,
			Entries:   []backup.ManifestEntry{},
		}
		if err := backup.WriteManifest(filepath.Join(dir, backup.ManifestFilename), m); err != nil {
			t.Fatalf("WriteManifest: %v", err)
		}
	}
	return home
}

// TestRunRestore_ListShowsBackupsNewestFirst verifies that `restore --list`
// prints backups newest-first with ID, timestamp, and source label.
func TestRunRestore_ListShowsBackupsNewestFirst(t *testing.T) {
	home := setupRestoreHome(t, 3)
	restoreHomeDir(t, home)

	var out strings.Builder
	err := RunRestore([]string{"--list"}, &out)
	if err != nil {
		t.Fatalf("RunRestore(--list) error = %v", err)
	}

	output := out.String()
	if output == "" {
		t.Fatalf("RunRestore(--list) produced no output")
	}

	// Must mention "backup" somewhere meaningful.
	if !strings.Contains(output, "backup") {
		t.Errorf("--list output should reference backups; got:\n%s", output)
	}

	// Must show backup IDs.
	if !strings.Contains(output, "backup-002") {
		t.Errorf("--list should show newest backup-002 first; got:\n%s", output)
	}

	// Newest backup-002 must appear before backup-000.
	idx002 := strings.Index(output, "backup-002")
	idx000 := strings.Index(output, "backup-000")
	if idx002 < 0 || idx000 < 0 {
		t.Fatalf("--list output missing expected backup IDs; got:\n%s", output)
	}
	if idx002 > idx000 {
		t.Errorf("--list should show newest (backup-002) before oldest (backup-000)")
	}
}

// TestRunRestore_ListEmptyShowsNoBackupsMessage verifies the empty-backup case.
func TestRunRestore_ListEmptyShowsNoBackupsMessage(t *testing.T) {
	home := t.TempDir()
	restoreHomeDir(t, home)

	var out strings.Builder
	err := RunRestore([]string{"--list"}, &out)
	if err != nil {
		t.Fatalf("RunRestore(--list, empty) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "no backup") {
		t.Errorf("expected 'no backup' message when empty; got:\n%s", output)
	}
}

// TestRunRestore_ByIDWithYesRestoresSuccessfully verifies that `restore <id> --yes`
// restores a backup without prompting when the restore function succeeds.
func TestRunRestore_ByIDWithYesRestoresSuccessfully(t *testing.T) {
	home := setupRestoreHome(t, 2)
	restoreHomeDir(t, home)

	// Track which manifest was restored.
	var restoredID string
	restorer := func(m backup.Manifest) error {
		restoredID = m.ID
		return nil
	}

	var out strings.Builder
	err := RunRestoreWithFn([]string{"backup-001", "--yes"}, restorer, &out)
	if err != nil {
		t.Fatalf("RunRestoreWithFn(backup-001 --yes) error = %v", err)
	}

	if restoredID != "backup-001" {
		t.Errorf("restored ID = %q, want backup-001", restoredID)
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "restor") {
		t.Errorf("output should confirm restore; got:\n%s", output)
	}
}

// TestRunRestore_UnknownIDReturnsError verifies that requesting an unknown backup ID
// returns a clear error and does NOT modify any files (restorer not called).
func TestRunRestore_UnknownIDReturnsError(t *testing.T) {
	home := setupRestoreHome(t, 2)
	restoreHomeDir(t, home)

	restoreCalled := false
	restorer := func(m backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	var out strings.Builder
	err := RunRestoreWithFn([]string{"does-not-exist", "--yes"}, restorer, &out)
	if err == nil {
		t.Fatalf("RunRestoreWithFn(unknown-id) expected error, got nil")
	}

	if restoreCalled {
		t.Errorf("restorer must NOT be called for unknown backup ID")
	}
}

// TestRunRestore_LatestWithYesRestoresNewest verifies that `restore latest --yes`
// restores the newest backup (highest CreatedAt) without prompting.
func TestRunRestore_LatestWithYesRestoresNewest(t *testing.T) {
	home := setupRestoreHome(t, 3)
	restoreHomeDir(t, home)

	var restoredID string
	restorer := func(m backup.Manifest) error {
		restoredID = m.ID
		return nil
	}

	var out strings.Builder
	err := RunRestoreWithFn([]string{"latest", "--yes"}, restorer, &out)
	if err != nil {
		t.Fatalf("RunRestoreWithFn(latest --yes) error = %v", err)
	}

	// backup-002 has the latest CreatedAt (2026-03-22).
	if restoredID != "backup-002" {
		t.Errorf("restored ID = %q, want backup-002 (newest)", restoredID)
	}
}

// TestRunRestore_LatestWithNoBackupsReturnsError verifies the empty-backup edge case.
func TestRunRestore_LatestWithNoBackupsReturnsError(t *testing.T) {
	home := t.TempDir()
	restoreHomeDir(t, home)

	restorer := func(m backup.Manifest) error { return nil }
	var out strings.Builder
	err := RunRestoreWithFn([]string{"latest", "--yes"}, restorer, &out)
	if err == nil {
		t.Fatalf("RunRestoreWithFn(latest, empty) expected error")
	}
}

// TestRunRestore_RequiresConfirmationWithoutYes verifies that without --yes,
// restore by ID returns a clear error requiring confirmation (non-interactive).
func TestRunRestore_RequiresConfirmationWithoutYes(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)

	restoreCalled := false
	restorer := func(m backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	var out strings.Builder
	// Provide a non-terminal reader so the confirmation prompt sees EOF.
	err := RunRestoreWithFnAndInput([]string{"backup-000"}, restorer, &out, strings.NewReader(""))
	if err == nil {
		t.Fatalf("RunRestoreWithFnAndInput without --yes and no tty input: expected error (confirmation required)")
	}
	if restoreCalled {
		t.Errorf("restorer must NOT be called when confirmation is not given")
	}
}

// TestRunRestore_InteractiveTypedYesConfirmationRestores verifies the positive
// interactive confirmation path: when the user types "yes" at the prompt,
// the backup is restored successfully without the --yes flag.
//
// This covers the spec scenario: "restore <id> then typed yes".
// Verify gap: no prior test exercised the typed-confirmation success path.
func TestRunRestore_InteractiveTypedYesConfirmationRestores(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)

	var restoredID string
	restorer := func(m backup.Manifest) error {
		restoredID = m.ID
		return nil
	}

	var out strings.Builder
	// Supply "yes\n" as stdin — simulates the user typing yes at the prompt.
	stdin := strings.NewReader("yes\n")
	err := RunRestoreWithFnAndInput([]string{"backup-000"}, restorer, &out, stdin)
	if err != nil {
		t.Fatalf("RunRestoreWithFnAndInput(typed yes) error = %v", err)
	}

	// The restorer MUST have been called.
	if restoredID != "backup-000" {
		t.Errorf("restored ID = %q, want backup-000 after typed confirmation", restoredID)
	}

	output := out.String()
	// Output must mention restore completion.
	if !strings.Contains(strings.ToLower(output), "restor") {
		t.Errorf("output should confirm restore after typed yes; got:\n%s", output)
	}
}

// TestRunRestore_InteractiveTypedNoDoesNotRestore verifies that typing anything
// other than "yes" at the prompt cancels the restore without error.
func TestRunRestore_InteractiveTypedNoDoesNotRestore(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)

	restoreCalled := false
	restorer := func(m backup.Manifest) error {
		restoreCalled = true
		return nil
	}

	var out strings.Builder
	// Supply "no\n" — user explicitly declined.
	stdin := strings.NewReader("no\n")
	err := RunRestoreWithFnAndInput([]string{"backup-000"}, restorer, &out, stdin)
	if err != nil {
		t.Fatalf("RunRestoreWithFnAndInput(typed no) error = %v (should be nil — cancel is not an error)", err)
	}

	if restoreCalled {
		t.Errorf("restorer must NOT be called when user types 'no'")
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "cancel") {
		t.Errorf("output should mention cancellation; got:\n%s", output)
	}
}

// TestRunRestore_UnknownFlagReturnsError verifies flag parse errors are surfaced.
func TestRunRestore_UnknownFlagReturnsError(t *testing.T) {
	var out strings.Builder
	err := RunRestore([]string{"--this-flag-does-not-exist"}, &out)
	if err == nil {
		t.Fatalf("RunRestore(unknown-flag) expected error")
	}
}

// TestRunRestore_BooleanFlagValues verifies explicit values and existing bare
// forms through the public restore entry point without touching real config.
func TestRunRestore_BooleanFlagValues(t *testing.T) {
	label := "install — " + time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC).Local().Format("2006-01-02 15:04")
	complete := "restore complete — restored backup backup-000 (" + label + ")\n"
	prompt := "Restore backup backup-000 (" + label + ")?\nThis will overwrite your current configuration. Type 'yes' to confirm: "
	const usage = "usage: gentle-ai restore [--list | latest | <id>] [--yes]"
	listing := "Available backups (1):\n  [1] backup-000  " + label + "\n"
	for _, test := range []struct {
		name      string
		args      []string
		input     string
		wantOut   string
		wantErr   string
		wantCalls int
	}{
		{name: "yes false without target", args: []string{"--yes=false"}, wantErr: usage},
		{name: "list false without target", args: []string{"--list=false"}, wantErr: usage},
		{name: "yes false requires confirmation", args: []string{"latest", "--yes=false"}, wantOut: prompt, wantErr: "confirmation: no confirmation provided (use --yes to skip prompt)"},
		{name: "yes false accepts confirmation", args: []string{"--yes=false", "latest"}, input: "yes\n", wantOut: prompt + complete, wantCalls: 1},
		{name: "yes false cancellation", args: []string{"latest", "--yes=false"}, input: "no\n", wantOut: prompt + "restore cancelled\n"},
		{name: "yes true after target", args: []string{"latest", "--yes=true"}, wantOut: complete, wantCalls: 1},
		{name: "yes true before target", args: []string{"--yes=true", "latest"}, wantOut: complete, wantCalls: 1},
		{name: "list true", args: []string{"--list=true"}, wantOut: listing},
		{name: "list false restores target", args: []string{"latest", "--list=false", "--yes"}, wantOut: complete, wantCalls: 1},
		{name: "bare list", args: []string{"--list"}, wantOut: listing},
		{name: "single dash list", args: []string{"-list"}, wantOut: listing},
		{name: "bare yes", args: []string{"latest", "--yes"}, wantOut: complete, wantCalls: 1},
		{name: "single dash yes", args: []string{"latest", "-yes"}, wantOut: complete, wantCalls: 1},
		{name: "short yes", args: []string{"latest", "-y"}, wantOut: complete, wantCalls: 1},
		{name: "last yes value wins", args: []string{"--yes", "latest", "--yes=false"}, input: "no\n", wantOut: prompt + "restore cancelled\n"},
		{name: "last list value wins", args: []string{"--list", "--list=false"}, wantErr: usage},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := setupRestoreHome(t, 1)
			restoreHomeDir(t, home)
			calls := 0
			restorer := func(m backup.Manifest) error {
				calls++
				if m.ID != "backup-000" {
					t.Errorf("restored ID = %q, want backup-000", m.ID)
				}
				return nil
			}
			var out strings.Builder
			err := RunRestoreWithFnAndInput(test.args, restorer, &out, strings.NewReader(test.input))
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if gotErr != test.wantErr {
				t.Errorf("error = %q, want %q", gotErr, test.wantErr)
			}
			if got := out.String(); got != test.wantOut {
				t.Errorf("stdout = %q, want %q", got, test.wantOut)
			}
			if calls != test.wantCalls {
				t.Errorf("restore calls = %d, want %d", calls, test.wantCalls)
			}
		})
	}
}

func TestRunRestore_EndOfFlagsAndExtraArguments(t *testing.T) {
	label := "install — " + time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC).Local().Format("2006-01-02 15:04")
	prompt := "Restore backup backup-000 (" + label + ")?\nThis will overwrite your current configuration. Type 'yes' to confirm: "
	const usage = "usage: gentle-ai restore [--list | latest | <id>] [--yes]"
	for _, test := range []struct {
		name    string
		args    []string
		wantOut string
		wantErr string
	}{
		{name: "explicit yes after terminator", args: []string{"latest", "--", "--yes=true"}, wantErr: usage},
		{name: "bare yes after terminator", args: []string{"latest", "--", "--yes"}, wantErr: usage},
		{name: "short yes after terminator", args: []string{"latest", "--", "-y"}, wantErr: usage},
		{name: "extra target without terminator", args: []string{"latest", "backup-000", "--yes"}, wantErr: usage},
		{name: "extra targets in list mode", args: []string{"--list", "latest", "backup-000"}, wantErr: usage},
		{name: "target after terminator requires confirmation", args: []string{"--", "latest"}, wantOut: prompt, wantErr: "confirmation: no confirmation provided (use --yes to skip prompt)"},
		{name: "terminator after target requires confirmation", args: []string{"latest", "--"}, wantOut: prompt, wantErr: "confirmation: no confirmation provided (use --yes to skip prompt)"},
		{name: "list after terminator is a target", args: []string{"--", "--list"}, wantErr: "backup \"--list\" not found — use `gentle-ai restore --list` to see available backups"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := setupRestoreHome(t, 1)
			restoreHomeDir(t, home)
			calls := 0
			restorer := func(backup.Manifest) error { calls++; return nil }
			var before, after, out strings.Builder
			if err := RunRestore([]string{"--list"}, &before); err != nil {
				t.Fatal(err)
			}
			err := RunRestoreWithFnAndInput(test.args, restorer, &out, strings.NewReader(""))
			if err == nil || err.Error() != test.wantErr {
				t.Errorf("error = %v, want %q", err, test.wantErr)
			}
			if out.String() != test.wantOut || calls != 0 {
				t.Errorf("stdout = %q, want %q; restore calls = %d, want 0", out.String(), test.wantOut, calls)
			}
			if err := RunRestore([]string{"--list"}, &after); err != nil {
				t.Fatal(err)
			}
			if after.String() != before.String() {
				t.Errorf("backup listing changed: before %q, after %q", before.String(), after.String())
			}
		})
	}
}

func TestRunRestore_InvalidFlagsDoNotRestore(t *testing.T) {
	home := setupRestoreHome(t, 1)
	restoreHomeDir(t, home)
	var help strings.Builder
	if err := RunRestore([]string{"--help"}, &help); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		flag string
		err  string
	}{
		{flag: "--yes=invalid", err: `invalid boolean value "invalid" for -yes: parse error`},
		{flag: "--list=invalid", err: `invalid boolean value "invalid" for -list: parse error`},
		{flag: "--unknown", err: "flag provided but not defined: -unknown"},
	} {
		for _, afterTarget := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/afterTarget=%t", test.flag, afterTarget), func(t *testing.T) {
				args := []string{test.flag, "latest", "--yes"}
				if afterTarget {
					args = []string{"latest", "--yes", test.flag}
				}
				calls := 0
				restorer := func(backup.Manifest) error { calls++; return nil }
				var before, after, out strings.Builder
				if err := RunRestore([]string{"--list"}, &before); err != nil {
					t.Fatal(err)
				}
				err := RunRestoreWithFnAndInput(args, restorer, &out, strings.NewReader("yes\n"))
				wantErr := "parse restore flags: " + test.err + " — run `gentle-ai restore --help` for the supported flags:\n" + strings.TrimSuffix(help.String(), "\n")
				if err == nil || err.Error() != wantErr {
					t.Errorf("error = %v, want %q", err, wantErr)
				}
				if out.String() != "" || calls != 0 {
					t.Errorf("rejected flag produced stdout %q and %d restore calls", out.String(), calls)
				}
				if err := RunRestore([]string{"--list"}, &after); err != nil {
					t.Fatal(err)
				}
				if after.String() != before.String() {
					t.Errorf("rejected flag changed backup listing: before %q, after %q", before.String(), after.String())
				}
			})
		}
	}
}

// --- helpers ---

// restoreHomeDir sets HOME to dir for the duration of the test.
func restoreHomeDir(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", orig) })
	os.Setenv("HOME", dir)

	// Mock function pointers to isolate home directory completely.
	rOSHome := osUserHomeDir
	rBackup := backup.UserHomeDirFn
	t.Cleanup(func() {
		osUserHomeDir = rOSHome
		backup.UserHomeDirFn = rBackup
	})
	osUserHomeDir = func() (string, error) { return dir, nil }
	backup.UserHomeDirFn = func() (string, error) { return dir, nil }
}
