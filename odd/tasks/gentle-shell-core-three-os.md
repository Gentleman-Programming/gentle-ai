# W15 — Main-only command-mode intent contract

This new task starts at clean local `main` commit `c9d6b0f7653fb1fbe2f56c280699d1399f0225df`. It ports only a pure shellinstaller data contract; there is **no installer, approval, Apply or Ready route** in this unit.

## Product effect matrix (intent, not execution)

| Command | Stable | Main |
| --- | --- | --- |
| `pi` | Describe replacement/upgrade of the **existing Pi** with pinned Stable Pi+Gentle-Shell, conditional on future physical InstanceID-bound consent and rollback. | Same existing-Pi replacement boundary, conditional on pinned Main source and the same future consent/rollback. |
| `gentle-shell` | Describe a **separate Pi, home and executable**, reachable only by `gentle-shell`; existing Pi untouched, pinned Stable. | Same isolated-Pi boundary, conditional on pinned Main source; existing Pi untouched. |

Command choice and channel choice are independent. A description does not select source bytes, read Pi settings, confer takeover authority, or authorize removal or installation.

## Scope and verification

- Seven NEW paths only: this task (≤65 lines) plus `internal/shellinstaller/{channel.go,channel_test.go,profile.go,profile_test.go,terminal_entrypoint.go,terminal_entrypoint_test.go}`. Total authored changed diff ≤400 lines.
- Six Go files must be byte-identical to the reviewed W14 core donor at `/home/devel/projects/gentle-ai-worktrees/shell-command-mode-contract-14`; compare full per-file SHA-256. W14 source is donor DATA, not a cherry-pick, branch merge or Main runtime proof.
- Keep `Profile.Validate` refusing `gentle-shell` until independently reviewed isolated source/instance/consent/install/Ready work exists. The legacy zero value remains Pi intent, not a takeover approval. Stable/Main source pinning is NOT implemented here.
- Main's existing retired-SDD run/sync NO-OP cases, legacy fields and guidance are untouched. Add no SDD offer/manage API; do not import core's live SDD behavior or claim complete SDD removal.
- Go tests are written as source DATA only. Independent adversarial STATIC review is the sole permitted verification now; no Go/Node/Pi tests/builds, config/Apply/root/network/CI, commit or delivery without fresh exact authority.
- Rollback boundary is deleting only these seven new paths. No Main, `main-current`, W14 donor, frozen W12, or U1 modifications. An operational Linux installer and protected WSL runner remain separately gated.

## W18 — Pi/Gentle-Shell draft and closed candidate gate (source-only)

- Start at CLEAN committed W15 `7eb498a53a4912ffd63d985dabf04ba2d840a6be`, not W16's legacy OpenCode SDD cleanup. Edit only this append (≤35 lines), NEW `internal/shellinstaller/draft_plan.go`/`draft_plan_test.go` (≤85/125 lines), NEW `internal/cli/shell_install_gate.go`/`shell_install_gate_test.go` (≤45/85 lines); total ≤375, hard ≤400 changed diff lines.
- Pure 2×2 `DraftPlan` records independent Stable/Main channel and `pi`/`gentle-shell` command intent; `pi` proposes replacing existing Pi only after future physical InstanceID-bound consent and rollback, while `gentle-shell` proposes separate Pi/home/executable without touching existing Pi.
- Every draft enumerates MISSING independent source, physical instance, consent, rollback, executed-byte and Ready proofs. This is UNEXECUTABLE data, not a source pin, approval, operation, installation, verified Ready, home/path resolution or command invocation.
- A distinct CLI **shell candidate** gate returns typed `not-authorized` for both modes BEFORE generic resolver/state/home/backup/command access; it neither calls `RunInstall`/pipeline nor creates any public app/TUI dispatch. Generic legacy Pi package installation remains unchanged and `Profile.Validate` still rejects `gentle-shell`.
- Stable/Main source identity is not resolved or attested. New seam does not offer/manage/call SDD or OpenCode SDD functions. W17 stays paused; neither W17 nor legacy OpenCode SDD is a Pi/Gentle-Shell install dependency.
- Go tests are source DATA only, RED/GREEN UNRUN; independent adversarial STATIC review required. No Go/Node/Pi test/compile, real configuration, Apply, root, network, CI, commit or delivery. Stop if cap/dependency closure fails or any Ready/authorization claim would be implied.

## W19 — separate Gentle-Shell negative boundary (source-only)

- From exact CLEAN W18 `a3c3c667b1ce1e90b57d6a9d869190af0c6db62a` in a NEW dedicated worktree; only this ODD append ≤35 diff lines, NEW `internal/shellinstaller/separate_boundary.go` ≤145 lines and NEW `separate_boundary_test.go` ≤190; max370/hard400. Rollback is deleting only the two NEW Go paths and this append; W18 code, Profile.Validate and CLI gate byte-unchanged.
- Pure value-only `DescribeSeparateBoundary(profile, observedSelectors)` ALWAYS returns a typed `not-authorized` refusal and a fixed nonempty independent-evidence requirement set, even for zero, malformed, forged-positive or clean absolute inputs. Only exact `gentle-shell` command intent is recognized; Stable/Main channel remains orthogonal, never resolved to source bytes or approval. Invalid command/channel also refuse; no empty rejected-selector list, claimed verification string, caller `Approved`/`Ready`, or default grants authority.
- Reject observed 3.7.0 resolver env `GENTLE_SHELL_PI`, unproven optional peer, PATH fallback, `--link`, saved link, path, `--home`, `GENTLE_SHELL_HOME`, inherited `PI_CODING_AGENT_DIR` alias, unresolved-default home, lifecycle postinstall or launcher auto-provision. These are untrusted observations, not physical InstanceID/executable/home/load proof. The 3.7.0 launcher is DATA, not a trusted executable; a syntactically clean absolute dedicated proposal remains refused until independent executable/home/load and source/instance/rollback/Ready evidence exists.
- Preserve Pi takeover as a separate consent-bound mode. No installed launcher reuse, process/FS/environment lookup, path resolution, import of Pi settings, SDD offer/manage, generic install, action, exec, Apply or Ready state. Profile.Validate still rejects gentle-shell and W18 CLI `GateShellInstallCandidate` always refuses.
- Tests are Go SOURCE DATA only: both channels, mode/default invalidity, every selector, claimed positive fields, absolute dedicated proposal and the zero-selector refusal/nonempty requirements. Independent adversarial STATIC exact full candidate; no Go/Node/Pi/Python run/compile, config/Apply/root/network/CI/commit/delivery. This is not an installed app/TUI/CLI route or runtime proof.

## W20 — existing-Pi takeover refusal (source-only)

- Start from CLEAN W19 `208a09cf02d58125806dc1e2765938a70a7a2032` in a dedicated worktree. Only this append (≤25 diff lines), NEW `internal/shellinstaller/takeover_boundary.go` (≤150), NEW `takeover_boundary_test.go` (≤170), and `internal/cli/shell_install_gate_test.go` (≤25); total allocation ≤370, hard cap 400. No commit, Go/Node/Pi/Python/C run, config, Apply, network or CI authority.
- W19's launcher negative examples came from a same-UID-writable LOCAL installation merely labeled 3.7.0. Its metadata differs from public registry `gentle-pi@3.7.0`; the approved raw tgz was never inventoried. Do not attribute local launcher branches or dependencies to the registry release.
- Pure takeover refusal requires an EXPLICIT `pi` command (legacy zero is not consent). Stable/Main are syntax only; Main remains STOP. Existing physical Pi executable/home InstanceID, exact action and source binding, fresh human consent, audited rollback snapshot, drift and executed-byte checks, and independent Ready all remain missing. A caller's claimed IDs, digests, approvals or empty rejection list are never evidence.
- W18 CLI gate and `Profile.Validate` remain unchanged and fail closed for actual shell execution; the generic Pi companion-package `RunInstall` is not a takeover route. Separate Gentle-Shell W19 remains untouched. Tests are SOURCE DATA pending an independently authorized runner and adversarial STATIC review; rollback only these exact W20 files/addendum.
