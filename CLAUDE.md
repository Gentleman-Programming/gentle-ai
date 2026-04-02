# Gentle AI — Go Toolkit for AI-Assisted Development
> Gentleman Living Architecture | Canvas: inline below

## Methodology: Gentleman Living Architecture
Workflow: Canvas → Orient → Plan → Delegate → Synthesize → Judge → PCD Loop → Hand Off

## Canvas
- **Problema**: AI-assisted development tools are scattered — each needs its own setup, config, and conventions.
- **Usuario**: Allan Guerrero + developers using Claude Code and similar AI coding tools.
- **Solución**: Unified Go toolkit that manages AI agent configuration, SDD workflow, and development automation.
- **Fuera de scope**: Not an AI model, not a chat interface. It's a TOOLCHAIN — manages the workflow around AI tools.

## Rules
1. **Go idioms**: table-driven tests, error wrapping, interfaces for testability
2. **SDD integration**: auto-init guard resolves Strict TDD from config, not prompts
3. **TOML config**: escape backslashes on Windows (known gotcha)
4. **PCD Loop (mandatory)**: After every task → Prevent (neurona?), Codify (skill?), Delegate (engram saved?)

## Stack
| Component | Technology | Purpose |
|-----------|-----------|---------|
| Language | Go 1.22+ | Core CLI |
| Config | TOML | Agent configuration |
| Testing | Go testing + e2e/ | Unit + end-to-end |
| SDD | OpenSpec (file-based) | Spec-driven development artifacts |
| Docs | Markdown | docs/ directory |

## Scopes
| Scope | Path | Responsibility |
|-------|------|----------------|
| CLI | `cmd/` | Entry points |
| Core | `internal/` | Business logic |
| E2E | `e2e/` | End-to-end tests |
| Scripts | `scripts/` | Automation helpers |
| OpenSpec | `openspec/` | SDD artifacts |
| Test data | `testdata/` | Test fixtures |

## Skills (2 project skills)
See `AGENTS.md` for skill routing (issue-creation, branch-pr).

## Current State
- **Health**: Active, recent commits: SDD auto-init guard, TOML Windows fix, GGA upgrade via git clone
- **Deploy**: Local CLI tool
- **Tests**: e2e/ directory exists with test infrastructure
- **License**: MIT

## Gotchas
| Gotcha | Severity | Notes |
|--------|----------|-------|
| TOML backslash on Windows | HIGH | Must escape, known fix in recent commits |
| GGA upgrade uses git clone | MEDIUM | Changed from go install to git clone |
| Backup skip directories | MEDIUM | Pre-upgrade snapshot skips dirs now |
