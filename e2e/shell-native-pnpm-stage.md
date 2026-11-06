# Private PNPM payload staging

`StageNativePNPMInstallation` copies all seven operational roots into a new private stage, breaking source hardlinks.
Only generated shims, PNPM store metadata, two operational settings and contained absolute Node links are relocated.
It verifies the complete copy and original, syncs directories and returns a PNPM code seal plus source target identities.
It does not publish or execute suppliers; final absolute links and final file identities require post-rename readback.
Run `go test -tags=shell_pnpm_publication -run '^TestUserPNPMStage$' ./internal/shellinstaller` in the qualified private sandbox.
Rollback removes this staging API, its regression and this note; the existing code-seal API remains independent.
