package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

// RestoreFunc is the function signature for restoring a backup from its manifest.
// It matches app.tuiRestore and backup.RestoreService.Restore signatures.
type RestoreFunc func(manifest backup.Manifest) error

// RunRestore is the top-level entry point for `gentle-ai restore [args]`.
// It reads backups from the real home directory and uses the default restore function.
func RunRestore(args []string, stdout io.Writer) error {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	restorer := defaultRestorer()
	return runRestoreWithHomeDir(args, restorer, stdout, os.Stdin, homeDir)
}

// RunRestoreWithFn is the testable variant of RunRestore. It uses the provided
// RestoreFunc and reads backups from the HOME environment variable (set by tests).
func RunRestoreWithFn(args []string, restorer RestoreFunc, stdout io.Writer) error {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	return runRestoreWithHomeDir(args, restorer, stdout, os.Stdin, homeDir)
}

// RunRestoreWithFnAndInput is the fully injectable variant used in tests that
// need to simulate stdin input (e.g. testing confirmation prompts).
func RunRestoreWithFnAndInput(args []string, restorer RestoreFunc, stdout io.Writer, stdin io.Reader) error {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	return runRestoreWithHomeDir(args, restorer, stdout, stdin, homeDir)
}

// runRestoreWithHomeDir is the internal implementation.
func runRestoreWithHomeDir(args []string, restorer RestoreFunc, stdout io.Writer, stdin io.Reader, homeDir string) error {
	// Pre-scan for --yes/-y and --list flags before standard flag parsing,
	// because positional arguments (e.g. `restore backup-001 --yes`) appear
	// before flags in the args slice and flag.FlagSet stops parsing at the
	// first non-flag argument.
	list := false
	yes := false
	var positional []string

	for _, a := range args {
		switch a {
		case "--list", "-list":
			list = true
		case "--yes", "-yes", "-y":
			yes = true
		default:
			if strings.HasPrefix(a, "-") {
				// Unknown flag — surface error via flag.FlagSet for consistent messages.
				fs := flag.NewFlagSet("restore", flag.ContinueOnError)
				fs.SetOutput(ioDiscard{})
				_ = fs.Bool("list", false, "")
				_ = fs.Bool("yes", false, "")
				if err := fs.Parse(args); err != nil {
					return fmt.Errorf("parse restore flags: %w", err)
				}
				return nil
			}
			positional = append(positional, a)
		}
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
