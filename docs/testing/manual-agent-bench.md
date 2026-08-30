# Manual live-agent bench

This foreground bench lets one installed agent client complete a tiny Go task, then exercise real receipt-driven development (RDD) in a fresh temporary repository. It is an operator-run check, not an automated model suite.

## Prerequisites

- POSIX Bash host; Windows-native support is out of scope.
- A controlling terminal: stdin, stdout, and stderr must all be TTYs.
- Installed `gentle-ai` with RDD already effectively `on`; the bench never enables it.
- Installed client versions: Claude Code 2.1.251, Codex CLI 0.149.1, Pi 0.84.3, or OpenCode 1.18.18.
- Git, Go, and `sha256sum`. Your existing `HOME`, authentication, and client configuration are used unchanged.

Check RDD before starting:

```bash
gentle-ai review mode status --cwd "$PWD" --json
```

## Run one session

```bash
scripts/manual-agent-bench.sh --agent claude --scenario direct-correct
```

The script requires a real terminal and launches exactly one client in the foreground. It never pipes or redirects the client, sets `CI=1`, answers prompts, creates a remote, or starts a background session.

## Run the matrix sequentially

Each iteration is one potentially paid, interactive session; run only the sessions you intend to observe.

```bash
for agent in claude codex pi opencode; do
  for scenario in direct-correct direct-rdd-fix sdd-correct sdd-rdd-fix; do
    scripts/manual-agent-bench.sh --agent "$agent" --scenario "$scenario"
  done
done
```

| Scenario | Required flow |
| --- | --- |
| `direct-correct` | Implement `Add`, run uncached tests, then complete RDD without SDD. |
| `direct-rdd-fix` | First reach a correct uncached state; the external injector plants `a - b` immediately before RDD. |
| `sdd-correct` | Plan first, implement only during apply, then verify, sync, prove archive completion, and complete RDD. |
| `sdd-rdd-fix` | Complete SDD while correct, then inject the defect after archive and before RDD; this post-SDD mutation is not an SDD verification escape. |

Defect scenarios expose a harness-owned injector outside the fixture through `MANUAL_AGENT_BENCH_INJECTOR`. It runs tests before changing only `calc/add.go`, records its event outside the candidate, and fails unchanged when the correct state is absent. The runtime must not repair that change before RDD sees it.

## Observe and verify

The harness creates a baseline commit with an unimplemented `Add`, no remote, and table-driven tests. It resolves the real `gentle-ai` before adding a transparent shim to `PATH`; the shim records bounded shell-quoted BEGIN/END argv and exit-status evidence outside the fixture, invokes the resolved binary with the terminal inherited, and preserves its status.

After the client exits, the harness checks that HEAD is still the baseline, no remote exists, final `go test ./... -count=1` passes, and the final `calc/add.go` digest matches the one recorded after successful acknowledgement. RDD requires a successful `review start` followed by successful `review acknowledge-approved`; SDD also requires two ordered successful `sdd-attempt acquire`/`settle` pairs before RDD, a complete dated archive with no active change, `Add` named in proposal/spec/tasks/verification evidence, no unchecked tasks, and a successful verification record. Defect scenarios also require injection to precede successful review start.

Digest binding proves only that the approved implementation remained final; model finding and correction causality remain human-observed.

Successful runs are removed unless `--keep-work` is supplied. Failed and kept runs print their harness-owned work-root path for inspection; helper scripts and evidence logs stay outside the reviewed fixture.

## Deterministic test

No real client is launched by the shell test. It uses verified temporary stubs to cover parsing, exact argv construction, uncached fixture and injector checks, SDD ordering/archive controls, foreground sequencing, post-run checks, and cleanup ownership:

```bash
bash scripts/test-manual-agent-bench.sh
```
