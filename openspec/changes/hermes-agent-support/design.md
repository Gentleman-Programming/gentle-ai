# Design: Hermes Agent Support

Add Hermes (Nous Research) as a TierFull agent with parity to existing agents: MCP wiring
(context7 + engram), SDD orchestrator instructions, strict-TDD and persona protocol — all
injected into Hermes's native global config (`~/.hermes/config.yaml`) and system prompt
(`~/.hermes/SOUL.md`). Hermes is detect-only (no auto-install) and uses YAML config, which
introduces the project's first `StrategyMergeIntoYAML` MCP strategy backed by hand-rolled,
comment-preserving YAML string helpers (no new module dependency).

## Context

- Hermes config is **global only** at `~/.hermes/` — there is NO workspace-first routing
  (contrast OpenClaw which writes `SOUL.md` into the active workspace dir). This means
  `run.go`/`sync.go` need no Hermes-specific workspace branches.
- MCP servers live under a top-level `mcp_servers:` key inside `~/.hermes/config.yaml`.
- `SOUL.md` is "slot #1" of the system prompt — guaranteed loaded every session. It is the
  natural target for persona, engram protocol, SDD orchestrator, and strict-TDD markers.
- `go.mod` has NO YAML or TOML library (validated). Codex's `internal/components/filemerge/toml.go`
  is the exact precedent: pure string-based, strip-then-re-append, comment-preserving outside
  managed blocks. Hermes's YAML usage (flat keys + one nested `mcp_servers` table) is structurally
  simple enough for the same approach.
- The generic persona assets — BOTH `generic/persona-gentleman.md` AND `generic/persona-neutral.md` —
  contain a `## Contextual Skill Loading (MANDATORY)` block that assumes a Claude-Code-style
  `<available_skills>` system-prompt mechanism. Hermes loads skills from `~/.hermes/skills/` by
  category via its own native loop, so this block does not apply as-is (Option B decision, #4747).

## Goals

- Register Hermes as `TierFull` (detect / validate / TUI / configure).
- Inject context7 + engram MCP into `~/.hermes/config.yaml` idempotently, without destroying
  user content or comments outside gentle-ai-managed server blocks.
- Inject engram-protocol / SDD-orchestrator / strict-TDD into `~/.hermes/SOUL.md` via markdown markers.
- Inject the correct Hermes-specific persona (gentleman AND neutral) with the skill-loading block
  rewritten for Hermes's native skill model.
- Document the complementary engram-vs-Hermes-native-memory relationship in SOUL.md (via the persona
  asset and/or engram protocol section — see Decision 7).
- Additive only: no behavior change for any existing agent.

## Non-Goals

- Auto-install: locked detect-only (`SupportsAutoInstall()=false`, `InstallCommand` returns a
  not-installable error mirroring OpenClaw). Hermes's curl|bash installer conflicts with the
  supply-chain posture.
- Profile-based config (`~/.hermes/profiles/<name>/`): target the global `~/.hermes/config.yaml`
  only; documented limitation.
- Permissions overlay: Hermes permission format is undocumented — `permissions` component returns
  `nil` for Hermes initially.
- Native `engram setup` slug: `SetupAgentSlug` returns `("", false)`; MCP is injected directly
  via the YAML helpers.
- Full YAML parser (anchors, multi-doc, deep nesting): only the flat-KV + single nested
  `mcp_servers` table subset Hermes actually uses.

---

## Decisions

### Decision 1 — MCP strategy: new `StrategyMergeIntoYAML`

| | |
|---|---|
| **Choice** | Add `StrategyMergeIntoYAML MCPStrategy = 4` to `internal/model/types.go`. |
| **Alternatives** | (a) Reuse `StrategyMergeIntoSettings` with a YAML branch — rejected: that path is JSON-merge-specific (`MergeJSONObjects`), conflating two formats in one strategy. (b) Reuse `StrategyTOMLFile` — rejected: semantically wrong; YAML indentation rules differ from TOML tables. |
| **Rationale** | A distinct enum keeps the dispatch switch honest (`mcp/inject.go`, `engram/inject.go`) and makes the new format self-documenting. Mirrors how Codex got its own `StrategyTOMLFile`. |

### Decision 2 — YAML mutation: hand-rolled string helpers, no `gopkg.in/yaml.v3`

| | |
|---|---|
| **Choice** | New file `internal/components/filemerge/yaml.go` with string-based block-upsert helpers, mirroring `toml.go`. |
| **Alternatives** | (a) Add `gopkg.in/yaml.v3` — rejected (LOCKED): yaml.v3 `Marshal` destroys user comments on round-trip; adds a dependency to an otherwise std-lib-only module; ~300KB binary growth. (b) `yaml.Node`-based comment-preserving edit — rejected: still a new dependency and far more code than the simple subset needs. |
| **Rationale** | Codex proves the pattern. Hermes YAML is a flat KV file plus one nested `mcp_servers:` table — well within reach of strip-then-re-append string manipulation that preserves everything outside managed blocks. |

### Decision 3 — System prompt strategy: `StrategyMarkdownSections` into global `SOUL.md`

| | |
|---|---|
| **Choice** | `SystemPromptStrategy() = StrategyMarkdownSections`; `SystemPromptFile(homeDir) = ~/.hermes/SOUL.md`. |
| **Alternatives** | (a) `StrategyFileReplace` (Codex/Qwen style) — viable but loses the clean per-section marker semantics that let engram/SDD/strict-TDD coexist without clobbering. (b) OpenClaw's `injectOpenClawSoulPersona` special-case — rejected: that exists ONLY because OpenClaw writes `SOUL.md` to the **workspace** dir. Hermes is global, so `SystemPromptFile(homeDir)` already resolves to `~/.hermes/SOUL.md` and the standard `StrategyMarkdownSections` flow handles it with NO special case. |
| **Rationale** | Marker sections are battle-tested (Claude Code, OpenClaw). engram, SDD, strict-TDD, and persona each inject their own `<!-- gentle-ai:ID -->` section. Global path = standard flow. |

### Decision 4 — Adapter shape: hybrid of OpenClaw (detect-only) + Codex (non-JSON string merge), global-only

| | |
|---|---|
| **Choice** | New `internal/agents/hermes/adapter.go` mirroring OpenClaw's detect-only structure (lookPath/statPath injectable, `AgentNotInstallableError`), but global-only (no `resolveWorkspaceDir`, no `validateOpenClawWorkspacePath`-style logic). |
| **Alternatives** | Copy OpenClaw verbatim including workspace logic — rejected: Hermes has no workspace-first config, so that code would be dead and misleading. |
| **Rationale** | Detect-only + manual-install error is identical to OpenClaw; the YAML MCP strategy is the only structural novelty. Global scope means no `run.go`/`sync.go` routing changes. |

### Decision 5 — Persona Option B: dedicated Hermes assets for BOTH gentleman AND neutral

This is the core persona decision and resolves the neutral question explicitly.

**Finding (verified in code):** `generic/persona-neutral.md` (lines 50-56) ALSO contains the
`## Contextual Skill Loading (MANDATORY)` block referencing `<available_skills>`. The mismatch
affects BOTH personas, not just gentleman. Today `personaContent()` (`persona/inject.go`) handles
`PersonaNeutral` with a single non-per-agent `assets.MustRead("generic/persona-neutral.md")` and
has NO per-agent neutral switch — gentleman is the only per-agent branch.

| | |
|---|---|
| **Choice** | Create TWO Hermes persona assets and refactor `personaContent()` to support per-agent neutral: <br>• `internal/assets/hermes/persona-gentleman.md` (copy of generic gentleman, skill-loading block rewritten) <br>• `internal/assets/hermes/persona-neutral.md` (copy of generic neutral, skill-loading block rewritten) |
| **Alternatives** | (a) Keep neutral on the generic asset (Option b in #4747) — rejected: neutral users would still get the wrong `<available_skills>` instruction for Hermes; that is exactly the bug Option B exists to fix. (b) Strip the skill-loading block entirely for Hermes instead of rewriting it — rejected: Hermes DOES have skills (`~/.hermes/skills/`); we want a correct instruction, not a missing one. |
| **Rationale** | The skill-loading mismatch is persona-independent, so the fix must be persona-independent. In Hermes, `SOUL.md` IS the agent identity, so persona→SOUL maps cleanly for both variants. |

**Rewritten skill-loading block** (same heading, Hermes-native body) for both Hermes assets:

```markdown
## Contextual Skill Loading (MANDATORY)

Your skills live under `~/.hermes/skills/`, organized by category. They are part of your
native skill set — there is no `<available_skills>` system-prompt block.

**Self-check BEFORE every response**: does this request match one of your installed skills
in `~/.hermes/skills/`? If yes, load and follow that skill's `SKILL.md` BEFORE generating
your reply. This is a blocking requirement, not optional context. Skipping it is a discipline
failure.

Multiple skills can apply at once. Match by file context (extensions, paths) and task context
(what the user is asking for).
```

**Exact `personaContent()` changes** (`internal/components/persona/inject.go`):

The current function (verbatim) is:

```go
func personaContent(agent model.AgentID, persona model.PersonaID) string {
	switch persona {
	case model.PersonaNeutral:
		return assets.MustRead("generic/persona-neutral.md")
	case model.PersonaCustom:
		return ""
	default:
		// Gentleman persona — try agent-specific asset, then generic fallback.
		switch agent {
		case model.AgentClaudeCode:
			return assets.MustRead("claude/persona-gentleman.md")
		...
		default:
			return assets.MustRead("generic/persona-gentleman.md")
		}
	}
}
```

Change the `PersonaNeutral` case to a per-agent inner switch (mirroring the gentleman branch),
and add a Hermes case to the gentleman branch:

```go
	case model.PersonaNeutral:
		switch agent {
		case model.AgentHermes:
			return assets.MustRead("hermes/persona-neutral.md")
		default:
			return assets.MustRead("generic/persona-neutral.md")
		}
```

```go
		// inside the gentleman (default) branch's inner agent switch:
		case model.AgentHermes:
			return assets.MustRead("hermes/persona-gentleman.md")
```

**Persona variant behavior (must all work):**

| Install persona | `personaContent` returns for Hermes | Notes |
|---|---|---|
| `gentleman` | `hermes/persona-gentleman.md` | `isGentlemanConversationPersona` = true |
| `gentleman-neutral-artifacts` | `hermes/persona-gentleman.md` | same gentleman branch (falls into default) |
| `neutral` | `hermes/persona-neutral.md` | NEW per-agent neutral path |
| `custom` | `""` (no-op) | unchanged |

Hermes uses `StrategyMarkdownSections`, so persona is injected as a `<!-- gentle-ai:persona -->`
marker section by the standard flow (no `injectOpenClawSoulPersona`-style special case). Because
`StrategyMarkdownSections` uses markers, `preserveManagedSections` (which is `StrategyFileReplace`/
`StrategyInstructionsFile`-only) is NOT involved — there is no duplication risk between persona and
engram/SDD sections; each lives in its own marker block.

### Decision 6 — Engram MCP injection wiring for YAML

| | |
|---|---|
| **Choice** | Add `case model.StrategyMergeIntoYAML:` to the MCP switch in `engram/inject.go`'s `injectWithOptions`, calling a new `engramYAMLOverlay` path that upserts the `engram` block under `mcp_servers:` via the new `filemerge` helper, using `stableEngramCommandForMergedConfig` for the command. Add `model.AgentHermes` to `isStandardAgent()`. **YAML command recovery is IN SCOPE** (see Decision 9): `existingMergedEngramCommand` gains a Hermes early branch so a prior YAML `command` is recovered (not clobbered). |
| **Rationale** | Engram already resolves a stable command per strategy; Hermes reuses that. Adding Hermes to `isStandardAgent` makes it prefer the stable `engram` command (Homebrew/relative) like other first-class agents when nothing is recovered. The engram protocol markdown injection is already handled by the existing `StrategyMarkdownSections` branch of step 2 — no Hermes-specific work there. |

### Decision 9 — YAML engram-command recovery (in scope this slice)

| | |
|---|---|
| **Choice** | Make `stableEngramCommandForMergedConfig` recover a prior engram command from `config.yaml`, exactly like it already does for JSON agents, by teaching `existingMergedEngramCommand` to read YAML. Add an early branch at the top of `existingMergedEngramCommand` (after the `len(raw)==0` guard, before the JSON `MergeJSONObjects` call): `if agentID == model.AgentHermes { return filemerge.ReadYAMLMCPServerCommand(string(raw), "engram") }`. Recovery uses the new read-only helper `filemerge.ReadYAMLMCPServerCommand`. |
| **Alternatives** | (a) Defer to a future refinement and fall back to stable `engram` (the original first-slice plan) — rejected by user: re-running gentle-ai would clobber a user's customized YAML engram command (e.g. an absolute path written by `engram setup`) with bare `engram`. (b) Add `gopkg.in/yaml.v3` to parse for recovery — rejected: inconsistent with the write-side hand-rolled decision (Decision 2); recovery only needs to find one `command` scalar/list, which block scanning handles. |
| **Rationale** | Recovery parity removes the only behavioral gap between Hermes and JSON agents on re-run. Placing the branch before the JSON parser means YAML never hits `MergeJSONObjects` (which would fail and silently lose the command). The branch generalizes: any future YAML agent can route to the same helper. Recovered commands still flow through `stableEngramCommandForExisting`, so a versioned Homebrew cellar path recovered from YAML is stabilized to `engram`/the stable path — identical to JSON agents. When nothing is recovered, `isStandardAgent(AgentHermes)=true` yields the stable `engram` fallback. |

**Control flow after the change:**

```
stableEngramCommandForMergedConfig(path, AgentHermes)
  └─ osReadFile(path) ok?
       └─ existingMergedEngramCommand(raw, AgentHermes)
            ├─ len(raw)==0 -> ("", false)
            ├─ agentID==AgentHermes -> filemerge.ReadYAMLMCPServerCommand(string(raw), "engram")   ◀ NEW
            └─ (JSON agents) MergeJSONObjects + json.Unmarshal + per-agent key switch
       ├─ recovered (cmd, true) -> stableEngramCommandForExisting(cmd, AgentHermes)   // cellar-version stabilization, reused unchanged
       └─ not recovered -> isStandardAgent(AgentHermes)=true -> preferredStableEngramCommand()  // stable "engram"
```

### Decision 7 — engram-vs-Hermes-native-memory documentation placement

| | |
|---|---|
| **Choice** | Document the complementary relationship inside the **Hermes persona asset** (`hermes/persona-gentleman.md` and `hermes/persona-neutral.md`) as a short subsection, NOT in the shared `claude/engram-protocol.md`. |
| **Alternatives** | (a) Add it to `claude/engram-protocol.md` — rejected: that asset is shared by every agent; Hermes-specific text would leak into all of them. (b) A separate `<!-- gentle-ai:hermes-memory-note -->` marker section injected by the engram component — rejected: adds a Hermes-only branch to the engram injector for one paragraph; the persona asset is already Hermes-specific and always present. |
| **Rationale** | Keeps Hermes-specific prose in Hermes-specific assets; avoids polluting shared engram content and avoids new injector branches. No duplication because the engram protocol section and the persona memory note address different things (the protocol = how to use engram tools; the note = how engram and Hermes-native memory coexist). |

### Decision 8 — Native skill-format risk: write, with a documented assumption to verify in apply

| | |
|---|---|
| **Choice** | Write gentle-ai `SKILL.md` files into `~/.hermes/skills/` (`SupportsSkills()=true`) using the standard SDD skills flow, but record a **documented assumption** that Hermes tolerates gentle-ai's `SKILL.md` frontmatter format. The SDD orchestrator instructions live in `SOUL.md` regardless, so even if Hermes ignores or rejects unknown skill files, the orchestrator protocol is still loaded. |
| **Recommendation** | Do NOT gate skill writing behind a feature flag for the first slice. The blast radius is contained: worst case Hermes ignores files it does not recognize in `~/.hermes/skills/`. The apply phase should add a verification step (manual or via Hermes docs) confirming the format is accepted; if Hermes rejects it, a follow-up change gates or transforms the format. |
| **Rationale** | SOUL.md is the guaranteed-loaded surface, so functionality does not depend on skill-file acceptance. Writing skills is consistent with every other TierFull agent and avoids special-casing on day one. |

---

## YAML Helper API Contract (`internal/components/filemerge/yaml.go`)

Mirror `toml.go` semantics: pure string manipulation, normalize `\r\n`→`\n`, strip any existing
gentle-ai-managed block, re-append a fresh block, return content ending in a single trailing `\n`.
2-space indentation throughout. No external dependency.

### Function signatures

```go
// UpsertYAMLMCPServerBlock removes any existing `<serverID>:` block nested under the
// top-level `mcp_servers:` key and re-appends a fresh block with command/args (and
// optional env). If `mcp_servers:` is absent, it is created. Everything outside the
// managed server block — other servers, top-level keys, and user comments — is preserved.
//
// command/args/env are emitted with 2-space indentation:
//
//	mcp_servers:
//	  <serverID>:
//	    command: <command>
//	    args:
//	      - <arg0>
//	      - <arg1>
//	    env:
//	      KEY: value
//
// Idempotent: calling twice with the same arguments yields identical output.
func UpsertYAMLMCPServerBlock(content, serverID, command string, args []string, env map[string]string) string

// UpsertHermesEngramBlock is a thin convenience wrapper that upserts the canonical
// engram MCP server block (command=engramCmd, args=["mcp","--tools=agent"], no env).
// Falls back to "engram" when engramCmd is empty. Mirrors UpsertCodexEngramBlock.
func UpsertHermesEngramBlock(content, engramCmd string) string

// UpsertHermesContext7Block is a thin convenience wrapper that upserts the canonical
// context7 MCP server block. Context7 is a remote/stdio server; the first slice uses
// the same stdio shape as Codex (command + pinned args from versions.Context7MCP).
func UpsertHermesContext7Block(content string) string

// ReadYAMLMCPServerCommand recovers the executable of a named MCP server's
// `command` from a YAML config (read-only — never mutates). It is the YAML
// counterpart of the JSON path inside engram's existingMergedEngramCommand,
// enabling gentle-ai to preserve a command already written for a server
// (e.g. an absolute path) instead of clobbering it on re-run.
//
// Algorithm (hand-rolled, NO gopkg.in/yaml.v3 — read-only block scanning,
// consistent with the write-side decision):
//   1. Normalize line endings; split into lines; ignore full-line comments (# ...).
//   2. Locate the top-level `mcp_servers:` line (trimmed == "mcp_servers:" AND
//      zero leading indent).
//   3. Within its child region (indent > 0, until the next zero-indent non-blank
//      line or EOF), find the `  <serverID>:` block at exactly 2-space indent.
//   4. Within that server's sub-block (indent deeper than the server key, up to
//      the next 2-space sibling or end of region), read `command:`:
//        • scalar string  -> `command: engram`     => "engram"
//        • YAML list       -> `command:` then `- engram` items => first element
//   5. Return ("", false) when mcp_servers / serverID / command is absent or the
//      file does not match the expected shape.
//
// Generalizes to any future YAML-backed agent; engram dispatches to it for Hermes.
func ReadYAMLMCPServerCommand(content string, serverID string) (string, bool)
```

> Implementation note for apply: prefer building `UpsertHermesEngramBlock` /
> `UpsertHermesContext7Block` on top of `UpsertYAMLMCPServerBlock` to keep one block-writing
> code path. `env` is included in the general signature for forward-compat even though the first
> slice's two servers do not use it (pass `nil`).

### Block-upsert semantics (the core algorithm)

1. Normalize line endings (`\r\n`→`\n`); split into lines.
2. Locate the top-level `mcp_servers:` line (trimmed == `mcp_servers:` AND zero leading indent).
3. **If `mcp_servers:` is present:** scan its child lines (indent > 0, until the next zero-indent
   non-blank line or EOF). Within that region, find an existing `  <serverID>:` (2-space indent)
   and drop that server's sub-block (all lines indented deeper than the server key, up to the next
   sibling server key at 2-space indent or the end of the `mcp_servers` region). Keep all other
   servers and any comments verbatim.
4. **Re-append** the fresh `  <serverID>:` block under the existing `mcp_servers:` key (insert at
   the end of the `mcp_servers` child region, preserving sibling servers and their comments).
5. **If `mcp_servers:` is absent (first run):** append `mcp_servers:\n  <serverID>:\n ...` at EOF,
   preserving all existing top-level content above it.
6. **If the file is absent/empty:** return just `mcp_servers:\n  <serverID>:\n ...\n` (the MCP/engram
   injectors read with `readFileOrEmpty`/`osReadFile` which return `""`/`nil` for missing files —
   the helper must treat empty content as "create from scratch", exactly like `toml.go` does).
7. Always end output with exactly one trailing `\n`.

> Difference from `toml.go`: TOML blocks are flat (`[mcp_servers.engram]` header + sibling
> key lines until the next `[`). YAML requires **indent-aware** block boundary detection because
> servers are nested two levels deep. This is the only added complexity and is the primary reason
> for an explicit golden-test matrix.

### Idempotency & comment-preservation contract

- Re-running with identical inputs MUST produce byte-identical output (drives `WriteFileAtomic`'s
  `Changed=false` no-op on the second run).
- Content **outside** any gentle-ai-managed server block is preserved verbatim, including:
  user comments (`# ...`), other top-level keys, and other MCP servers (user-defined or a sibling
  gentle-ai server).
- Comments **inside** a managed server block are gentle-ai-owned and may be lost on re-write
  (acceptable — same trade-off as Codex's managed blocks).

### Golden-test matrix (`yaml_test.go`)

| # | Scenario | Input | Expected |
|---|---|---|---|
| 1 | Empty/absent file → engram | `""` | `mcp_servers:` + nested `engram` block, 2-space indent |
| 2 | `mcp_servers:` absent, other top-level keys present | flat keys + comments | keys/comments preserved; `mcp_servers:` appended at EOF |
| 3 | `mcp_servers:` present, no engram | one user server under `mcp_servers:` | user server preserved; engram appended as sibling |
| 4 | Idempotency | output of #3 fed back in | byte-identical (no change) |
| 5 | Upsert replaces stale engram | engram with old args | old engram block removed; fresh block appended; siblings intact |
| 6 | User comments outside block | comments above/below `mcp_servers:` | all comments preserved |
| 7 | Two managed servers coexist | engram then context7 | both present, both at 2-space indent, both idempotent |
| 8 | CRLF input | `\r\n` line endings | normalized to `\n`, single trailing `\n` |
| 9 | env map rendered | server with `env` | `env:` sub-block with 2-space-deeper KV pairs |
| 10 | context7 wrapper | `UpsertHermesContext7Block` on empty | pinned `versions.Context7MCP` args |

---

## Dispatch Wiring

### `internal/components/mcp/inject.go`

Add a case to the `Inject` switch and a new `injectYAMLFile` function (mirrors `injectTOMLFile`):

```go
case model.StrategyMergeIntoYAML:
	return injectYAMLFile(homeDir, adapter)
```

```go
func injectYAMLFile(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	configPath := adapter.MCPConfigPath(homeDir, "context7")
	existingBytes, err := osReadFile(configPath) // returns nil for missing file
	if err != nil { ... }
	updated := filemerge.UpsertHermesContext7Block(string(existingBytes))
	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
	...
	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}
```

### `internal/components/engram/inject.go`

Add a case to the MCP-strategy switch inside `injectWithOptions` (step 1):

```go
case model.StrategyMergeIntoYAML:
	configPath := adapter.MCPConfigPath(configHomeDir, "engram")
	if configPath == "" { break }
	existing, err := readFileOrEmpty(configPath)
	if err != nil { return InjectionResult{}, err }
	engramCmd := stableEngramCommandForMergedConfig(configPath, adapter.Agent())
	updated := filemerge.UpsertHermesEngramBlock(existing, engramCmd)
	yamlWrite, err := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
	...
	changed = changed || yamlWrite.Changed
	files = append(files, configPath)
```

Add Hermes to `isStandardAgent`:

```go
func isStandardAgent(id model.AgentID) bool {
	switch id {
	case ..., model.AgentOpenClaw, model.AgentHermes:
		return true
	...
	}
}
```

Add the YAML recovery early branch to `existingMergedEngramCommand` (Decision 9). It goes
AFTER the `len(raw)==0` guard and BEFORE the JSON `MergeJSONObjects` call:

```go
func existingMergedEngramCommand(raw []byte, agentID model.AgentID) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}

	if agentID == model.AgentHermes { // generalizes to any future YAML agent
		return filemerge.ReadYAMLMCPServerCommand(string(raw), "engram")
	}

	normalized, err := filemerge.MergeJSONObjects(raw, []byte("{}"))
	// ... existing JSON path unchanged ...
}
```

> `MCPConfigPath(homeDir, _)` returns `~/.hermes/config.yaml` for both context7 and engram —
> a single config file, so the `serverName` argument is ignored (same pattern as Codex).
> Engram's step-2 system-prompt injection already covers Hermes via the existing
> `StrategyMarkdownSections` branch — no extra Hermes code needed there.
> `stableEngramCommandForMergedConfig` recovers a prior engram command from the existing config.
> For Hermes, `existingMergedEngramCommand` now branches to `filemerge.ReadYAMLMCPServerCommand`
> BEFORE the JSON parser (so YAML never hits `MergeJSONObjects`), recovering a customized YAML
> `command` (e.g. an absolute path) so re-runs do not clobber it. Recovered commands flow through
> `stableEngramCommandForExisting` (cellar-version stabilization, reused unchanged). When nothing
> is recovered, `isStandardAgent(AgentHermes)=true` yields the stable `engram` fallback. See
> Decision 9 for the full control flow.

---

## Adapter Method Table (`internal/agents/hermes/adapter.go`)

`ConfigPath(homeDir)` = `filepath.Join(homeDir, ".hermes")`.

| Method | Return value | Notes |
|---|---|---|
| `Agent()` | `model.AgentHermes` | new constant `"hermes"` |
| `Tier()` | `model.TierFull` | |
| `Detect(ctx, homeDir)` | `lookPath("hermes")` for installed/binaryPath; stat `~/.hermes` for configFound | mirrors OpenClaw/Codex |
| `SupportsAutoInstall()` | `false` | LOCKED detect-only |
| `InstallCommand(profile)` | `nil, AgentNotInstallableError{Agent: a.Agent()}` | mirror OpenClaw |
| `GlobalConfigDir(homeDir)` | `~/.hermes` | |
| `SystemPromptDir(homeDir)` | `~/.hermes` | |
| `SystemPromptFile(homeDir)` | `~/.hermes/SOUL.md` | global path → standard flow |
| `SkillsDir(homeDir)` | `~/.hermes/skills` | |
| `SettingsPath(homeDir)` | `~/.hermes/config.yaml` | the YAML config file |
| `SystemPromptStrategy()` | `model.StrategyMarkdownSections` | |
| `MCPStrategy()` | `model.StrategyMergeIntoYAML` | new enum |
| `MCPConfigPath(homeDir, _)` | `~/.hermes/config.yaml` | single file; serverName ignored |
| `SupportsOutputStyles()` | `false` | |
| `OutputStyleDir(_)` | `""` | |
| `SupportsSlashCommands()` | `false` | no documented commands dir |
| `CommandsDir(_)` | `""` | |
| `SupportsSubAgents()` | `false` | |
| `SubAgentsDir(_)` | `""` | |
| `EmbeddedSubAgentsDir()` | `""` | |
| `SupportsSkills()` | `true` | writes `~/.hermes/skills/` (see Decision 8) |
| `SupportsSystemPrompt()` | `true` | |
| `SupportsMCP()` | `true` | |

The adapter struct mirrors OpenClaw: injectable `lookPath`/`statPath`, a package-level
`LookPathOverride = exec.LookPath`, a `defaultStat`, an `AgentNotInstallableError` type, and a
`ConfigPath(homeDir)` helper. NO `resolveWorkspaceDir` / workspace-path validation (global-only).

---

## Other Component & Registration Changes (additive)

| File | Change |
|---|---|
| `internal/model/types.go` | `AgentHermes AgentID = "hermes"`; `StrategyMergeIntoYAML MCPStrategy = 4` |
| `internal/agents/factory.go` | import hermes; add to `NewAdapter()` + default registry |
| `internal/catalog/agents.go` | `{ID: AgentHermes, Name: "Hermes", Tier: TierFull, ConfigPath: "~/.hermes"}` |
| `internal/assets/assets.go` | add `all:hermes` to the `//go:embed` directive |
| `internal/assets/hermes/` | NEW: `persona-gentleman.md`, `persona-neutral.md`, `sdd-orchestrator.md` |
| `internal/components/sdd/inject.go` | `case model.AgentHermes: return "hermes/sdd-orchestrator.md"` in `sddOrchestratorAsset()` |
| `internal/components/persona/inject.go` | per-agent neutral switch + `AgentHermes` gentleman case (Decision 5) |
| `internal/components/engram/inject.go` | `StrategyMergeIntoYAML` case + `AgentHermes` in `isStandardAgent` (Decision 6) |
| `internal/components/engram/setup.go` | `case model.AgentHermes: return "", false` |
| `internal/components/mcp/inject.go` | `StrategyMergeIntoYAML` case + `injectYAMLFile` |
| `internal/components/permissions/inject.go` | `case model.AgentHermes: return nil` (skip; format unknown) |
| `internal/system/config_scan.go` | `{Agent: "hermes", Path: filepath.Join(homeDir, ".hermes")}` |
| `internal/cli/validate.go` | `case string(model.AgentHermes)` |
| `internal/tui/model.go` | `case string(model.AgentHermes)` in `loadSelection()` |

**No changes** to `run.go` / `sync.go`: Hermes is global-only, so there is no workspace routing
(this is the key contrast with OpenClaw, whose workspace-first `SOUL.md` requires special routing).

---

## Data Flow

```
TUI/CLI: user selects "hermes" (validate.go accepts; tui loadSelection restores)
  │
  ▼
Planner: standard component order (persona → engram → context7 → sdd → skills …)
  │
  ▼
Pipeline (all paths use ~/.hermes/ — global, no workspace dir):
  ├── persona:  inject <!-- gentle-ai:persona --> into SOUL.md
  │             (hermes/persona-gentleman.md OR hermes/persona-neutral.md)
  ├── engram:   stableEngramCommandForMergedConfig (recovers prior YAML command via
  │             ReadYAMLMCPServerCommand) → UpsertHermesEngramBlock → config.yaml
  │             mcp_servers.engram + InjectMarkdownSection engram-protocol → SOUL.md
  ├── context7: UpsertHermesContext7Block → config.yaml mcp_servers.context7
  ├── sdd:      InjectMarkdownSection sdd-orchestrator (+ strict-tdd-mode) → SOUL.md
  │             + write SDD skill files → ~/.hermes/skills/
  └── skills:   write selected skill files → ~/.hermes/skills/
  │
  ▼
Verify: SOUL.md contains persona + engram-protocol + sdd-orchestrator markers;
        config.yaml contains mcp_servers.engram + mcp_servers.context7 blocks.
```

---

## Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| YAML indentation errors (silently invalid config) | Med | Strict 2-space indent in helper; golden tests #1-#10 pin exact bytes |
| Comment loss inside managed blocks | Low | Documented: managed blocks are gentle-ai-owned; content outside preserved (golden #6) |
| Idempotency when `mcp_servers:` created on first run | Med | Golden #2 + #4 (create then re-run = no change) |
| Indent-aware block boundary bugs (nested-2-deep vs flat TOML) | Med | Explicit boundary rules in algorithm; golden #5/#7 cover sibling preservation |
| Hermes config.yaml schema drift (emerging agent) | Med | Schema knowledge isolated in `yaml.go`; pinned by golden tests; easy single-file update |
| YAML engram-command recovery (parsing the wrong shape) | Low | NOW IN SCOPE (Decision 9): `ReadYAMLMCPServerCommand` recovers the prior YAML `command` so re-runs do not clobber a customized path. Read-only block scanning; covered by `yaml_test.go` table tests (scalar, list, absent server, absent `mcp_servers`, comment lines) + a Hermes recovery case in `engram/inject_test.go`. Returns `("", false)` on any unexpected shape → safe stable `engram` fallback. |
| engram vs Hermes-native memory confusion | Med | Documented in Hermes persona asset (Decision 7) |
| Native skill-format rejection | Med | SOUL.md carries the orchestrator regardless; documented assumption to verify in apply (Decision 8) |
| Profiles not covered (`~/.hermes/profiles/`) | Low | Documented non-goal |

---

## Testing Strategy (per go-testing skill)

| Layer | Target | Pattern |
|---|---|---|
| Unit | YAML write helpers (`UpsertYAMLMCPServerBlock`, engram/context7 wrappers) | Table-driven golden matrix #1-#10; deterministic; `-update` path |
| Unit | YAML read helper (`ReadYAMLMCPServerCommand`) | Table-driven `yaml_test.go`: scalar `command: engram`; list `command:` with `- /path/engram` (first element); absent server under `mcp_servers`; absent `mcp_servers` key; comment lines ignored; recovered versioned cellar path |
| Unit | `Detect()` — binary found/missing, stat error, config dir present/absent | Table-driven with injected `lookPath`/`statPath` mocks (mirror OpenClaw/qwen) |
| Unit | `InstallCommand()` returns `AgentNotInstallableError` | Explicit error-type assertion |
| Unit | Config paths + capabilities + strategies | Table-driven name/expected pairs |
| Unit | `personaContent()` — gentleman/neutral/gentleman-neutral-artifacts/custom for Hermes | Table-driven; assert Hermes assets selected and skill-loading block rewritten |
| Integration | engram MCP YAML inject (`engram/inject_test.go`) | `t.TempDir()`; call `Inject`; assert `mcp_servers.engram` present + idempotent on re-run |
| Integration | engram YAML command recovery (`engram/inject_test.go`) | `t.TempDir()` with a `config.yaml` whose `mcp_servers.engram.command` is a custom absolute path; call `Inject`; assert the custom command is preserved (not replaced with bare `engram`); a versioned cellar command is stabilized to `engram`/stable path |
| Integration | context7 MCP YAML inject (`mcp/inject_test.go`) | `t.TempDir()`; assert `mcp_servers.context7` present |
| Integration | SDD inject (`sdd/inject_test.go`) | `t.TempDir()`; assert SOUL.md sdd-orchestrator marker + `~/.hermes/skills/` files; assert asset selection case |
| Integration | persona inject into SOUL.md | `t.TempDir()`; assert `<!-- gentle-ai:persona -->` section; assert engram/SDD sections coexist without duplication |
| Unit | `SetupAgentSlug(AgentHermes)` → `("", false)` (`setup_test.go`) | Table case |
| Registry | default registry includes Hermes (`registry_test.go`) | Extend `TestDefaultRegistryIncludesAllAgents` |
| CLI | validate accepts `"hermes"` | Extend mapping test |
| TUI | state restoration includes hermes (`model_test.go`) | Add to `makeDetectionWithAgents` known agents |
| System | `config_scan_test.go` agent count updated | Adjust total |

Golden files must be deterministic and updated only via the repo's `-update` path, then re-run
without `-update`. Use `t.TempDir()` for all filesystem tests; never touch a real home directory.

---

## Migration / Rollout

Fully additive. New adapter package, new asset directory, new enum value, and new switch cases
only — no existing case is modified except the `personaContent` neutral branch, which gains a
per-agent inner switch whose `default` returns the exact previous asset (`generic/persona-neutral.md`),
so every non-Hermes agent's behavior is byte-identical. No data migration required.

## Canonical pattern sources

- `openspec/changes/qwen-code-integration/` — most recent end-to-end agent integration (task slicing).
- `internal/agents/openclaw/adapter.go` — detect-only + `AgentNotInstallableError` + SOUL.md target.
- `internal/agents/codex/adapter.go` + `internal/components/filemerge/toml.go` — non-JSON string-merge precedent for the YAML helpers.
