# SDD Init Details

## Project Root Discovery

A workspace root without stack markers does not mean the workspace is unclassified — in a monorepo the markers live one or two levels down. Discover roots before classifying anything.

- **Marker files** that define a project root: `package.json`, `go.mod`, `pyproject.toml`, `setup.py`, `requirements.txt`, `Cargo.toml`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `Gemfile`, `composer.json`, `*.csproj`, `*.sln`, `mix.exs`, `pubspec.yaml`.
- **Depth**: scan the workspace root and up to **two** directory levels below it. Deeper nesting is reported as a limitation rather than scanned, so the cost stays bounded on large trees.
- **Skip** directories that never hold a first-party project root: `node_modules`, `vendor`, `target`, `dist`, `build`, `.venv`, `venv`, `__pycache__`, `.git`, `.gradle`, `bin`, `obj`, and any path ignored by `.gitignore`.
- **Workspace-manager roots count once**: when a root declares members (`workspaces` in `package.json`, `[workspace] members` in `Cargo.toml`, `pnpm-workspace.yaml`, `go.work`), use the declared members as the sub-project list instead of re-deriving it from the directory walk.
- **Naming**: the sub-project name is its path relative to the workspace root (`services/api`), not just the leaf, so two `api` directories never collide.

| Discovered | Layout | Persistence shape |
|---|---|---|
| marker at workspace root only | single project | one context, one capability set |
| markers only in sub-directories | monorepo | one entry per sub-project, workspace recorded as the container |
| marker at root **and** in sub-directories | monorepo with a root project | root project plus one entry per sub-project |
| no marker at any scanned depth | unclassified | report the scanned directories and the skip list applied |

Report per sub-project: relative path, language/stack, test runner, and resolved `strict_tdd`. Never average unlike sub-projects into a single verdict — a Python service with `pytest` and a Rust crate with `cargo test` are two answers, not one.

## Testing Capability Checklist

- Test runner: `package.json` scripts/deps, `pyproject.toml`, `pytest.ini`, `go.mod`, `Cargo.toml`, `Makefile`.
- Test layers: unit runner; integration libraries (`testing-library`, `httpx`, `httptest`, `WebApplicationFactory`); E2E tools (`playwright`, `cypress`, `selenium`, `chromedp`).
- Coverage: `vitest --coverage`, `jest --coverage`, `c8`, `pytest-cov`, `go test -cover`, `coverlet`.
- Quality: linter, type checker, formatter commands.

## Skill Registry Scan Rules

- Scan user skills: `~/.pi/agent/skills/`, `~/.config/agents/skills/`, `~/.agents/skills/`, `~/.kimi/skills/`, `~/.config/opencode/skills/`, `~/.config/kilo/skills/`, `~/.claude/skills/`, `~/.gemini/skills/`, `~/.gemini/antigravity/skills/`, `~/.cursor/skills/`, `~/.copilot/skills/`, `~/.codex/skills/`, `~/.codeium/windsurf/skills/`, `~/.qwen/skills/`, `~/.kiro/skills/`, and `~/.openclaw/skills/`.
- Scan project skills: `{project-root}/skills/`, `{project-root}/.opencode/skills/`, `{project-root}/.claude/skills/`, `{project-root}/.gemini/skills/`, `{project-root}/.cursor/skills/`, `{project-root}/.github/skills/`, `{project-root}/.codex/skills/`, `{project-root}/.qwen/skills/`, `{project-root}/.kiro/skills/`, `{project-root}/.openclaw/skills/`, `{project-root}/.pi/skills/`, `{project-root}/.agent/skills/`, `{project-root}/.agents/skills/`, and `{project-root}/.atl/skills/`.
- Skip `sdd-*`, `_shared`, and `skill-registry`; deduplicate by skill name, preferring project-level skills over user-level skills.
- Read each selected `SKILL.md` frontmatter as needed.
- Extract `name`, trigger text from `description`, full `SKILL.md` path, and scope.
- Treat the registry as an index, not a generated summary; subagents receive exact paths and read the full skill source of truth.
- Scan project convention files: `agents.md`, `AGENTS.md`, project-level `CLAUDE.md`, `.cursorrules`, `GEMINI.md`, and `copilot-instructions.md`.
- For index files such as `AGENTS.md`, extract referenced file paths and include both the index and referenced files in the registry.

## LLM-First Skill Criteria

- Treat skills as runtime instruction contracts, not human documentation.
- Required structure: frontmatter, Activation Contract, Hard Rules, Decision Gates, Execution Steps, Output Contract, References.
- Keep `description` quoted, one physical line, trigger-first, and no longer than 250 characters.
- Target 180-450 body tokens; move examples, schemas, edge cases, and background into local `references/` or `assets/`.
- References must be local files and stable relative to the skill directory when possible.
- Quality gates: hard rules are observable, decision gates cover real forks, output contract states exactly what to return, and references resolve locally.

## Engram Saves

```text
mem_save title/topic_key: sdd-init/{project}
type: architecture
content: detected project context markdown
capture_prompt: false when available

mem_save title/topic_key: sdd/{project}/testing-capabilities
type: config
content: testing capabilities markdown
capture_prompt: false when available
```

In a monorepo, save one capability observation per sub-project and keep the
workspace-level key as the index that lists them:

```text
mem_save title/topic_key: sdd/{project}/{sub-project-path}/testing-capabilities
type: config
content: testing capabilities markdown for that sub-project
capture_prompt: false when available

mem_save title/topic_key: skill-registry
type: config
content: registry markdown
capture_prompt: false when available
```

## OpenSpec Skeleton

```text
openspec/
├── config.yaml
├── specs/
└── changes/
    └── archive/
```

`config.yaml` should include concise context, `strict_tdd`, testing capabilities, and phase rules for proposal/spec/design/tasks/apply/verify/archive. Keep `context:` under 10 lines.

For a monorepo, keep the workspace-level `context:` under 10 lines and move the per-sub-project detail into a `projects:` list so unlike stacks stay separable:

```yaml
context: |
  workspace is a monorepo with 2 sub-projects.
strict_tdd: false          # workspace default; each project may override
projects:
  - path: a
    stack: Python
    testing: { unit: pytest, coverage: pytest-cov }
    strict_tdd: true
  - path: b
    stack: Rust
    testing: { unit: cargo test }
    strict_tdd: true
```

## Testing Capabilities Format

```markdown
## Testing Capabilities

**Strict TDD Mode**: {enabled/disabled}
**Detected**: {date}

### Test Runner

- Command: `{command}`
- Framework: {name}

### Test Layers

| Layer       | Available | Tool        |
| ----------- | --------- | ----------- |
| Unit        | ✅ / ❌   | {tool or —} |
| Integration | ✅ / ❌   | {tool or —} |
| E2E         | ✅ / ❌   | {tool or —} |

### Coverage

- Available: ✅ / ❌
- Command: `{command or —}`

### Quality Tools

| Tool         | Available | Command        |
| ------------ | --------- | -------------- |
| Linter       | ✅ / ❌   | {command or —} |
| Type checker | ✅ / ❌   | {command or —} |
| Formatter    | ✅ / ❌   | {command or —} |
```

## Output Templates

For each mode, include project, stack, persistence, Strict TDD Mode, Testing Capabilities table, artifacts created/saved, limitations where relevant, and next steps. Engram mode must mention local/non-shareable limitations; none mode must recommend enabling persistence.
