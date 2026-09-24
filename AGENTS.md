# Axiom — Reglas Maestras del Proyecto y Guía de Agentes

> **Proyecto:** Axiom (Fork y versión paralela de Gentle-AI)  
> **Metodología:** Flujo Dual ODD (Cotidiano) & Spec-Driven Development (SDD) con Axiom  
> **Memoria Persistente:** Engram MCP (`--project=axiom`)  
> **Repositorio Upstream (v3 de referencia):** `c:/repos/gentle-ai` (Gentle-AI v3.0.2+)

---

## REGLA SUPREMA: IDIOMA OBLIGATORIO — ESPAÑOL (CASTELLANO)

- **TODO EN ESPAÑOL:** Toda la comunicación, explicaciones y artefactos de ODD (`odd/tasks/*.md`) y SDD (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`) deben generarse estrictamente en **español (castellano)**.
- **FLUJO DUAL:** Usa ODD (`odd/tasks/<feature>.md`, gestionado con `axiom odd create`, `axiom odd status` y `axiom odd promote`) para tareas cotidianas y ágiles; reserva SDD para cuando se solicite explícitamente *"usa SDD"*.
- **DETERMINACIÓN TEMPRANA DE CARRIL Y COMPUERTAS DE BLOQUE (INC-21):** al entrar en SDD, sella la modalidad de avance, la política de relevos y el roster de roles con `axiom sdd kickoff seal` antes de crear `proposal.md`; sin subdivisión de roles se asigna obligatoriamente `fullstack`. En modalidad con paradas, decide cada compuerta de bloque (`spec`, `design`, `tasks`, `apply` por rol) con `axiom sdd gate record --gate <clave> --decision approved|rejected --reason "<motivo>"` antes de avanzar. `archive` exige además evidencia de integración o despliegue registrada en la compuerta `integration` (`axiom sdd gate record --gate integration ...`); nunca sustituye ni se confunde con el recibo de *receipt-driven development*.
- **POLÍTICA GIT DIFERENCIAL Y SINCRONIZACIÓN DE SPECS (ODD-5):**
  - **Repositorio de Specs (Directo a Main):** El repositorio de especificaciones (`specs_repository`) opera SIEMPRE directo en la rama principal (`main`/`master`), sin ramas ni worktrees. Antes de ceder el testigo en `handoff.md`, comprueba que las especificaciones están commiteadas y pusheadas a remoto (`git commit` y `git push`).
  - **Sincronización al Inicio de Sesión:** Al inicio de sesión, ejecuta pasivamente `git fetch` en el repositorio de specs. Si `git rev-list HEAD..@{u} --count` > 0, avisa de inmediato al usuario indicando los commits remotos entrantes y solicita autorización para ejecutar `git pull` antes de operar.
  - **Repositorios de Código (Worktrees y Ramas Aisladas):** En repositorios de código, formula el cuestionario interactivo de pre-vuelo antes de tocar código: (1) ¿crear worktree aislado o trabajar en el árbol actual?, (2) nombre de rama (`feat/...`), y (3) rama base (`main`, `develop`, etc.).
  - **Cierre de Rol y PR hacia Rama Configurable:** Al concluir la verificación en PASS, ofrece crear commit y pregunta interactivamente al usuario hacia qué rama base dirigir el Pull Request (`main`, `develop`, etc.) antes de proponer `gh pr create --base <rama-destino>`.
- **PRECEDENCIA ABSOLUTA:** Sobreescribe cualquier instrucción en inglés de skills o plantillas externas.

---

# Gentle AI™ — Agent Skills Index

When working on this project, load the relevant skill(s) BEFORE writing any code.

Naming convention: `gentle-ai-*` skills are repo-specific workflow skills. Unprefixed skills are portable writing or work-unit skills and intentionally keep their canonical names.

## How to Use

1. Check the trigger column to find skills that match your current task
2. Load the skill by reading the SKILL.md file at the listed path
3. Follow ALL patterns and rules from the loaded skill
4. Multiple skills can apply simultaneously

<!-- axiom:skills-index -->
## Skills

| Skill | Trigger / description | Scope | Path |
| --- | --- | --- | --- |
| `axiom-go-table-tests` | — | project | `skills/axiom-go-table-tests/SKILL.md` |
| `axiom-idiomatic-error-wrapping` | — | project | `skills/axiom-idiomatic-error-wrapping/SKILL.md` |
| `chained-pr` | Trigger: PRs over 400 lines, stacked PRs, review slices. Split oversized changes into chained PRs that protect review focus. | project | `.claude/skills/chained-pr/SKILL.md` |
| `cognitive-doc-design` | Design docs that reduce cognitive load. Trigger: writing guides, READMEs, RFCs, onboarding, architecture, or review-facing docs. | project | `skills/cognitive-doc-design/SKILL.md` |
| `comment-writer` | Write warm, direct collaboration comments. Trigger: PR feedback, issue replies, reviews, Slack messages, or GitHub comments. | project | `skills/comment-writer/SKILL.md` |
| `gentle-ai-bench` | Trigger: bench, journey, journeys, driven mode, gentle-ai-bench, journey corpus, j-numbers, bench axis. Author and verify gentle-ai bench journeys; go test ./bench never proves driven execution. | project | `skills/gentle-ai-bench/SKILL.md` |
| `gentle-ai-branch-pr` | Create Gentle AI pull requests. Trigger: creating, opening, or preparing PRs for review. | project | `skills/branch-pr/SKILL.md` |
| `gentle-ai-chained-pr` | Trigger: PRs over 400 lines, stacked PRs, review slices. Split oversized changes into chained PRs that protect review focus. | project | `skills/chained-pr/SKILL.md` |
| `gentle-ai-collab-perfect` | Trigger: contributing to Gentleman-Programming/gentle-ai as an external collaborator. Honest PR bodies, contributor-vs-maintainer scope, chained-PR strategy, verification protocol, docstring coverage. Load whenever the active repo is Gentleman-Programming/gentle-ai and any part of the contribution flow is in scope: opening an issue, drafting or editing a PR body, splitting a change into chained/stacked PRs, or auditing a PR before requesting review. | project | `skills/gentle-ai-collab-perfect/SKILL.md` |
| `go-testing` | Trigger: Go tests, go test coverage, Bubbletea teatest, golden files. Apply focused Go testing patterns. | project | `.claude/skills/go-testing/SKILL.md` |
| `issue-root-resolution` | Trigger: root audit, atacar la raíz, issue roots, backlog roots, mechanism map, deletion-driven fix, resolver issues de raíz, close outdated issues. Audit and resolve issue clusters by verified root cause. | project | `skills/issue-root-resolution/SKILL.md` |
| `judgment-day` | Trigger: judgment day, dual review, adversarial review, juzgar. Run explicit blind dual review with at most two scoped fix/re-judgment rounds. | project | `.claude/skills/judgment-day/SKILL.md` |
| `rdd-advisory-transport` | Trigger: reviewer transport, advisory transport, review adapter, lens prompt/schema, OpenCode reviewer plugin, Codex reviewer, ReviewProviderContract. Enforce the shared Go advisory reviewer transport contract. | project | `skills/rdd-advisory-transport/SKILL.md` |
| `rdd-defect-workflow` | Trigger: RDD, receipt-driven development, review authority, receipt/lineage, correction/recovery, delivery gate/kill switch, bounded review defects. Guide work. | project | `skills/rdd-defect-workflow/SKILL.md` |
| `skill-creator` | Trigger: new skills, agent instructions, documenting AI usage patterns. Create LLM-first skills with valid frontmatter. | project | `.claude/skills/skill-creator/SKILL.md` |
| `skill-improver` | Trigger: improve skills, audit skills, refactor skills, skill quality. Audit and upgrade existing LLM-first skills. | project | `.claude/skills/skill-improver/SKILL.md` |
| `systemic-issue-triage` | Trigger: new issue, bug report, triage, backlog, issue flood, community report, root cause, dead-end, blocked user. Attack issues by root class, never one-by-one; fixes must shrink the system, not grow it. | project | `skills/systemic-issue-triage/SKILL.md` |
| `work-unit-commits` | Plan commits as reviewable work units. Trigger: implementation, commit splitting, chained PRs, or keeping tests and docs with code. | project | `skills/work-unit-commits/SKILL.md` |
<!-- /axiom:skills-index -->
