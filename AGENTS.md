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

## Skills

| Skill | Trigger | Path |
|-------|---------|------|
| `issue-creation` | When creating a GitHub issue, reporting a bug, or requesting a feature. | [`internal/assets/skills/issue-creation/SKILL.md`](internal/assets/skills/issue-creation/SKILL.md) |
| `gentle-ai-branch-pr` | When creating a pull request, opening a PR, or preparing changes for review. | [`skills/branch-pr/SKILL.md`](skills/branch-pr/SKILL.md) |
| `gentle-ai-chained-pr` | When a change is too large for one review, or when creating chained/stacked pull requests. | [`skills/chained-pr/SKILL.md`](skills/chained-pr/SKILL.md) |
| `cognitive-doc-design` | When writing docs that must reduce cognitive load for readers or reviewers. | [`skills/cognitive-doc-design/SKILL.md`](skills/cognitive-doc-design/SKILL.md) |
| `comment-writer` | When drafting human comments, PR feedback, issue replies, or async updates. | [`skills/comment-writer/SKILL.md`](skills/comment-writer/SKILL.md) |
| `work-unit-commits` | When splitting implementation work into deliverable commits or chained PRs. | [`skills/work-unit-commits/SKILL.md`](skills/work-unit-commits/SKILL.md) |
| `rdd-defect-workflow` | When RDD defects involve receipts, authority, recovery, delivery gates, or kill switches. | [`skills/rdd-defect-workflow/SKILL.md`](skills/rdd-defect-workflow/SKILL.md) |
| `rdd-advisory-transport` | When changing reviewer transport, adapters, lens prompts/schemas, or transport capability policy. | [`skills/rdd-advisory-transport/SKILL.md`](skills/rdd-advisory-transport/SKILL.md) |
| `issue-root-resolution` | When auditing backlog roots, proposing cluster fixes, or closing resolved/outdated issues. | [`skills/issue-root-resolution/SKILL.md`](skills/issue-root-resolution/SKILL.md) |
| `systemic-issue-triage` | When triaging issues, bugs, backlogs, root causes, dead ends, or blocked users. | [`skills/systemic-issue-triage/SKILL.md`](skills/systemic-issue-triage/SKILL.md) |
| `gentle-ai-bench` | When touching `bench/`, journeys, driven mode, the journey corpus, or bench axes. | [`skills/gentle-ai-bench/SKILL.md`](skills/gentle-ai-bench/SKILL.md) |
