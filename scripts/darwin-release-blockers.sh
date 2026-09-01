#!/usr/bin/env bash
# Run the Darwin release-blocker manifest, guarding against manifest drift.
#
# Why the guard exists: `go test -run` with a pattern that matches nothing
# exits 0. A manifest entry naming a renamed or deleted test would therefore
# silently run nothing and the lane would go green while enforcing nothing —
# the same trap the bench capability probes closed. So before running anything
# this script asks each package, natively, via `go test -list`, whether every
# listed test still exists, and refuses to proceed on any miss.
#
# Modes:
#   verify  parse the manifest and run only the existence guard, then exit.
#           Works on any GOOS; on non-Darwin hosts, tests hidden behind a
#           darwin build tag are confirmed at source level instead, because
#           `go test -list` must execute the compiled test binary and a
#           cross-compiled one cannot run here.
#   run     verify, then execute every entry (default). Darwin only: the whole
#           point of the lane is native execution, so running the blockers on
#           another GOOS would prove nothing and is refused.
#
# Manifest format (scripts/darwin-release-blockers.manifest):
#   <package> <TestName> [count=N] [race]     ('#' starts a comment)
# Entries sharing package/count/race are batched into one `go test` call.
#
# Kept compatible with the macOS system bash (3.2): no associative arrays.
set -euo pipefail

export LC_ALL=C

mode="${1:-run}"
case "${mode}" in
  verify|run) ;;
  *)
    printf 'usage: %s [verify|run]\n' "$0" >&2
    exit 2
    ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${repo_root}/scripts/darwin-release-blockers.manifest"
cd "${repo_root}"

if [[ ! -f "${manifest}" ]]; then
  printf 'missing manifest: %s\n' "${manifest}" >&2
  exit 1
fi

host_goos="$(go env GOOS)"

# ---- parse ------------------------------------------------------------------
# entries: one line per manifest entry, tab-separated: pkg, test, count, race.
entries=""
lineno=0
while IFS= read -r raw || [[ -n "${raw}" ]]; do
  lineno=$((lineno + 1))
  line="${raw%%#*}"
  # shellcheck disable=SC2086 # word splitting is the parser here
  set -- ${line}
  [[ $# -eq 0 ]] && continue
  if [[ $# -lt 2 ]]; then
    printf 'malformed manifest line %d (need "<package> <TestName>"): %s\n' "${lineno}" "${raw}" >&2
    exit 1
  fi
  pkg="$1"
  test_name="$2"
  shift 2
  count=1
  race=0
  for opt in "$@"; do
    case "${opt}" in
      count=*)
        count="${opt#count=}"
        if ! [[ "${count}" =~ ^[1-9][0-9]*$ ]]; then
          printf 'manifest line %d: count must be a positive integer, got %s\n' "${lineno}" "${count}" >&2
          exit 1
        fi
        ;;
      race) race=1 ;;
      *)
        printf 'manifest line %d: unknown option %s\n' "${lineno}" "${opt}" >&2
        exit 1
        ;;
    esac
  done
  if ! [[ "${test_name}" =~ ^Test[A-Za-z0-9_]+$ ]]; then
    printf 'manifest line %d: %s is not a Go test identifier\n' "${lineno}" "${test_name}" >&2
    exit 1
  fi
  entries="${entries}${pkg}	${test_name}	${count}	${race}
"
done < "${manifest}"

if [[ -z "${entries}" ]]; then
  printf 'manifest is empty: %s\n' "${manifest}" >&2
  exit 1
fi

entry_total="$(printf '%s' "${entries}" | grep -c '^' || true)"
packages="$(printf '%s' "${entries}" | cut -f1 | sort -u)"

# ---- drift guard ------------------------------------------------------------
missing=0
for pkg in ${packages}; do
  # `go test -list` compiles AND executes the test binary just to print names,
  # which is exactly what makes it authoritative: a listed name is a runnable
  # test, not a string in a file.
  listed="$(go test -list '.*' "${pkg}" | grep '^Test' || true)"
  wanted="$(printf '%s' "${entries}" | awk -F'\t' -v p="${pkg}" '$1 == p {print $2}' | sort -u)"
  for test_name in ${wanted}; do
    # A here-string, not a pipe into grep. Under `set -o pipefail` the pipe
    # form reports the pipeline as failed whenever grep -q finds its match
    # early enough to exit while printf still has output buffered: printf takes
    # SIGPIPE, exits 141, and pipefail surfaces that instead of grep's success.
    # The test is present and the guard calls it drift. It fires only when the
    # listing outgrows the pipe buffer, so it grew with the suite and lands on
    # whichever entries sit early in the listing -- which is why the Darwin
    # lane failed on two of twenty-nine and looked arbitrary.
    if grep -qxF -- "${test_name}" <<<"${listed}"; then
      continue
    fi
    if [[ "${host_goos}" != "darwin" ]] && \
      grep -qrE "^func ${test_name}\(t \*testing\.T\)" "${pkg}"; then
      # A darwin-tagged test cannot be listed here: -list runs the compiled
      # test binary and a GOOS=darwin binary cannot execute on this host.
      # Source-level existence is the best portable answer; the CI lane runs
      # this same guard natively on macOS where -list is authoritative.
      printf 'note: %s %s not listable on GOOS=%s (platform build tag); source-level existence confirmed\n' \
        "${pkg}" "${test_name}" "${host_goos}"
      continue
    fi
    printf 'MANIFEST DRIFT: %s does not define %s\n' "${pkg}" "${test_name}" >&2
    missing=1
  done
done

if [[ "${missing}" -ne 0 ]]; then
  printf '\nrefusing to run: the manifest names tests that no longer exist.\n' >&2
  printf 'Fix the entry or remove it — a pattern matching nothing would exit 0\n' >&2
  printf 'and turn this lane into decoration.\n' >&2
  exit 1
fi

printf 'manifest verified: %s entries across %s packages all exist.\n' \
  "${entry_total}" "$(printf '%s\n' "${packages}" | grep -c '^' || true)"

if [[ "${mode}" == "verify" ]]; then
  exit 0
fi

if [[ "${host_goos}" != "darwin" ]]; then
  printf 'refusing to run release blockers on GOOS=%s: this lane exists for native\n' "${host_goos}" >&2
  printf 'Darwin execution. Use "%s verify" for the drift guard elsewhere.\n' "$0" >&2
  exit 1
fi

# ---- run --------------------------------------------------------------------
# Batch entries sharing package/count/race into one invocation; run every
# batch even after a failure so one red test does not hide another.
groups="$(printf '%s' "${entries}" | awk -F'\t' '{print $1 FS $3 FS $4}' | sort -u)"
status=0
while IFS='	' read -r pkg count race; do
  [[ -z "${pkg}" ]] && continue
  run_pattern="$(printf '%s' "${entries}" \
    | awk -F'\t' -v p="${pkg}" -v c="${count}" -v r="${race}" \
        '$1 == p && $3 == c && $4 == r {print $2}' \
    | sort -u | paste -s -d '|' -)"
  cmd=(go test -v "${pkg}" -run "^(${run_pattern})\$" -count="${count}" -timeout=10m)
  if [[ "${race}" == "1" ]]; then
    cmd+=(-race)
  fi
  printf '\n+ %s\n' "${cmd[*]}"
  if ! "${cmd[@]}"; then
    status=1
  fi
done <<EOF
${groups}
EOF

if [[ "${status}" -ne 0 ]]; then
  printf '\nDarwin release blockers FAILED.\n' >&2
  exit 1
fi
printf '\nDarwin release blockers passed.\n'
