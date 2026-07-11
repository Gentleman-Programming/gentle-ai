package app

import (
	"fmt"
	"io"
)

func printHelp(w io.Writer, version string) {
	fmt.Fprintf(w, `gentle-ai — Gentle-AI: Ecosystem, Frameworks, Workflows (%s)

USAGE
  gentle-ai                     Launch interactive TUI
  gentle-ai <command> [flags]

COMMANDS
  install      Configure AI coding agents on this machine
  uninstall    Remove Gentle AI managed files from this machine
  sync         Sync agent configs and skills to current version
  skill-registry refresh
               Refresh .atl/skill-registry.md with cache-hit fast path
  sdd-status [change]
               Print native SDD phase status for orchestrators
  sdd-continue [change]
               Print native SDD dispatcher routing output
	  review-start --cwd <repo> --lineage <id> --policy-file <path> [--mode ordinary_bounded] [--lens <name> ...]
	               Build a target and append to the repository-derived review store
	  review-resume --cwd <repo> --lineage <id>
	  review-step --cwd <repo> --lineage <id> --operation <operation> --input <json>
	               Append a lifecycle step; record-lens-result derives identity from structured findings and evidence
	  review-bundle-export --cwd <repo> --lineage <id> --out <path>
	               Export the validated full chain as a portable content-addressed bundle
	  review-bundle-import --cwd <repo> --bundle <path> --receipt <path> --request <path>
	               Validate and install a portable chain into this repository's store
  review-validate --cwd <repo> --receipt <path> --request <path>
               Derive current facts and validate a content-bound lifecycle receipt
  update       Check for available updates
  upgrade      Apply updates to managed tools
  restore      Restore a config backup
  doctor       Run ecosystem health diagnostics
  version      Print version

FLAGS
  --help, -h    Show this help

Run 'gentle-ai help' for this message.
Documentation: https://github.com/Gentleman-Programming/gentle-ai
`, version)
}
