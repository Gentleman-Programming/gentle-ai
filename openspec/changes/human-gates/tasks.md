# SDD Tasks: Human Gates Programáticos

- [ ] 1. Ampliar `ArtifactPaths` en `internal/sddstatus/status.go` con los campos de Gate 1, 2 y 3.
- [ ] 2. Actualizar `resolveArtifactPaths()` para buscar `gate-1-scope.md`, `gate-2-technical.md` y `gate-3-implementation.md`.
- [ ] 3. Ampliar `Dependencies` en `status.go` con los campos de Gate correspondientes.
- [ ] 4. Refactorizar `resolveDependencies()` para condicionar el avance de fases a los gates.
- [ ] 5. Actualizar `resolveNextRecommended()` para retornar `"approve-gate-X"` cuando sea necesario.
- [ ] 6. Añadir constantes para las fases/gates en `internal/sddstatus/status.go` (ej. `PhaseApproveGate1`).
- [ ] 7. Ejecutar `go test ./internal/sddstatus/...` y corregir los tests que asumen avances sin gates (es decir, actualizar los constructores de artifacts de los test).
