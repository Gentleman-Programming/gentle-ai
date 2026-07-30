#!/usr/bin/env bash
# Fail when a NEW unreachable function appears.
#
# Why a ratchet and not a clean gate: the repository already carries 230
# functions that nothing reaches from main, and most are legitimate (exported
# API exercised only by tests, platform-specific variants, mocks). Demanding
# that list reach zero before this guard can exist would mean the guard never
# exists. Freezing it and refusing growth is the part that pays for itself.
#
# What this catches: a function nothing calls. That is exactly how
# InstallCommandsForDep survived while the installation docs described the
# dependency remediation it would have performed, and nothing performed.
#
# What this does NOT catch, stated plainly so nobody trusts it further than it
# goes: an unreachable BRANCH inside a live function; a struct field that
# nothing ever assigns; a function that is called but whose effect is dead
# because its input is never configured. This is a call-graph tool. Those three
# shapes are found by executing the product, not by analyzing it.
#
# Regenerate the baseline after deliberately removing dead code:
#   scripts/deadcode-ratchet.sh --update
set -euo pipefail

# comm(1) requires both inputs sorted under the SAME collation, and silently
# produces nonsense when they are not. A baseline sorted under en_US.UTF-8 and
# compared under C (the usual CI locale) reports every baselined entry as new.
# Pinning the collation here and regenerating the baseline under it is what
# makes this reproducible off the author's machine.
export LC_ALL=C

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
baseline="${repo_root}/.deadcode-baseline.txt"
target="${DEADCODE_TARGET:-./cmd/gentle-ai}"
tool="golang.org/x/tools/cmd/deadcode@v0.30.0"

cd "${repo_root}"

# Normalize to "<file>\t<symbol>". Line and column are deliberately dropped:
# pinning them would turn every unrelated edit above a dead function into a
# spurious failure, and a guard that cries wolf gets disabled.
current="$(go run "${tool}" "${target}" 2>/dev/null \
  | sed -E 's/^(.+):[0-9]+:[0-9]+: unreachable func: (.+)$/\1\t\2/' \
  | sort -u)"

if [[ "${1:-}" == "--update" ]]; then
  printf '%s\n' "${current}" > "${baseline}"
  printf 'baseline updated: %s entries\n' "$(printf '%s\n' "${current}" | grep -c '^' || true)"
  exit 0
fi

if [[ ! -f "${baseline}" ]]; then
  printf 'missing %s — run: scripts/deadcode-ratchet.sh --update\n' "${baseline}" >&2
  exit 1
fi

added="$(comm -13 "${baseline}" <(printf '%s\n' "${current}") || true)"
removed="$(comm -23 "${baseline}" <(printf '%s\n' "${current}") || true)"

if [[ -n "${removed}" ]]; then
  printf 'note: %s baselined entries are now reachable or gone.\n' "$(printf '%s\n' "${removed}" | grep -c '^' || true)"
  printf '      Tighten the baseline with: scripts/deadcode-ratchet.sh --update\n\n'
fi

if [[ -n "${added}" ]]; then
  printf 'NEW unreachable functions (nothing calls these):\n\n' >&2
  printf '%s\n' "${added}" | sed 's/^/  /' >&2
  printf '\nEither wire it up, delete it, or if it is genuinely reachable in a way\n' >&2
  printf 'the call graph cannot see, run --update and say why in the commit.\n' >&2
  exit 1
fi

printf 'no new unreachable functions\n'
