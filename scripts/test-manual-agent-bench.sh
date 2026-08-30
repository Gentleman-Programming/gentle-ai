#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
bench="$repo_root/scripts/manual-agent-bench.sh"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/manual-agent-bench-test.XXXXXX")"
mkdir -p "$test_root/bin"
printf 'test root retained: %s\n' "$test_root"

cat >"$test_root/bin/gentle-ai" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ ${1:-} == --version ]]; then printf 'gentle-ai test\n'; exit 0; fi
if [[ ${1:-} == review && ${2:-} == mode ]]; then printf '{"effective":"on"}\n'; exit 0; fi
[[ ${1:-} == review && ${2:-} == acknowledge-approved && ${MANUAL_AGENT_BENCH_STUB_ACK_FAIL:-0} == 1 ]] && exit 1
[[ ${1:-} == sdd-attempt && ${MANUAL_AGENT_BENCH_STUB_ATTEMPT_FAIL:-0} == 1 ]] && exit 1
exit 0
SH

cat >"$test_root/bin/runtime" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
name="${0##*/}"
if [[ ${1:-} == --version ]]; then printf '%s test\n' "$name"; exit 0; fi
printf '%s\0' "$name" "$@" >>"$MANUAL_AGENT_BENCH_STUB_LOG"
body='return a + b'; [[ ${MANUAL_AGENT_BENCH_STUB_CONSTANT:-0} == 1 ]] && body='return 5'
printf 'package calc\n\nfunc Add(a, b int) int { %s }\n' "$body" > calc/add.go
go test ./... -count=1
if [[ $MANUAL_AGENT_BENCH_SCENARIO == sdd-* ]]; then [[ ${MANUAL_AGENT_BENCH_STUB_EARLY_INJECT:-0} != 1 ]] || "$MANUAL_AGENT_BENCH_INJECTOR"
  d=openspec/changes/archive/2025-01-01-test-change; mkdir -p "$d/specs/fixture"
  [[ ${MANUAL_AGENT_BENCH_STUB_BAD_ARCHIVE:-0} == 1 ]] && : >"$d/archive-report.md" || { for f in state.yaml proposal.md design.md tasks.md verify-report.md archive-report.md; do printf 'Add behavior\n' >"$d/$f"; done; printf 'Add behavior\n' >"$d/specs/fixture/spec.md"; [[ ${MANUAL_AGENT_BENCH_STUB_GARBAGE_ARCHIVE:-0} == 1 ]] || { printf -- '- [x] Implement Add\n' >"$d/tasks.md"; printf 'Add verified: PASS\n' >"$d/verify-report.md"; }; }
  if [[ ${MANUAL_AGENT_BENCH_STUB_NO_ATTEMPTS:-0} != 1 ]]; then for unit in apply verify; do gentle-ai sdd-attempt acquire --work-unit "$unit" || [[ ${MANUAL_AGENT_BENCH_STUB_ATTEMPT_FAIL:-0} == 1 ]]; gentle-ai sdd-attempt settle --work-unit "$unit" || [[ ${MANUAL_AGENT_BENCH_STUB_ATTEMPT_FAIL:-0} == 1 ]]; done; fi
fi
if [[ $MANUAL_AGENT_BENCH_SCENARIO == *-rdd-fix ]]; then "$MANUAL_AGENT_BENCH_INJECTOR"; [[ ${MANUAL_AGENT_BENCH_STUB_SECOND_INJECT:-0} != 1 ]] || { printf 'package calc\n\nfunc Add(a, b int) int { return a + b }\n' > calc/add.go; "$MANUAL_AGENT_BENCH_INJECTOR"; printf 'package calc\n\nfunc Add(a, b int) int { return a + b }\n' > calc/add.go; }; fi
if [[ ${MANUAL_AGENT_BENCH_STUB_NO_REVIEW:-0} != 1 ]]; then
  gentle-ai review status --cwd "$PWD" --contract gentle-ai.review-integration/v2 --next-transition
  [[ ${MANUAL_AGENT_BENCH_STUB_REVIEW:-full} == status ]] || { gentle-ai review start --cwd "$PWD"; [[ $MANUAL_AGENT_BENCH_SCENARIO == *-rdd-fix && ${MANUAL_AGENT_BENCH_STUB_POST_ACK_FIX:-0} != 1 ]] && printf 'package calc\n\nfunc Add(a, b int) int { return a + b }\n' > calc/add.go; gentle-ai review acknowledge-approved --lineage test || true; }
fi
if [[ $MANUAL_AGENT_BENCH_SCENARIO == *-rdd-fix && ${MANUAL_AGENT_BENCH_STUB_POST_ACK_FIX:-0} == 1 ]]; then
  printf 'package calc\n\nfunc Add(a, b int) int { return a + b }\n' > calc/add.go
fi
go test ./... -count=1
SH
chmod +x "$test_root/bin/gentle-ai" "$test_root/bin/runtime"
for agent in claude codex pi opencode; do ln -s runtime "$test_root/bin/$agent"; done

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
red=0
red() { printf 'RED: %s\n' "$*" >&2; red=1; }
run() {
  local agent=$1 scenario=$2 keep=${3:-1}
  local -a args=(--test-mode --agent "$agent" --scenario "$scenario")
  [[ $keep == 1 ]] && args+=(--keep-work)
  env SHELLOPTS=braceexpand:errexit:nounset:pipefail PATH="$test_root/bin:$PATH" TMPDIR="$test_root" MANUAL_AGENT_BENCH_TEST_ROOT="$test_root" \
    MANUAL_AGENT_BENCH_STUB_LOG="$test_root/runtime.argv" \
    MANUAL_AGENT_BENCH_TEST_RM_LOG="$test_root/cleanup.argv" "$bench" "${args[@]}"
}
work_root() { sed -n 's/^work root: //p' <<<"$1" | tail -1; }
assert_runtime() {
  local work=$1 agent=$2 expected_count=2 expected_flag=
  [[ $agent == opencode ]] && { expected_count=3; expected_flag=--prompt; }
  local -a argv=()
  mapfile -d '' -t argv <"$work/evidence/runtime.argv"
  [[ ${argv[0]} == "$agent" ]] || fail "expected $agent runtime"
  [[ ${#argv[@]} == "$expected_count" ]] || fail "wrong argv count for $agent"
  [[ -z $expected_flag || ${argv[1]} == "$expected_flag" ]] || fail "wrong flag for $agent"
}
archive_ok() {
  local d=$1/openspec/changes/archive/2025-01-01-test-change
  [[ -s $d/state.yaml && -s $d/proposal.md && -s $d/design.md && -s $d/tasks.md && -s $d/verify-report.md && -s $d/archive-report.md && ! -e $1/openspec/changes/test-change ]] && compgen -G "$d/specs/*/spec.md" >/dev/null
}
assert_sdd_prompt() {
  local -a argv=(); local prompt
  mapfile -d '' -t argv <"$1/evidence/runtime.argv"; prompt=${argv[${#argv[@]}-1]}
  [[ ${prompt%%'Plan first:'*} != "$prompt" && ${prompt#*'Plan first:'} == *'Implement Add only during apply'* ]] || red 'SDD prompt implements before planning'
  [[ $2 != *-rdd-fix || ${prompt#*'archive completion'} == *'MANUAL_AGENT_BENCH_INJECTOR'* ]] || red 'SDD injection precedes archive'
}
assert_fixture() {
  local work=$1 scenario=$2
  [[ $(git -C "$work/fixture" rev-list --count HEAD) == 1 ]] || fail "runtime created a commit"
  [[ -z $(git -C "$work/fixture" remote) ]] || fail "fixture has a remote"
  go test -C "$work/fixture" ./... -count=1
  grep -Fq 'review status' "$work/evidence/events" || fail "missing negotiated review status"
  grep -Fq 'review start' "$work/evidence/events" || fail "missing review transition"
  if [[ $scenario == sdd-* ]]; then
    archive_ok "$work/fixture" || fail "missing complete SDD archive"
  else
    [[ ! -e $work/fixture/openspec ]] || fail "direct scenario created OpenSpec artifacts"
  fi
  if [[ $scenario == *-rdd-fix ]]; then
    local injected reviewed
    injected=$(grep -n '^inject ' "$work/evidence/events" | head -1 | cut -d: -f1)
    reviewed=$(grep -n '^rdd END 0 review start ' "$work/evidence/events" | head -1 | cut -d: -f1)
    [[ -n $injected && -n $reviewed && $injected -lt $reviewed ]] || fail "injector was not before review"
  fi
}

if grep -E 'go test .*\.\/\.\.' "$bench" "$0" | grep -qv -- '-count=1'; then red 'load-bearing fixture test permits cache'; fi
help_output=$(PATH="$test_root/bin:$PATH" "$bench" --help)
grep -Fq -- '--scenario' <<<"$help_output" || fail "help omits scenario"; grep -Fq 'effective Go 1.22+ toolchain' "$repo_root/docs/testing/manual-agent-bench.md" || red 'documentation omits the effective Go 1.22+ toolchain prerequisite'
if PATH="$test_root/bin:$PATH" "$bench" --test-mode --agent nope --scenario direct-correct >/dev/null 2>&1; then
  fail "invalid agent was accepted"
fi
if PATH="$test_root/bin:$PATH" "$bench" --test-mode --agent claude --scenario direct-correct >/dev/null 2>&1; then
  fail "unverified test mode was accepted"
fi
if PATH="$test_root/bin:$PATH" "$bench" --agent claude --scenario direct-correct >/dev/null 2>&1; then
  fail "non-TTY live run was accepted"
fi
for agent in claude codex pi opencode; do for scenario in direct-correct direct-rdd-fix sdd-correct sdd-rdd-fix; do
  output=$(run "$agent" "$scenario")
  work=$(work_root "$output")
  [[ -d $work ]] || fail "kept work root missing"
  assert_runtime "$work" "$agent"
  assert_fixture "$work" "$scenario"
  [[ $scenario == sdd-* ]] && assert_sdd_prompt "$work" "$scenario"
done; done

if MANUAL_AGENT_BENCH_STUB_CONSTANT=1 run claude direct-correct >/dev/null 2>&1; then red 'constant implementation accepted'; else :; fi
if MANUAL_AGENT_BENCH_STUB_BAD_ARCHIVE=1 run pi sdd-correct >/dev/null 2>&1; then red 'incomplete SDD archive accepted'; else :; fi
if MANUAL_AGENT_BENCH_STUB_REVIEW=status run claude direct-correct >/dev/null 2>&1; then red 'review status alone accepted'; else :; fi
if failed_ack_output=$(MANUAL_AGENT_BENCH_STUB_ACK_FAIL=1 run claude direct-correct); then red 'nonzero RDD acknowledgement accepted'; else failed_ack_work=$(work_root "$failed_ack_output"); [[ -d $failed_ack_work && $(grep -Ec '^rdd END 1 review acknowledge-approved ' "$failed_ack_work/evidence/events" || true) -eq 1 ]] || red 'nonzero RDD acknowledgement omitted END evidence under inherited errexit'; fi
if MANUAL_AGENT_BENCH_STUB_NO_ATTEMPTS=1 run pi sdd-correct >/dev/null 2>&1; then red 'SDD archive without attempts accepted'; else :; fi
if MANUAL_AGENT_BENCH_STUB_ATTEMPT_FAIL=1 run pi sdd-correct >/dev/null 2>&1; then red 'failed SDD attempts accepted'; else :; fi
if MANUAL_AGENT_BENCH_STUB_GARBAGE_ARCHIVE=1 run pi sdd-correct >/dev/null 2>&1; then red 'semantic-free SDD archive accepted'; else :; fi
external="$test_root/external"; git init -q "$external"; git -C "$external" config user.email external@example.invalid; git -C "$external" config user.name external; printf 'external\n' >"$external/state"; git -C "$external" add state; git -C "$external" commit -qm external
ext_head=$(sha256sum "$external/.git/HEAD"); ext_config=$(sha256sum "$external/.git/config"); ext_index=$(sha256sum "$external/.git/index")
if hostile_output=$(GIT_DIR="$external/.git" GIT_INDEX_FILE="$external/.git/index" run claude direct-correct); then hostile_work=$(work_root "$hostile_output"); [[ -d $hostile_work && $ext_head == "$(sha256sum "$external/.git/HEAD")" && $ext_config == "$(sha256sum "$external/.git/config")" && $ext_index == "$(sha256sum "$external/.git/index")" ]] || red 'hostile Git selection changed external repository or broke fixture'
else red 'hostile Git selection prevented fixture run'; fi
if MANUAL_AGENT_BENCH_STUB_POST_ACK_FIX=1 run claude direct-rdd-fix >/dev/null 2>&1; then red 'post-acknowledgement implementation fix accepted'; else :; fi
if MANUAL_AGENT_BENCH_STUB_SECOND_INJECT=1 run claude direct-rdd-fix >/dev/null 2>&1; then red 'multiple injector events accepted'; else :; fi; if early_output=$(MANUAL_AGENT_BENCH_STUB_EARLY_INJECT=1 run pi sdd-rdd-fix); then red 'early SDD injection was accepted'; fi; early_work=$(work_root "$early_output"); if [[ -d $early_work && ! -e $early_work/fixture/openspec ]] && ! grep -Fq 'inject planted deterministic defect' "$early_work/evidence/events" && grep -Fq 'inject refused: complete SDD archive required' "$early_work/evidence/events"; then :; else red 'early SDD injection did not refuse before archive creation'; fi
if output=$(run claude direct-correct 0); then
  read -r cleanup_cmd flag root extra <"$test_root/cleanup.argv" || true; [[ $cleanup_cmd == /bin/rm && $flag == -rf && $root == "$test_root"/manual-agent-bench.* && -z $extra ]] || fail "cleanup did not own exactly one work root"
else
  fail "cleanup stub run failed: $output"
fi
if PATH="$test_root/bin:$PATH" MANUAL_AGENT_BENCH_TEST_ROOT="$test_root" \
  MANUAL_AGENT_BENCH_STUB_LOG="$test_root/no-review.argv" MANUAL_AGENT_BENCH_TEST_RM_LOG="$test_root/no-review-cleanup.argv" MANUAL_AGENT_BENCH_STUB_NO_REVIEW=1 \
  "$bench" --test-mode --agent claude --scenario direct-correct --keep-work >/dev/null 2>&1; then
  fail "missing review activity was accepted"
else
  :
fi
(( red == 0 )) || exit 1
printf 'manual agent bench tests: PASS\n'
