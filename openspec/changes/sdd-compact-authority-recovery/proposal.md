# Proposal: Recover SDD Verification from Compact Authority

## Intent

Allow `sdd-status` to discover exactly one approved, path-bound compact review authority before final verification and to retry a historical verification whose sole failure was missing authority. Preserve fail-closed routing, history, and archive protection.

## Scope

### In Scope
- Discover compact stores from the repository Git common-dir after apply.
- Bind authority to canonical paths under the active OpenSpec change.
- Require approved state, equal receipt, stable revision, and live post-apply `allow`.
- Handle foreign, stale, invalid, denied, and multiple lineages deterministically.
- Route authority-only historical failures to re-verification only when authority changed.
- Define the `sdd-verify` authority-preflight denial envelope and exit-125 contract.

### Out of Scope
- Pre-PR publication-boundary selection or authorization.
- Compact/legacy gate implementation changes.
- Mirror synthesis, authority repair, substantive-failure retry, or archive bypass.

## Delivery

The focused status fixtures exceed 400 lines. Use a single-PR `size:exception` with an 800 authored-line ceiling.

## Success Criteria

- [ ] Exactly one valid active-change authority makes only verify ready.
- [ ] Multi-lineage history can ignore only the exact observed stale denied predecessor.
- [ ] Invalid, ambiguous, unchanged, or substantive cases remain blocked.
- [ ] Historical reports remain immutable and archive requires a new passing verification.
- [ ] Embedded `sdd-verify` assets emit a strict machine-readable authority-only denial.
