# Private GGA configuration, not automatic activation

`userPrepareInstallStage` validates the full immutable profile/mode before any
writes. Explicit GGA is private Separate only; generic/Windows/Shared/public
admission remains refused. ON writes `TARGET/config/gga/config` with the explicitly
selected `PROVIDER`, 0700 directory/0600 file, LF and existing rooted exclusive
writer/sync/custody checks. OFF/absence creates no GGA directory or file.
No persona lookup for unmanaged; no project `.gga`, rules file or git hook write.

## Actual consumer proof

In the existing offline credentialless Guest:

```
go test -tags=shell_gga_canonical -run '^TestUserGGA' ./internal/shellinstaller
```

The frozen source archive for GGA v2.10.1 commit
`1124c3672f082c56b033c4e23a30e95d0e8cd593` is hash-checked before extracting
its four exact CLI/library members. Actual `gga config` reads native-prepared
ON/OFF/absent stages using the installer's private environment and leaves stage
preimages unchanged. Source reports `dev`, not an installed release receipt.
Configured text is not provider installation/authentication/model health or AI
review acceptance. No provider command, personal installation or hook activation.

Rollback removes this private-stage preparation and its tests; keep selection
transport/refusals. CLI provisioning and platform installation remain separate.

This recovery retains all source030 assertions without repeating local Go/CLI
qualification. The CLI wrapper reuses the bounded owned-group/pipe-wait helper;
it does not prove escaped-descendant containment or complete tree settlement.
No provider health, native installation or installed Ready was newly tested.

Context termination preserves `context.Canceled` and `context.DeadlineExceeded`
before GGA effects. Deterministic cancellation/past-deadline regression cases
also check the stage parent stays unchanged. Authored before the fix, they were
not run locally; current-head CI remains required.

## Qualified fixture parents

PR #5584 at `bb8ef77c6` failed `go test ./...` in the ON/OFF/absent configuration
and existing-configuration preservation fixtures. Those positive fixtures used
`t.TempDir()` without establishing the preparer's unchanged private0700 parent
contract. They now reuse the existing `installStagePath(t)` fixture; no
production admission check is changed or bypassed.

Explicit0755-parent controls retain physical refusal for all three GGA choices
and require the confirmed profile, parent identity/metadata and empty directory
inventory to remain unchanged, with no stage created. The original configuration,
permission, OFF/absence, unrelated-path, preimage and context-cause assertions stay
intact. These controls were authored before correcting the positive fixtures.
No local Go/CLI tests were run; corrected-head CI is still required. The old
head's skipped benchmark/evidence/deadcode and format steps are not PASS.
