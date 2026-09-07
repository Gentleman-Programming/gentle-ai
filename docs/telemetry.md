# Telemetry

Gentle AI sends a small amount of anonymous usage telemetry so the project
knows how many installs stay alive and how the review pipeline gets used,
without collecting anything about you, your code, your machine, or your
organization.

## What is sent

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

```
Gentle AI sends anonymous usage metrics (version, OS, agents, counters); run gentle-ai telemetry disable to opt out.
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
3. `CI=true` (most CI providers set this already)
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
