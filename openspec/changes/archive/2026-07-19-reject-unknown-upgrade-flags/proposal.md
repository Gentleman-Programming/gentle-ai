# Proposal: Reject Unknown Upgrade Flags

## Intent

Resolve approved issue #535: `gentle-ai upgrade` must fail clearly instead of silently ignoring unsupported flags and continuing toward upgrade work.

## Scope

### In Scope
- Reject unsupported dash-prefixed `upgrade` arguments, including `--bad-flag` and `-x`.
- Preserve `--dry-run`/`-n`, `--no-backup`, and `--` as the end-of-flags delimiter.
- Prove invalid flags return an error before update checks, spinner output, or upgrade execution.

### Out of Scope
- Changing positional tool-filter behavior; `upgrade typo` remains unchanged.
- Validating unknown tool names or changing `update.CheckFiltered`.
- Replacing the current parser with `flag.FlagSet`.

## Capabilities

### New Capabilities
- `upgrade-argument-validation`: Defines accepted upgrade flags, `--` delimiter behavior, and early rejection of unsupported flags.

### Modified Capabilities
- None.

## Approach

Validate arguments in `runUpgrade` before home-directory resolution and all update/upgrade work. Keep recognized flags, switch `--` into positional-filter mode, and return a stable error for every other dash-prefixed token. Add focused table-driven tests through existing injected seams.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/app/app.go` | Modified | Parse and reject unsupported upgrade flags early. |
| `internal/app/update_test.go` | Modified | Assert errors and zero side effects for invalid flags. |
| `internal/app/upgrade_test.go` | Modified | Cover CLI-level error and preserved positional behavior if needed. |
| `internal/update/check.go` | Unchanged | Continues to ignore unknown positional filters. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Parsing alters existing invocation semantics | Low | Cover recognized flags, delimiter, and positional filters. |
| Validation occurs after a side effect | Low | Test that checks and execution seams are never called. |

## Rollback Plan

Revert the isolated parser and its tests, restoring prior flag handling without changing update-tool filtering.

## Dependencies

- Approved GitHub issue #535.
- Strict TDD; planned delivery must use the ask-always PR strategy and remain within the 400 changed-line review budget unless explicitly re-decided.

## Success Criteria

- [ ] `upgrade --bad-flag` and `upgrade -x` return a clear non-zero error before upgrade work.
- [ ] `upgrade -- <tool>` treats `<tool>` as a positional filter; existing positional behavior remains unchanged.
- [ ] Focused tests demonstrate no update check, spinner output, or execution for invalid flags.
