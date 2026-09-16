# Reporte de Archivado: Integración de Comandos SDD en la CLI axiom (INC-13)

**Fecha:** 2026-09-16  
**Incremento:** `inc-13-sdd-commands-axiom-cli-integration`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El **Incremento 13 (INC-13: `inc-13-sdd-commands-axiom-cli-integration`)** integra de forma nativa las operaciones del ciclo de vida SDD (*Spec-Driven Development*) y revisión RDD (*Receipt-Driven Development*) bajo el ejecutable unificado `axiom`, migrando simultáneamente las instrucciones y contratos de los orquestadores y comandos slash para prescribir la sintaxis canónica de Axiom en lugar de Gentle AI:

1. **Enrutamiento de Ciclo de Vida SDD (`cmd/axiom/main.go`):**
   - Implementación de `runSDD(args, stdout, stderr)` conectando con las operaciones del paquete `internal/cli`:
     - `axiom sdd status` -> `cli.RunSDDStatus`
     - `axiom sdd continue` -> `cli.RunSDDContinue`
     - `axiom sdd attempt` -> `cli.RunSDDAttempt` (con canonicalización de argumentos de revisión)
     - `axiom sdd verify-validate` -> `cli.RunSDDVerifyValidate`
     - `axiom sdd archive-compose` -> `cli.RunSDDArchiveCompose`
     - `axiom sdd task-result` -> `cli.RunSDDTaskResult`
     - `axiom sdd preflight-hook` -> `cli.RunSDDPreflightHook`
   - Conexión de versión de aplicación con `cli.AppVersion = Version`.

2. **Enrutamiento de Revisión RDD (`cmd/axiom/main.go`):**
   - Implementación de `runReview(args, stdout, stderr)` conectando con las operaciones de revisión:
     - `axiom review mode` -> `cli.RunReviewMode`
     - `axiom review start` -> `cli.RunReviewStart`
     - `axiom review resume` -> `cli.RunReviewResume`
     - `axiom review step` -> `cli.RunReviewStep`
     - `axiom review bundle-export` -> `cli.RunReviewBundleExport`
     - `axiom review bundle-import` -> `cli.RunReviewBundleImport`
     - `axiom review validate` -> `cli.RunReviewValidateNonDeciding`
     - Invocación raíz `axiom review [flags]` -> `cli.RunReview`.

3. **Soporte de Alias Planos Directos:**
   - Soporte total para invocaciones directas con guión en la CLI de Axiom: `sdd-status`, `sdd-continue`, `sdd-attempt`, `sdd-verify-validate`, `sdd-archive-compose`, `sdd-task-result`, `sdd-preflight-hook`, `review-start`, `review-resume`, `review-step`, `review-bundle-export`, `review-bundle-import`, `review-validate`.
   - Garantiza compatibilidad retroactiva y flexibilidad para scripts existentes.

4. **Actualización de Prompts y Activos Embebidos (`internal/assets/`):**
   - `internal/assets/skills/_shared/sdd-orchestrator-sections.md`: Prescripción canónica de `axiom sdd status`, `axiom sdd continue`, `axiom sdd attempt acquire/settle` y `axiom review`.
   - `internal/assets/claude/sdd-orchestrator-workflow.md` y `claude/commands/sdd-status.md`: Migración a `axiom sdd ...`.
   - `internal/assets/opencode/commands/sdd-status.md` y `opencode/commands/sdd-continue.md`: Invocación canónica de `axiom sdd status` y `axiom sdd continue`.
   - `internal/assets/skills/_shared/sdd-status-contract.md`, `sdd-phase-common.md`, `sdd-verify/SKILL.md` y `sdd-archive/SKILL.md`: Actualizados a la sintaxis canónica de la CLI `axiom`.

5. **Pruebas y Validación Exhaustiva:**
   - Creación de `cmd/axiom/main_test.go` con 100% PASS cubriendo inicialización de versión, ayuda contextual, subcomandos desconocidos, salida JSON de status, códigos de error y equivalencia por subproceso de alias directos vs jerárquicos.
   - Actualización de aserciones en `internal/assets/assets_test.go` con 100% PASS.
   - Validación formal con `axiom sdd-verify-validate`: VERDICT PASS.
   - Sincronización viva de especificaciones con `axiom archive sync`: 40 especificaciones consolidadas.

---

## Artefactos Consolidados y Modificados

- **CLI Principal:**
  - `cmd/axiom/main.go`
  - `cmd/axiom/main_test.go` (nuevo)
- **Activos de Agentes y Orquestadores:**
  - `internal/assets/skills/_shared/sdd-orchestrator-sections.md`
  - `internal/assets/skills/_shared/sdd-status-contract.md`
  - `internal/assets/skills/_shared/sdd-phase-common.md`
  - `internal/assets/skills/sdd-archive/SKILL.md`
  - `internal/assets/skills/sdd-verify/SKILL.md`
  - `internal/assets/skills/sdd-verify/references/report-format.md`
  - `internal/assets/claude/sdd-orchestrator-workflow.md`
  - `internal/assets/claude/commands/sdd-status.md`
  - `internal/assets/opencode/commands/sdd-status.md`
  - `internal/assets/opencode/commands/sdd-continue.md`
  - `internal/assets/assets_test.go`
- **Ciclo SDD de INC-13:**
  - `openspec/changes/inc-13-sdd-commands-axiom-cli-integration/proposal.md`
  - `openspec/changes/inc-13-sdd-commands-axiom-cli-integration/spec.md`
  - `openspec/changes/inc-13-sdd-commands-axiom-cli-integration/design.md`
  - `openspec/changes/inc-13-sdd-commands-axiom-cli-integration/tasks.md`
  - `openspec/changes/inc-13-sdd-commands-axiom-cli-integration/verify-report.md`
  - `openspec/changes/inc-13-sdd-commands-axiom-cli-integration/archive-report.md`
- **Especificaciones Vivas:**
  - `openspec/specs/axiom-sdd-cli-integration/spec.md` (nueva)
  - `openspec/INDEX.md` (regenerado)
