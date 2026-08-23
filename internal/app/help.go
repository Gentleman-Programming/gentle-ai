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
  sdd-attempt <acquire|settle> --cwd <repo> --change <change>
               Run bounded normal orchestration without exposing runtime history
  sdd-verify-validate --input <path|-> --requirements <n> --scenarios <n>
               Validate exact verification-report bytes without persistence
  review start [--cwd <repo>] [--base-ref <ref>] [--focus <risk|resilience|readability|reliability>]
  review capture-result --lineage <id> --target <id> --lens <lens> --order <n> --input <review.json>
               Admit one reviewer result; the final capture closes and burns its review
  review capture-correction-plan --lineage <id> --target <id> --expected-revision <rev> --request-hash <hash> --correction-lines <n>
               Capture the positive bounded correction forecast before editing
  review capture-refuter --lineage <id> --target <id> --expected-revision <rev> --input <refuter.json>
               Admit the provider-bound refuter result when STATUS requests it
  review capture-validation --lineage <id> --target <id> --expected-revision <rev> --request-hash <hash> --input <validator.json>
               Admit targeted validator evidence; a passing capture closes and burns its review
  review inspect-candidate --repository-context <handle> --expected-revision <rev> --lineage <id> --target <id> --lens <lens> --order <n> --operation <operation>
               Read one bounded immutable candidate view through provider authority
  review validate --gate <gate> [--cwd <repo>]
               Validate delivery-gate syntax; ordinary repository policy decides delivery
  review status [--cwd <repo>]
               Read-only inventory of compact-v2 and shipped legacy-v1 authority
  review repair --preflight [--cwd <repo>]
               Classify the complete authority inventory before provider-owned repair
  review mode <enable|disable|status> [--cwd <repo>] [--scope <global|clone>]
               User-owned kill switch; off wins, no clone inherits an override,
               status never mutates, and re-enabling applies to future candidates only
               'review start' asks once per clone before a review that would do work;
               accepting the review records that answer, 'not now' applies to that candidate only
               and persists nothing, turning reviews off for good needs a deliberate
               'gentle-ai review mode disable', and a session without a terminal reviews
               the change and says so instead of asking

COMPATIBILITY COMMANDS
  review-start --cwd <repo> --lineage <id> --policy-file <path>
               Read-only legacy v1 surface; rejects new v1 authority and directs users to 'review start'
  review-step --cwd <repo> --lineage <id> --operation <operation> --input <json>
               Read-only legacy v1 surface; rejects mutation
  review-resume --cwd <repo> --lineage <id>
               Read shipped v1 authority without mutation
  review-bundle-export --cwd <repo> --lineage <id> --out <path>
               Export a read-only legacy v1 chain transport
  review-bundle-import --cwd <repo> --bundle <path> [--receipt <path> --request <path>]
               Import a read-only legacy v1 transport
  review-validate --cwd <repo> --receipt <path> (--request <path> | --lineage <id> --gate <gate>)
               Validate read-only legacy v1 authority; ordinary repository policy decides delivery
               Bundle, policy, ledger, fix-delta, evidence, CI, and release flags are compatibility inputs
  sdd-attempt <status|begin|finish|reset|repair> --cwd <repo> --change <change>
               Diagnose or explicitly recover the full native runtime-attempt ledger
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
