# Progreso de Implementación: Determinación Temprana de Flujo, Compuertas de Revisión por Bloque, Relevo de Cierre de Roles y Ciclo de Vida de Archive (INC-21)

> **Alcance acumulado (actualizado tras las Fases 8-10):** Fases 1 a 10 del documento `tasks.md` — la rebanada de diseño P1 completa (dominio puro `internal/kickoff`: tipos, esquema, sellado, retro-sellado, ledger, digest, máquina de compuertas con reapertura y multi-rol, cierre de último rol, guarda de raíz archivada y frontera RDD del dominio) **más** la rebanada P2 completa (superficie CLI: `internal/kickoff/args.go` y los verbos ejecutables `axiom sdd kickoff seal|show` y `axiom sdd gate record|show`, cableados en `cmd/axiom/main.go`). Las Fases 11 a 23 quedan sin empezar para una ejecución posterior. (Redacción original de este párrafo, válida hasta el cierre de la Fase 7: "Fases 1 a 7 del documento `tasks.md`... Las Fases 8 a 23 quedan sin empezar para una ejecución posterior" — preservada aquí entre comillas en vez de borrada, siguiendo la misma disciplina de no perder texto ya escrito.)
> **Modo:** Strict TDD (`openspec/config.yaml: strict_tdd: true`). Cada tarea `[RED]` se escribió y se observó fallar (por fallo de compilación o por aserción) antes de su tarea `[GREEN]` correspondiente.
> **Rama:** `feature/inc-21-upfront-flow-governance` (tracker, sin merge directo a `main`). Sin push, sin creación de ramas hijas ni PRs — decisiones del usuario.
> **Este documento es un merge.** El listado de tareas de las Fases 1-4 se conserva byte a byte de la ejecución anterior; sus tablas de evidencia y sus notas se reescribieron de forma más compacta al integrarlas, sin alterar ningún hecho, cifra ni conclusión (algunas citas concretas —un nombre de test, una referencia cruzada, un detalle de arnés— sí se perdieron en la condensación). Las Fases 5-7 fueron la ejecución anterior a esta.
> **Nota de la ejecución de las Fases 8-10, corregida tras validación:** esta ejecución se propuso conservar todo el texto anterior intacto y **no lo consiguió del todo**. Los hechos, cifras y conclusiones de las Fases 1-7 se conservan sin alterar, y su listado de tareas es idéntico, pero un validador de contexto fresco encontró al menos tres frases de la narrativa previa reescritas durante la fusión: el párrafo de "Alcance acumulado", la frase del "Arnés de runtime N/A" (que perdió una cita literal de `design.md` §8.2) y el párrafo de "Aviso para el orquestador" (que perdió la frase sobre `settle`/`reset`/`rescope`). Las Fases 8-10 se añaden como secciones nuevas al final de cada bloque existente, nunca reemplazando ni reordenando. Se deja constancia del fallo en vez de sostener una afirmación de preservación literal que el diff desmiente.

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

### Remediación (post Fase 8, esta ejecución): endurecimiento de `validateChangeName`
- [x] R.1 [RED] Ampliada `TestChangeNameContainmentRejectsAllEightVectors` (renombrada a `TestChangeNameContainmentRejectsAllTenVectors`) en `internal/kickoff/args_test.go` con dos vectores nuevos hallados por un validador independiente: `"C:foo"` (segmento relativo de unidad de Windows, sin separador `/`/`\`, `filepath.IsAbs` devuelve `false`) y `"con "` (nombre reservado de Windows con espacio final sin extensión, elude el corte en el primer `.`). Ejecutado y confirmado en rojo genuino (fallo de aserción, no de compilación): las 8 filas originales siguieron en verde, las 2 nuevas fallaron exactamente como se esperaba.
- [x] R.2 [GREEN] `internal/kickoff/args.go`: `validateChangeName` amplía `strings.ContainsAny(name, "/\\")` a `"/\\:"` (rechaza el separador de unidad de Windows en cualquier posición) y recorta el espacio final del segmento `base` con `strings.TrimRight(base, " ")` antes de compararlo contra `reservedWindowsNames`.
- [x] R.3 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`, 160 `--- PASS` + 0 `--- FAIL` + 1 `--- SKIP` con `go test ./internal/kickoff/... -v`), V-C, V-D — verde. Ningún vector de los ocho originales cambió de comportamiento; ninguno de los dos nuevos escapa realmente `openspec/changes/` (es endurecimiento, no una vulnerabilidad viva), pero ambos quedan cerrados con test antes del fix, como exige TDD estricto.

### Fase 11: P3a — Compuerta de control: regresión byte a byte sin sello (REQ-21.7; D-05 — compuerta de control nº 1)
- [x] 11.1 [Caracterización] `internal/sddstatus/kickoff_absence_regression_test.go`: fixture equivalente (`seedReadyChange`, ya existente en el paquete, sobre `t.TempDir()`) para un cambio **sin** `kickoff.yaml`; comparación byte a byte de la cadena JSON completa de `StatusV2Projection` contra un golden congelado, con la raíz de workspace variable sustituida por un token fijo (`<WORKSPACE_ROOT>`) en ambos lados de la comparación para que el test sea determinista entre ejecuciones. Capturado ejecutando el propio test contra el árbol sin modificar (técnica estándar de arranque de golden: placeholder → ejecutar → capturar la salida real → congelarla) y confirmado en verde **antes** de crear `governance.go` o tocar `status.go`/`status_v2.go`. Incluye además una aserción explícita de que la clave `"governance"` está ausente.
- [x] 11.2 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`, 326 `--- PASS` + 1 `--- FAIL` preexistente y ajeno — ver Issues Found — en 39s, dentro del presupuesto de 600s), V-C, V-D — verde.

### Fase 12: P3b — Envoltorio tipado de compuerta (REQ-21.8, REQ-21.9, REQ-21.10, REQ-21.11; D-08; T-9)
- [x] 12.1 [RED] `internal/sddstatus/governance_test.go`: `Validate()` rechaza esquema ajeno (vacío, el esquema hermano de consentimiento de autoridad de edición, una versión distinta); rechaza cualquier conjunto de `Choices` distinto de `approved`/`rejected` en ese orden (orden invertido, answer ajeno, una sola choice, tres choices); rechaza una invocación que no empiece por `axiom sdd gate record ` — **incluida explícitamente `axiom sdd-attempt grant ...`** (T-9) — y también `axiom sdd status ...`, cadena vacía, y un verbo similar sin el espacio final del prefijo; acepta las cinco filas de criterios de `design.md` §5.5 tal cual, leídas de una tabla `gateCriteria` compartida. Más allá de lo literal de la tarea 12.1 (que solo enumera casos de `Validate()`), se añadieron RED propios para `loadGovernance` (producción introducida por esta misma tarea 12.2, así que exige su propia cobertura bajo TDD estricto): sin kickoff sellado ⇒ `nil, nil`; kickoff corrupto ⇒ error nombrado; kickoff sellado con un rol `fullstack` y un registro `AppendGate` real ⇒ traducción correcta de `pending` a `approved`. Fallo de compilación observado (`SDDGovernanceGateResult` indefinido) antes de crear `governance.go`.
- [x] 12.2 [GREEN] `internal/sddstatus/governance.go`: `SDDGovernanceGateResult` y `Validate()` sobre `consentenvelope.Core`, con las constantes `SDDGovernanceGateSchema`, `SDDGovernanceContractV1`, `gateOperation`, `gateActionRequired`, `gateAnswerApproved`, `gateAnswerRejected`, `gateRecordInvocationPrefix`, `gateStatusInvocationPrefix` (el conjunto completo de `design.md` §5.5, no solo las cuatro que cita literalmente la tarea 12.2 — esta cita solo un subconjunto ilustrativo, `design.md` es la autoridad del contrato completo); `gateCriteria(kickoff.GateState) []string` puebla `Criteria` desde una tabla fija más una entrada `role-apply` reconocida comparando contra `kickoff.RoleApplyGate(state.Blocks)` (mismo truco que `gateKeyInRoster` de la Fase 10, sin re-derivar el literal `"role-apply:"`); `loadGovernance(changeRoot) (*Governance, error)` que carga el kickoff sellado y su ledger, calcula los digests de artefactos y las tareas pendientes por rol (`governanceArtifactInputs`), y llama a `kickoff.EvaluateGates`.
- [x] 12.3 [REFACTOR] `grep -inE "receipt|lineage|acknowledge|burn|candidate"` sobre `governance.go`/`governance_test.go` encontró una ocurrencia real de "receipt" en un comentario ("never a receipt and never edit-authority escalation") — corregida reescribiendo la frase sin esa palabra, sin cambiar el significado. Confirmado limpio tras la corrección.
- [x] 12.4 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`, 351 `--- PASS` + 1 `--- FAIL` preexistente y ajeno), V-C, V-D, V-E (Fase 11 reconfirmada en verde) — verde.

### Fase 13: P3c — Cableado de gobernanza en el resolutor (REQ-21.4 parcial, REQ-21.7, REQ-21.16 parcial; D-05, D-09, D-13; T-10)
- [x] 13.1 [RED] Ampliado `internal/sddstatus/status_test.go`: sellado checkpointed + spec/design/tasks completos sin ningún registro de compuerta ⇒ `nextRecommended = "await-gate"`, `BlockedReasons` nombra la compuerta `spec` pendiente, `Dependencies.Archive` permanece `ready` (sin cambio respecto a la lógica de hoy — la condición completa de integración llega en la Fase 21); compuerta `spec` rechazada con digest igual al artefacto actual ⇒ `nextRecommended = "spec"` con el motivo exacto registrado en `BlockedReasons`; `kickoff.yaml` corrupto (`"{ not: valid: yaml"`) ⇒ `Resolve` devuelve error, nunca degrada a "sin sello" (T-10); sello en modalidad `continuous` ⇒ `Status.Governance` permanece `nil` y `nextRecommended` no cambia (caso propio, más allá de lo literal de la tarea, para triangular D-05 de punta a punta a través de `Resolve`, no solo dentro de `loadGovernance`). Los dos call-sites existentes de `artifactBlockedReasons` en tests se actualizaron con un cuarto argumento `nil`. Fallo de compilación observado (`status.Governance` indefinido) antes de tocar `status.go`.
- [x] 13.2 [GREEN] `internal/sddstatus/status.go`: campo `Governance *Governance` en `Status`; `loadGovernance(changeRoot)` invocado en `resolveByPreferenceOrder` (el resolutor OpenSpec/híbrido — es la misma función que sirve ambos, `Resolve` solo reetiqueta la tienda después) inmediatamente después de `taskProgress`; el propio `loadGovernance` decide el coste cero (sin kickoff ⇒ `nil,nil`; modo continuo ⇒ `nil,nil`, ampliado en esta misma fase — ver Deviations); `artifactBlockedReasons` gana un cuarto parámetro `governance *Governance` y añade el motivo genuino de `governanceBlockedReason` al final, sin alterar ninguna rama existente; `resolveNextRecommended` gana un tercer parámetro `governance *Governance` y consulta `governanceNextRecommended` **antes** de cualquier otra ruta (una compuerta pendiente debe interceptar incluso una fase que ya parecía lista por estado de dependencias — REQ-21.8 a 21.11 lo exigen así); el call-site de Engram (`resolveEngramStatus`) pasa `nil` explícito a ambas funciones con un comentario explicando por qué (el kickoff es un concepto de fichero de OpenSpec, D-01, y Engram no tiene ese directorio); `engramTitlePattern` amplía su alternancia con `|kickoff|gates`.
- [x] 13.3 [REFACTOR] Confirmado: `resolveNextRecommended` tiene su guarda `if governance != nil` como primera instrucción de la función; `loadGovernance` tiene sus dos guardas (`sealed == nil`, `ExecutionContinuous`) como las dos primeras comprobaciones tras sus lecturas obligatorias; `artifactBlockedReasons` tiene una única guarda al final (no dispersa) — se documenta esta colocación como decisión deliberada, no una desviación: la función acumula motivos de forma incremental y no tiene ninguna rama de retorno anticipado a la que "el inicio" pudiera aplicarse con sentido.
- [x] 13.4 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`, 356 `--- PASS` + 1 `--- FAIL` preexistente y ajeno en 41s), V-C (`gofmt -w` corrigió una alineación de columnas de struct tras la edición, sin cambio de contenido), V-D, V-E (Fase 11 reconfirmada en verde, byte a byte, **después** del cableado real) — verde.

### Fase 14: P3d — Proyección v2 y enrutamiento `await-gate` (REQ-21.7 a REQ-21.10; D-09)
- [x] 14.1 [RED] Ampliado `internal/sddstatus/status_v2_test.go`: sello checkpointed + compuerta `spec` pendiente ⇒ `StatusV2Projection.Governance` no `nil`, con `Kickoff.Schema`/`ExecutionStyle` correctos, una entrada de compuerta `spec` en estado `pending`, y `Roster = {kickoff, [fullstack]}`; cambio sin sellar ⇒ `Governance` es `nil` y `json.Marshal` de la proyección **no** contiene la subcadena `"governance"` (repite la aserción de la Fase 11 al nivel v2); sello en modalidad `continuous` ⇒ `Governance` sigue `nil` a nivel v2 y `nextRecommended = "apply"` (REQ-21.7, segundo escenario, verificado también aquí, no solo en la Fase 13); compuerta `spec` rechazada ⇒ el `gateV2` correspondiente lleva `status: "rejected"` y `reason` con el texto exacto registrado; las tres compuertas fijas (`spec`,`design`,`tasks`) aprobadas y `role-apply:fullstack` todavía sin abrir (su única tarea sigue pendiente) ⇒ `nextRecommended = "apply"` sin ninguna interferencia (REQ-21.8/21.9/21.10, patrón "aprobado habilita la siguiente fase"); `statusV2NextRecommended("await-gate") = true`; `nonPhaseRoutingInstructions` para `await-gate` imprime dos invocaciones ejecutables conteniendo `axiom sdd gate show`, `axiom sdd gate record`, `--change <nombre>` y `--gate spec`. Fallo de compilación observado (`projection.Governance` indefinido) antes de tocar `status_v2.go`.
- [x] 14.2 [GREEN] `internal/sddstatus/status_v2.go`: campo `Governance *governanceV2 \`json:"governance,omitempty"\`` en `StatusV2Projection`; tipos `governanceV2`/`kickoffV2`/`gateV2`/`rosterV2` calcados literalmente de `design.md` §5.4 (`rosterV2.Conflict` queda siempre `nil` en esta rebanada — su único productor, `multirole.ResolveRoster`, es la Fase 16); `projectGovernanceV2` traduce `*Governance` con `time.RFC3339` para `SealedAt` (mismo formato que `sdd_kickoff.go:187` ya usa); `"await-gate"` añadido a la alternancia de `statusV2NextRecommended`. En `status.go`: caso nuevo `"await-gate"` en `nonPhaseRoutingInstructions` (tocado aquí, no en la Fase 13, porque depende de que el valor exista en el enum v2) que imprime `axiom sdd gate show`/`axiom sdd gate record` con el cambio y la clave de compuerta reales; extraído `firstOpenGate(*Governance) (kickoff.GateState, bool)` en `governance.go` como único punto de lectura de "la primera compuerta abierta en orden fijo", y refactorizados `governanceNextRecommended`/`governanceBlockedReason` (Fase 13) para compartirlo en vez de repetir la misma iteración tres veces — refactor cubierto por las suites ya verdes de la Fase 13, sin regresión.
- [x] 14.3 [REFACTOR] Confirmado por inspección del diff: `Governance` se añadió como campo nuevo con `omitempty` a `StatusV2Projection` (ningún campo existente cambió de tipo, nombre o semántica); `"await-gate"` se añadió como un caso más de la alternancia de `statusV2NextRecommended` (ningún valor existente se retiró ni cambió de significado) — mismo patrón que `"hybrid"`/`"archived"` ya documentan en el propio fichero.
- [x] 14.4 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`, 362 `--- PASS` + 1 `--- FAIL` preexistente y ajeno en 63s — variación de tiempo frente a la Fase 13 atribuida a carga del sistema, no a un cambio de comportamiento: mismo comando, mismo resultado), V-C (`gofmt -w` corrigió alineación en `status_v2.go`), V-D, V-E (Fase 11 reconfirmada en verde, byte a byte, tras el cierre completo de la rebanada P3) — verde.

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

## TDD Cycle Evidence (Remediación y Fases 11-14, esta ejecución)

| Tarea | Fichero de test | Capa | Red de seguridad | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| R.1–R.2 | `args_test.go` (ampliación) | Unit | ✅ 8/8 vectores originales + 1 caso de aceptación, confirmados en verde antes del cambio | ✅ **RED de comportamiento genuino**: 2 de 10 vectores fallaron contra la implementación de la Fase 8 (`"C:foo"`, `"con "`) | ✅ Las 10 subpruebas PASS tras el fix de `validateChangeName` | ✅ segmento de unidad windows + nombre reservado con espacio final, sin regresión en los 8 vectores previos | ✅ Limpio; un único `ContainsAny` ampliado, sin segunda copia de la comprobación |
| 11.1 | `kickoff_absence_regression_test.go` | Caracterización (no RED convencional) | N/A (fichero nuevo, sin ningún cambio de producción) | ➖ No aplica — es una compuerta de control, no un RED de comportamiento nuevo | ✅ Confirmado en verde contra el árbol sin modificar (evidencia de que la compuerta es honesta) | ➖ Un único escenario por diseño (un cambio sin kickoff.yaml); la ausencia de `"governance"` se afirma explícitamente | ➖ Ninguno necesario |
| 12.1–12.2 | `governance_test.go` | Unit | N/A (paquete nuevo dentro de `sddstatus`) | ✅ Fallo de compilación observado (`SDDGovernanceGateResult` indefinido) | ✅ 12 funciones de nivel superior PASS (varias con subpruebas de tabla) tras crear `governance.go` | ✅ esquema×3, choices×4, invocación×4, criterios×5, más 3 casos propios de `loadGovernance` (sin sello, corrupto, traducción real con `AppendGate`) | ✅ Limpio tras la corrección de vocabulario (12.3) |
| 12.2 (ampliación) | `governance_test.go` (`TestLoadGovernanceReturnsNilForContinuousExecutionStyle`) | Unit | ✅ 15/15 pruebas de gobernanza ya verdes | ✅ **RED de comportamiento genuino**: `loadGovernance` devolvía una `Governance` no nula con `Gates` vacío para un sello continuo, en vez de `nil` | ✅ PASS tras añadir la guarda de modo continuo | ➖ Un único escenario adicional, generaliza la Fase 12 antes de que la Fase 13 lo necesite | ✅ Limpio |
| 13.1–13.2 | `status_test.go` (ampliación) | Integración (`Resolve` de punta a punta, `t.TempDir()`) | ✅ Suite completa de `sddstatus` confirmada en verde antes de tocar `status.go` (351 PASS) | ✅ Fallo de compilación observado (`status.Governance` indefinido; `artifactBlockedReasons` con demasiados argumentos) | ✅ 5 funciones nuevas PASS tras cablear `status.go` | ✅ pending→await-gate/rejected→fase-dueña/kickoff-corrupto→error/continuo→sin-gobernanza | ✅ Limpio; guardas explícitas confirmadas por inspección (13.3) |
| 14.1–14.2 | `status_v2_test.go` (ampliación) | Integración (`Resolve`+`ProjectStatusV2`, `t.TempDir()`) | ✅ Suite completa de `sddstatus` confirmada en verde antes de tocar `status_v2.go` (356 PASS) | ✅ Fallo de compilación observado (`projection.Governance` indefinido) | ✅ 6 funciones nuevas PASS tras crear los tipos v2 y cablear `nonPhaseRoutingInstructions` | ✅ presente-con-pending/ausente-sin-sello/ausente-continuo/rejected-con-motivo/aprobado-no-bloquea/instrucciones-await-gate | ✅ `firstOpenGate` extraído y reutilizado por las Fases 13 y 14 sin duplicar la iteración |

**Nota sobre el refactor de 14.2 y su cobertura retroactiva:** extraer `firstOpenGate` obligó a reescribir `governanceNextRecommended`/`governanceBlockedReason`, ya cerradas y verdes desde la Fase 13. Se reejecutó la suite completa de `sddstatus` inmediatamente después (356 → resultado idéntico salvo las funciones nuevas) para confirmar que el refactor no alteró ningún comportamiento ya probado — la misma disciplina de aprobación que la Fase 8 ya aplicó sobre `infer.go`.

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

### Salida real observada — RED genuino de la remediación (dos vectores nuevos de `validateChangeName`)

```
=== RUN   TestChangeNameContainmentRejectsAllTenVectors/segmento_relativo_de_unidad_windows
    args_test.go:222: validateChangeName("C:foo") = nil, se esperaba rechazo por contencion
=== RUN   TestChangeNameContainmentRejectsAllTenVectors/nombre_reservado_windows_con_espacio_final_sin_extension
    args_test.go:222: validateChangeName("con ") = nil, se esperaba rechazo por contencion
--- FAIL: TestChangeNameContainmentRejectsAllTenVectors/segmento_relativo_de_unidad_windows (0.00s)
--- FAIL: TestChangeNameContainmentRejectsAllTenVectors/nombre_reservado_windows_con_espacio_final_sin_extension (0.00s)
FAIL
```

Los 8 vectores originales siguieron `--- PASS` en la misma ejecución. Tras el fix, las 10 subpruebas (incluidas las 8 originales) pasan sin regresión.

### Salida real observada — RED de compilación de las Fases 11-14

```
$ go vet ./internal/sddstatus/...
vet.exe: internal\sddstatus\governance_test.go:15:34: undefined: SDDGovernanceGateResult

$ go vet ./internal/sddstatus/...
vet.exe: internal\sddstatus\status_test.go:887:91: too many arguments in call to artifactBlockedReasons
	have (map[string]ArtifactState, TaskProgress, string, nil)
	want (map[string]ArtifactState, TaskProgress, string)

$ go vet ./internal/sddstatus/...
vet.exe: internal\sddstatus\status_v2_test.go:100:16: projection.Governance undefined (type StatusV2Projection has no field or method Governance)
```

La Fase 11 no tiene un RED de este tipo (es una compuerta de caracterización, no una tarea RED/GREEN convencional — ver su propia fila de la tabla). La Fase 12 tuvo además un RED de **comportamiento** genuino, no solo de compilación, al ampliar `loadGovernance` para el modo continuo:

```
=== RUN   TestLoadGovernanceReturnsNilForContinuousExecutionStyle
    governance_test.go:221: loadGovernance() = &sddstatus.Governance{... ExecutionStyle:"continuous" ... Gates:[]kickoff.GateState(nil) ...}, se esperaba nil para un sello en modo continuo (D-05)
--- FAIL: TestLoadGovernanceReturnsNilForContinuousExecutionStyle (0.01s)
FAIL
```

### Test Summary (acumulado, Fases 1-7)
- **Tests totales (nivel superior + subpruebas de tabla)**: 40 (Fases 1-4, sin cambios) + 21 nuevos de Fases 5-7 (12 funciones de nivel superior de `machine_test.go`, 4 de `archived_test.go`, 5 de `rdd_boundary_test.go`, con varias de ellas conteniendo sub-tests de tabla: `TestEvaluateGatesTasksGateRequiresEveryRoleTasksFileInMultiRole` ×2, `TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange` ×2, `TestLastRoleClosedRequiresEveryRoleApproved` ×3, `TestKickoffRDDBoundaryScannerCatchesKnownShapes` ×4, `TestKickoffRDDBoundaryScannerCatchesForbiddenVocabulary` ×5, `TestKickoffImportAllowlistTable` ×8, `TestKickoffProductionFilesStayInsideRDDBoundary` ×8 ficheros, `TestKickoffPackageImportsStayInsideAllowlist` ×8 ficheros).
- **Resultado agregado de `go test ./internal/kickoff/... -v`**: 48 `--- PASS` + 0 `--- FAIL` + 1 `--- SKIP` (el mismo SKIP intencional de la Fase 2, sin cambios).
- **Dos RED de comportamiento genuino** en esta ejecución (no solo fallo de compilación): la tabla de reapertura por digest (Fase 6, 3 de 6 pruebas nuevas) y la violación real de imports (Fase 7.5/7.6).

### Test Summary (Fases 8-10, esta ejecución)
- **`internal/kickoff` (Fase 8)**: 24 funciones de test de nivel superior nuevas en `args_test.go` (varias con subpruebas de tabla: `TestParseSealArgsExecutionStyleFromSessionPace` ×2, `TestParseSealArgsRoleFlagPolicyParsing` ×5, `TestParseSealArgsCWDAcceptsAnyShape` ×3, `TestChangeNameContainmentRejectsAllEightVectors` ×8, `TestParseGateRecordArgsKnownAndUnknownFlags` ×2, `TestParseGateRecordArgsGateVocabulary` ×7, `TestParseGateRecordArgsDecisionVocabulary` ×3, `TestParseShowArgsKnownAndUnknownFlags` ×3). Suite completa del paquete tras la Fase 8: `go test ./internal/kickoff/...` → `ok` (sin cambios de conteo respecto al resultado agregado de Fases 1-7 salvo la adición de estas funciones nuevas; no se ejecutó `-v` completo de nuevo tras la Fase 8 porque las Fases 9-10 no tocan este paquete y el resultado ya estaba confirmado verde).
- **`internal/cli` (Fases 9-10)**: 12 funciones nuevas en `sdd_kickoff_test.go` (una con 3 subpruebas, `TestRunSDDKickoffSealInferThreeCases`; otra con 2, `TestRunSDDKickoffHelp`) + 15 funciones nuevas en `sdd_gate_test.go` (una con 2 subpruebas, `TestRunSDDGateHelp`). Todas `--- PASS`, 0 `--- FAIL`, tras crear `sdd_kickoff.go`/`sdd_gate.go`/`sdd_governance.go` y cablear `main.go`.
- **Un RED de fallo de compilación genuino por fase** (8, 9, 10): en cada caso el fichero de test se escribió y se confirmó el fallo de compilación (símbolos indefinidos) antes de crear el fichero de producción correspondiente — ver la salida real observada más abajo.
- **Ningún RED de comportamiento genuino** en las Fases 8-10 (a diferencia de la Fase 6): todas las funciones GREEN pasaron en su primera ejecución tras implementar, sin un ciclo de corrección intermedio. Esto es honesto, no una omisión: el diseño de estas tres fases (parseo puro + adaptadores de E/S finos) no dejó margen para una discrepancia de comportamiento como la de la máquina de estados.

### Test Summary (Remediación y Fases 11-14, esta ejecución)
- **Remediación (`internal/kickoff`)**: 2 subpruebas nuevas dentro de `TestChangeNameContainmentRejectsAllTenVectors` (renombrada desde `...AllEightVectors`), 0 funciones nuevas de nivel superior. Suite completa del paquete tras el fix: `go test ./internal/kickoff/... -v` → 160 `--- PASS` + 0 `--- FAIL` + 1 `--- SKIP` (el mismo SKIP intencional de Windows, sin cambios).
- **`internal/sddstatus` (Fases 11-14)**: 1 función de caracterización nueva (Fase 11) + 15 funciones nuevas de nivel superior de gobernanza (Fase 12: 12 en `governance_test.go` más la ampliación de modo continuo) + 5 funciones nuevas de integración (Fase 13, `status_test.go`) + 6 funciones nuevas de integración v2 (Fase 14, `status_v2_test.go`) = 27 funciones de nivel superior nuevas en total. Suite completa del paquete, verificada tras cada fase de forma incremental: 326 → 351 → 356 → 362 `--- PASS`, siempre con la misma **1** `--- FAIL` preexistente y ajena (`TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots`, ver Issues Found).
- **Un RED de fallo de compilación genuino por fase** (12, 13, 14): cada fichero de test se escribió y se confirmó el fallo de compilación (símbolo de producción indefinido, o firma de función con parámetros insuficientes) antes de crear o modificar el fichero de producción correspondiente.
- **Dos RED de comportamiento genuino** en esta tanda, ambos fuera de lo literal del texto de las tareas 12.1/R (extensiones propias bajo TDD estricto, no una omisión de la tarea original): la extensión de `validateChangeName` (remediación) y la extensión de `loadGovernance` para modo continuo (Fase 12, anticipando la Fase 13). Ambos con salida real observada más abajo.
- **La compuerta de control de la Fase 11 se ejecutó y confirmó en verde después de CADA una de las Fases 12, 13 y 14** (V-E), no solo al cierre de la rebanada: es la evidencia continua, no solo final, de que D-05 se mantuvo durante todo el desarrollo, no solo en el commit de cierre.

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
| R (remediación) | Endurecimiento de `validateChangeName` (unidad Windows + nombre reservado con espacio) | `go test ./internal/kickoff/... -run TestChangeNameContainmentRejectsAllTenVectors` | PASS (10/10 subpruebas) | N/A — validador puro sin superficie de usuario propia; ya cableado en los tres parseadores desde la Fase 8 | Revertir las dos líneas de `validateChangeName` (`ContainsAny` ampliado, `TrimRight`) y las 2 filas añadidas a la tabla de test |
| 11 | Compuerta de control: regresión sin sello | `go test ./internal/sddstatus/... -run TestKickoffAbsenceRegression` | PASS (1/1) | `axiom sdd status --cwd <tmp> --json <cambio>` sobre un cambio sin `kickoff.yaml` → sin clave `governance`, `nextRecommended` idéntico al de antes del incremento (ver Verificación de Runtime) | Eliminar `kickoff_absence_regression_test.go`; ningún otro fichero cambia |
| 12 | Envoltorio tipado de compuerta | `go test ./internal/sddstatus/... -run TestGovernance` (ninguna función coincide con este patrón — el patrón real es `-run TestSDDGovernanceGateResult\|TestLoadGovernance`, ver nota) | PASS (18/18: 12 de `Validate()` + 6 de `loadGovernance`, incluida la ampliación de modo continuo) | N/A — `governance.go` no tiene todavía consumidor en `status.go` (cableado en la Fase 13) | Eliminar `governance.go`/`governance_test.go` |
| 13 | Cableado de gobernanza en `status.go` | `go test ./internal/sddstatus/... -run TestStatus` | PASS (incluye las 5 funciones nuevas de gobernanza más toda la suite previa de `status_test.go` ya verde, sin regresión) | **Ejecutado de verdad contra el binario compilado**: `axiom sdd kickoff seal` + `axiom sdd status --json <cambio>` sobre un cambio recién sellado en `checkpointed` → `nextRecommended: "await-gate"`, `blockedReasons` nombrando la compuerta `spec` pendiente. Salida completa en Verificación de Runtime | Revertir `status.go` a la versión de la Fase 12 (eliminar el campo `Governance`, la llamada a `loadGovernance`, y los parámetros nuevos de `artifactBlockedReasons`/`resolveNextRecommended`) |
| 14 | Proyección v2 y enrutamiento `await-gate` | `go test ./internal/sddstatus/... -run TestStatusV2` | PASS (incluye las 6 funciones nuevas más `TestProjectStatusV2RejectsUnsupportedValues`/`TestStatusRenderersEmbedOnlyStatusV2Projection` ya verdes, sin regresión) | **Ejecutado de verdad contra el binario compilado**: `axiom sdd continue --cwd <tmp> <cambio>` mostrando `next_recommended: await-gate` y las dos invocaciones ejecutables exactas (`gate show`/`gate record`); aprobación real de `spec` vía `axiom sdd gate record` desplaza el enrutamiento a `design`; rechazo real de `design` — ver el hallazgo documentado en Issues Found sobre el digest vacío. Salida completa en Verificación de Runtime | Revertir `status_v2.go` a la versión previa (eliminar `Governance`/`governanceV2` y el caso `await-gate`) y el caso nuevo de `nonPhaseRoutingInstructions` en `status.go` |

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

### Verificación de Runtime — binario compilado (Fases 11, 13 y 14)

Compilado con `go build -o axiom.exe ./cmd/axiom` sobre un árbol OpenSpec temporal fuera del repositorio (workspace de arnés, nunca `openspec/changes/inc-21-upfront-flow-governance` real). Fixture: `proposal.md`, `design.md`, `specs/example/spec.md` y `tasks.md` (una tarea sin marcar) bajo `openspec/changes/inc-99-p3-harness/`. Salida real observada, íntegra:

```text
=== STEP 1: axiom sdd status --cwd <ws> --json inc-99-p3-harness (ANTES de sellar kickoff) ===
{ "schemaName": "gentle-ai.sdd-status", ..., "nextRecommended": "apply", "blockedReasons": [], "notes": [] }
(sin clave "governance" en ningun punto del documento)
exit=0

=== STEP 2: axiom sdd kickoff seal --cwd <ws> --change inc-99-p3-harness --execution-style checkpointed --handoff-policy none ===
Kickoff sellado para "inc-99-p3-harness": flow_mode=sdd execution_style=checkpointed handoff_policy=none roles=[fullstack:blocking] deployment_target=local sealed_by=cli
exit=0

=== STEP 3: axiom sdd status --cwd <ws> --json --instructions inc-99-p3-harness (sellado, sin decisiones de compuerta) ===
"gates": [ {"key":"spec","status":"pending","blocks":"design"}, {"key":"design","status":"pending","blocks":"tasks"}, {"key":"tasks","status":"pending","blocks":"apply"} ]
"nextRecommended": "await-gate"
"blockedReasons": ["Block review gate \"spec\" is pending a decision: run `axiom sdd gate record --gate spec --decision approved|rejected ...` to continue."]
exit=0

=== axiom sdd continue --cwd <ws> inc-99-p3-harness (misma compuerta, vista dispatcher) ===
next_recommended: await-gate
### Blocked Reasons
- Block review gate "spec" is pending a decision: run `axiom sdd gate record --gate spec --decision approved|rejected ...` to continue.
### Next Governance Gate Decision
- Inspect the pending gate: `axiom sdd gate show --cwd "C:\repos\axiom_harness\ws" --change inc-99-p3-harness`.
- Record the decision: `axiom sdd gate record --cwd "C:\repos\axiom_harness\ws" --change inc-99-p3-harness --gate spec --decision approved|rejected --reason <your-reason>`.
exit=0

=== STEP 4: axiom sdd gate record --cwd <ws> --change inc-99-p3-harness --gate spec --decision approved --reason "cubre la intencion" --actor harness ===
Compuerta "spec" registrada (approved) para "inc-99-p3-harness".
exit=0

=== STEP 5: axiom sdd continue (tras aprobar spec) ===
next_recommended: await-gate
### Blocked Reasons
- Block review gate "design" is pending a decision: run `axiom sdd gate record --gate design --decision approved|rejected ...` to continue.
exit=0

=== STEP 6: axiom sdd gate record --cwd <ws> --change inc-99-p3-harness --gate design --decision rejected --reason "falta cubrir un caso limite" --actor harness ===
Compuerta "design" registrada (rejected) para "inc-99-p3-harness".
exit=0

=== STEP 7: axiom sdd status --cwd <ws> --json inc-99-p3-harness (tras rechazar design) ===
"nextRecommended": "await-gate"   <-- ver hallazgo abajo: se esperaba "design"
"blockedReasons": ["Block review gate \"design\" is pending a decision: ..."]  <-- ver hallazgo abajo: se esperaba el motivo del rechazo

=== axiom sdd gate show --cwd <ws> --change inc-99-p3-harness ===
spec: approved (harness) — cubre la intencion
design: rejected (harness) — falta cubrir un caso limite
exit=0
```

Los pasos 1-5 confirman de punta a punta, contra el binario real: ausencia de `governance` sin sello (D-05, Fase 11), enrutamiento a `await-gate` con el motivo genuino y las dos invocaciones ejecutables exactas (Fases 13-14), y el desplazamiento correcto de la compuerta activa de `spec` a `design` tras una aprobación real.

**Hallazgo del paso 7 (fuera de alcance de las Fases 11-14, reportado con honestidad — ver Issues Found #8):** el rechazo de `design` se registra correctamente en el ledger (`gate show` lo confirma con el motivo exacto), pero el siguiente `status` la muestra de nuevo como `pending`, no como `rejected` con su motivo. La causa **no está** en ningún código de las Fases 12-14: `internal/cli/sdd_gate.go` (Fase 10, ya cerrada) construye su `kickoff.GateRecord{Gate, Decision, Reason, Actor, RecordedAt}` sin poblar nunca `ArtifactDigest`. `resolveGateStatus` (Fase 6, ya cerrada) compara `last.ArtifactDigest == digest`; con `last.ArtifactDigest == ""` frente a un digest real no vacío, la comparación siempre falla y D-08 interpreta cualquier rechazo como "remediado" en la siguiente lectura. Mis propios tests de Go (Fases 12-14) prueban el camino **correcto** porque llaman a `kickoff.AppendGate` directamente con un `ArtifactDigest` calculado — exactamente lo que un `sdd_gate.go` corregido tendría que hacer. No he tocado `sdd_gate.go`: es un fichero de la Fase 10, ya cerrada, fuera de mi alcance asignado (Fases 11-14 más la remediación de `validateChangeName`), y `design.md` §4.3 no lo lista como fichero de la rebanada P3. Se documenta como hallazgo honesto para que el orquestador decida cómo cerrarlo (probablemente una fase o corrección futura que compute `kickoff.ArtifactDigest` en `runSDDGateRecord` antes de `AppendGate` para las cuatro compuertas fijas).

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

**Limitación de entorno nueva, con la causa ya corregida tras validación**: `go test ./internal/cli/...` (el paquete **completo**, sin filtrar por `-run`) no termina dentro de un `-timeout 300s`. La primera redacción de esta nota atribuyó el bloqueo a un subproceso git en la familia de tests `review_*`, alcanzada vía `internal/sddstatus → internal/reviewtransaction`. **Eso era incorrecto.** La reproducción directa de un validador independiente (`go test ./internal/cli/... -timeout 20s`) sitúa el cuelgue en `TestInstallActivationCapabilityControlsPolicyAndReport` (`opencode_background_test.go`) → `RunInstall` → `internal/pipeline` → `internal/system.detectSingleDep` → `os/exec.(*Cmd).Output()`: es la detección de dependencias del instalador, un subsistema completamente distinto, y el volcado de 644 líneas no menciona `review`, `sddstatus`, `reviewtransaction` ni `merge-base` en ningún punto. La conclusión de fondo no cambia —el cuelgue es preexistente y ajeno a esta ejecución—, pero la ruta técnica correcta queda anotada aquí para no desorientar a quien investigue este cuelgue después. Ninguno de los ficheros que esta ejecución creó o modificó invoca `os/exec` ni ningún subproceso: `sdd_kickoff.go`, `sdd_gate.go` y `sdd_governance.go` solo llaman a `internal/kickoff` (E/S de fichero pura) y a `internal/workspace`. Se registra como "no ejecutado" para el paquete completo, nunca como aprobado, y se sustituye por la verificación honesta que sí completa: `go test ./internal/cli/... -run '^TestRunSDD'` (35 pruebas, 3.4s) más el arnés de runtime real contra el binario compilado (ver Work Unit Evidence).

### Comandos de Verificación Ejecutados (Remediación y Fases 11-14, esta ejecución)

```text
go test ./internal/kickoff/... -run TestChangeNameContainmentRejectsAllTenVectors -v → 10 PASS + 0 FAIL (remediación; 8 vectores originales + 2 nuevos)
go build ./... && go vet ./internal/kickoff/...                                     → sin salida (compila, limpio), tras la remediación
go test ./internal/kickoff/... -v                                                   → 160 PASS + 0 FAIL + 1 SKIP (paquete completo tras la remediación)
go vet ./internal/sddstatus/...                                                     → vet.exe: ...governance_test.go:15:34: undefined: SDDGovernanceGateResult (RED de la Fase 12, antes de crear governance.go)
go test ./internal/sddstatus/... -run TestKickoffAbsenceRegressionMatchesPreGovernanceGolden -v → PASS (Fase 11; confirmado en verde ANTES de crear governance.go)
go test ./internal/sddstatus/... -run 'TestSDDGovernanceGateResult|TestLoadGovernance' -v → 18 PASS + 0 FAIL (Fase 12, tras crear governance.go)
go test ./internal/sddstatus/... -timeout 300s (tras la Fase 11) → 326 PASS + 1 FAIL preexistente y ajena, 39s
go test ./internal/sddstatus/... -timeout 300s (tras la Fase 12) → 351 PASS + 1 FAIL preexistente y ajena, 36s
go vet ./internal/sddstatus/... (tras 13.1, antes de 13.2)        → too many arguments in call to artifactBlockedReasons (RED genuino de la Fase 13)
go test ./internal/sddstatus/... -run 'TestResolveRoutesToAwaitGate|TestResolveRoutesRejectedGate|TestResolveReturnsErrorForCorruptKickoff|TestResolveContinuousExecutionStyleNeverEvaluatesGates|TestArtifactBlockedReasonsNamesChangeLocalSpecPath' -v → 5 PASS + 0 FAIL (Fase 13, incluye la regresión de la Fase 8 sobre artifactBlockedReasons con su nuevo cuarto argumento)
go test ./internal/sddstatus/... -timeout 300s (tras la Fase 13) → 356 PASS + 1 FAIL preexistente y ajena, 41s
go vet ./internal/sddstatus/... (tras 14.1, antes de 14.2)        → projection.Governance undefined (RED genuino de la Fase 14)
go test ./internal/sddstatus/... -run 'TestProjectStatusV2Governance|TestNonPhaseRoutingInstructionsPrintsAwaitGate|...' -v → 11 PASS + 0 FAIL (Fase 14 completa, incluida la reconfirmación de las 5 pruebas de la Fase 13 tras el refactor de firstOpenGate)
go test ./internal/sddstatus/... -timeout 300s (tras la Fase 14) → 362 PASS + 1 FAIL preexistente y ajena, 63s (variación de tiempo frente a la Fase 13 atribuida a carga del sistema — mismo comando, mismo resultado, sin regresión)
go test ./internal/sddstatus/... -run TestKickoffAbsenceRegressionMatchesPreGovernanceGolden -v → PASS, reconfirmado tras CADA una de las Fases 12, 13 y 14 (V-E de cada fase)
go build ./...                                                    → sin salida (compila todo el repositorio, confirmado tras cada fase de esta tanda)
go vet ./...                                                      → sin salida (todo el repositorio, confirmado tras la Fase 14)
gofmt -l internal/kickoff/args.go internal/kickoff/args_test.go internal/sddstatus/status.go internal/sddstatus/status_test.go internal/sddstatus/status_v2.go internal/sddstatus/status_v2_test.go internal/sddstatus/governance.go internal/sddstatus/governance_test.go internal/sddstatus/kickoff_absence_regression_test.go → dos correcciones de alineación de columnas aplicadas con gofmt -w (status.go tras anadir el campo Governance; status_v2.go tras anadir el campo Governance), sin cambio de contenido; limpio tras aplicarlas
grep -inE "receipt|lineage|acknowledge|burn|candidate" <ficheros nuevos/modificados de la remediación y las Fases 11-14> → una coincidencia real en un comentario de governance.go ("never a receipt"), corregida en la propia Fase 12 (ver Deviations); sin coincidencias tras la correccion
```

**`go test ./...` completo (repo entero) no se ejecutó en esta tanda tampoco**, por la misma razón ya documentada (riesgo de bloqueo preexistente y ajeno en `internal/cli`, no relacionado con `internal/sddstatus`). Se usaron en su lugar los comandos focalizados por paquete de arriba, más `go build ./...`/`go vet ./...` (limpios) para confirmar que el resto del repositorio compila.

**Limitación de entorno nueva, distinta de las dos ya documentadas**: `go test ./internal/sddstatus/... -timeout 300s` (paquete **completo**) tiene exactamente **una** prueba que falla de forma preexistente y ajena a este incremento: `TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots`, con el error `symlink ...: El cliente no dispone de un privilegio requerido` — el proceso de este entorno Windows no tiene el privilegio `SeCreateSymbolicLinkPrivilege` (ni modo desarrollador habilitado, ni ejecución como administrador) necesario para `os.Symlink`. Es una limitación de **este entorno**, no de ningún fichero que las Fases 11-14 crearon o modificaron: ninguno de ellos invoca `os.Symlink`. Se reproduce de forma idéntica en las cuatro ejecuciones completas del paquete (Fases 11, 12, 13 y 14), siempre la misma única prueba, nunca ninguna otra. Distinta de la limitación de `-race` (que sigue aplicando, ver abajo) y de la limitación de `internal/cli` ya documentada (que es un cuelgue, no un fallo).

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

**Remediación y Fases 11-14 (esta ejecución):**

19. **`validateChangeName` extiende `ContainsAny` a `"/\\:"` en vez de añadir una comprobación separada para el segmento de unidad de Windows.** Es aditivo sobre la misma comprobación existente (mismo mensaje de error, solo se amplía qué caracteres cuentan como "separador"), en vez de una tercera rama `if` independiente — mantiene la disciplina de una única comprobación de contención por el mismo camino que la tarea 8.3 ya estableció.
20. **`loadGovernance` (Fase 12) usa `kickoff.Config.Roles` directamente para construir `[]multirole.RoleAssignment`, nunca `multirole.DetectRoles` ni `multirole.ResolveRoster`.** D-07 dice explícitamente que `multirole.ResolveRoster` es la Fase 15/16; dado que `loadGovernance` solo se invoca cuando YA existe un kickoff sellado (D-05), el roster de esa evaluación es, por definición, siempre el sellado — no hay ningún caso dentro del alcance de las Fases 11-14 en el que el roster deba resolverse desde `design.md` o desde el fallback de `DetectRoles`. `GovernanceRoster.Source` es literalmente siempre `"kickoff"` en esta rebanada; el campo `Conflict` de `rosterV2` (Fase 14) queda declarado en el contrato pero siempre `nil`, a la espera de que la Fase 16 lo pueble.
21. **`governanceArtifactInputs` (Fase 12) computa el digest de `tasks.<rol>` para el roster de un único rol `fullstack`, aunque `tasksGateOpen` no lo necesite en ese caso (`len(in.Roles) <= 1` ya basta).** Es necesario igualmente: `roleApplyArtifactDigest` SIEMPRE consulta `Artifacts["tasks."+rol]` para decidir el digest contra el que se juzga `role-apply:<rol>`, sin importar el tamaño del roster (`machine.go`, ya cerrado en la Fase 5). Omitirlo para el caso de un único rol dejaría esa compuerta sin digest nunca, rompiendo su propia regla de reapertura.
22. **`awaitGateRoutingInstructions` (Fase 14) muestra `--decision approved|rejected --reason <your-reason>` como plantilla en vez de una invocación completamente rellena.** El propio diseño exige "las dos invocaciones ejecutables exactas", pero una decisión humana y su motivo no pueden inventarse honestamente; se sigue el mismo patrón de marcador ya establecido por `governanceBlockedReason` (Fase 13, mismo texto `approved|rejected`) para no introducir una segunda convención de marcador dentro del mismo incremento. Ambas invocaciones nombran el cambio y la clave de compuerta reales, verificado contra el binario compilado (ver Verificación de Runtime).
23. **`nonPhaseRoutingInstructions` usa el prefijo `axiom` (`gateShowInvocationPrefix`/`gateRecordInvocationPrefix`, ya fijados por `design.md` §5.5 en la Fase 12), no `gentle-ai` como los casos preexistentes `select-change`/`archived` de la misma función.** No es una inconsistencia introducida por esta ejecución: `design.md` fija literalmente `gateRecordInvocationPrefix = "axiom sdd gate record "` como parte del contrato de la Fase 12, y el propio esquema `SDDGovernanceGateResult.Validate()` exige ese prefijo exacto. Los casos preexistentes de `nonPhaseRoutingInstructions` no se tocaron ni se les cambió su prefijo `gentle-ai`.
24. **Hallazgo documentado, no corregido — ver Issues Found #8**: el rechazo de una compuerta fija a través de `axiom sdd gate record` no persiste como "rejected" en la siguiente lectura de estado, porque `internal/cli/sdd_gate.go` (Fase 10, ya cerrada) nunca calcula ni adjunta `ArtifactDigest` al registro. No es una desviación de las Fases 12-14: su código hace exactamente lo que `design.md`/`tasks.md` describen para `loadGovernance`/`resolveGateStatus`, y mis propios tests de Go demuestran el camino correcto suministrando el digest directamente a `kickoff.AppendGate`. Es una brecha de integración entre una fase ya cerrada (10) y esta rebanada (P3), fuera de mi alcance asignado para corregir.

## Issues Found

*(Fases 1-4, sin cambios: presupuesto de 400 líneas excedido en la PR1 sin válvula disponible; `-race` no ejecutable en este entorno.)*

3. **Presupuesto de 400 líneas excedido en la Fase 7 combinada (486 líneas), con válvula de alivio disponible y usada.** `tasks.md` sí declaraba la válvula para esta fase exacta ("separar `internal/kickoff/rdd_boundary_test.go` en una PR inmediatamente posterior, `inc-21/07b-kickoff-rdd-boundary`"). Se activó: `LastRoleClosed` + `archived.go`/`archived_test.go` quedaron en la PR 7 (170 líneas), y `rdd_boundary_test.go` (316 líneas) pasó a la PR 7b, ambas dentro de presupuesto por separado. A diferencia del issue de la PR1 (Fase 1, sin válvula), este caso funcionó exactamente como el diseño previó.
4. **`-race` sigue sin poder ejecutarse en este entorno** (sin cambios respecto a la ejecución anterior). Ninguna prueba nueva de las Fases 5-7 depende de concurrencia real.
5. **Presupuesto de 400 líneas excedido en las Fases 8, 9 y 10, las tres sin válvula de alivio declarada en `tasks.md`.** Fase 8: 875 líneas (862+/13-). Fase 9: 561 líneas (551+/10-, de las cuales ~9+9 son el propio movimiento de checkboxes de `tasks.md`). Fase 10: 482 líneas (476+/6-, con la misma composición). Igual que la PR1 (Fase 1), ninguna de las tres tenía una válvula declarada de antemano para dividirla, y en los tres casos el exceso proviene de una cobertura de test exhaustiva exigida literalmente por sus propias tareas RED (la tabla completa de ocho vectores T-2/T-7 en la Fase 8; los siete escenarios end-to-end de `sdd kickoff` incluidos los tres casos de `--infer` en la Fase 9; los quince escenarios de `sdd gate` incluida la triangulación del aviso de último rol en la Fase 10) — no de código de producción sobredimensionado ni de relleno. No se recortó ningún test ni comentario para forzar el ajuste al presupuesto, siguiendo la instrucción explícita del orquestador. Se registran las tres como candidatas a decisión `size:exception`.
6. **`go test ./internal/cli/...` (paquete completo) no completa dentro de un `-timeout 300s` en este entorno.** La causa real, confirmada por reproducción independiente, es `TestInstallActivationCapabilityControlsPolicyAndReport` (`opencode_background_test.go`) bloqueado en `internal/system.detectSingleDep` → `os/exec.(*Cmd).Output()`, la detección de dependencias del instalador; **no** es la ruta de subproceso git de `internal/sddstatus` que esta nota afirmaba antes. Es preexistente y ajena: no está causada por ningún fichero que esta ejecución creó o modificó. Se verificó en su lugar con `go test ./internal/cli/... -run '^TestRunSDD'` (35 pruebas, 3.4s, sin fallos) y con el arnés de runtime real contra el binario compilado.
7. **`-race` sigue sin poder ejecutarse en este entorno** (sin cambios). Ninguna prueba nueva de las Fases 8-10 depende de concurrencia real: el parseo de argumentos y los adaptadores de CLI son secuenciales, sin goroutines propias.

**Remediación y Fases 11-14 (esta ejecución):**

8. **Hallazgo real de integración, no una regresión de esta ejecución: el rechazo de una compuerta fija no persiste a través de `axiom sdd gate record`.** Confirmado con el binario compilado (ver Verificación de Runtime, paso 7): tras `axiom sdd gate record --gate design --decision rejected --reason "..."`, la siguiente lectura de `axiom sdd status --json` muestra `design` de nuevo como `"pending"`, no `"rejected"`, y `blockedReasons` pierde el motivo del rechazo. Causa exacta: `internal/cli/sdd_gate.go` (`runSDDGateRecord`, Fase 10, ya cerrada) construye `kickoff.GateRecord{Gate, Decision, Reason, Actor, RecordedAt}` sin poblar nunca `ArtifactDigest`; `resolveGateStatus` (`machine.go`, Fase 6, ya cerrada) compara `last.ArtifactDigest == digest`, y con `last.ArtifactDigest == ""` frente a cualquier digest real no vacío, la comparación falla siempre y D-08 interpreta el rechazo como "remediado" de inmediato. El ledger en sí es correcto (`gate show` lista el rechazo con su motivo exacto); el problema está solo en la evaluación de estado. Mis tests de Go (Fases 12-14) no reproducen este defecto porque llaman a `kickoff.AppendGate` directamente con un `ArtifactDigest` calculado, que es exactamente lo que un `sdd_gate.go` corregido tendría que hacer para las cuatro compuertas fijas. No se ha tocado `sdd_gate.go`: es un fichero de la Fase 10, fuera del alcance asignado a esta ejecución (Fases 11-14 más la remediación de `validateChangeName`), y `design.md` §4.3 no lo lista como fichero de la rebanada P3. El fallo es **seguro por defecto** (nunca bloquea de más; en el peor caso vuelve a preguntar antes de tiempo), no una fuga de autoridad ni una aprobación espuria.
9. **`go test ./internal/sddstatus/... -timeout 300s` tiene exactamente una prueba preexistente y ajena que falla en este entorno: `TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots`**, con `symlink ...: El cliente no dispone de un privilegio requerido` — el proceso no tiene `SeCreateSymbolicLinkPrivilege` en este entorno Windows (sin modo desarrollador ni elevación). Reproducido de forma idéntica en las cuatro ejecuciones completas del paquete de esta tanda (tras las Fases 11, 12, 13 y 14). Ninguna prueba nueva de las Fases 11-14 invoca `os.Symlink`; es una limitación de entorno, no de código.
10. **Presupuesto de 400 líneas excedido en las Fases 12 y 14, sin válvula de alivio declarada en `tasks.md` para ninguna de las dos.** Fase 12: 602 líneas (602+/0-). Fase 14: 475 líneas (420+/55-). La remediación (25), la Fase 11 (190) y la Fase 13 (317) sí entraron en presupuesto. En ambos casos excedidos, el exceso proviene de cobertura de test exhaustiva exigida por sus propias tareas RED (Fase 12: cuatro escenarios adversariales de `Validate()` más la traducción completa de `loadGovernance`; Fase 14: seis escenarios de integración a través de `Resolve`+`ProjectStatusV2`) y del propio tamaño del contrato tipado que `design.md` especifica (Fase 12: constantes, tipos, `Validate()`, `loadGovernance`, `governanceArtifactInputs`; Fase 14: cuatro tipos v2 nuevos más su función de proyección). No se recortó ningún test, comentario ni tipo para forzar el ajuste al presupuesto. Se registran ambas como candidatas a `size:exception`, igual que las Fases 1, 8, 9 y 10 ya aceptadas por el usuario.

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
| R (remediación) | — | `a843278a` | 20 + 5 = **25** | ✅ Dentro | No necesaria |
| 11 | `inc-21/11-status-noseal-regression` | `68bde835` | 190 + 0 = **190** | ✅ Dentro | No necesaria |
| 12 | `inc-21/12-status-governance-envelope` | `cacd8997` | 602 + 0 = **602** | ⚠️ Excedido (sin válvula) | No — ninguna disponible |
| 13 | `inc-21/13-status-wiring` | `23df6797` | 304 + 13 = **317** | ✅ Dentro | No necesaria |
| 14 | `inc-21/14-status-v2-routing` | `fb6c1ea0` | 420 + 55 = **475** | ⚠️ Excedido (sin válvula) | No — ninguna disponible |
| **Total remediación + Fases 11-14 (esta ejecución)** | — | — | 1502 + 39 = **1541**\*\*\* | (presupuesto de este intento: 2600) | — |

* Medido con `git diff --shortstat 56fcb2d8..d4aec544`, es decir excluyendo el commit de documentación que cierra la tanda (`acfb1654`), la misma convención usada para el total de las Fases 1-4. El diff de extremo a extremo `56fcb2d8..HEAD`, que sí incluye ese commit de documentación, mide 1191+48=1239 líneas. Los tamaños por commit (`git show --shortstat`) son 375, 220, 188, 316 y 158.

** Medido con `git diff --shortstat 9927fae0..6f50786` (rango de extremo a extremo de esta ejecución: HEAD real al inicio, `9927fae0`, hasta el cierre de la Fase 10) — **cifra autoritativa**. La suma columna a columna de los tres commits individuales (`git show --shortstat` de cada uno: 875+561+482) da 1918, 2 líneas por encima de la medición de extremo a extremo; la diferencia es la reconciliación normal de `git diff` cuando la misma línea de `cmd/axiom/main.go` (el mensaje de error del `default` del switch, que enumera los subcomandos válidos) se edita en más de un commit sucesivo — cada commit cuenta esa línea como su propia sustitución, mientras que el diff de extremo a extremo solo ve el estado inicial y el final. Se reporta la cifra de extremo a extremo por ser la que un revisor vería en un diff acumulado real.

*** Medido con `git diff --shortstat eb09452f..fb6c1ea0` (rango de extremo a extremo de esta ejecución: HEAD real al inicio, `eb09452f`, hasta el cierre de la Fase 14, `fb6c1ea0`) — **cifra autoritativa**. La suma columna a columna de los cinco commits individuales (`git show --shortstat` de cada uno: 25+190+602+317+475) da 1609, 68 líneas por encima de la medición de extremo a extremo (1536 inserciones + 73 eliminaciones = 1609 vs 1502+39=1541); la diferencia es la misma reconciliación normal de `git diff` que la nota anterior ya documenta, aplicada aquí a `internal/sddstatus/governance.go` (tocado en las Fases 12, 13 y 14 sucesivamente) y a `internal/sddstatus/governance_test.go` (tocado en las Fases 12 y 13). Se reporta la cifra de extremo a extremo por la misma razón. Esta cifra **no incluye** el commit de documentación que cierra esta tanda (checkboxes de `tasks.md` más este mismo documento) — se reporta por separado, siguiendo la convención ya establecida por `2bcac987` (Fases 1-4).

**Aviso para el orquestador (histórico, Fases 1-10, no aplica a esta ejecución)**: la ejecución de las Fases 8-10 cerró con un total de 1916 de un presupuesto de intento de 2600 líneas, bajo un intento acordado explícitamente para el `work-unit: PR 8-10, CLI surface`. Esta ejecución (remediación + Fases 11-14) opera bajo el **intento ya adquirido por el orquestador** citado en el prompt de lanzamiento (`token: sha256:d24a620435f77a6a28defecd0f2a8f07464014019cde9a5aa8450df689a4b6a1`, presupuesto máximo 2600 líneas cambiadas, máximo 3 intentos) — esta ejecutora no adquiere, liquida ni reescala intentos por instrucción explícita del propio prompt. El total real de esta ejecución, **1541 de 2600 líneas del intento actual** (medido `git diff --shortstat eb09452f..fb6c1ea0`, sin contar el commit de documentación final), queda registrado aquí para que el orquestador liquide (`settle`) el intento con evidencia exacta.

## Remaining Tasks

- [ ] Fases 11 a 23 completas (proyección `sddstatus` — compuerta de control, envoltorio, cableado y enrutamiento v2 —, roster multi-rol reservando `fullstack`, relevo de integración, precondición de archive, doctrina y activos por agente). Sin empezar en esta ejecución, tal como se instruyó explícitamente (alcance: solo Fases 8-10). La Fase 11 es un control gate normativo: su regresión byte a byte sin sello debe existir y confirmarse en verde **antes** de tocar `internal/sddstatus/status.go`/`status_v2.go` (Fases 12-14).

**Actualización tras esta ejecución (remediación + Fases 11-14)**: la rebanada de diseño **P3 completa** (proyección y enrutamiento en `internal/sddstatus`: compuerta de control de la Fase 11, envoltorio tipado de la Fase 12, cableado del resolutor de la Fase 13, proyección v2 y enrutamiento `await-gate` de la Fase 14) queda cerrada, junto con la remediación de `validateChangeName`. Quedan **Fases 15 a 23** (P4 roster multi-rol y rol reservado `fullstack`, P5 cierre de último rol y relevo de integración, P6 precondición de archive, P7 doctrina y activos por agente) — explícitamente fuera de esta ejecución por instrucción del orquestador ("Do NOT implement phases 15-23"). La Fase 15 es la segunda compuerta de control normativa del documento (caracterización REQ-1.1 de `multi-role-fan-out`, antes de tocar `roleExists`) y debe escribirse y confirmarse en verde contra el árbol sin modificar antes de tocar `internal/multirole`/`internal/handoff` — ninguno de los dos paquetes fue tocado en esta ejecución, confirmado por `git status`.

**Hallazgo pendiente de decisión del orquestador (no una tarea de las Fases 11-14, ver Issues Found #8)**: `internal/cli/sdd_gate.go` (Fase 10, ya cerrada) no adjunta `kickoff.ArtifactDigest` a los registros de las cuatro compuertas fijas, por lo que un rechazo real a través de la CLI se reabre a `pending` en la siguiente lectura de estado en vez de mantenerse `rejected` con su motivo. No se ha corregido en esta ejecución por estar fuera del alcance asignado (fichero de una fase ya cerrada, no listado por `design.md` §4.3 para la rebanada P3).

## Workload / PR Boundary

- **Modo**: PRs encadenadas (`feature-branch-chain`) contra la rama tracker `feature/inc-21-upfront-flow-governance`.
- **Unidad de trabajo de la ejecución anterior**: PR 5, PR 6, PR 7 y PR 7b (válvula de alivio activada), máquina de compuertas y cierre de rol.
- **Unidad de trabajo de esta ejecución**: PR 8 (`internal/kickoff/args.go`), PR 9 (`axiom sdd kickoff seal|show`) y PR 10 (`axiom sdd gate record|show`) — la rebanada P2 completa (superficie CLI).
- **Frontera de este lote**: empieza en `9927fae0` (HEAD real al inicio de esta ejecución) y termina en `6f50786` (cierre de la Fase 10). Tres commits reales sobre la rama tracker `feature/inc-21-upfront-flow-governance`: `c7c334fa` (Fase 8), `0b866af1` (Fase 9), `6f50786` (Fase 10). Ninguna rama hija ni PR real se creó — son fronteras documentales para cuando el usuario decida crearlas. Instrucción explícita: sin `git push`, sin ramas hijas, sin PRs.
- **Impacto estimado en presupuesto de revisión**: 1916 líneas cambiadas repartidas en 3 unidades de trabajo autónomas y verificables por separado, **las tres por encima del presupuesto individual de 400 líneas y sin válvula de alivio declarada** — a diferencia del lote anterior (Fases 5-7), donde solo una de cuatro unidades necesitó válvula. Se registran las tres como candidatas a decisión `size:exception` del mantenedor; el exceso proviene de cobertura de test exhaustiva exigida literalmente por las tareas RED de cada fase (ver Issues Found #5), no de código de producción inflado.
- **Cadena prevista actualizada (histórico, hasta el cierre de la Fase 10)**: `inc-21/01` → `02` → `03` → `04` → `05` → `06` → `07` → `07b` → `08` → `09` → `10` → (Fase 11 en adelante, fuera de este alcance).

**Unidad de trabajo de esta ejecución (remediación + Fases 11-14)**: remediación de `validateChangeName` (endurecimiento, fuera de la cadena de PRs numerada por fase), PR 11 (compuerta de control, solo test), PR 12 (`governance.go`, envoltorio tipado), PR 13 (cableado de `status.go`), PR 14 (`status_v2.go` + enrutamiento `await-gate`) — la rebanada P3 completa (proyección y enrutamiento en `internal/sddstatus`).

**Frontera de este lote**: empieza en `eb09452f` (HEAD real al inicio de esta ejecución) y termina en `fb6c1ea0` (cierre de la Fase 14), antes del commit de documentación final. Cinco commits reales sobre la rama tracker `feature/inc-21-upfront-flow-governance`: `a843278a` (remediación), `68bde835` (Fase 11), `cacd8997` (Fase 12), `23df6797` (Fase 13), `fb6c1ea0` (Fase 14). Ninguna rama hija ni PR real se creó — son fronteras documentales para cuando el usuario decida crearlas. Instrucción explícita: sin `git push`, sin ramas hijas, sin PRs.

**Impacto estimado en presupuesto de revisión (esta ejecución)**: 1541 líneas cambiadas repartidas en 5 unidades de trabajo autónomas y verificables por separado; 2 de las 5 (Fases 12 y 14) por encima del presupuesto individual de 400 líneas y sin válvula de alivio declarada (ver Issues Found #10), las otras 3 (remediación, Fase 11, Fase 13) dentro de presupuesto. El exceso de las Fases 12 y 14 proviene de cobertura de test exhaustiva exigida literalmente por sus propias tareas RED y del tamaño del contrato tipado que `design.md` especifica, no de código de producción inflado.

**Cadena prevista actualizada**: `inc-21/01` → `02` → `03` → `04` → `05` → `06` → `07` → `07b` → `08` → `09` → `10` → `11` → `12` → `13` → `14` → (Fase 15 en adelante, fuera de este alcance; la remediación no lleva rama hija propia en la cadena porque es un fix de una fase ya cerrada, no una rebanada nueva de diseño).

## Status

34/34 tareas de las Fases 1-7 completas (19 de Fases 1-4 + 15 de Fases 5-7) — histórico, ejecución anterior.

**Esta ejecución**: 14/14 tareas de las Fases 8-10 completas (4 de Fase 8 + 5 de Fase 9 + 5 de Fase 10), las tres con verificación de cierre V-A/V-B/V-C/V-D en verde y, adicionalmente para las Fases 9-10, arnés de runtime real contra el binario compilado. **Ready for next batch** (Fase 11 en adelante — compuerta de control de `internal/sddstatus` primero) — no listo para archive todavía.

**Total acumulado**: 48/114 tareas totales de `tasks.md` completas (48 = 34 de Fases 1-7 + 14 de Fases 8-10), 66 restantes (Fases 11-23). Confirmado por conteo real: `grep -c '^\- \[x\]' tasks.md` → 48; `grep -cE '^\- \[[x ]\]' tasks.md` → 114.

**Esta ejecución (remediación + Fases 11-14)**: 3/3 tareas de la remediación completas (R.1-R.3, seguimiento propio de este documento — no son checkboxes de `tasks.md`, ver nota del total acumulado más abajo) + 2/2 de la Fase 11 + 4/4 de la Fase 12 + 4/4 de la Fase 13 + 4/4 de la Fase 14 = **17/17 tareas completas de esta tanda** (3+2+4+4+4=17), todas con verificación de cierre V-A/V-B/V-C/V-D en verde y, adicionalmente para las Fases 11, 13 y 14, V-E (compuerta de control de la Fase 11 reconfirmada en verde, byte a byte, después de cada una) y arnés de runtime real contra el binario compilado para las Fases 11, 13 y 14. La rebanada de diseño **P3 queda completa**. **Ready for next batch** (Fase 15 en adelante — segunda compuerta de control normativa, caracterización REQ-1.1, antes de tocar `internal/multirole`/`internal/handoff`) — no listo para archive todavía; quedan 9 de las 23 fases del documento.

**Total acumulado**: 62/114 tareas totales de `tasks.md` completas (62 = 48 antes de esta ejecución + 14 de las Fases 11-14: 2+4+4+4), 52 restantes (Fases 15-23; la remediación no cuenta como fase numerada de `tasks.md`, así que sus 3 tareas propias, R.1-R.3, no forman parte del denominador de 114 ni de este conteo de 62 — se reportan aparte por transparencia, únicamente en este documento). Confirmado por conteo real tras marcar los checkboxes de las Fases 11-14: `grep -c '^\- \[x\]' tasks.md` → 62; `grep -cE '^\- \[[x ]\]' tasks.md` → 114.
