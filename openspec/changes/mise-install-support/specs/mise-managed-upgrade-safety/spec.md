# mise-Managed Upgrade Safety Specification

## Purpose

Defines how `gentle-ai upgrade` detects a mise-managed gentle-ai binary and
skips only its own self-upgrade with an actionable manual hint, leaving
every other requested tool and every non-mise install unaffected.

## Requirements

### Requirement: Mise-Managed Binary Detection

The upgrade preflight MUST determine whether the running gentle-ai binary is
managed by mise by testing containment of `os.Executable()` within the
resolved mise installs root, using `internal/pathidentity.Contains`.

#### Scenario: Binary inside the mise installs root is detected

- GIVEN the running gentle-ai binary's path is contained within the resolved
  mise installs root
- WHEN the upgrade preflight runs
- THEN it classifies the binary as mise-managed

#### Scenario: Non-mise install is unaffected

- GIVEN the running gentle-ai binary's path is not contained within any
  resolved mise installs root, or the root does not exist
- WHEN the upgrade preflight runs
- THEN it classifies the binary as not mise-managed
- AND no skip and no behavior change occurs

### Requirement: Mise Install-Root Resolution Order

The preflight MUST resolve the mise installs root by checking, in order,
`$MISE_INSTALLS_DIR`, then `$MISE_DATA_DIR/installs`, then
`$XDG_DATA_HOME/mise/installs`, then `~/.local/share/mise/installs`, using
the first of these that yields a usable root.

#### Scenario: Higher-priority override wins

- GIVEN both `$MISE_INSTALLS_DIR` and `$MISE_DATA_DIR` are set
- WHEN the preflight resolves the installs root
- THEN it uses `$MISE_INSTALLS_DIR` and does not consult the remaining
  fallbacks

#### Scenario: Falls through to the default path

- GIVEN none of `$MISE_INSTALLS_DIR`, `$MISE_DATA_DIR`, or `$XDG_DATA_HOME`
  are set
- WHEN the preflight resolves the installs root
- THEN it uses `~/.local/share/mise/installs`

### Requirement: Self-Upgrade Skip Is Per-Tool, Not Whole-Command Abort

When the running gentle-ai binary is mise-managed, `gentle-ai upgrade` MUST
skip only gentle-ai's own upgrade and MUST still upgrade every other
requested tool in the same invocation.

#### Scenario: Mixed invocation skips only gentle-ai

- GIVEN a mise-managed gentle-ai binary and an upgrade invocation requesting
  gentle-ai plus at least one other tool
- WHEN the upgrade runs
- THEN gentle-ai is reported as skipped
- AND every other requested tool upgrades normally in the same invocation

### Requirement: Skip Hint Is Instructional

When gentle-ai's self-upgrade is skipped because the binary is mise-managed,
the reported result MUST include a manual hint that states both the reason
(gentle-ai is managed by mise) and the corrective command
(`mise upgrade gentle-ai`), not a bare command alone.

#### Scenario: Skip message states reason and command

- GIVEN gentle-ai's self-upgrade is skipped as mise-managed
- WHEN the skip result is reported to the user
- THEN the message explains that gentle-ai is managed by mise and instructs
  running `mise upgrade gentle-ai`
