# CI benchmark parallelism

The required **Unit Tests** status aggregates Go formatting, root unit tests and benchmark evidence. Root tests and benchmark checks start independently; the aggregate fails unless all three jobs succeed, including when a prerequisite is skipped or cancelled.

The benchmark job builds its binaries from the exact event candidate and retains the module build, vet, tests and journey evidence assertions. No artifact from another run is used. Other required platform and distribution jobs are unchanged.

This change targets the critical path, not coverage or cache policy. Its actual impact and hosted-run behavior require exact-commit CI comparisons; local checks alone do not prove a speedup or delivery readiness.
