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
  review start [--cwd <repo>] [--base-ref <ref>] [--focus <risk|resilience|readability|reliability>]
  review finalize [--cwd <repo>] [--result <review.json> ...] [--evidence <path>]
  review validate --gate <gate> [--cwd <repo>]
               Normal review path; ordinary authority is compact state plus receipt

COMPATIBILITY COMMANDS
  review-start --cwd <repo> --lineage <id> --policy-file <path>
               Read-only legacy v1 surface; rejects new v1 authority and directs users to 'review start'
  review-step --cwd <repo> --lineage <id> --operation <operation> --input <json>
               Read-only legacy v1 surface; rejects mutation and directs users to 'review finalize'
  review-resume --cwd <repo> --lineage <id>
               Read shipped v1 authority without mutation
  review-bundle-export --cwd <repo> --lineage <id> --out <path>
               Export compact current-state transport or a legacy v1 chain transport
  review-bundle-import --cwd <repo> --bundle <path> [--receipt <path> --request <path>]
               Import compact transport; receipt/request extras apply only to legacy v1 transport
  review-validate --cwd <repo> --receipt <path> (--request <path> | --lineage <id> --gate <gate>)
               Validate legacy v1 authority; native mode needs lineage/gate and derives authority
               Bundle, policy, ledger, fix-delta, evidence, CI, and release flags are optional compatibility or exceptional inputs
  update       Check for available updates
  upgrade      Apply updates to managed tools
  restore      Restore a config backup
  doctor       Run ecosystem health diagnostics
  version      Print version

FLAGS
  --help, -h    Show global help; every review subcommand also supports help

Run 'gentle-ai help' for this message.
Documentation: https://github.com/Gentleman-Programming/gentle-ai
`, version)
}
