# Experience handoff for the common installer

E1 groundwork: `NewShellInstallModelWithExperience` carries the complete validated profile into the existing OS selector. `ShellInstallPresentationOf` supplies destination/Ready views without another installer model. Presentation and outcome are snapshots; rendering cannot edit the inspected request through an aliased profile.

The legacy constructor and nil-profile entry arguments remain unchanged. No worker starts during initialization, editing or cancellation. Real inspection still refuses unqualified public profiles before effects; this unit does not admit installation, add the Configure renderer, or claim an installed flow.

Verification in the existing offline Guest:

- Missing public interfaces: compiler RED, not behavioral RED.
- `go test -p 1 -count=1 -v -run '^TestShellInstallEmbedded(Experience|Presentation)' ./internal/cli`: 3 top-level tests / 2 subtests PASS, zero SKIP.
- `go test -p 1 -count=1 -v -run '^TestShellInstall' ./internal/cli`: selector regression PASS; actual package exit0 recorded separately from controller readback.
- CLI package cross-builds Linux amd64, Darwin arm64, Windows amd64 PASS: structural only, not native platform execution.
- Guest formatter readback applied only to the four changed Go files; final formatter diff empty.

Next E1 unit: connect common Configure rendering/input and destination/Ready to these APIs; keep backend admission in E2 and milestone #72 pending until the complete flow is wired. Rollback removes this API/test/doc unit and restores the original outcome getter/Windows fields, without reverting any producer or custody work.

This integration preserves the source Guest observations above as historical only. No local Go/CLI/SDK/PTY qualification was rerun; current-candidate functional CI and independent verification remain pending. Ready is pre-install review, never installed acceptance.
