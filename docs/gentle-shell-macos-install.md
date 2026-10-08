# Gentle Shell on macOS

**Source candidate only, not a release guide.** On an Apple silicon Mac with macOS 14 or newer, `gentle-ai shell install` creates a **Separate** private Gentle Shell: stock Pi 1.0.0, Gentle/native 4.0.0 and Node 24.18.0/npm, all from pinned artifacts. Shared mode and recovery are not available on macOS in this version. Native macOS qualification evidence is still pending.

## Quick path

```sh
gentle-ai shell install --target /owned/private-parent/shell --mode separate --inspect
gentle-ai shell install --target /owned/private-parent/shell --mode separate --confirm PRINTED_SHA256
/owned/private-parent/shell/bin/pi --version
```

Or run `gentle-ai shell install` with no flags for the installer TUI: Enter reviews, `y` confirms and closes the TUI before installation starts, Escape aborts. Launch `TARGET/bin/gentle-shell` or `TARGET/bin/pi`; `TARGET/bin/gentle-shell install` reopens the installer.

## Requirements

| Check | Requirement |
| --- | --- |
| Hardware and OS | Native Apple silicon (arm64), not Rosetta; macOS 14 or newer. Anything else refuses before any change. |
| User | Run as the target user. Root or `sudo` refuses. |
| Target parent | Owned by you, mode `0700`, on one filesystem. Absolute paths using only ASCII letters, digits, `/`, `_`, `.` and `-`. |
| Path spelling | `/tmp`, `/var` and `/etc` are resolved once to their `/private/...` spelling. Any other symbolic link in the path refuses. |
| Extended metadata | Owned entries refuse BSD flags (other than nodump, hidden, tracked and APFS compression), ACLs with any allow entry, and `com.apple.quarantine`. Deny-only ACLs and `com.apple.provenance` are accepted. |
| Supervisor binary | The running `gentle-ai`, after resolving links such as a Homebrew symlink, and every ancestor directory must be owned by you or root and not group/other writable. Only the root-owned sticky `/private/tmp` is excepted. |
| Durability | The installer flushes its own writes and directory entries with `F_FULLFSYNC`. A filesystem without `F_FULLFSYNC` (some network or FUSE mounts) refuses installation instead of risking unflushed data. |

## Modes on macOS

| Mode | macOS status |
| --- | --- |
| Separate | Available. New private prefix, runtime, HOME, agent and state; an existing personal Pi is untouched. |
| Shared (`--mode shared`) | Not yet available. Refuses at inspection, before any change. |
| `gentle-ai shell recover` | Not yet available. It restores Shared preimages, so it refuses too. |

The installer does not modify PATH or shell files, replace an unrelated `pi`, or require a root installation. It also writes pinned `fd` and `rg` into the private `AGENT/bin`, which stock Pi prefers over PATH.

## Containment: what it is and what it is not

macOS has no cgroups or systemd user manager. Each installer command and the launched Pi run in **their own process group with per-process resource limits**. Cancellation or a deadline kills the whole group, and the group is reaped until the kernel reports it empty.

| Linux unit property | macOS equivalent |
| --- | --- |
| `KillMode=control-group` | Kill and reap the owned process group |
| `TasksMax=64` | `RLIMIT_NPROC` = your current process count + 64 |
| `CPUQuota=100%` | `RLIMIT_CPU` = the command's deadline in seconds; none for a launched Pi session |
| `UMask=0077` | `umask 077` |
| `LimitNOFILE=1024:524288` | `RLIMIT_NOFILE` 1024 soft, 524288 hard (capped at the inherited hard limit) |
| (no equivalent) | `RLIMIT_FSIZE` 4 GiB per written file, for installer commands and the Pi session |

**Limitations, stated plainly:**

- **A descendant that calls `setsid` (or `setpgid`) escapes the group.** It is neither killed nor reaped.
- **There is no enforceable memory cap.** macOS refuses to lower `RLIMIT_DATA` and `RLIMIT_AS`, so there is no equivalent of Linux `MemoryMax`/`MemorySwapMax`.
- **`RLIMIT_NPROC` counts every process of your user**, not only the owned group. A busy session can hit the limit earlier than the Linux per-unit task cap.
- There is no `NoNewPrivileges` or capability drop.

## Pinned artifacts

Each archive pin equals its publisher's checksum. Member pins are derived from those exact archives.

| Artifact | Bytes | SHA-256 |
| --- | --- | --- |
| `node-v24.18.0-darwin-arm64.tar.gz` (nodejs.org) | 52087559 | `e1a97e14c99c803e96c7339403282ea05a499c32f8d83defe9ef5ec66f979ed1` |
| `bin/node` member | 120965360 | `ee6fb0e015284d83a91e8ec5213f43a157f8a392b58555301682892ba928c04a` |
| `gentle-ai_4.0.0_darwin_arm64.tar.gz` | 6143402 | `d2159caf6d68f367b18830ece6af71ef26963d5f5320d7df6a794773f45cc7e9` |
| `gentle-ai` member | 16047186 | `18a9f7fae55d85c95684b6d512a4a148d0cb24a856325f72573c34caf65159eb` |
| `fd-v10.5.0-aarch64-apple-darwin.tar.gz` | 1334374 | `b67e1836c468e42e411984b56e52fa7abec08c2bd22c867398e7cc134aac5e12` |
| `ripgrep-15.2.0-aarch64-apple-darwin.tar.gz` | 1764284 | `3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4` |

Stock Pi and its npm graph come from the same frozen lock as Linux, with the darwin arm64 optional packages selected.

## Node bootstrap publication

BSD `mv` cannot rename without replacing, so the bootstrap only stages Node. The installer then accepts only the exact reported stage path `<parent>/.gentle-node-stage.<8 characters>/node`, re-verifies every staged file against its inventory and the pinned `bin/node`, and publishes with an atomic `RENAME_EXCL` rename that refuses any existing destination.

## Repair Separate graph drift

Do not reinstall into a damaged target. Keep it, its agent data and evidence; inspect and confirm a **different, empty TARGET** with the Separate commands above. Review user-authored data before any manual transfer; do not copy managed runtime files or package registrations.

## Safety limits

Stock Pi's updater is retained. Readback accepts only complete known graphs; unknown versions, bytes or placement fail closed and keep evidence. Package lifecycle scripts stay disabled. Never delete uncertain roots, stages or evidence to retry.

See also: [Gentle Shell on Linux](gentle-shell-linux-install.md) and [Gentle Shell on Windows](gentle-shell-windows-install.md).
