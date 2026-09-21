# Tareas: Determinación Temprana de Flujo, Compuertas de Revisión por Bloque, Relevo de Cierre de Roles y Ciclo de Vida de Archive (inc-21-upfront-flow-governance)

> **Fuentes de alcance:** `spec.md` (5 capacidades, REQ-21.1–REQ-21.18), `design.md` (14 decisiones D-01 a D-14, 7 rebanadas P1–P7, matriz de pruebas de §6, matriz de amenazas de §7), `proposal.md` como contexto de intención.
> **Precedente de forma:** `openspec/changes/inc-20-upstream-reconciliation/tasks.md`.
> **Idioma del artefacto:** español (castellano peninsular, tuteo profesional). Identificadores Go, rutas, banderas de CLI, claves YAML y nombres de test permanecen en inglés.
> **Decisión O-1 ya resuelta (no reabrir):** el retro-sellado de un cambio preexistente (REQ-21.4) congela el rol que `DetectRoles` ya infiere hoy para ese cambio (en este workspace, `core`, porque `design.md` existe y `axiom.yaml` declara `core`). El retro-sellado **nunca** introduce la identidad `fullstack`. Para cambios **nuevos** sin subdivisión de roles, REQ-21.6 exige `fullstack` obligatorio, y el diseño lo resuelve con D-06/D-07: `multirole.ResolveRoster` como reconciliador nuevo, `DetectRoles` intacto, `fullstack` como identidad reservada añadida a las **dos** copias de `roleExists` (`internal/multirole/detector.go` y `internal/handoff/validator.go`).
> **Frontera normativa (design.md §1.3, no se repite en cada tarea):** ninguna tarea de este documento invoca `axiom review *`, lee el modo RDD, ni usa los selectores `ReviewCore`/`OfferReviewAfterVerify`. Ninguna compuerta SDD de este incremento se llama, ni se comporta como, `receipt`, `lineage`, `candidate`, `acknowledge` o `burn`.

---

## Resumen ejecutivo del pronóstico (detalle completo al final del documento)

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High
```

Estimación agregada: **~5.300–8.000 líneas autoría (adiciones + eliminaciones)** repartidas en **23 PRs encadenadas** contra la rama tracker `feature/inc-21-upfront-flow-governance`. Ninguna rebanada se planifica por encima de ~450 líneas; las marcadas `Medium-High` llevan una "válvula de alivio" explícita (un sub-corte ya identificado) por si el diff real crece más de lo previsto. Tabla completa, por rebanada, en `## Review Workload Forecast` al cierre de este documento.

---

## Nota de partición de rebanadas (ajuste sobre el diseño)

El diseño mide **7 rebanadas** (P1–P7) y delega en `sdd-tasks` "el pronóstico de líneas y la partición definitiva" (implícito en su propio alcance de dominio puro sin coste de E/S). Contrastadas contra los ficheros que cada rebanada crea o modifica (`design.md` §4), tres de ellas agrupan responsabilidades independientes que juntas superan el presupuesto de 400 líneas:

| Rebanada del diseño | Por qué no cabe en una PR | Partición final en este documento |
|---|---|---|
| **P1** (`internal/kickoff`, dominio puro: tipos, esquema, sellado, retro-sellado, ledger, digest, máquina de compuertas, guarda de archivado) | Un paquete nuevo con 7 ficheros de producción y una máquina de estados con reglas de reapertura por digest — solo esa máquina, con sus tablas de casos, ronda las 300–420 líneas por sí sola | **Fase 1** (tipos/esquema) → **Fase 2** (sellado) → **Fase 3** (retro-sellado) → **Fase 4** (ledger+digest) → **Fase 5** (máquina, mecánica básica) → **Fase 6** (máquina, reapertura y multi-rol) → **Fase 7** (cierre de rol + guarda de archivado + frontera RDD del dominio) |
| **P2** (superficie CLI: `args.go`, `sdd_kickoff.go`, `sdd_gate.go`, `main.go`) | El parseo de argumentos (con sus vectores adversariales T-2/T-7) y los dos verbos nuevos son tres responsabilidades verificables por separado | **Fase 8** (parseo) → **Fase 9** (verbo `kickoff`, incluye `--infer`) → **Fase 10** (verbo `gate`) |
| **P3** (proyección y enrutamiento en `internal/sddstatus`) | Toca dos ficheros centrales del resolutor (`status.go`, `status_v2.go`) más un fichero de envoltorio nuevo (`governance.go`), y lleva su propia compuerta de control (la regresión sin sello) que debe existir **antes** de tocar nada | **Fase 11** (compuerta de control) → **Fase 12** (envoltorio) → **Fase 13** (`status.go`) → **Fase 14** (`status_v2.go` + enrutamiento) |
| **P4** (roster de roles) | Lleva su propia compuerta de control (caracterización REQ-1.1) más la reconciliación de roster, que son dos historias de verificación distintas | **Fase 15** (compuerta de control + rol reservado) → **Fase 16** (roster) |
| **P5** (cierre de rol y relevo) | La construcción del relevo y su cableado en el punto de aprobación son verificables por separado | **Fase 17** (construcción) → **Fase 18** (cableado) |
| **P6** (precondición de archive) | La fontanería git, la verificación de evidencia y el cableado en el CLI/resolutor son tres unidades independientes | **Fase 19** (fontanería) → **Fase 20** (verificación de evidencia) → **Fase 21** (cableado) |
| **P7** (doctrina y activos) | La sección compartida nueva (con su registro) es conceptualmente distinta de insertar el marcador en 12 activos por agente | **Fase 22** (sección compartida + doctrina ODD) → **Fase 23** (activos por agente + skills + documentación) |

Resultado: **23 rebanadas** en vez de 7. Ninguna tarea de este documento toca `internal/cli/review_*.go`, `internal/app/**`, `internal/tui/**`, `internal/dashboard/assets/**`, `openspec/config.yaml`, `openspec/changes/archive/**`, ni ningún fichero de `internal/reviewtransaction/**` distinto de `snapshot.go` (y en él, solo la adición de un método) — lista completa en «Reglas de Comprobación y Alcance».

---

## Convención de ramas (`feature-branch-chain`)

- **Rama tracker (borrador, sin merge directo a `main`):** `feature/inc-21-upfront-flow-governance`.
- **Ramas hijas**, una por fase, nombradas `inc-21/NN-slug` (`NN` = número de fase con cero a la izquierda). PR #1 (Fase 1) apunta a la rama tracker; cada PR siguiente apunta a la rama de la PR inmediatamente anterior. Solo la rama tracker se fusiona a `main`, y solo cuando las 23 fases estén revisadas e integradas en ella.
- Antes de fusionar cada PR: `gh pr edit <N> --base <rama-padre-correcta>` explícito (evita el defecto de PRs apiladas que INC-20 ya documentó: fusionar con `--delete-branch` una PR que es base de otra cierra la PR hija en vez de reapuntarla).

```
feature/inc-21-upfront-flow-governance (tracker, sin merge directo)
 └─ inc-21/01-kickoff-types-schema
     └─ inc-21/02-kickoff-seal
         └─ inc-21/03-kickoff-retro-seal
             └─ inc-21/04-kickoff-ledger-digest
                 └─ inc-21/05-kickoff-gate-machine-core
                     └─ inc-21/06-kickoff-gate-machine-reopen
                         └─ inc-21/07-kickoff-role-closure-archived-guard
                             └─ inc-21/08-cli-args
                                 └─ inc-21/09-cli-kickoff-verb
                                     └─ inc-21/10-cli-gate-verb
                                         └─ inc-21/11-status-noseal-regression
                                             └─ inc-21/12-status-governance-envelope
                                                 └─ inc-21/13-status-wiring
                                                     └─ inc-21/14-status-v2-routing
                                                         └─ inc-21/15-roster-req11-characterization
                                                             └─ inc-21/16-roster-resolve
                                                                 └─ inc-21/17-handoff-closure
                                                                     └─ inc-21/18-handoff-wiring
                                                                         └─ inc-21/19-git-ancestor
                                                                             └─ inc-21/20-integration-evidence
                                                                                 └─ inc-21/21-archive-precondition
                                                                                     └─ inc-21/22-doctrine-shared-section
                                                                                         └─ inc-21/23-doctrine-agent-assets
```

---

## Protocolo de compuertas de control (aplica a las Fases 11 y 15)

El diseño exige dos tests de caracterización escritos **antes** de tocar ningún fichero de producción de su rebanada. No son tests RED convencionales (no fallan al escribirse: capturan el comportamiento **vigente**, que debe seguir pasando) — son la red de seguridad que demuestra que INC-21 no altera el comportamiento de una capacidad ya viva.

1. **Compuerta de control de la Fase 11 (P3):** captura byte a byte el `StatusV2Projection` que produce hoy un cambio **sin** `kickoff.yaml`. Se escribe y se confirma en verde **contra el árbol sin modificar**, antes de crear `governance.go` o tocar `status.go`/`status_v2.go`. Cada fase posterior de P3 (12, 13, 14) debe re-ejecutar esta compuerta y confirmarla en verde antes de cerrar.
2. **Compuerta de control de la Fase 15 (P4):** captura los tres escenarios vivos de `multi-role-fan-out-engine` REQ-1.1 (multi-rol con políticas distintas, rol ausente de `axiom.yaml` ⇒ error, fallback de rol único) sobre `DetectRoles`, sin tocar `roleExists`. Se confirma en verde contra el árbol sin modificar, y de nuevo tras añadir la identidad reservada `fullstack`.

Ninguna tarea de implementación de P3 o P4 se considera cerrada si su compuerta de control correspondiente no está en verde.

---

## Reglas de Comprobación y Alcance (aplican a TODAS las fases)

1. **Rutas prohibidas (criterio de aborto, design.md §4.8):** `internal/cli/review_*.go`, `internal/app/**`, `internal/tui/**`, `internal/dashboard/assets/**`, `openspec/config.yaml`, `openspec/changes/archive/**`, y cualquier fichero de `internal/reviewtransaction/**` distinto de `snapshot.go` (en el que solo se admite la adición del método de la Fase 19). Un diff que toque cualquiera de estas rutas se rechaza y se revierte hasta el último estado verde.
2. **Vocabulario prohibido (T-9, design.md §1.3 regla 2):** ningún fichero de producción de este incremento declara un campo, tipo o mensaje llamado `receipt`, `lineage`, `candidate`, `acknowledge` o `burn`; ninguno invoca `ReviewCore` ni `OfferReviewAfterVerify`.
3. **Filas de la matriz de amenazas marcadas `N/A` (T-1, T-4, T-5) no generan tarea** — INC-21 no clasifica ficheros por contenido, no interactúa con `push`/remotos, y no invoca `gh` ni automatización de PR (design.md §7.1).
4. **Sin operaciones git remotas.** Ninguna tarea de este documento ejecuta `git fetch`, `git ls-remote`, `git push` ni `gh` sobre ningún repositorio. La verificación de ancestro (Fase 19) es local, sobre el grafo ya presente.
5. **Protocolo de verificación por fase (V-A a V-F), referenciado por número en cada "Verificación de cierre":**
   - **V-A** — `go build ./...` y `go vet ./...` en verde.
   - **V-B** — `go test ./<paquetes tocados por la fase>/... -timeout 300s` en verde.
   - **V-C** — `gofmt -l` sobre los ficheros tocados por la fase, sin salida.
   - **V-D** — El diff no toca ninguna ruta prohibida (regla 1) ni introduce vocabulario prohibido (regla 2).
   - **V-E** — Cuando la fase pertenece a P3 o P4: la compuerta de control correspondiente (Fase 11 o Fase 15) sigue en verde.
   - **V-F** — `go test ./...` completo con `-timeout 900s` se ejecuta **una vez, al cerrar cada PR**, nunca como criterio de una tarea intermedia. `internal/sddstatus/...` se ejecuta **aparte**, con presupuesto propio de `-timeout 600s` (design.md §6.1 documenta el riesgo de bloqueo de esa ruta vía subproceso git); si agota el presupuesto, se registra como "no ejecutado", nunca como aprobado. Todo test nuevo de este incremento que dependa de git es hermético (`t.TempDir()` + `git init` local, sin red).
6. **Marcado `(read-only)`:** cuando una tarea cita como referencia un fichero que esa fase no crea ni modifica, la ruta se marca `(read-only)` inmediatamente después de la ruta entre comillas invertidas.
7. **Granularidad RED/GREEN:** dado que este documento se escribe antes de implementar (a diferencia del precedente de INC-20, redactado a posteriori), las tareas RED agrupan la tabla de casos de un mismo fichero/concern en vez de una tarea por función individual. La disciplina `strict_tdd: true` se mantiene: ninguna tarea GREEN se ejecuta sin que su RED (o su compuerta de control) se haya observado primero.
8. **Etiquetas de PR.** Toda PR de este documento necesita `type:feat` o `type:test` según corresponda, más `area:sdd-governance`.

---

## Fase 1: P1a — Tipos y esquema del dominio `kickoff` (REQ-21.5, REQ-21.6; D-01, D-02)

Aditivo puro, sin consumidores todavía. Paquete nuevo `internal/kickoff`.

- [x] 1.1 [RED] Escribir `internal/kickoff/schema_test.go`: tabla de casos para `Validate()` sobre `flow_mode`, `execution_style`, `handoff_policy`, `gate_policy`, `deployment_target`, `evidence_kind` — valores válidos aceptados, valores desconocidos rechazados con error nombrado, `schema:` desconocido rechazado, documento YAML malformado rechazado (nunca valor cero silencioso). Los tipos/funciones no existen: RED por fallo de compilación.
- [x] 1.2 [GREEN] Crear `internal/kickoff/types.go`: `Kickoff`, `FlowMode`, `ExecutionStyle`, `HandoffPolicy`, `Lifecycle`, `Gate`, `GateKey`, `GateDecision`, `EvidenceKind`, `GateRecord`, `GateLedger`, `GateState`, constantes de enum (`FlowODD`, `FlowSDD`, `ExecutionContinuous`, `ExecutionCheckpointed`, `HandoffNone`, `HandoffPerCheckpoint`, `GateSpec`, `GateDesign`, `GateTasks`, `GateIntegration`, `DecisionApproved`, `DecisionRejected`, `EvidencePRMerged`, `EvidenceDeployment`, `EvidenceAttestation`) y `RoleApplyGate(role string) GateKey` como única forma de construir la clave `role-apply:<rol>`.
- [x] 1.3 [GREEN] Crear `internal/kickoff/schema.go`: etiquetas YAML de `Kickoff`/`Lifecycle`, listas de valores admitidos por enum, y `Validate() error` que satisface la tabla de 1.1.
- [x] 1.4 [REFACTOR] Revisar `types.go`/`schema.go`: comentarios GoDoc en inglés, sin duplicación entre las listas de enum y las constantes declaradas.
- [x] 1.5 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 2: P1b — Sellado de kickoff, escritura única (REQ-21.5; D-01, D-02, D-04)

Depende de la Fase 1.

- [x] 2.1 [RED] Escribir `internal/kickoff/seal_test.go`: primer sellado escribe y devuelve el valor sellado; segundo sellado con contenido **distinto** no escribe y devuelve el ganador ya escrito; fichero preexistente ilegible ⇒ error nombrado; el fichero temporal no sobrevive al retorno (ni en éxito ni en colisión). Usar `t.TempDir()`.
- [x] 2.2 [GREEN] Crear `internal/kickoff/seal.go`: `Seal(changeRoot string, k Kickoff) (Kickoff, bool, error)` usando `reviewtransaction.PublishFileNoReplace` (`internal/reviewtransaction/store.go:913`, `(read-only)`) y relectura del ganador ante `os.ErrExist`, calcado del patrón de `ensureChangeInstanceMarker` (`internal/sddstatus/edit_authority_consent.go:94-125`, `(read-only)`). `Load(changeRoot string) (*Kickoff, error)`: ausencia de fichero ⇒ `nil, nil`, nunca error.
- [x] 2.3 [REFACTOR] Confirmar que `Seal` no dependa de estado global ni de reloj salvo `sealed_at`, inyectable para test.
- [x] 2.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 3: P1c — Regla de retro-sellado (REQ-21.4; D-01 a D-04; §8.1 del diseño; O-1 ya resuelta)

Depende de la Fase 2. Implementa la regla corregida de §8.1, **no** la redacción original de `spec.md:102` (superada por O-1).

- [x] 3.1 [RED] Escribir en `internal/kickoff/seal_test.go` (o `infer_test.go`) la tabla de los tres casos de retro-sellado: (a) existe `design.md` y `multirole.DetectRoles` devuelve roles sin error ⇒ se infieren **esos** roles, `execution_style: continuous`, `handoff_policy: none`, `execution_style_source: inferred`, `sealed_by: inferred`; (b) no existe `design.md` ⇒ se infiere `fullstack:blocking` con `tasks.md`/`verify-report.md`; (c) `DetectRoles` devuelve error (rol declarado ausente de `axiom.yaml`) ⇒ no se infiere nada, se devuelve una nota no bloqueante y el error de `DetectRoles` se propaga intacto. **Ninguno de los tres casos infiere `fullstack` cuando `DetectRoles` ya resuelve un rol sin error** — es la aserción que fija O-1.
- [x] 3.2 [GREEN] Añadir a `internal/kickoff/seal.go` (o crear `internal/kickoff/infer.go`): `InferKickoff(designPath string, wsConfig *workspace.WorkspaceConfig) (Kickoff, string, error)` (el `string` es la nota no bloqueante) implementando los tres casos de 3.1. Importa `internal/multirole` (`(read-only)` en esta tarea: `DetectRoles` no se modifica aquí) solo para invocar `DetectRoles`, sin tocar su firma ni su cuerpo.
- [x] 3.3 [REFACTOR] Confirmar que `InferKickoff` es una función pura respecto a E/S salvo la lectura ya encapsulada en `DetectRoles`; ningún camino de `InferKickoff` escribe `kickoff.yaml` (eso lo hace el llamador vía `Seal`, en la Fase 9).
- [x] 3.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 4: P1d — Ledger de compuertas y digest de artefactos (REQ-21.12; D-02, D-08)

Depende de la Fase 1. Independiente de las Fases 2–3.

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar `digest.go`/`digest_test.go` en una PR inmediatamente posterior (`inc-21/04b-kickoff-digest`), reapuntando la Fase 5 a esa PR en vez de a esta.

- [ ] 4.1 [RED] Escribir `internal/kickoff/ledger_test.go`: anexado preserva registros previos; escritura concurrente desde dos *goroutines* (`sync.WaitGroup`, ejecutar con `-race`) no pierde ningún registro; fichero ausente ⇒ `LoadGates` devuelve ledger vacío sin error.
- [ ] 4.2 [GREEN] Crear `internal/kickoff/ledger.go`: `AppendGate(changeRoot string, rec GateRecord) error` usando `reviewtransaction.AcquireAuthorityFileLock` (`store_lock.go:54`, `(read-only)`) + `ReplaceFileAtomic` (`store.go:918`, `(read-only)`) + `SyncReviewDirectory` (`store.go:93`, `(read-only)`); `LoadGates(changeRoot string) (GateLedger, error)`.
- [ ] 4.3 [RED] Escribir `internal/kickoff/digest_test.go`: mismo contenido con CRLF y con LF ⇒ mismo digest; orden de las rutas de entrada no altera el resultado; fichero ausente en la lista ⇒ error nombrado.
- [ ] 4.4 [GREEN] Crear `internal/kickoff/digest.go`: `ArtifactDigest(paths []string) (string, error)` — SHA-256 estable sobre contenido normalizado (CRLF→LF), en orden de ruta.
- [ ] 4.5 [REFACTOR] Revisar `ledger.go`/`digest.go`: sin duplicación de la lógica de normalización de fin de línea si ambos la necesitaran; comentarios GoDoc en inglés.
- [ ] 4.6 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 5: P1e — Máquina de compuertas, mecánica básica (REQ-21.7, REQ-21.8, REQ-21.9, REQ-21.10; D-05, D-08, D-09)

Depende de las Fases 1 y 4 (usa `GateLedger`). Implementa el subconjunto **no** condicionado por digest: modo continuo, apertura por artefacto, orden de evaluación fijo.

- [ ] 5.1 [RED] Escribir `internal/kickoff/machine_test.go`: `Inputs{Execution: ExecutionContinuous}` ⇒ `EvaluateGates` devuelve slice vacío sin error, **sin leer `Ledger` ni `RolePending`** (D-05: el modo continuo tiene coste cero); con `ExecutionCheckpointed`, un artefacto con `Artifacts["spec"] == ""` (aún no `done`) no abre la compuerta `spec`; un artefacto con digest presente abre la compuerta correspondiente en estado `pending`; el orden de evaluación es siempre `spec → design → tasks → role-apply:<rol1> … role-apply:<roln> → integration`, verificado con una tabla que declara los roles en orden distinto y confirma que la salida respeta el orden fijo, no el de declaración.
- [ ] 5.2 [GREEN] Crear `internal/kickoff/machine.go` con el esqueleto de `EvaluateGates(in Inputs) ([]GateState, error)`: corte inmediato en modo continuo (D-05); apertura de `spec`/`design`/`tasks`/`integration` por presencia de artefacto; orden de evaluación fijo. Definir `type Inputs struct { Execution ExecutionStyle; Roles []multirole.RoleAssignment; Artifacts map[string]string; RolePending map[string]int; VerifyFound bool; Ledger GateLedger }` (`internal/multirole` en `(read-only)` para esta tarea: solo se consume el tipo `RoleAssignment`, ya existente).
- [ ] 5.3 [REFACTOR] Extraer el orden fijo de compuertas a una tabla de constantes reutilizable por la Fase 6.
- [ ] 5.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 6: P1f — Máquina de compuertas, reapertura por digest y multi-rol (REQ-21.10, REQ-21.11, REQ-21.12; D-08 — normativo)

Depende de la Fase 5.

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar la generación de una `role-apply:<rol>` por rol del roster en una PR inmediatamente posterior (`inc-21/06b-kickoff-gate-machine-roleapply`).

- [ ] 6.1 [RED] Ampliar `internal/kickoff/machine_test.go`: `rejected` + digest actual **igual** al registrado ⇒ sigue `rejected`, `Reopened: false`; `rejected` + digest actual **distinto** ⇒ vuelve a `pending`, `Reopened: true` (REQ-21.12 sin verbo `reopen`); **`approved` no se invalida jamás al cambiar el digest** — caso explícito con digest distinto tras aprobación, la compuerta sigue `approved` (regla normativa de D-08, la más sensible de este incremento); roster multi-rol (`core`, `web`, `qa`) ⇒ se genera exactamente una compuerta `role-apply:<rol>` por rol, con la clave construida únicamente vía `RoleApplyGate`.
- [ ] 6.2 [GREEN] Completar `internal/kickoff/machine.go`: lógica de comparación de digest contra `Ledger` para decidir `pending`/`approved`/`rejected`/`Reopened`; generación de una `GateState` por cada `RoleApplyGate(rol)` del roster de `Inputs.Roles`, bloqueando `apply` del rol y el aviso de último rol mientras no esté `approved`.
- [ ] 6.3 [REFACTOR] Confirmar que la función completa sigue siendo pura (sin E/S, sin reloj) y que el criterio "`approved` es terminal" está expresado en un único punto del código, no repetido por cada clave de compuerta.
- [ ] 6.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 7: P1g — Cierre de último rol, guarda de raíz archivada y frontera RDD del dominio (REQ-21.13, REQ-21.18; D-10, D-14; T-9 parcial)

Depende de la Fase 6.

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar `internal/kickoff/rdd_boundary_test.go` en una PR inmediatamente posterior (`inc-21/07b-kickoff-rdd-boundary`).

- [ ] 7.1 [RED] Ampliar `internal/kickoff/machine_test.go`: `LastRoleClosed(roster, gates)` — N−1 de N roles con `role-apply` aprobada ⇒ `false`; los N aprobados ⇒ `true`; un rol rechazado ⇒ `false` aunque el resto esté aprobado (REQ-21.13, tercer escenario); roster de un único rol `fullstack` aprobado ⇒ `true` de inmediato.
- [ ] 7.2 [GREEN] Añadir `LastRoleClosed(roster multirole.RoleAssignment_o_similar, gates []GateState) bool` a `internal/kickoff/machine.go`, reutilizando `EvaluateGates` o su tabla de estados ya calculada.
- [ ] 7.3 [RED] Escribir `internal/kickoff/archived_test.go`: `openspec/changes/archive/2026-01-01-x` ⇒ rechazo; `openspec/changes/x` ⇒ aceptación; `../otro` y ruta absoluta fuera del workspace ⇒ rechazo por contención (T-7 parcial, vector "raíz de cambio").
- [ ] 7.4 [GREEN] Crear `internal/kickoff/archived.go`: `RefuseArchivedRoot(workspaceRoot, changeRoot string) error` con contención vía `filepath.Rel` (sin `..`, no absoluta).
- [ ] 7.5 [RED] Escribir `internal/kickoff/rdd_boundary_test.go`, calcado de `internal/sddstatus/review_offer_absence_guard_test.go` (`(read-only)`): escáner `go/ast` sobre `internal/kickoff/*.go` de producción que falla si aparece cualquiera de los cinco términos prohibidos (`receipt`, `lineage`, `candidate`, `acknowledge`, `burn`) o los selectores `ReviewCore`/`OfferReviewAfterVerify`; y un test de import que falla si `internal/kickoff` importa algo fuera de `internal/multirole`, `internal/reviewtransaction`, la librería estándar y `gopkg.in/yaml.v3`.
- [ ] 7.6 [GREEN] Confirmar que el escáner de 7.5 pasa sobre el estado actual del paquete (no requiere cambios de producción si las Fases 1–6 ya respetaron la frontera; si el escáner encuentra una violación, corregirla aquí).
- [ ] 7.7 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 8: P2a — Parseo de argumentos CLI (REQ-21.5, REQ-21.10, REQ-21.12; T-2, T-7)

Depende de la Fase 1 (usa los tipos de enum para validar banderas).

- [ ] 8.1 [RED] Escribir `internal/kickoff/args_test.go`: banderas conocidas y desconocidas para `seal`/`gate record`/`show`; `--decision rejected` sin `--reason` ⇒ error (REQ-21.12 exige motivo registrado); `--from-session-pace` y `--execution-style` explícitos a la vez ⇒ el explícito gana y se registra como sobrescritura deliberada (D-03); `role-apply:` sin nombre de rol ⇒ error; `--cwd` relativo, absoluto e inexistente; `--change` con `..`, `/`, `\`, ruta absoluta, nombres reservados de Windows (`con`, `nul`), cadena vacía y una cadena de 300 caracteres ⇒ rechazo por contención antes de construir ninguna ruta de escritura (T-2, T-7 — vectores completos de la tabla de ocho).
- [ ] 8.2 [GREEN] Crear `internal/kickoff/args.go`: `ParseSealArgs`, `ParseGateRecordArgs`, `ParseShowArgs`, parseo puro sin `io.Writer`, siguiendo el precedente de `sddstatus.ParseCommandArgs` (`internal/sddstatus/status.go:225`, `(read-only)`). El nombre del cambio se resuelve contra los directorios **ya existentes** bajo `openspec/changes/` — nunca se usa para crear un directorio nuevo.
- [ ] 8.3 [REFACTOR] Confirmar que los tres parseadores comparten la validación de contención de `--change`/`--cwd` sin duplicar la comprobación de `filepath.Rel`.
- [ ] 8.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 9: P2b — Verbo `axiom sdd kickoff` (REQ-21.4, REQ-21.5, REQ-21.14 parcial; D-04; T-8)

Depende de las Fases 2, 3 y 8.

- [ ] 9.1 [RED] Escribir `internal/cli/sdd_kickoff_test.go` (`bytes.Buffer` como `stdout`, al estilo de `internal/cli/sdd_archive_compose_test.go` `(read-only)`): `seal` correcto ⇒ salida 0 con resumen; `seal` sobre un cambio ya sellado ⇒ salida 0 con la configuración ganadora, **sin volver a escribir**; `seal --infer` sobre los tres casos de la Fase 3; sellado sobre una raíz bajo `archive/` ⇒ salida 1 nombrando la vía de bug/nuevo incremento (D-14); subverbo desconocido de `kickoff` ⇒ salida 1; `kickoff --help`/`kickoff -h` ⇒ salida 0 (T-8); `show` sobre un cambio sin sellar ⇒ salida explícita de "sin kickoff".
- [ ] 9.2 [GREEN] Crear `internal/cli/sdd_kickoff.go`: `RunSDDKickoff(args []string, stdout io.Writer) error` con subverbos `seal` (incluida la rama `--infer` que invoca `kickoff.InferKickoff` antes de `kickoff.Seal`) y `show`, adaptador fino al estilo de `internal/cli/sdd_status.go:12-39` (`(read-only)`). Antes de escribir, invoca `kickoff.RefuseArchivedRoot`.
- [ ] 9.3 [GREEN] Modificar `cmd/axiom/main.go`: un `case "kickoff"` nuevo en `runSDD` (`:1746-1762`) y una línea de ayuda (`:1730-1735`), calcados del patrón de los `case` ya existentes en ese `switch`. Confirmar antes de escribir que no existe ya ningún `case "kickoff"`.
- [ ] 9.4 [REFACTOR] Confirmar que `RunSDDKickoff` no contiene lógica de decisión (esa vive en `internal/kickoff`); es puro adaptador de E/S y formato.
- [ ] 9.5 [Verificación de cierre] V-A, V-B (`./internal/kickoff/... ./internal/cli/... ./cmd/axiom/...`), V-C, V-D.

## Fase 10: P2c — Verbo `axiom sdd gate` (REQ-21.10, REQ-21.11, REQ-21.12; T-8)

Depende de las Fases 4, 6, 7 y 8. Independiente de la Fase 9 salvo por compartir `main.go`.

- [ ] 10.1 [RED] Escribir `internal/cli/sdd_gate_test.go`: `record --decision approved` correcto ⇒ salida 0, registro anexado; `record --decision rejected` sin `--reason` ⇒ salida 1; `record --gate` con clave desconocida ⇒ salida 1 enumerando el vocabulario válido; `record --gate role-apply:<rol>` fuera del roster sellado ⇒ salida 1 nombrando el roster; registro sobre una raíz bajo `archive/` ⇒ salida 1; `gate --help`/subverbo desconocido ⇒ salidas 0/1 respectivamente (T-8); el aviso formal de último rol aparece **exactamente una vez** cuando la última `role-apply` se aprueba (con roster de un único rol `fullstack`, ver Fase 18 para el contenido completo del aviso — aquí solo se verifica el enrutamiento del verbo, no la construcción del relevo).
- [ ] 10.2 [GREEN] Crear `internal/cli/sdd_gate.go`: `RunSDDGate(args []string, stdout io.Writer) error` con subverbos `record` y `show`, invocando `kickoff.RefuseArchivedRoot`, `kickoff.AppendGate` y `kickoff.EvaluateGates`/`LastRoleClosed` para decidir si emitir el aviso de último rol (el cuerpo del relevo se completa en la Fase 18; aquí se deja el punto de extensión).
- [ ] 10.3 [GREEN] Modificar `cmd/axiom/main.go`: un `case "gate"` nuevo en `runSDD` y su línea de ayuda, mismo patrón que la Fase 9.
- [ ] 10.4 [REFACTOR] Confirmar que `sdd_kickoff.go` y `sdd_gate.go` comparten la comprobación de raíz archivada sin duplicar código (extraer un helper común si hiciera falta).
- [ ] 10.5 [Verificación de cierre] V-A, V-B (`./internal/kickoff/... ./internal/cli/... ./cmd/axiom/...`), V-C, V-D.

## Fase 11: P3a — Compuerta de control: regresión byte a byte sin sello (REQ-21.7; D-05 — compuerta de control nº 1)

**No depende de ninguna fase anterior de P1/P2.** Se escribe contra el árbol tal y como está hoy, antes de tocar `internal/sddstatus`. Ver «Protocolo de compuertas de control».

- [ ] 11.1 [Caracterización] Capturar la salida JSON completa de `StatusV2Projection` para un cambio **sin** `kickoff.yaml` (usar uno de los cambios activos ya existentes en `openspec/changes/` sobre un árbol temporal, o un fixture equivalente) y guardarla como comparación byte a byte en un test nuevo, `internal/sddstatus/kickoff_absence_regression_test.go`. Ejecutar y confirmar que **pasa hoy, sin ningún cambio de producción** — esta confirmación es la evidencia de que la compuerta de control es honesta antes de que exista nada que proteger.
- [ ] 11.2 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`, con el presupuesto de la regla 5), V-C, V-D.

## Fase 12: P3b — Envoltorio tipado de compuerta (REQ-21.8, REQ-21.9, REQ-21.10, REQ-21.11; D-08; T-9)

Depende de la Fase 11 (la compuerta de control ya existe) y de la Fase 6 (usa `kickoff.GateState`).

- [ ] 12.1 [RED] Escribir `internal/sddstatus/governance_test.go`: `Validate()` del envoltorio rechaza un esquema ajeno a `gentle-ai.sdd-governance.gate/v1`; rechaza cualquier conjunto de `Choices` distinto de exactamente `approved`/`rejected` en ese orden (calcado de `internal/consentenvelope/envelope.go:61-65`, `(read-only)`); rechaza una invocación que no empiece por `axiom sdd gate record ` — **incluida explícitamente una invocación con prefijo `sdd-attempt`**, que es el caso adversarial nombrado por T-9; acepta las cinco filas de criterios de `design.md` §5.5 (`spec`, `design`, `tasks`, `role-apply:<rol>`, `integration`) tal cual.
- [ ] 12.2 [GREEN] Crear `internal/sddstatus/governance.go`: `SDDGovernanceGateResult` y su `Validate()`, construido sobre `consentenvelope.Core` (`internal/consentenvelope/envelope.go:42-51`, `(read-only)`), con las constantes `SDDGovernanceGateSchema`, `SDDGovernanceContractV1`, `gateOperation`, `gateRecordInvocationPrefix`; `loadGovernance(changeRoot string) (*Governance, error)` que traduce `[]kickoff.GateState` a la vista de estado.
- [ ] 12.3 [REFACTOR] Confirmar que ningún campo de `SDDGovernanceGateResult` usa un nombre del vocabulario prohibido (regla 2 de alcance).
- [ ] 12.4 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`), V-C, V-D, V-E (Fase 11 sigue en verde, sin tocar todavía `status.go`).

## Fase 13: P3c — Cableado de gobernanza en el resolutor (REQ-21.4 parcial, REQ-21.7, REQ-21.16 parcial; D-05, D-09, D-13; T-10)

Depende de la Fase 12.

- [ ] 13.1 [RED] Ampliar `internal/sddstatus/status_test.go`: con `kickoff.yaml` sellado en `checkpointed` y una compuerta `pending`, `resolveNextRecommended` (`:1456-1483`, `(read-only)` en esta tarea — se modifica en 13.2) debe producir el valor `await-gate` una vez exista (introducido en 13.2); `artifactBlockedReasons` (`:1380-1402`) incluye el motivo genuino de la compuerta pendiente; con una compuerta `rejected` y digest sin cambios, `nextRecommended` vuelve a la fase dueña del artefacto rechazado con el motivo exacto registrado; un `kickoff.yaml` **ilegible** (YAML corrupto) produce un error del resolutor, **nunca** se degrada en silencio a "sin sello" (T-10, caso explícito); `dependencies.Archive` permanece con su lógica actual (`:1443-1445`) cuando no hay compuerta `integration` sellada todavía (la condición completa llega en la Fase 21).
- [ ] 13.2 [GREEN] Modificar `internal/sddstatus/status.go`: añadir el campo `Governance *Governance` a `Status` (`:151-188`); invocar `loadGovernance` en el resolutor OpenSpec/híbrido **solo cuando existe `kickoff.yaml` y `execution_style != continuous`** (D-05, coste cero para lo existente); nuevos motivos genuinos en `artifactBlockedReasons`; `resolveNextRecommended`/`resolveDependencies` (`:1427-1447`) consultan gobernanza solo con sello; `engramTitlePattern` (`:860`) admite los sufijos `kickoff` y `gates`.
- [ ] 13.3 [REFACTOR] Confirmar que ninguna rama nueva de `status.go` se ejecuta cuando `kickoff.yaml` no existe (guarda explícita al inicio de cada función tocada, no un `if` disperso).
- [ ] 13.4 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`), V-C, V-D, V-E (Fase 11 en verde, byte a byte).

## Fase 14: P3d — Proyección v2 y enrutamiento `await-gate` (REQ-21.7 a REQ-21.10; D-09)

Depende de la Fase 13.

- [ ] 14.1 [RED] Ampliar `internal/sddstatus/status_v2_test.go`: con sello y compuerta pendiente, `StatusV2Projection.Governance` no es `nil` y contiene `kickoff`/`gates`/`roster`; sin sello, `Governance` es `nil` y el campo no aparece en el JSON serializado (`omitempty`) — repite la aserción de la compuerta de control de la Fase 11 a nivel de v2; `statusV2NextRecommended` (`:209-219`) devuelve `"await-gate"` para una compuerta pendiente; `nonPhaseRoutingInstructions` (`:1552-1575`) imprime las dos invocaciones ejecutables exactas para `await-gate`, igual que ya hace para `select-change` (`:1554-1559`, `(read-only)`); cubrir con tabla los escenarios BDD de `spec.md` REQ-21.7 a REQ-21.10 que producen una ruta observable de enrutamiento.
- [ ] 14.2 [GREEN] Modificar `internal/sddstatus/status_v2.go`: `Governance *governanceV2 \`json:"governance,omitempty"\`` en `StatusV2Projection` (mismo patrón que `Consent`/`Archived`, `:193-196`, `:210-212`, `(read-only)` como precedente); tipos `governanceV2`, `kickoffV2`, `gateV2`, `rosterV2`; nuevo caso `await-gate` en `statusV2NextRecommended`; caso nuevo en `nonPhaseRoutingInstructions` de `status.go` (tocado aquí, no en la Fase 13, porque depende del enum v2).
- [ ] 14.3 [REFACTOR] Confirmar que el valor `await-gate` es aditivo puro: ningún valor ni campo existente del enum cambia de significado.
- [ ] 14.4 [Verificación de cierre] V-A, V-B (`./internal/sddstatus/...`), V-C, V-D, V-E (Fase 11 en verde).

## Fase 15: P4a — Compuerta de control: caracterización REQ-1.1 y rol reservado `fullstack` (REQ-21.6; D-06, D-07, H-4 — compuerta de control nº 2)

No depende de las Fases 1–14. Se escribe contra el árbol tal y como está hoy. Ver «Protocolo de compuertas de control».

> **Válvula de alivio:** si el diff real supera holgadamente 400 líneas, separar `internal/handoff/reserved_role_parity_test.go` en una PR inmediatamente posterior (`inc-21/15b-handoff-reserved-role-parity`).

- [ ] 15.1 [Caracterización] Escribir `internal/multirole/req11_characterization_test.go`: los tres escenarios vivos de `multi-role-fan-out-engine` REQ-1.1 sobre `DetectRoles` (`(read-only)` en esta tarea) — multi-rol con políticas distintas (`backend` blocking, `e2e` deferred); rol `database` declarado en `design.md` pero ausente de `axiom.yaml` ⇒ error; modo retrocompatible de rol único (fallback al rol principal del workspace con `blocking`, `tasks.md`, `verify-report.md`). Ejecutar y confirmar que **los tres pasan hoy, sin ningún cambio de producción**.
- [ ] 15.2 [RED] Escribir en `internal/multirole/types_test.go`: `IsReservedRole("fullstack")` ⇒ `true`; `IsReservedRole("Fullstack")` ⇒ `true` (insensible a mayúsculas); `IsReservedRole("database")` ⇒ `false`.
- [ ] 15.3 [GREEN] Modificar `internal/multirole/types.go`: añadir `const RoleFullstack = "fullstack"` y `func IsReservedRole(role string) bool`.
- [ ] 15.4 [RED] Escribir en `internal/multirole/detector_test.go`: `roleExists("fullstack", cfg)` ⇒ `true` aunque `axiom.yaml` no lo declare; `roleExists("database", cfg)` sin declarar ⇒ sigue produciendo el error existente (no debe cambiar).
- [ ] 15.5 [GREEN] Modificar `internal/multirole/detector.go`: en `roleExists` (`:120-127`), devolver `true` de inmediato cuando `IsReservedRole(role)`, antes de recorrer `cfg.Roles`. `DetectRoles` no se toca en ninguna línea.
- [ ] 15.6 [RED] Escribir `internal/handoff/reserved_role_parity_test.go`: sobre un corpus compartido de nombres de rol (reservados y no reservados), las dos copias de `roleExists` (`internal/multirole/detector.go` y `internal/handoff/validator.go`) producen exactamente el mismo veredicto para cada nombre.
- [ ] 15.7 [GREEN] Modificar `internal/handoff/validator.go`: misma rama de dos líneas en su copia de `roleExists` (`:140-147`).
- [ ] 15.8 [Caracterización — repetición] Reejecutar `internal/multirole/req11_characterization_test.go` tras 15.3–15.7 y confirmar que los tres escenarios siguen en verde, byte a byte respecto a 15.1.
- [ ] 15.9 [Verificación de cierre] V-A, V-B (`./internal/multirole/... ./internal/handoff/...`), V-C, V-D, V-E (caracterización REQ-1.1 en verde).

## Fase 16: P4b — Reconciliación de roster (REQ-21.6; D-06)

Depende de la Fase 15.

- [ ] 16.1 [RED] Escribir `internal/multirole/roster_test.go`: `sealed` no vacío ⇒ `Source: RosterSourceKickoff`, `Roles: sealed`; si `design.md` declara roles **distintos** de `sealed`, `Conflict` se puebla con el detalle y el roster sellado sigue mandando (nunca se resuelve en silencio); `sealed` vacío ⇒ `ResolveRoster` delega en `DetectRoles` y clasifica el resultado como `RosterSourceDesign` o `RosterSourceCompatibility` según corresponda; con `sealed` vacío, el resultado es **idéntico** al que devolvería `DetectRoles` directamente (test de equivalencia, no solo de forma).
- [ ] 16.2 [GREEN] Crear `internal/multirole/roster.go`: `RosterSource`, `Roster`, `RosterConflict`, `ResolveRoster(sealed []RoleAssignment, designPath string, wsConfig *workspace.WorkspaceConfig) (Roster, error)`, implementando exactamente las reglas de 16.1.
- [ ] 16.3 [GREEN] Modificar `cmd/axiom/main.go`: `runRoleList` (`:732`), `runRoleStatus` (`:783`) y `runRoleBarrier` (`:855`) pasan de invocar `DetectRoles` directamente a invocar `ResolveRoster`, mostrando `Source` y `Conflict` cuando `Conflict != nil`.
- [ ] 16.4 [REFACTOR] Confirmar que `ResolveRoster` no importa nada de `internal/kickoff` (la dirección de la arista es `kickoff → multirole`, nunca al revés — D-06 lo fija explícitamente para evitar un ciclo).
- [ ] 16.5 [Verificación de cierre] V-A, V-B (`./internal/multirole/... ./cmd/axiom/...`), V-C, V-D, V-E (caracterización REQ-1.1 en verde).

## Fase 17: P5a — Construcción del relevo de integración (REQ-21.14; D-11)

Depende de la Fase 16 (usa `Roster`) y de la Fase 4 (usa `GateLedger`).

- [ ] 17.1 [RED] Escribir `internal/kickoff/closure_test.go`: `IntegrationHandoff` produce un `handoff.Handoff` con `FromPhase: apply`, `ToPhase: verify`, `Status: ready`; las cinco secciones no vacías (`handoff.Validate`, `(read-only)`); consolida los artefactos de **todos** los roles del roster, no solo el último cerrado; regla de resolución de `to_role` en el orden exacto de D-11 — (1) rol `qa` insensible a mayúsculas si existe, (2) roster de un único rol ⇒ ese mismo rol (auto-relevo de `fullstack`), (3) en otro caso, el último rol cerrado; con `wsConfig == nil` o roster vacío, la regla es irrelevante porque `handoff.validateRoles` (`(read-only)`) no comprueba nada.
- [ ] 17.2 [GREEN] Crear `internal/kickoff/closure.go`: `IntegrationHandoff(changeName string, roster multirole.Roster, gates kickoff.GateLedger, artifacts ArtifactInventory) (*handoff.Handoff, error)`.
- [ ] 17.3 [REFACTOR] Confirmar que `closure.go` reutiliza `handoff.Handoff`/`handoff.WriteFile` sin duplicar su validación (D-11 rechaza explícitamente un formato de relevo propio).
- [ ] 17.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 18: P5b — Aviso de último rol y cableado del relevo (REQ-21.13, REQ-21.14, REQ-21.15; D-10, D-12)

Depende de las Fases 10 y 17.

- [ ] 18.1 [RED] Ampliar `internal/cli/sdd_gate_test.go`: al aprobar la `role-apply` del último rol pendiente (incluido el caso de roster de un único `fullstack`), la salida contiene el aviso formal exacto de REQ-21.13 **exactamente una vez**, y se escribe `handoff.md` en `openspec/changes/<cambio>/handoff.md` con `status: ready`; con roles aún pendientes, no se emite el aviso ni se escribe el relevo; una compuerta `role-apply` rechazada no dispara el aviso aunque sea el único rol pendiente (REQ-21.13, tercer escenario).
- [ ] 18.2 [GREEN] Modificar `internal/cli/sdd_gate.go`: al registrar una decisión `approved` sobre una compuerta `role-apply:<rol>`, invocar `kickoff.LastRoleClosed`; si es `true`, construir el relevo con `kickoff.IntegrationHandoff` (Fase 17), escribirlo con `handoff.WriteFile` (`(read-only)`) y emitir el aviso formal en `stdout`.
- [ ] 18.3 [RED] Ampliar `internal/sddstatus/governance_test.go` (o crear `handoff_dependency_test.go`): con `handoff.md` en `status: ready` y `to_phase: verify`, `dependencies.Verify` pasa a `ready`; con `status: blocked` o `needs_clarification`, `dependencies.Verify` pasa a `blocked` con un motivo genuino que cita el estado leído; fichero ausente o ilegible ⇒ se trata como ausencia de relevo (no bloqueo), salvo que `LastRoleClosed` ya sea `true`.
- [ ] 18.4 [GREEN] Modificar `internal/sddstatus/governance.go`: leer `openspec/changes/<cambio>/handoff.md` con `handoff.ParseFile` (`(read-only)`) cuando el kickoff sellado tiene roster multi-rol o `handoff_policy: per_checkpoint`, y ajustar `dependencies.Verify` según 18.3.
- [ ] 18.5 [REFACTOR] Confirmar que la lectura de `handoff.md` en `governance.go` es de solo lectura (no reescribe el relevo).
- [ ] 18.6 [Verificación de cierre] V-A, V-B (`./internal/kickoff/... ./internal/cli/... ./internal/sddstatus/...`), V-C, V-D, V-E (ambas compuertas de control en verde).

## Fase 19: P6a — Fontanería git de ancestro local (REQ-21.16; D-13; T-3, T-6)

Independiente de las Fases 1–18. Modifica un único fichero fuera de `internal/kickoff`.

- [ ] 19.1 [RED] Escribir/ampliar `internal/reviewtransaction/snapshot_test.go`: sobre un repositorio git temporal (`t.TempDir()` + `git init` local, sin red) — commit ancestro real ⇒ `true`; commit no-ancestro (rama divergente) ⇒ `false`; revisión inexistente ⇒ error nombrado; contexto ya cancelado ⇒ error, nunca `true`; `PATH` vacío o binario git ausente ⇒ error nombrado, nunca pánico; salida del subproceso distinta de 0 y de 1 (por ejemplo, código 128) ⇒ error, nunca `true`; **árbol de trabajo sucio (ficheros modificados sin commit) produce el mismo veredicto que un árbol limpio** (T-3: la comprobación es de solo lectura sobre el grafo de commits, nunca toca el índice).
- [ ] 19.2 [GREEN] Añadir a `internal/reviewtransaction/snapshot.go`: `(builder SnapshotBuilder) RevisionIsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)`, implementado sobre `runGitCapturedRangeWithTimeout` (`:1963`, `(read-only)` como mecanismo existente que se invoca, no se reescribe) ejecutando `git merge-base --is-ancestor <ancestor> <descendant>` con `ancestor`/`descendant` como elementos de *slice*, nunca interpolados en una cadena de *shell*. Salida 0 ⇒ `true`; salida 1 ⇒ `false`; cualquier otra ⇒ error.
- [ ] 19.3 [REFACTOR] Confirmar que el nuevo método no ejecuta `fetch`, `ls-remote` ni ninguna otra operación remota (T-4/T-5, N/A por diseño — este método debe seguir siendo la prueba viviente de esa fila N/A).
- [ ] 19.4 [Verificación de cierre] V-A, V-B (`./internal/reviewtransaction/...`), V-C, V-D — confirmar explícitamente que el diff de esta fase toca **solo** la adición del método en `snapshot.go`, ningún otro fichero de `internal/reviewtransaction/**`.

## Fase 20: P6b — Verificación de evidencia de integración (REQ-21.16; D-13; T-11)

Depende de la Fase 19.

- [ ] 20.1 [RED] Escribir `internal/kickoff/integration_test.go` con un doble de prueba para `AncestryChecker`: `evidence_kind: pr_merged` con ancestro confirmado ⇒ `verified: true`, registro anexado; `pr_merged` sin ancestro confirmado ⇒ rechazo explícito nombrando el commit y la rama comparados, **cero registros anexados**; `deployment`/`attestation` ⇒ `verified: false` siempre, registro anexado con actor, motivo y referencia; un comprobador que devuelve error ⇒ error propagado, **nunca** `verified: true` (T-11: Axiom no inventa un veredicto que no puede probar).
- [ ] 20.2 [GREEN] Crear `internal/kickoff/integration.go`: `type AncestryChecker interface { IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) }` (puerto de una sola función) y `VerifyIntegrationEvidence(ctx context.Context, ev Evidence, checker AncestryChecker) (Evidence, error)` implementando las cuatro reglas de 20.1. `internal/kickoff` no importa `internal/reviewtransaction` para esto: el caso se prueba con el doble, no con el ejecutor real.
- [ ] 20.3 [REFACTOR] Confirmar que ningún camino de `VerifyIntegrationEvidence` marca `verified: true` sin haber llamado al `checker` con éxito.
- [ ] 20.4 [Verificación de cierre] V-A, V-B (`./internal/kickoff/...`), V-C, V-D.

## Fase 21: P6c — Precondición de archive cableada (REQ-21.16; D-13)

Depende de las Fases 13, 19 y 20.

- [ ] 21.1 [RED] Ampliar `internal/cli/sdd_gate_test.go`: `record --gate integration --decision approved --evidence-kind pr_merged --commit <sha>` sobre un repositorio temporal con el commit como ancestro real de `main` ⇒ salida 0, registro `verified: true`; el mismo comando sin que el commit sea ancestro ⇒ salida 1, **nada anexado**, mensaje nombrando el commit y la rama comparados; `--evidence-kind deployment`/`attestation` sin `--commit` ⇒ salida 0, `verified: false` con actor/motivo/referencia.
- [ ] 21.2 [GREEN] Modificar `internal/cli/sdd_gate.go`: banderas `--evidence-kind`, `--commit`, `--base-ref`, `--evidence`, `--actor`; inyección del `AncestryChecker` real respaldado por `SnapshotBuilder.RevisionIsAncestor` (Fase 19).
- [ ] 21.3 [RED] Ampliar `internal/sddstatus/status_test.go`: con kickoff sellado y **sin** compuerta `integration` aprobada, `dependencies.Archive == blocked` con `blockedReasons` nombrando la precondición exacta que falta y la invocación ejecutable que la satisface; con la compuerta aprobada, `dependencies.Archive == ready`; **sin kickoff sellado**, `resolveDependencies` mantiene byte a byte su comportamiento actual (`:1443-1445`) — repetición de la compuerta de control de la Fase 11 sobre esta rama específica.
- [ ] 21.4 [GREEN] Modificar `internal/sddstatus/status.go`: condicionar `dependencies.Archive` a la compuerta `integration` **solo cuando hay sello** (D-13, coste cero para lo existente).
- [ ] 21.5 [Integración] Escribir un test de integración en `internal/sddstatus` con un árbol OpenSpec completo en `t.TempDir()`: sin registro `integration` ⇒ el cambio permanece fuera de `openspec/changes/archive/` y `openspec/specs/` no se modifica (REQ-21.16, segundo escenario); con él ⇒ el estado se proyecta `ready`.
- [ ] 21.6 [Verificación de cierre] V-A, V-B (`./internal/kickoff/... ./internal/cli/... ./internal/sddstatus/...`), V-C, V-D, V-E (compuerta de control de la Fase 11 en verde).

## Fase 22: P7a — Doctrina compartida de kickoff/compuertas y paso 0 de ODD (REQ-21.1, REQ-21.2, REQ-21.3, REQ-21.17; H-2)

Independiente de las Fases 1–21 en cuanto a compilación (son cambios de texto/doctrina), pero se secuencia al final porque documenta verbos que ya deben existir.

- [ ] 22.1 [RED] Ampliar `internal/components/agentguidance/routing_test.go` (o el test existente que cubra `RenderRouting`): el texto renderizado para cada `AgentID` contiene, **antes** del paso «1. Authorize», la pregunta bloqueante de carril de REQ-21.1 y la instrucción de no formular preguntas adicionales bajo ODD (REQ-21.2).
- [ ] 22.2 [GREEN] Modificar `internal/components/agentguidance/routing.go` (`:48-59`): insertar el «Paso 0» del protocolo — evaluación de alcance, pregunta bloqueante ODD/SDD, entrada directa a SDD para alcance inequívocamente arquitectónico (REQ-21.1–21.3) — inmediatamente antes de «1. Authorize» (`:50`).
- [ ] 22.3 [RED] Escribir/ampliar `internal/components/sdd/orchestrator_shared_sections_test.go`: la sección compartida nueva `SDD Change Kickoff and Block Gates` se renderiza de forma determinista (mismo contenido en ejecuciones repetidas) cuando un activo la registra.
- [ ] 22.4 [GREEN] Crear la sección nueva en `internal/assets/skills/_shared/sdd-orchestrator-sections.md`, entre marcadores `<!-- sdd-orchestrator-section:sdd-change-kickoff-and-block-gates:start|end -->` (patrón de `:8-18`, `(read-only)` como precedente de forma), documentando: el cuestionario de pre-vuelo por cambio (modalidad, política de relevos, roles — REQ-21.5, REQ-21.6), las cuatro compuertas de bloque (REQ-21.8 a REQ-21.11) y su frontera normativa con RDD (§1.3 del diseño, resumida en prosa para el agente).
- [ ] 22.5 [GREEN] Modificar `internal/components/sdd/orchestrator.go` (`:37-48`): registrar la sección nueva en `sharedOrchestratorSection`.
- [ ] 22.6 [Doc] Confirmar en `internal/assets/skills/sdd-archive/SKILL.md` que la sección «Archive Readiness» (`:72-76`) ya describe correctamente que `archive` exige evidencia de integración/despliegue (REQ-21.16/21.17); si no lo hace, ampliarla citando `axiom sdd gate record --gate integration` como la vía para aportar esa evidencia. (Sin cambio de mecanismo: REQ-21.17 no introduce código nuevo, solo esta referencia doctrinal — design.md, trazabilidad).
- [ ] 22.7 [Verificación de cierre] V-A, V-B (`./internal/components/... ./internal/assets/...`), V-C, V-D.

## Fase 23: P7b — Activos por agente, skills de tareas, convención OpenSpec y documentación (REQ-21.5, REQ-21.6, REQ-21.10; §4.7 del diseño)

Depende de la Fase 22 (la sección compartida ya existe para poder referenciarla desde cada activo).

- [ ] 23.1 [RED] Ampliar `internal/assets/assets_test.go`: para cada uno de los doce `AgentID` (`claude`, `codex`, `cursor`, `gemini`, `generic`, `hermes`, `kimi`, `kiro`, `opencode`, `qwen`, `antigravity`, `windsurf`), el activo `sdd-orchestrator*.md` renderizado contiene el marcador `{{GENTLE_AI_SDD_SECTION:SDD Change Kickoff and Block Gates}}` resuelto con el contenido de la sección compartida de la Fase 22.
- [ ] 23.2 [GREEN] Insertar el marcador `{{GENTLE_AI_SDD_SECTION:SDD Change Kickoff and Block Gates}}` en cada uno de los doce `internal/assets/{claude,codex,cursor,gemini,generic,hermes,kimi,kiro,opencode,qwen,antigravity,windsurf}/sdd-orchestrator*.md`.
- [ ] 23.3 [GREEN] Modificar `internal/assets/skills/sdd-tasks/SKILL.md`: documentar que, cuando el roster sellado es multi-rol, la producción es `tasks.<rol>.md` por rol en vez de un único `tasks.md` (REQ-21.10, segundo escenario).
- [ ] 23.4 [GREEN] Modificar `internal/assets/skills/_shared/openspec-convention.md`: añadir `kickoff.yaml`, `gates.yaml` y `handoff.md` al árbol de convención (`:5-24`) y a la tabla de rutas (`:26-41`).
- [ ] 23.5 [Infra] Regenerar los siete *goldens* afectados bajo `testdata/golden/sdd-*.golden` (ficheros generados: excluidos del recuento de líneas autoría de este pronóstico, incluidos en la identidad de snapshot completa).
- [ ] 23.6 [Doc] Actualizar `AGENTS.md`, `GEMINI.md` y `docs/ROADMAP.md`: alta de INC-21 y referencia a los verbos nuevos `axiom sdd kickoff` / `axiom sdd gate`.
- [ ] 23.7 [Verificación de cierre] V-A, V-B (`./internal/assets/... ./internal/components/...`), V-C, V-D, V-F (suite completa `go test ./...` con `-timeout 900s`, y `./internal/sddstatus/...` aparte con `-timeout 600s`, como cierre de la cadena completa de 23 PRs).

---

## Trazabilidad rápida (fase → requerimiento / decisión)

| Fase | Rebanada | Requerimientos / decisiones cubiertos |
|---|---|---|
| 1 | P1a | REQ-21.5, REQ-21.6 (tipos base); D-01, D-02 |
| 2 | P1b | REQ-21.5; D-01, D-02, D-04 |
| 3 | P1c | REQ-21.4; D-01–D-04; §8.1; O-1 (resuelta) |
| 4 | P1d | REQ-21.12 (soporte de ledger/digest); D-02, D-08 |
| 5 | P1e | REQ-21.7, REQ-21.8, REQ-21.9, REQ-21.10 (mecánica); D-05, D-08, D-09 |
| 6 | P1f | REQ-21.10, REQ-21.11, REQ-21.12; D-08 |
| 7 | P1g | REQ-21.13 (predicado), REQ-21.18 (guarda); D-10, D-14; T-9 |
| 8 | P2a | REQ-21.5, REQ-21.10, REQ-21.12; T-2, T-7 |
| 9 | P2b | REQ-21.4, REQ-21.5, REQ-21.14 (parcial); D-04; T-8 |
| 10 | P2c | REQ-21.10, REQ-21.11, REQ-21.12; T-8 |
| 11 | P3a | REQ-21.7; D-05 — **compuerta de control** |
| 12 | P3b | REQ-21.8–21.11; D-08; T-9 |
| 13 | P3c | REQ-21.4 (parcial), REQ-21.7, REQ-21.16 (parcial); D-05, D-09, D-13; T-10 |
| 14 | P3d | REQ-21.7–21.10; D-09 |
| 15 | P4a | REQ-21.6; D-06, D-07, H-4 — **compuerta de control** |
| 16 | P4b | REQ-21.6; D-06 |
| 17 | P5a | REQ-21.14; D-11 |
| 18 | P5b | REQ-21.13, REQ-21.14, REQ-21.15; D-10, D-12 |
| 19 | P6a | REQ-21.16; D-13; T-3, T-6 |
| 20 | P6b | REQ-21.16; D-13; T-11 |
| 21 | P6c | REQ-21.16; D-13 |
| 22 | P7a | REQ-21.1, REQ-21.2, REQ-21.3, REQ-21.17; H-2 |
| 23 | P7b | REQ-21.5, REQ-21.6, REQ-21.10; §4.7 |

---

## Review Workload Forecast

| Campo | Valor |
|---|---|
| Estimación agregada de líneas cambiadas (adiciones + eliminaciones, autoría, excluidos generados) | **~5.300–8.000 líneas** repartidas en 23 PRs (detalle por fase abajo) |
| Riesgo de presupuesto de 400 líneas | **High** (agregado del incremento; por PR individual, ninguna se planifica por encima de ~450 y la mayoría son Low/Medium) |
| PRs encadenados recomendados | **Yes** |
| Partición sugerida | 23 rebanadas (tabla siguiente) |
| Estrategia de entrega | `auto-chain` |
| Estrategia de cadena | `feature-branch-chain` (tracker `feature/inc-21-upfront-flow-governance`) |

Líneas de guarda exactas (contrato de herramienta, no traducir):

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High
```

### Estimación y riesgo por PR

| # | PR (rama hija) | Fase | Contenido | Líneas estimadas | Riesgo |
|---|---|---|---|---|---|
| 1 | `inc-21/01-kickoff-types-schema` | 1 | `types.go` + `schema.go` + validación | 180–280 | Medium |
| 2 | `inc-21/02-kickoff-seal` | 2 | `seal.go` (Seal/Load) | 150–220 | Low-Medium |
| 3 | `inc-21/03-kickoff-retro-seal` | 3 | Regla de retro-sellado (§8.1) | 180–270 | Medium |
| 4 | `inc-21/04-kickoff-ledger-digest` | 4 | `ledger.go` + `digest.go` | 290–450 | Medium-High (válvula: separar digest) |
| 5 | `inc-21/05-kickoff-gate-machine-core` | 5 | `machine.go` mecánica básica | 230–320 | Medium |
| 6 | `inc-21/06-kickoff-gate-machine-reopen` | 6 | `machine.go` reapertura + multi-rol | 290–420 | Medium-High (válvula: separar generación role-apply) |
| 7 | `inc-21/07-kickoff-role-closure-archived-guard` | 7 | `LastRoleClosed` + `archived.go` + frontera RDD | 260–420 | Medium-High (válvula: separar escáner RDD) |
| 8 | `inc-21/08-cli-args` | 8 | `args.go` | 260–380 | Medium |
| 9 | `inc-21/09-cli-kickoff-verb` | 9 | `sdd_kickoff.go` + `main.go` parcial | 260–400 | Medium |
| 10 | `inc-21/10-cli-gate-verb` | 10 | `sdd_gate.go` + `main.go` parcial | 220–340 | Medium |
| 11 | `inc-21/11-status-noseal-regression` | 11 | Compuerta de control (solo test) | 80–150 | Low |
| 12 | `inc-21/12-status-governance-envelope` | 12 | `governance.go` (envoltorio) | 260–380 | Medium |
| 13 | `inc-21/13-status-wiring` | 13 | `status.go` cableado | 260–380 | Medium |
| 14 | `inc-21/14-status-v2-routing` | 14 | `status_v2.go` + `await-gate` | 260–380 | Medium |
| 15 | `inc-21/15-roster-req11-characterization` | 15 | Compuerta de control + rol reservado | 300–410 | Medium-High (válvula: separar parity test) |
| 16 | `inc-21/16-roster-resolve` | 16 | `roster.go` + wiring `main.go` | 280–420 | Medium-High (válvula: separar wiring de `main.go`) |
| 17 | `inc-21/17-handoff-closure` | 17 | `closure.go` | 280–400 | Medium |
| 18 | `inc-21/18-handoff-wiring` | 18 | Aviso último rol + cableado | 230–340 | Medium |
| 19 | `inc-21/19-git-ancestor` | 19 | `RevisionIsAncestor` | 180–270 | Low-Medium |
| 20 | `inc-21/20-integration-evidence` | 20 | `integration.go` | 220–340 | Medium |
| 21 | `inc-21/21-archive-precondition` | 21 | Cableado de precondición | 260–380 | Medium |
| 22 | `inc-21/22-doctrine-shared-section` | 22 | Sección compartida + `routing.go` | 170–310 | Medium |
| 23 | `inc-21/23-doctrine-agent-assets` | 23 | 12 activos + skills + docs | 195–345 | Medium |

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Tipos y validación de esquema del kickoff | PR 1 | `go test ./internal/kickoff/... -run TestValidate` | N/A — biblioteca pura sin superficie de usuario hasta PR 9 | Eliminar `types.go`/`schema.go`; aditivo puro, sin llamadores |
| 2 | Sellado con escritura única y relectura del ganador | PR 2 | `go test ./internal/kickoff/... -run TestSeal` | N/A — sin superficie de usuario todavía | Eliminar `seal.go`; nada lo invoca aún |
| 3 | Regla de retro-sellado (3 casos, O-1 aplicada) | PR 3 | `go test ./internal/kickoff/... -run TestInferKickoff` | N/A — sin superficie de usuario todavía | Eliminar la función `InferKickoff` |
| 4 | Ledger solo-anexar + digest estable | PR 4 | `go test ./internal/kickoff/... -race -run 'TestLedger\|TestDigest'` | N/A — sin superficie de usuario todavía | Eliminar `ledger.go`/`digest.go` |
| 5 | Máquina de compuertas: modo continuo, apertura, orden fijo | PR 5 | `go test ./internal/kickoff/... -run TestEvaluateGates` | N/A — función pura sin E/S | Eliminar el esqueleto de `EvaluateGates` |
| 6 | Máquina de compuertas: reapertura por digest, multi-rol | PR 6 | `go test ./internal/kickoff/... -run TestEvaluateGates` | N/A — función pura sin E/S | Revertir a la mecánica básica de la PR 5 |
| 7 | Cierre de último rol + guarda de archivado + frontera RDD | PR 7 | `go test ./internal/kickoff/...` | N/A — sin superficie de usuario todavía | Eliminar `LastRoleClosed`, `archived.go`, `rdd_boundary_test.go` |
| 8 | Parseo de argumentos con contención de rutas | PR 8 | `go test ./internal/kickoff/... -run TestParse` | N/A — sin verbo CLI todavía | Eliminar `args.go` |
| 9 | Verbo `axiom sdd kickoff seal\|show` (incl. `--infer`) | PR 9 | `go test ./internal/cli/... -run TestRunSDDKickoff` | `axiom sdd kickoff seal --cwd <tmp> --change <c> --execution-style checkpointed --handoff-policy none` sobre un cambio de prueba | Eliminar `sdd_kickoff.go` y su `case` en `main.go` |
| 10 | Verbo `axiom sdd gate record\|show` | PR 10 | `go test ./internal/cli/... -run TestRunSDDGate` | `axiom sdd gate record --gate spec --decision approved --reason test` sobre el cambio sellado por la PR 9 | Eliminar `sdd_gate.go` y su `case` en `main.go` |
| 11 | Compuerta de control: regresión sin sello | PR 11 | `go test ./internal/sddstatus/... -run TestKickoffAbsenceRegression` | `axiom sdd status --json` sobre un cambio sin `kickoff.yaml` | Eliminar el test de regresión; ningún otro fichero cambia |
| 12 | Envoltorio tipado de compuerta | PR 12 | `go test ./internal/sddstatus/... -run TestGovernance` | N/A — sin cableado en `status.go` todavía | Eliminar `governance.go` |
| 13 | Cableado de gobernanza en `status.go` | PR 13 | `go test ./internal/sddstatus/... -run TestStatus` | `axiom sdd status --json --instructions` sobre un cambio sellado con compuerta pendiente | Revertir `status.go` a la versión de la PR 12 |
| 14 | Proyección v2 y enrutamiento `await-gate` | PR 14 | `go test ./internal/sddstatus/... -run TestStatusV2` | `axiom sdd status --json` mostrando `nextRecommended: await-gate` | Revertir `status_v2.go` |
| 15 | Compuerta de control REQ-1.1 + rol reservado `fullstack` | PR 15 | `go test ./internal/multirole/... ./internal/handoff/...` | `axiom role list` sobre un `design.md` con roles multi-rol (sin cambio de comportamiento) | Revertir `roleExists` en ambos ficheros; el error de `database` sigue intacto |
| 16 | Reconciliación de roster (`ResolveRoster`) | PR 16 | `go test ./internal/multirole/... -run TestResolveRoster` | `axiom role list` sobre un cambio con kickoff sellado en `fullstack` | Revertir `main.go` a `DetectRoles` directo; eliminar `roster.go` |
| 17 | Construcción del relevo de integración | PR 17 | `go test ./internal/kickoff/... -run TestIntegrationHandoff` | N/A — sin cableado en el CLI todavía | Eliminar `closure.go` |
| 18 | Aviso de último rol + escritura del relevo | PR 18 | `go test ./internal/cli/... ./internal/sddstatus/...` | `axiom sdd gate record --gate role-apply:fullstack --decision approved` sobre el único rol de un cambio, observando el aviso y `handoff.md` | Revertir el cableado en `sdd_gate.go`/`governance.go`; `closure.go` queda sin invocar |
| 19 | `RevisionIsAncestor` local, sin red | PR 19 | `go test ./internal/reviewtransaction/... -run TestRevisionIsAncestor` | `git merge-base --is-ancestor` manual sobre un repo de prueba, comparado con el método | Eliminar el método nuevo; `snapshot.go` vuelve a su versión previa |
| 20 | Verificación de evidencia de integración | PR 20 | `go test ./internal/kickoff/... -run TestVerifyIntegrationEvidence` | N/A — probado con doble de `AncestryChecker`, sin cableado real todavía | Eliminar `integration.go` |
| 21 | Precondición de archive cableada de punta a punta | PR 21 | `go test ./internal/cli/... ./internal/sddstatus/...` | `axiom sdd gate record --gate integration --decision approved --evidence-kind pr_merged --commit <sha>` sobre un repo de prueba con el commit como ancestro real | Revertir el cableado; `dependencies.Archive` vuelve a su condición previa |
| 22 | Doctrina compartida + paso 0 de ODD | PR 22 | `go test ./internal/components/...` | `axiom sync` sobre un agente de prueba, confirmando el paso 0 en el texto renderizado | Revertir `routing.go`, la sección nueva y su registro |
| 23 | Activos por agente + skills + documentación | PR 23 | `go test ./internal/assets/...` | `axiom sync` sobre los doce agentes, confirmando el marcador resuelto en cada uno | Revertir los doce activos y los *goldens* a su estado previo |
</content>
</invoke>
