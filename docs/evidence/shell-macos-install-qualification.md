# macOS Shell installer qualification boundary

**macOS support is declared only by a passing receipt from the native qualification workflow for the exact candidate.** The receipt proves one real Separate install, one real Shared install with a refusal control, and one Shared recovery on Apple silicon. It does not remove the macOS containment limitations. See the [user guide](../gentle-shell-macos-install.md) for requirements, modes and recovery.

## Quick path

1. The [workflow](../../.github/workflows/shell-macos-qualification.yml) runs on `macos-latest` (arm64) when the installer surface changes, or by manual dispatch.
2. It runs the native `shellinstaller`, `scripts` and CLI `Shell` tests, builds `gentle-ai`, then runs the [qualification script](../../e2e/shell-macos-qualification.py).
3. Download the `shell-macos-qualification-<sha>` artifact: `receipt.json` must say `"result": "pass"` and `sourceSHA` must equal the candidate commit.

Run it locally on an Apple silicon Mac (macOS 14+, not root):

```sh
base="$(mktemp -d /private/tmp/gentle-macos-qualification.XXXXXX)"
go build -mod=readonly -o "$base/gentle-ai" ./cmd/gentle-ai
/usr/bin/env -i PATH=/usr/bin:/bin TMPDIR="$base" /usr/bin/python3 -I e2e/shell-macos-qualification.py \
  --gentle-ai "$base/gentle-ai" --receipt "$base/receipt.json"
```

## What a passing receipt proves

| Checkpoint | Observed on the real host |
| --- | --- |
| `host`, `root` | Native arm64, macOS 14+, non-root; one new `0700` root under the canonical `/private/...` TMPDIR. Installer commands get only `HOME`, `TMPDIR`, `XDG_*` inside that root and `PATH=/usr/bin:/bin`. |
| `separate-inspect`, `separate-confirm` | Separate install through printed consent; the manifest binds the candidate `gentle-ai` hash and the Node pin. |
| `separate-launch` | `bin/pi --version` and `bin/gentle-shell --version` print `1.0.0` with stdin closed; private Node prints `v24.18.0`. |
| `separate-artifacts` | Installed `node` and Gentle `gentle-ai` equal their darwin pins (bytes and SHA-256). Retained `fd`/`rg` archives equal their pins and the installed `AGENT/bin` tools equal the archive members. |
| `shared-fixture`, `shared-fd-refusal` | A Shared inspect against an agent with a pre-existing `bin/fd` refuses, names the conflict, creates no target and leaves the prefix, agent and parent inventories unchanged. |
| `shared-inspect` … `shared-artifacts` | Shared install binds the selected prefix and agent; launch versions and artifact pins as above. |
| `recover-inspect` … `recover-restore` | Damaging the coding-agent `package.json` makes Pi refuse to start; the recovery token is stable after damage; a wrong token refuses without changes; the printed token restores the original `package.json` and `settings.json` bytes and retains both quarantines. |
| `residue`, `real-home` | No stage or workspace residue (`.gentle-node-stage.*`, `.gentle-user-*` and similar); redirected HOME, TMPDIR and XDG directories stay empty; the real-HOME locations the stack would write (`.pi`, `.npm`, `.gentle-ai`, `.config`, `.cache`, `.local`, …) are unchanged. In `Library/Caches` and `Library/Application Support`, which macOS daemons rewrite constantly, the run fails when any child named after the stack (gentle, node, npm, pi, rg, fd, ripgrep, earendil, supervisor, go-build) appears or changes; other changes are recorded in the receipt as `realHomeSystemChurn`. |

Each command has its own deadline inside a whole-run deadline (22 min) that nests inside the step (25 min) and job (55 min) deadlines. On a timeout or unexpected exit the receipt names the failing checkpoint and command, its limit and elapsed seconds, and a bounded output tail.

## What it does not prove

| Not qualified | Why it matters |
| --- | --- |
| `setsid`/`setpgid` escape | A descendant that leaves the process group is neither killed nor reaped. The qualification does not test or remove this limitation. |
| Memory cap | macOS refuses to lower `RLIMIT_DATA`/`RLIMIT_AS`; there is no `MemoryMax` equivalent. Not qualified away. |
| Per-unit task cap | `RLIMIT_NPROC` counts every process of the user, not only the owned group. |
| Privilege hardening | No `NoNewPrivileges` or capability drop exists on macOS. |
| Node durability | The Node helper's `fsync` may fall back from `F_FULLFSYNC`; only Go writes fail closed. |
| Other hosts | One `macos-latest` image per run. Other macOS versions, Intel/Rosetta refusal and network or FUSE filesystems are covered by unit tests, not by this receipt. |
| Interactive and fault paths | The installer TUI, cancellation, publication faults, stock `pi update` and a full Pi session are not exercised. |
| Full HOME inventory | The real-HOME check is a fixed watch list, not a byte inventory of the whole home directory. Inside the two system-volatile `Library` directories it attributes changes by child name: on `macos-latest` every observed change came from Apple daemons (Spotlight, TCC, CloudKit, Safari, geoanalyticsd, remindd, …), so a write under an unrelated name would not be caught there. |
| Publisher availability | Every run downloads pinned artifacts from their publishers; an outage fails the run without saying anything about the installer. |

## Recorded runs

| Run | Candidate | Host | Result |
| --- | --- | --- | --- |
| Operator Mac, 2026-10-08 | `8ac19f32d` | macOS 26.5.2, arm64, Python 3.9.6 | Pass in 224 s (Separate 44 s, Shared 78 s) |

The CI receipt on `macos-latest` for the delivered candidate is still required; this table is not a substitute for it.
