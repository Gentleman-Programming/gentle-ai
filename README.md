<!-- markdownlint-disable-next-line MD041 -->
<a id="top"></a>

<div align="center">

<img width="100%" alt="Axiom banner" src="docs/assets/brand/axiom-banner.svg" />

<h1>Axiom</h1>

<p><strong>Plataforma Determinista de Ingeniería de Software, Orquestación Multi-Agente y Spec-Driven Development (SDD).</strong></p>

<p>
<img src="https://img.shields.io/badge/go-1.25+-38BDF8?style=for-the-badge&labelColor=0F172A" alt="Go 1.25+">
<img src="https://img.shields.io/badge/agents-16-818CF8?style=for-the-badge&labelColor=0F172A" alt="16 agents">
<img src="https://img.shields.io/badge/lifecycle-SDD%20%2B%20RDD-38BDF8?style=for-the-badge&labelColor=0F172A" alt="SDD + RDD">
<img src="https://img.shields.io/badge/multi--project-Hub-C084FC?style=for-the-badge&labelColor=0F172A" alt="Multi-Project Hub">
<a href="LICENSE"><img src="https://img.shields.io/badge/MIT-818CF8?style=for-the-badge&labelColor=0F172A" alt="License: MIT"></a>
</p>

<p>
<a href="docs/quickstart.md"><strong>Guía Rápida</strong></a> &bull;
<a href="docs/architecture.md"><strong>Arquitectura</strong></a> &bull;
<a href="docs/agents.md"><strong>Agentes</strong></a> &bull;
<a href="docs/intended-usage.md"><strong>Documentación</strong></a>
</p>

<br/>

<p>
Tus agentes de IA generan código, pero pierden contexto en cada sesión y carecen de un método determinista para validar lo que hacen.
<strong>Axiom les dota de memoria cognitiva acumulativa, un ciclo de vida SDD riguroso, arquitectura multi-proyecto y evidencia auditable.</strong>
</p>

<br/>

<sub><strong>COMPATIBLE CON EL AGENTE QUE YA UTILIZAS</strong></sub>

<strong><a href="docs/agents.md#pi">Pi</a></strong> ·
<strong><a href="docs/agents.md#opencode">OpenCode</a></strong> ·
<strong><a href="docs/agents.md#claude-code">Claude Code</a></strong> ·
<strong><a href="docs/agents.md#codex">Codex</a></strong> ·
<strong><a href="docs/agents.md#cursor">Cursor</a></strong> ·
<strong><a href="docs/agents.md#vs-code-copilot">VS Code Copilot</a></strong> ·
<strong><a href="docs/agents.md#gemini-cli">Gemini CLI</a></strong> ·
<strong><a href="docs/agents.md#kilo-code">Kilo Code</a></strong><br/>
<strong><a href="docs/agents.md#kimi-code">Kimi Code</a></strong> ·
<strong><a href="docs/agents.md#kiro-ide">Kiro IDE</a></strong> ·
<strong><a href="docs/agents.md#qwen-code">Qwen Code</a></strong> ·
<strong><a href="docs/agents.md#hermes">Hermes</a></strong> ·
<strong><a href="docs/agents.md#antigravity">Antigravity</a></strong> ·
<strong><a href="docs/agents.md#windsurf">Windsurf</a></strong> ·
<strong><a href="docs/agents.md#openclaw">OpenClaw</a></strong> ·
<strong><a href="docs/agents.md#trae">Trae</a></strong>

<sub>16 integraciones · configuración no-intrusiva · <a href="docs/agents.md">comparar capacidades →</a></sub>

</div>

---

## Características y Capacidades

---

### Engram™ — Keep your project context

<img width="100%" src="docs/assets/diagrams/engram-memory.svg" alt="Three work sessions separated by a restart and by context compaction. Each break cuts the session layer but stops at the memory layer underneath. The first session saves a decision, the next one asks memory before asking you, and weeks later the same question is answered from memory instead of by re-reading the repository." />

The cost of a fresh session is not the tokens — it is you, re-explaining the same decisions every morning. Engram removes that: your agent writes down what it learns as it goes and reaches for it before it reaches for you, so context accumulates instead of resetting.

**[Docs →](docs/engram.md)**

---

### SDD — Give each change a clear path

<img width="100%" src="docs/assets/diagrams/sdd-cycle.svg" alt="The SDD cycle is selected only by explicit request or accepted proposal. Explore can use optional Research; Proposal, Spec, Design and Tasks create formal planning artifacts; Apply uses configured TDD. Optional Verify reports practical diagnostics, including for partial work, but does not gate Archive: a separate path leads from Apply straight to Archive when Verify is skipped. Archive records actual state and history, including unfinished work when explicitly archived. It is not shipping, approval, or RDD." />

When you explicitly choose Spec-Driven Development, proposal, specification, design, and task artifacts make the plan reviewable before implementation. File-backed storage keeps them on disk; Engram-backed storage keeps them in memory. Apply follows the configured TDD mode. Research and Verify are optional: Verify can diagnose partial work and report practical findings, but it is not an archive gate. Archive records the actual state and history, including unfinished work when you explicitly archive it; it does not ship or approve the change, and SDD does not invoke RDD. TDD is also available in ODD; it does not require an SDD phase.

**[Docs →](docs/intended-usage.md)**

---

### RDD — Check finished work at the right depth

<img width="100%" src="docs/assets/diagrams/rdd-review.svg" alt="How RDD checks a finished change. The exact change is frozen to a lineage, revision and target, then a read-only risk assessment picks the depth: passive gets a structural readback with zero reviewer lenses, medium gets one focused lens, high gets the canonical 4R — Risk, Resilience, Readability and Reliability. At most one bounded correction is allowed, and one exact acknowledgement closes the transaction. Delivery stays human-owned." />

Receipt-Driven Development (RDD) is opt-in and stays off until you enable it. Its point is that a review cannot drift: the candidate is frozen before anything reads it, so the evidence belongs to the exact version you are about to rely on — not to whatever the worktree looked like a moment later. The depth comes from that frozen candidate rather than from the model's judgment, and the result is informational. Commit, push and release stay your call.

**[Docs →](docs/review-integration.md)**

---

### Deterministic by design — Know the next valid step

<img width="100%" src="docs/assets/diagrams/deterministic.svg" alt="A different agent, a different model and a brand-new session all converge on the gentle-ai binary. It reads the change state from files on disk and returns the only valid next transition, so no model votes on what comes next. The answer is always one of four public states: Working, Checking, Ready, or Needs your decision." />

A model that guesses the next step guesses differently tomorrow, and differently again for your teammate. That is the gap between a workflow and a suggestion. The **`gentle-ai` binary** owns native SDD status and RDD review transitions, and because it reads state from files rather than from a context window, two people on two machines get the same answer — and so does the same person a month later.

**[Docs →](docs/trigger-rules.md)**

---

### Gentle Shell — A complete workspace for Pi

<img width="100%" src="docs/assets/features/gentle-shell.png" alt="Gentle Shell running an SDD sub-agent, with the todo list and live context and spend information" />

**The way Gentle-AI was intended.** Gentle-AI brings our native Pi extensions together in one focused development environment: orchestrate specialized agents, monitor usage for supported provider accounts, and review code changes in a built-in diff.

**[Docs →](docs/pi.md)**

---

### 16 agents — Keep the agent you already use

<img width="100%" src="docs/assets/features/agents.png" alt="The installer configuring multiple agents" />

Gentle-AI brings its shared workflow to Pi, OpenCode, Claude Code, Codex, and twelve more agents. Each integration uses that agent's native capabilities, so available features such as delegation and RDD review can differ.

**[Docs →](docs/agents.md)**

---

### Also in the box

| Component | What it does |
| :--- | :--- |
| **Skills library** | Loaded automatically when the task matches |
| **Context7 MCP** | Optional, selectable live framework and library documentation |
| **CodeGraph** | Read-only symbol graph of your codebase |
| **Security deny-list** | Blocks `~/.ssh`, `.env` and credential files |
| **Config backups** | Snapshotted before every single write |
| **Doctor** | `gentle-ai doctor` — read-only health report |
| **Personas** | Optional personas; Gentleman is a caring but rigorous mentor who guides you toward your goal |
| **Themes** | Gentleman and Gentleman-Cute |
| **Per-phase model assignment** | Assign a model to each phase in Pi and OpenCode |

> **Every component, skill and preset: [Full breakdown →](docs/components.md)**

<div align="right"><a href="#top">Back to top</a></div>

<div align="center"><img src="docs/assets/brand/rose.png" width="28" alt="" /></div>

## Get started

```bash
# macOS (Homebrew)
brew install gentleman-programming/tap/gentle-ai

# macOS / Linux (curl)
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash

# Windows (PowerShell) — source install, needs Go 1.25.10+
go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@latest
```

```bash
gentle-ai          # pick your agents, components and persona
gentle-ai doctor   # verify — read-only, changes nothing
```

Then use your agent normally. Your configs are snapshotted before every write, and **Gentle-AI never installs an AI agent for you** — it configures what you already have.

> **Beta channel, signature verification and per-distro prerequisites: [Quickstart →](docs/quickstart.md)**

<div align="right"><a href="#top">Back to top</a></div>

<div align="center"><img src="docs/assets/brand/rose.png" width="28" alt="" /></div>

## Documentation

| Where to go | What you'll find |
| :--- | :--- |
| **[Intended Usage](docs/intended-usage.md)** | The mental model. If you read one page, read this one. |
| **[Quickstart](docs/quickstart.md)** · **[Usage](docs/usage.md)** | Install, prerequisites, every CLI command and flag |
| **[Agents](docs/agents.md)** | Feature matrix and per-agent notes for all 16 |
| **[Routing](docs/trigger-rules.md)** | How the agent picks direct, delegated or SDD |
| **[Review](docs/review-integration.md)** · **[Architecture](docs/architecture/organic-rdd.md)** | The RDD contract, lifecycle and threat model |
| **[Engram](docs/engram.md)** · **[Components](docs/components.md)** | Memory commands, skills, presets and personas |
| **[Contributing](CONTRIBUTING.md)** · **[Codebase Guide](docs/CODEBASE-GUIDE.md)** | Extend or contribute |
| **[Telemetry](docs/telemetry.md)** | What we count, and how to turn it off |

<div align="right"><a href="#top">Back to top</a></div>

<div align="center"><img src="docs/assets/brand/rose.png" width="28" alt="" /></div>

## Community

Everything labelled [`up-for-grabs`](https://github.com/Gentleman-Programming/gentle-ai/issues?q=is%3Aissue+is%3Aopen+label%3Aup-for-grabs) is scoped and unclaimed — pick one and it's yours.

<div align="center">

<a href="docs/community-roadmap.md"><img src="https://img.shields.io/badge/Community%20Roadmap-F095C8?style=for-the-badge&labelColor=1A1218&logo=readthedocs&logoColor=F095C8" alt="Community Roadmap"></a>
<a href="CONTRIBUTING.md"><img src="https://img.shields.io/badge/Contributing%20Guide-F095C8?style=for-the-badge&labelColor=1A1218&logo=git&logoColor=F095C8" alt="Contributing Guide"></a>
<a href="CONTRIBUTORS.md"><img src="https://img.shields.io/badge/Contributors-D7A0B8?style=for-the-badge&labelColor=1A1218&logo=github&logoColor=D7A0B8" alt="Contributors"></a>

<br/><br/>

<a href="CONTRIBUTORS.md">
  <img width="100%" src="https://contrib.rocks/image?repo=Gentleman-Programming/gentle-ai&columns=16" alt="Gentle-AI contributors" />
</a>

<p><sub>This project exists because of these people.</sub></p>

</div>

<div align="right"><a href="#top">Back to top</a></div>

---

## Origen y Evolución

**Axiom** es una plataforma independiente de ingeniería aumentada nacida como evolución y bifurcación de *Gentle-AI*. Conserva la robustez del arnés determinista y la metodología de Spec-Driven Development (SDD), incorporando gobernanza multi-rol, catálogo de autoskills, motor de código semántico y un Hub centralizado multi-proyecto.

<div align="center">

<br/>

<h3>Axiom: Deterministic Multi-Agent Engineering</h3>

<br/>

<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-38BDF8?style=for-the-badge&labelColor=0F172A" alt="License: MIT"></a>

</div>

> **Nota de marca y atribución:** Axiom respeta los derechos y marcas originales de sus componentes base (Gentle AI™, Engram™). El código se distribuye bajo licencia de código abierto MIT. Véase [LICENSE](LICENSE).
