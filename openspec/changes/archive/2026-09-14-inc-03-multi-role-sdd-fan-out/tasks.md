# Tareas: Despliegue Multi-Rol y Barrera de Sincronización en SDD (INC-03)

## Fase 1: Modelos de Dominio y Extractor de Roles (`internal/multirole`)

- [x] T-01 Crear `internal/multirole/types.go`: Definir `GatePolicy` (`blocking`, `deferred`, `optional`), `RoleAssignment`, `RoleTaskProgress`, `RoleExecutionStatus`, `DeferredTask` y `BarrierReport`.
- [x] T-02 Crear `internal/multirole/detector.go`: Implementar `DetectRoles` y `ParseRolesMarkdown` para extraer roles declarados en `design.md` con su `gate_policy`, validación contra `axiom.yaml` y fallback retrocompatible para mono-rol.

## Fase 2: Motor de Barrera de Sincronización y Tareas Diferidas (`internal/multirole`)

- [x] T-03 Crear `internal/multirole/barrier.go`: Implementar `EvaluateBarrier`, `CountTasks`, extracción de `DeferredTask` con referencia de origen `[Ref: <cambio>]` y soporte de migración a archivo acumulativo.
- [x] T-04 Crear `internal/multirole/multirole_test.go`: Suite completa de pruebas unitarias cubriendo extracción de roles con políticas, cómputo de tareas, barrera aprobada con roles bloqueantes y advertencia para roles diferidos, bloqueos obligatorios y formateo de tareas diferidas.

## Fase 3: Integración de Subcomandos CLI (`cmd/axiom`)

- [x] T-05 Ampliar `cmd/axiom/main.go`: Incorporar el grupo de comandos `axiom role` con:
  - `axiom role list [--change <nombre>] [--path <directorio>]`
  - `axiom role status [--change <nombre>] [--path <directorio>]`
  - `axiom role barrier [--change <nombre>] [--path <directorio>] [--migrate-deferred]`
- [x] T-06 Formatear salidas de consola legibles en español, avisos de tareas diferidas y códigos de salida estándar (`0` para conforme, `1` para bloqueado/error).

## Fase 4: Verificación Integral y Ejecución

- [x] T-07 Ejecutar suite de pruebas unitarias `go test -v ./internal/multirole/...` y `go test ./...` asegurando cero regresiones.
- [x] T-08 Recompilar binario `axiom.exe` y verificar ejecución en terminal de los comandos `axiom role list`, `status` y `barrier`.
