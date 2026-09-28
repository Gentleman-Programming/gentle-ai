# Maintainer Playbook

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Use this page before reviewing or making a change. It turns the codebase map into practical guardrails.

## Product capabilities

| Capability | Maintainer lens |
|---|---|
| Agent setup | Does the selected adapter receive the right files without clobbering user content? |
| Persistent memory wiring | Does Engram MCP setup stay stable across terminals and IDE-launched agents? |
| ODD workflow | Do shared guidance, feature-task tracking, delegated workers, and supported model assignments stay consistent across agents? |
| Sync | Is repeated sync a no-op when assets are current? |
| Backup/rollback | Can users recover from every managed mutation? |
| TUI | Does the interactive path match CLI behavior where it should? |
| Updates | Are binary upgrade and post-upgrade sync guidance clear? |
| Skill registry | Does `.atl/skill-registry.md` reflect the skills users and subagents need? |
| Community tools | Is optional tool guidance separate from plugin registration and external runtime ownership? |

## Navigate as a maintainer

1. Read the user-facing doc first.
2. Find the owning package in [Repository map](repository-map.md).
3. Inspect nearby tests before changing code.
4. Check generated assets and golden files if output changes.
5. Update the smallest relevant doc page.

## Guardrails

- **Do not touch local runtime state**: `.engram/engram.db`, `.engram/cloud.json`, and accidental `main` files are not documentation inputs.
- **Do not invent external internals**: dashboard, cloud, and Engram store implementation must be backed by source or linked external docs.
- **Do not bypass adapters**: path and strategy logic belongs in agent packages.
- **Do not bypass backups**: any user config write needs a recovery story.
- **Do not duplicate references**: link to [Usage](../usage.md), [Engram Commands](../engram.md), or specialized docs for detailed command/API behavior.

## Checklists by change type

### Agent support

- [ ] Add or update `model.AgentID` and `catalog` entry.
- [ ] Implement adapter paths and strategies.
- [ ] Wire component support through existing interfaces.
- [ ] Add tests for paths and generated config.
- [ ] Update [Agents](../agents.md) and README table if user-visible.

### Component behavior

- [ ] Keep behavior in `internal/components/<component>/`.
- [ ] Verify planner dependencies still order correctly.
- [ ] Test idempotency and merge behavior.
- [ ] Update docs for the affected component only.

### CLI or TUI behavior

- [ ] Keep flag parsing in CLI packages.
- [ ] Mirror interactive choices in TUI only when intended.
- [ ] Update [Usage](../usage.md).
- [ ] Add screen or app dispatch tests.

### Skill registry behavior

- [ ] Keep scan/cache behavior in `internal/skillregistry/`.
- [ ] Route command changes through `internal/app/` skill-registry dispatch.
- [ ] Update [Skill Registry](../skill-registry.md) when user-visible flags or output change.
- [ ] Regenerate `.atl/skill-registry.md` only when the indexed skills intentionally change.

### Community tool or OpenCode plugin behavior

- [ ] Use `internal/components/communitytool/` for external tool install orchestration plus managed guidance/config/MCP reconciliation.
- [ ] Use `internal/components/opencodeplugin/` for OpenCode TUI plugin registration: external plugins add package names, while Gentle Logo writes/registers the managed local plugin file.
- [ ] Keep external runtime ownership explicit in docs.
- [ ] Test idempotent sync/registration behavior.

### Upgrade or deferred sync behavior

- [ ] Keep version/update checks in `internal/update/` and external tool execution in `internal/update/upgrade/`.
- [ ] Confirm TUI upgrade and upgrade+sync screens still match CLI behavior.
- [ ] Document post-upgrade sync guidance without implying unmanaged local files are modified.
- [ ] Test skipped, failed, and partial upgrade paths.

### Docs-only change

- [ ] Validate claims against source files.
- [ ] Link to existing references instead of copying them.
- [ ] Run markdown link validation for changed docs.
- [ ] Keep English filenames, headings, labels, and content.

## PR/review playbook

| Review question | Why it matters |
|---|---|
| Does the change have one owner? | Prevents scattered behavior and future drift. |
| Is sync idempotent? | Users run sync after every upgrade. |
| Are user files protected? | This tool edits external config roots. |
| Are docs linked from the right entry point? | Maintainers and users need discoverability. |
| Are external capabilities represented honestly? | Avoids promising dashboard/cloud/API behavior that is not in this repo. |

## Navigation

Previous: [Integrations](integrations.md) | Next: [Reference map](reference-map.md)
