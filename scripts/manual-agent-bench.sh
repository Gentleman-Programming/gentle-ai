#!/usr/bin/env bash
# Manual live-agent RDD bench. It deliberately owns only its temporary fixture.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/manual-agent-bench.sh --agent <claude|codex|pi|opencode> --scenario <name> [--keep-work]

Scenarios: direct-correct, direct-rdd-fix, sdd-correct, sdd-rdd-fix.
Runs one foreground, terminal-attached client. Native prompts remain yours to answer.
--test-mode is reserved for scripts/test-manual-agent-bench.sh and verified stubs.
EOF
}
die() { printf 'manual-agent-bench: %s\n' "$*" >&2; exit 2; }
agent=''
scenario=''
keep=0
test_mode=0
while (($#)); do
  case $1 in
    --agent) [[ $# -ge 2 && -z $agent ]] || die 'provide --agent once'; agent=$2; shift 2 ;;
    --scenario) [[ $# -ge 2 && -z $scenario ]] || die 'provide --scenario once'; scenario=$2; shift 2 ;;
    --keep-work) keep=1; shift ;;
    --test-mode) test_mode=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done
[[ $agent =~ ^(claude|codex|pi|opencode)$ ]] || die 'agent must be claude, codex, pi, or opencode'
[[ $scenario =~ ^(direct-correct|direct-rdd-fix|sdd-correct|sdd-rdd-fix)$ ]] || die 'invalid --scenario'
if (( !test_mode )) && { [[ ! -t 0 || ! -t 1 || ! -t 2 ]]; }; then
  die 'live runs require stdin, stdout, and stderr to be TTYs'
fi
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_NAMESPACE

real_gentle="$(command -v gentle-ai || true)"
runtime_path="$(command -v "$agent" || true)"
[[ -n $real_gentle ]] || die 'gentle-ai is not installed'
[[ -n $runtime_path ]] || die "$agent is not installed"
command -v git >/dev/null || die 'git is not installed'
command -v go sha256sum >/dev/null || die 'go or sha256sum is not installed'
if (( test_mode )); then
  test_root=${MANUAL_AGENT_BENCH_TEST_ROOT:-}
  [[ -n $test_root && $real_gentle == "$test_root/bin/"* && $runtime_path == "$test_root/bin/"* ]] ||
    die 'test mode requires verified commands under MANUAL_AGENT_BENCH_TEST_ROOT/bin'
  [[ -n ${MANUAL_AGENT_BENCH_TEST_RM_LOG:-} ]] || die 'test mode requires a cleanup log'
fi
printf 'gentle-ai: '; "$real_gentle" --version
printf '%s: ' "$agent"; "$agent" --version
printf 'git: '; git --version
printf 'go: '; go version

work=''
success=0
cleanup() {
  [[ -n $work ]] || return
  if (( success && !keep )); then
    if (( test_mode )); then
      printf '%q ' /bin/rm -rf "$work" >"$MANUAL_AGENT_BENCH_TEST_RM_LOG"
    else
      /bin/rm -rf "$work"
    fi
  else
    printf 'work root: %s\n' "$work"
  fi
}
work="$(mktemp -d "${TMPDIR:-/tmp}/manual-agent-bench.XXXXXX")"
trap cleanup EXIT
fixture="$work/fixture"
evidence="$work/evidence"
shim_dir="$work/shim"
mkdir -p "$fixture/calc" "$evidence" "$shim_dir"

cat >"$fixture/go.mod" <<'EOF'
module manual-agent-bench

go 1.22
EOF
cat >"$fixture/calc/add.go" <<'EOF'
package calc

func Add(a, b int) int { return 0 }
EOF
cat >"$fixture/calc/add_test.go" <<'EOF'
package calc

import "testing"

func TestAdd(t *testing.T) {
	for _, tt := range []struct{ a, b, want int }{{2, 3, 5}, {0, 7, 7}, {-2, 3, 1}, {-2, -3, -5}} {
		if got := Add(tt.a, tt.b); got != tt.want { t.Fatalf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want) }
	}
}
EOF
git -C "$fixture" init -q
git -C "$fixture" config user.email manual-agent-bench@example.invalid
git -C "$fixture" config user.name manual-agent-bench
git -C "$fixture" add go.mod calc/add.go calc/add_test.go
git -C "$fixture" commit -qm 'baseline fixture'
baseline="$(git -C "$fixture" rev-parse HEAD)"

mode="$("$real_gentle" review mode status --cwd "$fixture" --json)"
grep -Eq '"effective"[[:space:]]*:[[:space:]]*"on"' <<<"$mode" || die 'RDD effective mode must be on; enable it yourself before running'
export MANUAL_AGENT_BENCH_EVENTS="$evidence/events"
export MANUAL_AGENT_BENCH_REAL_GENTLE="$real_gentle"
cat >"$shim_dir/gentle-ai" <<'EOF'
#!/usr/bin/env bash
printf 'rdd BEGIN ' >>"$MANUAL_AGENT_BENCH_EVENTS"; printf '%q ' "$@" >>"$MANUAL_AGENT_BENCH_EVENTS"; printf '\n' >>"$MANUAL_AGENT_BENCH_EVENTS"
"$MANUAL_AGENT_BENCH_REAL_GENTLE" "$@"; status=$?
printf 'rdd END %s ' "$status" >>"$MANUAL_AGENT_BENCH_EVENTS"; printf '%q ' "$@" >>"$MANUAL_AGENT_BENCH_EVENTS"; printf '\n' >>"$MANUAL_AGENT_BENCH_EVENTS"
[[ $status == 0 && ${1:-} == review && ${2:-} == acknowledge-approved ]] && printf 'rdd ACK %s\n' "$(sha256sum "$MANUAL_AGENT_BENCH_FIXTURE/calc/add.go" | cut -d' ' -f1)" >>"$MANUAL_AGENT_BENCH_EVENTS"
exit "$status"
EOF
chmod +x "$shim_dir/gentle-ai"

injector="$work/inject-defect.sh"
export MANUAL_AGENT_BENCH_FIXTURE="$fixture"
cat >"$injector" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if ! go test -C "$MANUAL_AGENT_BENCH_FIXTURE" ./... -count=1; then
  printf 'inject refused: prerequisites failed\n' >>"$MANUAL_AGENT_BENCH_EVENTS"
  exit 1
fi
printf 'package calc\n\nfunc Add(a, b int) int { return a - b }\n' >"$MANUAL_AGENT_BENCH_FIXTURE/calc/add.go"
printf 'inject planted deterministic defect\n' >>"$MANUAL_AGENT_BENCH_EVENTS"
EOF
chmod +x "$injector"

archive_valid() {
  local a n f
  for a in "$fixture"/openspec/changes/archive/????-??-??-*; do
    [[ -d $a && ${a##*/} =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}-.+ ]] || continue; n=${a##*/}; n=${n:11}
    [[ ! -e "$fixture/openspec/changes/$n" && -s $a/state.yaml && -s $a/proposal.md && -s $a/design.md && -s $a/tasks.md && -s $a/verify-report.md && -s $a/archive-report.md ]] || continue
    compgen -G "$a/specs/*/spec.md" >/dev/null || continue
    for f in "$a"/proposal.md "$a"/specs/*/spec.md "$a"/tasks.md "$a"/verify-report.md; do grep -Eiq '(^|[^[:alnum:]_])add([^[:alnum:]_]|$)' "$f" || continue 2; done
    ! grep -Eq '^[[:space:]]*[-*][[:space:]]+\[[[:space:]]\]' "$a/tasks.md" && grep -Eiq '(^|[^[:alnum:]_])(pass|passed|success|successful|verified)([^[:alnum:]_]|$)' "$a/verify-report.md" && return
  done
  return 1
}
prompt="You are the only implementation runtime. Work in the current tiny Go fixture. Never commit, push, add a remote, open a PR, launch background agents, or bypass native prompts. Keep all work foreground."
if [[ $scenario == direct-* ]]; then
  prompt+=" Implement Add(a, b) correctly and run go test ./... -count=1. Do not use SDD or create OpenSpec artifacts."; phase='implementation and uncached tests'
else
  prompt+=" Plan first: proposal, specs, design, tasks. Implement Add only during apply, run go test ./... -count=1, then verify, sync, and archive according to the installed runtime workflow; prove archive completion."; phase='SDD archive completion'
fi
if [[ $scenario == *-rdd-fix ]]; then
  export MANUAL_AGENT_BENCH_INJECTOR="$injector"
  prompt+=" After $phase, execute \$MANUAL_AGENT_BENCH_INJECTOR exactly once. It plants an external defect. Do not repair it before RDD observes and corrects it; finish with passing tests."
  [[ $scenario == sdd-* ]] && prompt+=" This post-SDD mutation is not an SDD verification escape."
else
  unset MANUAL_AGENT_BENCH_INJECTOR || true
fi
prompt+=" Then initiate and follow real negotiated RDD transitions yourself, using only native returned transitions and obtaining human consent where asked."
export MANUAL_AGENT_BENCH_SCENARIO="$scenario"
export PATH="$shim_dir:$PATH"
case $agent in
  claude) command=(claude "$prompt") ;;
  codex) command=(codex "$prompt") ;;
  pi) command=(pi "$prompt") ;;
  opencode) command=(opencode --prompt "$prompt") ;;
esac
printf '%s\0' "${command[@]}" >"$evidence/runtime.argv"
(
  cd "$fixture"
  "${command[@]}"
)

rdd_ok() { local start ack digest; start=$(grep -n '^rdd END 0 review start ' "$evidence/events" | head -1 | cut -d: -f1); ack=$(grep -n '^rdd END 0 review acknowledge-approved ' "$evidence/events" | head -1 | cut -d: -f1); digest=$(sed -n 's/^rdd ACK \([[:xdigit:]]\{64\}\)$/\1/p' "$evidence/events" | tail -1); [[ -n $start && -n $ack && $start -lt $ack && $digest =~ ^[[:xdigit:]]{64}$ && $(sha256sum "$fixture/calc/add.go" | cut -d' ' -f1) == "$digest" ]]; }
sdd_ok() { local start; start=$(grep -n '^rdd END 0 review start ' "$evidence/events" | head -1 | cut -d: -f1); [[ -n $start ]] && awk -v end="$start" 'NR>=end { exit } /^rdd END 0 sdd-attempt acquire / { a++ } /^rdd END 0 sdd-attempt settle / { s++; if (s > a) bad=1 } END { exit !(a >= 2 && s >= 2 && a == s && !bad) }' "$evidence/events"; }
[[ "$(git -C "$fixture" rev-parse HEAD)" == "$baseline" ]] || die 'runtime created a commit'
[[ -z "$(git -C "$fixture" remote)" ]] || die 'runtime added a remote'
go test -C "$fixture" ./... -count=1
rdd_ok || die 'RDD did not successfully start and acknowledge approval through the shim'
if [[ $scenario == sdd-* ]]; then
  sdd_ok || die 'SDD scenario lacks successful paired attempt activity before RDD'
  archive_valid || die 'SDD scenario lacks a complete archived OpenSpec change'
else
  [[ ! -e $fixture/openspec ]] || die 'direct scenario created OpenSpec artifacts'
fi
if [[ $scenario == *-rdd-fix ]]; then
  injected=$(grep -n '^inject planted' "$evidence/events" | head -1 | cut -d: -f1)
  reviewed=$(grep -n '^rdd END 0 review start ' "$evidence/events" | head -1 | cut -d: -f1)
  [[ -n $injected && -n $reviewed && $injected -lt $reviewed ]] || die 'injector did not precede successful review start'
fi
printf 'manual agent bench completed: %s / %s\n' "$agent" "$scenario"
success=1
