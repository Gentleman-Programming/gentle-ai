# SDD Spec: Human Gates Programáticos

## Escenarios (Given/When/Then)

**Escenario 1: Gate 1 Pendiente**
- **Given** que `proposal.md` existe, pero `gate-1-scope.md` no existe.
- **When** se evalúa el estado SDD.
- **Then** `Gate1Scope` es `DependencyReady`, `Specs` y `Design` son `DependencyBlocked`, y el paso recomendado es `"approve-gate-1"`.

**Escenario 2: Gate 1 Aprobado**
- **Given** que tanto `proposal.md` como `gate-1-scope.md` existen.
- **When** se evalúa el estado SDD.
- **Then** `Gate1Scope` es `DependencyAllDone`, y `Specs` y `Design` son `DependencyReady` (o `"spec"` es el recomendado).

*(El mismo patrón aplica para Gate 2 sobre Spec/Design y Gate 3 sobre Tasks)*.
