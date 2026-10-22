# Gentle Shell from the Windows installer TUI

**Implementation candidate, not yet qualified for Windows users.** This change
ports the existing Separate installer TUI to Windows 11 x64. Actual Windows
installation, both UI openings and terminal restoration remain unverified.
Cross-compilation is not a substitute for those checks.

## Candidate path

From a build of this feature, in a normal **non-Administrator** Windows terminal:

```powershell
gentle-ai shell install
```

Choose a private local NTFS destination, review its physical selection, and
confirm installation. After a successful installation, the owned bindings are
`TARGET\bin\gentle-shell.cmd` and `TARGET\bin\pi.cmd`. No personal PATH,
shell profile, existing Pi or personal configuration is overwritten.
These commands are candidate instructions, not a claim that stock native 4.0.0
already contains this unreleased installer.

## Review boundaries

| Boundary | Implementation |
|---|---|
| Scope | Windows 11 x64, Separate only; Server and ARM64 refused |
| Tooling | Private pinned Node 24.18.0 and Go 1.27.1 ZIPs |
| Composition | Stock Pi 1.0.0 and gentle-pi 4.0.0, complete SHA512 source lock, no implicit lifecycle scripts |
| Native | Explicit authenticated stock Go SumDB source-build API for native 4.0.0 |
| Ownership | Current SID, private DACLs, NTFS volume/file IDs, no reparse/drive aliases or selected hard links |
| Publication | Owned sibling stage, confirmation readback, no replace-existing rename |
| Processes | Suspended child bound before resume; Job Object CPU/memory/process readback, inherited console, bounded cleanup |
| Preservation | Isolated HOME/agent/config, caller CWD inherited; console modes saved/restored; personal PATH unchanged |

## Launch custody: artefacts versus runtime state

Every launch re-walks the **installed artefacts** with the complete per-file
owner, DACL, reparse, NTFS, hard-link and selection-name checks and the bounds
below, and compares supervisor, runtimes, helpers and source bytes with their
retained authorities. There is no size/mtime shortcut.

**Runtime state** written after publication by the owned tools is not an
artefact: `home` (GOPATH/GOMODCACHE, set explicitly), `state` (GOCACHE),
`tmp`, `runtime\cache` (npm) and `agent\sessions` (Pi names sessions after the
caller CWD). Launch walks them recursively with the same per-entry owner,
DACL, reparse, hard-link, regular-file and resource checks as artefacts; the
file-count, per-file (32 MiB) and aggregate (2 GiB) bounds include them. The
only difference is naming: names strictly **below** these fixed roots are data
and may contain `!`, `%`, `&` or `^` (Go's escaped module paths, CWD-named
sessions). The roots, their ancestors, artefacts and bindings keep the strict
names. Large real caches beyond those bounds are not yet qualified and fail
closed.

`agent\settings.json` keeps exact `packages`/`npmCommand` and the codemode
exclusion. Following the pinned Pi 1.0.0 `SettingsManager`, it also accepts
the UI-written typed scalars `lastChangelogVersion`, `defaultProvider`,
`defaultModel`, `defaultThinkingLevel` and a path-free `theme` name. Every
other key (resources, skills, prompts, themes, shell paths, editors, session
redirection, proxies, trust and telemetry) is still refused, including other
`/settings` toggles; those remain an unqualified usability limit.

Cancellation of `shell install` signals a session-local event first. The
worker cancels its own context, terminates the remaining members of its own
Job Object (only after reading back its exact supervisor limits), and removes
its stage only when the stage identity and complete alias-free inventory
still match. After 120 seconds the supervisor kills the job; any uncertain or
remaining stage is preserved and reported, never removed by pattern.

Job committed-memory limits do not claim to disable Windows' pagefile.
Ownership controls assume cooperative same-account actors; they are not
hostile same-SID custody, loaded-byte attestation or escaped-descendant immunity.

Bootstrap bounds: Node ZIP 64 MiB, Go ZIP 128 MiB, tools 32 MiB; authenticated
bootstrap executables up to 128 MiB; complete Windows inventory up to 2 GiB.
The old Linux controls and frozen Linux work are unchanged.

## Acceptance still required

- [ ] Original exact-head formatting, builds and applicable tests pass.
- [ ] Credentialless, resource-bounded **actual Windows 11 x64** laboratory is independently established.
- [ ] A user installs through the existing TUI, not a fake installer or helper.
- [ ] Both owned bindings open authentic UIs and settle their exits.
- [ ] A launch after real Go module/build cache use, a model/theme change and a `!` CWD session opens again.
- [ ] A canceled installation (timeout and Ctrl-C) leaves no owned stage and no foreign deletion.
- [ ] Caller CWD/console/foreground and personal configuration/project preimages are preserved.
- [ ] Required target checks and exact-head Windows smoke pass before ready-for-review.

`shell-windows-crosscheck.yml` runs formatting, portable TUI tests, JavaScript
syntax and Windows cross-compilation in a bounded Linux Guest. It explicitly
does **not** run Windows, establish a Windows laboratory or qualify this feature.
After successful checks it exports the exact-head Windows product and test
executables with a source-commit record and SHA256 manifest. These are inert
qualification artifacts, not a signed release: extract and execute them only
inside the qualified isolated Windows Guest, never on the operator's system.
Shared, update, force/recovery, Darwin, registration and the full Ready contract
are outside this minimum change.

## Integration and rollback

The feature starts from Main, not the unmerged Linux PR #5242. It reuses that
installer's TUI/request vocabulary without importing the large Linux backend.
When Linux is integrated by a separately authorized maintainer, reconcile the
shared portable route and unsupported-platform build tag; do not replace a
working Linux backend with this change's unsupported-platform stub.

Rollback removes the early `shell` app route and the new Windows installer,
helper, tests, documentation and static-check workflow together. It does not
remove a user's existing Pi, project, personal configuration or installed data.
Delivery leaf: #5271 under canonical cross-platform tracker #4935. No Main
merge, release or issue closure is authorized by this candidate.
