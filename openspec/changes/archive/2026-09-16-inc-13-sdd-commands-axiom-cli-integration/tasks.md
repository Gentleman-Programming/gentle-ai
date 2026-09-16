# Tareas de Implementación: Integración de Comandos SDD en la CLI axiom (INC-13)

## Fase 1: Integración de Comandos SDD y Review en `cmd/axiom/main.go`

- [x] T-01 Añadir enrutamiento y ejecución del grupo de comandos `axiom sdd <status|continue|attempt|verify-validate|archive-compose|task-result|preflight-hook>` en `cmd/axiom/main.go` conectando con `internal/cli`.
- [x] T-02 Añadir enrutamiento y ejecución del grupo de comandos `axiom review <mode|start|resume|step|bundle-export|bundle-import|validate>` en `cmd/axiom/main.go` conectando con `internal/cli`.
- [x] T-03 Añadir soporte para alias planos directos (`sdd-status`, `sdd-continue`, `sdd-attempt`, `sdd-verify-validate`, `sdd-archive-compose`, `sdd-task-result`, `sdd-preflight-hook`, `review-start`, `review-validate`, etc.) en `cmd/axiom/main.go`.
- [x] T-04 Actualizar la función `printHelp()` en `cmd/axiom/main.go` para documentar los nuevos comandos SDD y de revisión.

## Fase 2: Actualización de Prompts y Activos Embebidos (`internal/assets/`)

- [x] T-05 Actualizar `internal/assets/skills/_shared/sdd-orchestrator-sections.md` para prescribir llamadas canónicas a `axiom sdd status`, `axiom sdd continue`, `axiom sdd attempt` y `axiom review`.
- [x] T-06 Actualizar flujos y comandos de Claude (`internal/assets/claude/sdd-orchestrator-workflow.md` y `internal/assets/claude/commands/sdd-status.md`) apuntando a `axiom sdd ...`.
- [x] T-07 Actualizar comandos slash de OpenCode en `internal/assets/opencode/commands/` (`sdd-status.md` y `sdd-continue.md`) apuntando a `axiom sdd ...`.
- [x] T-08 Actualizar contratos compartidos y skills (`internal/assets/skills/_shared/sdd-status-contract.md`, `internal/assets/skills/_shared/sdd-phase-common.md`, `internal/assets/skills/sdd-verify/SKILL.md`, `internal/assets/skills/sdd-archive/SKILL.md`) prescribiendo la CLI `axiom`.

## Fase 3: Pruebas Unitarias y de Integración

- [x] T-09 Crear suite de pruebas `cmd/axiom/main_test.go` verificando el despacho de comandos `axiom sdd status`, `axiom sdd continue`, `axiom sdd attempt`, alias directos y control de errores.
- [x] T-10 Actualizar aserciones de pruebas en `internal/assets/assets_test.go` para validar los comandos canónicos de `axiom sdd` en los prompts.
- [x] T-11 Ejecutar suites de pruebas de `cmd/axiom/...` y `internal/assets/...` asegurando paso limpio (PASS).

## Fase 4: Verificación SDD y Cierre

- [x] T-12 Validar el estado formal con `gentle-ai sdd-status inc-13-sdd-commands-axiom-cli-integration` y redactar `verify-report.md`.
- [x] T-13 Archivar formalmente el cambio y consolidar la especificación viva en `openspec/specs/axiom-sdd-cli-integration/spec.md`.
