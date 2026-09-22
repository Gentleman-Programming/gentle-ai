# Progreso de Implementación — inc-22-axiom-updater-and-skills-index-governance

> **Incremento:** `inc-22-axiom-updater-and-skills-index-governance`
> **Fase SDD:** `sdd-apply` · **Fecha:** 2026-09-22
> **Modo:** Strict TDD · **Test runner:** `go test`
> **Almacén de artefactos:** `hybrid` · **Estrategia de entrega:** `auto-chain` / `stacked-to-main`
> **Idioma del artefacto:** español (castellano), registro neutro y profesional, tuteo. Identificadores Go, rutas, comandos CLI y literales técnicos en inglés.
> **Formato de continuación:** si este documento supera los 50.000 bytes, las partes siguientes se serializan como `apply-progress_part2.md`, `apply-progress_part3.md`, etc. (mismo patrón que INC-21).

---

## Estado acumulado

| Fase | Tareas | Estado | Rama | Commit |
|---|---|---|---|---|
| Fase 1 — S1a: identidad del self-tool y módulo declarado | 1.1–1.6 | **Completada** | `inc-22/01-update-identity-module` | _ver «Unidad de trabajo»_ |
| Fases 2–22 | — | Pendientes | — | — |

---

## Fase 1: S1a — Identidad del self-tool y módulo declarado (REQ-22.2, REQ-22.3; D-01)

### Tareas completadas

- [x] 1.1 [RED] `internal/update/identity_test.go`: tabla de `IsSelfToolName` e `IsSelfTool`
- [x] 1.2 [RED] `internal/update/module_test.go`: tabla de `GoInstallResolvable()`
- [x] 1.3 [GREEN] `internal/update/types.go`: `selfToolNames`, `IsSelfToolName`, `IsSelfTool`, `ToolInfo.GoModulePath`, `ToolInfo.GoInstallResolvable()`
- [x] 1.4 [GREEN] `internal/update/registry.go`: entrada self-tool declara `GoModulePath`
- [x] 1.5 [REFACTOR] Confirmación de invariantes de identidad
- [x] 1.6 [Verificación de cierre] V-A, V-B, V-C, V-D

### Archivos tocados

| Archivo | Acción | Qué se hizo |
|---|---|---|
| `internal/update/identity_test.go` | Creado | Tablas `TestIsSelfToolName` (8 casos) y `TestIsSelfTool` (3 casos) |
| `internal/update/module_test.go` | Creado | Tabla `TestGoInstallResolvable` (9 casos), con `Owner`/`Repo` deliberadamente ajenos a las rutas bajo prueba |
| `internal/update/types.go` | Modificado | `selfToolNames` + `IsSelfToolName` + `IsSelfTool`; campo `ToolInfo.GoModulePath`; método `GoInstallResolvable()`; auxiliar `isGoMajorVersionSuffix` |
| `internal/update/registry.go` | Modificado | La entrada self-tool declara `GoModulePath: "github.com/gentleman-programming/gentle-ai/v3"` (literal de `go.mod:1`), sin renombrar ni reordenar campos |

### TDD Cycle Evidence

| Tarea | Test File | Capa | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|------|------------|-----|-------|-------------|----------|
| 1.1 | `internal/update/identity_test.go` | Unit | N/A (fichero nuevo) | ✅ Escrito y observado en fallo de compilación | ✅ Pasó | ✅ 8 + 3 casos | ✅ Limpio |
| 1.2 | `internal/update/module_test.go` | Unit | N/A (fichero nuevo) | ✅ Escrito y observado en fallo de compilación | ✅ Pasó | ✅ 9 casos | ✅ Limpio |
| 1.3 | (satisfechos por 1.1 y 1.2) | Unit | ✅ línea base capturada | — | ✅ 20/20 subtests | ✅ incluida arriba | ✅ Limpio |
| 1.4 | (satisfecho por 1.2) | Unit | ✅ sin regresión | — | ✅ 20/20 subtests | ✅ incluida arriba | ✅ Limpio |

**RED observado:** sí. Prueba real:

```text
internal\update\identity_test.go:25:14: undefined: IsSelfToolName
internal\update\identity_test.go:46:14: undefined: IsSelfTool
internal\update\module_test.go:87:5: unknown field GoModulePath in struct literal of type ToolInfo
internal\update\module_test.go:90:19: tool.GoInstallResolvable undefined (type ToolInfo has no field or method GoInstallResolvable)
FAIL	github.com/gentleman-programming/gentle-ai/v3/internal/update [build failed]
```

**GREEN observado:** sí. Prueba real: `go test ./internal/update/ -run 'TestIsSelfTool|TestGoInstallResolvable' -v` → `PASS` con **20/20 subtests** en verde (8 `IsSelfToolName` + 3 `IsSelfTool` + 9 `GoInstallResolvable`).

**REFACTOR (1.5):** hecho. Comprobaciones:
- El único literal `"gentle-ai"` **nuevo** como nombre de identidad está dentro de `selfToolNames`. Los demás occurrences en `types.go`/`registry.go` son preexistentes (comentarios y campos del registro) o el literal de módulo que la tarea 1.4 exige literalmente.
- `types.go` no duplica la lógica de identidad: `IsSelfTool` delega en `IsSelfToolName`; `GoInstallResolvable` no compara nombres de herramienta.
- `gofmt -l` sobre los cuatro ficheros tocados: sin salida.

### Work Unit Evidence

| Evidencia | Valor |
|---|---|
| Focused test command y resultado exacto | `go test ./internal/update/ -run 'TestIsSelfTool\|TestGoInstallResolvable' -v -timeout 300s` → `PASS` (`ok ... 0.936s`), 20/20 subtests |
| Runtime harness y resultado | **N/A** — biblioteca pura sin superficie de usuario ni proceso hijo (predicados y un método sobre `ToolInfo`) |
| Rollback boundary | Revertir el commit de la unidad deshace exactamente esta rebanada: `internal/update/types.go` y `internal/update/registry.go` vuelven a su estado previo y se eliminan `internal/update/identity_test.go`, `internal/update/module_test.go` y este documento. Sin llamadores nuevos: nadie consume aún `IsSelfTool*`/`GoInstallResolvable`, luego el árbol sigue compilando tras el revert |

### Unidad de trabajo

- **Rama:** `inc-22/01-update-identity-module` (convención `stacked-to-main` de `tasks.md`)
- **Commit:** único commit de unidad (tests + producción + este documento juntos)
- **Presupuesto de revisión:** ~65 líneas de producción + ~130 de tests + este documento → por debajo del umbral de 400 líneas cambiadas
- **Mensaje:** `feat(update): add self-tool identity predicate and declared GoModulePath`

### Desviaciones respecto al diseño

**D-01 — `GoInstallResolvable()` refina el cuerpo del snippet.** El snippet de `design.md` §D-01 propone:

```go
return p == m || strings.HasPrefix(p, m+"/")
```

Ese cuerpo devuelve `true` para `GoModulePath = "github.com/gentleman-programming/gentle-ai"` (sin `/v3`) con `GoImportPath = ".../gentle-ai/v3/cmd/gentle-ai"`, porque `strings.HasPrefix(p, m+"/")` sí se cumple a nivel de cadena. Sin embargo `tasks.md` 1.2 y `design.md` §6 (fila «Unit — modulo declarado») exigen **`false`** para ese caso, con razón semántica: en el sistema de módulos de Go, `m + "/vN"` (N ≥ 2) es un **módulo distinto** de `m`, luego `go install .../gentle-ai/v3/...` resuelve el módulo `.../gentle-ai/v3` y no el declarado `.../gentle-ai`.

La afirmación de `tasks.md` 1.2 de que el caso es «false … con la regla de D-01» es **incorrecta respecto al código** del snippet (el código daría `true`), pero el **resultado esperado** (`false`) es el correcto y es el que fijan la tabla de aceptación de `tasks.md` y el plan de pruebas de `design.md` §6.

Se ha implementado por tanto la **misma aproximación** de D-01 (solo se lee el módulo declarado; nunca se deriva de `Owner`/`Repo`) con un cuerpo que satisface la tabla completa: tras comprobar igualdad o prefijo por segmento de camino, se rechaza cuando el primer segmento restante es un sufijo de versión mayor de Go (`v2`, `v3`, …). Se añaden dos casos de triangulación que constriñen la lógica: un módulo v1 que sí resuelve su propio `cmd/` (`true`) y un prefijo solo de cadena (`.../gentle-ai` frente a `.../gentle-ai-extra/...`, `false`).

Ninguna otra desviación. El resto de la implementación sigue D-01 literalmente.

### Problemas encontrados

1. **La lista de «fallos ambientales conocidos» del prompt estaba incompleta.** El prompt anunciaba `TestNoUpdatesPath` y `TestDetectHomebrewOwnershipWith`. La línea base real de `go test ./internal/update/...` en esta máquina falla además en (todos ellos **preexistentes**, idénticos antes y después del cambio):
   - `TestCheckSingleTool_EngramUsesBinaryReleaseChannel` (depende de estado de máquina)
   - `TestInstallScriptBetaGoInstallBypassesPublicGoProxy` (requiere `bash`; WSL no encuentra `/bin/bash`)
   - `TestReleaseSecurityScriptsAreSyntacticallyValidAndFailClosed` (7 subtests; requiere `bash -n`)
   - `TestRequireCISuccessSelectsNewestExactCommitRun` (6 subtests; requiere `bash`)
   - `TestCanonicalReleasePublicKeysControlRealLinkerBuild` (requiere `bash`)
   - `TestReleaseDistributionPolicyAssertionFailsClosed` / `TestReleaseDistributionPolicyAcceptsSemanticYAMLFormatting` / `TestModifiedReleaseVerifierCannotGainWriteAuthority` (requieren `bash`)
   - `TestRunStrategyUsesCaskOwnershipAndMigrationGuidance` (en `internal/update/upgrade`; requiere `printf`)
   
   **Ninguno se ha tocado** (fuera de alcance). **Cero regresiones** atribuibles a esta rebanada: el set de fallos es idéntico al de la línea base.

2. **Conflicto interno en `design.md`:** §D-01 (código) y §6 (plan de pruebas) no coinciden en el caso de prefijo parcial. Se ha resuelto a favor de §6/tabla de `tasks.md` y se documenta arriba. Recomendación para fases posteriores: alinear el snippet de D-01 con el cuerpo implementado.

### Comprobaciones de cierre (1.6)

| Compuerta | Comando | Resultado observado |
|---|---|---|
| V-A build | `go build ./...` | sin salida (OK) |
| V-A vet | `go vet ./internal/update/...` | sin salida (OK) |
| V-B foco nuevo | `go test ./internal/update/ -run 'TestIsSelfTool\|TestGoInstallResolvable' -v -timeout 300s` | `PASS`, 20/20 subtests |
| V-B paquete | `go test ./internal/update/... -timeout 300s` | **FAIL** solo con los fallos ambientales preexistentes listados arriba; **cero** fallos nuevos |
| V-C formato | `gofmt -l internal/update/types.go internal/update/registry.go internal/update/identity_test.go internal/update/module_test.go` | sin salida (OK) |
| V-D rutas prohibidas | `git diff --name-only` | solo `internal/update/registry.go`, `internal/update/types.go` (+ ficheros nuevos de test). Ninguna ruta prohibida tocada |
| V-E compuerta de control | — | N/A (la Fase 1 no tiene compuerta; las compuertas son de las Fases 6, 7, 12, 17 y 19) |

### Tareas restantes

- [ ] Fase 2 — S1b: instrucciones de instalación derivadas de `ToolInfo` (REQ-22.2; D-04)
- [ ] Fase 3 — S1c: mutación campo a campo de `init()` en `cmd/axiom` (REQ-22.3; D-01)
- [ ] Fases 4–8 y 10–22 (y el hito de decisión O-2 de la Fase 9, **no** autorizado)
