# SDD Proposal: Human Gates Programáticos

## Enfoque
1. Introducir 3 nuevos tipos de artefactos al `ArtifactPaths` y al estado del Engram:
   - `gate-1-scope.md`
   - `gate-2-technical.md`
   - `gate-3-implementation.md`
2. Modificar la estructura `Dependencies` en `status.go` para añadir los campos `Gate1Scope`, `Gate2Technical`, `Gate3Implementation`.
3. Actualizar `resolveDependencies` para condicionar `Specs`, `Design`, `Tasks` y `Apply` a la existencia de sus respectivos gates. Si el gate falta, el gate entra en estado `DependencyReady` (requiere acción) y la fase siguiente se bloquea.
4. Actualizar `resolveNextRecommended` para priorizar los estados de aprobación ("approve-gate-1", etc.) antes de continuar la generación de código.
