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
| Fase 1 — S1a: identidad del self-tool y módulo declarado | 1.1–1.6 | **Completada** | `inc-22/01-update-identity-module` | `901c8f88` |
| Fase 2 — S1b: instrucciones de instalación derivadas de `ToolInfo` | 2.1–2.7 | **Completada** | `inc-22/02-source-install-derived` | _ver «Unidad de trabajo»_ |
| Fases 3–22 | — | Pendientes | — | — |

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

- [x] ~~Fase 2 — S1b: instrucciones de instalación derivadas de `ToolInfo` (REQ-22.2; D-04)~~ → **completada más abajo**
- [ ] Fase 3 — S1c: mutación campo a campo de `init()` en `cmd/axiom` (REQ-22.3; D-01)
- [ ] Fases 4–8 y 10–22 (y el hito de decisión O-2 de la Fase 9, **no** autorizado)

---

## Fase 2: S1b — Instrucciones de instalación derivadas de `ToolInfo` (REQ-22.2; D-04)

### Tareas completadas

- [x] 2.1 [RED] `internal/update/source_install_test.go`: tabla de `SourceInstallCommand(tool, version)` (9 casos) + barrido `TestSourceInstallCommandForkNeverNamesUpstream`
- [x] 2.2 [RED] `internal/update/instructions_test.go`: `TestSelfHintDerivedFromToolInfo` (8 casos) + barrido `TestSelfHintScanAcrossPlatforms`
- [x] 2.3 [GREEN] `internal/update/instructions.go`: `SourceInstallCommand` + `sourceInstallTarget` + `sourceCloneBuildCommand`; `updateHint`/`gentleAIHint` parametrizados por `tool`; **eliminado** `GentleAISourceInstallCommand` sin envoltorio
- [x] 2.4 [GREEN] `internal/update/check.go`: `isGentleAIRepo` delega en `IsSelfTool`; `applyBetaMainHeadStatus` compone vía `SourceInstallCommand`
- [x] 2.5 [GREEN] `internal/update/check_test.go`: el caso de `GentleAISourceInstallCommand` pasa a `SourceInstallCommand`; fixtures de `TestUpdateHint` enriquecidos con los campos reales del registro (ninguna expectativa debilitada)
- [x] 2.6 [REFACTOR] Confirmado: ningún compositor de mensajes recibe `owner, repo, name` sueltos; todo deriva de `ToolInfo`
- [x] 2.7 [Verificación de cierre] V-A, V-B, V-C, V-D

### Archivos tocados

| Archivo | Acción | Qué se hizo |
|---|---|---|
| `internal/update/source_install_test.go` | Creado | `TestSourceInstallCommand` (9 casos: 4 resolubles + 3 fork + 2 campos vacíos) y `TestSourceInstallCommandForkNeverNamesUpstream` (5 versiones) |
| `internal/update/instructions_test.go` | Creado | `TestSelfHintDerivedFromToolInfo` (8 casos fork/upstream × SO) y `TestSelfHintScanAcrossPlatforms` (2 herramientas × 3 SO × 2 funciones) |
| `internal/update/instructions.go` | Modificado | `SourceInstallCommand(tool, version)` con dos ramas (D-04); `sourceInstallTarget`; `sourceCloneBuildCommand`; `updateHint` enruta el self-tool vía `IsSelfTool`; `gentleAIHint(tool, profile)` deriva `brew upgrade <Name>`, `homebrewPackageInstalled(tool.Name)`, la URL `install.sh` de `<Owner>/<Repo>`, el binario de darwin de `<Name>` y la instrucción de Windows de `SourceInstallCommand`. **Eliminado** `GentleAISourceInstallCommand` |
| `internal/update/check.go` | Modificado | `isGentleAIRepo` → `return IsSelfTool(tool)` (D-01); `applyBetaMainHeadStatus` → `SourceInstallCommand(result.Tool, result.LatestVersion)` (D-04) |
| `internal/update/check_test.go` | Modificado | Caso beta → `SourceInstallCommand(result.Tool, …)`; fixtures de `TestUpdateHint` del self-tool enriquecidos con `Owner`/`Repo`/`GoImportPath`/`GoModulePath` del registro real (expectativas intactas) |
| `internal/update/upgrade/strategy.go` | Modificado | Los 2 call sites de `update.GentleAISourceInstallCommand` → `update.SourceInstallCommand(r.Tool, r.LatestVersion)` (mecánico, ver Desviaciones) |
| `internal/update/upgrade/effective_method_routing_test.go` | Modificado | Fixture de `TestGentleAIWindowsWithoutGoNamesRunnableSourceInstall`: añadido `GoModulePath` para declarar el módulo upstream (ver Problemas) |
| `internal/update/upgrade/windows_distribution_policy_test.go` | Modificado | `TestGentleAIWindowsUpgradeFailsClosedToSourceInstall`: expectativa realineada a REQ-22.2 (clon + build derivado, **cero** `go install`); `wantTarget` pasa de `@v2.2.0` a `v2.2.0` |

### TDD Cycle Evidence

| Tarea | Test File | Capa | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|------|------------|-----|-------|-------------|----------|
| 2.1 | `internal/update/source_install_test.go` | Unit | N/A (fichero nuevo) | ✅ Escrito y observado en fallo de compilación | ✅ Pasó | ✅ 9 + 5 casos | ✅ Limpio |
| 2.2 | `internal/update/instructions_test.go` | Unit | ✅ `TestUpdateHint` 12/12 en línea base | ✅ Escrito y observado en fallo de compilación | ✅ Pasó | ✅ 8 + 12 combinaciones | ✅ Limpio |
| 2.3 | (satisfechos por 2.1 y 2.2) | Unit | ✅ línea base capturada | — | ✅ 15/15 en verde | ✅ incluida arriba | ✅ Limpio |
| 2.4 | (satisfechos por 2.1 y 2.2) | Unit | ✅ sin regresión | — | ✅ 15/15 en verde | ✅ incluida arriba | ✅ Limpio |
| 2.5 | (satisfecho por 2.2) | Unit | ✅ sin regresión | — | ✅ `TestUpdateHint` 12/12 | ✅ incluida arriba | ✅ Limpio |

**RED observado:** sí. Prueba real:

```text
internal\update\instructions_test.go:89:34: too many arguments in call to gentleAIHint
	have (ToolInfo, system.PlatformProfile)
	want (system.PlatformProfile)
internal\update\instructions_test.go:126:63: too many arguments in call to gentleAIHint
internal\update\source_install_test.go:102:11: undefined: SourceInstallCommand
internal\update\source_install_test.go:134:10: undefined: SourceInstallCommand
FAIL	github.com/gentleman-programming/gentle-ai/v3/internal/update [build failed]
```

**GREEN observado:** sí. Prueba real: `go test ./internal/update/ -run 'TestSourceInstall|TestSelfHint|TestUpdateHint|TestGentleAI|TestIsSelfTool|TestGoInstallResolvable|TestCheckSingleToolGentleAI' -v` → `PASS` con **15/15 tests** en verde (9 + 5 de `SourceInstallCommand`, 2 de escaneo de hints, 12 subtests de `TestUpdateHint` incluidos en su test, más los de identidad y los 7 `TestCheckSingleToolGentleAI*`).

**REFACTOR (2.6):** hecho. Barrido de firmas sobre `internal/update/*.go` (sin `upgrade/`) con parámetros sueltos `owner|repo|name|toolName`:

```text
homebrew.go:16: func defaultHomebrewPackageInstalled(toolName string) bool
homebrew.go:74: func DetectHomebrewOwnership(toolName string) (HomebrewOwnership, error)
types.go:13: func IsSelfToolName(name string) bool
```

Los tres son **consultas o predicados de identidad**, no compositores de mensajes. Todos los compositores de mensajes de `internal/update` (`SourceInstallCommand`, `sourceCloneBuildCommand`, `updateHint`, `updateHintForOwnership`, `openCodeRegisteredNotMaterializedHint`, `gentleAIHint`) reciben `ToolInfo`. Cumple la invariante.

### Work Unit Evidence

| Evidencia | Valor |
|---|---|
| Focused test command y resultado exacto | `go test ./internal/update/ -run 'TestSourceInstall\|TestSelfHint\|TestUpdateHint\|TestGentleAI\|TestIsSelfTool\|TestGoInstallResolvable\|TestCheckSingleToolGentleAI' -timeout 300s` → `ok github.com/gentleman-programming/gentle-ai/v3/internal/update 1.689s`, 15/15 tests en verde |
| Runtime harness y resultado | **N/A** — funciones puras de composición de cadenas sobre `ToolInfo`; sin proceso hijo, red ni superficie de usuario en esta rebanada |
| Rollback boundary | Revertir el commit de la unidad deshace exactamente esta rebanada: `instructions.go` recupera `GentleAISourceInstallCommand` y sus 2 call sites en `upgrade/strategy.go`, y se eliminan los dos ficheros de test nuevos. `check.go` vuelve al predicado literal. Sin dependencias nuevas fuera de `internal/update` |

### Unidad de trabajo

- **Rama:** `inc-22/02-source-install-derived` apilada sobre `inc-22/01-update-identity-module` (convención `stacked-to-main` de `tasks.md`)
- **Commit:** único commit de unidad (tests + producción + este documento + `tasks.md` juntos)
- **Presupuesto de revisión:** ~55 líneas de producción + ~230 de tests + ajustes de fixtures → por debajo del umbral de 400 líneas cambiadas
- **Mensaje:** `feat(update): derive install instructions from ToolInfo (D-04)`

### Desviaciones respecto al diseño

**D-04 — la eliminación de `GentleAISourceInstallCommand` obliga a tocar `upgrade/strategy.go`, que la tabla §4 de S1b no lista.** `design.md` D-04 enumera como consumidores a eliminar `check.go:201`, `instructions.go:71`, `strategy.go:635`, `strategy.go:767` y `check_test.go:630`. Pero la tabla de «Cambios de Ficheros por Rebana» de §4 para **S1b** solo lista `instructions.go` y `check.go`; `strategy.go` aparece en **S2c** (Fase 6).

Es un **conflicto interno del propio diseño**: no se puede eliminar la función en la Fase 2 sin que `internal/update/upgrade` deje de compilar. Se ha resuelto haciendo en `strategy.go` **solo la adaptación mecánica de los 2 call sites** (`update.SourceInstallCommand(r.Tool, r.LatestVersion)`), sin entrar en el re-anclaje de identidad de esas funciones (URLs de `<Owner>/<Repo>`, `isBetaGentleAIUpgrade`, `gentleAIModulePath`, fallback genérico de Windows), que **queda íntegro para la Fase 6** (tareas 6.1–6.5). Recomendación: añadir `upgrade/strategy.go` a la fila S1b de §4, o mover la eliminación de la función a la Fase 6.

**`gentleAIHint` en darwin deriva el binario de `tool.Name`.** La tarea 2.3 enumera tres derivaciones (`brew upgrade <Name>`, `homebrewPackageInstalled(tool.Name)`, URL `install.sh`) y no menciona la frase de darwin. Se deriva igualmente (`tool.Name + " upgrade (downloads pre-built binary)"`) porque el escenario REQ-22.2 «La pista de actualización nombra el fork, no upstream» exige que la instrucción del fork «DEBE referenciar la identidad del fork», y la frase literal `gentle-ai upgrade` no lo hacía. Para la entrada upstream el resultado es **idéntico** al de hoy (cero cambio de comportamiento); para el fork pasa a `axiom upgrade (…)`.

### Problemas encontrados

1. **`design.md` §4 (S1b) incompleto** — ver Desviaciones. Conflicto interno, no bloqueante.

2. **Dos tests de `upgrade/` fijaban el literal `go install` upstream que D-04 elimina**, con fixtures que no declaraban `GoModulePath` (campo añadido en la Fase 1). Su tratamiento difiere porque modelan situaciones distintas:
   - `TestGentleAIWindowsUpgradeFailsClosedToSourceInstall`: su fixture **no** declara `GoImportPath`, y esa ausencia es *load-bearing* (`gentleAISelfUpgradeMethod` enruta a `InstallGoInstall` solo si `GoImportPath != ""`; el test afirma `result.Method == update.InstallBinary`). Enriquecer el fixture rompía la intención del test. Se ha **revertido** el fixture y se ha realineado la expectativa a REQ-22.2: sin módulo declarado **no** se emite `go install`, y la vía manual es el clon + build derivado. Se añade la aserción explícita de **cero** `go install` en la pista.
   - `TestGentleAIWindowsWithoutGoNamesRunnableSourceInstall`: su fixture **sí** declara `GoImportPath` (modela una herramienta que podría hacer `go install` si hubiera Go), y `GoAvailable: false` mantiene el ruteo en `InstallBinary` con independencia de ese campo. Se ha añadido `GoModulePath` para que el `go install` que el test exige sea **legal** bajo D-04 (derivado de un módulo declarado resoluble) en vez del literal hardcodeado.

3. **Los fallos ambientales conocidos siguen ahí, con el set ampliado ya documentado en la Fase 1** (regla 5): `TestNoUpdatesPath`, `TestDetectHomebrewOwnershipWith`, `TestCheckSingleTool_EngramUsesBinaryReleaseChannel`, `TestInstallScriptBetaGoInstallBypassesPublicGoProxy`, `TestReleaseSecurityScriptsAreSyntacticallyValidAndFailClosed` (7 subtests), `TestRequireCISuccessSelectsNewestExactCommitRun` (6 subtests), `TestCanonicalReleasePublicKeysControlRealLinkerBuild`, `TestReleaseDistributionPolicyAssertionFailsClosed`, `TestReleaseDistributionPolicyAcceptsSemanticYAMLFormatting`, `TestModifiedReleaseVerifierCannotGainWriteAuthority` y `TestRunStrategyUsesCaskOwnershipAndMigrationGuidance` (en `upgrade/`). Casi todos por `bash`/`printf` ausentes en este Windows. **Ninguno se ha tocado.** El fallo de `TestCheckSingleTool_EngramUsesBinaryReleaseChannel` queda documentado con su salida real: `engram status = "version-unknown", want "up-to-date" (installed="", latest="1.15.13", err=<nil>)` — dependencia de estado de máquina (detección de `engram` instalado), no atribuible a esta rebanada.

4. **Corrección de un error del propio test RED durante el GREEN.** El barrido de literales upstream de `TestSelfHintDerivedFromToolInfo` se aplicaba inicialmente a **toda** entrada self-tool, incluida la upstream — cuyo módulo legítimo **es** `gentleman-programming/gentle-ai`. La prohibición de REQ-22.2 está acotada a la **entrada del fork**. El barrido se ha acotado en consecuencia (las aserciones de cadena exacta de ambos pares siguen intactas, y `TestSelfHintScanAcrossPlatforms` ya estaba bien acotado).

**Cero regresiones** atribuibles a esta rebanada: el set de fallos de `go test ./internal/update/...` es **idéntico** al de la línea base (capturada antes de escribir el RED).

### Comprobaciones de cierre (2.7)

| Compuerta | Comando | Resultado observado |
|---|---|---|
| V-A build | `go build ./...` | sin salida (OK) |
| V-A vet | `go vet ./...` | sin salida (OK) |
| V-B foco nuevo | `go test ./internal/update/ -run 'TestSourceInstall\|TestSelfHint\|TestUpdateHint\|TestGentleAI\|TestIsSelfTool\|TestGoInstallResolvable\|TestCheckSingleToolGentleAI' -v -timeout 300s` | `PASS`, 15/15 tests en verde |
| V-B patrón literal de la tarea | `go test ./internal/update/... -run 'TestSourceInstall\|TestInstructions\|TestUpdateHint\|TestCheck\|TestGentleAI' -timeout 300s` | `ok` en `upgrade/`; en `internal/update` solo falla `TestCheckSingleTool_EngramUsesBinaryReleaseChannel` (ambiental, regla 5, documentado arriba). Nota: el patrón `TestInstructions` no casa ningún test; los tests nuevos se llaman `TestSelfHint*` y se han añadido al foco |
| V-B paquete | `go test ./internal/update/... -timeout 600s` | **FAIL** solo con los fallos ambientales preexistentes listados arriba; **cero** fallos nuevos |
| V-C formato | `gofmt -l internal/update/ internal/update/upgrade/` | sin salida (OK) |
| V-D rutas prohibidas | `git diff --name-only` | solo `internal/update/check.go`, `internal/update/check_test.go`, `internal/update/instructions.go`, `internal/update/upgrade/effective_method_routing_test.go`, `internal/update/upgrade/strategy.go`, `internal/update/upgrade/windows_distribution_policy_test.go` (+ ficheros nuevos de test). Ninguna ruta prohibida tocada; ningún branding heredado alterado |
| V-E compuerta de control | — | N/A (la Fase 2 no tiene compuerta; las compuertas son de las Fases 6, 7, 12, 17 y 19) |

### Tareas restantes

- [ ] Fase 3 — S1c: mutación campo a campo de `init()` en `cmd/axiom` (REQ-22.3; D-01)
- [ ] Fases 4–8 y 10–22 (y el hito de decisión O-2 de la Fase 9, **no** autorizado)
