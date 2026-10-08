# Gentle Shell on Linux

**Source candidate only, not a release guide.** Use an authorized, pinned Linux amd64 build in a bounded Guest. It targets stock Pi 1.0.0 and Gentle/native 4.0.0 with independently pinned modern roots and Node 24.18.0/npm. This is not runtime proof or release readiness; see [qualification evidence and limits](evidence/shell-linux-install-qualification.md).

Other platforms: [macOS (Separate only)](gentle-shell-macos-install.md) and [Windows](gentle-shell-windows-install.md).

## Requirements

- Non-root Linux amd64; an already-qualified execution boundary or an existing delegated **systemd user manager >=254**. No sudo, system-manager, new-delegation or container fallback.
- Physical execution checks: cgroup2, capabilities **0**, NoNewPrivs **1**, memory **3 GiB**, swap **0**, CPU **1**, tasks **64**. Missing prerequisites refuse before package JavaScript runs.
- The manager route requires a physically trusted stock `/usr/bin/setpriv` (util-linux). It clears inherited/ambient capabilities and sets NoNewPrivs before the supervisor starts; the supervisor still verifies all four active capability sets, UID and exact cgroup limits. The manager and its bounding set are not reconfigured; this is not full Guest qualification.
- Selected roots and target parent: owned, private and on one filesystem. Use absolute paths containing only ASCII letters, digits, `/`, `_`, `.` and `-`, including parents; aliases/collisions refuse. TARGET, selected prefix and agent must be mutually disjoint (none contains another).
- Supervisor executable and ancestors: current-user or root owned, not group/other writable; only root-owned sticky `/tmp` is excepted. Refusals name the unsafe ancestor. Use a qualifying location, not permission changes to an unrelated shared prefix.

## Install

| Mode | Selection and effects |
| --- | --- |
| Separate | New private prefix, runtime, HOME, agent and state; existing personal Pi is untouched. |
| Shared | Explicit existing owned global Pi prefix and agent; both new bindings use those same objects. Review settings and agent tool changes below. |

Run `gentle-ai shell install`. Arrows select mode; Tab cycles mode-appropriate fields (Shared adds prefix/agent). Enter reviews; `y` confirms and closes the TUI before terminal handoff. Escape aborts before installation; Ctrl-C during installation requests cancellation and waits for stop/reap.

The confirmation review wraps to terminal width. Use PgUp/PgDn to scroll and Home/End to reach the first/last page, including the full Shared settings preview and recovery warnings; scrolling or resizing does not confirm or change your selection.

For command-line installation, inspect, then use the **exact fresh printed confirmation** for that unchanged selection:

```sh
gentle-ai shell install --target /owned/private-parent/shell --mode separate --inspect
gentle-ai shell install --target /owned/private-parent/shell --mode separate --confirm PRINTED_SHA256
```

For Shared, use `--mode shared` and supply `--prefix /owned/selected-prefix --agent /owned/selected-agent` on **both** calls. Do not reuse consent after changing paths or selected contents.

Launch `TARGET/bin/gentle-shell` or `TARGET/bin/pi`. Normal launches have no installer menu; `TARGET/bin/gentle-shell install` reopens it. The installer does not modify PATH or shell files, replace an unrelated `pi`, or require a root installation.

## Shared settings: preview before confirming

Inspect prints the selected agent's `settings.json` path and **only** its `packages` and `npmCommand` before/after values, including actual physical paths:

| Key | Before | After |
| --- | --- | --- |
| `packages` | Existing array, or absent | Existing entries plus `PREFIX/lib/node_modules/gentle-pi` |
| `npmCommand` | Absent | `["TARGET/runtime/node/bin/node", "TARGET/runtime/node/lib/node_modules/npm/bin/npm-cli.js", "--prefix", "PREFIX"]` |

`TARGET` and `PREFIX` above stand for your selected absolute paths, not literal settings values. The command uses pinned Node/npm. Other settings keys and existing package entries are preserved; foreign npm overrides or Gentle declarations refuse. Preview is disclosure, not authority or a backup; confirm only the exact fresh inspected selection.

Shared also writes **two new files** into the selected agent: `AGENT/bin/fd` (fd 10.5.0) and `AGENT/bin/rg` (ripgrep 15.2.0), which stock Pi prefers over PATH. Personal tools are never replaced: an existing `fd` or `rg` (including links), or an `AGENT/bin` that is not an owned physical `0700`/`0755` directory, refuses at inspect and confirmation before any change. Both pinned archives are downloaded and verified in the private stage before the prefix or settings change.

## Undo Shared changes

Use the **actual installed TARGET**, never a prefix or agent:

```sh
gentle-ai shell recover TARGET inspect
gentle-ai shell recover TARGET PRINTED_CONFIRMATION
```

If a Shared installation fails as uncertain **before TARGET exists**, the failure prints `workspace=WORKSPACE`. Use `WORKSPACE/installed` as ROOT in the same two commands; it holds any saved preimages, and recovery refuses if none were saved. Keep that workspace until recovery completes.

Recovery requires intact saved preimages and fresh printed consent. It restores the **whole prefix and agent**, so it can overwrite later edits, not just the two settings keys. Changed selection/root identities or corrupt preimages refuse. Quarantines, evidence and command bindings remain: this is **not uninstall or TARGET deletion**.

**Recover before deleting TARGET.** Shared `npmCommand` invokes Node/npm inside TARGET; deleting it can break the selected Pi's package operations even while its prefix remains. Keep TARGET and its saved preimages until recovery completes.

If TARGET was already removed, this recovery command cannot reconstruct its lost preimages. Restore the selected prefix and agent only from an independently verified backup outside TARGET; do not guess prior settings or redirect `npmCommand` to an arbitrary runtime. Without that backup, preserve the damaged Shared selection and use a different, empty Separate target as described below. That gives a new installation, not restoration of the old Shared prefix or settings.

## Repair Separate graph drift

Without an intact recovery snapshot, do not reinstall into the damaged target. Keep the previous target, agent configuration/history and evidence; inspect and confirm a **different, empty TARGET** using the Separate commands above, then use its bindings.

There is no automatic migration. Review user-authored data before manual transfer; **do not copy managed `npmCommand`, package registrations or runtime files** from the damaged installation. A valid saved upgrade snapshot can instead use the recovery route.

## Safety limits

Stock Pi's updater is retained. Readback accepts only complete known prior/modern graphs; unknown versions, bytes or placement fail closed and retain evidence. Package lifecycle scripts stay disabled. Installation/UI opening or a same-version reinstall does not qualify the full update journey.

Never delete uncertain roots, stages, locks or evidence to retry. Inspection inventories are not backups; recovery is not hostile-same-UID custody or full disaster recovery. Actual manager, cancellation, startup/update and recovery qualification remains pending on the current candidate.
