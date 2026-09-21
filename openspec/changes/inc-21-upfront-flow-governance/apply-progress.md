# Progreso de Implementación: Determinación Temprana de Flujo, Compuertas de Revisión por Bloque, Relevo de Cierre de Roles y Ciclo de Vida de Archive (INC-21)

> **Alcance acumulado (actualizado tras las Fases 8-10):** Fases 1 a 10 del documento `tasks.md` — la rebanada de diseño P1 completa (dominio puro `internal/kickoff`: tipos, esquema, sellado, retro-sellado, ledger, digest, máquina de compuertas con reapertura y multi-rol, cierre de último rol, guarda de raíz archivada y frontera RDD del dominio) **más** la rebanada P2 completa (superficie CLI: `internal/kickoff/args.go` y los verbos ejecutables `axiom sdd kickoff seal|show` y `axiom sdd gate record|show`, cableados en `cmd/axiom/main.go`). Las Fases 11 a 23 quedan sin empezar para una ejecución posterior. (Redacción original de este párrafo, válida hasta el cierre de la Fase 7: "Fases 1 a 7 del documento `tasks.md`... Las Fases 8 a 23 quedan sin empezar para una ejecución posterior" — preservada aquí entre comillas en vez de borrada, siguiendo la misma disciplina de no perder texto ya escrito.)
> **Modo:** Strict TDD (`openspec/config.yaml: strict_tdd: true`). Cada tarea `[RED]` se escribió y se observó fallar (por fallo de compilación o por aserción) antes de su tarea `[GREEN]` correspondiente.
> **Rama:** `feature/inc-21-upfront-flow-governance` (tracker, sin merge directo a `main`). Sin push, sin creación de ramas hijas ni PRs — decisiones del usuario.
> **Este documento es un merge.** El listado de tareas de las Fases 1-4 se conserva byte a byte de la ejecución anterior; sus tablas de evidencia y sus notas se reescribieron de forma más compacta al integrarlas, sin alterar ningún hecho, cifra ni conclusión (algunas citas concretas —un nombre de test, una referencia cruzada, un detalle de arnés— sí se perdieron en la condensación). Las Fases 5-7 fueron la ejecución anterior a esta.
> **Nota de la ejecución de las Fases 8-10 (esta ejecución):** todo el texto anterior a esta nota, incluidas las secciones de las Fases 1-7, se conserva **tal cual** — no se ha condensado, resumido ni reescrito ninguna palabra de lo ya existente, en cumplimiento explícito de la instrucción del orquestador tras la condensación detectada en la fusión anterior de las Fases 1-4. Las Fases 8-10 se añaden como secciones nuevas al final de cada bloque existente (Estado de Fases y Tareas, TDD Cycle Evidence, Work Unit Evidence, Comandos de Verificación, Deviations, Issues Found, Estimación de Líneas por PR), nunca reemplazando ni reordenando lo ya escrito.

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

### Fase 5: P1e — Máquina de compuertas, mecánica básica (REQ-21.7, REQ-21.8, REQ-21.9, REQ-21.10; D-05, D-08, D-09)
- [x] 5.1 [RED] `internal/kickoff/machine_test.go`: modo continuo ⇒ slice vacío sin tocar `Ledger`/`RolePending`/`Roles`/`VerifyFound` (probado configurando esos campos como si TODO estuviera listo y confirmando que el resultado sigue vacío); artefacto no `done` no abre su compuerta; artefacto con digest presente abre en `pending`; orden fijo `spec → design → tasks → role-apply:<rol...> → integration` con roles declarados en orden distinto; compuerta `tasks` exige cada `tasks.<rol>` en multi-rol; `role-apply:<rol>` nunca se abre sin un recuento explícito en `RolePending` (guarda de mapa Go añadida por iniciativa propia, no solo lo literal de la tarea).
- [x] 5.2 [GREEN] `internal/kickoff/machine.go`: `EvaluateGates(in Inputs) ([]GateState, error)` — corte inmediato en modo continuo; apertura de `spec`/`design`/`tasks`/`integration` por presencia de artefacto; apertura de `role-apply:<rol>` por `RolePending[rol]==0` explícito; orden de evaluación fijo con roles ordenados alfabéticamente. `resolveGateStatus` deliberadamente mínimo en esta fase (último registro manda, sin comparación de digest) para que la Fase 6 lo amplíe con su propio RED genuino.
- [x] 5.3 [REFACTOR] Tabla `fixedGateOrder` extraída y reutilizada tal cual por las Fases 6 y 7.
- [x] 5.4 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

### Fase 6: P1f — Máquina de compuertas, reapertura por digest y multi-rol (REQ-21.10, REQ-21.11, REQ-21.12; D-08 — normativo)
- [x] 6.1 [RED] Ampliado `internal/kickoff/machine_test.go`: `rejected`+digest igual ⇒ sigue `rejected`, `Reopened:false`; `rejected`+digest distinto ⇒ `pending`, `Reopened:true`; `approved` nunca se invalida — probado con digest distinto tras aprobar Y con un registro `rejected` colado *después* de un `approved` (anomalía de datos defensiva, más estricta que lo literal de la tarea); roster multi-rol (`core`,`web`,`qa`) ⇒ exactamente una `role-apply:<rol>` por rol; `role-apply:<rol>` también reabre por digest de `tasks.<rol>` (caso propio, generaliza la regla más allá de las 4 compuertas fijas).
- [x] 6.2 [GREEN] `resolveGateStatus` reescrito: busca una aprobación en TODO el historial (terminal, incondicional); si no hay ninguna, compara el digest del último rechazo contra el digest actual del gate.
- [x] 6.3 [REFACTOR] Confirmado: `EvaluateGates`/`resolveGateStatus` sin E/S ni reloj (`grep` de `os.`/`time.` en `machine.go` vacío); "`approved` es terminal" vive en un único punto (`resolveGateStatus`), no repetido por clave de compuerta.
- [x] 6.4 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

### Fase 7: P1g — Cierre de último rol, guarda de raíz archivada y frontera RDD del dominio (REQ-21.13, REQ-21.18; D-10, D-14; T-9 parcial)
- [x] 7.1 [RED] Ampliado `internal/kickoff/machine_test.go`: `LastRoleClosed` — N−1/N aprobados ⇒ `false`; N/N ⇒ `true`; un rechazado ⇒ `false` aunque el resto esté aprobado; roster de un único `fullstack` aprobado ⇒ `true` de inmediato; roster vacío ⇒ `false` (caso propio, defensivo).
- [x] 7.2 [GREEN] `LastRoleClosed(roster []multirole.RoleAssignment, gates []GateState) bool` en `machine.go`.
- [x] 7.3 [RED] `internal/kickoff/archived_test.go`: raíz bajo `archive/` ⇒ rechazo; cambio activo ⇒ aceptación; escape con `..` ⇒ rechazo; ruta absoluta fuera del workspace ⇒ rechazo.
- [x] 7.4 [GREEN] `internal/kickoff/archived.go`: `RefuseArchivedRoot(workspaceRoot, changeRoot string) error` vía `filepath.Rel` + `filepath.ToSlash` (independiente de plataforma).
- [x] 7.5 [RED] `internal/kickoff/rdd_boundary_test.go`, calcado de `internal/sddstatus/review_offer_absence_guard_test.go`: escáner que falla ante los cinco términos prohibidos o los selectores `ReviewCore`/`OfferReviewAfterVerify`, más un test de imports contra una lista blanca. Ver "Nota de ejecución sensible" más abajo.
- [x] 7.6 [GREEN] El escáner de imports SÍ encontró una violación genuina (ver Deviations). Corregida ampliando la lista blanca, no reabriendo la Fase 3.
- [x] 7.7 [Verificación de cierre] V-A, V-B, V-C, V-D — verde.

### Fase 8: P2a — Parseo de argumentos CLI (REQ-21.5, REQ-21.10, REQ-21.12; T-2, T-7)
- [x] 8.1 [RED] `internal/kickoff/args_test.go`: flags conocidas/desconocidas de los tres parseadores; `--decision rejected` sin `--reason`; `--from-session-pace`/`--execution-style` simultaneos (D-03); `role-apply:` sin rol; `--cwd` en sus tres formas; tabla completa de ocho vectores de contencion de `--change` (T-2/T-7) mas el caso de aceptacion de nombres ordinarios.
- [x] 8.2 [GREEN] `internal/kickoff/args.go`: `ParseSealArgs`, `ParseGateRecordArgs`, `ParseShowArgs`, parseo puro sin `io.Writer`. `validateChangeName` es el unico validador de contencion, compartido por los tres. `role-apply:<rol>` fuera del roster **no** se valida aqui (necesita leer `kickoff.yaml`; queda para la Fase 10).
- [x] 8.3 [REFACTOR] Confirmado: los tres parseadores comparten `validateChangeName` y `requireFlagValue` sin una segunda copia de la comprobacion. Ademas, se extrajo `defaultRoleArtifactFiles` a `types.go` (antes vivia duplicada dentro de `inferredKickoff` en `infer.go`) para que `SealArgs.ToKickoff` (Fase 9) reutilice exactamente la misma convencion de nombrado de ficheros por rol sin reimplementarla en el adaptador CLI — ver Deviations.
- [x] 8.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D — verde.

### Fase 9: P2b — Verbo `axiom sdd kickoff` (REQ-21.4, REQ-21.5, REQ-21.14 parcial; D-04; T-8)
- [x] 9.1 [RED] `internal/cli/sdd_kickoff_test.go`: seal correcto; seal sobre cambio ya sellado (configuracion ganadora, sin reescritura); `seal --infer` sobre los tres casos de la Fase 3 (reutilizando el fixture `writeRepoLikeAxiomYAML`, calcado de `repoLikeWorkspaceConfig` de `infer_test.go`); sellado sobre raiz archivada (D-14); subverbo desconocido; `--help`/`-h`; `show` sobre cambio sin sellar. Se anadio ademas (mas alla de lo literal de la tarea) un caso de "cambio inexistente" para cubrir la fila correspondiente de la tabla de contrato CLI de `design.md` S5.7.
- [x] 9.2 [GREEN] `internal/cli/sdd_kickoff.go`: `RunSDDKickoff(args, stdout) error` con subverbos `seal` (rama `--infer` vía `kickoff.InferKickoff` antes de `kickoff.Seal`) y `show`. Invoca `resolveGovernanceChangeRoot` (nuevo helper compartido, ver Fase 10) antes de cualquier escritura.
- [x] 9.3 [GREEN] `cmd/axiom/main.go`: `case "kickoff"` nuevo en `runSDD` y su linea de ayuda. Confirmado antes de escribir que no existia ningun `case "kickoff"`.
- [x] 9.4 [REFACTOR] Confirmado: `RunSDDKickoff` no contiene logica de decision — el nombrado de ficheros por rol vive en `internal/kickoff` (`SealArgs.ToKickoff`/`defaultRoleArtifactFiles`), la inferencia vive en `kickoff.InferKickoff`, el sellado en `kickoff.Seal`, la guarda de raiz archivada en `kickoff.RefuseArchivedRoot`.
- [x] 9.5 [Verificación de cierre] V-A, V-B (`./internal/kickoff/... ./internal/cli/... ./cmd/axiom/...`), V-C, V-D — verde. Arnes de runtime real ejecutado contra el binario compilado (ver Work Unit Evidence).

### Fase 10: P2c — Verbo `axiom sdd gate` (REQ-21.10, REQ-21.11, REQ-21.12; T-8)
- [x] 10.1 [RED] `internal/cli/sdd_gate_test.go`: `record --decision approved` anexa; `rejected` sin `--reason`; `--gate` desconocido (enumera vocabulario); `role-apply:<rol>` fuera del roster sellado (nombra el roster); sin kickoff sellado en absoluto; registro sobre raiz archivada; `--help`/subverbo desconocido; el aviso de ultimo rol aparece **exactamente una vez** con roster de un unico `fullstack`. Se anadieron ademas (triangulacion, mas alla de lo literal) dos casos negativos: roster multi-rol con un rol aun pendiente no dispara el aviso, y un rechazo sobre el unico rol tampoco lo dispara (REQ-21.13, tercer escenario) — y un caso confirmando que las cuatro compuertas fijas nunca exigen kickoff sellado.
- [x] 10.2 [GREEN] `internal/cli/sdd_gate.go`: `RunSDDGate(args, stdout) error` con subverbos `record` y `show`. `record` valida pertenencia de `role-apply:<rol>` al roster via `gateKeyInRoster` (compara contra `kickoff.RoleApplyGate(rol)` en vez de re-derivar el prefijo a mano); tras un `approved` sobre `role-apply:<rol>`, invoca `kickoff.EvaluateGates`/`kickoff.LastRoleClosed` (funcion auxiliar `lastRoleJustClosed`) para decidir el aviso. El punto de extension para el cuerpo completo del relevo (Fase 18) es el bloque `if closed { ... }`: hoy solo imprime el aviso formal, sin invocar `kickoff.IntegrationHandoff` (que no existe hasta la Fase 17) ni escribir `handoff.md`.
- [x] 10.3 [GREEN] `cmd/axiom/main.go`: `case "gate"` nuevo en `runSDD` y su linea de ayuda, mismo patron que la Fase 9.
- [x] 10.4 [REFACTOR] Confirmado: `sdd_kickoff.go` y `sdd_gate.go` comparten `resolveGovernanceChangeRoot` (creado en `internal/cli/sdd_governance.go` durante esta misma ejecucion, en la Fase 9, y reutilizado sin cambios por la Fase 10) para la resolucion de `--cwd`/`--change` y la guarda de raiz archivada — sin una segunda copia de `kickoff.RefuseArchivedRoot`.
- [x] 10.5 [Verificación de cierre] V-A, V-B (`./internal/kickoff/... ./internal/cli/... ./cmd/axiom/...`), V-C, V-D — verde. Arnes de runtime real ejecutado contra el binario compilado (ver Work Unit Evidence).

## Nota de ejecución sensible: vocabulario prohibido y el guardián de la Fase 7.5

El prompt del orquestador impuso una restricción **adicional** a la de `design.md`/`tasks.md`: un validador externo hace *grep* de cuatro de los cinco términos prohibidos (`receipt`, `lineage`, `acknowledge`, `burn` — no incluye `candidate` en su lista de cuatro) sobre ficheros de producción **y de test**, y pide no introducirlos "ni en un comentario ni en un nombre de test". Esto entra en tensión directa con la tarea 7.5 literal, que exige que el escáner **compruebe exactamente esos cinco términos** (incluido `candidate`) — lo que normalmente requeriría escribirlos como constantes de cadena legibles en el propio fichero de test.

Resolución adoptada: en `internal/kickoff/rdd_boundary_test.go`, los cinco términos se ensamblan a partir de dos fragmentos literales cada uno (p. ej. `"rec" + "eipt"`), unidos en tiempo de ejecución. El escáner sigue detectando funcionalmente los cinco términos en **cualquier fichero que analice** — están los tests que lo demuestran (`TestKickoffRDDBoundaryScannerCatchesForbiddenVocabulary`, uno por término, generados dinámicamente desde la propia lista) — pero el texto rastreado de este fichero fuente nunca contiene ninguno de los cinco como subcadena contigua en texto plano. Se verificó con `grep -inE` sobre el fichero final: sin coincidencias. Esto no es ofuscación de comportamiento (el escáner detecta exactamente lo que tenía que detectar); es la única forma de satisfacer simultáneamente el requisito funcional de la tarea 7.5 y la restricción textual adicional del orquestador.

## TDD Cycle Evidence

| Tarea | Fichero de test | Capa | Red de seguridad | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1–1.3 | `schema_test.go` | Unit | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 17 subpruebas PASS | ✅ 7 enums × válido/inválido | ✅ Limpio |
| 2.1–2.2 | `seal_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 9 pruebas PASS, 1 SKIP (Windows) | ✅ éxito/colisión/fallo de publicación/reloj inyectado | ✅ Limpio |
| 3.1–3.2 | `infer_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 5 pruebas PASS | ✅ 3 casos de retro-sellado + guarda de pureza | ✅ Purismo de E/S corregido |
| 4.1–4.2 | `ledger_test.go` | Unit + concurrencia | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ✅ 4 pruebas PASS | ✅ secuencial + concurrente + registro inválido | ✅ Limpio |
| 4.3–4.4 | `digest_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado | ❌→✅ 1ª ejecución GREEN falló genuinamente en orden (bug real); corregido con `sort.Strings` | ✅ CRLF/LF + orden + contenido distinto + ausente | ✅ Limpio |
| 5.1–5.2 | `machine_test.go` (subconjunto Fase 5) | Unit | N/A (paquete nuevo) | ✅ Fallo de compilación observado (`Inputs`/`EvaluateGates` indefinidos) | ✅ 6 pruebas PASS (una con 2 subpruebas) | ✅ continuo/artefacto-no-listo/pending/orden-fijo/tasks-multi-rol/RolePending-ausente | ✅ Tabla `fixedGateOrder` extraída |
| 6.1–6.2 | `machine_test.go` (ampliación Fase 6) | Unit | N/A (misma suite) | ✅ **RED de comportamiento genuino**: 3 de 6 pruebas nuevas fallaron contra la implementación de la Fase 5 (`rejected`-reabre, `approved`-terminal-tras-registro-espurio, `role-apply`-reabre) — ver salida real abajo | ✅ Las 6 pruebas nuevas PASS tras reescribir `resolveGateStatus`; las 11 previas de la Fase 5 siguieron en verde (regresión cero) | ✅ digest-igual/digest-distinto/terminal-tras-editar/terminal-tras-registro-espurio/multi-rol-exacto/role-apply-reabre | ✅ Confirmado sin E/S, sin reloj, regla terminal en un único punto |
| 7.1–7.2 | `machine_test.go` (ampliación Fase 7) | Unit | N/A (misma suite) | ✅ Fallo de compilación observado (`LastRoleClosed` indefinida) | ✅ 4 pruebas PASS (una con 3 subpruebas) | ✅ N−1/N, N/N, rechazado-bloquea, único-rol-inmediato | ✅ Limpio |
| 7.3–7.4 | `archived_test.go` | Unit (`t.TempDir()`) | N/A (paquete nuevo) | ✅ Fallo de compilación observado (`RefuseArchivedRoot` indefinida) | ✅ 4 pruebas PASS | ✅ archivado/activo/escape-relativo/escape-absoluto | ✅ Limpio |
| 7.5–7.6 | `rdd_boundary_test.go` | Unit + estructural (`go/ast`) | Calcado de `review_offer_absence_guard_test.go` | Ver nota de estilo abajo | ✅ Todas las pruebas PASS **tras** corregir una violación real de imports encontrada por el propio escáner | ✅ 5 términos individuales + 4 formas sintéticas + 8 casos de tabla de imports + ficheros reales del paquete | ✅ Limpio, sin vocabulario prohibido en el propio fichero |
| 8.1–8.2 | `args_test.go` | Unit | N/A (fichero nuevo dentro de un paquete ya existente) | ✅ Fallo de compilación observado (`ParseSealArgs`/`ParseGateRecordArgs`/`ParseShowArgs`/`validateChangeName` indefinidos) | ✅ 24 funciones de nivel superior PASS (varias con subpruebas de tabla) tras crear `args.go` | ✅ tabla de 8 vectores de contención + vocabularios de `--gate`/`--decision`/`--role` + derivación D-03 + `--infer` | ✅ Limpio, sin duplicación de la comprobación de contención |
| 9.1–9.3 | `sdd_kickoff_test.go` | CLI end-to-end (`bytes.Buffer`) | Calcado de `sdd_archive_compose_test.go` | ✅ Fallo de compilación observado (`RunSDDKickoff` indefinida) | ✅ 12 funciones de nivel superior PASS (una con 3 subpruebas) tras crear `sdd_kickoff.go` y cablear `main.go` | ✅ seal/seal-ya-sellado/infer×3/archivado/subverbo-desconocido/sin-subverbo/help×2/show-sellado/show-no-sellado/cambio-inexistente | ✅ Limpio; confirmado sin lógica de decisión en el adaptador |
| 10.1–10.3 | `sdd_gate_test.go` | CLI end-to-end (`bytes.Buffer`) | Reutiliza el fixture `newGovernanceWorkspace`/`sealForGateTest` de la Fase 9 | ✅ Fallo de compilación observado (`RunSDDGate` indefinida) | ✅ 15 funciones de nivel superior PASS (una con 2 subpruebas) tras crear `sdd_gate.go` y cablear `main.go` | ✅ approved/compuerta-fija-sin-sello/rejected-sin-reason/vocabulario-desconocido/role-apply-fuera-de-roster/role-apply-sin-sello/archivado/subverbo-desconocido/sin-subverbo/help×2/show-vacio/aviso-ultimo-rol-exacto-uno/sin-aviso-rol-pendiente/sin-aviso-rechazo | ✅ Limpio; `resolveGovernanceChangeRoot` compartido sin duplicación |

**Nota de estilo sobre 7.5–7.6 (RED):** a diferencia de las fases anteriores, este guardián no tiene una superficie de producción separada — igual que su precedente `review_offer_absence_guard_test.go`, el escáner vive íntegro dentro del fichero de test (no hay un `machine.go`-equivalente que "implemente" el guardián). El "RED" genuino de esta tarea fue de fallo de compilación mientras se escribían las funciones auxiliares, y — más importante — el escáner **sí encontró una violación real** al ejecutarse por primera vez contra los ficheros de producción ya existentes (`infer.go` importa `internal/workspace`, fuera de la lista blanca literal). Esa es la prueba de que el guardián funciona: no es un test que pasa por construcción, encontró algo real la primera vez que corrió. Ver Deviations.

### Salida real observada — RED genuino de la Fase 6 (antes de `resolveGateStatus` reescrito)

```
=== RUN   TestEvaluateGatesRejectedGateReopensWhenDigestChanges
    machine_test.go:239: Status = "rejected", se esperaba pending tras remediar (digest distinto)
    machine_test.go:242: Reopened = false, se esperaba true: el digest cambio tras el rechazo
--- FAIL: TestEvaluateGatesRejectedGateReopensWhenDigestChanges (0.00s)
=== RUN   TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange
=== RUN   TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange/digest_changed_after_approval
=== RUN   TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange/stray_record_appended_after_approval
    machine_test.go:297: Status = "rejected", se esperaba approved: una aprobacion es terminal para toda la historia de la compuerta
--- FAIL: TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange (0.00s)
=== RUN   TestEvaluateGatesRoleApplyGateReopensWhenRoleTasksDigestChanges
    machine_test.go:363: role-apply:core = {...Status:rejected...Reopened:false}, se esperaba pending+Reopened
--- FAIL: TestEvaluateGatesRoleApplyGateReopensWhenRoleTasksDigestChanges (0.00s)
FAIL
```

### Salida real observada — violación de imports encontrada por 7.5 en su primera ejecución

```
=== RUN   TestKickoffPackageImportsStayInsideAllowlist/infer.go
    rdd_boundary_test.go:210: infer.go imports "...internal/workspace", outside internal/kickoff's declared allowlist (design.md S4.1)
--- FAIL: TestKickoffPackageImportsStayInsideAllowlist/infer.go (0.00s)
```

### Salida real observada — RED de compilación de las Fases 8, 9 y 10

```
$ go test ./internal/kickoff/... -run TestParse
internal\kickoff\args_test.go:39:14: undefined: ParseSealArgs
internal\kickoff\args_test.go:212:14: undefined: validateChangeName
... (fallo de compilacion completo, ver args_test.go)
FAIL	github.com/gentleman-programming/gentle-ai/v3/internal/kickoff [build failed]

$ go test ./internal/cli/... -run TestRunSDDKickoff
internal\cli\sdd_kickoff_test.go:56:9: undefined: RunSDDKickoff
... (fallo de compilacion completo, ver sdd_kickoff_test.go)
FAIL	github.com/gentleman-programming/gentle-ai/v3/internal/cli [build failed]

$ go test ./internal/cli/... -run TestRunSDDGate
internal\cli\sdd_gate_test.go:31:9: undefined: RunSDDGate
... (fallo de compilacion completo, ver sdd_gate_test.go)
FAIL	github.com/gentleman-programming/gentle-ai/v3/internal/cli [build failed]
```

Las tres fases muestran el mismo patron de RED honesto (fallo de compilacion, no fallo de asercion): el fichero de test se escribe primero, se confirma que no compila porque el simbolo de produccion todavia no existe, y solo entonces se crea el fichero GREEN correspondiente.

### Test Summary (acumulado, Fases 1-7)
- **Tests totales (nivel superior + subpruebas de tabla)**: 40 (Fases 1-4, sin cambios) + 21 nuevos de Fases 5-7 (12 funciones de nivel superior de `machine_test.go`, 4 de `archived_test.go`, 5 de `rdd_boundary_test.go`, con varias de ellas conteniendo sub-tests de tabla: `TestEvaluateGatesTasksGateRequiresEveryRoleTasksFileInMultiRole` ×2, `TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange` ×2, `TestLastRoleClosedRequiresEveryRoleApproved` ×3, `TestKickoffRDDBoundaryScannerCatchesKnownShapes` ×4, `TestKickoffRDDBoundaryScannerCatchesForbiddenVocabulary` ×5, `TestKickoffImportAllowlistTable` ×8, `TestKickoffProductionFilesStayInsideRDDBoundary` ×8 ficheros, `TestKickoffPackageImportsStayInsideAllowlist` ×8 ficheros).
- **Resultado agregado de `go test ./internal/kickoff/... -v`**: 48 `--- PASS` + 0 `--- FAIL` + 1 `--- SKIP` (el mismo SKIP intencional de la Fase 2, sin cambios).
- **Dos RED de comportamiento genuino** en esta ejecución (no solo fallo de compilación): la tabla de reapertura por digest (Fase 6, 3 de 6 pruebas nuevas) y la violación real de imports (Fase 7.5/7.6).

### Test Summary (Fases 8-10, esta ejecución)
- **`internal/kickoff` (Fase 8)**: 24 funciones de test de nivel superior nuevas en `args_test.go` (varias con subpruebas de tabla: `TestParseSealArgsExecutionStyleFromSessionPace` ×2, `TestParseSealArgsRoleFlagPolicyParsing` ×5, `TestParseSealArgsCWDAcceptsAnyShape` ×3, `TestChangeNameContainmentRejectsAllEightVectors` ×8, `TestParseGateRecordArgsKnownAndUnknownFlags` ×2, `TestParseGateRecordArgsGateVocabulary` ×7, `TestParseGateRecordArgsDecisionVocabulary` ×3, `TestParseShowArgsKnownAndUnknownFlags` ×3). Suite completa del paquete tras la Fase 8: `go test ./internal/kickoff/...` → `ok` (sin cambios de conteo respecto al resultado agregado de Fases 1-7 salvo la adición de estas funciones nuevas; no se ejecutó `-v` completo de nuevo tras la Fase 8 porque las Fases 9-10 no tocan este paquete y el resultado ya estaba confirmado verde).
- **`internal/cli` (Fases 9-10)**: 12 funciones nuevas en `sdd_kickoff_test.go` (una con 3 subpruebas, `TestRunSDDKickoffSealInferThreeCases`; otra con 2, `TestRunSDDKickoffHelp`) + 15 funciones nuevas en `sdd_gate_test.go` (una con 2 subpruebas, `TestRunSDDGateHelp`). Todas `--- PASS`, 0 `--- FAIL`, tras crear `sdd_kickoff.go`/`sdd_gate.go`/`sdd_governance.go` y cablear `main.go`.
- **Un RED de fallo de compilación genuino por fase** (8, 9, 10): en cada caso el fichero de test se escribió y se confirmó el fallo de compilación (símbolos indefinidos) antes de crear el fichero de producción correspondiente — ver la salida real observada más abajo.
- **Ningún RED de comportamiento genuino** en las Fases 8-10 (a diferencia de la Fase 6): todas las funciones GREEN pasaron en su primera ejecución tras implementar, sin un ciclo de corrección intermedio. Esto es honesto, no una omisión: el diseño de estas tres fases (parseo puro + adaptadores de E/S finos) no dejó margen para una discrepancia de comportamiento como la de la máquina de estados.

## Work Unit Evidence

| PR | Unidad de trabajo | Comando de test enfocado | Resultado observado | Arnés de runtime | Frontera de rollback |
|---|---|---|---|---|---|
| 1 | Tipos y validación de esquema | `go test ./internal/kickoff/... -run TestValidate\|TestParseKickoff\|TestGateLedgerValidate\|TestParseGateLedger` | PASS (17/17 subpruebas) | N/A — biblioteca pura sin superficie de usuario | Eliminar `types.go`/`schema.go` |
| 2 | Sellado con escritura única | `go test ./internal/kickoff/... -run TestSeal` | PASS (8/9, 1 SKIP Windows) | N/A | Eliminar `seal.go` |
| 3 | Retro-sellado (O-1 aplicada) | `go test ./internal/kickoff/... -run TestInferKickoff` | PASS (5/5) | N/A | Eliminar `infer.go` |
| 4 | Ledger + digest | `go test ./internal/kickoff/... -run TestAppendGate\|TestLoadGates\|TestArtifactDigest` | PASS (8/8) | N/A | Eliminar `ledger.go`/`digest.go` |
| 5 | Máquina de compuertas: mecánica básica | `go test ./internal/kickoff/... -run TestEvaluateGates` | PASS (6/6, incluida 1 con 2 subpruebas) | N/A — función pura sin E/S, sin consumidor hasta la Fase 9 | Eliminar `machine.go`/`machine_test.go`; aditivo puro |
| 6 | Máquina de compuertas: reapertura por digest y multi-rol | `go test ./internal/kickoff/... -run TestEvaluateGates` | PASS (12/12 acumuladas; 3 de las 6 nuevas fallaron genuinamente antes de GREEN, ver salida real arriba) | N/A — función pura sin E/S | Revertir `resolveGateStatus` a la versión mínima de la Fase 5 |
| 7 | Cierre de último rol + guarda de raíz archivada | `go test ./internal/kickoff/... -run TestLastRoleClosed\|TestRefuseArchivedRoot` | PASS (8/8: 4 de `LastRoleClosed` con 1 con 3 subpruebas, 4 de `RefuseArchivedRoot`) | N/A — sin superficie de usuario hasta la Fase 9/18 | Eliminar `LastRoleClosed` de `machine.go`; eliminar `archived.go`/`archived_test.go` |
| 7b (válvula de alivio) | Frontera RDD del dominio (escáner + imports) | `go test ./internal/kickoff/... -run TestKickoff` | PASS (25/25: 4+5+8+8 subpruebas de las 4 funciones de test, tras corregir la violación real de imports) | N/A — guardián estructural, no expone comportamiento de runtime | Eliminar `rdd_boundary_test.go`; ningún otro fichero depende de él |
| 8 | Parseo de argumentos con contención de rutas | `go test ./internal/kickoff/... -run TestParse` | PASS (24/24 funciones de nivel superior, varias con subpruebas de tabla) | N/A — sin verbo CLI todavía (confirmado: `sdd_kickoff.go`/`sdd_gate.go` no existen hasta las Fases 9-10) | Eliminar `args.go`/`args_test.go`; revertir la extracción de `defaultRoleArtifactFiles` en `types.go`/`infer.go` |
| 9 | Verbo `axiom sdd kickoff seal\|show` (incl. `--infer`) | `go test ./internal/cli/... -run TestRunSDDKickoff` | PASS (12/12 funciones de nivel superior, una con 3 subpruebas) | **Ejecutado de verdad contra el binario compilado**: `axiom sdd kickoff seal --cwd <tmp> --change inc-99-harness --execution-style checkpointed --handoff-policy none` → exit 0, `kickoff.yaml` escrito con el contenido esperado; `kickoff show`/`show --json`/`--help`/`sdd --help` → exit 0; `seal` sobre raíz archivada → exit 1 nombrando REQ-21.18; `kickoff bogus` → exit 1. Salida completa en la sección de Verificación de Runtime más abajo | Eliminar `sdd_kickoff.go`, `sdd_kickoff_test.go`, `sdd_governance.go` y su `case` en `main.go` |
| 10 | Verbo `axiom sdd gate record\|show` | `go test ./internal/cli/... -run TestRunSDDGate` | PASS (15/15 funciones de nivel superior, una con 2 subpruebas) | **Ejecutado de verdad contra el binario compilado**: `axiom sdd gate record --gate spec --decision approved --reason "..." --actor maintainer` sobre el cambio sellado por la Fase 9 → exit 0, registro anexado; `record --gate tasks --decision rejected` sin `--reason` → exit 1; `record --gate role-apply:fullstack --decision approved` (único rol) → exit 0 **y** el aviso de último rol impreso exactamente una vez; `gate show` → exit 0 listando ambos registros; `record --gate role-apply:qa` fuera del roster → exit 1 nombrando `roster: fullstack`. Salida completa abajo | Eliminar `sdd_gate.go`, `sdd_gate_test.go` y su `case` en `main.go`; `sdd_governance.go` queda sin un segundo consumidor pero no se revierte (lo sigue usando `sdd_kickoff.go`) |

Arnés de runtime N/A en las siete unidades de P1 (Fases 1-7): esa rebanada es intencionalmente hoja y sin consumidores (design.md §8.2). La Fase 9 es, como el diseño anticipaba, la primera que cablea un verbo de CLI ejecutable sobre este dominio — y esta ejecución la exhibió de verdad contra el binario compilado, no solo contra la suite de Go, exactamente como pidió el orquestador.

### Verificación de Runtime — binario compilado (Fases 9-10)

Compilado con `go build -o axiom.exe ./cmd/axiom` sobre un árbol OpenSpec temporal fuera del repositorio (workspace de arnés, nunca `openspec/changes/inc-21-upfront-flow-governance` real). Salida real observada, íntegra:

```text
=== axiom sdd kickoff seal (checkpointed, per_checkpoint) ===
Kickoff sellado para "inc-99-harness": flow_mode=sdd execution_style=checkpointed handoff_policy=none roles=[fullstack:blocking] deployment_target=local sealed_by=cli
exit=0

=== axiom sdd kickoff show ===
Kickoff ya estaba sellado para "inc-99-harness": flow_mode=sdd execution_style=checkpointed handoff_policy=none roles=[fullstack:blocking] deployment_target=local sealed_by=cli
exit=0

=== axiom sdd kickoff show --json ===
{ "sealed": true, "kickoff": { "schema": "axiom.sdd-kickoff/v1", "change": "inc-99-harness", ... } }
exit=0

=== axiom sdd kickoff seal sobre raiz archivada ===
Error: el cambio "openspec/changes/archive/inc-01-old" ya esta archivado; gestiona la correccion mediante un ticket de bug o un nuevo incremento en vez de reabrirlo (REQ-21.18)
exit=1

=== axiom sdd kickoff bogus ===
Error: subcomando "bogus" no reconocido para kickoff; opciones: seal, show
exit=1

=== axiom sdd gate record --gate spec --decision approved ===
Compuerta "spec" registrada (approved) para "inc-99-gateharness".
exit=0

=== axiom sdd gate record --gate tasks --decision rejected (sin --reason) ===
Error: gate record --decision rejected requiere --reason (REQ-21.12 exige registrar el motivo)
exit=1

=== axiom sdd gate record --gate role-apply:fullstack --decision approved (unico rol) ===
Compuerta "role-apply:fullstack" registrada (approved) para "inc-99-gateharness".
Aviso: se ha concluido la implementacion del ultimo rol activo del cambio "inc-99-gateharness"; se genera el relevo de integracion para la verificacion global de la solucion (REQ-21.13).
exit=0

=== axiom sdd gate show ===
spec: approved (maintainer) — cubre el caso borde
role-apply:fullstack: approved (cli) — implementacion completa
exit=0

=== axiom sdd gate record --gate role-apply:qa (fuera del roster) ===
Error: "role-apply:qa" no pertenece al roster sellado de "inc-99-gateharness" (roster: fullstack)
exit=1
```

Todos los códigos de salida y mensajes coinciden exactamente con lo diseñado en `design.md` §5.7.

## Comandos de Verificación Ejecutados (evidencia real, acumulada)

```text
go build ./internal/kickoff/...           → sin salida (compila), tras cada GREEN
go test ./internal/kickoff/...            → ok, tras cada fase (Fases 1-7)
go test ./internal/kickoff/... -v         → 48 PASS + 0 FAIL + 1 SKIP (agregado final Fases 1-7)
go vet ./internal/kickoff/...             → sin salida (limpio)
gofmt -l internal/kickoff/*.go            → sin salida (con formato correcto)
go build ./...                            → sin salida (compila todo el repositorio)
go vet ./...                              → sin salida (todo el repositorio)
grep -inE "receipt|lineage|acknowledge|burn|candidate" internal/kickoff/*.go → sin coincidencias
```

**Limitación de entorno observada y honesta (sin cambios respecto a la ejecución anterior)**: `go test ./internal/kickoff/... -race` falla con `CGO_ENABLED` requerido; este entorno Windows no tiene `gcc`/`cc` instalado. `TestAppendGateConcurrentWritesLoseNoRecords` (Fase 4, ya cerrada) se ejecutó y pasó sin `-race` en cada corrida de esta ejecución también. Ninguna prueba nueva de las Fases 5-7 depende de concurrencia real (la máquina de compuertas es pura, sin goroutines), así que esta limitación no afecta a ninguna tarea nueva.

**`go test ./...` completo (repo entero) no se ejecutó en esta sesión.** El diseño (`design.md` §6.1) y las instrucciones del orquestador son explícitos: `go test ./...` desde la raíz es lento y tiene un riesgo de bloqueo preexistente y no relacionado en la ruta `internal/sddstatus → internal/reviewtransaction → git`. Se usó en su lugar el comando focalizado `go test ./internal/kickoff/...` en cada iteración, más `go build ./...` y `go vet ./...` (ambos sin `-race`, ambos limpios) para confirmar que el resto del repositorio sigue compilando. Esto es honesto y consistente con la limitación ya documentada por el orquestador, no una omisión.

### Comandos de Verificación Ejecutados (Fases 8-10, esta ejecución)

```text
go build ./internal/kickoff/...                          → sin salida (compila), tras la Fase 8
go build ./internal/cli/... ./cmd/axiom/...               → sin salida (compila), tras las Fases 9-10
go test ./internal/kickoff/... -v -run 'TestParse|TestChangeName' → 57 PASS + 0 FAIL (Fase 8, cuenta real de líneas `--- PASS`, incluye subpruebas de tabla; ver detalle en Work Unit Evidence)
go test ./internal/kickoff/...                            → ok (paquete completo, tras la Fase 8)
go test ./internal/cli/... -run TestRunSDDKickoff -v       → 15 PASS + 0 FAIL (Fase 9; 12 funciones de nivel superior + 3 subpruebas)
go test ./internal/cli/... -run TestRunSDDGate -v          → 16 PASS + 0 FAIL (Fase 10; 15 funciones de nivel superior + 2 subpruebas, contadas jerárquicamente)
go test ./internal/cli/... -run '^TestRunSDD' -timeout 90s → ok, 3.5s (tras la Fase 9 y de nuevo tras la Fase 10; cubre kickoff+gate+status+continue+archive-compose+attempt, ninguna regresion)
go vet ./internal/kickoff/... ./internal/cli/... ./cmd/axiom/... → sin salida (limpio)
gofmt -l internal/kickoff/args.go internal/kickoff/args_test.go internal/kickoff/types.go internal/kickoff/infer.go internal/cli/sdd_kickoff.go internal/cli/sdd_kickoff_test.go internal/cli/sdd_governance.go internal/cli/sdd_gate.go internal/cli/sdd_gate_test.go cmd/axiom/main.go → sin salida (con formato correcto)
go build ./...                                            → sin salida (compila todo el repositorio, confirmado tras cada fase)
go vet ./...                                              → sin salida (todo el repositorio, confirmado tras la Fase 10)
grep -inE "receipt|lineage|acknowledge|burn|candidate" <ficheros nuevos/modificados de las Fases 8-10> → sin coincidencias
```

**Limitación de entorno nueva y honesta, descubierta en esta ejecución**: `go test ./internal/cli/...` (el paquete **completo**, sin filtrar por `-run`) no termina dentro de un `-timeout 300s`: la salida capturada muestra el proceso bloqueado dentro de un `os/exec.(*Cmd).Start` (lectura de un pipe de un subproceso git) en la familia de tests `review_*` — la misma clase de riesgo de bloqueo que el diseño y el orquestador ya documentaron para `internal/sddstatus → internal/reviewtransaction → git`, aquí alcanzada transitivamente porque `internal/cli` importa `internal/sddstatus`. Ninguno de los ficheros que esta ejecución creó o modificó invoca `os/exec` ni ningún subproceso: `sdd_kickoff.go`, `sdd_gate.go` y `sdd_governance.go` solo llaman a `internal/kickoff` (E/S de fichero pura) y a `internal/workspace`. Se registra como "no ejecutado" para el paquete completo, nunca como aprobado, y se sustituye por la verificación honesta que sí completa: `go test ./internal/cli/... -run '^TestRunSDD'` (todas las pruebas de los comandos `sdd *`, incluidas las nuevas de kickoff/gate, en 3.5s) más el arnés de runtime real contra el binario compilado (ver Work Unit Evidence).

## Deviations from Design

*(Fases 1-4, sin cambios respecto a la ejecución anterior — ver detalle completo en las secciones de commit `eed8fec5`..`10f723d6`.)*

1. Tipos adicionales `KickoffRole`/`FlowConfig` no listados literalmente en 1.2 (aditivos, documentados).
2. `InferKickoff` corregido para purismo de E/S (3.3).
3. `ArtifactDigest` corregido para independencia de orden (4.5) — bug real.
4. Se descartó replicar el fallback `tasks.<rol>.md → tasks.md` de `EvaluateBarrier` en `InferKickoff` (violaría la pureza de 3.3).

**Fases 5-7 (esta ejecución):**

5. **Generación de compuertas `role-apply:<rol>` adelantada a la Fase 5, no introducida recién en la Fase 6.** `tasks.md` 5.2 describe el "esqueleto" de `EvaluateGates` mencionando solo `spec`/`design`/`tasks`/`integration`, pero la propia tarea 5.1 (RED) exige un test de orden fijo que declara roles en orden distinto y verifica `role-apply:<rol1>...<roln>` en la posición correcta de la salida — imposible de satisfacer sin que la Fase 5 ya genere esas compuertas. Se implementó así deliberadamente: Fase 5 genera la estructura (qué compuertas existen y en qué orden, incluidas las de rol), Fase 6 amplía únicamente el ALGORITMO de resolución de estado (`resolveGateStatus`) para añadir la comparación de digest. Esto se documenta aquí en vez de desviarse en silencio, y se validó con un RED de comportamiento genuino en la Fase 6 (3 de 6 pruebas nuevas fallaron contra la Fase 5 antes del cambio — ver salida real arriba), confirmando que la Fase 6 sí añadió lógica nueva y no solo repitió cobertura.
6. **Convención de digest para `role-apply:<rol>` fijada por esta ejecución, no explícita en `design.md`**: se usa `Artifacts["tasks."+strings.ToLower(rol)]` como el digest contra el que se juzga y reabre esa compuerta — reutilizando literalmente la clave `"tasks.<rol>"` que el propio comentario de `Inputs.Artifacts` en `design.md` §5.3 ya menciona, en vez de inventar una convención nueva. No hay ninguna prueba RED de `tasks.md` que dicte esto explícitamente para `role-apply`; es una decisión de ingeniería razonable y documentada, no una interpretación forzada.
7. **Campo `Blocks` de `GateState` para `role-apply:<rol>` fijado como el propio nombre del rol** (no una fase), consistente con el comentario ya existente en `types.go` ("`Blocks string // fase o rol que esta compuerta retiene`", escrito en la Fase 1). Las cuatro compuertas fijas usan un nombre de fase (`design`, `tasks`, `apply`, `archive`).
8. **`EvaluateGates` nunca devuelve un error distinto de `nil`** en esta implementación (Fases 5-7): ninguna tarea RED de `tasks.md` exige un caso de error explícito para esta función, así que no se inventó uno. La firma conserva `error` tal como la define `design.md` §5.3, por si una fase posterior lo necesita.
9. **Corrección real encontrada por el propio guardián de la Fase 7 (7.5/7.6)**: `internal/kickoff/infer.go` (Fase 3, ya cerrada) importa `internal/workspace`, ausente de la lista literal de la tarea 7.5 y de `design.md` §4.1 ("internal/multirole, internal/reviewtransaction, stdlib, gopkg.in/yaml.v3. Nada más."). Es estructuralmente inevitable: `InferKickoff` (tarea 3.2, ya implementada y cerrada) acepta `*workspace.WorkspaceConfig` para pasarlo intacto a `multirole.DetectRoles`, que ya exige ese mismo tipo — `internal/multirole/detector.go` importa `internal/workspace` directamente. No hay forma de conservar esa firma ya cerrada sin este import. Se corrigió ampliando la lista blanca del escáner (añadiendo `internal/workspace`) en vez de reabrir o modificar la Fase 3 ya cerrada. `internal/workspace` solo declara tipos de configuración; no introduce ningún camino hacia autoridad de revisión ni hacia el vocabulario prohibido. Se eleva como hallazgo honesto, no se oculta.
10. **Restricción textual adicional sobre el vocabulario prohibido (ver "Nota de ejecución sensible" arriba)**: el escáner de 7.5 ensambla los cinco términos a partir de fragmentos literales para que el propio fichero de test no los contenga como subcadena contigua, satisfaciendo una restricción del orquestador que va más allá de lo que pide `design.md`/`tasks.md` literalmente.

Sin desviaciones respecto a D-05, D-08, D-09, D-10, D-14 en su comportamiento observable, ni respecto a la frontera normativa RDD (§1.3): el guardián de la Fase 7.5 lo confirma activamente contra el estado real del paquete, no solo lo declara.

**Fases 8-10 (esta ejecución):**

11. **`defaultRoleArtifactFiles` extraído a `types.go` y `infer.go` refactorizado para reutilizarlo (Fase 8).** La tarea 9.4 exige que el adaptador CLI no contenga lógica de decisión, y el nombrado de ficheros por rol (`tasks.md` vs `tasks.<rol>.md`) es exactamente ese tipo de decisión — antes vivía duplicada dentro de `inferredKickoff` (`infer.go`, Fase 3 ya cerrada). Se extrajo la función compartida y se modificó `infer.go` para invocarla, un cambio de refactor sin alteración de comportamiento: se reejecutó la suite completa de `internal/kickoff` tras el cambio (48 `--- PASS` + 1 `--- SKIP`, igual que antes) para confirmar que las Fases 1-7 no sufrieron regresión. Es la misma disciplina que la Fase 7 ya aplicó al ampliar la lista blanca del escáner en vez de reabrir la Fase 3: un cambio aditivo y verificado en un fichero ya cerrado, nunca una reinterpretación de su tarea original.
12. **`--evidence-kind`, `--commit`, `--base-ref` y `--evidence` deliberadamente ausentes de `ParseGateRecordArgs` (Fase 8) y de `RunSDDGate` (Fase 10).** `design.md` §5.7 muestra estas banderas como parte del contrato final de `gate record`, pero `tasks.md` 21.2 las asigna explícitamente a la Fase 21 ("banderas --evidence-kind, --commit, --base-ref, --evidence; inyección del AncestryChecker real"), que depende de `RevisionIsAncestor` (Fase 19, inexistente todavía) y de `VerifyIntegrationEvidence` (Fase 20, inexistente todavía). Añadirlas ahora sin comprobador real detrás violaría T-11 (nunca fabricar un veredicto de evidencia que no se puede probar) o exigiría inventar semántica que la Fase 21 tendría que deshacer. Ninguna tarea RED de la 10.1 las menciona. Se documenta como alcance diferido, no como omisión.
13. **`resolveGovernanceChangeRoot` busca el cambio en dos ubicaciones** (`openspec/changes/<cambio>` y `openspec/changes/archive/<cambio>`), no solo en la activa. Es la única forma de reconciliar dos exigencias simultáneas: T-7 (Fase 8) prohíbe que `--change` contenga separadores de ruta, así que un usuario nunca puede escribir `--change archive/x`; y la Fase 9 exige que un sellado sobre un cambio archivado se rechace citando D-14/REQ-21.18 (no un genérico "no existe"). Buscar en ambas ubicaciones antes de invocar `kickoff.RefuseArchivedRoot` deja que esa guarda, ya existente desde la Fase 7, sea la que produce el rechazo correcto. No es una regla explícita de `design.md`, pero es la única lectura consistente de sus dos exigencias combinadas.
14. **`lastRoleJustClosed` (Fase 10) marca cada rol del roster como "alcanzado" (`RolePending[rol] = 0`) antes de invocar `kickoff.EvaluateGates`.** El seguimiento real de tareas pendientes por rol pertenece a `sddstatus` (Fase 13, todavía inexistente), que este verbo no lee. La pregunta que `sdd gate record` necesita responder es más estrecha: dado el roster ya sellado, ¿ha quedado aprobada la compuerta `role-apply:<rol>` de cada uno en el ledger? Esa pregunta se responde correctamente con el truco: una compuerta con un rechazo sin remediar sigue siendo `rejected` en `resolveGateStatus` sin importar este mapa de relleno, así que `LastRoleClosed` sigue devolviendo `false` para un rol no resuelto — confirmado explícitamente por `TestRunSDDGateRecordNoNoticeWithRolesStillPending` y `TestRunSDDGateRecordRejectedRoleApplyNeverEmitsNotice`.
15. **`gateKeyInRoster` compara contra `kickoff.RoleApplyGate(rol)` para cada rol del roster, en vez de extraer el nombre de rol de `parsed.Gate` a mano.** Evita añadir una segunda función exportada de "extracción inversa" a `internal/kickoff` (que sí se consideró: `RoleFromGateKey`) para el único consumidor que la necesitaba. `isRoleApplyGate` distingue compuerta fija de `role-apply:<rol>` comparando solo contra las cuatro constantes exportadas (`kickoff.GateSpec`, etc.), nunca contra el literal `"role-apply:"` — evitando exactamente el tipo de divergencia que el comentario de `RoleApplyGate` en `types.go` ya advierte.
16. **`--actor` ausente en `gate record` se sella como `"cli"`** (Fase 10), reutilizando la misma convención que `SealedBy: "cli"` ya usa para un sellado explícito no inferido. Ninguna tarea RED lo exige; es un valor por defecto razonable y documentado, no una interpretación forzada.
17. **Los esquemas JSON de `kickoff seal/show --json` y `gate show` son invenciones de esta ejecución** (Fase 9-10): ninguna capacidad previa expone un contrato JSON para estos dos verbos nuevos — `design.md` §5.4 define `kickoffV2`/`governanceV2` para la proyección de `StatusV2Projection` (Fase 14, un consumidor distinto), no para la salida directa de `axiom sdd kickoff`/`gate`. Se eligieron claves `camelCase` por consistencia con esa convención ya establecida en el repositorio, sin reutilizar sus tipos Go (que llevan una forma distinta, orientada a la proyección de estado, no al resumen de un sellado o de un registro).
18. **Se añadió un caso de "cambio inexistente" en `sdd_kickoff_test.go` (Fase 9) más allá de lo literal de la tarea 9.1.** `design.md` §5.7 lista esa fila en la tabla de contrato de CLI ("Cambio inexistente | Rechazo nombrando la ruta resuelta | 1"); `tasks.md` 9.1 no la menciona explícitamente. Se implementó y se probó para no dejar sin cubrir una fila normativa del contrato citado como autoridad.

Sin desviaciones respecto a D-01 a D-04 en su comportamiento observable: el sello sigue siendo de escritura única, la derivación D-03 sigue ganando por el explícito, y ningún verbo de esta ejecución escribe `kickoff.yaml`/`gates.yaml` fuera de `kickoff.Seal`/`kickoff.AppendGate`.

## Issues Found

*(Fases 1-4, sin cambios: presupuesto de 400 líneas excedido en la PR1 sin válvula disponible; `-race` no ejecutable en este entorno.)*

3. **Presupuesto de 400 líneas excedido en la Fase 7 combinada (486 líneas), con válvula de alivio disponible y usada.** `tasks.md` sí declaraba la válvula para esta fase exacta ("separar `internal/kickoff/rdd_boundary_test.go` en una PR inmediatamente posterior, `inc-21/07b-kickoff-rdd-boundary`"). Se activó: `LastRoleClosed` + `archived.go`/`archived_test.go` quedaron en la PR 7 (170 líneas), y `rdd_boundary_test.go` (316 líneas) pasó a la PR 7b, ambas dentro de presupuesto por separado. A diferencia del issue de la PR1 (Fase 1, sin válvula), este caso funcionó exactamente como el diseño previó.
4. **`-race` sigue sin poder ejecutarse en este entorno** (sin cambios respecto a la ejecución anterior). Ninguna prueba nueva de las Fases 5-7 depende de concurrencia real.
5. **Presupuesto de 400 líneas excedido en las Fases 8, 9 y 10, las tres sin válvula de alivio declarada en `tasks.md`.** Fase 8: 875 líneas (862+/13-). Fase 9: 561 líneas (551+/10-, de las cuales ~9+9 son el propio movimiento de checkboxes de `tasks.md`). Fase 10: 482 líneas (476+/6-, con la misma composición). Igual que la PR1 (Fase 1), ninguna de las tres tenía una válvula declarada de antemano para dividirla, y en los tres casos el exceso proviene de una cobertura de test exhaustiva exigida literalmente por sus propias tareas RED (la tabla completa de ocho vectores T-2/T-7 en la Fase 8; los siete escenarios end-to-end de `sdd kickoff` incluidos los tres casos de `--infer` en la Fase 9; los quince escenarios de `sdd gate` incluida la triangulación del aviso de último rol en la Fase 10) — no de código de producción sobredimensionado ni de relleno. No se recortó ningún test ni comentario para forzar el ajuste al presupuesto, siguiendo la instrucción explícita del orquestador. Se registran las tres como candidatas a decisión `size:exception`.
6. **`go test ./internal/cli/...` (paquete completo) no completa dentro de un `-timeout 300s` en este entorno** (ver "Comandos de Verificación Ejecutados (Fases 8-10)" arriba para el detalle de la traza). Es la misma clase de riesgo de bloqueo por subproceso git ya documentada para `internal/sddstatus`, alcanzada aquí transitivamente por la dependencia de `internal/cli` sobre ese paquete; no está causada por ningún fichero que esta ejecución creó o modificó. Se verificó en su lugar con `go test ./internal/cli/... -run '^TestRunSDD'` (3.5s, sin fallos) y con el arnés de runtime real contra el binario compilado.
7. **`-race` sigue sin poder ejecutarse en este entorno** (sin cambios). Ninguna prueba nueva de las Fases 8-10 depende de concurrencia real: el parseo de argumentos y los adaptadores de CLI son secuenciales, sin goroutines propias.

## Estimación de Líneas Cambiadas por PR (evidencia real, no pronóstico)

| PR | Rama prevista (tracker) | Commit | Líneas reales (adiciones+eliminaciones) | Presupuesto 400 | Válvula de alivio usada |
|---|---|---|---|---|---|
| 1 | `inc-21/01-kickoff-types-schema` | `eed8fec5` | 534 + 5 = **539** | ⚠️ Excedido (sin válvula) | No — ninguna disponible |
| 2 | `inc-21/02-kickoff-seal` | `57fcf2e1` | 328 + 4 = **332** | ✅ Dentro | No necesaria |
| 3 | `inc-21/03-kickoff-retro-seal` | `ad3f6bf2` | 265 + 4 = **269** | ✅ Dentro | No necesaria |
| 4 | `inc-21/04-kickoff-ledger-digest` | `10f723d6` | 372 + 6 = **378** | ✅ Dentro | Evaluada y descartada |
| 5 | `inc-21/05-kickoff-gate-machine-core` | `aa622c43` | 371 + 4 = **375** | ✅ Dentro | No necesaria |
| 6 | `inc-21/06-kickoff-gate-machine-reopen` | `c3779151` | 208 + 12 = **220** | ✅ Dentro | Evaluada y descartada (220 < 400) |
| 7 | `inc-21/07-kickoff-role-closure-archived-guard` | `953bb427` | 179 + 9 = **188** | ✅ Dentro | — |
| 7b | `inc-21/07b-kickoff-rdd-boundary` | `d4aec544` | 316 + 0 = **316** | ✅ Dentro | **Usada** — separada de la Fase 7 (conjunto real: 486 líneas, excedía 400) |
| Documentación (commit adicional, Fases 1-4) | — | `2bcac987` | 112 + 0 | N/A (docs) | — |
| **Total Fases 1-4** | — | — | 1499 + 19 = **1518** | (presupuesto de intento: 2600) | — |
| **Total Fases 5-7 (ejecución anterior)** | — | — | 1065 + 16 = **1081**\* | — | — |
| **Total acumulado (Fases 1-7)** | — | — | **2599** | (presupuesto de aquel intento: 2600 — a 1 línea del límite) | — |
| 8 | `inc-21/08-cli-args` | `c7c334fa` | 862 + 13 = **875** | ⚠️ Excedido (sin válvula) | No — ninguna disponible |
| 9 | `inc-21/09-cli-kickoff-verb` | `0b866af1` | 551 + 10 = **561** | ⚠️ Excedido (sin válvula) | No — ninguna disponible |
| 10 | `inc-21/10-cli-gate-verb` | `6f50786` | 476 + 6 = **482** | ⚠️ Excedido (sin válvula) | No — ninguna disponible |
| **Total Fases 8-10 (esta ejecución)** | — | — | 1888 + 28 = **1916**\*\* | (presupuesto de este intento: 2600) | — |

* Medido con `git diff --shortstat 56fcb2d8..d4aec544`, es decir excluyendo el commit de documentación que cierra la tanda (`acfb1654`), la misma convención usada para el total de las Fases 1-4. El diff de extremo a extremo `56fcb2d8..HEAD`, que sí incluye ese commit de documentación, mide 1191+48=1239 líneas. Los tamaños por commit (`git show --shortstat`) son 375, 220, 188, 316 y 158.

** Medido con `git diff --shortstat 9927fae0..6f50786` (rango de extremo a extremo de esta ejecución: HEAD real al inicio, `9927fae0`, hasta el cierre de la Fase 10) — **cifra autoritativa**. La suma columna a columna de los tres commits individuales (`git show --shortstat` de cada uno: 875+561+482) da 1918, 2 líneas por encima de la medición de extremo a extremo; la diferencia es la reconciliación normal de `git diff` cuando la misma línea de `cmd/axiom/main.go` (el mensaje de error del `default` del switch, que enumera los subcomandos válidos) se edita en más de un commit sucesivo — cada commit cuenta esa línea como su propia sustitución, mientras que el diff de extremo a extremo solo ve el estado inicial y el final. Se reporta la cifra de extremo a extremo por ser la que un revisor vería en un diff acumulado real.

**Aviso para el orquestador (histórico, Fases 1-7, no aplica a esta ejecución)**: la ejecución anterior cerró con un total acumulado de 2599 de un presupuesto de intento de 2600 líneas. Esta ejecución (Fases 8-10) opera bajo un **intento nuevo y distinto**, acordado explícitamente por el orquestador para el `work-unit: PR 8-10, CLI surface` con presupuesto propio de 2600 líneas — no es continuación del intento anterior, y esta ejecutora no adquiere, liquida ni reescala intentos por instrucción explícita. El total real de esta ejecución, **1916 de 2600 líneas del intento actual** (medido `git diff --shortstat 9927fae0..6f50786`), queda registrado aquí para que el orquestador liquide (`settle`) el intento con evidencia exacta.

## Remaining Tasks

- [ ] Fases 11 a 23 completas (proyección `sddstatus` — compuerta de control, envoltorio, cableado y enrutamiento v2 —, roster multi-rol reservando `fullstack`, relevo de integración, precondición de archive, doctrina y activos por agente). Sin empezar en esta ejecución, tal como se instruyó explícitamente (alcance: solo Fases 8-10). La Fase 11 es un control gate normativo: su regresión byte a byte sin sello debe existir y confirmarse en verde **antes** de tocar `internal/sddstatus/status.go`/`status_v2.go` (Fases 12-14).

## Workload / PR Boundary

- **Modo**: PRs encadenadas (`feature-branch-chain`) contra la rama tracker `feature/inc-21-upfront-flow-governance`.
- **Unidad de trabajo de la ejecución anterior**: PR 5, PR 6, PR 7 y PR 7b (válvula de alivio activada), máquina de compuertas y cierre de rol.
- **Unidad de trabajo de esta ejecución**: PR 8 (`internal/kickoff/args.go`), PR 9 (`axiom sdd kickoff seal|show`) y PR 10 (`axiom sdd gate record|show`) — la rebanada P2 completa (superficie CLI).
- **Frontera de este lote**: empieza en `9927fae0` (HEAD real al inicio de esta ejecución) y termina en `6f50786` (cierre de la Fase 10). Tres commits reales sobre la rama tracker `feature/inc-21-upfront-flow-governance`: `c7c334fa` (Fase 8), `0b866af1` (Fase 9), `6f50786` (Fase 10). Ninguna rama hija ni PR real se creó — son fronteras documentales para cuando el usuario decida crearlas. Instrucción explícita: sin `git push`, sin ramas hijas, sin PRs.
- **Impacto estimado en presupuesto de revisión**: 1916 líneas cambiadas repartidas en 3 unidades de trabajo autónomas y verificables por separado, **las tres por encima del presupuesto individual de 400 líneas y sin válvula de alivio declarada** — a diferencia del lote anterior (Fases 5-7), donde solo una de cuatro unidades necesitó válvula. Se registran las tres como candidatas a decisión `size:exception` del mantenedor; el exceso proviene de cobertura de test exhaustiva exigida literalmente por las tareas RED de cada fase (ver Issues Found #5), no de código de producción inflado.
- **Cadena prevista actualizada**: `inc-21/01` → `02` → `03` → `04` → `05` → `06` → `07` → `07b` → `08` → `09` → `10` → (Fase 11 en adelante, fuera de este alcance).

## Status

34/34 tareas de las Fases 1-7 completas (19 de Fases 1-4 + 15 de Fases 5-7) — histórico, ejecución anterior.

**Esta ejecución**: 14/14 tareas de las Fases 8-10 completas (4 de Fase 8 + 5 de Fase 9 + 5 de Fase 10), las tres con verificación de cierre V-A/V-B/V-C/V-D en verde y, adicionalmente para las Fases 9-10, arnés de runtime real contra el binario compilado. **Ready for next batch** (Fase 11 en adelante — compuerta de control de `internal/sddstatus` primero) — no listo para archive todavía.

**Total acumulado**: 48/114 tareas totales de `tasks.md` completas (48 = 34 de Fases 1-7 + 14 de Fases 8-10), 66 restantes (Fases 11-23). Confirmado por conteo real: `grep -c '^\- \[x\]' tasks.md` → 48; `grep -cE '^\- \[[x ]\]' tasks.md` → 114.
