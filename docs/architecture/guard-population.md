# Guard population declarations

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

Guard population declarations make the accepted input set of selected production guards explicit at the check itself. They cover guards in `internal/cli` and `internal/reviewtransaction`.

## Review rule

A production Go guard qualifies when both of these are true:

1. It is in `internal/cli` or `internal/reviewtransaction`.
2. It decides whether external, repository, filesystem, or persisted state is legitimate for a security, integrity, admission, repair, or governance boundary.

Arbitrary control flow does not qualify. Shell and workflow guards are out of scope.

Families are discovered from the declarations in source, not from a fixed list. The table below illustrates the families declared today; it is not a closed or complete set:

| Family | Legitimate population under review |
|---|---|
| `shared-rar-owner` | Owners allowed to host shared Windows review authority |
| `darwin-search-ancestor` | Directory permission shapes allowed during secure ancestry traversal |
| `nested-worktree-scope` | Opaque nested repositories excluded from or admitted to review scope |
| `authority-repair-removal` | Damaged authority graphs on which one repair may proceed |
| `persisted-sync-state-integrity` | Persisted sync state admitted before persona mutation |

Reviewers own identification of a new or omitted qualifying guard. The mechanism cannot derive the real-world population from source and MUST NOT be described as semantic completeness.

## Declaration contract

Place one declaration immediately above the `if`, `switch`, or `return` node that enforces the population boundary:

```go
// guard:population <family> <too-tight|too-loose|fail-closed>: <legitimate population and exclusion boundary>
```

Use `too-tight` when drift is expected to reject legitimate inputs, `too-loose` when it may admit illegitimate inputs, and `fail-closed` when the important contract is safe refusal under uncertainty.

`TestEveryGuardPopulationDeclarationIsAdjacentAndUnique` scans both packages and fails when a declaration is malformed, is not directly above an `if`, `switch`, or `return` node, reuses a family already declared elsewhere, or when the scan finds no declarations at all. There is no registry to regenerate: adding, moving, or rewording a declaration needs only a well-formed, adjacent marker.

## Proof boundary

The scan proves declaration syntax, AST adjacency, and family uniqueness. It does not prove that the population claim is true, that every qualifying guard was identified, or that tests sample the outside world. Review must challenge each claim against the named behavior tests that exercise the guard, and against production platforms, repository shapes, persisted states, and incident evidence.
