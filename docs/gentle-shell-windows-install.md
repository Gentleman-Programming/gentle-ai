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
| gentle-ai TUI experience | Persona, Background subagents, Strict TDD, Global RDD, CodeGraph and Context7 are staged on Stable and on Main preview; Main preview selects the Main channel; "On my existing Pi" is shown disabled with the Separate-only reason; `selection.json` never retains the profile |
| Tooling | Private pinned Node 24.18.0 and Go 1.27.1 ZIPs |
| Composition | Stock Pi 1.0.0 and gentle-pi 4.0.0, complete SHA512 source lock, no implicit lifecycle scripts |
| Native | Explicit authenticated stock Go SumDB source-build API for native 4.0.0 |
| Ownership | Current SID, private DACLs, NTFS volume/file IDs, no reparse/drive aliases or selected hard links |
| Publication | Owned sibling stage, confirmation readback, no replace-existing rename |
| Processes | Suspended child bound before resume; Job Object CPU/memory/process readback, inherited console, bounded cleanup |
| Preservation | Isolated HOME/agent/config, caller CWD inherited; console modes saved/restored; personal PATH unchanged |

## Persistent memory (Engram)

Separate provisions Engram from the gentle-pi it installs, never from a
second version list. The `gentle-engram` companion joins the verified npm
composition at that gentle-pi's exact pin and is declared in the private
agent's `settings.json` exactly as bundled Gentle Shell declares it
(`npm:gentle-engram@<version>`). The SHA-256-pinned Engram core is published
to `AGENT\bin\engram.exe`. Memory is shared like the bundled install: a
launch reuses a verified Engram server on `127.0.0.1:7437`, or starts one on
your real `~/.engram` store, so every agent sees the same memory. If a piece
is missing or cannot be verified, the install still succeeds and Shell runs
without memory. On Windows both channels provision this stock composition
before any Main overlay, so Stable and Main preview installs both get Engram
memory. Windows installs are Separate only, so there is no Shared or
"On my existing Pi" variant to receive Engram.

### How each launch decides memory

A Separate installation shares your global Engram memory, the same way the bundled Gentle Shell and your other agents do. Each launch decides memory once, in the entry process before the owned Job Object starts, and never fails the launch because memory is unavailable:

- **Version and help runs never probe or start a server**: a launch with `--version`, `-v`, `--help` or `-h` runs with `ENGRAM_STARTUP_POLICY=disabled` and no notice, as in the bundled Gentle Shell.
- **You already chose Engram settings.** If your environment sets `ENGRAM_BIN`, `ENGRAM_URL`, `ENGRAM_DATA_DIR`, `ENGRAM_PORT`, `ENGRAM_SOCKET`, `ENGRAM_DATABASE_URL` or `ENGRAM_SERVER_MODE` (or an `ENGRAM_STARTUP_POLICY` other than `allow`), those exact values are passed to the session unchanged, nothing is probed or started, and memory runs disabled for that launch, as in the bundled Gentle Shell. An explicit `ENGRAM_BIN` or `ENGRAM_URL` is never replaced; without your own `ENGRAM_BIN`, the session gets the installation's verified core instead of any `engram` on `PATH`.
- **A verified server is running.** A server on `127.0.0.1:7437` whose `/compatibility` answer proves a startup-verified store is reused through `ENGRAM_URL=http://127.0.0.1:7437`.
- **Nothing is listening.** The installation's verified Engram core (`AGENT\bin\engram.exe`) starts `engram serve` detached in its own process group, outside the Job Object, with your real home, so the store is your usual `%USERPROFILE%\.engram`. This store is the only thing a Separate installation writes outside its own folder; the server keeps running after the session ends, like any shared Engram server.
- **Anything else disables memory for that launch.** A listener that does not prove compatibility, a store migrated by a newer Engram than the installation's core, a missing core, or a server that does not report ready within about 10 seconds starts nothing. The session starts with `ENGRAM_STARTUP_POLICY=disabled` and a one-line notice explaining why. Gentle Shell never stops or replaces a server already listening on `127.0.0.1:7437`, and rechecks on every launch.

Shared and agent installations keep their existing memory setup; this decision applies to Separate only. Apart from your own Engram selections, your `PATH`, tokens and other settings are never passed to the session or the server.

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
