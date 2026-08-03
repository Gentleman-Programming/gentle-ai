package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
)

// BackupFlags contains the parsed flags for gentle-ai backup subcommands.
type BackupFlags struct {
	Subcommand string
	JSON       bool
	KeepCount  int
	Force      bool
}

// DefaultBackupRetentionCount is the default retention count for gentle-ai backup clean.
const DefaultBackupRetentionCount = 5

// ParseBackupFlags parses CLI flags for gentle-ai backup subcommands.
func ParseBackupFlags(args []string) (BackupFlags, error) {
	if len(args) == 0 {
		return BackupFlags{Subcommand: "list"}, nil
	}

	sub := args[0]
	switch sub {
	case "list", "ls":
		fs := flag.NewFlagSet("backup list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		jsonFlag := fs.Bool("json", false, "output JSON format")
		if err := fs.Parse(args[1:]); err != nil {
			return BackupFlags{}, fmt.Errorf("parse backup list flags: %w", err)
		}
		if fs.NArg() > 0 {
			return BackupFlags{}, fmt.Errorf("unexpected argument %q for backup list", fs.Arg(0)) // refusal:by-design operator-knowledge: gentle-ai backup list takes no positional arguments; use --json for JSON format
		}
		return BackupFlags{
			Subcommand: "list",
			JSON:       *jsonFlag,
		}, nil

	case "clean":
		fs := flag.NewFlagSet("backup clean", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		keepFlag := fs.Int("keep", DefaultBackupRetentionCount, "number of recent backups to keep")
		forceFlag := fs.Bool("force", false, "suppress confirmation prompt")
		yesFlag := fs.Bool("yes", false, "suppress confirmation prompt (alias for --force)")
		fs.BoolVar(yesFlag, "y", false, "suppress confirmation prompt (alias for --force)")

		if err := fs.Parse(args[1:]); err != nil {
			return BackupFlags{}, fmt.Errorf("parse backup clean flags: %w", err)
		}
		if fs.NArg() > 0 {
			return BackupFlags{}, fmt.Errorf("unexpected argument %q for backup clean", fs.Arg(0)) // refusal:by-design operator-knowledge: gentle-ai backup clean takes no positional arguments; use --keep to specify retention count
		}
		if *keepFlag < 0 {
			return BackupFlags{}, fmt.Errorf("invalid --keep value %d — must be >= 0", *keepFlag) // refusal:by-design operator-knowledge: keep count must be non-negative
		}

		return BackupFlags{
			Subcommand: "clean",
			KeepCount:  *keepFlag,
			Force:      *forceFlag || *yesFlag,
		}, nil

	case "help", "--help", "-h":
		return BackupFlags{Subcommand: "help"}, nil

	default:
		return BackupFlags{}, fmt.Errorf("unknown backup subcommand %q — expected 'list' or 'clean'", sub) // refusal:by-design operator-knowledge: run gentle-ai backup --help to view available subcommands
	}
}

// RunBackup dispatches and executes gentle-ai backup subcommands.
func RunBackup(args []string, stdin io.Reader, stdout io.Writer) error {
	flags, err := ParseBackupFlags(args)
	if err != nil {
		return err
	}

	if flags.Subcommand == "help" {
		PrintBackupHelp(stdout)
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}

	backupDir := filepath.Join(homeDir, ".gentle-ai", "backups")

	switch flags.Subcommand {
	case "list":
		report, err := backup.ListBackups(backupDir)
		if err != nil {
			return fmt.Errorf("list backups: %w", err)
		}

		if flags.JSON {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal backup list JSON: %w", err)
			}
			_, _ = fmt.Fprintln(stdout, string(data))
			return nil
		}

		_, _ = fmt.Fprint(stdout, RenderBackupListReport(report))
		return nil

	case "clean":
		if !flags.Force {
			if stdin == nil {
				stdin = os.Stdin
			}
			fmt.Fprintf(stdout, "This will purge stale backups (retaining up to %d). Continue? [y/N]: ", flags.KeepCount)
			var answer string
			if scanner := bufio.NewScanner(stdin); scanner.Scan() {
				answer = strings.TrimSpace(scanner.Text())
			}
			if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
				fmt.Fprintln(stdout, "Clean operation cancelled.")
				return nil
			}
		}

		deleted, err := backup.CleanBackups(backupDir, flags.KeepCount)
		if err != nil {
			return fmt.Errorf("clean backups: %w", err)
		}

		if len(deleted) == 0 {
			_, _ = fmt.Fprintf(stdout, "No stale backups to clean (retaining up to %d backups).\n", flags.KeepCount)
			return nil
		}

		_, _ = fmt.Fprintf(stdout, "Cleaned %d stale backup(s) (retained %d most recent):\n", len(deleted), flags.KeepCount)
		for _, id := range deleted {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", id)
		}
		return nil

	default:
		return fmt.Errorf("unhandled backup subcommand %q", flags.Subcommand) // refusal:by-design operator-knowledge: unhandled subcommand branch
	}
}

// RenderBackupListReport formats a BackupListReport into a clean human-readable table.
func RenderBackupListReport(report backup.BackupListReport) string {
	if report.TotalCount == 0 {
		return fmt.Sprintf("No safety backups found in %s\n", report.BackupRoot)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s  %-32s  %-6s  %-10s  %s\n", "TIMESTAMP", "NAME / REASON", "FILES", "SIZE", "AGE"))
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	for _, b := range report.Backups {
		tsStr := b.Timestamp.Local().Format("2006-01-02 15:04:05")
		reason := b.ShortReason()
		if runes := []rune(reason); len(runes) > 32 {
			reason = string(runes[:29]) + "..."
		}
		pinnedStr := ""
		if b.Pinned {
			pinnedStr = " [pinned]"
		}
		sb.WriteString(fmt.Sprintf("%-20s  %-32s  %-6d  %-10s  %s%s\n", tsStr, reason, b.FileCount, b.SizeHuman, b.Age, pinnedStr))
	}

	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("Total: %d backup(s) occupying %s in %s\n", report.TotalCount, report.TotalHuman, report.BackupRoot))

	return sb.String()
}

// PrintBackupHelp outputs usage guidance for gentle-ai backup.
func PrintBackupHelp(w io.Writer) {
	fmt.Fprintln(w, `gentle-ai backup — Inspect and manage stored safety backups

Usage:
  gentle-ai backup list [--json]
  gentle-ai backup clean [--keep <N>] [--force]

Commands:
  list, ls    List stored backups with creation timestamps, file counts, and disk sizes
  clean       Purge older unpinned safety backups beyond the retention count

Flags:
  --json      Output machine-readable JSON format (list only)
  --keep N    Number of recent unpinned backups to retain (clean only, default: 5)
  --force, -y Suppress confirmation prompts (clean only)`)
}
