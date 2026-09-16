package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
)

// RunBackup is the top-level CLI entry point for `gentle-ai backup [subcommand] [flags]`.
func RunBackup(args []string, stdout io.Writer) error {
	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	return runBackupWithInput(args, stdout, os.Stdin, homeDir)
}

// RunBackupWithInput is the testable entry point allowing injected stdin and home directory.
func RunBackupWithInput(args []string, stdout io.Writer, stdin io.Reader, homeDir string) error {
	return runBackupWithInput(args, stdout, stdin, homeDir)
}

func runBackupWithInput(args []string, stdout io.Writer, stdin io.Reader, homeDir string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printBackupHelp(stdout)
		return nil
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "list", "ls":
		return runBackupList(subArgs, stdout, homeDir)
	case "clean":
		return runBackupClean(subArgs, stdout, stdin, homeDir)
	default:
		return fmt.Errorf("unknown backup subcommand %q — run 'gentle-ai backup --help' for available subcommands", subcommand)
	}
}

func printBackupHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: gentle-ai backup <subcommand> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  list, ls    List stored backups and their disk footprint")
	fmt.Fprintln(w, "  clean       Purge old backups according to retention policy")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags for list:")
	fmt.Fprintln(w, "  --json      Output machine-readable JSON metadata")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags for clean:")
	fmt.Fprintln(w, "  --keep N    Number of most recent backups to retain (default: 5)")
	fmt.Fprintln(w, "  --force, -y Skip confirmation prompt")
}

func runBackupList(args []string, stdout io.Writer, homeDir string) error {
	fs := flag.NewFlagSet("backup list", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	jsonOutput := fs.Bool("json", false, "Output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse backup list flags: %w", err)
	}

	backupRoot := backupRootDir(homeDir)
	report, err := backup.ListBackupsReport(backupRoot)
	if err != nil {
		return fmt.Errorf("inspect backups: %w", err)
	}

	if *jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	if report.TotalCount == 0 {
		fmt.Fprintf(stdout, "No backups found in %s\n", backupRoot)
		return nil
	}

	now := time.Now()
	fmt.Fprintf(stdout, "%-21s %-36s %-8s %-10s %s\n", "TIMESTAMP", "NAME / REASON", "FILES", "SIZE", "AGE")
	for _, b := range report.Backups {
		ts := b.CreatedAt.Format("2006-01-02 15:04:05")
		nameReason := b.Name
		if b.Reason != "" && b.Reason != b.Name {
			nameReason = fmt.Sprintf("%s (%s)", b.Name, b.Reason)
		}
		if b.Pinned {
			nameReason = "[pinned] " + nameReason
		}
		age := backup.FormatAge(b.CreatedAt, now)
		fmt.Fprintf(stdout, "%-21s %-36s %-8d %-10s %s\n", ts, nameReason, b.FileCount, b.SizeHuman, age)
	}
	fmt.Fprintln(stdout)

	backupWord := "backups"
	if report.TotalCount == 1 {
		backupWord = "backup"
	}
	fmt.Fprintf(stdout, "Total: %d %s occupying %s in %s\n",
		report.TotalCount, backupWord, backup.FormatSize(report.TotalBytes), backupRoot)
	return nil
}

func runBackupClean(args []string, stdout io.Writer, stdin io.Reader, homeDir string) error {
	keep := backup.DefaultRetentionCount
	force := false

	// Parse flags manual/flexible so flags can appear anywhere
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--keep" || arg == "-keep":
			if i+1 >= len(args) {
				// refusal:by-design operator-knowledge: only the operator knows the intended retention count; provide a positive integer value or run 'gentle-ai backup --help'
				return fmt.Errorf("flag needs an argument: %s", arg)
			}
			i++
			var val int
			if _, err := fmt.Sscanf(args[i], "%d", &val); err != nil || val <= 0 {
				// refusal:by-design operator-knowledge: only the operator knows the desired retention count; supply a positive integer or run 'gentle-ai backup --help'
				return fmt.Errorf("--keep must be a positive integer, got %q", args[i])
			}
			keep = val
		case strings.HasPrefix(arg, "--keep="):
			var val int
			if _, err := fmt.Sscanf(strings.TrimPrefix(arg, "--keep="), "%d", &val); err != nil || val <= 0 {
				// refusal:by-design operator-knowledge: only the operator knows the desired retention count; supply a positive integer or run 'gentle-ai backup --help'
				return fmt.Errorf("--keep must be a positive integer, got %q", strings.TrimPrefix(arg, "--keep="))
			}
			keep = val
		case arg == "--force" || arg == "-force" || arg == "-y" || arg == "--yes" || arg == "-yes":
			force = true
		default:
			// refusal:by-design operator-knowledge: unknown flag specified; run 'gentle-ai backup --help' to see supported options
			return fmt.Errorf("unknown flag %q for backup clean", arg)
		}
	}

	backupRoot := backupRootDir(homeDir)

	if !force {
		fmt.Fprintf(stdout, "Purge older backups keeping the %d most recent? Type 'yes' to confirm: ", keep)
		scanner := bufio.NewScanner(stdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read confirmation input: %w", err)
			}
			// refusal:by-design operator-knowledge: non-interactive invocation requires explicit bypass; re-run with --force or -y to skip confirmation
			return fmt.Errorf("no confirmation provided (use --force or -y to skip prompt)")
		}
		answer := strings.TrimSpace(scanner.Text())
		if !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(stdout, "clean cancelled")
			return nil
		}
	}

	deleted, err := backup.Prune(backupRoot, keep)
	if err != nil {
		return fmt.Errorf("clean backups: %w", err)
	}

	if len(deleted) == 0 {
		fmt.Fprintf(stdout, "No backups were eligible for cleanup (retaining %d most recent unpinned backups).\n", keep)
		return nil
	}

	fmt.Fprintf(stdout, "Cleaned %d backup(s):\n", len(deleted))
	for _, id := range deleted {
		fmt.Fprintf(stdout, "  - %s\n", id)
	}
	return nil
}
