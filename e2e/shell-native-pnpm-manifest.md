# Retained PNPM manifest readback

An optional `pnpm` descriptor adds the trusted staging code seal, root-relative package location, Shell version and distinct adjacent Pi peer version to the existing manifest. Absent metadata preserves npm serialization and readback. The descriptor contains no source inode stamps or per-file inventory and remains within the existing 4096-byte manifest bound.

Linux amd64 readback checks Separate root identities, exact supervisor/bindings, final physical package targets and the full PNPM seal. Mutable agent contents are not code. Shared mode, mixed npm/optional-component metadata and unsupported Unix platforms fail closed. This readback does not publish or activate options. PNPM-specific bindings are verified separately from unchanged legacy npm binding bytes.

Run the qualified private fixture with `go test -tags=shell_pnpm_publication -run '^TestUserPNPMManifest' ./internal/shellinstaller`, then the default package suite. Tests retain the original DATA read-only, stage independently, validate final-root readback and reject inconsistent descriptors and new unsealed code.
