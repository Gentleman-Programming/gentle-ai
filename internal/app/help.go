package app

import (
	"fmt"
	"io"
)

func printHelp(w io.Writer, version string) {
	fmt.Fprintf(w, `gentle-ai — AI Gentle Stack (%s)

USAGE
  gentle-ai                     Launch interactive TUI
  gentle-ai <command> [flags]

COMMANDS
  install      Configure AI coding agents on this machine
  sync         Sync agent configs and skills to current version
  update       Check for available updates
  upgrade      Apply updates to managed tools
  restore      Restore a config backup
  version      Print version

FLAGS
  --help, -h    Show this help

Run 'gentle-ai help' for this message.
Documentation: https://github.com/Gentleman-Programming/gentle-ai
`, version)
}

// printNoTTYGuidance tells the user why gentle-ai refused to start and lists
// the non-interactive subcommands that still work without a terminal.
func printNoTTYGuidance(w io.Writer, version string) {
	fmt.Fprintf(w, `gentle-ai %s — no interactive terminal detected.

The interactive TUI requires a TTY on stdin. If you are running from a
script, CI, or a non-interactive SSH session, use a non-interactive
subcommand instead:

  gentle-ai --version        Print version and exit
  gentle-ai --help           Show full command-line help
  gentle-ai update           Check for available updates
  gentle-ai upgrade          Apply updates to managed tools
  gentle-ai install [flags]  Configure AI coding agents
  gentle-ai sync             Re-apply configuration to current version
  gentle-ai restore [id]     Restore a config backup

`, version)
}
