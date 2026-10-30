# Gentle Shell on Linux

**Source candidate only, not a release guide.** Use an authorized, pinned Linux amd64 build in a bounded Guest. It targets stock Pi 1.0.0 and Gentle/native 4.0.0 with independently pinned modern roots and Node 24.18.0/npm. This is not runtime proof or release readiness; see [qualification evidence and limits](evidence/shell-linux-install-qualification.md).

Other platforms: [macOS](gentle-shell-macos-install.md) and [Windows](gentle-shell-windows-install.md).

## Requirements

- Non-root Linux amd64; an already-qualified execution boundary or an existing delegated **systemd user manager >=254**. No sudo, new-delegation or container fallback.
- **Root opt-in**: root installs refuse by default with a full explanation. A root operator may accept the risk explicitly — `--accept-root-risk` on the CLI, or typing `I accept the risk` on the installer TUI warning screen — and every accepted root run then uses the same sandbox properties through a transient **system** unit, records `rootRisk` in the manifest, and prints a sandbox-disabled warning on every launch. Delegated operations refuse with named causes where root has no support (for example the fresh native slice); the private-sandbox guarantees never silently pretend to hold for root.
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

The ordinary `gentle-ai` Welcome screen starts with the featured **Install Gentle-Shell, our own agent** campaign below the logo. Enter opens the grouped configuration menu and its focused detail panel; narrower terminals use the same choices in a compact layout. End selects Continue. The selector uses the existing Bubble Tea program; a confirmed selection returns to the host before installation starts. The host then rechecks the physical selection and fresh confirmation token. Cancelling returns to Welcome without starting a worker.

The embedded Welcome flow restores the original destination editor with a field-specific **About** column. Its Ready screen has **Install Gentle-Shell / Back / Cancel** on the left and the focused action’s pageable request/scopes on the right. Use arrows or j/k for actions and Enter to select; y/n, Ctrl-E and Escape keep their existing meanings. The `[offset/last]` footer reports review line offsets, not pages. Back preserves the complete request while clearing consent; no action focus, scrolling or rendering starts installation. This presentation does not change the standalone CLI selector.

This entry-point slice connects the existing installation protocol. It does not establish original-installer/PNPM publication or installed-platform qualification.

The internal request binds preference values to physical confirmation while preserving legacy bytes when no profile is supplied. Non-nil profiles are currently refused before installation effects; this slice does not expose new preference controls or silently drop unsupported choices.

The private-stage preference writer is connected before runtime bootstrap. It preserves nil legacy stages, writes only canonical private files for supported internal selections, and leaves unmanaged persona bytes untouched. Public profile admission and installed-platform qualification remain disabled/pending; explicit RDD choices are still refused.

Fresh private-stage creation and preferences are now composed by the installation path: validation precedes creation, the legacy layout is retained for nil profiles, and the caller keeps ownership of cleanup/evidence. This does not enable public profile admission.

Persona/background serialization now has one side-effect-free canonical codec consumed by the POSIX writer. Native custody, exclusive writes and staged readback remain in that writer; Windows non-nil profiles stay refused and no Windows guest qualification is admitted.

Manage on a detected installation splits the selected delta: preference-only changes use **Apply preferences** in place; a component change (CodeGraph, or any change on an **On my existing Pi** install) offers **Update installation**, which reinstalls the target with the selected profile, including the preference changes of the same summary. **On my existing Pi** never offers CodeGraph ON: the CodeGraph library stages only into a Separate or Shared install.

The confirmation review wraps to terminal width. Use PgUp/PgDn to scroll and Home/End to reach the first/last page, including the full Shared settings preview and recovery warnings; scrolling or resizing does not confirm or change your selection.

For command-line installation, inspect, then use the **exact fresh printed confirmation** for that unchanged selection:

```sh
gentle-ai shell install --target /owned/private-parent/shell --mode separate --inspect
gentle-ai shell install --target /owned/private-parent/shell --mode separate --confirm PRINTED_SHA256
```

For Shared, use `--mode shared` and supply `--prefix /owned/selected-prefix --agent /owned/selected-agent` on **both** calls. Do not reuse consent after changing paths or selected contents.

Launch `TARGET/bin/gentle-shell` or `TARGET/bin/pi`. Normal launches have no installer menu; `TARGET/bin/gentle-shell install` reopens it. Separate publishes `~/.local/bin/gentle-shell` and `~/.local/bin/gs`. When `~/.local/bin` is not on the PATH of the terminal that started the install, it appends one marked line, `export PATH="$HOME/.local/bin:$PATH" # gentle-ai: gentle-shell launcher`, to the startup file your shell reads for new terminals: `$ZDOTDIR/.zshrc` (or `~/.zshrc`) for zsh, `~/.bashrc` for bash, `conf.d/gentle-ai-shell.fish` (the same line in fish syntax) for fish, and `~/.profile` otherwise. It never writes through a symlink, never adds the line twice, and prints the line to add by hand when it may not edit that file. Uninstall removes exactly that line and deletes the file only when nothing at all is left in it (0 bytes). The installer does not otherwise modify PATH or shell files, replace an unrelated `pi`, or require a root installation.

The installer TUI runs the portable direct worker, so it first probes the delegated systemd unit every later launch uses. When that probe fails (for example no `XDG_RUNTIME_DIR=/run/user/UID` or no user manager), the install is refused **before any change**: no target, launcher or registry entry is created. A published installation is recorded in `~/.gentle-ai/shell-installs.json` (0600) for Manage. That directory may be gentle-ai's usual `0755` state directory, but it must be owned by you, not a symlink, and not writable by group or others. If the installation cannot be recorded, the completion output says so; the installation itself is kept.

On the Stable SDK channel, gentle-pi is installed at the newest release of the reviewed supplier list, and every update check compares the installed version against that list live — a newly reviewed release needs no gentle-ai rebuild. See [the Stable trust boundary](gentle-shell-stable-trust-boundary.md) for which authority admits supplier code and what an attacker would need.

## Shared settings: preview before confirming

Inspect prints the selected agent's `settings.json` path and **only** its `packages` and `npmCommand` before/after values, including actual physical paths:

| Key | Before | After |
| --- | --- | --- |
| `packages` | Existing array, or absent | Existing entries plus `PREFIX/lib/node_modules/gentle-pi` |
| `npmCommand` | Absent | `["TARGET/runtime/node/bin/node", "TARGET/runtime/node/lib/node_modules/npm/bin/npm-cli.js", "--prefix", "PREFIX"]` |

`TARGET` and `PREFIX` above stand for your selected absolute paths, not literal settings values. The command uses pinned Node/npm. Other settings keys and existing package entries are preserved; foreign npm overrides or Gentle declarations refuse. Preview is disclosure, not authority or a backup; confirm only the exact fresh inspected selection.

Shared also writes **two new files** into the selected agent: `AGENT/bin/fd` (fd 10.5.0) and `AGENT/bin/rg` (ripgrep 15.2.0), which stock Pi prefers over PATH. Personal tools are never replaced: an existing `fd` or `rg` (including links), or an `AGENT/bin` that is not an owned physical `0700`/`0755` directory, refuses at inspect and confirmation before any change. Both pinned archives are downloaded and verified in the private stage before the prefix or settings change.

## On my existing Pi: agent checks and rollback

"On my existing Pi" keeps a private prefix and runtime and reuses your Pi agent (`~/.pi/agent`). The agent may be `0755`, as Pi creates it. It must be a directory you own that no other user can write. The install writes `settings.json` (`packages`, `npmCommand`), `mcp.json` when Context7 is selected, and the pinned `bin/fd` and `bin/rg`.

Pi downloads its own `fd` and `rg` into `AGENT/bin` on its first run. The install does not refuse them. It sets them aside instead:

1. It moves Pi's `bin/fd` and `bin/rg` into `TARGET/backup/agent-tools/`. Their SHA-256, size and mode are recorded in `TARGET/state/agent-tools.json`.
2. It writes the pinned `fd` and `rg` in their place.
3. Uninstall removes the pinned copies and puts Pi's back, with the same bytes and modes.

If `bin/fd` or `bin/rg` changed after the install (for example, Pi downloaded a newer copy), uninstall does not overwrite it. Pi's original is kept in `TARGET.removed-UTC.pi-tools/`, next to its `agent-tools.json`. Uninstall still completes, and Manage shows it as a success with a note naming that path. A slot that is now empty gets Pi's original back.

Only regular files are set aside. These are refused as before: a link, a directory or another entry in place of `fd` or `rg`; a tool with extra hard links; a tool other users can write; and, on macOS, a tool with an ACL or BSD file flags.

Shared mode keeps its refusal of an existing `fd` or `rg`. It has no agent rollback image and no uninstall that could put them back.

These conditions refuse before anything is written, and the message names the path and the fix:

| Condition | Fix |
| --- | --- |
| `AGENT/bin/fd` or `AGENT/bin/rg` is a link, a directory or another non-regular entry, has extra hard links, or is writable by other users | Move it out of `AGENT/bin` (`chmod go-w FILE` for permissions). Pi downloads its own again if it is ever missing. |
| `settings.json` sets `npmCommand` or already declares `gentle-pi` (often left by an earlier attempt) | Remove that key or `packages` entry. |
| `settings.json` or `mcp.json` is not a JSON object, is a link, has extra hard links, or is writable by other users | Fix the file (`chmod go-w FILE` for permissions). |
| The agent is writable by other users | `chmod go-w AGENT`. |
| The agent is a symbolic link | Select its real path instead. |
| macOS: the agent has an ACL or BSD file flags (immutable, append-only) | `chmod -N AGENT` for an ACL; `chflags nouchg,nouappnd AGENT` for flags. |
| Main preview with an agent that is not `0700` | `chmod 700 AGENT`, or choose Stable or Separate. |

If the install fails after its first write, the installer puts back the exact bytes, modes and presence of every file it touched in the agent, including Pi's own `fd` and `rg` once they were set aside. It also removes its own temporaries and any `bin` directory it created. If the agent was replaced during the install, the installer does not write into it. Instead, the original files are saved beside the target, and the failure message gives their path.

Once the installation is published, the agent is not rolled back. A later failure (readback, cleanup or timeout) is reported, and the agent keeps the bindings the published install records. Uninstall restores the agent from that evidence.

Uninstall puts `settings.json` and `mcp.json` back as they were before the install: the same bytes and mode (Pi creates `settings.json` as `0644`). A file that did not exist before the install, such as an `mcp.json` that only Context7 created, is removed. The install records each file's presence and mode in `TARGET/state/agent.preimage.json`. Installs made before this record existed restore these files as owner-only `0600`, which never widens access. Uninstall refuses before any change when the agent changed since the install or the evidence is damaged.

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

## Global RDD source interface

The source CLI supports `gentle-ai review mode status --global-only --json` and the corresponding `enable`/`disable` operations. This route reads or writes only the global HOME source without repository resolution, including its unknown-value repair commands. It requires global scope and refuses every supplied `--expected-revision`. Ordinary status/review resolution is unchanged: a clone-local OFF still wins even when the global-source view reports ON.

This source-only interface does not alter the current user switch or installed binaries, admit installer profiles, or supply a compatible published artifact. Explicit staged RDD preferences remain refused until their actual supported native effect and readback are integrated. The retained POSIX proof cut includes an actual Git-invocation shim with an ordinary-route negative control, printed global-only repairs, and unreadable-file byte/identity/mode/mtime preservation. These are recovered assertions, not a new execution, Windows custody proof or installed acceptance.

## Context7 selection contract

The internal DTO and named transport accept only explicit Context7 ON/OFF or omission, bind canonical bytes to confirmation, and preserve historical bytes when omitted. Empty/null/duplicate/unknown/invalid values remain refused. This is inert choice support: public profiles and explicit Context7 stage effects remain refused, without configuration writes or an implied Ready result. The default UI API remains with its actual future consumer.

## Memory

A Separate installation shares your global Engram memory, the same way the bundled Gentle Shell and your other agents do. Each launch decides memory once, in the entry process before the bounded systemd unit starts, and never fails the launch because memory is unavailable:

- **You already chose Engram settings.** If your environment sets `ENGRAM_BIN`, `ENGRAM_URL`, `ENGRAM_DATA_DIR`, `ENGRAM_PORT`, `ENGRAM_SOCKET`, `ENGRAM_DATABASE_URL` or `ENGRAM_SERVER_MODE` (or an `ENGRAM_STARTUP_POLICY` other than `allow`), those exact values are passed to the session unchanged, nothing is probed or started, and memory runs disabled for that launch, as in the bundled Gentle Shell. An explicit `ENGRAM_BIN` or `ENGRAM_URL` is never replaced; without your own `ENGRAM_BIN`, the session gets the installation's verified core instead of any `engram` on `PATH`.
- **Credentials never cross the unit's command line.** The bounded unit receives its environment on the `systemd-run` command line, which other local processes can read. So on Linux `ENGRAM_DATABASE_URL`, or an `ENGRAM_URL` with credentials (or one that does not parse), is not passed. That launch starts with memory disabled and a notice instead.
- **The root-risk launch never starts a shared server.** A launch as root runs in a system unit that receives no shared-memory decision, so a Separate session there starts with memory disabled (`ENGRAM_STARTUP_POLICY=disabled`) instead of running an unverified `engram` from `PATH`.
- **Native PNPM trees run without memory.** A Separate launch through a native PNPM tree always starts with memory disabled.
- **A verified server is running.** A server on `127.0.0.1:7437` whose `/compatibility` answer proves a startup-verified store is reused through `ENGRAM_URL=http://127.0.0.1:7437`.
- **Nothing is listening.** The installation's verified Engram core (`AGENT/bin/engram`) starts `engram serve` in its own session outside the unit, with your real home, so the store is your usual `~/.engram`. This store is the only thing a Separate installation writes outside its own folder; the server keeps running after the session ends, like any shared Engram server.
- **Anything else disables memory for that launch.** A listener that does not prove compatibility, a store migrated by a newer Engram than the installation's core, a missing core, or a server that does not report ready within about 10 seconds starts nothing. The session starts with `ENGRAM_STARTUP_POLICY=disabled` and a one-line notice explaining why. Gentle Shell never stops or replaces a server already listening on `127.0.0.1:7437`, and rechecks on every launch.

Shared and agent installations keep their existing memory setup; this decision applies to Separate only. Apart from your own Engram selections, your `PATH`, tokens and other settings are never passed to the session or the server.
