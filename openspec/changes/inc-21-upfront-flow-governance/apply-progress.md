# Progreso de Implementación: Determinación Temprana de Flujo, Compuertas de Revisión por Bloque, Relevo de Cierre de Roles y Ciclo de Vida de Archive (INC-21)

> **Alcance de esta ejecución:** únicamente las Fases 1 a 4 del documento `tasks.md` (rebanada de diseño P1, dominio puro `internal/kickoff`: tipos, esquema, sellado, retro-sellado, ledger y digest). Las Fases 5 a 23 quedan sin empezar para una ejecución posterior.
> **Modo:** Strict TDD (`openspec/config.yaml: strict_tdd: true`). Cada tarea `[RED]` se escribió y se observó fallar (por fallo de compilación o por aserción) antes de su tarea `[GREEN]` correspondiente.
> **Rama:** `feature/inc-21-upfront-flow-governance` (tracker, sin merge directo a `main`). Sin push, sin creación de ramas hijas ni PRs — decisiones del usuario.

## Estado de Fases y Tareas

### Fase 1: P1a — Tipos y esquema del dominio `kickoff` (REQ-21.5, REQ-21.6; D-01, D-02)
- [x] 1.1 [RED] `internal/kickoff/schema_test.go`: tabla de casos para `Validate()`.
- [x] 1.2 [GREEN] `internal/kickoff/types.go`: `Kickoff`, `FlowMode`, `ExecutionStyle`, `HandoffPolicy`, `Lifecycle`, `Gate`, `GateKey`, `GateDecision`, `EvidenceKind`, `GateRecord`, `GateLedger`, `GateState`, enums, `RoleApplyGate`.
- [x] 1.3 [GREEN] `internal/kickoff/schema.go`: vocabularios cerrados y `Validate() error`.
- [x] 1.4 [REFACTOR] GoDoc en inglés; sin duplicación entre listas de enum y constantes.
- [x] 1.5 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

### Fase 2: P1b — Sellado de kickoff, escritura única (REQ-21.5; D-01, D-02, D-04)
- [x] 2.1 [RED] `internal/kickoff/seal_test.go`: primer sello escribe; segundo sello con contenido distinto no reescribe; fichero ilegible falla; sin temporal residual.
- [x] 2.2 [GREEN] `internal/kickoff/seal.go`: `Seal`/`Load` sobre `PublishFileNoReplace`, calcado de `ensureChangeInstanceMarker`.
- [x] 2.3 [REFACTOR] `Seal` solo depende de `nowFunc` (inyectable) como reloj; sin estado global.
- [x] 2.4 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

### Fase 3: P1c — Regla de retro-sellado (REQ-21.4; D-01 a D-04; §8.1; O-1 ya resuelta)
- [x] 3.1 [RED] `internal/kickoff/infer_test.go`: los tres casos de retro-sellado, incluida la aserción que fija O-1 (nunca `fullstack` cuando `DetectRoles` ya resuelve un rol sin error).
- [x] 3.2 [GREEN] `internal/kickoff/infer.go`: `InferKickoff(designPath, wsConfig) (Kickoff, string, error)`.
- [x] 3.3 [REFACTOR] Se detectó y corrigió una violación de pureza: la primera versión de `InferKickoff` llamaba a `os.Stat` por su cuenta para distinguir "sin design.md" de "rol inválido", además de la lectura ya encapsulada en `DetectRoles`. Se sustituyó por `errors.Is(detectErr, os.ErrNotExist)` sobre el propio error de `DetectRoles`, dejando `InferKickoff` sin más E/S que la de `DetectRoles`, tal como exige la tarea.
- [x] 3.4 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

### Fase 4: P1d — Ledger de compuertas y digest de artefactos (REQ-21.12; D-02, D-08)
- [x] 4.1 [RED] `internal/kickoff/ledger_test.go`: anexado preserva historial; escritura concurrente (`sync.WaitGroup`, 20 goroutines) no pierde registros; fichero ausente ⇒ ledger vacío sin error.
- [x] 4.2 [GREEN] `internal/kickoff/ledger.go`: `AppendGate`/`LoadGates` sobre `AcquireAuthorityFileLock` + `ReplaceFileAtomic` + `SyncReviewDirectory`, con reintento acotado (200 intentos × 2 ms) ante `ErrStoreLockContended` (el cerrojo es no bloqueante por diseño; el reintento lo posee el llamador).
- [x] 4.3 [RED] `internal/kickoff/digest_test.go`: mismo contenido con CRLF/LF ⇒ mismo digest; orden de rutas de entrada no altera el resultado; fichero ausente ⇒ error.
- [x] 4.4 [GREEN] `internal/kickoff/digest.go`: `ArtifactDigest(paths []string) (string, error)`.
- [x] 4.5 [REFACTOR] Se detectó y corrigió un bug real en GREEN: la primera versión hasheaba las rutas en el orden recibido, no en orden canónico, y el test de invariancia de orden (4.3) falló genuinamente en rojo tras implementar GREEN. Corregido ordenando una copia de `paths` con `sort.Strings` antes de hashear, sin mutar el slice del llamador. Sin duplicación de normalización de fin de línea (vive solo en `digest.go`).
- [x] 4.6 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

## TDD Cycle Evidence

| Tarea | Fichero de test | Capa | Red de seguridad | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1–1.3 | `schema_test.go` | Unit | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 17 subpruebas PASS | ✅ 7 enums × válido/inválido | ✅ Limpio |
| 2.1–2.2 | `seal_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 9 pruebas PASS, 1 SKIP (Windows) | ✅ éxito/colisión/fallo de publicación/reloj inyectado | ✅ Limpio |
| 3.1–3.2 | `infer_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 5 pruebas PASS | ✅ 3 casos de retro-sellado + guarda de pureza | ✅ Purismo de E/S corregido (ver 3.3 arriba) |
| 4.1–4.2 | `ledger_test.go` | Unit + concurrencia (`sync.WaitGroup`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 4 pruebas PASS | ✅ secuencial + concurrente + registro inválido | ✅ Limpio |
| 4.3–4.4 | `digest_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ❌→✅ 1ª ejecución GREEN falló genuinamente en `TestArtifactDigestPathOrderDoesNotAlterResult` (bug real de orden); corregido con `sort.Strings` | ✅ CRLF/LF + orden + contenido distinto + fichero ausente | ✅ Limpio |

### Test Summary
- **Tests totales escritos**: 40 (27 funciones de test de nivel superior + 13 sub-tests de tablas: 7 en `TestKickoffValidateEnums`, 6 en `TestGateLedgerValidate`)
- **Tests en verde**: 39 PASS + 1 SKIP (intencional, paridad con precedente `edit_authority_test.go` en Windows) + 0 FAIL
- **Capas usadas**: Unit (40), Integración (0 — dominio puro sin consumidores todavía), E2E (0)
- **Funciones puras creadas**: `Validate` (×3), `RoleApplyGate`, `ArtifactDigest`, `InferKickoff`, `inferredKickoff`, `normalizeLineEndings`

## Work Unit Evidence

| PR | Unidad de trabajo | Comando de test enfocado | Resultado observado | Arnés de runtime | Frontera de rollback |
|---|---|---|---|---|---|
| 1 | Tipos y validación de esquema | `go test ./internal/kickoff/... -run TestValidate\|TestParseKickoff\|TestGateLedgerValidate\|TestParseGateLedger` | PASS (17/17 subpruebas) | N/A — biblioteca pura sin superficie de usuario hasta fases posteriores | Eliminar `types.go`/`schema.go`; aditivo puro, sin llamadores |
| 2 | Sellado con escritura única | `go test ./internal/kickoff/... -run TestSeal` | PASS (8/9, 1 SKIP Windows) | N/A — sin superficie de usuario todavía | Eliminar `seal.go`; nada lo invoca aún |
| 3 | Retro-sellado (O-1 aplicada) | `go test ./internal/kickoff/... -run TestInferKickoff` | PASS (5/5) | N/A — sin superficie de usuario todavía | Eliminar `infer.go` |
| 4 | Ledger + digest | `go test ./internal/kickoff/... -run TestAppendGate\|TestLoadGates\|TestArtifactDigest` | PASS (8/8) | N/A — sin superficie de usuario todavía | Eliminar `ledger.go`/`digest.go` |

Arnés de runtime N/A en las cuatro unidades: la rebanada P1 es intencionalmente hoja y sin consumidores (design.md §8.2, "P1: paquete nuevo sin consumidores: el binario no cambia de comportamiento"); la Fase 9 (fuera de este alcance) es la primera que cablea un verbo de CLI ejecutable.

## Comandos de Verificación Ejecutados (evidencia real, por fase y agregada)

```text
go test ./internal/kickoff/...            → ok (todas las fases, tras cada GREEN/REFACTOR)
go vet ./internal/kickoff/...             → sin salida (limpio)
gofmt -l internal/kickoff/*.go            → sin salida (con formato correcto)
go build ./...                            → sin salida (compila todo el repositorio)
go vet ./...                              → sin salida (todo el repositorio)
```

**Limitación de entorno observada y honesta**: la tabla de pruebas del diseño (§6) pide `-race` para `TestAppendGateConcurrentWritesLoseNoRecords`. `go test ./internal/kickoff/... -race` falla con `CGO_ENABLED` requerido, y este entorno Windows no tiene `gcc`/`cc` instalado (verificado: `where gcc`/`where cc` no encuentran ningún binario, y no hay MinGW/MSYS2/TDM-GCC/Rtools en las rutas habituales). La prueba de concurrencia se ejecutó y pasó SIN `-race` (funcionalmente correcta: 20/20 goroutines, sin registro perdido ni duplicado), pero el detector de carreras de Go no pudo ejecutarse en esta máquina. Esto es una limitación del entorno, no un defecto de la implementación ni del test.

## Deviations from Design

1. **Tipo adicional no listado explícitamente en tasks.md 1.2**: se añadió `KickoffRole` (rol individual sellado bajo `kickoff.roles`, con `Role`, `GatePolicy`, `TasksFile`, `VerifyFile`) porque `multirole.RoleAssignment` no tiene campos para `tasks_file`/`verify_file`, y `internal/multirole` debe permanecer intacto en estas fases (esos dos campos son exclusivos de D-07/Fase 15, fuera de este alcance). Se añadió también `FlowConfig` como el tipo nombrado para el bloque `kickoff:` del documento sellado (evita un campo `Kickoff.Kickoff` auto-referenciado). Ambos son aditivos, no exportan comportamiento nuevo más allá de lo que el esquema YAML de `design.md` §5.1 ya exige, y están documentados con GoDoc.
2. **Corrección de pureza en `InferKickoff` (Fase 3, tarea 3.3)**: la primera implementación hacía una llamada extra a `os.Stat(designPath)` antes de invocar `DetectRoles`, violando "sin más E/S que la ya encapsulada en DetectRoles". Corregido inspeccionando `errors.Is(detectErr, os.ErrNotExist)` sobre el error que `DetectRoles` ya devuelve, sin ninguna llamada de E/S adicional.
3. **Corrección de orden en `ArtifactDigest` (Fase 4, tarea 4.3)**: la primera implementación hasheaba las rutas en el orden literal del slice de entrada; el test RED de invariancia de orden falló genuinamente en la primera ejecución GREEN. Corregido con `sort.Strings` sobre una copia del slice antes de hashear.
4. **`resolveRoleArtifactFile` (barrido, no publicado)**: durante el diseño de la Fase 3 se consideró replicar la lógica de resolución de `tasks.<rol>.md` → `tasks.md` de `multirole.EvaluateBarrier` (`barrier.go:87-89`) con sondeos `os.Stat` adicionales. Se descartó explícitamente por violar la pureza exigida en 3.3 y porque ninguna prueba RED de 3.1 lo exigía; los roles no-`fullstack` retro-sellados usan siempre la convención `tasks.<rol>.md`/`verify-report.<rol>.md` (la que `EvaluateBarrier` intenta primero), sin sondeo de existencia.

No hay desviaciones respecto a D-01 a D-04, D-08 (la parte de tipos/vocabulario), ni respecto a la frontera normativa de RDD (§1.3): ningún fichero de este alcance importa `internal/reviewtransaction` más allá de `PublishFileNoReplace`, `ReplaceFileAtomic`, `SyncReviewDirectory` y `AcquireAuthorityFileLock`/`AuthorityFileLock`/`ErrStoreLockContended`, todos ellos fontanería explícitamente autorizada por design.md §1.3 regla 1 y su guardián estructural (`review_offer_absence_guard_test.go`).

## Issues Found

1. **Presupuesto de 400 líneas por PR excedido en la PR 1, sin válvula de alivio disponible.** `tasks.md` solo declara válvulas de alivio explícitas para las Fases 4, 6, 7 y 15 — la Fase 1 no tiene una. El pronóstico de diseño para la PR 1 era 180–280 líneas; el diff real (`types.go` + `schema.go` + `schema_test.go`, más el `tasks.md` de marcado) fue de 534 inserciones + 5 eliminaciones = **539 líneas cambiadas**, principalmente por la cobertura de tabla exhaustiva de 17 subpruebas que exige la disciplina TDD estricta y la instrucción explícita de "nunca borrar tests para caber en el presupuesto". No se recortaron pruebas ni comentarios para forzar el ajuste al presupuesto. Se recomienda al mantenedor aceptar `size:exception` para la PR 1, o bien reparticionarla retroactivamente en dos PRs (tipos+enums / esquema+validación) si el flujo de revisión real lo exige.
2. **`-race` no ejecutable en este entorno** (ver limitación de entorno arriba). No es un defecto de INC-21.

## Estimación de Líneas Cambiadas por PR (evidencia real, no pronóstico)

| PR | Rama prevista (tracker) | Commit | Líneas reales (adiciones+eliminaciones) | Presupuesto 400 | Válvula de alivio usada |
|---|---|---|---|---|---|
| 1 | `inc-21/01-kickoff-types-schema` | `eed8fec5` | 534 + 5 = **539** | ⚠️ Excedido (sin válvula declarada en tasks.md) | No — ninguna disponible para la Fase 1 |
| 2 | `inc-21/02-kickoff-seal` | `57fcf2e1` | 328 + 4 = **332** | ✅ Dentro | No necesaria |
| 3 | `inc-21/03-kickoff-retro-seal` | `ad3f6bf2` | 265 + 4 = **269** | ✅ Dentro | No necesaria |
| 4 | `inc-21/04-kickoff-ledger-digest` | `10f723d6` | 372 + 6 = **378** | ✅ Dentro | Evaluada y descartada (378 < 400; no hacía falta separar `04b-kickoff-digest`) |
| **Total (Fases 1-4)** | — | — | **1499 + 19 = 1518** | (presupuesto de intento: 2600) | — |

## Remaining Tasks

- [ ] Fases 5 a 23 completas (dominio de la máquina de compuertas con reapertura, superficie CLI, proyección `sddstatus`, roster multi-rol, relevo de integración, precondición de archive, doctrina y activos por agente). Sin empezar en esta ejecución, tal como se instruyó explícitamente.

## Workload / PR Boundary

- **Modo**: PRs encadenadas (`feature-branch-chain`) contra la rama tracker `feature/inc-21-upfront-flow-governance`.
- **Unidad de trabajo actual**: PR 1-4, dominio puro `internal/kickoff` (tipos, esquema, sellado, retro-sellado, ledger, digest).
- **Frontera de este lote**: empieza en `d1200ddf` (commit de planificación de INC-21) y termina en `10f723d6` (cierre de la Fase 4). Ninguna rama hija ni PR real se creó — son fronteras documentales para cuando el usuario decida crearlas.
- **Impacto estimado en presupuesto de revisión**: 1518 líneas cambiadas repartidas en 4 unidades de trabajo autónomas y verificables por separado; una (PR 1) por encima del presupuesto individual de 400 sin válvula declarada (ver Issues Found).

## Status

19/19 tareas de las Fases 1-4 completas. **Ready for next batch** (Fase 5 en adelante) — no listo para archive todavía (19/114 tareas totales de `tasks.md` completas, 95 restantes).
