#!/usr/bin/env bash
# Local cross-lane integration battery. NOT wired into CI on purpose:
# the optional --with-model lane spends real reviewer model runs on the
# development subscription, and every lane drives a real gentle-ai binary
# end to end against live scratch repositories.
#
# Usage:
#   scripts/cross-lane-battery.sh --binary /path/to/gentle-ai [--with-model] [--with-host] [--keep-work]
#
# Deterministic lanes (always run):
#   opencode  - drives the REAL OpenCode transport plugin bytes through an
#               emulated Task hook surface with HOST-assembled binding frames
#               (lens frame and validator role frame).
#   claude    - one low-risk full lifecycle to gate allow, plus one medium
#               candidate consent/v3 round-trip; --with-model additionally
#               runs the real compiled claude-code reviewer runtime.
#   advisory  - the middle path: a medium candidate reviewed into one WARNING
#               and one SUGGESTION must reach an approved receipt that
#               declares both non-blocking and offers no correction route.
#   schema    - validates every envelope captured above against the published
#               schemas in contracts/review-integration/.
#
# --with-host lanes (REAL host applications, dev subscription):
#   host-codex    - `codex exec` spawned by the compiled codex adapter runs a
#                   medium reviewer capture (--agent codex) to the receipt.
#   host-pi       - the INSTALLED gentle-pi review-host-relay code runs a real
#                   locked-down print-mode `pi` reviewer; refuter/validator
#                   legs run through the Go-owned pi spawn (--execute).
#   host-opencode - a real headless `opencode run` session in a sandboxed HOME
#                   with the real transport plugin: relay start/completion
#                   frames must flow through the live host hooks.
# Every with-host lane is bounded, PASS/FAIL/SKIP(reason) per check, sandboxed
# where a host store would otherwise be touched, and the summary prints the
# real model runs spent.
#
# Exit code is non-zero when any check fails. Known-red checks (pending
# fixes referenced in the output) still fail: red is the battery working.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"
exec go run ./scripts/crosslane "$@"
