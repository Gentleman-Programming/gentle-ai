# SDD Init Details

## Testing Capability Checklist

- Test runner: `package.json` scripts/deps, `pyproject.toml`, `pytest.ini`, `go.mod`, `Cargo.toml`, `Makefile`.
- Test layers: unit runner; integration libraries (`testing-library`, `httpx`, `httptest`, `WebApplicationFactory`); E2E tools (`playwright`, `cypress`, `selenium`, `chromedp`).
- Coverage: `vitest --coverage`, `jest --coverage`, `c8`, `pytest-cov`, `go test -cover`, `coverlet`.
- Quality: linter, type checker, formatter commands.

## Workspace Project Discovery

Start at the authoritative workspace root. Complete this discovery before stack or test-runner classification and before applying the no-runner fallback.

1. Read explicit workspace membership first when the workspace config declares it (for example package-manager workspaces or Cargo workspace members). Inspect each declared member that resolves inside the workspace root.
2. If there is no explicit membership, inspect candidate project roots at the workspace root and at most two directory levels below it. This bounded fallback covers direct children such as `A/pyproject.toml` and `B/Cargo.toml`; do not recursively crawl beyond that depth.
3. Never descend into VCS, dependency, generated/build, cache, virtual-environment, or nested-repository boundaries. At minimum exclude `.git`, `node_modules`, `vendor`, `dist`, `build`, `out`, `target`, `.cache`, `__pycache__`, `.venv`, `venv`, and directories that contain their own `.git` file or directory.
4. Treat a directory with a supported project marker as one project root. For every discovered project, retain its workspace-relative path, stack, test command, test layers, coverage command, and quality-tool commands. Do not collapse multiple commands into a chosen workspace runner.

Use one workspace-level project-context and testing-capabilities result. In the result, represent projects as a table with one entry per relative path and keep each project's commands in that entry. In `openspec/config.yaml` when it is written, use a `projects:` list with the same entries and commands. Existing consumers may only render the aggregate result, so do not imply that they can execute a single combined command.

Resolve Strict TDD after the complete project list is evaluated; only then apply the no-runner fallback. An explicit workspace-level test command is an existing command or target defined by the workspace or explicit config that runs the complete in-scope set from the authoritative workspace root. Do not synthesize or concatenate independent project commands.

- Preserve explicit `strict_tdd: false`.
- Use explicit `strict_tdd: true` only when an explicit workspace-level test command covers every in-scope project; otherwise fail closed to false and explain that downstream execution requires a workspace-wide command.
- Without an explicit value, default to true only when the discovered project set is non-empty and one explicit workspace-level test command covers every in-scope project.
- When zero projects are discovered or no explicit workspace-level command covers every in-scope project, use `strict_tdd: false`. Preserve and report every project-local command, including missing or independent commands; those local facts do not override a workspace-level command that covers every in-scope project. Explain the no-runner or workspace-wide-command fallback.

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

## Testing Capabilities Format

```markdown
## Testing Capabilities

**Strict TDD Mode**: {enabled/disabled}
**Detected**: {date}

### Projects

| Relative path | Stack | Test command | Framework |
| ------------- | ----- | ------------ | --------- |
| `{path}` | {stack} | `{command or —}` | {name or —} |

### Test Layers

| Relative path | Layer       | Available | Tool        |
| ------------- | ----------- | --------- | ----------- |
| `{path}` | Unit        | ✅ / ❌   | {tool or —} |
| `{path}` | Integration | ✅ / ❌   | {tool or —} |
| `{path}` | E2E         | ✅ / ❌   | {tool or —} |

### Coverage

| Relative path | Available | Command |
| ------------- | --------- | ------- |
| `{path}` | ✅ / ❌ | `{command or —}` |

### Quality Tools

| Relative path | Tool         | Available | Command        |
| ------------- | ------------ | --------- | -------------- |
| `{path}` | Linter       | ✅ / ❌   | {command or —} |
| `{path}` | Type checker | ✅ / ❌   | {command or —} |
| `{path}` | Formatter    | ✅ / ❌   | {command or —} |
```

## Output Templates

For each mode, include project, stack, persistence, Strict TDD Mode, Testing Capabilities table, artifacts created/saved, limitations where relevant, and next steps. Engram mode must mention local/non-shareable limitations; none mode must recommend enabling persistence.
