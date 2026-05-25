# Tasks — fix-copilot-cli-pr-review

## Phase 1 — RED (tests fallan intencionalmente)

### 1.1 Actualizar `TestDetectionMissingConfig` para que espere `installed=false`

- **File**: `internal/agents/copilotcli/adapter_test.go`
- **Action**: Cambiar la aserción `if !installed` a `if installed` en `TestDetectionMissingConfig`
- **Expected**: El test falla (RED) porque el código actual devuelve `installed=true`
- **Verify**: `go test ./internal/agents/copilotcli/... → FAIL`

## Phase 2 — GREEN (tests pasan)

### 2.1 Corregir lógica de detección en `adapter.go`

- **File**: `internal/agents/copilotcli/adapter.go`
- **Action**: Cambiar `installed := err == nil` por `binaryFound := err == nil` y retornar `installed = binaryFound && stat.exists` (o `false` cuando el config no existe)
- **Expected**: `TestDetectionMissingConfig` pasa (GREEN)
- **Verify**: `go test ./internal/agents/copilotcli/... → PASS`

## Phase 3 — Cleanup

### 3.1 Eliminar comentario TDD obsoleto en `types_copilotcli_test.go`

- **File**: `internal/model/types_copilotcli_test.go`
- **Action**: Eliminar la línea `// RED: this test references AgentCopilotCLI before it exists.`

### 3.2 Refactorizar `inject.go` — if independientes → switch

- **File**: `internal/components/mcp/inject.go`
- **Action**: Convertir los 4 `if adapter.Agent() == ...` a un `switch adapter.Agent()`
- **Verify**: `go test ./internal/components/mcp/... → PASS`

### 3.3 Actualizar verify-report.md

- **File**: `openspec/changes/archive/2026-05-25-copilot-cli-adapter/verify-report.md`
- **Action**: Actualizar la fila de `internal/cli` en la tabla de "full suite" para indicar que el fix ya fue incluido en el PR (`TestNormalizeInstallFlagsDefaults` ahora pasa)

## Phase 4 — Verify

### 4.1 Correr suite completa

- `go test ./...` → todos los tests pasan
- `go vet ./...` → sin errores

### 4.2 Generar verify-report

- Completar `openspec/changes/fix-copilot-cli-pr-review/verify-report.md`
