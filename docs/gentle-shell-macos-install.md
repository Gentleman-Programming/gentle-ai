# Gentle Shell on macOS

**Source candidate only, not a release guide.** On an Apple silicon Mac with macOS 14 or newer, `gentle-ai shell install` creates a private Gentle Shell in **Separate** or **Shared** mode: stock Pi 1.0.0, Gentle/native 4.0.0 and Node 24.18.0/npm, all from pinned artifacts. `gentle-ai shell recover` restores Shared preimages. Modes, confirmations and recovery follow the [Linux contract](gentle-shell-linux-install.md); this page lists what differs on macOS. Support is declared only by a passing [native macOS qualification](evidence/shell-macos-install-qualification.md) receipt for the exact candidate.

## Quick path

```sh
gentle-ai shell install --target /owned/private-parent/shell --mode separate --inspect
gentle-ai shell install --target /owned/private-parent/shell --mode separate --confirm PRINTED_SHA256
/owned/private-parent/shell/bin/pi --version
```

For Shared, use `--mode shared` and supply `--prefix /owned/selected-prefix --agent /owned/selected-agent` on **both** calls.

Or run `gentle-ai shell install` with no flags for the installer TUI: Enter reviews, `y` confirms and closes the TUI before installation starts, Escape aborts. Launch `TARGET/bin/gentle-shell` or `TARGET/bin/pi`; `TARGET/bin/gentle-shell install` reopens the installer.

## Installing from the gentle-ai TUI

The main `gentle-ai` menu entry **Install Gentle-Shell, our own agent** installs Separate into `~/.local/share/gentle-shell/install` and publishes `~/.local/bin/gentle-shell`. On macOS it writes the same canonical preference files as Linux amd64; **Manage → Apply preferences** rewrites those same files in place.

In Manage, a component change (CodeGraph, or any change on an **On my existing Pi** install) shows **COMPONENT CHANGES** and offers **Update installation**: it reinstalls the target with the selected profile, including the preference changes of the same summary. Preference-only changes keep **Apply preferences**. **On my existing Pi** never offers CodeGraph ON: choosing it turns CodeGraph off with "CodeGraph needs a Separate install; it stays off for On my existing Pi".

| Option | macOS |
| --- | --- |
| Persona: Gentleman, Neutral, Custom | Supported. Custom leaves `config/persona.json` untouched. |
| Background subagents, Strict TDD | Supported, ON or OFF (`config/background-subagents.json`, `config/strict-tdd.json`, both with their `schema`). |
| Global RDD | Supported, ON or OFF (`home/.gentle-ai/state.json` `rdd_mode`). |
| Context7 | Supported, ON or OFF (`agent/mcp.json` `context7` entry with `url`, `enabled` and `exposure`). |
| CodeGraph | Supported, ON (the default) or OFF. ON stages the pinned `@colbymchenry/codegraph` 1.4.1 darwin-arm64 library under `runtime/codegraph`, records `"codegraph": true` in `installation.json` and puts `runtime/codegraph/bin/codegraph` on the launched Separate shell's PATH. Adding or removing CodeGraph on a managed install uses Manage → Update installation. |
| SDK source | Stable or Main preview, exactly as on Linux amd64. Main preview keeps stock Pi installed and composes the latest Gentle Shell `main` snapshot as a private package under `runtime/main-sdk` (with the pinned darwin-arm64 stock native member and the `runtime/node_modules -> ../prefix/lib/node_modules` link), then selects it in `agent/settings.json`. `installation.json` records it under `main_sdk`. Every other option above works with either channel. |

Stable installs gentle-pi at the newest release of the reviewed supplier list and detects new reviewed releases at every update check without a gentle-ai rebuild; see [the Stable trust boundary](gentle-shell-stable-trust-boundary.md) for which authority admits supplier code and what an attacker would need.

### Switching Stable and Main preview

On an installed Separate target, **Manage → SDK source → Apply preferences** records the channel in `config/update-channel.json` (`{"channel":"main"}` or `{"channel":"stable"}`), then **Update now** applies it with the in-place channel overlay, as on Linux amd64. The overlay changes only the SDK declaration and `installation.json` `main_sdk`. Node, `fd`, `rg`, CodeGraph, preferences, the agent home and the supervisor stay untouched. Going back to Stable removes `runtime/main`, `runtime/main-source`, `runtime/main-sdk` and `runtime/node_modules`. If any step fails, every overlay path is removed again, the previous `settings.json` and `installation.json` bytes come back, and the next attempt starts clean.

## Requirements

| Check | Requirement |
| --- | --- |
| Hardware and OS | Native Apple silicon (arm64), not Rosetta; macOS 14 or newer. Anything else refuses before any change. |
| User | Run as the target user. Root or `sudo` refuses. |
| Target parent | Owned by you, mode `0700`, on one filesystem. Absolute paths using only ASCII letters, digits, `/`, `_`, `.` and `-`. |
| Path spelling | `/tmp`, `/var` and `/etc` are resolved once to their `/private/...` spelling. Any other symbolic link in the path refuses. |
| Extended metadata | Every checked entry refuses BSD flags (other than nodump, hidden, tracked and APFS compression) and ACLs with any allow entry. Deny-only ACLs and `com.apple.provenance` are accepted. `com.apple.quarantine` refuses on entries the installer creates; your Shared prefix and agent, their saved preimages and the running `gentle-ai` are yours, so a quarantine left by an approved download is accepted there. |
| Supervisor binary | The running `gentle-ai`, after resolving links such as a Homebrew symlink, and every ancestor directory must be owned by you or root and not group/other writable. Only the root-owned sticky `/private/tmp` is excepted. |
| Durability | The installer's own Go writes and directory entries flush with `F_FULLFSYNC`; a filesystem without it (some network or FUSE mounts) refuses installation instead of risking unflushed data. The Node provisioning helper (prefix, settings and recovery copies) uses Node's `fsync`, which tries `F_FULLFSYNC` and falls back to a weaker barrier or plain `fsync` where the filesystem lacks it. |

## Modes on macOS

| Mode | Selection and effects |
| --- | --- |
| Separate | New private prefix, runtime, HOME, agent and state; an existing personal Pi is untouched. |
| Shared (`--mode shared`) | Explicit existing owned global Pi prefix and agent on the target's filesystem; both new bindings use those same objects. Inspect previews the `settings.json` `packages` and `npmCommand` changes exactly as on Linux. |

Separate publishes `~/.local/bin/gentle-shell` and `~/.local/bin/gs`. When `~/.local/bin` is not on the PATH of the terminal that started the install, it appends one marked line, `export PATH="$HOME/.local/bin:$PATH" # gentle-ai: gentle-shell launcher`, to the startup file your shell reads for new terminals: `$ZDOTDIR/.zshrc` (or `~/.zshrc`) for zsh, the first existing of `~/.bash_profile`, `~/.bash_login` and `~/.profile` (else a new `~/.bash_profile`) for bash, `conf.d/gentle-ai-shell.fish` (the same line in fish syntax) for fish, and `~/.profile` otherwise. It never writes through a symlink, never adds the line twice, and prints the line to add by hand when it may not edit that file. Uninstall removes exactly that line and deletes the file only when nothing at all is left in it (0 bytes). The installer does not otherwise modify PATH or shell files, replace an unrelated `pi`, or require a root installation. It writes pinned `fd` and `rg` (the `aarch64-apple-darwin` builds below) into `AGENT/bin`, which stock Pi prefers over PATH: the private agent (Separate) or the selected `--agent` (Shared). Shared never replaces personal tools: an existing `fd` or `rg` (including links), or an `AGENT/bin` that is not an owned physical `0700`/`0755` directory with clean metadata, refuses at inspect and confirmation before any change.

"On my existing Pi" reuses your Pi agent (`~/.pi/agent`, `0755` as Pi creates it) and follows the same agent checks and rollback as on Linux. See [On my existing Pi](gentle-shell-linux-install.md#on-my-existing-pi-agent-checks-and-rollback). Pi's own `bin/fd` and `bin/rg` are set aside inside the installation, replaced by the pinned builds below, and restored by uninstall. A failure before publication restores them with the rest of the agent. Uninstall restores `settings.json` and `mcp.json` with their original bytes and modes, and removes an `mcp.json` that did not exist before. Every refusal happens before any write, and a failure after the first write restores the agent exactly.

## Undo Shared changes

Use the **actual installed TARGET**, never a prefix or agent:

```sh
gentle-ai shell recover TARGET inspect
gentle-ai shell recover TARGET PRINTED_CONFIRMATION
```

Recovery is the Linux contract: it needs intact saved preimages and fresh printed consent, restores the **whole prefix and agent**, moves the current contents to a retained quarantine beside the preimages, and is not uninstall. If a Shared installation fails as uncertain before TARGET exists, use the printed `WORKSPACE/installed` as ROOT. **Recover before deleting TARGET**; see [Undo Shared changes](gentle-shell-linux-install.md#undo-shared-changes) for the full rules. On macOS the restore command runs under the same process-group containment as installation.

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

CodeGraph ON adds two npm archives of the same 1.4.1 release as Linux, each pinned to its registry `dist.integrity` (SHA-512). The bundled `node` member of the darwin-arm64 archive (120573328 bytes) is never written; the library runs on the pinned Node above.

| Artifact (registry.npmjs.org) | Bytes | Integrity |
| --- | --- | --- |
| `@colbymchenry/codegraph-1.4.1.tgz` | 210435 | `sha512-xZayboJt6T3QBzBsBsI0eJ3J/tN4vYs6LMuvrxJVdM71yQFjrFKFeV3KsbicAgkj6AHnTXxUNk26g9BYwkNMgg==` |
| `@colbymchenry/codegraph-darwin-arm64-1.4.1.tgz` | 50996796 | `sha512-USSpExjuxZWN0OeNexZp2V5fCfOI4ZekqE79CJiBdGdPbqGTB+857ImIqzWGyF/rAyyED94Wc0Cd82t+uWD30w==` |

Stock Pi and its npm graph come from the same frozen lock as Linux, with the darwin arm64 optional packages selected.

## Node bootstrap publication

BSD `mv` cannot rename without replacing, so the bootstrap only stages Node. The installer then accepts only the exact reported stage path `<parent>/.gentle-node-stage.<8 characters>/node`, re-verifies every staged file against its inventory and the pinned `bin/node`, and publishes with an atomic `RENAME_EXCL` rename that refuses any existing destination.

## Repair Separate graph drift

Do not reinstall into a damaged target. Keep it, its agent data and evidence; inspect and confirm a **different, empty TARGET** with the Separate commands above. Review user-authored data before any manual transfer; do not copy managed runtime files or package registrations.

## Safety limits

Stock Pi's updater is retained. Readback accepts only complete known graphs; unknown versions, bytes or placement fail closed and keep evidence. Package lifecycle scripts stay disabled. Never delete uncertain roots, stages or evidence to retry.

See also: [Gentle Shell on Linux](gentle-shell-linux-install.md) and [Gentle Shell on Windows](gentle-shell-windows-install.md).
