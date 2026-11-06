# Linux Shell installer qualification boundary

**Current source candidate: full qualification, pinned release and current-tree/upstream CI remain pending.** This note is evidence context, not a release guide or a fresh runtime receipt. See the [user guide](../gentle-shell-linux-install.md) for selection, settings effects and recovery.

## Evidence scope

- [Guest fixture](../../e2e/shell-linux-user-install-guest.py): `smoke` and `full` are distinct; authored assertions are not observed results.
- [Workflow](../../.github/workflows/shell-linux-first-ci.yml): exact candidate receipts must identify their source; earlier/frozen baselines do not qualify the current tree.
- The old published `685` smoke concerns a different candidate. Earlier Separate install/warm UI observations do not establish a fresh current-candidate smoke PASS.
- Reported JavaScript controls (22) and predicate checks (3), earlier Go CLI checks and cross-compilation are partial evidence, **not the full installer/startup/update/recovery journey**. No tests or candidate runtimes were executed for this documentation rewrite.
- Modern roots target Pi 1.0.0 and Gentle/native 4.0.0 with fixed Node 24.18.0/npm; pinning is not runtime proof. Stock Pi and its updater remain unchanged; only complete authenticated known prior/modern graphs are accepted.

## Optional-graph comparison boundary

- After complete physical/source-byte, range, bin, supplier, native and settings verification, comparison may omit a single exact retained, locked, nonapplicable, nonroot optional placement. Live packages and the original graph witness are not rewritten; malformed, duplicate, relocated, unknown and required rows still drift.
- The separately authenticated `user-global-graph.mjs` asset is imported only at that final comparison, not during helper-only recovery. Settings publication remains synced before its graph witness.
- The in-memory current-helper regression exercises the complete 315-node lock corpus (297 observed rows versus the original 272-row witness) for both profiles. This is comparison-boundary evidence, **not a current-candidate physical stock update or full Guest qualification**; those remain pending.

## Caller-project preservation policy

- Each Guest launch compares the whole physical project inventory before and after the PTY: paths, file bytes, link targets and mode/uid/gid/dev/inode/size/mtime/ctime of every object.
- One narrow exception: the root `.git` **directory** may differ only in `mtime_ns`/`ctime_ns`. The native review integration creates and deletes a private `.gentle-ai-review-index-*` index (and Git's `.lock` for it) beside Git's control files, which moves those directory times. Nested `.git` names, other directories and files get no exception.
- Any `.gentle-ai-review-index-*` or associated `.lock` left in the project refuses, even when unchanged across the launch. Separate Git directories and disabling RDD are not accepted workarounds.
- [Preservation controls](../../e2e/shell-linux-preservation.test.py) cover this with synthetic inventories and local temporary fixtures. They are not native Guest proof: whether the pinned runtime leaves only these times and no residue still needs a fresh exact-candidate `full` run.

## Exact limits and debt

- Execution: non-root Linux amd64; cgroup2, capabilities 0, NoNewPrivs 1, memory 3 GiB, swap 0, CPU 1, tasks 64. Existing real delegated systemd user manager >=254 when direct entry is not already qualified; no fallback or skipped-manager readiness claim.
- Shared inventory: at most 250,000 entries, 32 MiB per regular file, 1 GiB total per selected tree. Foreign ownership, writable/special objects, escaping links and changed preimages refuse. Inventory is not a backup.
- Recovery restores whole confirmed prefix/agent preimages, can overwrite subsequent edits and retains quarantines/evidence/bindings; not uninstall, target deletion, hostile-same-UID custody or full disaster recovery.
- Sealed Node uses bundled plus system CA roots, not caller certificate selectors, proxies or loader injections; TLS verification stays enabled. Test CA/key fixtures must remain credentialless and Guest-only, never host trust-store changes.
- Fixed cold `fd`/`rg` archives and physical helper readback, project-preserving startup and both bindings still need fresh exact-candidate runtime evidence. Archive hashes are not independent signature/public-checksum claims.
- Full debt: cold first-install TUI, command registration/full startup closure, native execution, Shared settings/consent/recovery, real prior-to-modern stock update/force, cancellation/reap/PTY/foreground restoration, actual manager controllers/termination, retry/collisions, publication/readback/cleanup faults, personal/project preservation and formatting.
- The 97 protected-preview/private-installer baseline exceptions and their retired APIs are removed; retained runtime/native/TLS primitives and mixed tests live in neutral files. UID65532 primitive execution and independent current-tree full-suite proof remain required.
- Current source-boundary tests cover supplier/resolver/Node file mutations, identity, modes, size, ownership, aliases and cancellation without executing fixtures. Current finalizer tests preserve published recovery evidence on cancellation and unpublished Shared preimages after provisioning begins. These are focused boundaries, not full installation wiring or real interruption/Guest proof.

**Only observed exact-candidate receipts can change these limits. A limited smoke cannot establish functional readiness or full qualification.**
