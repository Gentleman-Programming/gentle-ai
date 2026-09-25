# Agentes compatibles con Axiom

← [Back to README](../README.md)

---

## Agent Matrix

| Agent           | ID               | Skills       | MCP | Delegation                       | Output Styles | Slash Commands | Config Path                         |
| --------------- | ---------------- | ------------ | --- | -------------------------------- | ------------- | -------------- | ----------------------------------- |
| Claude Code     | `claude-code`    | Yes          | Yes | Full (Task tool)                 | Yes           | No             | `~/.claude`                         |
| OpenCode        | `opencode`       | Yes          | Yes | Full (multi-mode overlay)        | No            | Yes            | `~/.config/opencode`                |
| Kilo Code       | `kilocode`       | Yes          | Yes | Full (multi-mode overlay)        | No            | Yes            | `~/.config/kilo`                    |
| Gemini CLI      | `gemini-cli`     | Yes          | Yes | Full (experimental)              | No            | No             | `~/.gemini`                         |
| Cursor          | `cursor`         | Yes          | Yes | Full (native subagents)          | No            | No             | `~/.cursor`                         |
| VS Code Copilot | `vscode-copilot` | Yes          | Yes | Full (runSubagent)               | No            | No             | `~/.copilot` + VS Code User profile |
| Codex           | `codex`          | Yes          | Yes | Native multi-agent (default; solo fallback) | No            | No             | `~/.codex`                          |
| Windsurf        | `windsurf`       | Yes (native) | Yes | Solo-agent                       | No            | No             | `~/.codeium/windsurf`               |
| Antigravity     | `antigravity`    | Yes (native) | Yes | Solo-agent + Mission Control     | No            | No             | `~/.gemini/antigravity`             |
| Kimi Code       | `kimi`           | Yes          | Yes | Full (native custom agents)      | Via KIMI.md include [^kimi-output-style] | No | `~/.kimi`                           |
| Qwen Code       | `qwen-code`      | Yes          | Yes | Full (native sub-agents)         | No            | Yes            | `~/.qwen`                           |
| Kiro IDE        | `kiro-ide`       | Yes          | Yes | Full (native subagents)          | No            | No             | `~/.kiro`                           |
| OpenClaw        | `openclaw`       | Yes          | Yes | Solo-agent                       | No            | No             | `~/.openclaw`                       |
| Trae            | `trae-ide`       | Yes          | Yes | Solo-agent                       | No            | No             | `~/.trae`                           |
| Pi              | `pi`             | Yes          | Yes | Full (package-managed subagents) | No            | Yes            | `~/.pi`                             |
| Hermes          | `hermes`         | Yes          | Yes | Full (delegate_task ephemeral)   | No            | No             | `~/.hermes`                         |

La mayoría de los agentes recibe la política completa del orquestador SDD y las skills correspondientes en su directorio de configuración. Axiom la incorpora al prompt de sistema; OpenCode y Kilo Code la reciben mediante el overlay compatible con OpenCode (`opencode.json`). Pi es la excepción: Axiom aprovisiona paquetes de Pi y `gentle-pi` —nombre del paquete externo— gestiona las skills, los prompts, los agentes SDD y las cadenas durante la ejecución. El agente usa SDD cuando el trabajo lo requiere o cuando se lo pides.

`axiom install --scope=workspace` permite limitar al workspace los ficheros de agente compatibles, no solo los de Claude Code. En ese ámbito, Axiom escribe prompts de sistema, skills, agentes SDD y ficheros de persona en el proyecto actual cuando el agente admite configuración local. Las integraciones globales —por ejemplo, paquetes o ajustes que el agente solo lee desde su configuración de usuario— permanecen globales. Algunos nombres de ruta, paquetes, marcadores y variables `gentle-ai` o `GENTLE_AI_*` se conservan como identificadores heredados de compatibilidad; no son el comando actual del producto, que es `axiom`.

[^kimi-output-style]: Kimi has no `settings.json` `outputStyle` mechanism like Claude Code. Instead, `KIMI.md` unconditionally includes `output-style.md` as a Jinja module — the canonical tone/language/philosophy channel for Kimi's persona (`persona.md` carries only tooling/action directives plus a pointer to this module).

---

## Delegation Models

| Model                 | How It Works                                                                                                                                                                                       | Agents                                                                                                    |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **Full (sub-agents)** | Each SDD phase runs in an isolated context window via native sub-agent delegation, package-managed subagents, or an OpenCode-compatible overlay. The orchestrator coordinates; sub-agents execute. | Claude Code, OpenCode, Kilo Code, Gemini CLI, Cursor, VS Code Copilot, Kimi Code, Kiro IDE, Qwen Code, Pi |
| **Full (delegate_task)** | The orchestrator uses Hermes's native `delegate_task` primitive to spawn ephemeral workers in fresh context windows. Workers receive only a self-contained mission; the parent receives only their final summary. Toolsets, MCP, and skills must be passed explicitly (not inherited by default). | Hermes |
| **Native multi-agent** | The orchestrator delegates through the agent's native collaboration tools when configured and available, with inline execution as a graceful fallback. | Codex |
| **Solo-agent**        | All SDD phases run inline in the same conversation. The orchestrator IS the executor. Engram™ provides cross-phase persistence.                                                                     | Windsurf, Antigravity, OpenClaw, Trae                                                                     |

### Cursor Native Subagents

Cursor usa su sistema integrado `.cursor/agents/`. Axiom escribe ficheros de agentes en `~/.cursor/agents/sdd-{phase}.md`; Cursor delega según el campo `description` de la cabecera YAML de cada fichero.

- `sdd-explore` and `sdd-verify` run with `readonly: false` so they can inspect the codebase and execute verification commands
- Each subagent gets its own context window (fresh context, no pollution)
- The orchestrator resolves skill paths from the skill registry and passes exact `SKILL.md` files in the invocation message

### Windsurf Cascade

Windsurf runs as a solo-agent (no custom sub-agents). The orchestrator leverages Windsurf-native features:

- **Plan Mode** — creates persistent plan documents that can be @mentioned across sessions; ideal for spec and design artifacts on large changes
- **Code Mode** — default agentic execution mode
- **Native Workflows** — `sdd-new` is available as a `.windsurf/workflows/sdd-new.md` workflow
- **Size Classification** — the orchestrator routes tasks through Small/Medium/Large decision paths

### Antigravity + Mission Control

Antigravity is an agent-first platform with built-in sub-agents (Browser, Terminal) managed by Mission Control. However, custom sub-agent creation is not yet available. SDD phases run inline, with Mission Control handling automatic delegation to built-in sub-agents when specialized tooling is needed (e.g., Browser for research during `sdd-explore`).

### Kiro Native Subagents

Kiro usa agentes personalizados en `~/.kiro/agents/`. Axiom escribe los agentes de fase (`sdd-init` a `sdd-onboard` y los agentes de Judgment Day) y traduce el campo `model:` de las asignaciones de Kiro (`auto|opus|sonnet|haiku|minimax|glm|deepseek|qwen`) a los identificadores nativos de Kiro.

- Frontmatter includes `includeMcpJson: true` for all phase agents
- Phase-specific tools are preserved (`sdd-explore` and `sdd-verify` use read/shell/context7 as required)
- El orquestador permanece en el fichero de steering `~/.kiro/steering/gentle-ai.md`; el nombre conserva la ruta heredada y el contenido delega en los agentes nativos.

---

## SDD Mode Support

| Feature          | Claude Code | OpenCode | Kilo Code | Gemini CLI | Cursor | VS Code Copilot | Codex | Windsurf | Antigravity | Kiro IDE | Qwen Code | OpenClaw | Trae |   Pi    | Hermes |
| ---------------- | :---------: | :------: | :-------: | :--------: | :----: | :-------------: | :---: | :------: | :---------: | :------: | :-------: | :------: | :--: | :-----: | :----: |
| SDD orchestrator |     Yes     |   Yes    |    Yes    |    Yes     |  Yes   |       Yes       |  Yes  |   Yes    |     Yes     |   Yes    |    Yes    |   Yes    | Yes  |   Yes   |  Yes   |
| Single-mode SDD  |     Yes     |   Yes    |    Yes    |    Yes     |  Yes   |       Yes       |  Yes  |   Yes    |     Yes     |   Yes    |    Yes    |   Yes    | Yes  |   Yes   |  Yes   |
| Multi-mode SDD   |      —      |   Yes    |    Yes    |     —      |   —    |        —        |   —   |    —     |      —      |  Yes\*   |     —     |    —     |  —   | Yes\*\* |   —    |

**Multi-mode** (assigning different AI models to each SDD phase) is supported by **OpenCode** and **Kilo Code** through the OpenCode-compatible multi-mode overlay, and by **Kiro IDE** through native subagent `model:` frontmatter. All other agents run in **single-mode** — the orchestrator manages everything using whatever model the agent is already running.

> \* **Kiro multi-mode** assigns models per phase through `KiroModelAssignments` (configured via _Configure Models → Configure Kiro models_ in the TUI). The selected Kiro alias (`auto|opus|sonnet|haiku|minimax|glm|deepseek|qwen`) is resolved to a Kiro-native model ID and stamped into each `~/.kiro/agents/sdd-{phase}.md` at sync time.

> **Pi multi-mode** lo gestionan los paquetes de Pi. `gentle-pi` instala los recursos de agentes SDD y cadenas en `.pi/agents/` y `.pi/chains/`; las asignaciones de modelo viven en esos ficheros o en los pasos de las cadenas.

---

## Agent Notes

### Claude Code

- Sub-agents via the native Task tool with isolated context windows
- Slash commands for SDD phases are namespaced `/gentle-sdd-*` (`/gentle-sdd-init`, `/gentle-sdd-new`, `/gentle-sdd-continue`, etc.) so no command shares a name with a delegate-only SDD skill
- MCP servers configured as plugins in `~/.claude/mcp/`
- Output styles in `~/.claude/output-styles/`
- System prompt via markdown sections in `~/.claude/CLAUDE.md`
- Managed hooks in `~/.claude/settings.json`: `UserPromptSubmit` refreshes the skill registry; `SessionStart` and `Stop` maintain the review reminder baseline. The `PreToolUse(Agent)` SDD guard reads the parent-confirmed grouped `AskUserQuestion` preflight from the session transcript the hook runner supplies, prepends the canonical `## SDD Session Preflight` block to every packaged `sdd-*` phase dispatch, and refuses missing, child-session, model-authored, malformed, or unanswered authority. The guard cannot detect a transcript file deliberately edited through a shell, so it defeats prompt shortcuts, not adversarial forgery. Uninstall also removes stale preflight producer hooks from earlier installations.

### OpenCode

- Full multi-agent overlay with 11 named agents in `opencode.json` (`gentle-orchestrator` plus 10 SDD phase agents)
- Slash commands for SDD phases (`/sdd-new`, `/sdd-explore`, etc.)
- Native OpenCode `task` subagents; the managed task-result plugin records grouped `question` answers only for root sessions, injects the canonical SDD preflight block into every packaged SDD phase, and refuses missing, forged, child-session, malformed, or expired authority; it also canonicalizes the grouped `question` options before they are shown and accepts picked answers tolerantly, so the preflight never falls back to typed chat answers
- La ejecución en segundo plano se configura con `axiom install` / `axiom sync`, usando `--opencode-background-subagents=auto|on|off` o la variable heredada `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS`.
- CLI precedence is flag, non-empty environment, prior managed state, then `auto`; the interactive OpenCode + SDD installer prompts only when that preference is unresolved
- Los launchers gestionados se encuentran bajo `~/.gentle-ai/bin/` —ruta heredada conservada por compatibilidad— y respetan `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=false`; reinicia OpenCode después de activarlos.
- `serve`, `attach`, Desktop, and sessions not launched through the managed launcher use the safe foreground fallback
- Background jobs are process-local and non-durable, have no filesystem isolation, and must not be used for dependent phases or parallel writers in one worktree
- The TUI model picker asynchronously discovers the active project's effective providers and models through `opencode models --verbose`, including custom, authenticated, plugin, and dynamic providers
- Only models OpenCode reports with tool-call capability appear as selectable SDD-capable options
- Antes de seleccionar modelos, conecta tus proveedores de IA y vuelve al selector; Axiom no actualiza el catálogo de OpenCode.
- Axiom establece `share: disabled` como valor predeterminado de los agentes SDD de OpenCode por privacidad y conserva los valores gestionados por el usuario, como `manual` o `auto`.
- OpenCode Desktop SDD commands resolve the project with `git rev-parse --show-toplevel || pwd` before acting, avoiding Electron current-working-directory drift.
- Review launch runs from an ordinary already-running OpenCode session: no restart, child process, special user-visible session, or `OPENCODE_DISABLE_PROJECT_CONFIG` / `OPENCODE_DISABLE_EXTERNAL_SKILLS` variable is required (rdd-advisory-transport SKILL.md).

### Kilo Code

- **Detección**: Axiom detecta Kilo Code en `~/.config/kilo` y comprueba si el ejecutable `kilo` está en `PATH`.
- Uses the OpenCode-compatible adapter: `AGENTS.md`, `skills/`, `commands/`, and `opencode.json` live under `~/.config/kilo`
- Full SDD delegation is provided by the merged multi-agent overlay in `~/.config/kilo/opencode.json`, not by a separate native sub-agent directory
- Kilo no expone la vía de interceptación de herramientas ejecutables gestionada por Axiom; su preflight sigue siendo un bloqueo a nivel del prompt y no declara aplicar autoridad en tiempo de ejecución.
- MCP servers are merged into `opencode.json`; Engram uses the OpenCode-style local MCP entry with `command` as an array
- Auto-install is supported via npm: `npm install -g @kilocode/cli`

### Gemini CLI

- Sub-agents are experimental: require `experimental.enableAgents: true` in `settings.json`
- Custom sub-agents defined as markdown files in `~/.gemini/agents/`

### Cursor

- Subagentes nativos en `~/.cursor/agents/sdd-{phase}.md` (ficheros instalados por Axiom)
- Skills at `~/.cursor/skills/`
- System prompt in `~/.cursor/rules/gentle-ai.mdc`
- MCP config in `~/.cursor/mcp.json`

### VS Code Copilot

- Uses the `runSubagent` tool with support for parallel execution
- Skills at `~/.copilot/skills/`
- System prompt at `Code/User/prompts/gentle-ai.instructions.md`
- MCP config at `Code/User/mcp.json`

### Codex

- CLI-native agent with TOML config at `~/.codex/config.toml`
- Skills at `~/.codex/skills/`
- System prompt at `~/.codex/AGENTS.md`
- Engram instruction files at `~/.codex/engram-instructions.md`
- MCP servers (Engram and Context7) are upserted as `[mcp_servers.<name>]` blocks in `~/.codex/config.toml`
- SDD model-selection profiles written as separate files at `~/.codex/<name>.config.toml`. GPT-5.6 defaults require Codex >= 0.144.0 (the separate-file mechanism itself is available since 0.134.0). Select a profile at runtime via `codex --profile <name>`:

  Los valores predeterminados de modelo y esfuerzo varían según el preset. Estos niveles de esfuerzo son la política de carga de trabajo de Axiom, no valores predeterminados de Codex. Los carriles distinguen el trabajo de cada fase: `sdd-strong` razona sobre el contexto recibido, `sdd-mid` implementa en un bucle agentic y `sdd-cheap` transcribe estructuras con contexto breve y resultados verificables.

  Every curated preset runs the main orchestrator/session at `medium` effort — it plans, routes and adjudicates rather than doing the delegated work. The orchestrator *model* varies: Low-cost runs it on `gpt-5.6-terra` so a Plus plan can still afford `gpt-5.6-sol` in the strong carril, where the reasoning pays. Custom and legacy state preserve existing top-level settings:

  | Profile | Low-cost | Recommended | Powerful | SDD phases |
  |---------|----------|-------------|----------|------------|
  | Orchestrator | `gpt-5.6-terra` / `medium` | `gpt-5.6-sol` / `medium` | `gpt-5.6-sol` / `medium` | main session |
  | `sdd-strong` | `gpt-5.6-sol` / `medium` | `gpt-5.6-sol` / `medium` | `gpt-5.6-sol` / `xhigh` | explore, propose, design, verify, judge |
  | `sdd-mid` | `gpt-5.6-terra` / `medium` | `gpt-5.6-terra` / `high` | `gpt-5.6-sol` / `high` | apply, fix-agent |
  | `sdd-cheap` | `gpt-5.6-luna` / `high` | `gpt-5.6-luna` / `high` | `gpt-5.6-luna` / `high` | spec, tasks, archive, onboard |

- Explicit saved Codex model assignments are preserved on sync, including older pinned IDs such as `gpt-5.5` or `gpt-5.4-mini`. The narrow exception is the exact former implicit-default tuple (`sdd-strong=gpt-5.5`, `sdd-mid=gpt-5.5`, `sdd-cheap=gpt-5.4-mini`), which sync treats as Recommended and upgrades to the current GPT-5.6 tuple; partial, extended, or otherwise different maps remain custom and unchanged.
- GPT-5.6 `max` reasoning effort and `ultra` mode are intentionally not enabled by this default update. `max` requires confirmed Codex support; `ultra` changes orchestration semantics and needs separate design.
- La delegación SDD multiagente queda habilitada por defecto: Axiom escribe `features.multi_agent = true` y `agents.max_threads = 4` / `agents.max_depth = 2` en `~/.codex/config.toml`. Para desactivarla, configura `multi_agent = false` en `[features]`. La delegación requiere ese ajuste y las herramientas nativas de Codex `spawn_agent`, `wait_agent` y `list_agents`; si falta cualquiera, la orquestación continúa en modo de agente único.
- **Delegation**: Native multi-agent by default, with graceful solo-agent fallback

### Windsurf

- Skills at `~/.codeium/windsurf/skills/` (native Windsurf feature)
- MCP config at `~/.codeium/windsurf/mcp_config.json`
- Global rules at `~/.codeium/windsurf/memories/global_rules.md`
- Workflows at `.windsurf/workflows/` (workspace-scoped)

### Antigravity

- Skills at `~/.gemini/antigravity/skills/` (native Antigravity feature)
- MCP config at `~/.gemini/antigravity/mcp_config.json`
- System prompt appended to `~/.gemini/GEMINI.md` (shared with Gemini CLI — collision check warns if both are installed)
- Mission Control handles built-in sub-agent delegation (Browser, Terminal) automatically
- Settings managed via the IDE's Agent settings UI, not via `settings.json`

### Kimi Code

- Installation requires the `uv` Python package manager (`uv tool install kimi-cli`).
- Root custom agent at `~/.kimi/agents/gentleman.yaml` with `system_prompt_path: ../KIMI.md`
- `KIMI.md` is a thin Jinja template that includes modular prompt files:
  `persona.md`, `output-style.md`, `engram-protocol.md`, `sdd-orchestrator.md`
- Built-in Kimi variables are preserved in `KIMI.md`: `${KIMI_AGENTS_MD}` and `${KIMI_SKILLS}`

### Kiro IDE

- **Detección**: Axiom detecta Kiro mediante el ejecutable `kiro` en `PATH`; también informa de si existe `~/.kiro`. Tener solo el directorio de configuración no basta para marcar Kiro como instalado.
- **Steering file** (all platforms): `~/.kiro/steering/gentle-ai.md` with frontmatter `inclusion: always`
- Native subagents at `~/.kiro/agents/sdd-{phase}.md` (10 files)
- Skills (all platforms) at `~/.kiro/skills/`
- **MCP config at a separate root** — always `~/.kiro/settings/mcp.json` (macOS/Linux) or `%USERPROFILE%\.kiro\settings\mcp.json` (Windows), regardless of GlobalConfigDir
- Native Kiro specs workflow: `.kiro/specs/<feature>/requirements.md`, `design.md`, `tasks.md` — with approval gates before apply and archive phases
- Manual install only — download from [kiro.dev/downloads](https://kiro.dev/downloads)
- See [docs/kiro.md](kiro.md) for full path reference and SDD behavior details

### Qwen Code

- **Detección**: Axiom detecta Qwen Code por su raíz de configuración (`~/.qwen`) y comprueba si el ejecutable `qwen` está en `PATH`.
- **Config root**: `~/.qwen/` (cross-platform)
- **System prompt**: `~/.qwen/QWEN.md` (managed via `StrategyFileReplace`)
- **Skills**: `~/.qwen/skills/`
- **MCP config**: `~/.qwen/settings.json` (managed via `StrategyMergeIntoSettings` with `mcpServers` key)
- **Slash commands**: `~/.qwen/commands/*.md` — supports custom namespaced slash commands (e.g., `commands/sdd/init.md` → `/sdd:init`)
- **Permissions**: `auto_edit` mode — auto-approves file edits, manual approval for shell commands
- **Install**: via npm — `npm install -g @qwen-code/qwen-code@latest`
- **Engram slug**: `"qwen-code"` for `engram setup` integration
- **SDD orchestrator**: `internal/assets/qwen/sdd-orchestrator.md` with Qwen-specific path references

### OpenClaw

- **Detección**: Axiom detecta OpenClaw mediante el ejecutable `openclaw` en `PATH` y la raíz `~/.openclaw`.
- **Instalación**: manual; instala primero OpenClaw y después ejecuta `axiom install --agent openclaw`.
- **Workspace activo**: Axiom lee `agents.defaults.workspace` de `~/.openclaw/openclaw.json` y escribe allí los ficheros de instrucciones.
- **Instructions**: Engram and SDD protocols are injected into workspace `AGENTS.md`; persona is injected into workspace `SOUL.md`.
- **MCP config**: Engram and Context7 are merged into global `~/.openclaw/openclaw.json` under `mcp.servers`; legacy root `mcpServers` entries are migrated.
- **Skills**: selected portable skills and SDD phase skills are workspace-scoped at `<workspace>/.openclaw/skills/`.

### Trae

- **Detección**: Axiom detecta Trae mediante `~/.trae` (aplicación de escritorio, sin ejecutable en `PATH`).
- **Global config root**: `~/.trae/` (cross-platform)
- **Skills**: `~/.trae/skills/`
- **System prompt / rules**: injected via `StrategyMarkdownSections` into the OS-specific `user_rules.md`
  - macOS: `~/Library/Application Support/Trae/User/user_rules.md`
  - Linux: `~/.config/Trae/User/user_rules.md` (respects `XDG_CONFIG_HOME`)
  - Windows: `%APPDATA%\Trae\User\user_rules.md`
- **MCP config**: same OS-specific dir → `mcp.json` (Cursor-compatible `mcpServers` object format)
- **Install**: desktop app only — manual install required; no `--auto-install` support

### Pi

For the full Pi command and package reference, see [Pi Agent](pi.md).

- **Detección**: Axiom detecta Pi mediante el ejecutable `pi` en `PATH` y la raíz `~/.pi`.
- **Instalación**: Pi debe estar instalado antes. Axiom aprovisiona entonces el conjunto de integración de Pi con:
  - `pi install npm:gentle-pi`
  - `pi install npm:gentle-engram`
  - `pi install npm:pi-mcp-adapter`
  - `npm exec --yes --package gentle-engram@latest -- pi-engram init`
  - `pi install npm:@juicesharp/rpiv-ask-user-question`
  - `pi install npm:pi-web-access`
  - `pi install npm:pi-btw`
- **Paquete `gentle-pi`**: el paquete externo aporta a Pi el flujo SDD/OpenSpec, directrices TDD, valores de seguridad, comandos `/gentle:*`, skills, prompts, agentes SDD y cadenas. En un `session_start` normal, copia recursos del proyecto a `.pi/agents/`, `.pi/chains/` y `.pi/gentle-ai/support/` sin sobrescribir ficheros locales, salvo que el comando de recuperación de Pi use `--force`. Iniciar Pi con `pi -ns` omite la carga inicial de skills y hooks; en ese modo no se produce esa actualización automática.
- **Package metadata**: latest verified `gentle-pi` version is `2.5.0`; npm lists `alan_buscaglia` as maintainer, with source at [Gentleman-Programming/gentle-pi](https://github.com/Gentleman-Programming/gentle-pi) and package docs at [npm: gentle-pi](https://www.npmjs.com/package/gentle-pi).
- **Cambio de persona**: `gentle-pi` gestiona `/gentle:persona`, que alterna entre `gentleman` y `neutral`, guarda `.pi/gentle-ai/persona.json` y puede requerir `/reload` o una nueva sesión para actualizar el prompt activo.
- **Asignación de modelos**: `gentle-pi` gestiona `/gentle:models`. Abre un diálogo nativo de Pi para agentes de proyecto, usuario e integrados, prioriza los agentes SDD, guarda `.pi/gentle-ai/models.json` y aplica los cambios en `.pi/agents/*.md` o `.pi/settings.json`.
- **`gentle-engram` package**: adds persistent Engram memory for Pi. It captures sessions, exposes Engram MCP tools through `pi-mcp-adapter`, and degrades safely when the local `engram` binary is missing.
- **MCP adapter wiring**: ComponentEngram declares `npm:pi-mcp-adapter` in `.pi/agent/settings.json` packages and adds `pi-mcp-adapter` `^2.6.0` to `.pi/npm/package.json` without removing unrelated user entries. `pi-engram init` owns the Pi Engram MCP config schema and is run during installation.
- **Subagentes**: `gentle-pi` proporciona Gentle Agents para descubrir y ejecutar agentes SDD desde `.pi/agents/`, con las herramientas `subagent_*` que antes ofrecía `npm:pi-subagents-j0k3r`. Axiom no instala ese paquete por separado; si ya existe una entrada `npm:pi-subagents-j0k3r`, se elimina de `settings.json` durante la siguiente instalación o actualización y Pi lo desinstala al sincronizar sus paquetes.
- **Subagentes en segundo plano**: se configuran con `axiom install` / `axiom sync`, mediante `--pi-background-subagents=auto|on|off` o la variable heredada `GENTLE_AI_PI_BACKGROUND_SUBAGENTS`. No hay launcher ni activación adicional, porque la primitiva es la extensión `pi-subagents-j0k3r` ya instalada.
- CLI precedence is flag, non-empty environment, prior managed state, then `auto`; `auto` never enables by itself, unresolved non-interactive `auto` stays foreground, and the interactive Pi installer prompts only when that preference is unresolved.
- The resolved on/off policy is projected to `~/.pi/gentle-ai/background-subagents.json` as `{"schema":"gentle-pi.background-subagents/v1","policy":"on"|"off"}` (the base directory honors `GENTLE_PI_CONFIG_HOME`); `off` rewrites the policy instead of deleting files, and a file at that path without the managed schema marker is never overwritten.
- **`@juicesharp/rpiv-ask-user-question` package**: lets Pi child agents ask the active user session for clarification when they need human input.
- **Pi companion packages**: `pi-web-access` and `pi-btw` add web access and companion workflow support. Todo tracking ships inside `gentle-pi` (Gentle Todo); an existing `@juicesharp/rpiv-todo` entry is dropped from `settings.json` on the next install or update, and Pi uninstalls it on its next package sync.
- **Solo Pi**: cuando Pi es el único agente seleccionado, Axiom omite las preguntas de persona, componentes del ecosistema y TDD estricto, porque las gestiona `gentle-pi`.

### Hermes Ephemeral Delegation

Hermes uses `delegate_task` to spawn ephemeral sub-agents. Each worker starts in a fresh context window and returns only its final summary to the parent orchestrator.

**Delegation rules:**

- Delegate when work needs broad exploration (4+ files), multi-file implementation, test/build execution, or fresh adversarial review.
- Each worker mission must be self-contained: include the exact goal, file paths or targets, relevant prior context, constraints, expected evidence, and the toolsets/MCP/skills the worker is allowed to use.
- Toolsets, MCP servers, and skills are NOT automatically inherited from the parent; pass them explicitly in the mission when `inherit_mcp_toolsets` is false (the default).
- Prefer parallel workers only for truly independent workstreams. Dependencies must run sequentially.
- Treat worker output as self-report: verify file writes, test pass/fail, URLs, and external side effects before reporting success.

**Tuning knobs** (configure in `~/.hermes/config.yaml` under the `delegation` key):

| Parameter | Default | Effect |
|-----------|---------|--------|
| `max_spawn_depth` | 2 | Maximum recursive delegation depth |
| `max_concurrent_children` | 4 | Maximum parallel workers |
| `max_iterations` | agent default | Iteration budget per worker |
| `child_timeout_seconds` | agent default | Hard timeout per worker |
| `inherit_mcp_toolsets` | false | When true, workers inherit parent MCP toolsets automatically |
| `subagent_auto_approve` | false | When true, workers auto-approve tool calls |

La tabla completa de decisión de delegación está en `~/.hermes/skills/hermes-ephemeral-delegation/SKILL.md` (instalada por Axiom). El orquestador SDD de `~/.hermes/SOUL.md` remite a esa skill.

### Hermes

- **Detección**: Axiom informa por separado de la presencia del ejecutable `hermes` en `PATH` y de la raíz `~/.hermes`; el directorio de configuración determina si Hermes figura como configurado, aunque falte el ejecutable.
- **Instalación**: solo detección; Axiom no instala Hermes. Instálalo manualmente y después ejecuta `axiom install --agent hermes`.
- **Config path**: `~/.hermes/` (config.yaml, SOUL.md, skills/)
- **MCP config**: Engram and Context7 are injected as YAML blocks under `mcp_servers:` in `~/.hermes/config.yaml` (`StrategyMergeIntoYAML`). Pre-existing top-level keys (e.g. `model:`) are preserved verbatim.
- **Prompt de sistema**: Axiom escribe el orquestador SDD y la persona en `~/.hermes/SOUL.md` mediante los marcadores Markdown heredados `<!-- gentle-ai:sdd-orchestrator -->` y `<!-- gentle-ai:persona -->`.
- **Skills**: Axiom escribe las skills de fases SDD en `~/.hermes/skills/`; el registro de skills también consulta esa ruta.
- **Permisos**: Hermes utiliza un formato de permisos no documentado. Axiom no inyecta permisos en Hermes.
- **Profiles**: Hermes does not support multi-mode SDD (no per-phase model routing). Single-mode only.
- **Memory**: Hermes has a native memory and skill-learning loop. Engram complements it — Engram provides cross-agent, cross-session memory protocol so knowledge is portable across all agents, not just Hermes.
- **Marcadores de persona e identidad**: Los marcadores `<!-- gentle-ai:persona -->` / `<!-- /gentle-ai:persona -->` en `SOUL.md` delimitan la sección que Axiom gestiona y actualiza durante la sincronización; sus identificadores se conservan por compatibilidad. La plantilla actual de persona de Hermes incluye literalmente `## Identity` con la instrucción de identificarse como **Gentle AI running on Hermes Agent** en cualquier idioma. Esa identidad pertenece a la plantilla heredada y no cambia el nombre de producto Axiom. Cualquier sección `## Identity` escrita fuera de los marcadores se conserva y puede contradecir la sección gestionada.
