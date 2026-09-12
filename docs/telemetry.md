# Telemetry

Gentle AI sends a small amount of anonymous usage telemetry so the project
knows how many installs stay alive and how the review pipeline gets used,
without collecting anything about you, your code, your machine, or your
organization.

## Runtime metrics: one attempt, no client storage

A tool event makes one best-effort send attempt to the collector. Failure silently
loses that observation. Runtime metrics never enter a local queue, outbox, file,
lock, cooldown, daemon, or retry scheduler. Existing telemetry opt-out policy is
read-only; this path never enrolls, repairs policy, or generates an install ID.
Stale runtime queue/outbox/kick/lock files are ignored and left untouched. There is
no migration cleanup. Legacy install/heartbeat telemetry below is separate.

### Pi command contract

Invoke `gentle-ai telemetry runtime send --json` with one sanitized JSON object on
stdin, at most **16 KiB**. The CLI allows **500 ms to read stdin**, then discards
incomplete input. Close stdin normally. Launch asynchronously from the tool event
hook; **do not await the process or network**. Discard command output/errors and
never retry or retain failed input. The command itself runs synchronously for one
bounded stdin read followed by one HTTP attempt and never spawns itself. No `ingest`, `flush`, or `capabilities` route
remains. Unsupported older binaries must fail closed, not fall back to intake.

The unreleased [aggregate schema](../contracts/telemetry/runtime/v1/schemas/aggregate.schema.json)
requires exactly `schema`, `registry`, `host`, and `rows`. **Remove `batch_id` from
Pi's mirrored schema and producer.** No source/session/task/install/user identity
is accepted. Example single response observation:

```json
{
  "schema": "gentle-ai.telemetry-runtime-aggregate/v1",
  "registry": 1,
  "host": "pi",
  "rows": [{
    "model": {"provider":"openai","id":"gpt-5.4"},
    "model_evidence": "response",
    "agent_kind": "built_in",
    "agent_class": "sdd-apply",
    "selected_effort": "minimal",
    "effective_effort": "unavailable",
    "launches": null,
    "responses": 1,
    "input_tokens": {"reported":1,"unavailable":0,"unsupported":0,"sum":12},
    "output_tokens": {"reported":1,"unavailable":0,"unsupported":0,"sum":0},
    "cache_read_tokens": {"reported":0,"unavailable":1,"unsupported":0,"sum":0},
    "cache_creation_tokens": {"reported":0,"unavailable":1,"unsupported":0,"sum":0},
    "reasoning_tokens": {"reported":0,"unavailable":0,"unsupported":1,"sum":0},
    "total_tokens": {"reported":0,"unavailable":1,"unsupported":0,"sum":0},
    "error_category": "none",
    "duration": {"kind":"unavailable","measured_count":0,"sum_ms":null}
  }]
}
```

The command returns exactly:

```json
{"schema":"gentle-ai.telemetry-runtime-send/v1","decision":"stored"}
```

| Decision | Meaning |
|---|---|
| `stored`, `duplicate` | Exact HTTP 200 collector acknowledgement observed |
| `disabled` | Existing policy disallows sending; no HTTP |
| `discarded` | Invalid input, unavailable endpoint, timeout, or failed/invalid acknowledgement; nothing retained |
| `ignored` | OpenCode only: partial, summary, or non-assistant event |

These decisions exit successfully and never contain raw input or errors. Invalid
command syntax returns a fixed usage error, without echoing arguments. Stdout
write failures remain ordinary CLI errors, not permission to retry metrics.

### Public observation fields

Rows are independent observations, **not session composition**. Up to 32 rows fit
within the same 16 KiB ceiling; do not accumulate sessions or send overlapping
cumulative snapshots. No parent-child links or source IDs exist.

| Field | Source rule |
|---|---|
| `host` | `pi`, `opencode`, `claude-code`, or `codex`; never a hostname |
| `model` | Registered public provider/model pair, otherwise the `unknown` or `custom` pair |
| `model_evidence` | `selected`, `response`, or `unknown`; selected model is not proof of response model |
| `agent_kind`, `agent_class` | Closed broad category and known package class, otherwise `unknown`; no private agent names. See the canonical vocabulary below. |
| `selected_effort`, `effective_effort` | Independent source evidence; never infer one from the other |
| `launches`, `responses` | Source-observed event occurrence counts, `null` if unobserved, or `"unsupported"`; not sessions, successes, or reconstructed composition |
| Six token fields | Independent `{reported, unavailable, unsupported, sum}` coverage objects |
| `duration` | Typed source-reported request or message elapsed time, never inferred latency |
| `error_category` | `none`, `unknown`, `auth`, `output_length`, `aborted`, `api`, `rate_limit`, or `server`; never error text |

### Agent class vocabulary

Canonical `agent_class` values are:

- Fixed classes: `orchestrator`, `worker`, `explore`, `verify`, `unknown`
- SDD agents: `sdd-init`, `sdd-explore`, `sdd-research`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, `sdd-archive`, `sdd-onboard`, `sdd-status`, `sdd-sync`
- Judgment Day agents: `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`
- Review agents: `review-risk`, `review-readability`, `review-reliability`, `review-resilience`, `review-refuter`, `review-validator`

The collector accepts deprecated `sdd-proposal` from older registry-1 clients and normalizes it to `sdd-propose` before storage.

Occurrence counts remain because they describe the event's observed coverage, not
session reconstruction. Token coverage does not derive from these counts. A
reported zero has positive `reported`; `reported:0` requires `sum:0`. Unavailable
coverage on one response is `{reported:0, unavailable:1, unsupported:0, sum:0}`;
no token observations at all use four zeros. Do not derive totals from other
counters or change source cache semantics. Counts are exact integers from 0 through
999999999999; equivalent numeric spellings are canonicalized without float loss.

Effort values are `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`,
`not_selected`, `unsupported`, `unavailable`, `unknown`, or `custom`.
`off` is a selection, not unsupported reasoning. Unknown association means
unavailable evidence, not fabricated consumption or effort.

Without timing evidence use exactly
`{"kind":"unavailable","measured_count":0,"sum_ms":null}`. Measured timing uses
`kind: request|message`, positive `measured_count`, and nonnegative `sum_ms` up to
999999999999 (fractional milliseconds allowed). Count timed attempts/messages,
including failures; never label message elapsed time as provider latency.

Unknown fields, missing fields, duplicate keys, aliases, trailing JSON, unregistered
raw names, and invalid bounds are refused. Registry `1` is a static allowlist, not
routing attestation; there is no network model lookup. Official source references:

- https://platform.claude.com/docs/en/models/opus-5/overview
- https://platform.claude.com/docs/en/models/haiku-4-5/overview
- https://code.claude.com/docs/en/monitoring-usage
- https://developers.openai.com/codex/models

Update the Go allowlist and schema together when deliberately revising this registry.

### One bounded HTTPS attempt

The existing default or `GENTLE_AI_TELEMETRY_ENDPOINT` supplies the HTTPS authority.
Native code replaces the path with `/v1/runtime-events` and removes query/fragment.
HTTP, URL userinfo (including username-only), and redirects are refused. One POST
has a **3-second total network timeout**, including response reading; acknowledgements
are capped at **1 KiB**. The CLI reads raw stdin once with a **500 ms deadline**
and a 16 KiB limit (plus one oversize-detection byte), then passes an in-memory
reader to either native path. Timeout returns the fixed `discarded` decision with
zero HTTP. Input and network budgets total **3.5 seconds**, plus bounded parsing
and policy work; blocking policy filesystem reads or stdout writes are not covered.
OpenCode's existing 4-second process timeout remains an outer safety bound.

The CLI does not close caller-owned readers or shared `os.Stdin`. A timed-out read
can leave **one process-scoped reader goroutine** until stdin completes or the
standalone process exits. A buffered result prevents late completion from blocking;
the deadline timer is canceled on return. This is not a daemon or retry mechanism.
The library `SendRuntime`/`SendOpenCode`/`SendClaude`/`SendCodex` APIs still accept caller-owned readers:
their context bounds HTTP, not arbitrary blocking `Read` implementations. Embedded
callers must supply bounded input; only the CLI owns this pre-read deadline.

Native code generates a fresh independent cryptorandom 32-hex `delivery_id` and
converts only the envelope to
[`gentle-ai.telemetry-runtime-event/v1`](../contracts/telemetry/runtime/v1/schemas/event.schema.json).
Public rows remain unchanged. Neither payload nor ID is saved. There are no
idempotency headers, replayable request bodies, retries, backoff, or spacing state.
Every non-200 status (including 400/409/429/500), malformed acknowledgement, lost
response, or timeout discards the metrics. Even a lost successful response does
not authorize another client attempt.

Fresh environment and existing enrolled policy checks run before stdin, after
reading, and immediately before HTTP. Missing/malformed policy, opt-out, or notice
not shown fail closed. A policy change after the final check can race a request;
no checkpoint can unsend HTTP already in flight. No policy lock is introduced.

## Automatic OpenCode collection

The managed plugin invokes `gentle-ai telemetry runtime opencode --json` directly
and asynchronously. Native code checks policy, normalizes one bounded source
observation, then uses the same one-attempt sender. Example stdin:

```json
{"schema":"gentle-ai.telemetry-opencode/v1","info":{"role":"assistant","time":{"created":1,"completed":3},"providerID":"anthropic","modelID":"claude-opus-5","agent":"sdd-apply"}}
```

Only completed non-summary assistant `message.updated` events qualify. Source
compatibility is pinned to [plugin 1.18.30](https://unpkg.com/@opencode-ai/plugin@1.18.30/dist/index.d.ts)
and its [V1 SDK](https://unpkg.com/@opencode-ai/sdk@1.18.30/dist/gen/types.gen.d.ts),
not V2; this is not an installed-runtime smoke test. Source timestamps become
message elapsed time and do not leave native code. The source `mode` is forwarded
as `agent` only when it is 1-64 printable ASCII characters. Unknown provider/model
names become `custom`; error names/status become closed categories. Prompts, parts,
paths, raw error text, tools, and source/session/task IDs never enter the envelope.

The hook does not read message/session IDs, reconstruct sessions, dedupe across
events, or retain failed payloads. Each event gets at most one native process.
Bounds are 32 in-flight processes, 16 KiB stdin, 1 KiB output, and a 4-second process
safety timeout around native's 3-second HTTP budget. Saturation discards new events,
not queues them. Callbacks only release process bookkeeping; they never retry.
Disposal terminates active children. Repeated source events can be counted again:
this intentionally makes no exactly-once coverage claim.

Missing/default-zero source tokens remain unavailable; available reasoning is
preserved, and total tokens are not inferred. Native `build` and `plan`, plus
`gentle-orchestrator`, map to the orchestrator class. OpenCode's managed fallback
agents map `explore` to the built-in `explore` class and `general` to the built-in
`worker` class. Agent names in the runtime contract's named Gentle AI allowlist
map to their built-in class. Other non-empty names map to `custom`/`unknown`;
their raw names never leave native code. Missing names remain `unknown`/`unknown`.

For the observed agent, native code reads up to 1 MiB from the local `opencode.json`
`agent.<name>.model` and `agent.<name>.variant` assignment. A valid contract effort
becomes `selected_effort`; `effective_effort` remains `unavailable` because OpenCode
does not report it. A response provider/model remains authoritative with
`model_evidence: response`; the assigned model is used with
`model_evidence: selected` only when the response omits provider/model. Missing,
oversized, or invalid configuration falls back to unavailable/unknown attribution
and never blocks the send.

Install/sync still reconcile the dedicated `plugins/telemetry-runtime.ts` and
`.gentle-ai-telemetry-runtime.json` ownership manifest for selected OpenCode,
independently of SDD and using the existing scope/XDG resolution. These are static
installation assets, **not metric state**. Managed byte/hash/mode checks, guarded
rollback, unowned/edited-file preservation, and validated-pair uninstall remain
unchanged. Each managed plugin asset change must append the immediately previous
embedded asset digest to the explicit provenance allowlist so owned installs can
upgrade without treating arbitrary self-consistent content as package provenance.
Disable leaves the plugin inert under existing policy; installation never reenrolls
or changes exporter settings.

## Automatic Claude Code collection

Claude Code installs asynchronous `Stop` and `SubagentStop` command hooks that
invoke `gentle-ai telemetry runtime claude --json`. Each hook starts one one-shot
process; native code checks the existing telemetry policy before reading stdin,
uses the same 16 KiB/500 ms input bound, and sends at most once with no daemon,
queue, persistence, retry, or filesystem mutation.

For `SubagentStop`, the documented `agent_type` names the subagent frontmatter.
Names in Gentle AI's runtime agent-class registry become `built_in` observations;
all other names become `custom`/`unknown` without transmitting the name.

For `SubagentStop`, the adapter reads at most the last 512 KiB of the matching
agent transcript, from inside the user's home only. It scans backward for the
last complete assistant record carrying valid usage, skipping malformed,
partial, and non-usage records. A memory-only `last_assistant_message` match is
stronger correlation evidence, but a missing or different value does not discard
usage because `agent_transcript_path` is scoped to that subagent run. The record
supplies response model plus input, output, cache-read, and cache-creation tokens.
Reasoning tokens are unsupported and total tokens are unavailable; neither is
inferred.

`Stop` always becomes an `orchestrator` activity-only launch-style observation.
It does not read or attribute transcript usage or a response model: without a
unique response identity or persistent replay state, a repeated final message
could match an older record when the current transcript row has not been flushed.

For a known named subagent, at most 64 KiB of
`~/.claude/agents/<agent_type>.md` supplies selected model and selected effort.
The selected model is used only when no response model exists. The hook contract
does not expose effective effort, duration, or an error shape: effective effort
and duration remain unavailable, while successful `Stop`/`SubagentStop` events
use error category `none` (API failures fire the separate `StopFailure` event).
Selected aliases map as `sonnet` to `claude-sonnet-5`, `opus` to
`claude-opus-5`, and `haiku` to `claude-haiku-4-5`; `inherit`, `default`, and an
empty selector remain unknown. Transcript release/revision suffixes are reduced
by longest registered-ID prefix, so for example `claude-sonnet-5-20260501` maps
to `claude-sonnet-5`. Other model IDs map to `custom`/`custom`.

If a subagent transcript is missing, unreadable, or has no valid assistant usage,
the completion records the same launch-style occurrence with unavailable token
coverage rather than claiming a response. This preserves observed agent activity
without fabricating response evidence. Transcript paths,
hook/session/agent IDs, cwd, prompts, messages, transcript content, and private
agent names never enter the aggregate or logs. Symlinks resolving outside the
user's home are refused.

Hook and subagent-field behavior follows the official
[Claude Code hooks reference](https://code.claude.com/docs/en/hooks) and
[subagent reference](https://code.claude.com/docs/en/sub-agents). Transcript JSONL
`message.model` and `message.usage` shapes are observed implementation evidence,
not documented compatibility guarantees in those references.

## Automatic Codex collection

Managed Codex `hooks.json` entries invoke
`gentle-ai telemetry runtime codex --json` asynchronously for `SubagentStop` and
`Stop`. These events and their input fields are documented by the
[Codex hooks reference](https://developers.openai.com/codex/hooks). Runtime policy is
checked before hook stdin, local state, or transcript data is read, again after
stdin, and before the one-attempt sender. Disabled telemetry therefore leaves
the installed hooks inert. The command writes no stdout because Codex validates
JSON-looking output as a hook response even for asynchronous hooks.

The hook input is limited to 16 KiB. For `SubagentStop`, native code separately
reads at most the first 64 KiB of `agent_transcript_path` for attribution. Only
the first `session_meta` record is eligible: the basename of
`source.subagent.thread_spawn.agent_path` is normalized from Codex's underscored
task name to the hyphenated runtime class, then checked against the exact Go
allowlist. A directly allowlisted hook `agent_type` takes precedence; every
unrecognized result remains `custom`/`unknown`. The nickname, IDs, and source
path are discarded in memory and never enter telemetry.

Native code also retains at most the final 256 KiB of `agent_transcript_path`
for `SubagentStop`, or `transcript_path` for `Stop`; one look-behind byte
determines whether the first retained line is complete.
The final observed `turn_context` starts the eligible evidence segment. Its valid
model and effort and that segment's latest valid `event_msg` `token_count`
`last_token_usage` are used together. The public hook contract does not establish
that hook `turn_id` has matching semantics in both parent and subagent transcripts,
so sequence segmentation is the bounded fallback and private IDs stay memory-only.
Codex's [subagent documentation](https://developers.openai.com/codex/subagents)
describes subagent configuration. `Stop` is classified as the orchestrator.
When response model evidence is missing, a valid persisted Gentle AI phase or
orchestrator model assignment is used with `selected` evidence. The transcript
`effort` or nested `collaboration_mode.settings.reasoning_effort` is effective
evidence only and never becomes `selected_effort`. Missing, malformed, or
unreadable transcript/state data never fails the send and remains unavailable.

| Codex `last_token_usage` field | Runtime field |
| --- | --- |
| `input_tokens` | `input_tokens` |
| `cached_input_tokens` | `cache_read_tokens` |
| `cache_write_input_tokens` | `cache_creation_tokens` |
| `output_tokens` | `output_tokens` |
| `reasoning_output_tokens` | `reasoning_tokens` |
| `total_tokens` | `total_tokens` |

No identifier, path, prompt, summary, assistant message, raw error, or rate-limit
detail enters the runtime payload. Missing counters are unavailable rather than
zero, totals are not inferred, duration is unavailable, and there is no daemon,
queue, retry, telemetry persistence, or filesystem mutation. Codex explicitly
notes that transcript format is not a stable hook interface, so this parser is a
bounded best-effort compatibility layer and may lose coverage after Codex format
changes.

**Automatic sources:** Pi, OpenCode, Claude Code, and Codex integrations provide
one-shot runtime telemetry. Schema host values remain for compatibility, not as
evidence of installed support. No installation, deployment, or external-network
validation is implied here.

## Anonymous runtime collector and retention

`POST /v1/runtime-events` keeps its existing endpoint, SQLite storage, shared
per-peer in-memory limiter, and strict schema admission. It accepts exactly
`schema`, `registry`, `delivery_id`, `host`, and `rows`. No caller identities or
activity timestamps are accepted; `received_at` is server delivery time only.

The [four Grafana runtime tables](telemetry-collector.md#runtime-received-observations)
show received response/launch occurrences, six independent token sums with coverage,
and duration by timing kind and error category. They use UTC receipt days and the
dashboard time range over retained raw observations (90 days with the existing
service), not complete consumption. Missing token values remain distinct from
reported zero; selected effort never substitutes for effective effort.

HTTP 200 acknowledges a committed `stored` or `duplicate` result:

```json
{"schema":"gentle-ai.telemetry-runtime-delivery/v1","decision":"stored"}
```

Other statuses have no error body: 400 invalid, 409 conflicting delivery identity,
413 oversized, 429 quota, 500 storage failure. The client discards all of them.
The collector's defensive deduplication remains, but the client never retries.
Public observations are not authenticated measurement or unique people/sessions.
Raw requests, delivery identities, storage errors and addresses are not logged.

SQLite `user_version` remains `1`. Existing additive admission/migration and
transactional `runtime_deliveries`/ordered `runtime_rows` storage are unchanged;
unknown versions, collisions, or incompatible schemas fail closed. Replays do
not add rows or refresh `received_at`. Legacy install events/rollups are untouched.

The existing `--retention-days` policy and startup/daily `RunMaintenance` apply.
The cutoff is UTC midnight minus the configured days; equality survives. Purge
removes children then deliveries with expired legacy events in one transaction,
with rollback on failure. Its returned count remains legacy-event count only.
Runtime-only databases still purge. Cancellation or rollup failure can defer
cleanup; existing zero/negative date arithmetic is unchanged.

Deduplication lasts only while a delivery record is retained. Expiry removes its
identity and canonical payload, with no permanent tombstone. Replaying an expired
identity can be stored again; this is not exactly-once delivery forever. Native
clients have no late-retry policy because they retain nothing and never retry.
There is no runtime daily rollup, database-size quota, automatic vacuum, new
server scheduler, or production configuration change. Purged runtime
observations have no durable daily rollup. See [collector operations](telemetry-collector.md).

## Read collection policy without side effects

Run `gentle-ai telemetry policy --json` for runtime collection permission,
not a send attempt. Omit `--json` for a concise human-readable answer.
Unlike `status --json`, which still creates missing state for a stable ID,
`policy` never creates or repairs state, generates IDs, increments counters,
enrolls an installation, or sends anything.

```json
{"schema":"gentle-ai.telemetry-policy/v1","operation":"policy","enabled":true,"source":"default","reason":"enabled"}
```

The result has exactly these five fields, with no identifiers, endpoints,
counters, paths, or raw errors. `source` uses the existing kill-switch values:
`DO_NOT_TRACK`, `GENTLE_AI_TELEMETRY`, `CI`, `state`, or `default`.
`reason` is one of `enabled`, `disabled`, `enrollment_pending`, or
`state_unavailable`. Environment opt-outs take precedence. Otherwise, missing,
unreadable, malformed, or incomplete policy state returns disabled with
`state_unavailable`; explicit `enabled` and `notice_shown` booleans and a
nonempty existing install ID are required. A notice not yet shown returns
`enrollment_pending`. Counters and send history are not required for permission.
Runtime send performs these checks internally; no policy subprocess is necessary.
Legacy install/heartbeat scheduling and development-build suppression are separate.

## Legacy install/heartbeat: what is sent

Two event kinds, each a single JSON POST under 4 KiB:

- `install` — sent once per installation, the first time `install`, `update`,
  or `sync` completes successfully. An existing installation that predates
  telemetry picks this up on its next `update` or `sync`.
- `heartbeat` — sent at most once every 24 hours, opportunistically, by
  `install`, `update`, or `sync`.

Every event carries:

- a random `install_id` (UUID v4), generated once and stored locally — never
  a machine ID, MAC address, or anything else that could be shared with
  another tool
- the `gentle-ai` version, `os`, and `arch` (the same values `--version`
  effectively describes)
- the agents and components you have installed (e.g. `claude-code`, `sdd`)
- whether receipt-driven development (RDD) is enabled
- on `heartbeat` only, counters since the previous successful send: `syncs`,
  `sdd_phase_runs`, `reviews_approved`, `reviews_correction`,
  `reviews_escalated`

Nothing else. In particular: no paths, repository names, usernames,
hostnames, prompts, diffs, source code, or IP addresses. The collector does
not store the client IP address either. Run `gentle-ai telemetry preview` at
any time to see the exact bytes that would be sent next — that command never
sends anything.

The full JSON contract lives at
[`contracts/telemetry/v1/schemas/event.schema.json`](../contracts/telemetry/v1/schemas/event.schema.json).

## When it is sent

The very first time `install`, `update`, or `sync` ever completes on a fresh
installation, gentle-ai does exactly one thing: it prints this line to
stderr, synchronously, in that same command —

```text
Gentle AI sends anonymous usage metrics (version, OS, agents, counters) and may send anonymous runtime usage from supported Pi/OpenCode/Codex integrations (public model, effort, agent class, available token usage, timing, error categories); runtime usage is never stored locally; run gentle-ai telemetry disable to opt out.
```

— and stores a locally generated `install_id`. **Nothing is sent on that
first run.** The first actual `install` event is only sent starting from the
*next* trigger: the following `install`/`update`/`sync`, or the 24-hour
heartbeat window, whichever comes first. This means if you run
`gentle-ai telemetry disable` before that next run, nothing was ever sent
about your installation.

That notice line is printed exactly once, ever, per installation — every
run after the first behaves purely as described below.

From the second trigger onward, sending is fire-and-forget: a detached
background process performs the actual network call (2 second connect
timeout, 3 second total timeout) after `install`, `update`, or `sync`
finishes. It never blocks the triggering command and never changes its exit
code or output.

A review's outcome (approved, one bounded correction, or escalated) and a
completed `sdd-attempt finish|settle` each increment their own local counter
first, and only then opportunistically check whether a heartbeat is due —
the same 24-hour limit and failure backoff apply, so this adds at most one
send per day even for a host that finishes many reviews or SDD phases in a
row. This is what lets a host such as Gentle Pi, which drives gentle-ai only
through `review ...` and `sdd-attempt ...` and never through
`install`/`update`/`sync`, still send a heartbeat.

## Opting out

Telemetry respects, in this order:

1. `DO_NOT_TRACK` set to anything but empty, `0`, or `false`
2. `GENTLE_AI_TELEMETRY=0`
3. `CI` or `GITHUB_ACTIONS` set to anything but empty, `0`, or `false` (most CI providers export one of them)

Legacy install/heartbeat builds without a release identity (`gentle-ai --version` reporting `dev` or `0.0.0-dev`, which is what a plain `go build` or a test harness produces) never send those events or write their telemetry state; the legacy collector refuses such versions too. Pseudo-versions from `go install ...@main` carry a commit stamp and count as real installs. Runtime observations carry no version and use the existing enrolled policy checks above.
4. `gentle-ai telemetry disable`

Any one of these disables sending; nothing else needs to change. Re-enable a
local opt-out with `gentle-ai telemetry enable`.

## Commands

```
gentle-ai telemetry status [--json]
gentle-ai telemetry enable
gentle-ai telemetry disable
gentle-ai telemetry preview [--json]
gentle-ai telemetry trigger [--json]
```

- `status` reports whether sending is enabled and which of the sources above
  decided that. On its very first call it persists the (otherwise stable)
  `install_id` if none exists yet; it never changes `enabled`, the notice
  history, or the counters. It also reports `last_failure_at` and, while a
  failed send is still in its 6-hour backoff window, `backoff_until`.
- `enable` / `disable` set the local opt-out persisted next to the rest of
  gentle-ai's state.
- `preview` prints the exact event that would be sent next, without sending
  it.
- `trigger` is the host entry point: it runs exactly the same opportunistic
  check `install`/`update`/`sync` already run internally (enrollment,
  install-once, the 24-hour heartbeat limit, the failure backoff, and every
  kill switch all apply). A host that only ever drives gentle-ai through
  `review ...` or `sdd-attempt ...` — Gentle Pi, for example — can call this
  once per session to still get a heartbeat instead of never sending one.
  Finishing a native review or an `sdd-attempt finish|settle` already
  triggers this internally too, so `trigger` mainly matters for a host that
  never runs any of those either. It always exits 0 and never blocks on the
  network.

## Retention

The collector retains raw events for 90 days, after which they are deleted
or aggregated. Since events carry no identifying information, there is
nothing to look up or delete on request.

## Endpoint

The default collector is `https://telemetry.gentlemanprogramming.com/v1/events`,
overridable with `GENTLE_AI_TELEMETRY_ENDPOINT` (useful for self-hosting or
testing against a local collector).
