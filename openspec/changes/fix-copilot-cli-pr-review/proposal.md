# fix-copilot-cli-pr-review

## Problem

El PR #656 (`feat(copilot-cli): add support for GitHub Copilot CLI`) recibió 5 comentarios del Copilot PR Reviewer bot. Dos son issues críticos que rompen el contrato del spec y deben corregirse antes de mergear.

## Affected Packages

- `internal/agents/copilotcli` — adapter.go, adapter_test.go
- `internal/model` — types_copilotcli_test.go
- `internal/components/mcp` — inject.go
- `openspec/changes/archive/2026-05-25-copilot-cli-adapter` — verify-report.md

## Changes Summary

### Critical (rompen el spec)

1. **`adapter.go` — Detección incorrecta**: `installed` se basa solo en si el binario está en PATH, pero el spec exige que TAMBIÉN exista `~/.copilot/config.json`. Fix: `installed = binaryFound && configFound`.

2. **`adapter_test.go` — Test inconsistente**: `TestDetectionMissingConfig` afirma `installed=true` cuando falta el config. Debe afirmar `installed=false` para alinearse con el spec y el fix anterior.

### Minor (claridad y limpieza)

3. **`types_copilotcli_test.go` — Comentario TDD obsoleto**: El comentario `// RED: this test references AgentCopilotCLI before it exists` es confuso porque la constante ya existe. Eliminarlo.

4. **`inject.go` — Ramas mutuamente exclusivas con if independientes**: Cuatro `if` independientes son difíciles de extender y ocultan la exclusividad. Convertir a `switch`.

5. **`verify-report.md` — Reporte desactualizado**: El reporte dice que `TestNormalizeInstallFlagsDefaults` falla, pero el PR ya incluyó el fix. Actualizar para reflejar el estado real.

## Rollback Plan

Bajo riesgo — todos los cambios son en tests, comentarios y un refactor de switch/if sin cambio de comportamiento, excepto el fix de detección que corrige un bug del spec. Si hay regresión inesperada: `git revert HEAD`.
