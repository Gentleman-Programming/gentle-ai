package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
)

// RestoreFunc is the function signature for restoring a backup from its manifest.
// It matches app.tuiRestore and backup.RestoreService.Restore signatures.
type RestoreFunc func(manifest backup.Manifest) error

// RunRestore is the top-level entry point for `gentle-ai restore [args]`.
// It reads backups from the real home directory and uses the default restore function.
func RunRestore(args []string, stdout io.Writer) error {
	restorer := defaultRestorer()
	return runRestoreWithHomeDir(args, restorer, stdout, os.Stdin, osUserHomeDir)
}

// RunRestoreWithFn is the testable variant of RunRestore. It uses the provided
// RestoreFunc and reads backups from the HOME environment variable (set by tests).
func RunRestoreWithFn(args []string, restorer RestoreFunc, stdout io.Writer) error {
	return runRestoreWithHomeDir(args, restorer, stdout, os.Stdin, osUserHomeDir)
}

// RunRestoreWithFnAndInput is the fully injectable variant used in tests that
// need to simulate stdin input (e.g. testing confirmation prompts).
func RunRestoreWithFnAndInput(args []string, restorer RestoreFunc, stdout io.Writer, stdin io.Reader) error {
	return runRestoreWithHomeDir(args, restorer, stdout, stdin, osUserHomeDir)
}

// newRestoreFlagSet binds parsed flags to the values that drive restore.
// The custom Usage adds the positional backup-selection syntax the flag package
// cannot derive; PrintDefaults keeps descriptions derived from registrations.
func newRestoreFlagSet(list, yes *bool) *flag.FlagSet {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	// Descriptions are what the derived usage shows the operator,
	// so they are the documentation rather than a placeholder.
	fs.BoolVar(list, "list", false, "list available backups without restoring (--list=false disables listing)")
	fs.BoolVar(yes, "yes", false, "skip confirmation prompt (--yes=false requires confirmation)")
	fs.Usage = func() {
		// Keep the "Usage of " prefix: derivedUsageText strips everything
		// before it, so the flag package's duplicated error line is not
		// reported twice alongside the custom block.
		fmt.Fprintf(fs.Output(), "Usage of %s:\n", fs.Name())
		fmt.Fprintln(fs.Output(), "  gentle-ai restore [--list | latest | <id>] [--yes]")
		fmt.Fprintln(fs.Output(), "  -- ends flag parsing; at most one backup target is accepted")
		fs.PrintDefaults()
	}
	return fs
}

// runRestoreWithHomeDir is the internal implementation. The home directory is
// resolved lazily via resolveHome, after the argument pre-scan: an explicit
// help request or an unknown flag must be answered before any attempt to
// resolve the home directory, so `restore --help` works even on hosts where
// the home directory cannot be resolved.
func runRestoreWithHomeDir(args []string, restorer RestoreFunc, stdout io.Writer, stdin io.Reader, resolveHome func() (string, error)) error {
	list := false
	yes := false
	fs := newRestoreFlagSet(&list, &yes)
	var positional []string
	flagsEnded := false

	for _, a := range args {
		if flagsEnded {
			positional = append(positional, a)
			continue
		}
		if a == "--" {
			flagsEnded = true
			continue
		}
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		// Keep the existing short alias without a duplicate help entry.
		if a == "-y" {
			a = "--yes"
		}
		// Parse each flag against the same bound values, so flags after a
		// positional still work and explicit boolean values are not discarded.
		if err := parseCommandFlags(fs, []string{a}); err != nil {
			if writeHelpRequest(err, stdout) == nil {
				return nil
			}
			return fmt.Errorf("parse restore flags: %w", err)
		}
	}

	// Reject surplus targets before listing, prompting, or restoring anything.
	if len(positional) > 1 {
		return fmt.Errorf("usage: gentle-ai restore [--list | latest | <id>] [--yes]")
	}

	// Resolve the home directory only once the request is known to need it.
	homeDir, err := resolveHome()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	// Load backups from the real backup directory.
	backups := listBackupsFromDir(homeDir)

	// --list mode: print all backups and exit.
	if list {
		return renderRestoreList(backups, stdout)
	}

	// If no subcommand argument, show usage.
	if len(positional) == 0 {
		return fmt.Errorf("usage: gentle-ai restore [--list | latest | <id>] [--yes]")
	}

	target := positional[0]

	// Resolve the target manifest.
	manifest, err := resolveRestoreTarget(target, backups)
	if err != nil {
		return err
	}

	// Confirm unless --yes was supplied.
	if !yes {
		confirmed, err := promptRestoreConfirm(manifest, stdin, stdout)
		if err != nil {
			return fmt.Errorf("confirmation: %w", err)
		}
		if !confirmed {
			fmt.Fprintln(stdout, "restore cancelled")
			return nil
		}
	}

	// Execute restore.
	if err := restorer(manifest); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	fmt.Fprintf(stdout, "restore complete — restored backup %s (%s)\n", manifest.ID, manifest.DisplayLabel())
	return nil
}

// renderRestoreList writes the backup listing to stdout.
// Backups are already sorted newest-first by listBackupsFromDir.
// Each entry shows: index, ID, DisplayLabel (source + timestamp + file count),
// and the gentle-ai version that created the backup when known.
func renderRestoreList(backups []backup.Manifest, stdout io.Writer) error {
	if len(backups) == 0 {
		fmt.Fprintln(stdout, "no backups found")
		return nil
	}

	fmt.Fprintf(stdout, "Available backups (%d):\n", len(backups))
	for i, m := range backups {
		line := fmt.Sprintf("  [%d] %s  %s", i+1, m.ID, m.DisplayLabel())
		if m.CreatedByVersion != "" {
			line += fmt.Sprintf("  [v%s]", m.CreatedByVersion)
		}
		fmt.Fprintln(stdout, line)
	}
	return nil
}

// resolveRestoreTarget finds the manifest matching the given target string.
//   - "latest": returns the newest backup (first in the sorted slice).
//   - "<id>":   returns the backup whose ID matches exactly.
//
// Returns an error if no backup matches.
func resolveRestoreTarget(target string, backups []backup.Manifest) (backup.Manifest, error) {
	if target == "latest" {
		if len(backups) == 0 {
			return backup.Manifest{}, fmt.Errorf("no backups available to restore")
		}
		return backups[0], nil
	}

	for _, m := range backups {
		if m.ID == target {
			return m, nil
		}
	}

	return backup.Manifest{}, fmt.Errorf("backup %q not found — use `gentle-ai restore --list` to see available backups", target)
}

// promptRestoreConfirm asks the user to confirm a restore operation.
// Returns (true, nil) on confirmation, (false, nil) on any non-confirm input,
// and (false, err) when stdin cannot be read.
func promptRestoreConfirm(manifest backup.Manifest, stdin io.Reader, stdout io.Writer) (bool, error) {
	fmt.Fprintf(stdout, "Restore backup %s (%s)?\n", manifest.ID, manifest.DisplayLabel())
	fmt.Fprintf(stdout, "This will overwrite your current configuration. Type 'yes' to confirm: ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read confirmation input: %w", err)
		}
		// EOF without input — treat as rejection (non-interactive).
		return false, fmt.Errorf("no confirmation provided (use --yes to skip prompt)")
	}

	answer := strings.TrimSpace(scanner.Text())
	return strings.EqualFold(answer, "yes"), nil
}

// listBackupsFromDir reads and sorts backups from the given homeDir.
// It is equivalent to app.ListBackups but operates on an explicit homeDir,
// keeping the cli package independent from the app package.
func listBackupsFromDir(homeDir string) []backup.Manifest {
	backupRoot := backupRootDir(homeDir)
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return nil
	}

	manifests := make([]backup.Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := fmt.Sprintf("%s/%s/%s", backupRoot, entry.Name(), backup.ManifestFilename)
		m, err := backup.ReadManifest(manifestPath)
		if err != nil {
			continue
		}
		manifests = append(manifests, m)
	}

	// Sort newest-first.
	for i := 0; i < len(manifests); i++ {
		for j := i + 1; j < len(manifests); j++ {
			if manifests[j].CreatedAt.After(manifests[i].CreatedAt) {
				manifests[i], manifests[j] = manifests[j], manifests[i]
			}
		}
	}

	return manifests
}

// backupRootDir returns the path to the backup directory under homeDir.
func backupRootDir(homeDir string) string {
	return homeDir + "/.gentle-ai/backups"
}

// defaultRestorer returns the standard backup.RestoreService.Restore function.
func defaultRestorer() RestoreFunc {
	return func(m backup.Manifest) error {
		return backup.RestoreService{}.Restore(m)
	}
}
