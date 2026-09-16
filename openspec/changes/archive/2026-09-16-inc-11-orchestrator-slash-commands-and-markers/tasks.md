# Tareas de Implementación: axiom-orchestrator, Comandos Slash Canónicos y Migración de Marcadores (INC-11)

## Fase 1: Identidad de `axiom-orchestrator` en OpenCode (`internal/assets/opencode/`, `internal/opencode/`)

- [x] T-01 Actualizar `internal/assets/opencode/sdd-overlay-multi.json` y `sdd-overlay-single.json` renombrando la clave `gentle-orchestrator` a `axiom-orchestrator` con descripción "Axiom SDD Orchestrator - coordinates sub-agents, never does work inline".
- [x] T-02 Actualizar `internal/opencode/config.go` para mapear de forma transparente `gentle-orchestrator` y `sdd-orchestrator` hacia `axiom-orchestrator` en `configuredAssignments`, añadir `axiom-orchestrator` a `managedOpenCodeAgentKeys` y reconocer `axiom/sdd` en `managedConfigPriority`.
- [x] T-03 Actualizar los comandos en `internal/assets/opencode/commands/*.md` cambiando `agent: gentle-orchestrator` por `agent: axiom-orchestrator` e instruir al modelo como `axiom-orchestrator`.

## Fase 2: Comandos Slash Canónicos `/sdd-*` en Claude Code (`internal/assets/claude/`, `internal/components/sdd/`)

- [x] T-04 Renombrar los 11 comandos en `internal/assets/claude/commands/` de `gentle-sdd-*.md` a `sdd-*.md`.
- [x] T-05 Actualizar `internal/components/sdd/commands.go` eliminando el prefijo para Claude Code en `SlashCommandFileName`, y adaptando `LegacyClaudeCommandPath` e `IsLegacyClaudeCommandPath` para detectar y retirar los comandos legados con prefijo `gentle-sdd-*.md`.
- [x] T-06 Actualizar referencias a comandos en `internal/components/sdd/inject.go`, tests de assets y componentes.

## Fase 3: Marcadores de Sección Canónicos y Retrocompatibilidad (`internal/components/filemerge/`, `internal/components/uninstall/`)

- [x] T-07 Actualizar `internal/components/filemerge/section.go` para usar `<!-- axiom:` como prefijo canónico de escritura e inyección, reconociendo tanto `<!-- axiom:` como `<!-- gentle-ai:` para reemplazo, extracción, reparación de huérfanos y eliminación.
- [x] T-08 Actualizar `internal/components/uninstall/cleaners.go` para detectar tanto `<!-- axiom:` como `<!-- gentle-ai:` en `removeManagedPersonaPreamble`.

## Fase 4: Marcador de Instancia de Cambio `.axiom-instance` (`internal/sddstatus/`)

- [x] T-09 Actualizar `internal/sddstatus/edit_authority_consent.go` para definir `.axiom-instance` como archivo primario y mantener lectura retrocompatible de `.gentle-ai-instance`.
- [x] T-10 Actualizar los tests de edit authority y consent para verificar `.axiom-instance` con soporte retroactivo.

## Fase 5: Pruebas, Golden Files y Verificación Formal

- [x] T-11 Actualizar tests unitarios en `internal/opencode/`, `internal/components/sdd/`, `internal/components/filemerge/`, `internal/components/uninstall/`.
- [x] T-12 Actualizar golden files afectados si difieren por los nuevos marcadores o nombres canónicos.
- [x] T-13 Ejecutar `go test ./...` y verificar que la suite completa pase en verde.
- [x] T-14 Ejecutar `gentle-ai sdd-status inc-11-orchestrator-slash-commands-and-markers` y validar avance hacia apply.

