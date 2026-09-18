# Tareas: Flujo ODD y Promoción a SDD (inc-19-odd-workflow-and-promotion)

> **Fuente de alcance:** `proposal.md`, `spec.md` (16 requerimientos, 37 escenarios), `design.md` (987 líneas, autoritativo).
> **Entradas de línea base:** observación Engram `#360` (línea base de tests en rojo) y `openspec/config.yaml` (`strict_tdd: true`, reglas de fase `tasks`).
> **Idioma del artefacto:** español (castellano peninsular). Identificadores Go, rutas, comandos y las cuatro líneas de guarda de pronóstico (contrato de herramienta) permanecen en inglés.

---

## Nota de partición de rebanadas (ajuste sobre la propuesta)

La propuesta (`proposal.md` §4.6) y el diseño (`design.md` §4, §8) proponen **6 rebanadas** (P1–P6). Al descomponer el diseño real en tareas verificables con TDD estricto, dos de esas seis rebanadas resultan demasiado grandes para el presupuesto de 400 líneas incluso de forma optimista, mientras que las otras cuatro se mantienen razonablemente cerca del límite:

| Rebanada del diseño | Motivo del ajuste | Partición final en este documento |
|---|---|---|
| P1 — Núcleo del documento vivo | 8 ficheros de producción nuevos con lógica real (parser, almacén, espejo con subproceso) más sus tablas de test exigidas por `strict_tdd` y por la matriz de amenazas (T-6, T-7): estimación agregada muy por encima de 400 líneas, incluso siendo aditivo puro. | **Fase 1 (PR1a)** — tipos, nombre, plantilla. **Fase 2 (PR1b)** — parser, almacén, espejo, render. Ambas compilan y prueban de forma independiente porque `internal/odd` no tiene consumidores hasta la Fase 3. |
| P3 — Promoción | Mezcla dos riesgos de naturaleza distinta: (a) una extensión de severidad **alta** (R3) sobre un contrato ya publicado (REQ-15.1), que se beneficia de ser revisada en aislamiento total; y (b) el dominio y la CLI de promoción, con su propia carga de pruebas. | **Fase 4 (PR3a)** — extensión protegida y mínima de `CreateIncrement`, con el test de caracterización [D-03] como única puerta de control. **Fase 5 (PR3b)** — dominio `odd.Promote`, `RenderProposalBody` y `axiom odd promote`. |
| P2, P4, P5, P6 | Estimación cercana o moderadamente por encima de 400 líneas, pero cada una es un único concern coherente (una superficie de CLI, una superficie de UI, una pantalla de TUI, activos/documentación). Fragmentarlas perdería más coherencia narrativa de la que ganaría en presupuesto. | Se mantienen como **Fase 3 (PR2)**, **Fase 6 (PR4)**, **Fase 7 (PR5)** y **Fase 8 (PR6)**, con nota de partición interna opcional en el pronóstico para quien ejecute `sdd-apply`, a decidir con el diff real. |

Resultado: **8 rebanadas** en vez de 6. El orden de apilado y las fronteras de reversión respetan la precedencia ya fijada por el diseño (P1 → P2 → P3 → {P4, P5} → P6); ver `Review Workload Forecast` para el diagrama actualizado.

Ninguna tarea de esta partición toca `internal/sddstatus/`, `internal/cli/sdd_*.go`, `internal/agents/researchcapability/`, `openspec/INDEX.md` ni `openspec/config.yaml` — criterio de aceptación explícito por rebanada, verificado en la tarea de cierre de cada fase.

---

## Review Workload Forecast

| Campo | Valor |
|---|---|
| Estimación agregada de líneas cambiadas | ~4.300–5.400 líneas (todas las rebanadas, adiciones puras salvo Fase 4/7 que también modifican ficheros existentes) |
| Riesgo de presupuesto de 400 líneas | **High** |
| PRs encadenados recomendados | **Yes** |
| Partición sugerida | 8 rebanadas (ver tabla siguiente) apiladas contra `main` en secuencia |
| Estrategia de entrega | `auto-chain` (fijada por la sesión) |
| Estrategia de cadena | `stacked-to-main` (fijada por el orquestador — descarta `feature-branch-chain` por divergencia de larga vida sin beneficio) |

Líneas de guarda exactas (contrato de herramienta, no traducir):

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

### Estimación y riesgo por rebanada

| # | Rebanada | Contenido (ficheros nuevos/modificados) | Estimación de líneas | Riesgo | Candidato a partición en vivo si el diff real lo exige |
|---|---|---|---|---|---|
| 1 | PR1a — Fundamentos | `document.go`, `errors.go`, `name.go`, `template.go` + tests | ~380–460 | Medium (límite) | No necesario |
| 2 | PR1b — Parser/almacén/espejo | `parse.go`, `store.go`, `mirror.go`, `render.go` + tests + `.gitkeep` | ~700–970 | **High** | Si excede ~600: separar «parser + almacén» de «espejo + render» |
| 3 | PR2 — CLI create/status | `args.go`, `odd_create.go`, `odd_status.go`, `main.go` (parcial) + tests | ~550–700 | Medium-High | Si excede ~600: separar «create» de «status» (comparten `args.go`, aceptable solaparlo) |
| 4 | PR3a — Extensión de `CreateIncrement` | `create_increment_characterization_test.go`, `types.go`, `service.go` | ~250–320 | Low-Medium | No necesario — deliberadamente aislada por severidad R3 |
| 5 | PR3b — Dominio y CLI de promoción | `promote.go`, `odd_promote.go`, `name_parity_test.go`, `main.go` (resto) + tests | ~700–910 | **High** | Si excede ~600: separar «`RenderProposalBody`» de «ciclo `Promote` + CLI + paridad de nombres» |
| 6 | PR4 — Web UI | `odd_service.go`, `types.go` (DTOs), `server.go`, `dashboard_test.go` (extensión), `assets/{index.html,app.js,style.css}` | ~600–700 | Medium-High | Si excede ~600: separar «backend Go» de «superficie SPA» |
| 7 | PR5 — TUI | `odd_features.go`, `governance.go`, `sdd_increments.go`, `router.go`, `model.go` + tests | ~450–560 | Medium | No necesario |
| 8 | PR6 — Activos y documentación | `internal/assets/…`, `docs/ROADMAP.md`, `AGENTS.md`, `GEMINI.md` | ~230–500 | Medium (incertidumbre por alcance de activos) | No necesario |

Ninguna estimación cuenta contra el presupuesto de revisión los documentos `odd/tasks/*.md` que un usuario cree en tiempo de ejecución (regla de proceso O-3, ver más abajo): no aplica numéricamente a este incremento porque ninguna de las 8 rebanadas confirma en su propio diff un fichero `odd/tasks/*.md` de contenido (solo `.gitkeep`, vacío).

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Fundamentos del documento vivo: tipos, validación de nombre, plantilla canónica | PR 1a | `go test ./internal/odd/...` | N/A — paquete sin consumidores todavía; el binario `axiom` no cambia de comportamiento observable | Eliminar `internal/odd/document.go`, `errors.go`, `name.go`, `template.go` y sus tests; aditivo puro |
| 2 | Parser, almacén y espejo Engram de solo lectura | PR 1b | `go test ./internal/odd/...` | N/A — mismo motivo que la unidad 1 | Eliminar `internal/odd/parse.go`, `store.go`, `mirror.go`, `render.go`, `.gitkeep` y sus tests; revertir **después** de la unidad 1 |
| 3 | CLI `axiom odd create` y `axiom odd status` (`--json`, `--check-mirror`) | PR 2 | `go test ./internal/odd/... ./internal/cli/...` | `axiom odd create demo && axiom odd status && axiom odd status --json && axiom odd status --check-mirror` sobre workspace temporal | Eliminar `internal/cli/odd_create.go`, `odd_status.go`, `internal/odd/args.go`, el `case "odd"` y `runODD` de `main.go`, y las líneas de `printHelp()`; revertir después de la unidad 5 |
| 4 | Extensión protegida de `CreateIncrement` con `ProposalBody` opcional (REQ-15.1) | PR 3a | `go test ./internal/dashboard/...` | `curl -X POST localhost:PUERTO/api/increments` con y sin `proposal_body` sobre un servidor de prueba | Revertir `CreateIncrementRequest`/`CreateIncrement` a su firma previa; revertir antes de la unidad 5 |
| 5 | Dominio de promoción ODD→SDD y `axiom odd promote` | PR 3b | `go test ./internal/odd/... ./internal/dashboard/... ./internal/cli/...` | `axiom odd promote demo --dry-run` y `axiom odd promote demo` sobre workspace temporal, confirmando `openspec/changes/demo/proposal.md` y la marca en `odd/tasks/demo.md` | Eliminar `internal/odd/promote.go`, `internal/cli/odd_promote.go`, `internal/dashboard/name_parity_test.go`, revertir el subcomando `promote` de `main.go`; los cambios SDD ya promovidos se conservan (son datos) |
| 6 | Web UI: rutas `/api/odd*`, DTOs, `odd_service.go`, superficie SPA | PR 4 | `go test ./internal/dashboard/...` | `axiom ui` sobre workspace de prueba; navegar a la pestaña ODD, crear/promover/comprobar espejo desde el navegador | Eliminar `internal/dashboard/odd_service.go`, los cinco registros de ruta, `TestODDEndpoints`, y las adiciones en `assets/{index.html,app.js,style.css}`; independiente de la unidad 7 |
| 7 | TUI: pantalla `ScreenODDFeatures`, menú de Gobernanza, conmutación de carril | PR 5 | `go test ./internal/tui/...` | `axiom tui` interactivo: Gobernanza → «6. Carril Ágil ODD» → navegar, promover, comprobar espejo, volver | Eliminar `internal/tui/screens/odd_features.go`, revertir adiciones en `governance.go`, `sdd_increments.go`, `router.go` y los cinco puntos de `model.go`; independiente de la unidad 6 |
| 8 | Activos/directrices ODD en castellano y documentación (`ROADMAP.md`, `AGENTS.md`, `GEMINI.md`) | PR 6 | `go test ./internal/assets/...` | N/A — cambios de contenido estático sin superficie ejecutable propia | Revertir los ficheros de `internal/assets/` y la documentación; trivial, sin dependencias de datos |

Diagrama de apilado actualizado:

```
main
 └─ PR1a fundamentos ──► PR1b parser/almacén/espejo ──► PR2 CLI create|status ──► PR3a CreateIncrement ──► PR3b promoción ──┬─► PR4 Web UI ──┐
                                                                                                                             └─► PR5 TUI ─────┴─► PR6 activos y docs
```

PR4 y PR5 dependen ambas de PR3b y ninguna de la otra; se apilan en ese orden por simplicidad de revisión, no por dependencia técnica (igual que en el diseño original).

---

## Reglas de Comprobación y Alcance (aplican a TODAS las fases)

1. **Nunca uses `go test ./...` en verde como criterio de aceptación de una tarea o de una rebanada.** La línea base ya está en rojo hoy (observación Engram `#360`), con seis fallos ambientales conocidos, verificados por reproducción propia:
   - `internal/update`: `TestNoUpdatesPath`, `TestDetectHomebrewOwnershipWith`.
   - `internal/sddstatus`: `TestResolveBindingChangeRootAcceptsAnAliasedRepositoryRoot`, `TestResolveBindingChangeRootStillRejectsAForeignWorkspace`, `TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots`, `TestCanonicalVerifyReportPathsAcceptsTwoSpellingsOfOneWorkspace`.
   Cinco de los seis fallan por falta de privilegio de `os.Symlink` en Windows; el sexto asume una máquina sin `axiom`/`gga` instalados. Ninguna tarea de este documento corrige estos seis fallos: tienen trabajo propio asignado aparte.
2. **Comprobación por paquete durante el trabajo.** Cada tarea de cierre de fase usa `go build ./...`, `go vet ./...` (ambos limpios hoy, sin excepciones) y `go test` acotado a los paquetes que la fase modifica (`./internal/odd/...`, `./internal/cli/...`, `./internal/dashboard/...`, `./internal/tui/...`, `./internal/assets/...`, según corresponda). Nunca se incluye `./internal/sddstatus/...` en una comprobación de tarea: este incremento no lo toca y ese paquete tarda 499 s en aislamiento.
3. **Una única comprobación de suite completa, reservada para el cierre del incremento** (tarea 8.6): `go test ./... -timeout 900s`, con el criterio **"ningún fallo nuevo respecto a los seis conocidos"** — nunca "todo en verde".
4. **Ficheros prohibidos (criterio de aborto, no negociable):** ningún diff de ninguna fase puede contener rutas bajo `internal/sddstatus/`, `internal/cli/sdd_*.go` o `internal/agents/researchcapability/`. `openspec/INDEX.md` y `openspec/config.yaml` permanecen sin modificar en todo el incremento.
5. **No se corrigen en este incremento** (tienen trabajo propio asignado aparte, tocarlos aquí ensancharía el alcance):
   - El misrouting de índices de `ScreenSDDIncrements` (`internal/tui/model.go:3153-3163` frente a `internal/tui/screens/sdd_increments.go:30-36`). El diseño lo neutraliza estructuralmente [D-12, D-13] sin corregirlo; las tareas de la Fase 7 lo respetan explícitamente.
   - `validIncrementNameRegex` aceptando nombres de dispositivo reservados de Windows (`internal/dashboard/service.go:762`) — hallazgo O-2 del diseño, deliberadamente diferido.
6. **Decisiones del orquestador ya reflejadas en las tareas:**
   - **O-1 (aceptada):** el código de salida de `axiom odd status --check-mirror` ante el estado `divergente` es `0` (igual que `no disponible`), valor conservador y ampliable después sin romper compatibilidad. Ver tarea 3.5.
   - **O-3 (regla de proceso):** los documentos `odd/tasks/*.md` que un usuario cree en tiempo de ejecución se versionan en Git pero **no cuentan contra el presupuesto de revisión de 400 líneas** de ningún PR futuro que los incluya. No se materializa en código ni en `.gitignore` (decisión D1 del diseño, ya cerrada: se versionan). No tiene efecto numérico en las 8 rebanadas de este documento, que no comprometen contenido de `odd/tasks/*.md` como fuente propia.
7. **TDD estricto (`strict_tdd: true`):** toda tarea de implementación ordena RED → GREEN → REFACTOR con evidencia observada (comando ejecutado + resultado), nunca inventada. Convención de tests: tabla de `structs` anónimos + `t.Run`, sin `os.Exit` ni `panic` (skill `axiom-go-table-tests`).
8. **Marcado `(read-only)`:** cuando una tarea cita como modelo de estilo un fichero que esta fase no crea ni modifica (por ejemplo, un adaptador `sdd_*.go` existente o `internal/multirole/barrier.go`), la ruta se marca `(read-only)` inmediatamente después de la ruta entre comillas invertidas, para no confundirlo con un objetivo de edición.

---

## Fase 1: PR1a — Fundamentos del documento vivo (REQ-19.1, REQ-19.2)

Paquete nuevo `internal/odd`, sin consumidores todavía. Aditivo puro.

- [x] 1.1 [Infra] Crear `internal/odd/document.go` con los tipos base: `Status` (`StatusActive`, `StatusPromoted`), `SectionID` y sus doce constantes, `CanonicalSections` (slice ordenado con las doce secciones canónicas), `Task`, `Document`, `FeatureSummary`, `StatusReport`; etiquetas `json` en inglés, comentarios GoDoc en castellano [D-14, diseño §5.1]. Sin comportamiento propio: no requiere RED previo.
- [x] 1.2 [Infra] Crear `internal/odd/errors.go` con los siete errores centinela de §5.6 del diseño (`ErrInvalidFeatureName`, `ErrReservedName`, `ErrPathEscape`, `ErrFeatureExists`, `ErrFeatureNotFound`, `ErrAlreadyPromoted`, `ErrMalformedDocument`), mensajes en castellano.
- [x] 1.3 [RED] Escribir `internal/odd/name_test.go` (frontera T-7 de la matriz de amenazas del diseño): tabla sobre `ValidateFeatureName` y `DocumentPath` cubriendo kebab-case válido, mayúsculas, cadena vacía, nombre de 300 caracteres (>64), `..`, `../..`, `/etc/passwd`, `C:\Windows`, `a/b`, `a\b`, y la denylist completa de nombres reservados de Windows en minúsculas (`con`, `prn`, `aux`, `nul`, `com1`…`com9`, `lpt1`…`lpt9`). Verificar además que, tras cada caso rechazado, no aparece ningún fichero fuera de `t.TempDir()`. Ejecutar y registrar el fallo observado (RED).
- [x] 1.4 [GREEN] Crear `internal/odd/name.go`: `ValidateFeatureName` (regex `^[a-z0-9]+(-[a-z0-9]+)*$`, límite de 64 caracteres, denylist de nombres reservados) y `DocumentPath` (`filepath.Join` + comprobación de contención vía `filepath.Rel`) [D-10], hasta que 1.3 quede en verde.
- [x] 1.5 [RED] Escribir `internal/odd/template_test.go`: las doce secciones de `CanonicalSections` presentes y en el orden canónico dentro del texto generado por `RenderNew`; dos invocaciones con la misma entrada producen bytes idénticos (determinismo); el resultado termina en salto de línea.
- [x] 1.6 [GREEN] Crear `internal/odd/template.go`: `RenderNew(feature, today string) string` con la plantilla canónica de doce secciones en castellano peninsular, hasta que 1.5 quede en verde.
- [x] 1.7 [REFACTOR] Revisar `document.go`, `errors.go`, `name.go`, `template.go`: comentarios GoDoc en castellano, envoltura de errores con `%w` donde corresponda, sin duplicación entre `ValidateFeatureName` y `DocumentPath`.
- [x] 1.8 [Verificación de cierre PR1a] Ejecutar `go build ./...`, `go vet ./...` y `go test ./internal/odd/...`; confirmar que el diff de esta rebanada no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`; registrar la evidencia observada (comando + resultado).

## Fase 2: PR1b — Parser, almacén y espejo (REQ-19.1, REQ-19.3, REQ-19.4)

Depende de la Fase 1 (usa `Document`, `SectionID`, `CanonicalSections`, `RenderNew`).

- [x] 2.1 [RED] Escribir `internal/odd/parse_test.go` (forma del documento): cabecera `activo`/`promovido`; sección canónica ausente ⇒ se registra en `Warnings` sin devolver error; tarea de checklist sin identificador estable ⇒ `Warnings`; documento sin encabezado `# ODD: <feature>` ⇒ `ErrMalformedDocument`; normalización de CRLF.
- [x] 2.2 [GREEN] Crear `internal/odd/parse.go`: `Parse(raw string) (*Document, error)` — detección de cabecera, troceado por `##`, checklist con identificadores estables (`T1`, `T2`, …) que sobreviven a reordenaciones (REQ-19.3), hasta que 2.1 quede en verde.
- [x] 2.3 [RED] Extender `internal/odd/parse_test.go` con el caso de progreso [D-04]: documento adversarial con casillas `- [ ]`/`- [x]` en tres secciones distintas (Checklist accionable, Criterios de aceptación, Comprobaciones aplicables), donde solo las de "Checklist accionable" cuentan; checklist vacío produce 0/0 sin división por cero.
- [x] 2.4 [GREEN] Extender `internal/odd/parse.go`: recortar el texto de la sección "Checklist accionable" antes de invocar `multirole.CountTasks` (`internal/multirole/barrier.go:16-41`) (read-only), en vez de pasar el documento completo, hasta que 2.3 quede en verde.
- [x] 2.5 [RED] Escribir `internal/odd/store_test.go` con `t.TempDir()`: `Create` sobre una *feature* ya existente ⇒ `ErrFeatureExists`; `Load` de una *feature* inexistente ⇒ `ErrFeatureNotFound`; `Scan` sin el directorio `odd/tasks/` ⇒ lista vacía sin error; `MarkPromoted` reescribe la cabecera con el estado `promovido`, la referencia al cambio y la fecha.
- [x] 2.6 [GREEN] Crear `internal/odd/store.go`: `Create`, `Load`, `Scan`, `MarkPromoted` — única capa del paquete con E/S de ficheros — hasta que 2.5 quede en verde.
- [x] 2.7 [RED] Escribir `internal/odd/mirror_test.go` con un `Exporter` falso instrumentado: estado `sincronizado`; estado `divergente`; exportador que devuelve error, que devuelve una lista vacía, que devuelve observaciones sin `Topic` coincidente, y con `Topic` coincidente pero sin el centinela `# ODD: <feature>` ⇒ los cuatro casos proyectan `no disponible` con un `Reason` distinto y descriptivo cada uno; normalización de CRLF y espacios finales antes de comparar SHA-256.
- [x] 2.8 [GREEN] Crear `internal/odd/mirror.go`: `MirrorState`, `MirrorReport`, `Observation`, el puerto `Exporter`, y `CheckMirror(ctx, doc, export) MirrorReport` (extracción por centinela + comparación SHA-256 tras normalizar) [D-05], hasta que 2.7 quede en verde.
- [x] 2.9 [RED] Extender `internal/odd/mirror_test.go` (frontera T-6 de la matriz de amenazas): `DefaultExporter` con `PATH` apuntando a un directorio vacío (`t.Setenv("PATH", t.TempDir())`) ⇒ error, proyectado por `CheckMirror` a `no disponible`; contexto ya cancelado (`context.WithCancel` cancelado antes de invocar) ⇒ `no disponible` sin bloqueo; verificar que el fichero temporal creado por `DefaultExporter` no existe tras el retorno en ningún caso.
- [x] 2.10 [GREEN] Extender `internal/odd/mirror.go`: `DefaultExporter(ctx, root) ([]Observation, error)` vía `exec.CommandContext` con `argv` como *slice* literal de tres elementos (nunca interpolación de cadena), *timeout* de 10 s, fichero temporal por `os.CreateTemp` + `defer os.Remove`, hasta que 2.9 quede en verde.
- [x] 2.11 [RED] Escribir `internal/odd/render_test.go`: `RenderStatusText` produce la salida esperada para 0, 1 y N *features*, incluyendo la línea de estado del espejo cuando `MirrorReport` está presente.
- [x] 2.12 [GREEN] Crear `internal/odd/render.go`: `RenderStatusText(report) string` para la salida de texto de la CLI, hasta que 2.11 quede en verde.
- [x] 2.13 [Infra] Crear `odd/tasks/.gitkeep` para materializar el directorio versionado (decisión D1 cerrada: los documentos ODD se versionan en Git).
- [x] 2.14 [REFACTOR] Revisar `parse.go`, `store.go`, `mirror.go`, `render.go`: envoltura de errores con `%w`, comentarios en castellano; confirmar que no se duplica el patrón de `internal/dashboard/service.go` (read-only) ni de `internal/tui/model.go` (read-only) más allá de la reutilización deliberada de `multirole.CountTasks` [D-11].
- [x] 2.15 [Verificación de cierre PR1b] Ejecutar `go build ./...`, `go vet ./...` y `go test ./internal/odd/...` (paquete completo); confirmar que el diff acumulado de las Fases 1+2 no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`; registrar la evidencia observada.

## Fase 3: PR2 — CLI `axiom odd create` y `axiom odd status` (REQ-19.5, REQ-19.6, REQ-19.7, parte de REQ-19.8)

Depende de las Fases 1 y 2.

- [ ] 3.1 [RED] Escribir `internal/odd/args_test.go` (frontera T-2 de la matriz de amenazas): tabla para `ParseCreateArgs`, `ParseStatusArgs` y `ParsePromoteArgs` — banderas conocidas y desconocidas, posicional ausente, `--json` combinado con `--check-mirror`, `--cwd` relativo, `--cwd` absoluto, `--cwd` ausente (por defecto `"."`), y las banderas `--name`/`--dry-run`/`--intent`/`--type` de `promote` (su uso en la CLI llega en la Fase 5; el parseo se cubre aquí).
- [ ] 3.2 [GREEN] Crear `internal/odd/args.go`: `ParseCreateArgs`, `ParseStatusArgs`, `ParsePromoteArgs` — parseo puro, sin `io.Writer`, siguiendo el precedente de `sddstatus.ParseCommandArgs` (read-only) — hasta que 3.1 quede en verde.
- [ ] 3.3 [RED] Escribir `internal/cli/odd_create_test.go` (T-2): `RunODDCreate` con `bytes.Buffer` como stdout — creación exitosa con `--cwd` relativo y con `--cwd` absoluto imprime la ruta creada y devuelve `nil`; nombre inválido o en colisión devuelve error sin crear el fichero; `--cwd` inexistente devuelve error **antes** de cualquier `MkdirAll`, con cero directorios creados.
- [ ] 3.4 [GREEN] Crear `internal/cli/odd_create.go`: `RunODDCreate(args []string, stdout io.Writer) error`, adaptador fino sobre `odd.Create`, al estilo de `internal/cli/sdd_status.go:13-40` (read-only), hasta que 3.3 quede en verde.
- [ ] 3.5 [RED] Escribir `internal/cli/odd_status_test.go`: salida en texto por defecto vs `--json`; `--check-mirror` devuelve **exit 0** para los tres estados del espejo (`sincronizado`, `divergente`, `no disponible`) [D-05, decisión O-1 confirmada]; sin `--check-mirror` no se invoca ninguna comparación contra Engram; *feature* nombrada inexistente devuelve error con exit distinto de 0.
- [ ] 3.6 [GREEN] Crear `internal/cli/odd_status.go`: `RunODDStatus(args []string, stdout io.Writer) error` con ramas texto/`--json` y `--check-mirror` opcional que solo invoca `odd.CheckMirror` cuando la bandera está presente, hasta que 3.5 quede en verde.
- [ ] 3.7 [RED] Escribir una tabla de enrutamiento (frontera T-8 de la matriz de amenazas) en `internal/cli/odd_create_test.go` o en un test de enrutamiento de `cmd/axiom` si ya existe convención: `odd` sin argumentos, `odd --help`, `odd -h`, `odd inexistente`, `odd create ...`, `odd status ...`; confirmar que no existía ya un `case "odd"` en el `switch` de `main.go:179` antes de este cambio.
- [ ] 3.8 [GREEN] Modificar `cmd/axiom/main.go`: añadir `case "odd":` al `switch arg1` (`:179`) y la función `runODD(args []string, stdout, stderr io.Writer) int` calcada de `runSDD` (`main.go:1704-1750`, mismo fichero), con los subcomandos `create` y `status` únicamente (`promote` se añade en la Fase 5); ayuda con exit 1 cuando falta el subcomando, exit 0 con `--help`/`-h`, mensaje explícito y exit 1 ante subcomando desconocido [D-16, sin alias hifenados], hasta que 3.7 quede en verde.
- [ ] 3.9 [GREEN] Modificar `cmd/axiom/main.go`: añadir las líneas `axiom odd create|status|promote` en `printHelp()` dentro de "COMANDOS DE GOBERNANZA Y WORKSPACE", junto a `change create` y `sdd status` (`:56-123`), y un ejemplo `axiom odd status` en la sección de ejemplos (REQ-19.8; la entrada de `promote` se documenta como texto de ayuda aquí, su implementación llega en la Fase 5).
- [ ] 3.10 [REFACTOR] Revisar `internal/odd/args.go`, `internal/cli/odd_create.go`, `internal/cli/odd_status.go`: mensajes de error en castellano, consistencia con los adaptadores CLI existentes, comentarios GoDoc.
- [ ] 3.11 [Verificación de cierre PR2] Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/odd/... ./internal/cli/...`; probar manualmente `axiom odd create demo`, `axiom odd status`, `axiom odd status --json` y `axiom odd status --check-mirror` sobre un workspace temporal; confirmar que el diff no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`.

## Fase 4: PR3a — Extensión protegida de `CreateIncrement` (REQ-15.1)

**Puerta de control de máxima severidad (riesgo R3 de la propuesta).** No depende de `internal/odd`; se apila tras la Fase 3 por coherencia narrativa con la promoción, pero es técnicamente independiente.

- [ ] 4.1 [RED — PUERTA DE CONTROL, primera tarea de la rebanada, obligatoria antes de tocar `CreateIncrement`] Crear `internal/dashboard/create_increment_characterization_test.go` con `TestCreateIncrement_PlantillaVigenteSinCuerpoSembrado`: capturar los bytes exactos que `CreateIncrement` produce **hoy, antes de cualquier cambio**, para tres casos — intento y tipo explícitos, intento vacío (usa el texto por defecto), tipo vacío — comparando con `!=` sobre la cadena íntegra (nunca `strings.Contains`) [D-03]. El literal esperado se transcribe de la salida real del binario actual. Ejecutar contra el código sin modificar: debe pasar en **VERDE** (caracteriza el comportamiento vigente, no una regresión).
- [ ] 4.2 [RED] Extender `create_increment_characterization_test.go` con un caso que envíe `CreateIncrementRequest` con un campo `ProposalBody` no vacío: hoy ese campo no existe, así que este caso falla en compilación o en verde-falso — registrar el fallo observado.
- [ ] 4.3 [GREEN] Modificar `internal/dashboard/types.go`: añadir `ProposalBody string \`json:"proposal_body,omitempty"\`` a `CreateIncrementRequest` (`:122-126`) [D-02], hasta que 4.2 compile.
- [ ] 4.4 [RED] Completar el caso de 4.2: con `ProposalBody` no vacío, `proposal.md` debe contener exactamente esos bytes, sin la plantilla generada. Confirmar que sigue en rojo (la rama condicional aún no existe).
- [ ] 4.5 [GREEN] Modificar `internal/dashboard/service.go`: insertar la rama condicional de tres líneas entre `:836` y `:838` — `if strings.TrimSpace(req.ProposalBody) != "" { proposalContent = req.ProposalBody }` [D-02] — hasta que 4.4 quede en verde. **Confirmar explícitamente que el caso original de 4.1 (sin `ProposalBody`) sigue en VERDE byte a byte, sin ninguna modificación de su literal esperado.**
- [ ] 4.6 [RED] Extender `create_increment_characterization_test.go` con un test de integración vía `httptest.NewServer(srv.Router())`: `POST /api/increments` sin `proposal_body` produce la plantilla vigente; **con** `proposal_body` produce exactamente esos bytes (regresión de REQ-15.1 sobre el endpoint ya existente, extremo a extremo).
- [ ] 4.7 [GREEN] Confirmar 4.6 en verde; si falla, ajustar únicamente la deserialización/paso de datos, nunca el contrato de `CreateIncrementRequest` para el caso sin `proposal_body`.
- [ ] 4.8 [REFACTOR] Revisar `create_increment_characterization_test.go`: nombres de test descriptivos, tabla `t.Run` conforme a la skill `axiom-go-table-tests`, sin literales duplicados entre casos.
- [ ] 4.9 [Verificación de cierre PR3a — puerta de control del incremento] Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/dashboard/...`; confirmar que `TestCreateIncrement_PlantillaVigenteSinCuerpoSembrado` (caso sin `ProposalBody`) sigue en VERDE byte a byte; confirmar que `axiom change create` y `POST /api/increments` sin `proposal_body` no cambian de comportamiento observable; confirmar que el diff no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`.

## Fase 5: PR3b — Dominio y CLI de la promoción ODD → SDD (REQ-19.9, REQ-19.10, REQ-19.11, REQ-19.12, resto de REQ-19.8)

Depende de las Fases 1, 2 (dominio `odd`), 3 (`runODD`/`args.go`) y 4 (campo `ProposalBody`).

- [ ] 5.1 [RED] Escribir `internal/odd/promote_test.go` — cuerpo sembrado (D-09, frontera T-9 de la matriz de amenazas): tabla sobre `RenderProposalBody` — `## Capacidades` contiene únicamente el marcador de pendiente para `sdd-spec` y ninguna capacidad inventada; cada sección vacía del documento origen produce `_(sin contenido en el documento ODD de origen)_`; el cuerpo sembrado conserva bytes idénticos a las secciones de entrada, sin sanear ni reescribir Markdown del usuario; la cabecera de procedencia (`odd/tasks/<feature>.md`, fecha) está siempre presente, incluso cuando el documento origen la contradice; la tabla "Estado heredado de ODD" refleja el estado real (`- [x]`/`- [ ]`) de cada tarea del checklist.
- [ ] 5.2 [GREEN] Crear `internal/odd/promote.go` (primera parte): `RenderProposalBody(doc *Document, changeName, today string) string` con el mapeo exacto de §5.8 del diseño, hasta que 5.1 quede en verde.
- [ ] 5.3 [RED] Extender `internal/odd/promote_test.go` — ciclo de promoción con un `Scaffolder` falso instrumentado: documento ya promovido ⇒ `ErrAlreadyPromoted` y el `Scaffolder` falso **no se invoca** [D-08]; `--dry-run` ⇒ cero invocaciones del `Scaffolder` y cero escrituras; `Scaffolder` que devuelve error ⇒ el documento ODD permanece intacto (sin marca); fallo simulado al escribir la marca (`MarkPromoted`) tras una creación exitosa ⇒ `PromoteResult.Warning` poblado, `MarkWritten == false`, y el mensaje de remediación exacto de D-07.
- [ ] 5.4 [GREEN] Extender `internal/odd/promote.go`: interfaz `Scaffolder`, `ScaffoldRequest`, `ScaffoldResult`, `PromoteOptions`, `PromoteResult`, y `Promote(opts, sc)` con la secuencia de dos fases — validar → renderizar → `Scaffold` → `MarkPromoted` — nunca al revés [D-01, D-07], hasta que 5.3 quede en verde.
- [ ] 5.5 [RED] Crear `internal/dashboard/name_parity_test.go`: test de propiedad sobre un corpus compartido de nombres — todo nombre aceptado por `odd.ValidateFeatureName` también casa con `validIncrementNameRegex` (`internal/dashboard/service.go:762`) [D-10]. Único paquete con visibilidad de la regex no exportada.
- [ ] 5.6 [GREEN] Si 5.5 falla, ajustar **únicamente** `internal/odd/name.go` (nunca `validIncrementNameRegex`, cuya corrección de nombres reservados de Windows queda deliberadamente fuera de alcance — hallazgo O-2) hasta que la propiedad se cumpla.
- [ ] 5.7 [RED] Escribir `internal/cli/odd_promote_test.go`: `RunODDPromote` con un `Scaffolder` falso — resumen y exit 0 en éxito; `--dry-run` imprime el cuerpo por stdout con exit 0 y cero escrituras (ni propuesta, ni directorio de cambio, ni marca); *feature* ya promovida ⇒ exit 1 con mensaje de idempotencia; colisión de nombre (activo o archivado) ⇒ exit 1 con mensaje de colisión; fallo de marca tras creación exitosa ⇒ exit 1 con la línea de remediación exacta de D-07.
- [ ] 5.8 [GREEN] Crear `internal/cli/odd_promote.go`: `RunODDPromote` y `dashboardScaffolder` (adaptador que satisface `odd.Scaffolder` sobre `dashboard.Service.CreateIncrement`; único punto del incremento donde `internal/cli` importa `internal/dashboard` [D-01]), hasta que 5.7 quede en verde.
- [ ] 5.9 [RED] Extender la tabla de enrutamiento T-8 de la tarea 3.7 con el subcomando `promote` completo: `odd promote <feature>`, `odd promote <feature> --dry-run`, `odd promote <feature> --name <otro>`.
- [ ] 5.10 [GREEN] Modificar `cmd/axiom/main.go`: añadir el subcomando `promote` (banderas `--name`, `--intent`, `--type`, `--dry-run`) a `runODD` y completar su línea en el texto de ayuda de `runODD` y de `printHelp()` (REQ-19.8), hasta que 5.9 quede en verde.
- [ ] 5.11 [REFACTOR] Revisar `internal/odd/promote.go`, `internal/cli/odd_promote.go`: envoltura de errores con `%w`, mensajes en castellano, sin duplicación con `odd_create.go`/`odd_status.go`.
- [ ] 5.12 [Verificación de cierre PR3b] Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/odd/... ./internal/dashboard/... ./internal/cli/...`; probar manualmente `axiom odd promote demo --dry-run` (confirmar cero escrituras) y `axiom odd promote demo` (confirmar `openspec/changes/demo/proposal.md` y la marca en `odd/tasks/demo.md`) sobre un workspace temporal; confirmar que `TestCreateIncrement_PlantillaVigenteSinCuerpoSembrado` (Fase 4) sigue en verde; confirmar que el diff no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`.

## Fase 6: PR4 — Web UI (REQ-19.13, parte de REQ-19.15)

Depende de la Fase 5 (usa `odd.Scan`, `odd.Load`, `odd.Create`, `odd.Promote`, `odd.CheckMirror` y los tipos del dominio). Independiente de la Fase 7.

- [ ] 6.1 [RED] Escribir `internal/dashboard/odd_service_test.go`: `GetODDFeatures` sobre un workspace de prueba (`t.TempDir()` + `odd.Create`); `GetODDFeature` con *feature* existente e inexistente; `CreateODDFeature` delega en `odd.Create` y traduce sus errores a los de la API; `PromoteODDFeature` delega en `odd.Promote`; `CheckODDMirror` delega en `odd.CheckMirror` con un `Exporter` inyectado.
- [ ] 6.2 [GREEN] Crear `internal/dashboard/odd_service.go`: `GetODDFeatures`, `GetODDFeature`, `CreateODDFeature`, `PromoteODDFeature`, `CheckODDMirror` sobre `*Service`, fichero propio para no inflar `service.go`, hasta que 6.1 quede en verde.
- [ ] 6.3 [RED] Añadir (en `odd_service_test.go` o un test de `types.go` si existe convención) casos que confirmen que `ODDCreateRequest`, `ODDPromoteRequest`, `ODDMirrorRequest` deserializan correctamente desde JSON de entrada.
- [ ] 6.4 [GREEN] Modificar `internal/dashboard/types.go`: añadir los tres DTO de **entrada** únicamente — `ODDCreateRequest`, `ODDPromoteRequest{Feature, Name, DryRun}`, `ODDMirrorRequest{Feature}` [D-15]; confirmar que `IncrementDetailDTO` y el resto de tipos existentes no cambian de forma, hasta que 6.3 quede en verde.
- [ ] 6.5 [RED] Extender `internal/dashboard/dashboard_test.go` con `TestODDEndpoints` (mismo patrón que `TestArchiveEndpoints`/`TestProjectsEndpoints`, vía `httptest.NewServer(server.Router())`): `GET /api/odd` lista `[]odd.FeatureSummary`; `POST /api/odd` crea (201) y rechaza nombre inválido (400); `POST /api/odd/promote` con `dry_run:true` no escribe nada; `POST /api/odd/check-mirror` devuelve los tres estados según el `Exporter` inyectado; método no permitido en cada ruta (405); `GET /api/odd/{feature}` no colisiona con `POST /api/odd/promote` (resolución de patrón más largo del `ServeMux`, igual que `/api/increments/continue` frente a `/api/increments/`).
- [ ] 6.6 [GREEN] Modificar `internal/dashboard/server.go`: registrar las cinco rutas en `registerRoutes()` (`:40-72`) y sus manejadores, calcados de `handleIncrements` (`:251-275`, mismo fichero); orden de registro que evite colisión de patrón (rutas más específicas antes que `/api/odd/`), hasta que 6.5 quede en verde.
- [ ] 6.7 [Implementación de interfaz, sin RED de Go] Modificar `internal/dashboard/assets/index.html`: botón de navegación `data-tab="tab-odd"` tras `tab-increments` (`:40-42`) y `<section id="tab-odd">` con cabecera, filtros (Todos/Activos/Promovidos), botón «+ Nuevo documento ODD», botón «Comprobar espejo» y contenedor `#odd-container`.
- [ ] 6.8 [Implementación de interfaz, sin RED de Go] Modificar `internal/dashboard/assets/app.js`: `loadODD()`, `renderODDFeatures()`, `createODDFeature()`, `promoteODDFeature()`, `checkODDMirror()`, `focusIncrement(name)`; en `renderIncrements()` (`:704`), insignia «← Origen ODD» construida desde el mapa cliente `promoted_to → feature`, sin cambios en el endpoint de incrementos.
- [ ] 6.9 [Implementación de interfaz, sin RED de Go] Modificar `internal/dashboard/assets/style.css`: `.lane-badge`, `.lane-badge--promoted`, `.mirror-state--*`, reutilizando `.increments-grid` y las clases de tarjeta existentes.
- [ ] 6.10 [Runtime harness manual — REQ-19.13] Levantar `axiom ui` sobre un workspace de prueba con documentos ODD variados (activo, promovido, ninguno) y verificar visualmente: listado con progreso, insignia de promovido, filtros, comprobación de espejo, salto al cambio SDD promovido, ausencia de recarga manual de página. Registrar el resultado observado.
- [ ] 6.11 [REFACTOR] Revisar `odd_service.go`, los cambios en `server.go` y los tres ficheros de `assets/`: nomenclatura consistente con el patrón de incrementos existente, mensajes en castellano.
- [ ] 6.12 [Verificación de cierre PR4] Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/dashboard/...`; confirmar que el dashboard SDD existente (`/api/increments`, pestaña de incrementos) sigue funcionando sin cambio de comportamiento; confirmar que el diff no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`.

## Fase 7: PR5 — TUI (REQ-19.14, resto de REQ-19.15)

Depende de la Fase 5. Independiente de la Fase 6.

- [ ] 7.1 [RED] Escribir `internal/tui/screens/odd_features_test.go`: tabla para `ODDFeaturesActionAt` — acción correcta en cada posición del cursor con 0, 1 y N *features* (`ODDActionSelectFeature`, `ODDActionCreate`, `ODDActionPromote`, `ODDActionCheckMirror`, `ODDActionGoToSDDLane`, `ODDActionBack`); `screenOptionCount` debe coincidir con `len(ODDFeaturesOptions(...))` para cada tamaño de lista.
- [ ] 7.2 [GREEN] Crear `internal/tui/screens/odd_features.go`: `ODDFeatureInfo`, `ODDFeaturesOptions`, tipo `ODDAction` con sus constantes, `ODDFeaturesActionAt` (resolvedor tipado, única fuente de verdad del mapeo cursor → acción [D-12]), `RenderODDFeatures`, hasta que 7.1 quede en verde.
- [ ] 7.3 [RED] Extender (o crear) `internal/tui/screens/governance_test.go`: `GovernanceOptions()` tiene 7 entradas tras añadir «6. Carril Ágil ODD (documentos vivos, promoción)» antes de «Volver al menú principal»; el texto de la nueva opción coincide con el especificado.
- [ ] 7.4 [GREEN] Modificar `internal/tui/screens/governance.go`: insertar la nueva entrada en `GovernanceOptions()` antes de "Volver al menú principal" (`:11-18`), hasta que 7.3 quede en verde.
- [ ] 7.5 [RED] Extender (o crear) `internal/tui/screens/sdd_increments_test.go`: `RenderSDDIncrements` incluye la línea de cabecera "Carril activo: SDD · esc → Gobernanza → «6» para el carril ágil (ODD)"; `SDDIncrementsOptions()` conserva **exactamente** su longitud y contenido previos (protege D-13: cambio de solo renderizado; no toca el manejador `internal/tui/model.go:3153-3163` (read-only), cuyo misrouting preexistente es conocido y está fuera de alcance).
- [ ] 7.6 [GREEN] Modificar `internal/tui/screens/sdd_increments.go`: añadir la línea de cabecera en `RenderSDDIncrements` sin modificar `SDDIncrementsOptions` [D-13], hasta que 7.5 quede en verde.
- [ ] 7.7 [RED] Extender `internal/tui/model_test.go`: `Model.Update(tea.KeyMsg{...})` — Enter en `ScreenGovernance` con cursor 5 ⇒ `ScreenODDFeatures`; Enter con cursor 6 ⇒ `ScreenWelcome` (protege el desplazamiento de «Volver»); Esc en `ScreenODDFeatures` ⇒ `ScreenGovernance`; Enter sobre una *feature* promovida en `ScreenODDFeatures` ⇒ `ScreenSDDIncrements` con `SDDActiveChange` fijado a `PromotedTo`.
- [ ] 7.8 [GREEN] Modificar `internal/tui/model.go` — aplicar los cinco puntos de integración juntos en el mismo cambio (omitir cualquiera produce una pantalla muda o un cursor fuera de rango): constante `ScreenODDFeatures` (junto al bloque `ScreenGovernance`/`ScreenSDDIncrements`, `:582-584`); `case ScreenODDFeatures` en `View()` (junto a `:1662`); `case ScreenODDFeatures` en `screenOptionCount` (junto a `:4532`); en el manejador de `ScreenGovernance` (`:3114-3134`) nuevo `case 5` (navega a `ScreenODDFeatures` tras `loadODDFeatures()`) y `case 6` ahora para "Volver" (antes `case 5`); nuevo `case ScreenODDFeatures` en el manejador de Enter usando `screens.ODDFeaturesActionAt`; `loadODDFeatures()` definido junto a `loadSDDIncrements()` (`:5882`). **No modificar el manejador de `ScreenSDDIncrements` (`:3153-3163`)** — su misrouting preexistente es conocido y está fuera de alcance [D-12, D-13]. Ejecutar hasta que 7.7 quede en verde.
- [ ] 7.9 [GREEN] Modificar `internal/tui/router.go`: añadir `ScreenODDFeatures: {Backward: ScreenGovernance}` al mapa `linearRoutes`, junto a `ScreenLivingDoc: {Backward: ScreenGovernance}` (`:69`).
- [ ] 7.10 [REFACTOR] Revisar los cinco ficheros tocados en esta fase: nomenclatura consistente con `ScreenSDDIncrements`/`ScreenMultiRole`, comentarios en castellano.
- [ ] 7.11 [Verificación de cierre PR5] Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/tui/...`; navegar manualmente `axiom tui` desde Gobernanza hasta la pantalla ODD y de vuelta, confirmando que ninguna otra pantalla de Gobernanza (Hub, Incrementos SDD, Multi-Rol, Handoffs, Specs Vivas) perdió su cursor o su ruta de retroceso; confirmar que el diff no contiene ficheros bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`.

## Fase 8: PR6 — Activos y documentación (cierre del incremento)

Independiente de las Fases 6 y 7 en cuanto a compilación; cierra la entrega completa del incremento.

- [ ] 8.1 [RED] Extender la batería existente `internal/assets/assets_test.go` con casos para las nuevas directrices/plantillas ODD: renderizado determinista (misma entrada ⇒ mismos bytes) y ausencia de vínculo evento → acción, conforme a `organic-agent-trigger-rules` (riesgo R7).
- [ ] 8.2 [GREEN] Crear/editar las directrices, prompts y plantillas ODD en castellano peninsular bajo la Persona `axiom` en `internal/assets/`, siguiendo la convención ya vigente de esa Persona en el mismo directorio; instalación solo-texto, sin router de ciclo de vida, hasta que 8.1 quede en verde.
- [ ] 8.3 [Documentación] Modificar `docs/ROADMAP.md`: reescribir la entrada de INC-19 (`:278-289`) con el alcance real (carril ODD operativo + puerta de promoción; poda del motor SDD diferida a INC-20), corregir la fila de catálogo (`:66`), dar de alta la entrada de INC-20 (retirada del contrato del motor SDD: `internal/sddstatus`, `axiom sdd attempt`, `internal/agents/researchcapability`), y ajustar el contador de la Fase 4 (`:8`).
- [ ] 8.4 [Documentación] Modificar `AGENTS.md` y `GEMINI.md`: añadir referencia breve a los comandos `axiom odd create|status|promote` ya operativos (cambio menor; la doctrina del carril dual ya está escrita en ambos ficheros).
- [ ] 8.5 [Verificación] Confirmar explícitamente que `.gitignore` permanece sin cambios (decisión D1 cerrada: los documentos ODD se versionan) y que `odd/tasks/.gitkeep` (creado en la Fase 2) sigue versionado.
- [ ] 8.6 [Verificación de cierre del incremento — suite completa reservada] Ejecutar `go build ./...` y `go vet ./...` (deben quedar limpios); ejecutar `gofmt -l` y confirmar que no señala ningún fichero tocado por el incremento (riesgo R9); ejecutar **una única vez**, como comprobación reservada de cierre, `go test ./... -timeout 900s` y confirmar el criterio **"ningún fallo nuevo respecto a los seis conocidos"** de la línea base (observación Engram `#360`: `TestNoUpdatesPath`, `TestDetectHomebrewOwnershipWith` en `internal/update`; `TestResolveBindingChangeRootAcceptsAnAliasedRepositoryRoot`, `TestResolveBindingChangeRootStillRejectsAForeignWorkspace`, `TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots`, `TestCanonicalVerifyReportPathsAcceptsTwoSpellingsOfOneWorkspace` en `internal/sddstatus`); confirmar que ningún diff de ninguna fase (1–8) contiene ficheros bajo `internal/sddstatus/`, `internal/cli/sdd_*.go` o `internal/agents/researchcapability/`, y que `openspec/INDEX.md` y `openspec/config.yaml` permanecen sin modificar.

---

## Trazabilidad rápida (fase → requerimiento)

| Fase | Requerimientos cubiertos |
|---|---|
| 1, 2 | REQ-19.1, REQ-19.2, REQ-19.3, REQ-19.4 |
| 3 | REQ-19.5, REQ-19.6, REQ-19.7, parte de REQ-19.8 |
| 4 | REQ-15.1 (modificado) |
| 5 | REQ-19.9, REQ-19.10, REQ-19.11, REQ-19.12, resto de REQ-19.8 |
| 6 | REQ-19.13, parte de REQ-19.15 |
| 7 | REQ-19.14, resto de REQ-19.15 |
| 8 | Transversal (documentación de producto; sin escenario BDD propio) |
