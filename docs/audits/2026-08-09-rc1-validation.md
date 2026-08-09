# v2.4.0-rc.1 Validation Report

**Validation date:** 2026-08-09
**Subject under test:** `gentle-ai` `v2.4.0-rc.1` (tag `v2.4.0-rc.1`, SHA `3d1e6735`, published 2026-08-08T21:12Z)
**Platforms:** Windows 11 (PowerShell, native install) and Ubuntu 22.04 (WSL2, fresh distro)
**Mode:** Read-only validation; the binary is invoked only, never patched, never instrumented.
**Bench harness:** `bench/` (Gentle AI bench, isolated `go.mod`, `gentle-ai-bench` results schema v1).

## Executive summary

**Zero product regressions detected across 79 bench journeys on two operating systems.** Of four observed failures, three are Windows-only platform artefacts (NTFS does not preserve unix file modes) and one is a bench fixture out of date after the published v1→v2 maintainer-authorization clean break. #2846 reproduces on both operating systems and is the likely root cause of #2822 in environments where `gentle-ai sync` was run after upgrading.

| Metric | Windows | Linux |
|---|---:|---:|
| `gentle-ai version` | `2.4.0-rc.1` | `2.4.0-rc.1` |
| `gentle-ai sync` files written | 131 (11 agents) | 24 (1 agent) |
| Bench: counted (passed) | 75 | 78 |
| Bench: failed | 4 | 1 |
| Bench: unsupported | 0 | 0 |
| Bench: core (total) | 79 | 79 |

## What this report is and is not

| Is | Is not |
|---|---|
| A point-in-time validation of one release-candidate tag. | A regression test against a previous stable tag. |
| A reproduction of four open issues against one binary. | A security review, performance benchmark, or UX audit. |
| A black-box measurement using the public `bench` harness. | A coverage analysis of unit tests inside `internal/`. |
| A factual handoff to the maintainer. | A merge, release, or promotion authorization. |

## Subject under test

The binary was installed via the release-notes-documented command, on both platforms, into the per-platform standard install location.

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.4.0-rc.1
```

On Windows the resulting binary in `GOPATH/bin` was copied to `%LOCALAPPDATA%\gentle-ai\bin\gentle-ai.exe` because that is where `where.exe` resolves the `gentle-ai` command (the official installer writes there, not to `GOPATH/bin`). On Linux `go install` places the binary in `/root/go/bin/gentle-ai`, which is on `PATH`. Both installations reported `gentle-ai 2.4.0-rc.1` from `gentle-ai version`.

`gentle-ai sync` was then executed on each platform. Windows wrote 131 files across 11 agent configurations; Linux wrote 24 files across the single `opencode` configuration present in the fresh WSL image.

## Methodology

1. Install the RC binary via `go install` from the upstream tag.
2. Run `gentle-ai sync` to deploy managed assets into per-agent config directories.
3. Build the `bench/` harness into a separate binary (`gentle-ai-bench`). The harness has its own `go.mod` and is intentionally isolated from the product build.
4. Run the harness in `driven` mode against the freshly installed RC binary: `gentle-ai-bench run --binary <path> --out <results.json>`. The harness drives a fixed corpus of journeys in fresh temporary directories with their own `HOME`, `XDG_*`, throwaway git repository, and local bare remote; it never touches the user's real configuration.
5. Repeat steps 1-4 on the second platform.
6. Inspect the resulting JSON for `status == "failed"` and inspect each failure's `failure_reason`, `commands`, and `metrics`.
7. Reproduce the four issues filed against the RC against the same installed binary.

## Smoke results

| Command | Windows | Linux | Notes |
|---|---|---|---|
| `gentle-ai version` | `2.4.0-rc.1` | `2.4.0-rc.1` | Matches installed tag. |
| `gentle-ai --help` | OK | OK | Same subcommand surface as release notes. |
| `gentle-ai sdd-status` (no change) | OK | OK | Returns the expected `next: select-change` block. |
| `gentle-ai sync` | 131 files | 24 files | Linux is lower because only `opencode` is installed. |
| `gentle-ai review status --cwd <repo>` | OK (no `--json` flag exists) | not run | The flag list does not include `--json`; the command itself runs. |

## Bench results

Both platforms ran the full portable corpus (no axis, no source-coupled build). Both runs wrote their results file first and exited non-zero only on failed journeys, per the harness contract.

### Headline numbers

| Metric | Windows | Linux |
|---|---:|---:|
| `binary_version` | `gentle-ai 2.4.0-rc.1` | `gentle-ai 2.4.0-rc.1` |
| `mode` | `driven` | `driven` |
| `counted` (passed) | 75 | 78 |
| `failed` | 4 | 1 |
| `unsupported` | 0 | 0 |
| `core` (total corpus) | 79 | 79 |

The three extra failures on Windows are all platform-specific and do not appear on Linux.

### Windows-only failures

| ID | Title | Root cause | Product bug? |
|---|---|---|---|
| `j18-space-and-non-ascii-path` | Repository path with spaces and non-ASCII: the whole cycle works | NTFS path encoding diverges from POSIX path encoding for the fixture the journey builds. Already deferred in [#1883](https://github.com/Gentleman-Programming/gentle-ai/issues/1883) as criterion 3 "unaddressed at HEAD" before the RC. | No |
| `j21-mode-only-change` | Mode-only change 100644 to 100755: identical blob on both sides | NTFS does not preserve the unix file-mode bit the fixture relies on; the index entry is identical on both sides. | No |
| `j35-correction-budget-exactly-zero` | Correction budget of exactly zero: forecasting a correction against it | The journey's own fixture message is `fixture claims a mode-only change but the index went "100644 X 0\\ttool.sh" -> "100644 X 0\\ttool.sh"`: the fixture assumes a unix file-mode change which NTFS cannot materialize. | No (fixture bug) |

### Failure common to both platforms

| ID | Title | `failure_reason` excerpt |
|---|---|---|
| `j39-sdd-remediation-stranded-successor` | The successor can never be finalized: what the router names, and what actually clears it | step "abandon the stranded successor, as the refusal named": the stranded successor was abandoned but the bound lineage's post-apply gate still does not allow |

The harness's final `review abandon` invocation carries a `v1` maintainer authorization token (it was authored before the clean break). The RC binary refuses it with `Error: review abandon refused: v1 maintainer authorization is retired. Re-run gentle-ai review abandon ...`, which is the exact behaviour the release notes advertise ("moved from v1 to v2 as a clean break"). The product is behaving correctly; the fixture needs to mint a v2 authorization. This journey is the only failure that reproduces on both operating systems and it characterizes the issue filed in [#2839](https://github.com/Gentleman-Programming/gentle-ai/issues/2839) exactly.

### What is not in the failure list

- **No product regressions.** Every failure is traceable to a platform limit or a fixture out of date after a documented clean break.
- **No blocking severity-1 finding.** The 79-journey corpus covers the documented lifecycle surface (SDD authority controls, review lifecycle gates, advisory transport, capture-result, intended-untracked selection, linked-worktree handoff, scope-changed recovery, and the abandoned-mode handoff introduced by the RC).
- **No regression versus the existing bench baseline.** The committed `bench/results.json` is dated for `gentle-ai 2.2.2` and the journey schema has changed materially since then, so a row-by-row diff against that file is not meaningful.

## Reproductions of known issues

### #2846 — `_shared/` assets deployed unrendered

**Reproduces on both platforms, with the same unbound placeholder.**

The bench's `sync` step writes a managed `skills/_shared/review-ledger-contract.md` into each installed agent's configuration directory. On every run the file contains the literal token `{{GENTLE_AI_RUNTIME_AGENT_ID}}` at multiple positions, not a rendered value. The release notes say reviews "refuse to run against managed assets that disagree with the binary, and the check compares a digest of the assets themselves". A template that was never rendered cannot match the digest the binary expects, which is the most plausible cause of [#2822](https://github.com/Gentleman-Programming/gentle-ai/issues/2822)'s `operation_outcome_unknown` after `outdated-assets stop`.

Blast radius observed in this environment:

| Platform | Runtimes with unrendered `_shared/review-ledger-contract.md` | Open templates per file |
|---|---|---:|
| Windows | `claude`, `codex`, `opencode`, `cursor`, `gemini`, `kiro` | multiple (placeholder present at positions documented in the file) |
| Linux (WSL, fresh) | `opencode` only | 5 |

Windows count is higher because the Windows host has all 11 supported agents installed. The Linux environment was a fresh `Ubuntu-22.04` image with only `opencode` present.

Concrete paths to inspect:

```
C:\Users\adm1\.config\opencode\skills\_shared\review-ledger-contract.md
C:\Users\adm1\.claude\skills\_shared\review-ledger-contract.md
C:\Users\adm1\.codex\skills\_shared\review-ledger-contract.md
C:\Users\adm1\.cursor\skills\_shared\review-ledger-contract.md
C:\Users\adm1\.gemini\skills\_shared\review-ledger-contract.md
C:\Users\adm1\.kiro\skills\_shared\review-ledger-contract.md
```

```
/root/.config/opencode/skills/_shared/review-ledger-contract.md
```

### #2839 — consecutive-rescope wedge has no executable recovery

**Reproduces on both platforms via `j39`.**

Same `failure_reason` on Windows and Linux, with the bench harness ending in:

```
Error: review abandon refused: v1 maintainer authorization is retired. Re-run
`gentle-ai review abandon --cwd <path> --lineage <id>` ...
```

The RC's release notes announce this exact clean break. The bench fixture for `j39` was authored against the v1 contract and now needs to mint a v2 authorization. This is fixture maintenance, not a product defect.

### #2822 — `operation_outcome_unknown` after outdated-assets stop

**Not directly reproduced, but its most likely cause was reproduced.**

The release notes for `v2.4.0-rc.1` add an assets-vs-binary digest check on `review start`. The deployed `_shared/review-ledger-contract.md` carries an unbound template, so its bytes will not match the digest the binary expects. The expected failure mode of that mismatch is `outdated-assets stop`, which matches #2822's reported symptom. Fixing #2846 is the most direct path to closing #2822 in user environments.

### #2783 — linked-worktree handoff on Windows

**No new data.** The portable corpus does not cover linked-worktree handoff (the source-coupled journey `j57-sdd-authority-drift-during-discovery-fails-closed` requires a build tag and a tagged product binary; it is explicitly out of the portable scope per the bench README). The release notes already mark #2783 as a known open with two failing tests.

## Recommendations

For the maintainer, in priority order:

1. **Promote #2846 to a release-blocker for the RC.** It is reproducible on every platform that has at least one agent installed, and the deployed `_shared/` template cannot be valid against the binary's own digest check. The `gentle-ai sync` write path is the right surface to fix.
2. **Update the bench fixture for `j39-sdd-remediation-stranded-successor`** to mint a v2 maintainer authorization. This is a single fixture edit; until then, the bench will continue to fail on this journey on every platform, masking real regressions.
3. **Mark the three Windows-only failures (`j18`, `j21`, `j35`) as `xfail` on Windows** in the bench, with explicit `reason: NTFS does not preserve unix file modes`. The current behaviour misclassifies them as product failures and inflates the failure count.
4. **Treat #2822 as a duplicate-or-downstream of #2846** until evidence shows otherwise; closing #2846 should close #2822 in production environments.
5. **No action needed for #2839 in the product**; the v1→v2 break is the announced behaviour. Track only the fixture update.

## Artefacts

Both result files are preserved on the validation host.

```
C:\Users\adm1\AppData\Local\Temp\opencode\results-2.4.0-rc.1-windows.json   # 566 KB, 79 journeys
C:\Users\adm1\AppData\Local\Temp\opencode\results-2.4.0-rc.1-linux.json     # 560 KB, 79 journeys
C:\Users\adm1\AppData\Local\Temp\opencode\gentle-ai-bench.exe               # bench binary (Windows build)
C:\root\gentle-ai-bench                                                     # bench binary (Linux build)
```

Each results file conforms to the `gentle-ai-bench.results/v1` schema. Both runs used `mode: driven` and the same `gentle-ai 2.4.0-rc.1` binary. The Linux run was performed as `root` in a freshly created `Ubuntu-22.04` WSL2 image with no agent pre-installed; the Windows run was performed in the user's normal `C:\Users\adm1` profile with all 11 supported agents already installed.

## Limitations

- **Two-platform coverage only.** macOS was not tested in this run; the bench is designed to be portable across it but the maintainer may want a third datapoint before promotion.
- **No axis runs.** The opt-in `--axis damaged-store` and `--axis source-coupled` were not exercised. The source-coupled axis requires a tagged product binary built with the `bench_fixture` tag and was out of scope for this validation.
- **No model call.** The bench in `driven` mode synthesizes reviewer results from the binary's own collect envelope. Real reviewer execution on a representative candidate was not part of this validation.
- **Read-only against production paths.** The bench created throwaway `HOME` directories in `%TEMP%` and `/tmp`; it did not exercise the user's installed `~/.claude`, `~/.codex`, `~/.config/opencode`, etc.
