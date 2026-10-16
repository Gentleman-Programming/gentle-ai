# Windows install/open draft checkpoint

This is scoped lab evidence for the unfinished Windows Separate installer, not general Windows or release approval.

## Historical install/open evidence
- Fresh run `4a890cf18dbd463397afb027b3a7aa6b`: existing installer TUI, installation exit 0, completed destination, fresh Gentle Shell UI and `/quit` exit 0.
- Compiled product source tree: `921353370c4e16201346e62b7364ab82b5e25e49`.
- Executable SHA-256: `0c328410602921a57cdc2933986506e8592b625fefbfd87eff635c66d99efaed` (25,843,200 bytes).
- Native run `1f710208d4894b32b8710c88d4b71487`: focused installer/CLI suites and Windows build exited 0; an unelevated symlink negative was skipped.
- Windows 11 Enterprise 26100 x64, unelevated credentialless lab account, physical Job memory/process/CPU limits and NTFS volume readback. Caller CWD, environment, PATH and fixture checks passed.

## Settings compatibility and repeated entries
- RED run `cddf7111bab54b388e01f33956c735d2`: installer exit 0, 18/19 fixtures passed; only `sdk-changelog-version` failed its own acceptance assertion, not infrastructure.
- The correction preserves exact `packages` and `npmCommand`, permits only string-valued `lastChangelogVersion`, and rejects unknown keys. It does not authorize arbitrary SDK configuration mutations.
- Native build `013a1697633f40aaa87fabe5a245d877`: focused installer/CLI suites and build exited 0, with the same symlink-negative skip.
- Corrected source tree: `9eed2a6200fb2d41363996ca59c3ce2ce5d25915`; executable SHA-256: `4320710ab25ede3e9850acdf82a671452c8b93a9b3a8268e0a58f0c8b4a2c8a9` (25,843,712 bytes).
- Fresh UI run `4753f431f9544464b938d85ea483b92f`: 19/19 settings fixtures passed, then installed `gentle-shell.cmd`, `pi.cmd`, `gentle-shell.cmd`, `pi.cmd` each produced a fresh opening and `/quit` return with exit 0.
- That checkpoint exercised installed verification after SDK state mutation, not distinct product roles: its wrappers shared a Pi path. Caller CWD/environment/PATH, fixture preservation, resource readbacks, capture and owned CMD exit passed.

## Stable/Main source and authority
- Parent-observed RED `b1e9f99193154cc084b3c1e430220a26` failed only three channel and two role assertions.
- The candidate binds the selected channel into confirmation and retained selection, uses authenticated role bin metadata, and provides a current-CMD-only PATH activation command.
- Gentle Shell receives an owned isolated home/config and installed-version provisioning marker. Settings retain exact package/npm bindings; only the exact builtin-codemode exclusion and string changelog marker are permitted. Unproven theme changes remain refused.
- Main descriptor RED `3ead2318648e483a9f4d3bba8e5bca55` failed only its two valid-descriptor assertions. The current source candidate resolves the canonical publisher's Main ref once after confirmation and before staging, retains its immutable commit ZIP/digest, and binds the generated npm lock and selected channel.
- Main overlays only authenticated repository source. Stock npm dependencies and the SumDB-built 4.0.0 native companion remain separate authorities; unsupported package/dependency/native-runtime contracts fail closed. Every launch compares the entire Main source set/bytes to the retained ZIP without re-resolving Main. TLS publisher provenance is not an independently signed-binary claim.
- Stock supplier helper hashes gate every native installer invocation. Main overlay/verification instead uses its frozen source authority; the retained native version/method/SumDB checksum/binary digest checks remain mandatory. Actual Main RED `ee620c17f3a44362a03a49ea10c9884a` exposed the former unconditional stock-source comparison after overlay; the correction does not invoke a Main native installer.
- Verification rejects incomplete/unknown selection fields and binds the destination to the actual published root, not just a rewritten manifest hash. Shell uses authenticated adjacent Pi without a wrapper override.
- This candidate supports fresh owned installations only. Legacy channel-less installations deliberately fail verification; no migration/update or edits to existing user roots are attempted.

## Current controlled Guest matrix
- Native run `65c710b4b38b42169e1fcc35effdb3bd`: focused Windows installer/CLI suites and build exited 0. Permission-dependent reparse/symlink negatives remain skipped.
- Product/test/control source tree: `261a61e3e9ccc60a999b5adef202143346b684f3`; executable SHA-256: `8616c569bea55b2286c4bcb1a47133ed90ba760e984b2fdd25d425d705057920` (25,902,080 bytes).
- Main run `5cbb28a553224d339751db266d72aed1` and stable run `24e1af3fc7a548da8ca88ea8ef43d587` used that same product: fresh installer TUI and installation exit 0; 37/37 Guest-executed VM/source-region settings/config/native-authority fixtures per channel (not 37 live installation scenarios).
- Each channel opened `gentle-shell`, `pi`, `gentle-shell`, `pi` freshly and returned `/quit` exit 0. A single trusted Guest process snapshot per opening checked quoted owned Node/CLI argv: direct Pi had no Shell launcher; Shell had its own launcher and Pi descendant.
- Bare names resolved after explicit PATH activation in a nested current CMD, not through a personal/global PATH change. Caller CWD/environment/PATH, fixture preservation, physical resource readbacks, complete capture and owned CMD cleanup passed.
- Main retained commit `aa2c03896be9866ab0af89bbc621d4c2a8fcf9c2`, ZIP SHA-256 `432aaa147885071aa84dd4a2b4630f646350d835de88dfe0f8aa0c21addd8aeb` and generated lock SHA-256 `da06d9e1da42c7edf19d74989b799315e97aa5a3b56d50b05403240bc5cc50c7`; launch verification did not follow the moving ref.
- These are successful scoped fresh-Separate checks, not native review closure or full/default-Windows qualification.

## Review corrections after the matrix (not yet Guest-executed)
- Launch verification still walks every entry recursively with unchanged per-entry guards and bounds; only names strictly below the fixed runtime state roots (`home`, `state`, `tmp`, `runtime\cache`, `agent\sessions`) may contain `!`, `%`, `&` or `^`. Large-cache performance/bounds remain unqualified. Settings additionally accept the pinned Pi 1.0.0 UI scalars (model, provider, thinking level, path-free theme). Install cancellation is cooperative with owned-job quiescence and strict owned-stage removal.
- `windows-owned-settings.test.mjs` replaced `unproven-theme` with `theme-path-injection` (still 37 fixtures; Guest digest updated). `windows-mutable-settings.test.mjs` is a portable VM region check, not runtime evidence.
- The new native Windows unit tests only cross-compiled; no Guest run, RED or installed-runtime proof exists for these corrections yet. The matrix above predates them.

## Limits and follow-up
- The disposable guests' system-root ACLs were modified. Untouched/default Windows and independent stock-UI authenticity are not qualified.
- Full receipts remain `QualifiedCandidateGuest=false` and `FunctionalReady=false`; these are scoped lab checks, not native review closure or release approval.
- Stable pins version 4.0.0. Both channels passed the scoped fresh-install/role/current-console/repeat matrix above. Shared/existing Pi integration and configured-model operation are not qualified; missing-model UI was visible. Ctrl-D, console modes, foreground restoration and personal-installation preservation remain unproved.
- `e2e/windows-ui-acceptance-normal.ps1` is a guest payload, not a standalone host command. It requires the external lab controller, qualified Job helper and ConPTY helper; those dependencies are not delivered by this checkpoint. Do not execute it on the operator host.

## Rollback
Revert this checkpoint's product changes with their tests, control and evidence document. It introduces no personal PATH/configuration migration.
