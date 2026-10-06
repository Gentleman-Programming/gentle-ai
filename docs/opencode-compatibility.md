# OpenCode compatibility

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

Gentle AI selects integrations from the detected OpenCode major version. It does
not silently migrate an existing installation. Unknown, unsupported or ambiguous
version evidence refuses incompatible writes rather than assuming V2.

| Surface | V1 | V2 (tested release: 2.0.4) |
| --- | --- | --- |
| Config, model references and permissions | Existing behavior retained | Native config/profile/MCP handling and permission-preserving merges tested |
| Managed plugins | Existing assets retained | Four assets: telemetry, model catalog, skill registry and review transport |
| Gentle logo | Existing placement unchanged | Explicitly skipped; no equivalent `home_logo` slot, no relocation or config writes |
| Community TUI plugins | Existing integration retained | Compatibility unproven; installation/update refuses, not silently omitted |
| Native review | Existing V1 capability path retained | Admitted only for a V2 runtime whose managed plugin declares the V2 relay contract (see below) |

## What has actually been checked

The four current V2 managed assets target released `@opencode/plugin@2.0.4`
declarations. Earlier isolated actual-host fixtures checked global and project scopes.
A scripted loopback provider proved foreground subagent dispatch, hook ordering,
structured completed output and exact raw child bytes. It also proved inherited
project/child-agent instructions and an empty deny-all child tool inventory.

That ordinary child is **not an isolated reviewer**. Scripted output is transport
proof, not model quality, authority or consent.

## V2 native review

The native capability gate admits OpenCode review only for a matching pair: no
relay declaration with a detected V1 runtime, or the exact managed V2 relay
declaration (`GENTLE_AI_OPENCODE_RELAY_CONTRACT=gentle-ai.opencode-relay/v2-staged`)
with a detected V2 runtime. Any other declaration refuses before the version
probe, and a disagreeing pair refuses, so a V2 host whose PATH resolves a
coexisting V1 binary never inherits V1 capability (or the reverse).

Each `gentle-ai` process runs the `opencode --version` probe at most once per
resolved executable and reuses that answer, failures included, at every review
gate (assess, STATUS, START, consent, relay, capture), so eligibility cannot
change between steps of one invocation. The supported-runtime list in a refusal
never probes: it names what the binary supports, and an OpenCode refusal states
the host condition that failed. A probe that times out (3 seconds) is reported
as a timeout, not as an unsupported runtime; re-run once `opencode --version`
answers promptly.

Proven scope, on a real OpenCode 2.0.19 host with SDK 2.0.4 and external network
denied: the managed V2 review plugin and the real Go relay admit the lens,
refuter and targeted-validator roles; the production gate decides (the relay
keeps the plugin's declaration and detects the real host version), and with no
`opencode` on the relay PATH the same Task is refused. The model is replaced by
a loopback provider that replays Go-computed replies, so this is transport and
admission proof, not reviewer quality. Negatives each leave review authority
unchanged: stale revision, wrong target, background dispatch, session reuse,
replay of a captured Task, truncated, empty, `<task`-prefixed or wrong-role
output, a child provider error, and a Task dispatched to another agent than the
one bound to its role.

- **Agent binding.** The plugin forwards the dispatched subagent name with the
  prompt, and Go refuses a Task whose agent is not the lens agent of its slot,
  `review-refuter` for a refuter Task, or `review-validator` for a validator Task.
  The V1 plugin forwards it too; an older V1 plugin without it is still admitted.
- **Refusal cause.** A refused V2 Task reaches the parent as
  `opencode_review_transport_relay_refused (reason: <reason>)`, where the reason
  is one bounded code (`capability_unavailable`, `envelope_invalid`,
  `agent_mismatch`, `binding_mismatch`, `stale_authority`, `output_refused`,
  `provider_failed`, `relay_unavailable`, `dispatch_refused`). When native
  admission refused the child's result, the refusal also names one bounded cause:
  `opencode_review_transport_relay_refused (reason: output_refused, cause: <cause>)`,
  where the cause is `reviewer_result_not_admissible`,
  `validator_result_not_admissible`, `targeted_validation_inconclusive`,
  `role_capture_failed`, or a native admission diagnostic code
  (`inspection_coverage`, `invalid_finding_location`,
  `evidence_path_out_of_scope`, `proof_path_out_of_scope`,
  `candidate_causality_unclaimed_id`, `candidate_causality_evidence_degraded`).
  The refused bytes and the full admission reason are preserved under
  `<git common dir>/gentle-ai/rejected-results/<lineage>/`. Raw child output,
  paths and free text never reach the parent.
- **Host subagent preamble.** OpenCode 2.0.19 prepends
  `You are a subagent spawned by another session.` to every subagent prompt, so
  the reviewer receives that host line before the Go materialization. The host
  harness tolerates only this exact preamble.
- **Hook chain.** When a plugin's `execute.before` or `execute.after` hook throws
  (as the review plugin does to refuse), OpenCode 2.0.19 skips later plugins'
  hooks for that call. Other plugins must not rely on observing refused review
  Tasks.

V1 regression coverage and V2 fixture checks are recorded in
[`odd/tasks/opencode-v2-support.md`](../odd/tasks/opencode-v2-support.md). Integrated functional checks passed through the scoped follow-ups recorded there;
the initial full-suite command failed on its CLI timeout and legacy Bash. Native
review and complete runtime certification remain separate pending gates.

## Installation and verification boundaries

New CLI installation advice uses `@opencode/cli`; its required install scripts must
not be disabled. Plugin dependencies differ: V1 uses `@opencode-ai/plugin`, while
V2 uses `@opencode/plugin`. Existing user-owned incompatible assets are preserved.
The accepted temporary logo omission does not authorize dropping other features.

Install and global sync converge the managed plugins identically on both runtime
majors: they rewrite them from the running binary, recreate missing ones, and
retire legacy plugins. Retired plugins are `background-agents.ts` and
`review-result-artifacts.ts` (OpenCode and Kilocode) and
`sdd-task-result-artifacts.ts` (OpenCode only). A plugin path counts as Gentle
AI-owned only when it is a regular file whose bytes match a plugin some Gentle AI
release shipped for that name (the digest registry in
`internal/components/opencoderuntimeplugins/`, regenerated with `go generate`
from the release tags; the generator refuses an incomplete local tag set, so run
`git fetch --tags` first). Anything else, including a `plugins` path that is a
symlink or not a directory, stops the operation before any plugin changes and
names the path to move or delete. A symlinked config root is followed. Install,
sync, and upgrade snapshot every plugin path install can write or remove, and
post-sync verification checks that every retired plugin is gone for OpenCode and
Kilocode. Uninstall removes only bytes the same registry recognizes and leaves a
symlinked `plugins` directory and its contents untouched.

Before writing V2 managed plugins, install and sync check the SDK installed in
the OpenCode config directory. Any 2.x release at or above 2.0.4 is accepted, so
an SDK that matches a newer OpenCode runtime is kept as is. A missing SDK, an older
release, a prerelease or another major is refused, and the refusal names the
version it found. The printed command pins the minimum exactly
(`npm install --save-exact ... @opencode/plugin@2.0.4` or
`bun add --exact @opencode/plugin@2.0.4`), so a later routine install or update in
that directory cannot float it. Newer 2.x releases are accepted by semantic
versioning; the managed assets only call `Plugin.define`, but SDK type contracts
are checked against 2.0.4 only.

A config directory that predates V2 can still hold the V1 SDK
`@opencode-ai/plugin` and its peer-installed `@opentui` packages. Those conflict
with the optional `@opentui/core` peer of `@opencode/plugin`, so npm stops with
`ERESOLVE`. When the V1 SDK is present, the npm refusal says so and prints the
same exact install with `--force`, which accepts that optional peer mismatch
and, in the #5208 report, removed no packages. Do not use `--legacy-peer-deps`
or uninstall `@opencode-ai/plugin`: both prune the peer-installed `@opentui`
and `solid-js` packages that existing OpenCode TUI plugins load.

The local conformance harness uses disposable configuration, an allowlisted
environment, fixture authentication and process cleanup. Its loopback mode requires
a supported per-process network guard and never uses an external model. CI checks
released SDK types and fixture unit tests; it does not imply that the Darwin-only
organic host fixture ran on every supported operating system. The broader runtime
matrix (other V2 releases, operating systems, and real reviewer models) remains
unproven.

The opt-in real-host Go tests (`GENTLE_AI_REAL_OPENCODE`, `GENTLE_AI_REAL_PYTHON`,
`GENTLE_AI_REAL_OPENCODE_SDK`, and the real-npm fixtures) require
`GENTLE_AI_APPROVED_TMPDIR` to name the approved absolute temporary root, with
`TMPDIR` equal to it; they fail with that message when it is absent.

### Generic-only host check

Select the host release independently of the existing SDK 2.0.4 fixture:

```sh
python3 -B scripts/test-opencode-v2-host.py /path/to/opencode /path/to/node_modules \
  --host-version 2.0.18 --temp-root "$TMPDIR" --generic-task-only
```

The temporary root must already exist and be writable; an explicitly provided
`TMPDIR` also works without `--temp-root`. Both version and server subprocesses
run under the Darwin loopback-only network sandbox, with isolated HOME/XDG/TMPDIR
and explicit config/test-home directories. No dependencies are installed.

This mode checks only generic foreground dispatch, raw output, hooks, inherited
instructions and tool inventory. It does not copy a native binary, call shell or
review APIs, or configure a review actor; the V2 review proof above uses the
separate `--review-scenario` mode (`--capability-gate stubbed|real`). The legacy
`--loopback /path/to/gentle-ai` review check is mutually exclusive with this mode.
The harness has 66 fixture unit tests (`python3 -m unittest test_opencode_v2_host_test`
from `scripts/`). An isolated OpenCode 2.0.18 run with SDK 2.0.4
also passed in both global and project scopes: four managed plugins activated,
the foreground child returned the expected raw output, hook ordering and
instruction inheritance matched expectations, and the deny-all child exposed
no tools. External network access was blocked. This does not prove the TUI
installation path or positive native review admission.
