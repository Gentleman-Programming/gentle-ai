# Community Roadmap

Where to find work that is ready to be picked up, and what "ready" means here.

## Start here

**[→ Every issue currently up for grabs](https://github.com/Gentleman-Programming/gentle-ai/issues?q=is%3Aissue+is%3Aopen+label%3Aup-for-grabs)**

That link is the roadmap. It is a live query, not a list maintained by hand, so
it can never go stale the way a copied table does.

## What `up-for-grabs` guarantees

An issue carries this label only when all three are true:

1. **It is scoped.** The problem is stated, the failure is reproducible or the
   behaviour is specified, and the issue names where in the tree to look.
2. **It is approved.** It also carries `status:approved`, which is what lets a
   PR be opened at all — see [CONTRIBUTING.md](../CONTRIBUTING.md). Without it
   your PR is rejected before review, no matter how good the code is.
3. **Nobody is on it.** If a maintainer or contributor picks it up, the label
   comes off, so you are not racing someone silently.

An issue without the label is not closed to you — it usually means it is still
missing information (`status:needs-info`) or needs an architectural decision
before code makes sense (`status:needs-design`). Comment on those instead of
opening a PR; the decision has to land first or the work gets thrown away.

## Picking one up

1. Comment that you are taking it. That is all the claim needed.
2. Read the issue's "where it comes from" section if it has one — most name the
   exact file and line, and several already contain the diagnosis.
3. Open a PR that links the issue. CI checks the link and the
   `status:approved` label automatically.

## Reading the labels

| Label | Meaning |
| --- | --- |
| `up-for-grabs` | Ready for you. Scoped, approved, unclaimed. |
| `help wanted` | The maintainers would especially like outside help here. Often the larger ones. |
| `status:approved` | A PR may be opened. Required. |
| `status:needs-design` | Valid, but an architectural decision comes first. Discuss, do not implement. |
| `status:needs-info` | Waiting on the reporter. |
| `priority:high` / `medium` / `low` | Maintainer ordering, not difficulty. |
| `type:bug` / `type:feature` / `type:docs` / … | What kind of change it is. |

`priority:high` does **not** mean hard, and `priority:low` does not mean easy.
Some of the highest-priority items are single-line fixes with a long
explanation; some low-priority ones touch a lot of surface.

## If you have a Windows machine

[#1983](https://github.com/Gentleman-Programming/gentle-ai/issues/1983) is the
most valuable thing an outside contributor can do right now, and it is hard for
the maintainers to do well because it needs a real Windows box.

The Windows test lane never completed a single run until 2026-07-29 — its job
timeout was shorter than one of its own steps, so every push was cancelled
partway. Fixing that revealed nineteen genuine failures that had been invisible
for months. They are grouped by cause in the issue, and **each group is
independent**: you can fix one and open a PR without touching the others.

Until they are green the lane runs outside `ci.yml`, so it reports without
gating releases. When they are green it moves back and gates again.

## If you want something smaller

Sort the query by
[`type:docs`](https://github.com/Gentleman-Programming/gentle-ai/issues?q=is%3Aissue+is%3Aopen+label%3Aup-for-grabs+label%3Atype%3Adocs)
or look for issues whose body already contains the diagnosis — several name the
exact function and the reason the current code is wrong, which turns the work
into applying a decision rather than making one.

## Where the design lives

Before changing review, delivery or SDD behaviour, read:

- [Organic RDD architecture](architecture/organic-rdd.md) — how a candidate is
  reviewed as evidence while ordinary repository policy controls delivery
- [Review authority threat model](review-authority-threat-model.md) — what the
  authority store defends against, and what it deliberately does not
- [Organic implementation routing](trigger-rules.md) — how work is routed
  before review ever runs

RDD is **Receipt-Driven Development**. Its review evidence never authorizes
commit, push, PR, release, or archive; reviewing is separate from ordinary
repository delivery policy.
