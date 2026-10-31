# Gentle Shell Stable: the reviewed-list trust boundary

Stable is the channel that installs gentle-pi releases reviewed by this
repository. This page states exactly which authority admits executable
supplier code on Stable, what an attacker would need to abuse it, what the
design buys, and why that trade-off is acceptable for Stable on its own terms
— independent of how Main preview treats trust.

## Which authority admits supplier code

The single authority is **the reviewed supplier list**,
`scripts/gentle-pi-suppliers.json` in the gentle-ai repository:

```json
{"schema": "gentle-shell-reviewed-suppliers/v1", "releases": {"<x.y.z>": {
  "integrity": "sha512-…", "installer": "<sha256>", "binary": "<sha256>" }}}
```

Each entry binds one exact gentle-pi release to:

- the npm tarball SRI of that release (what `npm ci` must resolve), and
- the SHA-256 of the two native supplier files inside the package
  (`scripts/gentle-ai-installer.mjs` and `runtime/gentle-ai-binary.mjs`).

The installer reads the list live from the repository `main` branch at check
and update time (bounded fetch, strict validation) and falls back to the copy
embedded in the gentle-ai binary when the fetch or validation fails. The
selected list is written into the install stage — and published inside the
installation — so the provisioner and the Go supplier check verify exactly the
list the check used. Every per-package
canonical-URL/SRI gate still applies to the full resolved closure; on top of
that, gentle-pi's locked SRI and both supplier digests must equal the list
entry before any supplier file runs. A listed version with a mismatched SRI or
supplier bytes is refused, never bypassed.

On every later verify — including the launch-time `verify` of an already
installed tree — the provisioner re-reads the list from the installation's own
staged copy at its root. That copy was written by the installer into the
private stage and published with the installation, so `verify` trusts the
installation's own directory: the same-user boundary. Anyone who can rewrite
that staged list can already rewrite every other file in the installation, so
it adds no weaker link. Installations recorded before the staged list existed,
and Main preview installs (which never take the reviewed-list route), keep
their supplier authentication on the embedded copy instead: the Go launch
boundary checks both supplier files against the embedded list entry for the
installed gentle-pi version and refuses a mismatch, exactly as the frozen
provisioner did before the list.

Reviewing a release is a one-line change to that JSON. It becomes the install
target for every user on their next update check, without a new gentle-ai
binary.

## What an attacker would need

To run malicious supplier code through Stable, an attacker would need **both**:

1. **write access to repository `main`** — to add or change a list entry, and
2. **a published npm tarball matching that entry** — the entry's integrity and
   supplier digests must match the artifacts npm actually serves.

Compromising only one side is not enough: a tarball without a list entry is
never resolved (gentle-pi is pinned to a listed version), and a list entry
whose digests do not match the published bytes fails the install closed.

## What Stable gains

Without the list, every reviewed gentle-pi release required a new gentle-ai
binary (the release was pinned inside the frozen lock the binary embedded).
With the list:

- a gentle-pi release ships to all three operating systems by merging a
  one-line JSON change, and
- an unreviewed or broken release can never break an install or update — the
  newest *listed* release is installed instead.

## Why this is acceptable for Stable, on its own terms

Stable's trust anchor is the same repository `main` that already publishes
every gentle-ai binary users download. Anyone able to write to `main` can ship
a new gentle-ai build directly, so the list adds no new trust root; it only
narrows what a `main` write can do (it must also control the matching npm
artifacts). The reviewer-visible diff is one JSON line naming an exact version
and its digests, which is easier to audit than a binary release.

No signing mechanism is used. Main preview deliberately trusts a live `main`
snapshot for its entire SDK; that Main does so does not by itself justify the
Stable design — the justification above (no new trust root, exact-version
binding, fail-closed mismatches) stands for Stable independently.

## Where the gates live

| Gate | Where |
| --- | --- |
| List schema/digest validation, live read, embedded fallback | `internal/shellinstaller/user_reviewed_suppliers.go` |
| Stage write (provisioner and Go check use the same list) | `internal/shellinstaller/user_reviewed_stage_unix.go`, `user_install_windows.go` |
| Lock generation, SRI completion, supplier pin enforcement | `scripts/provision-gentle-shell-private-global.mjs`, `scripts/provision-gentle-shell-windows.mjs`, `scripts/complete-generated-lock-sri.mjs` |
| Update detection against the newest listed release | `internal/shellinstaller/user_shell_update_unix.go`, `user_shell_registry_windows.go` |
| Launch-time `verify` of a tree without a staged list (pre-list installs, Main preview) against the embedded entry | `internal/shellinstaller/user_reviewed_stage_unix.go` (`userVerifyRecordedSuppliers`) |
