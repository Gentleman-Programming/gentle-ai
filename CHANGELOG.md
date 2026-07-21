# Changelog

All notable changes to gentle-ai are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and the project adheres to
[Semantic Versioning](https://semver.org/).

## Unreleased

### Added

- `gentle-ai review inspect-authority --cwd <repo>` (#1582): read-only
  enumeration of every compact-v2 recovery edge. The walker classifies each
  edge through the existing `validateCompactRecoveryEdge` source of truth,
  emits invalid edges as JSON (sorted by successor lineage and revision) and
  surfaces load errors as diagnostics, never mutating authority or relaxing
  the `compact_reconcile.go:221` strict gate. The output JSON keys are
  `summary`, `edges`, and `diagnostics` (in that order); `anomaly_class`
  strings are `unchanged_target`, `malformed_recovery_authorization`, and the
  joined `unchanged_target,malformed_recovery_authorization` for edges that
  fail both sentinels. This prerequisite surface unblocks the batch
  reconciliation plan tracked under #1452.