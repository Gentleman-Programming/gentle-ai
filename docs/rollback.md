# Backup & Rollback Guide

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

The backup system automatically snapshots your configuration files before every install, sync, and upgrade. Backups are compressed, deduplicated, and automatically pruned to keep disk usage under control.

## How it works

Every time you run `gentle-ai install`, `sync`, or `upgrade`, the system:

1. **Computes a checksum** of all files that will be backed up
2. **Skips the backup** if it would be identical to the most recent one (dedup)
3. **Creates a compressed snapshot** (`snapshot.tar.gz`) with all your config files
4. **Prunes old backups** — keeps the 5 most recent, deletes the rest

## Snapshot contents

- `manifest.json` — metadata (source, timestamp, file count, checksum, pin status)
- `snapshot.tar.gz` — compressed archive of all backed-up files
- For paths that did not exist before the operation, the manifest tracks `existed=false`

> **Backup scope**: pre-upgrade and pre-sync snapshots cover only the agents listed in `state.InstalledAgents` (`~/.gentle-ai/state.json`). Config directories for agents you installed outside of gentle-ai are not included in the snapshot.

Legacy (pre-v1.16) backups use a `files/` directory with plain copies instead of a tar.gz archive. Both formats are fully supported for restore.

## Retention policy

| Setting | Default | Behavior |
|---------|---------|----------|
| Keep count | 5 | The 5 most recent unpinned backups are kept |
| Pinned backups | Never deleted | Survive pruning regardless of count |
| Duplicates | Skipped | If config hasn't changed, no new backup is created |
| Compression | Always | New backups use tar.gz (~75% smaller) |

## Pinning backups

You can mark any backup as "pinned" in the TUI to protect it from automatic pruning:

1. Run `gentle-ai` and navigate to the **Backups** screen
2. Use `j`/`k` to select a backup
3. Press **`p`** to toggle pin/unpin
4. Pinned backups show a `[pinned]` indicator

Pinned backups are never automatically deleted, even when the retention limit is exceeded.

## Managing backups (TUI)

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `Enter` | Restore selected backup |
| `p` | Pin/unpin (protect from pruning) |
| `r` | Rename (add a description) |
| `d` | Delete |
| `Esc` | Back |

## Restore behavior

- If `existed=true`: restores the file from the snapshot to its original path
- If `existed=false`: removes the file (reverting files created during install)
- Restore is atomic per file write — no partial restores
- Works with both compressed (tar.gz) and legacy (plain file) backups

## If verification fails

1. Review failed checks in verification report
2. Restore from latest snapshot via the TUI or `gentle-ai restore latest`
3. Re-run install with `--dry-run` to validate plan
4. Re-run install after fixing external dependencies

### Recover a preserved native review agent

**Keeping the file is valid.** A “preserved, not updated” warning means Gentle AI cannot verify ownership: the ledger entry is missing or the file differs from its recorded hash. It does not prove that you customized the file. Even bytes matching the current template do not prove continuing consent to management; a ledger-less matching file remains unmanaged.

To opt back into management for one warned file:

1. Back up the **exact path printed in the warning** outside the agent directory. Open the backup and compare its bytes with the original before proceeding; retain any customizations.
2. Remove **only that warned file**, after verifying the backup. Do not delete the agent directory, other agents, or the ownership ledger. Do not hand-write ledger entries or hashes.
3. Rerun your existing `gentle-ai install` or `gentle-ai sync` invocation with the same runtime, scope, and model choices. For an already configured runtime, sync restores persisted model assignments. Use an explicit scope; for workspace scope, run from the original project directory. Retain any other flags you previously used.

Examples for already configured runtimes in **global** scope (choose only the runtime that owns the warned path):

| Runtime | Recovery command |
|---------|------------------|
| Claude Code | `gentle-ai sync --agent claude-code --scope global` |
| Kiro IDE | `gentle-ai sync --agent kiro-ide --scope global` |
| Kimi | `gentle-ai sync --agent kimi --scope global` |

For workspace installations, use `--scope workspace` instead, from the original project directory. If reinstalling rather than syncing, follow the [install examples](usage.md#install) and retain your original agent, component/preset, scope, and model choices; do not substitute a fresh preset blindly. Sync/install can refresh other managed assets in the selected runtime and scope, not just this file.

Afterward, confirm the file was regenerated and the warning for that path is gone on the next sync. A retired agent filename may no longer be generated; removing it does not bring back a retired template. Compare any customized backup with the new file before selectively reapplying changes. **Do not blindly overwrite the regenerated file with the backup**: modified bytes will again be preserved rather than updated. Keep the verified backup until recovery is confirmed.

## What rollback does NOT cover

- Packages installed via `brew install`, `apt-get install`, or `pacman -S` are not uninstalled during rollback. The snapshot system handles configuration files only.
- If you need to undo a package install, use your platform's package manager directly (e.g., `brew uninstall`, `sudo apt-get remove`, `sudo pacman -R`).
