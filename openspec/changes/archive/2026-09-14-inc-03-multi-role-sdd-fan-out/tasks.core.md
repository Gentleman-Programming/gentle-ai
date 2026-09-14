# Tareas de Implementación: Rol Core Engine

- [x] T-01 Crear modelos de dominio `types.go` (`GatePolicy`, `RoleAssignment`, `BarrierReport`, `DeferredTask`).
- [x] T-02 Implementar detector de roles `detector.go` (`DetectRoles`, `ParseRolesMarkdown`, fallback y validación `axiom.yaml`).
- [x] T-03 Implementar motor de barrera de sincronización `barrier.go` (`EvaluateBarrier`, `CountTasks`, migración diferida).
- [x] T-04 Incorporar subcomandos CLI `axiom role list/status/barrier` en `cmd/axiom/main.go`.
