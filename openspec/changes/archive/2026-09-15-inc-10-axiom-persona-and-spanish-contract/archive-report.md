# Reporte de Archivado: Persona Axiom, Contrato Lingüístico en Español Peninsular y Desactivación del Voseo (INC-10)

**Fecha:** 2026-09-15  
**Incremento:** `inc-10-axiom-persona-and-spanish-contract`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El Incremento 10 (INC-10) culmina la adopción de la identidad canónica y el marco lingüístico de Axiom:

1. **Persona Rector Oficial (`axiom`):** Se introdujo `model.PersonaAxiom = "axiom"` como la persona predeterminada en todo el ecosistema (CLI, TUI, instalador y adaptadores). Se mantiene retrocompatibilidad total con `gentleman` y `neutral`.
2. **Erradicación del Voseo Forzado:** Se sustituyó el voseo rioplatense obligatorio por **castellano de España (peninsular)** con tuteo técnico profesional ("tienes", "haz", "revisa", "fíjate"), prohibiendo expresamente términos regionales ("che", "tenés", "podés", "hacé").
3. **Reforma del Contrato Lingüístico SDD:** En proyectos bajo gobernanza Axiom, todos los artefactos de ingeniería y metodología SDD (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`) se redactan obligatoriamente en español (castellano peninsular). El idioma inglés queda reservado de forma estricta y exclusiva para los identificadores de código fuente (structs, funciones, métodos, variables) y palabras clave del compilador.
4. **Despliegue y Reconciliación en Adaptadores:** Se crearon y conectaron los assets correspondientes (`persona-axiom.md` y `output-style-axiom.md`) para Claude Code, OpenCode, Kilocode, Kimi, Kiro IDE y Hermes Agent, garantizando la retirada limpia de estilos obsoletos.

---

## Artefactos Consolidados y Modificados

- **Modelos y Núcleo:**
  - `internal/model/types.go`
  - `internal/cli/validate.go`
  - `internal/tui/screens/persona.go`
  - `internal/tui/model.go`
- **Componente de Persona y Adaptadores:**
  - `internal/components/persona/resources.go`
  - `internal/components/persona/inject.go`
  - `internal/assets/generic/persona-axiom.md`
  - `internal/assets/claude/persona-axiom.md`
  - `internal/assets/claude/output-style-axiom.md`
  - `internal/assets/opencode/persona-axiom.md`
  - `internal/assets/kiro/persona-axiom.md`
  - `internal/assets/hermes/persona-axiom.md`
  - `internal/assets/kimi/output-style-axiom.md`
- **Contratos y Skills SDD:**
  - `internal/assets/skills/_shared/sdd-orchestrator-sections.md`
  - `internal/assets/codex/sdd-orchestrator.md`
  - `openspec/specs/persona-behavior-contract/spec.md`
- **Suites de Pruebas Unitarias y Golden Fixtures:**
  - `internal/components/persona/persona_language_contract_test.go`
  - `internal/components/persona/resources_test.go`
  - `internal/tui/screens/persona_language_contract_test.go`
  - `internal/cli/persona_language_contract_test.go`
  - `internal/cli/install_test.go`
  - `internal/assets/language_contract_test.go`
  - `internal/components/sdd/inject_test.go`
  - `internal/components/sdd/review_ledger_contract_test.go`
  - `internal/components/sdd/inject_pi_cleanup_test.go`
  - `testdata/golden/*` (fixtures regenerados y validados)