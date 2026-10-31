# CI resilience without weaker checks

CI keeps its original job topology, required names, unit/dead-code checks and complete benchmark evidence. No native, race, fresh-install or benchmark assertions are removed.

## Avoid obsolete work and bound stalled setup

Only superseded `pull_request` runs cancel. The group includes workflow, event and PR number; non-PR events use unique run IDs, so main, scheduled and manual runs do not replace each other.

Cache segments have a two-minute download limit. Every Go setup step has a four-minute fail-closed limit. A cache miss never skips tests, and there is no failure tolerance. These bounds limit failure feedback; they do not prove that the upstream egress/cache stall is fixed or that successful CI is faster. Cache identity, publication policy, pins and permission scopes stay unchanged.

## Measured experiment, not delivered

An isolated benchmark-job trial passed all checks but did not establish a successful-CI speedup: fresh baseline 600 seconds; parallel candidates 581 and 617 seconds (mean 599). Root-test variability was substantial. The added jobs and aggregate were therefore removed before merge; all original benchmark commands and assertions remain in `Unit Tests`.

Observed cancellation, independent checks and exact-head hosted CI are recorded in the pull request. Cancelled or failed runs are not successful timing samples. Future acceleration needs representative measurements, not a shorter aggregate-only job.
