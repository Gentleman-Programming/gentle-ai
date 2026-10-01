# Durable Go installer tasks

## Current CodeRabbit follow-up — PRs #4694 and #4695

Tracker: `odd/tasks/durable-go-installer.md`; full mirror #4715, topic `odd/durable-go-installer/tasks`, project `gentle-ai`. Current status supersedes history below. Never stage `odd/`.

Current authority: bookkeeping only. Parent completed both user-approved upstream merges and separately consented native RDD acknowledgments. Merges remain LOCAL; Windows fixes uncommitted. No delivery approval or maintainer exception. Next: maintainer exception, then authorized publication. No source/Git/network/test/review/delegation operations in this update.

- [x] DGI-07 — Nightly-only proof: five variants passed, zero skips; reply `4034151175` GET-verified. Bot withdrew false-positive `4030505544` in `4034156433`; all Unix threads resolved. Assertions retained.
- [x] DGI-08 — Fixed Windows findings `4030490967`, `4030490975`, `4030490987`, `4030491001`: executable lookup, location-independent guidance, legacy TLS 1.2, sanitized HTTP diagnostics. Full writer receipt below; fixes remain uncommitted.
- [ ] DGI-09 — Await **MAINTAINER APPROVAL / size:exception**, requested in GET-verified `5710568784`; 421-line Windows candidate unchanged. Four Windows threads await published fixes. Missing `type:*` is separate; no approval or bot acceptance claimed.
- [x] DGI-10 — Conflict-free integrations, signed-off/SSH-signed. Pre-merge full readback 49,084/49,083 bytes. `merge.ff=only` refused; `--no-ff --no-commit` succeeded. Index: upstream README/two SVGs (+157/-84) only. Receipt below.
- [x] DGI-11 — Parent-supplied native RDD receipts: four lenses each, no findings; separate explicit user consents. Both returned `gentle-ai.review-acknowledged/v1`, action `acknowledged`, authority `burned`. No correction or extra edits; acknowledgment is NOT delivery approval. RDD **on/global** by explicit user choice; clone-local off cleared.
  - Unix lineage `review-48f96310dc1c0835`; target `sha256:0fcf1c5c8fb5cf8d4462771fc0ae3300937448d219aa1456e6dd0cfd2e4a4735`; consumed revision `sha256:d5c7199d45f531f5d5ffda54f86f9452cb7c9ee7e05b760e9945c555dfd88afc`. Provider base `2594581` predates upstream: 610 lines/6 files, NOT actual PR 288.
  - Windows lineage `review-7782ab34ae853adc`; target `sha256:af97bd4b9a2940284527c7a5bcf67beea50f19499d432aaf376f038fcd66f19e`; consumed revision `sha256:43e6e4159654856b5c8b618db3150a8936cad25f34b14778f1e2c677b57c8a26`. Workspace scope 123 lines/4 files includes user `.gitignore`, NOT permission to commit it. Untracked explicitly excluded.

### DGI-10 integration receipt

- Windows merge `32779712d7706d07e3002fb69debcc96f4e4ca9b`; ordered parents `536cf9c6ce01290deacb9697efcb2df5492a6ee2`, `712ebdc78ebe57005c8f9364e21ed4b2d392ca5b`.
- Unix merge `b7067993870ce452b640f033c65e21ce852906a8`; ordered parents `39bad4aa81cf011e404f98e5779aee1e9447610c`, same upstream. Both ancestry checks passed, indexes empty, Unix clean. Committed installer bytes equal pre-merge parents; imported docs equal upstream. Windows residual scope unchanged.
- Exact offline JSON commands below: Windows 9 top-level/49 subtests, 47.572s; filtered policy 1 test, 0.231s; Unix 8 top-level/31 subtests, 13.691s. All exit 0, zero failures/skips. Check-only gofmt, Windows parser, Unix Bash syntax, scoped diff checks: exit 0, no output. Go 1.27.1/macOS arm64; runtime limitations unchanged.
- Against new upstream: Windows committed +319/-18=337, three-file candidate +400/-21=421; Unix +261/-27=288, two files. No imported docs in PR diff. Both merges local; Windows fixes uncommitted. RDD complete; maintainer exception pending.
- Required SHA-256 before=after: installer test `72d314e155d801b80d6c091bcbb6cbcb0e73897ed8f8b8893cc4a2c90d93657c`; policy `e44f732e5eb295c7791961415bb60456531733497ebc3d229acb6b4670217f9b`; PowerShell `f544686cfa0e48c180749876dee7285c4c68ced3720d1fa540d56951933c183c`; user `.gitignore` `3dcca80c5694450eadfcccf107be9274b18494dec2ad3967e3d304dadb0c3c7c`. Windows Bash and three unrelated markdown hashes also unchanged; `.codegraph/` retained. `odd/` never staged. Discovery #4761; skill_resolution: paths-injected, including git-commit and all five references.

### Authorized edit scope and verification

- Roots: Windows main checkout; Unix sibling `gentle-ai-worktrees/pr4694-unix`. Existing platform branches unchanged. Preserve Windows three-file fix, user `.gitignore`, untracked files; never stage `odd/`. Integration rollback: imported docs only.
- TDD **on**, `openspec/config.yaml:13,43`, runner `go test ./...`; retain historical RED/GREEN. Historical RDD-off receipts remain historical. Commands below are completed merge evidence, not bookkeeping execution authority.
- In each `internal/update/`: `env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -json install_script_test.go`.
- Windows only, mandatory filter: `env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -json -run '^TestWindowsInstallAndUpgradeContainNoRemoteBinaryOrScriptPath$' windows_distribution_policy_test.go release_security_test.go`.
- Merge checks already passed. PS7/macOS doubles do not prove Windows/PS5.1, live TLS/install, or minimum Go. No full suite/build/install. CI limit remains 400; retain assertions.

### DGI-08 writer receipt

- Pre-source gate: actual tracker and full 45,938-byte mirror #4715 read back; decoding the JSON envelope confirmed equality. History retained.
- Changed only the three authorized source/test files. The fixture resolves `go` with `exec.LookPath`, makes its path absolute before restricting child PATH, and runs from a separate temporary working directory. Missing Go explicitly skips. Guidance no longer assumes a local script filename. Production supplements explicit legacy protocol sets with TLS 1.2, preserves default/modern settings, and provides narrow sanitized request diagnostics; metadata remains inside its cleanup boundary.
- RED, before production edits: from `internal/update/`, use the exact offline prefix above with `/opt/homebrew/bin/go test -count=1 -v -run '^TestWindowsInstallScript(HTTPSProtocols|GoFailures)$' install_script_test.go`. Exit 1, 31.884s: two top-level failures; 26/33 subtests passed, seven failed (four TLS scenarios, three HTTP diagnostics), zero skips. The exact filtered policy command above failed as expected: exit 1, 0.214s, one failing test. No RED is claimed for the fixture-only lookup correction.
- GREEN: identical focused command exited 0, 31.668s, two top-level tests and 33/33 subtests passed, zero skips. The filtered policy command exited 0, 0.159s, one test passed, zero skips.
- Final normalized-byte checks: `/opt/homebrew/bin/gofmt -w internal/update/install_script_test.go internal/update/windows_distribution_policy_test.go` preceded final verification. Full installer command above exited 0, 48.190s: nine top-level tests and 49 subtests passed (15 channels, 12 TLS, 21 failures/refusals, one repository-module case), zero skips. Filtered policy command exited 0, 0.176s, one test passed, zero skips. Check-only gofmt was empty; the exact PowerShell parser command in DGI-06 and scoped `git diff --check -- scripts/install.ps1 internal/update/install_script_test.go internal/update/windows_distribution_policy_test.go` exited 0.
- Go 1.27.1/PowerShell 7.6.6 on macOS: both HTTP doubles assert stable/beta TLS state/order for default, legacy, TLS12, TLS13-only, and mixed sets. HTTP failures assert sanitized context/guidance, no install, cleanup, and restored environment. TLS-configuration catch inspected, not fault-injected.
- Delta versus prior HEAD: +100/-22 = 122. Full PR versus `9bf454d4d40803635bd302451ec8ed08d78bd1f4`: +400/-21 = 421 (tests +321/-8; policy +1/-1; PowerShell +78/-12), previously 337. Preserve this three-file delta; it is the rollback boundary.
- Writer retained HEAD/branch, empty index, `.gitignore`, Bash, unrelated untracked files, and `.codegraph/`; no remote/build/install/review/commit/push/delegation. Writer `skill_resolution: paths-injected`; bugfix #4757. Current handoff: DGI-07/08 complete, DGI-09 delivery blocked as above. Never stage `odd/`.

## Historical independent-platform delivery handoff

Current handoff: DGI-05 and DGI-06 are committed as TWO independent platform units after parent readback and explicit branch/extraction/test/commit authorization. Unix `1e3856ee4715b33fbe7f98e246d79c5d04799f1a` is 271 changed lines; Windows `536cf9c6ce01290deacb9697efcb2df5492a6ee2` is 337. Both directly parent local upstream `9bf454d4d40803635bd302451ec8ed08d78bd1f4` and target `main`, no stack or size exception. Current branch is `fix/install-scripts-dynamic-module-windows`; merged branch `fix/install-scripts-dynamic-module` remains at `a5260ca69deb18a2fc3cbb87d8ade3eda3996795`. Parent independent checks and remote issues/PRs/pushes remain pending. No remote operations, application builds, real installs, or nested delegates ran. `odd/` must NEVER be staged or committed. Earlier sections retain historical evidence, not current authorization; this independent-PR strategy supersedes the earlier advisory cap, combined delivery, and tracker-staging instructions.

## Current independent-platform delivery plan

### Verified merged baseline and scope

- Local HEAD has parents `4c9d8d442f96a85e285ce4ea535e43f7ccb8e840` and `9bf454d4d40803635bd302451ec8ed08d78bd1f4`. Original Unix unit is `51f0632f7492d84141b806e3a75a0f65c074ea36`; original Windows unit is `4c9d8d442f96a85e285ce4ea535e43f7ccb8e840`. Both production scripts are byte-identical between their original platform commit and merged HEAD. Preserve those bytes in the appropriate slice; no demonstrated production change is needed.
- Actual merged diff against the base: `internal/update/install_script_test.go` +483/-130; `internal/update/windows_distribution_policy_test.go` +0/-21; `scripts/install.ps1` +53/-9; `scripts/install.sh` +34/-18. Total: 748 authored changed lines. Original commit totals were 282 Unix and 391 Windows, but those are not independent PR totals against the new base.
- CodeGraph inspected the Go test files and helper dependencies; local Git supplied historical source and unsupported shell-script diffs. The actual common regression is `TestInstallScriptsGoInstallPackageMatchesModuleMajor`, not `TestInstallScriptsUseCurrentGoModule`. The base checks both platform package constructors plus stale-major/comment guards; the merge changed it to invoke both fixtures. A platform slice must not import the other platform's fixture merely to satisfy this test.
- Construct each candidate from the same base, not from the combined branch and not by stacking/cherry-picking the entire Windows commit. Keep all six base top-level installer tests, including the old environment-helper regressions. Change only the selected platform's three dynamic module call expectations. Preserve its clobber guards and helper assertions, including Unix nonempty GONOSUMDB preservation, which the new whole-script fixture does not separately cover. Retain the opposite platform's base tests and production file byte-for-byte.
- Add the selected platform's existing two behavioral tests and fixture as an unchanged contiguous source segment. No new framework, cosmetic line compression, assertion deletion, code/test split, or cross-platform fixture duplication. Keep the shared major test's table and static guards; adapt only the selected table row to its dynamic constructor, then execute that platform's fixture using repository go.mod and assert the exact pinned target and one install. Put the runtime assertion inside `t.Run(tc.script, ...)` so an unavailable interpreter skips only that subtest, not the remaining static checks. Retain the other table row's base behavior. Correct the shared test comment accordingly.
- In-memory candidates were formatted through gofmt stdin/stdout only. Python SequenceMatcher with autojunk disabled estimates +211/-8 installer-test lines for Unix and +265/-8 for Windows. Including exact production diffs and the Windows one-line policy expectation replacement gives approximately **271 Unix / 337 Windows** changed lines. These are estimates, not Git PR receipts: calibration on the merged test yielded +493/-140 versus Git's +483/-130. Final per-branch `git diff --numstat` is authoritative and must remain <=400.
- Source integrity targets: Unix SHA-256 `3ab9b1b16066db28263874c604f50135f0927de1c9554a4f02ce6165f7c9e84a`; Windows SHA-256 `2697cfe4a817c9a8b66ca7d77070ffb99ab75a9246d495e89e0746c935797b63`.
- At preparation start there were no tracked edits and four unrelated untracked entries. A concurrent `.gitignore` change adding `odd/` appeared later; this preparer did not make it. Preserve it and let the parent own its separate disposition; do not add it to either platform slice. `git check-ignore` now reports `.gitignore:40:odd/`; the tracker remains absent from the index. Preserve all four unrelated untracked entries.

### [x] DGI-05 — Independent Unix slice and focused proof

Status: complete after explicit parent readback and authorization. Commit `1e3856ee4715b33fbe7f98e246d79c5d04799f1a` on `fix/install-scripts-dynamic-module-unix`, direct parent `9bf454d4d40803635bd302451ec8ed08d78bd1f4`. Actual Git total **271** (+245/-26): installer tests +211/-8; Bash +34/-18. Exactly the approved two files; tracker and `.gitignore` were not staged.

Execution receipt: targeted gofmt completed before checks. The exact full-file offline command below exited 0, `ok command-line-arguments 14.290s`: 8 top-level tests, 30 platform scenarios and one repository-module subtest passed, zero skips. Bash syntax, ShellCheck, gofmt -l (empty), working/cached diff checks exited 0. Byte comparison confirms the entire three-function Unix fixture/test segment equals merged HEAD, production Bash equals original/merged source, unchanged Windows tests and production remain at base, and every base top-level test remains. The retained Unix helper test still asserts nonempty GONOSUMDB preservation. No fresh RED was invented for this extraction.

Before commit, inspected status, scoped full diff, recent ten commits, and staged paths. `git commit -S -s -F -` succeeded with a Conventional Commit subject and contiguous rationale bullets; no hooks/config changes, attribution, or remote operation. `.gitignore` remains unstaged and byte-identical at SHA-256 `3dcca80c5694450eadfcccf107be9274b18494dec2ad3967e3d304dadb0c3c7c`. Merged branch remains untouched. Parent independent verification and publication remain pending. `skill_resolution: paths-injected`: required go-testing, work-unit-commits, branch-pr, git-commit plus its five references loaded; platform guidance loaded. Next: mirror/readback this receipt, then DGI-06 on an independent base branch.

- Branch: `fix/install-scripts-dynamic-module-unix` (35-character slug), independently based on `9bf454d4d40803635bd302451ec8ed08d78bd1f4`, PR target `main`; one Unix-specific issue, approved before PR publication. No issue/PR has been created or remotely inspected here.
- Exact changed files: `scripts/install.sh` and `internal/update/install_script_test.go`. Start the script from merged HEAD/original Unix bytes. Add merged installer-test lines 68-256 (channel cases, failures, runGoInstallerFixture) before the common major-regression comment, preserving the segment exactly. Add runtime import. Keep all base tests; adjust only Unix dynamic helper-call strings and the Unix common-major branch as described above. Do not touch PowerShell or the policy file.
- Preserve all 15 successes and 15 failure cases, pinned metadata/target, one channel resolution/install, beta env preservation/deduplication, stable env, real parser syntax coverage, fail-closed doubles, and cleanup sentinel checks; also run the repository-module runtime regression and retained base helper/source assertions.
- Estimate: tests +211/-8, script +34/-18, total approximately 271. Rollback boundary: this branch's two-file installer/test delta only.

From `internal/update/`, after implementation, run the full file-list suite (all eight top-level tests; 30 platform leaf scenarios plus the repository-module runtime subtest). This also verifies the unchanged Windows source guards without requiring PowerShell execution:

```sh
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -v install_script_test.go
```

From repository root:

```sh
/bin/bash --noprofile --norc -n scripts/install.sh
/opt/homebrew/bin/shellcheck scripts/install.sh
/opt/homebrew/bin/gofmt -l internal/update/install_script_test.go
git diff --check 9bf454d4d40803635bd302451ec8ed08d78bd1f4
git diff --numstat 9bf454d4d40803635bd302451ec8ed08d78bd1f4
```

### [x] DGI-06 — Independent Windows slice and focused proof

Status: complete after explicit parent readback and authorization. Commit `536cf9c6ce01290deacb9697efcb2df5492a6ee2` on `fix/install-scripts-dynamic-module-windows`, direct parent `9bf454d4d40803635bd302451ec8ed08d78bd1f4`; independent of Unix. Actual Git total **337** (+319/-18): installer tests +265/-8, policy +1/-1, PowerShell +53/-9. Exactly the approved three files; no tracker or `.gitignore` staged.

Execution receipt: targeted gofmt of the two intended test files preceded all checks. The exact installer file-list command below exited 0, `ok command-line-arguments 34.739s`: 8 top-level tests, 36 Windows scenarios plus one repository-module subtest passed, zero skips. The exact named original-location policy test command exited 0, `ok command-line-arguments 0.213s`: one top-level test, no other release tests executed, zero skips. PowerShell parser-only syntax, gofmt -l (empty), and working/cached diff checks exited 0. No extraction fix or production change was needed; historical RED evidence remains historical.

Byte comparison confirms the full three-function Windows behavior/fixture segment equals merged HEAD, PowerShell equals original/merged production, Bash and unaffected base tests remain at base, and all six base top-level installer tests remain. The policy file differs from base solely in the guidance string. Status, scoped full diff, staged paths, and last ten commits were inspected before `git commit -S -s -F -`; it succeeded without hook/config changes, attribution, or remote operation. `.gitignore` carried across both switches unchanged and unstaged; merged branch remains intact. Worker leaves Windows checked out. Parent independent verification, issue creation, pushes, and PRs remain pending.

- Branch: `fix/install-scripts-dynamic-module-windows` (38-character slug), independently based on the same upstream commit, PR target `main`; one Windows-specific issue, approved before PR publication. No chained PR or stack.
- Exact changed files: `scripts/install.ps1`, `internal/update/install_script_test.go`, and `internal/update/windows_distribution_policy_test.go`. Start the script from merged HEAD/original Windows bytes. Add merged installer-test lines 286-527 (channel cases, failures, runPowerShellInstallerFixture) before the common major-regression comment unchanged; add encoding/json and runtime imports. Preserve all base tests and adapt only Windows helper-call expectations and the Windows common-major branch. Leave Bash at the base.
- Keep `TestWindowsInstallAndUpgradeContainNoRemoteBinaryOrScriptPath` in its original base policy file. Change only the required guidance string from the v3 go-install command to the raw literal `.\install.ps1 -Method go -Channel stable`. Preserve every forbidden-path and distribution-hold assertion. Do not reproduce merged installer-test lines 258-284 or the 21-line relocation deletion. This eliminates unnecessary relocation and its duplicate reader closure without losing coverage.
- Preserve all 15 channel/major successes and 21 failures/refusals, parser environment restoration for present/absent values and failures, exact metadata/install pinning and counts, parser syntax, cleanup and sentinel contents, binary/insecure holds, plus retained source/helper guards and repository-module runtime coverage.
- Estimate: installer tests +265/-8, PowerShell +53/-9, policy +1/-1, total approximately 337. Rollback boundary: this branch's three-file Windows installer/test delta only.

From `internal/update/`, run the full installer file-list suite and then the original-location guidance contract. The second command compiles the policy and release-security test files to supply `readRepositoryFile` and YAML types, but its exact `-run` selects only the guidance test; do NOT run those two files without this filter because other tests launch release tooling. Static helper inspection found no missing cross-file helper beyond readRepositoryFile and no init functions; YAML v3.0.1 exists in the local module cache. Compilation/execution still needs per-branch verification:

```sh
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -v install_script_test.go
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -v -run '^TestWindowsInstallAndUpgradeContainNoRemoteBinaryOrScriptPath$' windows_distribution_policy_test.go release_security_test.go
```

From repository root:

```sh
/opt/homebrew/bin/pwsh -NoLogo -NoProfile -NonInteractive -Command '$tokens = $null; $parseErrors = $null; $null = [System.Management.Automation.Language.Parser]::ParseFile((Join-Path (Get-Location) "scripts/install.ps1"), [ref]$tokens, [ref]$parseErrors); if ($parseErrors.Count -ne 0) { $parseErrors | ForEach-Object { Write-Error $_ }; exit 1 }'
/opt/homebrew/bin/gofmt -l internal/update/install_script_test.go internal/update/windows_distribution_policy_test.go
git diff --check 9bf454d4d40803635bd302451ec8ed08d78bd1f4
git diff --numstat 9bf454d4d40803635bd302451ec8ed08d78bd1f4
```

### Verification, authority, and next handoff

- `openspec/config.yaml:13,43` still explicitly enables strict TDD; lines 46-48 configure `go test ./...`. This task is mechanical extraction of previously implemented/RED-GREEN-verified behavior, not a new feature. Preserve historical RED/GREEN evidence below, compare assertions and production bytes, and rerun focused checks on each actual branch. Do not manufacture new RED evidence or claim prior combined-branch results prove the independent branches. Broad runner/build/install execution is not authorized by the config.
- Preparation ran only read-only source/history inspection, installed Go help, in-memory gofmt/diff analysis, and tracking updates. No tests, installer/parser execution, application build, install, branch creation, staging, commit, or remote request ran. Installed executable paths resolve to Go 1.27.1, PowerShell 7.6.6, and ShellCheck 0.11.0; prior native Bash baseline is 3.2.57. Proposed file-list syntax and test flags were checked against installed Go help. No new runtime success is claimed.
- Historical runtime evidence is macOS arm64 with Bash 3.2/PowerShell 7, not native Windows/PowerShell 5.1, Linux, minimum Go 1.25.10, or live installation. Missing interpreter/cache remains a reported blocker or skip, never authorization to install dependencies, access the network, or bypass execution policy. Offline Go settings do not make arbitrary subprocesses a security sandbox.
- Both independent branches edit the shared test; they are independently reviewable/buildable candidates, not guaranteed conflict-free merges. When one lands, reconcile the second's common regression to retain both platform checks without copying a second fixture/framework; rerun its focused checks and recount against the then-current target. No third coordination PR or automatic stack is authorized.
- Parent must read back the local tracker and complete #4715 before any source edits, confirm ignored odd never enters staging, and own branch creation/source implementation next. Remote issue/PR authority is separate and remains unused. Preserve the concurrent `.gitignore` change separately. Record exact executed/skipped counts and final Git line counts in DGI-05/DGI-06 before claiming either ready.

## Historical implementation and verification evidence

## Objective, problem, and authorization

Implement one minimal channel-aware Go installer fix with focused regressions. The original installer selected `/v2@latest` or `/v2@main` while beta env exceptions and current `go.mod` named `/v3`; another hard-coded major would repeat the failure.

The parent read back this tracker and full observation #4715 and authorized DGI-01 plus writer verification DGI-02. Single-writer paths: `scripts/install.sh`, `internal/update/install_script_test.go`, and this tracker. Full Engram mirror: project `gentle-ai`, topic `odd/durable-go-installer/tasks`.

No remote requests, real installs, application builds, commits, pushes, nested delegates, or unrelated edits. Focused offline tests may compile their test executable; targeted gofmt and ShellCheck are approved. RDD is off clone-local per parent. Preserve `.codegraph/`, `pr-1280-carrier-comment-draft.md`, `sdd-orchestrator-findings.md`, and `upstream-merge-update-notes.md`. Approximately 400 authored lines is advisory, not a hard cap.

## Chosen implementation

- Stable reuses `get_latest_version` and installs the selected actual tag, never `@latest`. Beta/nightly resolves exactly one `refs/heads/main` with one public canonical HTTPS `git ls-remote --exit-code` call; require a full 40-hex SHA and pin metadata plus installation to it.
- A subshell resolver fetches that tag/SHA's `go.mod` into a real mktemp file, immediately registers EXIT cleanup, and invokes `GOTOOLCHAIN=local GOWORK=off GOFLAGS= go mod edit -json`. A bounded sed extraction reads canonical `Module.Path`; stdout contains only the resolved path.
- An anchored literal repository allowlist accepts the root or canonical `/vN`, N >= 2, without regex interpolation, guessed-major loops, jq, or a custom go.mod parser. Install `${module}/cmd/gentle-ai@${version}`.
- Beta env exceptions use the exact discovered module and preserve existing values/avoid duplicates through the existing helper. Stable env, Homebrew/binary/PowerShell behavior, channel routing, destination checks, and install-error propagation remain unchanged.
- Bash 3.2 remains supported. Resolver-only toolchain restrictions do not change the real install's normal toolchain selection. Stable tags are assumed immutable conventional release refs; external tag retargeting, unknown future Go grammar, and changed module layouts are not guaranteed. Existing go-install restrictions on target replace/exclude directives remain outside scope.

## TDD source and evidence baseline

- Strict TDD explicitly enabled in `openspec/config.yaml:13,43`; configured runner `go test ./...` at lines 46-48, also CI line 58. Not inferred from framework presence. Broad tests are outside this focused authorization.
- `go.mod` declares `/v3`, requires Go 1.25.10. Local inspected Go: `/opt/homebrew/bin/go`, 1.27.1 darwin/arm64. Native `/bin/bash`: 3.2.57; PATH otherwise selects Homebrew Bash.
- Initial branch `fix/durable-go-installer`, HEAD `2594581e1ca227b6dea85a2861dd03e6d427974a`; only the four unrelated untracked entries above. Parent reported revert complete and configured-SSH upstream refresh already up-to-date. No delegate remote freshness claim.
- Preparation inspected original `scripts/install.sh:269-327,334-361,597-603`, Go test `:66-129`, go.mod, config, and CI. CodeGraph read the Go test; Bash Read followed reported unsupported-file CodeGraph failures. Original Bash regression asserted literal v3 strings and ran only the env helper.
- Local read-only `go mod edit -json go.mod` and bounded extraction succeeded. `/dev/stdin` failed with `RLock /dev/stdin: operation not supported`, motivating a real temporary file. Preparation did not run tests, builds, installs, or remote requests. Parent subsequently authorized implementation and tests.

## Stable tasks and acceptance

### [x] DGI-01 — Coherent fix and regressions

Status: complete. Code and tests form one deliverable/rollback unit: only the Go installer changes and Bash-focused tests, not other installer methods.

- Whole-script `/bin/bash` tests use temporary HOME/TMPDIR, an allowlisted PATH, and fail-closed git/curl/go-install/verification doubles; only the offline Go metadata parser executes for real. No production sourcing guard or general test framework.
- 15 success cases: stable/beta/nightly crossed with root, v3, v4, v5, v12. Assert exact version/module, one beta ref lookup, pinned SHA metadata, exact beta env preservation/deduplication, unchanged stable env, and cleanup. A second git call would return another commit.
- 15 failure cases: missing/duplicate/malformed module; lookalike repository/regex dots, trailing segment, noncanonical major; missing/wrong/malformed/duplicate ref; git/download/parser/install failure. Invalid resolution never installs. Temp cleanup preserves an unrelated sentinel.
- Real parser receives comments, blank lines, quoted module/trailing comment, and `go 1.99.0` on all success cases. Unknown future syntax is not guaranteed.
- RED before installer edit: focused command exited 1; 29/30 leaf cases failed, only existing install-error propagation passed. Calls showed `/v2/cmd/gentle-ai@latest` and `@main`, missing pinned metadata, invalid metadata reaching install.
- GREEN: same command exited 0, `ok command-line-arguments 13.535s`; 2 top-level tests and all 30 leaf cases passed.
- Intermediate harness corrections: line-anchor git call counting (avoid `.git ` URL substring); fixture-local telemetry off-mode file and `TEST_TELEMETRY_DIR` after a Go sidecar raced TempDir cleanup. Local installed Go source confirmed the hook. No user telemetry settings changed; no production correction was needed after first edit.

### [x] DGI-02 — Final writer verification and handoff

Status: complete. Targeted gofmt preceded final checks. Initial ShellCheck identified SC1011 (adjacent regex/ANSI quoting) and SC1007 (empty assignment spelling); split SHA validation from exact-ref equality and spelled the parser GOFLAGS value explicitly empty. No behavior expansion. Parent independent spotcheck remains pending.

## Exact commands and results

From repository root: `gofmt -w internal/update/install_script_test.go` exited 0 before final verification. `shellcheck scripts/install.sh` with ShellCheck 0.11.0 exited 0 after the two targeted corrections.

From `internal/update/`, run the approved commands (the focused test command is also the RED/GREEN command):

```sh
/bin/bash --noprofile --norc -n ../../scripts/install.sh
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -v -run '^TestInstallScript' install_script_test.go
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -v install_script_test.go
git diff --check
```

Final observed results in command order: syntax exit 0; focused exit 0 (2 top-level tests, 30 leaf cases, 16.410s); full-file exit 0 (6 top-level tests, 30 leaf cases, 14.844s, including 4 unchanged PowerShell source regressions); diff-check exit 0. No skipped tests. The native Bash 3.2 whole-script fixtures exercised the runtime boundary; no live install was performed.

Final tracked diff: 212 insertions, 70 deletions (282 authored lines), only the installer and focused test file. Tracker is an additional untracked document; total including its 66 lines is 348, below the advisory 400. Status retained the four unrelated untracked entries unchanged. Installer changes are limited to install_go and its new 16-line resolver; no other install method was edited.

Keep helpers self-contained: file-list tests compile only this standard-library test file, not the update package. Offline Go env prevents Go downloads, not arbitrary subprocess networking; command doubles provide the intended local test boundary, not a security sandbox. Real network installation is deliberately unproven.

## Progress and next step

Preparation/readback and writer checks complete; 2/2 tasks complete. Tracker and full mirror read back at both task boundaries. Independent verification and parent spotcheck are complete (below). No application build/install or publication is part of completion.

## Independent verification and final parent check

- Independent verifier ran the full-file offline test command above without `-v`: exit 0, `ok command-line-arguments 12.537s`; Bash syntax and `git diff --check` also exited 0. No deterministic in-scope regression or unnecessary production expansion found.
- Parent reran `/bin/bash --noprofile --norc -n scripts/install.sh` and `git diff --check`: exit 0. Final status preserves unrelated files and limits implementation edits to the two authorized files.
- Native risk assessment was unavailable because untracked files lacked an explicit declaration. RDD remained off; no review lifecycle or approval was claimed. The unassessable verification path used the independent verifier instead.
- Execution proof is macOS arm64, Bash 3.2.57, Go 1.27.1. Linux and the repository-minimum Go 1.25.10 remain untested. The fixture's `TEST_TELEMETRY_DIR` is an internal Go test hook; real installation and unknown future contracts remain unproven.
- Next: user review of the uncommitted fix. No commit or push performed.

## PowerShell continuation: approved local scope, pending implementation

Parent-retrieved PR #4683 comments: `r4027657171` (Bash package/env mismatch) is addressed by the committed durable fix; `r4027657105` (PowerShell counterpart) remains. Local preparation verified branch `fix/durable-go-installer` and the commit above. The user approved fixing both locally; parent reads back this tracker and full mirror #4715 before source implementation. No delegate fetched comments or remote state.

Preparation may update only this existing tracker and its full Engram mirror. Subsequent single-writer implementation roots are `scripts/install.ps1`, `internal/update/install_script_test.go`, and `internal/update/windows_distribution_policy_test.go` (only the guidance contract test relocation described below). Preserve all completed Bash behavior and the user-owned residual `scripts/install.sh` formatting byte-for-byte. No remote requests, real installs, application builds, commits, pushes, nested delegates, native review, or new plan files. RDD remains off. Broad package tests are not authorized merely because config names `go test ./...`.

### Focused findings and minimal design

- `scripts/install.ps1:32` hard-codes v3 stable guidance; `Install-ViaGo:89-121` selects v2 with `latest`/`main`, while `:100-104` uses v3 env exceptions. `Main:195` already maps nightly to beta. Keep that routing, `Add-GoEnvPattern:123-138`, destination checks, and the Windows distribution hold intact.
- Resolve stable with native `Invoke-RestMethod` against canonical GitHub `releases/latest` and use its actual `tag_name`; beta resolves canonical `commits/main` once and requires one scalar full 40-hex SHA. Validate the release tag before constructing metadata URLs/install operands. Fetch that exact tag/SHA's `go.mod` with native `Invoke-WebRequest -UseBasicParsing -OutFile` into a real temporary file. No git prerequisite, guessed major, remote-script execution, or new dependency. These are planned production calls, not authorization to execute them during this task.
- Parse only with `go mod edit -json <temp-file>` and `ConvertFrom-Json`; check native exit status before JSON access. Accept `Module.Path` only as the canonical literal repository root or `/vN`, N >= 2, using case-sensitive whole-string anchors and no regex interpolation. Use that same module/version for `go install` and the three beta env exceptions; stable preserves existing exceptions.
- Save process `GOTOOLCHAIN`, `GOWORK`, and `GOFLAGS`, set parser-only local/off/empty values, and restore original values including absence in `finally`, before installation. Always clean the owned temporary file, including metadata/parser failures, without removing unrelated files. Preserve normal install toolchain selection. PowerShell 5.1-compatible syntax and ASCII/no-BOM remain required; avoid PS7-only JSON switches or syntax.
- Replace the fixed-major stable guidance with local ` .\install.ps1 -Method go -Channel stable` guidance (without the leading space in the actual command), preserving all distribution-hold/refusal text. This points to the already obtained local installer, never a download/evaluation one-liner.
- `internal/update/install_script_test.go:257-307` is a literal-v3 source assertion, not PowerShell execution proof. Replace that assertion with self-contained offline PowerShell behavior fixtures; retain ASCII/BOM/string checks and all Bash cases. Invoke installed `powershell` on Windows when available, otherwise `pwsh`, with `-NoLogo -NoProfile -NonInteractive`; skip explicitly if neither exists, never install a runtime or bypass execution policy. Use temporary homes/metadata and native PowerShell command doubles for HTTP, install, `go env`, and Windows-only `chcp`; allow only the real offline Go metadata parser through the Go double. Do not execute a real installed application. Unknown mock calls fail closed. Exercise whole-script channel routing, not a hand-copied resolver, without a production test guard or general mock framework.
- Necessary third root: `windows_distribution_policy_test.go:60-79`, `TestWindowsInstallAndUpgradeContainNoRemoteBinaryOrScriptPath`, requires the v3 literal at line 73. Move only this focused test into the self-contained installer test file, using its own standard-library file reads, preserving the forbidden binary/remote-script assertions and updating only the installer guidance expectation. The original file imports YAML and relies on `readRepositoryFile` in `release_security_test.go:16`; moving the small contract test avoids broad package/dependency execution just to verify source guidance. `check_test.go:1220` tests updater text, not the installer; no edit needed there or in updater production code.

### [x] DGI-03 — Durable PowerShell fix with offline regressions

Status: complete after explicit parent readback and implementation authorization. RED preceded production edits and GREEN followed. One deliverable/rollback unit comprises only the PowerShell resolver/guidance changes and their focused tests, including the policy-test relocation; no Bash rollback.

Acceptance:
- Stable, beta, and nightly resolve root/v3/future canonical majors from the actual parsed module, pin metadata and install to the same tag/SHA, and perform one channel resolution/install. Include a moving-main double so duplicate resolution cannot silently pass.
- Exercise real parser syntax with leading comments/blank lines, quoted module/trailing comment, and a higher `go` directive without toolchain download. Assert exact beta env preservation/deduplication, stable env unchanged, and parser-only environment restoration before mocked install.
- Missing/malformed release or SHA, HTTP failures, missing/duplicate/malformed module, lookalike/extra-segment/noncanonical-major paths, parser failure, and install failure fail closed. Invalid resolution never installs; install failures remain failures. Owned metadata is cleaned and an unrelated sentinel survives on success and failure.
- Legacy binary and insecure requests remain refused, without HTTP/install calls; no remote script/binary guidance is introduced. Test fixture uses no Bash prerequisite on Windows. Existing source-format guards and Bash cases still pass.
- Record exact RED/GREEN command, exit status, actual executed/skipped cases, and any harness corrections. No passing result is claimed in preparation.

### [x] DGI-04 — Focused verification, preservation, and handoff

Status: writer and parent verification complete. From `internal/update/`, the exact RED/GREEN and full-file commands are:

```sh
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -v -run '^TestWindowsInstall' install_script_test.go
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 install_script_test.go
```

From repository root, after targeted Go formatting of the two authorized test files, check parser syntax without executing the installer and verify whitespace:

```sh
/opt/homebrew/bin/pwsh -NoLogo -NoProfile -NonInteractive -Command '$tokens = $null; $parseErrors = $null; $null = [System.Management.Automation.Language.Parser]::ParseFile((Join-Path (Get-Location) "scripts/install.ps1"), [ref]$tokens, [ref]$parseErrors); if ($parseErrors.Count -ne 0) { $parseErrors | ForEach-Object { Write-Error $_ }; exit 1 }'
git diff --check
```

Require proof that the relocated guidance contract ran in the file-list suite, no pre-existing source checks were lost, and only the three authorized source/test roots changed. Preserve this preparation's SHA-256 for user-owned `scripts/install.sh`: `54cdc01cdbc0722ce3a62315c7b69c55942a23f17659b07e2def8c0f19867c96`. Update this tracker and full mirror, read both back, and return exact results plus rollback boundary to the parent.

Preparation evidence: CodeGraph read the two Go test files; direct Read covered the unsupported PowerShell source and tracker/config. Narrow installer-reference search found only the two relevant Go test files (an Engram-download test has a comparison comment, not a dependency). Installed `pwsh` is 7.6.6; `powershell` is absent. Local command discovery confirmed the native HTTP/JSON cmdlets; installed Go help confirmed the metadata-parser invocation. No network, tests, application builds, or installs were run during preparation. PowerShell 5.1 execution, native Windows behavior, live GitHub/installation, and repository-minimum Go remain unproven; PS7/macOS evidence cannot close those gaps. Commands above are pending execution against the planned changes, not successful results.

### PowerShell execution evidence and parent handoff

- RED: the focused command above exited 1, `FAIL command-line-arguments 29.978s`. All 15 success cases and 17 resolution-failure cases failed; the updated guidance contract failed. The three source-format checks and existing install-exit/binary-hold/insecure-hold cases passed. This was before any PowerShell production edit.
- First GREEN attempt: exit 1, 31.574s, 32/35 leaf cases passed. Three root-module cases exposed absent environment values being restored as empty strings through `[Environment]::SetEnvironmentVariable` on PS7/macOS. Production now removes absent `Env:` entries explicitly and restores existing values normally. Corrected GREEN: exit 0, 31.337s, 6 top-level tests and 35/35 leaf cases passed without skips.
- Added the necessary absent-environment parser-failure scenario, verified sentinel contents rather than presence alone, and selected `go.exe` for Windows fixture execution. No production behavior expansion. Final PowerShell coverage is 15 channel/major successes and 21 failures/refusals, including parser-env restoration on success and failure, existing and absent values, and cleanup. Native doubles execute the whole script; only offline Go metadata parsing is real.
- Before final verification, `/opt/homebrew/bin/gofmt -w internal/update/install_script_test.go internal/update/windows_distribution_policy_test.go` from repository root exited 0. The exact parser-only command above and `git diff --check` exited 0.
- Final focused command: exit 0, `ok command-line-arguments 32.083s`; 6 top-level tests, 36 leaf cases passed, no skips, including the relocated distribution-policy contract. Final full-file command: exit 0, `ok command-line-arguments 49.272s`. This also retains the committed Bash regressions; no new historical Bash receipt claim is made.
- Implementation diff in the three authorized roots: 317 insertions, 74 deletions, 391 authored lines (PowerShell 53/9; installer tests 264/44; policy-test relocation 0/21). Tracker bookkeeping is additional. No general framework, dependency, production test guard, git prerequisite, remote script execution, or application build was introduced.
- `scripts/install.sh` remains byte-identical to the authorized SHA-256 above. Status retains the four pre-existing untracked entries; no unrelated source was changed. No commit, push, remote request, real install, native review, or RDD transaction occurred.
- Changed locations: `scripts/install.ps1:32,89,132` (guidance, pinned selection, parser/cleanup); `internal/update/install_script_test.go:258,286,324,378` (relocated policy contract, channels, failures, native fixture); `internal/update/windows_distribution_policy_test.go` (removed only the relocated 21-line test). Rollback removes only these PowerShell behavior/test changes and returns the policy test to its original file; preserve committed Bash behavior and user-owned formatting.
- `skill_resolution: paths-injected`: `/Users/pablo.nazarvasscompany.com/.agents/skills/powershell-windows/SKILL.md`, `/Users/pablo.nazarvasscompany.com/.agents/skills/go-testing/SKILL.md`, `/Users/pablo.nazarvasscompany.com/projects/github.com/pablon/gentle-ai/skills/work-unit-commits/SKILL.md` were loaded during preparation and retained for implementation.
- Runtime proof is PowerShell 7.6.6 on macOS arm64 with Go 1.27.1; native Windows/PowerShell 5.1, repository-minimum Go, and live GitHub/installation remain unproven. Interpreter absence is an explicit fixture skip; no runtime installation or execution-policy bypass is allowed. Parent-provided PS5.1 documentation states finally runs on exit; local fixtures prove cleanup with Stop-WithError under PS7, not PS5.1. Bugfix discovery saved as Engram #4725.

### Final parent verification and delivery state

The following evidence was supplied by the parent for bookkeeping; this update ran no additional tests or source edits.

- Independent verifier ran from `internal/update/`: `env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS= /opt/homebrew/bin/go test -count=1 -json install_script_test.go`. Exit 0, 46.355s; 8 top-level tests, 36 PowerShell plus 30 Bash leaf cases, zero skips. PowerShell parser syntax and diff-check exited 0. Static inspection found no blocker or PS7-only production construct; this is not native Windows/5.1 execution proof.
- Parent independently reran the exact tracker parser command and `git diff --check`: both exited 0. The Bash SHA-256 remains `54cdc01cdbc0722ce3a62315c7b69c55942a23f17659b07e2def8c0f19867c96`; user-owned formatting remains unstaged and preserved.
- RDD status remains off. Assessment refused the missing untracked-inventory declaration, so independent verification was used; no native review or review transaction is claimed.
- Parent-read [Bash comment r4027657171](https://github.com/Gentleman-Programming/gentle-ai/pull/4683#discussion_r4027657171) is addressed in commit `0f221b1d5e5de489025e80e853455e2b44ebedf4`. Parent-read [PowerShell comment r4027657105](https://github.com/Gentleman-Programming/gentle-ai/pull/4683#discussion_r4027657105) is addressed by the local uncommitted follow-up. No GitHub reply, thread resolution, or push was performed.
- Native Windows/PowerShell 5.1, repository-minimum Go, and live GitHub/installation remain unproven. Existing skill resolution is inherited; no unrelated documents, source, tests, or summaries were changed in this bookkeeping step.

Historical handoff: user review and, only when authorized, commit the PowerShell follow-up. Progress: 4/4 tasks complete, parent-verified. The earlier requirement to preserve unrelated Bash formatting applied before the authorization below.

### User-authorized PowerShell commit preparation

- The user explicitly authorized removing only the residual Bash formatting and then committing the PowerShell fix, its two associated test files, and this tracker as one logical unit. The parent restored `scripts/install.sh`; this commit preparer made no Bash or source/test edits. `git diff --exit-code 0f221b1d5e5de489025e80e853455e2b44ebedf4 -- scripts/install.sh` exited 0 with no diff. Historical formatting hashes above are no longer the preservation target.
- Current commit scope is exactly `scripts/install.ps1`, `internal/update/install_script_test.go`, `internal/update/windows_distribution_policy_test.go`, and `odd/tasks/durable-go-installer.md` on `fix/durable-go-installer`. Proposed subject: `fix(install): resolve Windows Go modules across major versions`. No unknown commit hash is recorded; this entry describes preparation, not a completed commit.
- Reconciled this full tracker with Engram observation #4715 before updating. Full-file offline test command from DGI-04 reran from `internal/update/`: exit 0, `ok command-line-arguments 46.778s`. Native Bash syntax, the exact PowerShell parser-only command above, and `git diff --check` all exited 0. Existing independent evidence records 36 PowerShell and 30 Bash leaf cases with zero skips; this non-verbose rerun does not independently report case counts.
- Native Windows/PowerShell 5.1, repository-minimum Go, and live installation remain unproven. RDD remains off; no lifecycle, assessment, application build, install, remote operation, push, configuration update, or nested delegation is part of this commit. Preserve all four unrelated untracked entries. Stage only the four authorized paths after tracker/mirror readback, inspect the complete cached diff and whitespace check, then commit with configured SSH signing and signoff without bypassing hooks or amending.
