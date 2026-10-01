# Release Checklist v0.1.0

> [!WARNING]
> **Historical record.** This document is a preserved snapshot kept for provenance; it is not current guidance. The reference docs track `main` — see the [v3.7.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v3.7.0/docs) for the latest release.

## Scope freeze

- [ ] Confirm MVP scope remains macOS + Claude Code + OpenCode only.
- [ ] Confirm no post-MVP features are merged.

## Quality gates

- [ ] Run targeted unit tests for verify and golden coverage.
- [ ] Run install CLI parity tests.
- [ ] Validate dry-run output is generated without apply side effects.

## Manual smoke

- [ ] Dry-run install on macOS.
- [ ] Real install on macOS test account.
- [ ] Validate key output paths for Claude Code and OpenCode.
- [ ] Validate Engram™ health endpoint is reachable when selected.

## Documentation

- [ ] README references quickstart/non-interactive/rollback docs.
- [ ] Quickstart commands reflect current CLI flags.
- [ ] Rollback guidance matches backup/restore behavior.

## Release prep

- [ ] Tag `v0.1.0`.
- [ ] Publish release notes with known limitations.
- [ ] Announce explicit MVP constraints (macOS only, limited agent support).
