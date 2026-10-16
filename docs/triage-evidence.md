# Triage evidence workflow

A manually triggered, **read-only** GitHub Actions workflow that produces a bounded
evidence report over the open `status:needs-review` backlog (plus open issues that
carry no lifecycle label at all). It exists to give maintainers a repeatable way to
answer one question per issue: *what evidence exists, and what should happen next?*

It never closes, labels, comments, edits, or creates anything. Outcomes are report
data, not decisions. Closing or labeling an issue remains a separate human action
governed by `internal/assets/skills/issue-creation/references/delegated-workflow-actions.md`.

## Outcome semantics

| Outcome | Meaning for the maintainer |
| --- | --- |
| `related-change-found` | A released change or in-flight PR already touches this report's area. Evaluate the cited change before re-testing the report. |
| `retest-requested` | The report names a version older than the current release (or says an upgrade changed behavior). Retest on current to gather evidence. An old version is never evidence of a current defect. |
| `current-evidence` | The report names the current release or an unreleased `main` build **and** carries reproduction evidence. A live candidate, not a proof. |
| `insufficient-evidence` | The report could not meet the evidence threshold. The rationale says exactly what is missing. Absence of evidence is never treated as absence of a bug. |

An old reported version, issue age, or "upgrading fixed it" is never enough, alone,
to close an issue or assign responsibility.

## How to trigger

1. Repository → **Actions** → **Triage Evidence** → **Run workflow**.
2. Optional inputs:
   - `repo` — default `Gentleman-Programming/gentle-ai`.
   - `max-issues` — issues kept in the report (default 40).
   - `max-searches` — total related-search API calls across the run (default 30).
   - `generated-at` — pinned ISO timestamp; pass the same value to reproduce a
     byte-identical report for unchanged evidence.
3. Read the job summary and download the `triage-evidence` artifact.

## What it reads

- Open issues labeled `status:needs-review` (first), then open issues with no
  lifecycle label (`status:approved` / `status:needs-review` / `status:needs-design` /
  `status:needs-info`).
- The release list (newest stable tag = "current"; prereleases are never "current").
- One bounded `search/issues` query per issue, with up to `max-searches` queries
  total. Release matching reuses the already-fetched release list (no extra API
  calls).

## Safety contract

- Permissions: `contents: read`, `issues: read`. Nothing else, on every job.
- No LLM calls, no repository secrets (uses `github.token` scoped to the run).
- All counters and page sizes are bounded; the renderer hard-caps the artifact.
- Issue content and related metadata are HTML-escaped before rendering; only
  `http(s)` URLs become links; issue content is never interpolated into shell
  commands, paths, or environment variables.
- Deterministic for unchanged evidence: classification is pure Go, items are
  sorted by issue number, and `generated-at` pins the timestamp.
- API failures are reported inside the artifact as unavailable evidence, never as
  a crash or a partial row.
- `gentle-report` (consent-based reporting of newly detected internal defects) is
  untouched; this workflow only assists with the existing backlog.

## Running locally

```sh
GITHUB_TOKEN="$(gh auth token)" go run ./cmd/gentle-triage \
  --repo Gentleman-Programming/gentle-ai \
  --max-issues 40 --max-searches 30 \
  --generated-at 2026-09-19T00:00:00Z \
  --output triage-evidence.md
```

Budget defaults: 40 issues, 5 related candidates per issue, 10 releases considered,
30 search calls, 10 pages per list, 100 per page. All overridable via flags shown by
`go run ./cmd/gentle-triage -h`.

## Files

- `.github/workflows/triage-evidence.yml` — the workflow (manual dispatch).
- `cmd/gentle-triage/` — the binary (REST wiring, bounds, artifact/summary output).
- `internal/triageevidence/` — pure classification: version/channel parsing,
  evidence extraction, outcome classification, keyword derivation, report renderer.