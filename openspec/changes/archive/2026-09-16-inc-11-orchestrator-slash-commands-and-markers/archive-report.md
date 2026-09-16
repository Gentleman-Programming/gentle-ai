# Reporte de Archivado: Orquestador axiom-orchestrator, Comandos Slash Canónicos y Migración de Marcadores (INC-11)

**Fecha:** 2026-09-16  
**Incremento:** `inc-11-orchestrator-slash-commands-and-markers`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El **Incremento 11 (INC-11: `inc-11-orchestrator-slash-commands-and-markers`)** consolida la autonomía de identidad y la normalización de la interfaz de agente para Axiom:

1. **Identidad Canónica del Orquestador (`axiom-orchestrator`):**
   - Se registró formalmente el agente `axiom-orchestrator` en las plantillas overlay de OpenCode (`sdd-overlay-multi.json` y `sdd-overlay-single.json`) con la descripción *"Axiom SDD Orchestrator - coordinates sub-agents, never does work inline"*.
   - Se actualizó el motor de configuración en `internal/opencode/config.go` para mapear de manera transparente asignaciones previas de `gentle-orchestrator` y `sdd-orchestrator` hacia `axiom-orchestrator`.
   - Se actualizaron todos los comandos de OpenCode (`sdd-apply.md`, `sdd-archive.md`, `sdd-status.md`, `sdd-continue.md`, etc.) asignándolos a `agent: axiom-orchestrator` y declarando el rol correspondiente.

2. **Estandarización de Comandos Slash Canónicos en Claude Code (`/sdd-*`):**
   - Se renombraron los 11 comandos slash en `internal/assets/claude/commands/` de `gentle-sdd-*.md` a sus nombres canónicos directos `sdd-*.md` (`sdd-apply.md`, `sdd-verify.md`, `sdd-archive.md`, `sdd-status.md`, `sdd-init.md`, `sdd-new.md`, `sdd-explore.md`, `sdd-research.md`, `sdd-onboard.md`, `sdd-ff.md`, `sdd-continue.md`).
   - Se adaptó `internal/components/sdd/commands.go` eliminando el prefijo propietario y proporcionando detección y retiro automático de comandos legados con prefijo `gentle-sdd-*.md`.

3. **Marcadores Canónicos de Sección y Soporte Dual:**
   - Se actualizó `internal/components/filemerge/section.go` para emitir canónicamente marcadores con prefijo `<!-- axiom:<id> --> ... <!-- /axiom:<id> -->`.
   - Se garantizó compatibilidad dual tolerante con marcadores preexistentes `<!-- gentle-ai:<id> -->` para operaciones de reemplazo, extracción, reparación y desinstalación (`cleaners.go`).

4. **Centinela de Cambio `.axiom-instance`:**
   - Se estableció `.axiom-instance` como el marcador primario de instancia y consentimiento de edición en `internal/sddstatus/edit_authority_consent.go`, manteniendo soporte de lectura y validación retrocompatible si se detecta `.gentle-ai-instance`.

---

## Artefactos Consolidados y Modificados

- **OpenCode & Assets:**
  - `internal/assets/opencode/sdd-overlay-multi.json`
  - `internal/assets/opencode/sdd-overlay-single.json`
  - `internal/assets/opencode/commands/*.md`
  - `internal/opencode/config.go`
  - `internal/opencode/config_test.go`
- **Claude Code & Comandos SDD:**
  - `internal/assets/claude/commands/sdd-*.md`
  - `internal/components/sdd/commands.go`
  - `internal/components/sdd/commands_test.go`
  - `internal/components/sdd/inject.go`
- **Marcadores de Sección y Limpieza:**
  - `internal/components/filemerge/section.go`
  - `internal/components/filemerge/section_test.go`
  - `internal/components/uninstall/cleaners.go`
  - `internal/components/uninstall/cleaners_test.go`
- **Centinela de Consentimiento y Status:**
  - `internal/sddstatus/edit_authority_consent.go`
  - `internal/sddstatus/edit_authority_test.go`
  - `internal/sddstatus/status_v2_clean_break_test.go`
- **Golden Fixtures:**
  - `testdata/golden/*` (fixtures actualizados para Claude, OpenCode, Codex, Gemini, Kiro, Windsurf)
