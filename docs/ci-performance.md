# CI benchmark parallelism

The required **Unit Tests** status aggregates Go formatting, root unit tests and benchmark evidence. Root tests and benchmark checks start independently; the aggregate fails unless all three jobs succeed, including when a prerequisite is skipped or cancelled.

The benchmark job builds its binaries from the exact event candidate and retains the module build, vet, tests and journey evidence assertions. No artifact from another run is used. Other required platform and distribution jobs are unchanged.

The benchmark change targets the critical path without changing coverage or cache keys/trust. Its actual impact and hosted-run behavior require exact-commit CI comparisons; local checks alone do not prove a speedup or delivery readiness.

## Superseded PRs and bounded setup

Only a newer run of the same pull request cancels its predecessor. Push, schedule and manual runs have unique concurrency groups and are not cancelled by this policy.

Cache segments use the supported two-minute download timeout. Each Go setup step has a four-minute hard limit and does not tolerate failure: unavailable setup cannot turn unexecuted tests green. These limits bound failure feedback; they neither diagnose the previously unexplained silent interval nor count a failed run as a performance improvement.
