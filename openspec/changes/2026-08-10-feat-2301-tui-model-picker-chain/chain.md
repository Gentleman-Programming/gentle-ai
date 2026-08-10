# #2301 — TUI Model Picker Adaptive + Role Guidance

This change ships the TUI model picker adaptive viewport and the
contextual role guidance feature in three reviewable slices, each
under the 400-line cognitive-load budget.

## Strategy

Feature-branch-chain. Each slice lands as its own PR; the tracker
branch (`feat/2301-tui-model-picker`) only merges after every child
PR has landed.

## Slice order

| # | Branch | Base | Scope | Lines | PR |
|---|--------|------|-------|-------|----|
| 1 | `feat/2301-01-adaptive-viewport` | `feat/2301-tui-model-picker` | viewport-aware phase list + sub list budgets, resize clamps | ~398 | linked |
| 2 | `feat/2301-02-guidance-framework` | `feat/2301-01-adaptive-viewport` | guidance struct, resolver, renderers, toggle; empty map | ~298 | linked |
| 3 | `feat/2301-03-guidance-entries` | `feat/2301-02-guidance-framework` | 19 per-role entries + 3 entry-specific tests | ~197 | linked |

## Why a chain

- The original PR #2890 was 891 changed lines and tripped the
  `Check PR Cognitive Load` maintainer gate (>400).
- Splitting along slice boundaries keeps each PR reviewable in
  about 30 minutes.
- The 19-entry map was the size driver; pulling it out to slice 3
  lets reviewers focus on the framework contract first.

## Issue closure

`#2301` closes when this tracker merges — i.e. once all three slices
land in order and the tracker fast-forwards to their combined tip.

## Rollback

Each slice can be reverted independently. The tracker can be deleted
without affecting already-merged slices.
