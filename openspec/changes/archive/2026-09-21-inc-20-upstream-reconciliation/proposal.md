# Propuesta: Reconciliación con Upstream e Identidad de Distribución (inc-20-upstream-reconciliation)

> **Incremento:** `inc-20-upstream-reconciliation`
> **Fase del roadmap:** Fase 4 — Evolución Arquitectónica, Flujo Orgánico (ODD) y Absorción Upstream v3
> **Responsabilidad:** Arquitectura / Integración Upstream / Distribución
> **Fecha:** 2026-09-18
> **Idioma:** Español (castellano peninsular)
> **Sustituye por completo** el alcance planificado de INC-20 en `docs/ROADMAP.md:67,291-299` (`inc-20-sdd-engine-contract-retirement`).

---

## 0. Resumen ejecutivo

El INC-20 planificado —retirada del contrato publicado de *attempts*, presupuesto y remediación— **está obsoleto: upstream ya ejecutó esa poda**. Ejecutarlo tal y como está escrito reimplementaría trabajo existente sobre una base que ya no coincide con su origen. Este incremento lo reescribe como lo que el terreno exige: **reconciliar el fork con `Gentleman-Programming/gentle-ai` de forma auditada, por tandas temáticas, sin perder los 19 incrementos de producto propio.**

| Pregunta | Respuesta |
|---|---|
| ¿Qué se entrega? | Un protocolo de absorción verificable con su **registro durable de absorción**, la identidad de distribución de Axiom resuelta en todo su inventario de contrato publicado, cobertura de CI sobre el binario real `cmd/axiom`, y la absorción por tandas de los 55 commits de upstream ausentes. |
| ¿Por qué no un merge, un rebase o un re-fork? | Los tres están descartados con motivo (§4.2). La intersección real de conflicto son **131 ficheros**; la absorción por tandas temáticas es la única que mantiene cada rebanada revisable y revertible. |
| ¿Qué defecto vivo destapó el mapeo? | **El instalador del fork instala upstream, no Axiom** (`scripts/install.sh:17-19`). Y **el CI nunca ejercita `cmd/axiom`**: valida el wrapper deprecado `cmd/gentle-ai`. Ese hueco explica que el enrutado roto de `axiom review start` sobreviviera meses. |
| ¿Cuál es el riesgo dominante? | **Recaer en INC-18**: absorber a medias y certificar verde con una suite filtrada. Se combate con dos reglas antirrecaída elevadas a criterio de éxito verificable (§10.1). |
| ¿Está listo para ejecutar? | **Parcialmente.** Las fases F0 y F1 dependen de una decisión de producto no resuelta (**D2**, §9.2) que además está en el **camino crítico de toda la absorción**. La fase F6 depende de **D1** (§9.1). Ninguna de las dos se resuelve aquí. |

---

## 1. Propósito (Intent)

### 1.1 El problema

**a) El alcance planificado de INC-20 ya no existe como problema.** Medido sobre `upstream/main` el 2026-09-18: `internal/sddstatus/runtime_ledger.go` tiene 4255 líneas en el fork y 919 en upstream —upstream ya podó 3336—; `internal/agents/researchcapability` está eliminado en upstream y sigue presente en el fork; `internal/cli/sdd_attempt.go` está acompañado en upstream de `sdd_attempt_retirement_test.go`. Los commits que lo cubren son `18fa04fb`, `15cbbde4`, `ba3ed690`, `62ce74b7` y `e0774e05`.

**b) La divergencia crece de forma monótona y ya no es trivial.** Desde el ancestro común `266574b0`, el fork tocó 600 ficheros y upstream 931, con una **intersección de conflicto real de 131 ficheros**. Faltan 77 commits de upstream (55 sin contar merges) y upstream no tiene 67 commits propios. Zonas calientes de la intersección: `internal/cli/` (`review_*.go`, `sdd_*.go`, `doctor.go`, `run.go`, `sync.go`), `internal/components/sdd/`, `internal/sddstatus/` (`status.go`, `review_door.go`, `runtime_ledger.go`), `internal/assets/`, unos 30 ficheros de `testdata/golden/` e `internal/tui/`.

**c) La identidad de Axiom nunca alcanzó la superficie de distribución.** El rebranding de INC-01 y INC-14 se quedó en el producto; el contrato publicado sigue siendo el de upstream:

| Sitio | Estado verificado | Tipo de contrato |
|---|---|---|
| `scripts/install.sh:17-19` | `GITHUB_OWNER="Gentleman-Programming"`, `GITHUB_REPO="gentle-ai"`, `BINARY_NAME="gentle-ai"` | Instalador público |
| `scripts/install.ps1`, tap de Homebrew | Mismo estado | Instalador público |
| `.github/workflows/ci.yml` | Compila y ejecuta `./cmd/gentle-ai`; **ningún paso ejercita `cmd/axiom`** | CI/CD + hueco de cobertura |
| `scripts/verify-release-assets.sh`, `release-preflight.sh`, `promote-stable-preflight.sh` | Comprueban `$GITHUB_REPOSITORY == "Gentleman-Programming/gentle-ai"` | Compuerta de release |
| `scripts/crosslane/*.go` | Crea un shim ejecutable `gentle-ai` en `$PATH`; parsea `words[0] != "gentle-ai"` | Activo de pruebas de revisor |
| `contracts/**/*.schema.json` | `$id: https://gentle-ai.dev/...`; prefijos `gentle-ai.review-integration/v1`, `gentle-ai.sdd-status/v2` como valores reales | **Namespace de protocolo publicado (contrato de interoperabilidad)** |
| `go.mod` y 686 ficheros | `github.com/gentleman-programming/gentle-ai/v2`; el fork no cita `/v3` en ningún sitio | Ruta de módulo Go |
| `deploy/telemetry/*.service`, dashboards Grafana | Nombres de servicio de upstream | Despliegue |

**Consecuencia directa: quien ejecuta hoy el instalador del fork no instala Axiom.** Descarga el release de upstream. Esto no es deuda cosmética; es que el producto no se distribuye.

**d) El CI valida el binario equivocado.** `cmd/axiom` son 1919 líneas con una capa de producto entera (`init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui`) que **ningún paso de CI ha ejecutado nunca**. Todo el pipeline valida el wrapper deprecado.

### 1.2 Por qué ahora

1. **Hay base verde por primera vez, y es perecedera.** El PR #22 (`fix/saneamiento-ci-main`) deja los 38 tests y las 49 aserciones E2E en verde. Una reconciliación sobre árbol rojo es **inverificable por construcción**: ante un fallo no se puede distinguir lo heredado de lo absorbido. Ese es exactamente el suelo que INC-18 no tuvo.
2. **El coste de reconciliar crece con el tiempo, no con el trabajo.** Cada commit de upstream que se acumula amplía la intersección de 131 ficheros. Cada incremento propio la amplía también. Esperar no abarata nada.
3. **El defecto de distribución es una pérdida de producto en curso.** No tiene dependencia de upstream y puede cerrarse antes que cualquier absorción.
4. **El mapeo ya está hecho.** Los 55 commits están taxonomizados por zona y riesgo (§4.3). Esa medición caduca en cuanto upstream avanza.

### 1.3 Cómo se ve el éxito

Un mantenedor puede responder, con evidencia en el repositorio y sin arqueología en Git: **qué commit de upstream está absorbido, cuál se descartó deliberadamente y por qué**. El instalador instala Axiom. El CI ejercita el binario que la gente usa. Y ningún «PASS 100 %» vuelve a apoyarse en una suite filtrada.

---

## 2. Alcance (Scope)

### 2.1 Dentro de alcance

1. **Protocolo de absorción verificable**, con sus **dos reglas antirrecaída** (§4.1) y el **registro durable de absorción** `docs/upstream-absorption-ledger.md` (commit de upstream → tanda del fork → estado: `absorbido` | `descartado-deliberadamente` | `revertido`), con espejo en Engram.
2. **Identidad de distribución resuelta** en todo el inventario de §1.1(c), según lo que decida **D2**: instalador, tap, compuertas de release, workflows, shim de `crosslane`, namespace de `contracts/**`, ruta de módulo Go y nombres de servicio de telemetría.
3. **Cobertura de CI sobre `cmd/axiom`**: paso de construcción y ejercicio del binario real, incluida la superficie exclusiva de Axiom. Ver §4.4 para el tratamiento de lo que ese paso destape.
4. **Absorción de los 55 commits de upstream sin merges**, por tandas temáticas (§4.3), cada una con su lista de ficheros derivada y su verificación completa.
5. **Migración de la ruta de módulo** al destino que fije D2.2 (por defecto `/v3`, commit upstream `2594581e`), en PR aislado y mecánico.
6. **Inventario ejecutable de divergencias deliberadas** (§2.3) como lista comprobable por tanda, no como acuerdo tácito.
7. **Deltas de especificación viva** derivados de lo absorbido (§3.2).
8. **Actualización de `docs/ROADMAP.md`:** reescritura de la fila de catálogo (`:67`) y de la entrada (`:291-299`) con el alcance real, y ajuste del contador de la Fase 4 (`:8,10`).

### 2.2 Fuera de alcance

| Excluido | Motivo |
|---|---|
| Las **10 journeys de bench en rojo** destapadas por el CI | Deuda heredada, ajena a upstream, de tamaño no acotado. El módulo `bench/` es un segundo módulo Go sin `go.work` y **no lo alcanza `go test ./...` desde la raíz**. Incremento propio. |
| **Reparar** lo que destape la cobertura de `cmd/axiom` | Se entrega el instrumento de medida, no la reparación de superficie desconocida (§4.4). |
| Contribuir los 67 commits propios a upstream | Es una decisión de relación con el proyecto origen (**D3**), no una tarea de absorción. |
| Re-fork o reescritura de historia | Riesgo alto de perder 19 incrementos de producto propio. Descartado con motivo (§4.2). |
| Los 22 commits de merge de upstream | Se absorbe contenido, no topología de historia. |
| Revertir cualquier divergencia deliberada (§2.3) | Prohibición explícita, verificable por tanda. |
| Aceptar la `size:exception` del PR #22 | Es **precondición** de este incremento (§8), no entregable suyo. |
| Unificación del derivador de estado TUI ↔ Web UI | Deuda estructural preexistente, ajena a la reconciliación. |
| Edición de `openspec/INDEX.md` y `openspec/config.yaml` | El índice lo actualiza la fase de archivado; el config es fuente de solo lectura. |

### 2.3 Divergencias deliberadas: inventario de no-reversión

Ninguna tanda puede revertir, total ni parcialmente, lo siguiente. Es criterio de aceptación por rebanada, no una recomendación.

| # | Divergencia | Origen |
|---|---|---|
| V1 | Renombrado de producto `gentle-ai` → `axiom` en la superficie de usuario | INC-01, INC-14 |
| V2 | Marcadores `axiom:` con detección dual retrocompatible (`internal/components/uninstall/cleaners.go`) | INC-11 |
| V3 | Agente `axiom-orchestrator` y comandos slash sin prefijo | INC-11 |
| V4 | Supresión del logo de OpenCode en presets (REQ-09.1) | INC-09 |
| V5 | No forzado de `theme` en `settings.json` (REQ-09.2) | INC-09 |
| V6 | Persona Axiom en castellano peninsular y localización integral de la TUI | INC-10, INC-16 |
| V7 | ODD como paquete Go con CLI, TUI y Web UI | INC-19 |
| V8 | Capa Hub multi-repositorio: `internal/hub`, `workspace`, `multirole`, `handoff`, `semantic`, `livingdoc` | INC-01 a INC-07, INC-12 a INC-15 |

**V7 es la única cuya permanencia está sujeta a decisión abierta (D1).** Las otras siete son firmes.

---

## 3. Capacidades (Capabilities)

> Contrato con la fase `sdd-spec`. Nombres verificados contra `openspec/specs/` (49 dominios vivos).

### 3.1 Capacidades nuevas

- `upstream-absorption-protocol`: método normativo de absorción — derivación obligatoria de la lista de ficheros desde `git show <sha> --stat`, verificación con suite completa sin filtrar más `e2e/e2e_test.sh`, comprobación del inventario de no-reversión por tanda, y estructura, estados y completitud del registro durable de absorción.
- `axiom-distribution-identity`: contrato de nombre publicado por clase de superficie — instalador y tap, compuertas de release, workflows de CI, shim de `crosslane`, namespace de protocolo en `contracts/**`, ruta de módulo Go y nombres de servicio de telemetría. Incluye qué es interoperabilidad (no renombrable sin romper consumidores) y qué es identidad (renombrable).
- `axiom-binary-ci-coverage`: el pipeline DEBE construir y ejercitar `cmd/axiom`, incluida su superficie exclusiva; ventana de carácter informativo acotada y registrada cuando el paso destape fallos preexistentes.

### 3.2 Capacidades modificadas

**Firmes** (dependen solo de que su tanda se ejecute):

| Capacidad | Requerimiento afectado | Qué cambia |
|---|---|---|
| `axiom-sdd-cli-integration` | REQ-13.3 `axiom sdd attempt` | Upstream retiró la gobernanza de *attempts* (`18fa04fb`) preservando las concesiones de edición. Absorberlo modifica el requerimiento. |
| `rdd-sdd-receipt-consumption` | REQ-3 (propiedad del registro de *attempts*, confirmada por mantenedor 2026-08-02), REQ-5 (`ReceiptRef` en el registro de runtime) | Upstream eliminó RDD del ciclo de vida SDD (`e0774e05`). El propietario declarado del registro cambia. Delta coordinado con el anterior. |
| `sdd-research` | REQ-1 «Closed Capability Admission» | Upstream sustituyó la admisión por investigación opcional (`ba3ed690`) y eliminó `internal/agents/researchcapability` (`15cbbde4`). La premisa del dominio («Selected research is required») deja de sostenerse. |

**Condicionales** (su delta existe o no según una decisión abierta o una medición previa):

| Capacidad | Condición | Naturaleza probable del delta |
|---|---|---|
| `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion`, `odd-ui-integration` | **D1** | Sin delta con D1(b). Delta aditivo con D1(a)/(d). **Delta destructivo sobre 15 requerimientos y 33 escenarios archivados el 2026-09-18** con D1(c). |
| `organic-agent-trigger-rules` | **D1** | Upstream hizo de ODD el protocolo obligatorio del orquestador (`70c774f8`, `1b202d77`, `cfc415ce`), que es el dominio exacto de esta capacidad. Delta probable salvo con D1(b). |
| `axiom-tui-branding` | **D2.4** | REQ-14.4 define la pasarela de deprecación de `gentle-ai`. Si D2.4 la retira, delta destructivo acotado. |
| `rdd-decoupling-v3-stability` | Medición de solape con INC-18 (§4.3, F3) | INC-18 absorbió al menos 3 PRs upstream a medias. Si F3 completa lo que quedó fuera, REQ-18.x puede necesitar `MODIFIED` para reflejar el alcance real. |

### 3.3 Capacidades revisadas que **no** requieren delta

Verificado para evitar deltas espurios en la fase de especificación:

| Capacidad | Por qué no cambia |
|---|---|
| `visual-decoupling` (REQ-09.1 a REQ-09.4) | Rige estética, presets y rutas de instrucciones. La identidad de distribución opera sobre instalador, release y protocolo: superficies disjuntas. Además V4 y V5 son no-reversibles. |
| `workspace-topology` (REQ-1.1 identidad y versión) | Fija el comportamiento de `axiom --version`, no la procedencia del artefacto distribuido. |
| ~~`axiom-user-state-and-env`~~ | ~~`~/.axiom` y la precedencia `AXIOM_*` son estado de usuario, no contrato de distribución.~~ **Retirada de esta tabla el 2026-09-19: la exclusión era por categoría, no por medición.** Ver la nota siguiente. |
| `tui-spanish-localization`, `persona-behavior-contract` | Protegidas por V6. Ninguna tanda las toca. |
| `living-documentation` | El registro de absorción es un documento de repositorio, no una especificación viva indexada. |

> **Corrección de esta tabla (2026-09-19).** La cabecera dice «Verificado». Para `axiom-user-state-and-env` no lo estaba: se excluyó razonando que el estado de usuario **no es** contrato de distribución, y esa es una exclusión por categoría, no una medición. La superficie no estaba sana.
>
> Medido: `internal/backup/manifest.go:170-186` declara `~/.axiom/backups` raíz canónica de **todos** los respaldos y etiqueta `~/.gentle-ai/backups` como heredada. Los tres lectores lo respetan. Los **cinco** escritores de producción resuelven la raíz por literal en vez de por la función del paquete, y **no coinciden entre sí**: `internal/cli/sync.go:509`, `internal/cli/run.go:709`, `internal/update/upgrade/executor.go:488` y `:514` e `internal/components/uninstall/service.go:178` escriben la heredada; `internal/dashboard/service.go:1190` (dentro de `CreateBackup`) escribe la canónica, pero también por literal — acierta por coincidencia, no por diseño.
>
> El estado de respaldos del usuario queda partido **por superficie, no por versión**: `axiom sync` lo deja en una raíz y la Web UI en la otra. Y `cmd/axiom/main.go:141` le nombra al usuario `~/.axiom/backups/`, donde cuatro de los cinco escritores no escriben.
>
> La suite no podía verlo: **18 aserciones en 9 ficheros** repiten el literal heredado y **ninguna** el vigente. El test no anula la comprobación, la copia.
>
> Queda cubierto por REQ-20.13 y REQ-20.14 bajo `axiom-distribution-identity`. Se ubica ahí, y no en `axiom-user-state-and-env`, porque la **causa** es la migración de identidad —los lectores se migraron y los escritores no—; el síntoma funcional es aguas abajo de ella. La reserva razonada de la fase de especificación sobre esta ubicación queda registrada, no descartada.
>
> **Lo que esta corrección deja apuntado por encima del caso concreto:** «no requiere delta» y «no pertenece a esta categoría» no son la misma afirmación, y solo la primera exige haber mirado. Una tabla que las mezcla bajo el rótulo «Verificado» produce exactamente el punto ciego que INC-18 dejó y que este incremento existe para cerrar.

---

## 4. Enfoque (Approach)

### 4.1 Las dos reglas antirrecaída (derivadas de INC-18)

INC-18 absorbió parcialmente al menos 3 PRs de upstream: traía la producción y su test Go directo, e ignoraba documentación, scripts de integración y reescrituras de test grandes. Dejó ocho tests rojos durante meses y documentación falsa en `docs/components.md`. La causa mecánica es doble y está verificada:

1. `design.md` y `tasks.md` **fijaron a priori** la lista de ficheros en vez de derivarla del commit de origen.
2. `verify-report.md` certificó «PASS 100 %» con `go test` **filtrado por patrón**, sin ejecutar nunca la suite completa ni `e2e/e2e_test.sh`.

De ahí, dos reglas normativas —no consejos— que la fase de especificación debe convertir en requerimientos y la de verificación en evidencia:

> **RA-1 — Derivación obligatoria de la lista de ficheros.**
> Ninguna tanda declara su alcance a mano. Se deriva de `git show <sha-upstream> --stat` para cada commit de la tanda. El diff de la tanda se contrasta contra esa derivación y **todo fichero presente en la derivación y ausente del diff lleva motivo escrito** en el registro de absorción. Un artefacto de planificación que enumere ficheros sin citar su derivación es motivo de rechazo de la tanda.

> **RA-2 — Verificación sin filtrar.**
> La evidencia de verificación de toda tanda es `go build ./...`, `go vet ./...`, `go test ./...` **sin bandera `-run`** y `e2e/e2e_test.sh`. Ninguna ejecución filtrada por patrón se acepta como evidencia. La cobertura declarada es honesta: el módulo `bench/` queda fuera del alcance de la suite raíz y se declara como tal, no se omite.

RA-2 es satisfacible sobre esta base precisamente porque `bench/` es un módulo independiente sin `go.work`: las 10 journeys en rojo no entran en `go test ./...` desde la raíz y no hacen imposible la regla.

### 4.2 Estrategias descartadas

| Estrategia | Motivo del descarte |
|---|---|
| Merge directo de `upstream/main` | Produce un diff irrevisable sobre 131 ficheros en conflicto real y mezcla en un solo acto decisiones de producto (ODD, identidad) con mecánica (`/v3`). No hay revisión posible. |
| Rebase del fork sobre upstream | Se paga el mismo conflicto una vez por cada uno de los 67 commits propios. Coste superlineal sin beneficio de revisabilidad. |
| Re-fork desde upstream y reaplicación | Riesgo alto de perder o degradar 19 incrementos de producto propio, incluida la capa Hub completa (V8). |

**Elegida: absorción por tandas temáticas** (cherry-pick agrupado), ordenadas por riesgo de conflicto real ascendente y con las fases dependientes de decisión colocadas donde su decisión tiene más recorrido para resolverse.

### 4.3 Fases

| # | Fase | Contenido | Commits / ficheros | Depende de |
|---|---|---|---|---|
| **F0** | Identidad de distribución y cobertura de `cmd/axiom` | Instalador, tap, compuertas de release, workflows, shim de `crosslane`, namespace de `contracts/**`, nombres de servicio; paso de CI sobre `cmd/axiom`; alta del registro de absorción | 0 upstream | **D2** |
| **F1** | Ruta de módulo | `/v2` → destino de D2.2 (por defecto `/v3`, `2594581e`). Mecánico: 1 línea por fichero en 628, salvo 4 excepciones | 2 / ~682 | **D2.2**, F0 |
| **F2** | Telemetría VictoriaMetrics | Sin intersección de conflicto. **Tanda de calibración del protocolo** | 14 / — | F1 |
| **F3** | Reviewer y parsing de OpenCode | Precedida de una medición del solape con lo que INC-18 trajo a medias | 6 / 7 | F1 |
| **F4** | Poda y refactor SDD | Zona más caliente. **Re-derivación, no cherry-pick** (§4.5). Arrastra los 3 deltas firmes de §3.2 | 13 / ~40 | F1, F3 |
| **F5** | CLI y community-tools RTK | Solape parcial con el fork; requiere reconciliación manual por commit | 7 / — | F4 |
| **F6** | Reconciliación ODD | Conflicto de merge bajo (17 ficheros, casi todos golden), **riesgo conceptual altísimo** | 7 / 17 | **D1**, F1 |
| **F7** | Defectos dispersos, documentación y cierre del registro | Riesgo bajo. Cierra el ledger con los 55 commits clasificados | 6 / — | F2–F6 |

Total absorbido: 2+14+6+13+7+7+6 = **55 commits**, la totalidad de los commits de upstream sin merges.

**Por qué F1 va antes que toda absorción, y no al final.** El exploratorio la situaba como fase 4.

> **Corrección medida (obligatoria de leer antes de aceptar este orden).** La primera redacción de esta propuesta justificaba el adelanto afirmando que «todo commit posterior a `2594581e` trae bloques de importación que citan `/v3`», con un coste de **55 reescrituras**. **Es falso.** Medido sobre los 55 commits sin merges de `main..upstream/main`:
>
> | Medida | Valor |
> |---|---|
> | Commits que **añaden** líneas con `gentle-ai/v3` | **3** |
> | Commits cuyo diff **menciona** `/v3`, incluido el contexto | **5** |
> | Commits que tocan ficheros `.go` | 41 |
>
> El impuesto real es de **5 commits sobre 55**, no 55. La cifra original estaba inflada por un factor de once, y por sí sola **no justifica** adelantar la fase.

El adelanto sigue siendo defendible, pero por otras razones que hay que sopesar explícitamente:

- **Es mecánica pura y de diff enorme** (~682 ficheros, una línea cada uno salvo cuatro). Hacerla pronto saca ese ruido del camino y evita que contamine el diff de las tandas de contenido.
- **Sus 4 ficheros no mecánicos** (`scripts/install.sh`, `scripts/install.ps1`, `scripts/crosslane/battery.go`, `scripts/crosslane/host.go`) son exactamente los que F0 ya toca para el contrato de nombre: F0 y F1 son vecinos naturales, y separarlas obliga a tocar dos veces los mismos ficheros.
- **D2.2 hay que decidirla igual.** No es una decisión que el orden de fases pueda evitar, solo aplazar.

En contra del adelanto: con el impuesto real en 5 commits, **posponer F1 al final también es viable** y tiene la ventaja de no bloquear la absorción tras una decisión de producto. El orden de esta propuesta mantiene F1 en primera posición por los dos primeros argumentos, no por el impuesto; si el usuario prefiere desbloquear la absorción antes de decidir D2.2, mover F1 al final es un cambio legítimo y no invalida el resto del plan.

**Válvula si D2.2 se demora.** F2 (telemetría, sin conflicto) puede ejecutarse antes que F1. El impuesto de reescritura ya está medido (5 commits sobre 55, §4.2), así que la válvula deja de ser una medición y pasa a ser lo que siempre debió ser: desbloquear absorción real mientras D2.2 se decide. No es una reordenación permanente: F3 en adelante siguen bloqueadas por F1.

### 4.4 La cobertura de `cmd/axiom`: ¿fase 0 o incremento propio?

El encargo pide razonarlo. La respuesta es **partida, y la partición es la decisión**:

- **Abrir el hueco pertenece a F0.** El paso de CI toca `.github/workflows/ci.yml`, el mismo fichero que el contrato de nombre. Separarlos garantiza dos incrementos editando el mismo workflow con conflicto asegurado entre ellos. Además, el instrumento de medida debe existir **antes** de absorber: es lo que convierte la verificación de F1–F7 en algo que observa el binario real y no el wrapper deprecado.
- **Reparar lo que destape no pertenece a F0.** Son 1919 líneas nunca ejercitadas. La superficie roja resultante es de tamaño desconocido y potencialmente grande. Arrastrarla a este incremento repetiría con exactitud la historia de las 10 journeys de bench: deuda heredada de tamaño no acotado metida en una rebanada que tenía otro objetivo.
- **Resolución.** F0 entrega el paso de CI. Es **bloqueante** para lo que el contrato de nombre toca (construcción del binario y humo de las rutas afectadas), e **informativo y acotado** para el resto de la superficie si esta resulta roja. Toda la superficie roja se inventaría y se da de alta como incremento sucesor nombrado (INC-21), igual que se hizo con las journeys de bench. Medir es barato y acotado; reparar no lo es. Hacer visible el hueco es el entregable; cerrarlo no.

Un paso informativo permanentemente rojo sería un olor. Por eso la ventana informativa es **acotada y registrada**: la capacidad `axiom-binary-ci-coverage` debe exigir que esa ventana declare su inventario y su incremento sucesor, no que sea indefinida.

### 4.5 Por qué F4 se re-deriva en vez de cherry-pickearse

La poda SDD de upstream son 13 commits sobre ~40 ficheros en la zona de mayor divergencia (`internal/sddstatus`, `internal/cli/sdd_*.go`, `internal/components/sdd/`). El fork tiene ahí 4255 líneas donde upstream tiene 919: no es una diferencia que un cherry-pick resuelva, es una reescritura. Además arrastra tres deltas de especificación viva sobre requerimientos confirmados por mantenedor.

El método para F4 es distinto al del resto: **se toma el resultado de upstream como objetivo declarado y se re-deriva sobre el fork**, con la lista de ficheros de upstream (RA-1) usada como *lista de comprobación de cobertura*, no como parche a aplicar. Todo fichero de esa lista que no aparezca en el diff de la tanda lleva motivo escrito. Es la misma regla, aplicada a una tanda que se escribe en vez de aplicarse.

### 4.6 El registro durable de absorción

`docs/upstream-absorption-ledger.md`, con espejo en Engram. Motivo: el artefacto que hoy falta es exactamente el que habría evitado INC-18. Una tabla de 55 filas con `sha` → `tanda` → `estado` → `evidencia` → `motivo si se descarta` es la diferencia entre que la próxima reconciliación empiece midiendo y que empiece adivinando.

Vive en `docs/` y no en `openspec/changes/<cambio>/` **a propósito**: un artefacto de cambio desaparece en el archivo y deja de ser consultable como estado actual. El registro es estado vivo del repositorio y sobrevive al cierre del incremento.

### 4.7 Entrega

Ocho fases, ninguna encajable en 400 líneas modificadas salvo F7. Estrategia de sesión: `auto-chain`. Cadena **apilada contra `main`** (`stacked-to-main`): cada fase deja el árbol compilando y verde por sí sola. F1 es la excepción de tamaño evidente (~682 ficheros, 1 línea cada uno): es un PR mecánico que requiere `size:exception` explícita y **no admite mezclarse con nada más**.

```
main
 └─ F0 identidad + CI ──► F1 ruta de módulo ──► F2 telemetría ──► F3 reviewer ──► F4 poda SDD ──► F5 CLI/RTK ──┐
                                                       └─► F6 ODD (D1) ──────────────────────────────────────┴─► F7 cierre
```

F6 depende de F1 y de D1, pero no de F2–F5: puede adelantarse en cuanto D1 esté resuelta. La partición definitiva y el pronóstico de líneas corresponden a `sdd-tasks`.

---

## 5. Áreas afectadas

### 5.1 Paquetes `internal/` afectados (regla `rules.proposal` de `openspec/config.yaml`)

| Paquete | Impacto | Detalle |
|---|---|---|
| `internal/sddstatus` | **Modificado (mayor)** | `status.go`, `review_door.go`, `runtime_ledger.go` (4255 → objetivo upstream). Zona más caliente. F4. |
| `internal/cli` | **Modificado (mayor)** | `sdd_*.go` (F4), `review_*.go` (F3), `doctor.go`, `run.go`, `sync.go` (F5). 294 ficheros divergentes. |
| `internal/components/sdd` | **Modificado** | Arrastrado por la poda SDD. F4. |
| `internal/assets` | **Modificado** | Instrucciones de agente afectadas por F4 y, condicionalmente, por F6. 98 ficheros divergentes. Sometido a `assets_test.go`. |
| `internal/agents/researchcapability` | **Eliminado (probable)** | Upstream lo borró (`15cbbde4`). Su eliminación arrastra el delta de `sdd-research`. F4. |
| `internal/tui` | **Modificado (menor)** | 95 ficheros divergentes; V6 protege la localización. Riesgo de reversión accidental alto. |
| `internal/odd` | **Condicional (D1)** | Intocado con D1(b). Ampliado con D1(a)/(d). **Eliminado con D1(c)** — en cuyo caso deja de ser una fase y pasa a incremento propio. |
| `internal/components/uninstall` | **Intocado (prohibido)** | `cleaners.go` implementa la detección dual de marcadores (V2). |
| `internal/hub`, `workspace`, `multirole`, `handoff`, `semantic`, `livingdoc` | **Intocados (prohibidos)** | Producto propio (V8). Upstream no tiene equivalente; cualquier diff aquí es una reversión accidental. |

### 5.2 Fuera de `internal/`

| Ruta | Impacto | Detalle |
|---|---|---|
| `go.mod` y 686 ficheros con importaciones | Modificado | F1, mecánico salvo 4 excepciones. |
| `scripts/install.sh`, `install.ps1` | Modificado | F0 + excepción no mecánica de F1. Contrato publicado. |
| `scripts/verify-release-assets.sh`, `release-preflight.sh`, `promote-stable-preflight.sh` | Modificado | Compuertas de release atadas a `Gentleman-Programming/gentle-ai`. F0. |
| `scripts/crosslane/battery.go`, `host.go` | Modificado | Shim `gentle-ai` en `$PATH` y parsing por `words[0]`. F0 + excepción de F1. |
| `.github/workflows/ci.yml` | Modificado | Nombre del binario y alta del paso sobre `cmd/axiom`. F0. |
| `contracts/**/*.schema.json` | **Condicional (D2.3)** | `$id` y prefijos de protocolo. Cambiarlos es romper interoperabilidad, no renombrar. |
| `cmd/gentle-ai`, `cmd/axiom` | Modificado | Según D2: qué binario se publica y bajo qué nombre. |
| `deploy/telemetry/*.service`, dashboards | Modificado | Nombres de servicio. F0. |
| `testdata/golden/**` (~30 en la intersección) | Modificado | Regenerados con `-update` y **diffeados en busca de cambio semántico**, nunca aceptados a ciegas. |
| `docs/upstream-absorption-ledger.md` | **Nuevo** | Registro durable. F0 lo crea, F7 lo cierra. |
| `openspec/specs/` | Modificado | 3 deltas firmes; hasta 7 condicionales (§3.2). |
| `docs/ROADMAP.md` | Modificado | Fila `:67`, entrada `:291-299`, contador `:8,10`. |
| `bench/` | **Intocado (prohibido)** | Las 10 journeys en rojo son incremento aparte. |
| `openspec/INDEX.md`, `openspec/config.yaml` | **Intocados (prohibidos)** | Índice: fase de archivado. Config: solo lectura. |

---

## 6. Riesgos

| # | Riesgo | Prob. | Sev. | Mitigación |
|---|---|---|---|---|
| R1 | **Recaída de INC-18**: absorción parcial certificada como completa por una suite filtrada. | Alta | **Alta** | RA-1 y RA-2 (§4.1) elevadas a criterio de éxito verificable (§10.1) y a requerimiento en `upstream-absorption-protocol`. Todo fichero derivado y ausente lleva motivo escrito en el registro. |
| R2 | **Reversión accidental de una divergencia deliberada** (logo de OpenCode, forzado de `theme`, persona en castellano, marcadores `axiom:`, capa Hub). | Alta | **Alta** | Inventario V1–V8 (§2.3) como lista comprobable por tanda, no como acuerdo tácito. Criterio de aceptación por rebanada: el diff no revierte ninguna entrada del inventario. |
| R3 | **D2.2 bloquea el camino crítico.** F1 es prerrequisito de F2–F7; sin decisión de ruta de módulo, la absorción no arranca. | Media | **Alta** | Severidad rebajada respecto a la primera redacción: medido el impuesto real en 5 commits sobre 55 (§4.2), F1 puede moverse al final sin invalidar el plan, así que D2.2 aplaza pero no bloquea. Válvula de §4.3: F2 se adelanta y la absorción arranca igual. |
| R4 | **Doble reescritura masiva**: migrar a `/v3` y después decidir que la ruta debe ser de Axiom, reescribiendo 686 ficheros dos veces. | Media | Media | F1 no arranca hasta que D2.2 fije el **destino final**, no solo el salto de mayor. Una sola reescritura, al destino definitivo. |
| R5 | La cobertura de `cmd/axiom` destapa una superficie roja **no acotada** en 1919 líneas nunca ejercitadas. | Alta | Media | Partición de §4.4: el paso se entrega, la reparación no. Ventana informativa acotada, inventario escrito e incremento sucesor nombrado. |
| R6 | **F4 desborda**: la poda SDD toca la zona de 4255 → 919 líneas, con 3 deltas sobre requerimientos confirmados por mantenedor. | Alta | Alta | Re-derivación en vez de cherry-pick (§4.5). Tests de caracterización sobre la compuerta `verify → archive` **antes** de tocar nada, heredando el blindaje que el INC-20 original ya identificaba como paso previo. |
| R7 | **Churn de goldens**: ~30 ficheros golden en la intersección, regenerados en masa, ocultando un cambio semántico real. | Alta | Media | Regeneración con `-update` seguida de **diff dirigido a cambio semántico**. Ningún golden se acepta sin leer su diff. Excluidos del recuento de líneas de revisión, incluidos en la identidad de la instantánea. |
| R8 | **Upstream sigue moviéndose**: los 55 commits son 60 o 70 al cerrar el incremento. | Alta | Baja | El registro de absorción es incremental y fecha su medición. El incremento cierra sobre el conjunto medido el 2026-09-18; lo posterior es reconciliación siguiente, no ampliación de alcance. |
| R9 | **Deltas destructivos sobre capacidades archivadas ayer** (las 4 de ODD: 15 requerimientos, 33 escenarios, archivadas el 2026-09-18) si D1 resuelve por (c). | Media | Alta | D1 no se resuelve aquí. Si resuelve por (c), F6 deja de ser fase y pasa a incremento propio con su propio ciclo y su aviso de delta destructivo (`rules.archive`). |
| R10 | **El PR #22 no se fusiona** (`size:exception` pendiente) y el incremento arranca sobre árbol rojo. | Media | Alta | Dependencia dura declarada (§8). Ninguna tanda arranca sin base verde; en árbol rojo, RA-2 no distingue lo heredado de lo absorbido y toda la verificación pierde valor. |
| R11 | **F1 rompe consumidores del módulo Go** al cambiar la ruta de importación. | Baja | Baja | El fork **no tiene base instalada por esta vía**: su instalador nunca lo instaló (§1.1c). El defecto de distribución elimina, paradójicamente, el riesgo de migración. Se documenta, no se mitiga. |
| R12 | **Cambiar el namespace de `contracts/**` rompe interoperabilidad** con consumidores compatibles con upstream. | Media | Alta | D2.3 se plantea explícitamente como decisión de interoperabilidad, no de marca. Por defecto **no se toca** mientras no haya respuesta. |
| R13 | **Solape opaco con INC-18**: F3 reaplica lo que INC-18 ya trajo a medias, con conflictos y duplicidades. | Media | Media | F3 arranca con una medición explícita del solape (`git show --stat` de los 3 PRs implicados contra el árbol actual) **antes** de absorber nada. |
| R14 | Ruido de formato en un diff de 682 ficheros: `gofmt -l .` señalaba 18 ficheros no canónicos preexistentes. | Alta | Baja | Normalización **solo** de ficheros tocados. F1 se verifica con `gofmt -l` sobre su propio conjunto, no sobre el árbol. |

---

## 7. Plan de rollback

### 7.1 Reversión por fase

Cada fase es una unidad revertible en aislamiento mediante `git revert` de su commit de fusión.

| Fase | Alcance del rollback | Observación |
|---|---|---|
| F0 | Restaura los nombres y las compuertas previos, y retira el paso de CI. | **Restaura un estado defectuoso conocido** (el instalador vuelve a instalar upstream). Es seguro, no es deseable. |
| F1 | Revierte la ruta de módulo. Mecánico y limpio por construcción: es una línea por fichero. | Revertir F1 **invalida toda tanda posterior ya aplicada**: debe revertirse en último lugar. |
| F2–F5, F7 | Eliminan su tanda absorbida. Independientes entre sí, dentro del orden de la cadena. | Cada reversión **debe marcar sus commits como `revertido` en el registro**, no borrar la fila. |
| F6 | Depende de D1. Con (a)/(d) es aditiva y revertible sin más. Con (c) no es una reversión, es una restauración de código eliminado: no se contempla aquí porque (c) sale de este incremento. | — |

### 7.2 El registro es parte del rollback

Revertir una tanda **sin actualizar el registro de absorción reproduce el defecto original de INC-18**: la próxima reconciliación creería absorbido algo que ya no está en el árbol. Por eso el estado `revertido` es un valor de primera clase del registro y no una omisión. Es criterio de aceptación de toda reversión.

### 7.3 Criterios de aborto

Se detiene el incremento y se revierte hasta el último estado verde si se cumple cualquiera de estas condiciones:

- Un diff de tanda revierte, total o parcialmente, cualquier entrada del inventario V1–V8.
- Una tanda se declara verificada con `go test -run <patrón>` o sin `e2e/e2e_test.sh` (violación de RA-2).
- Una tanda declara su lista de ficheros sin derivarla de `git show --stat` (violación de RA-1).
- La suite raíz sin filtrar queda en rojo al cierre de una tanda y el fallo no está inventariado como preexistente y ajeno.
- Un diff contiene ficheros bajo `bench/`, `internal/hub/`, `openspec/INDEX.md` u `openspec/config.yaml`.

### 7.4 Rollback de datos

No hay migración de datos ni cambio de esquema persistido. Los 19 incrementos archivados no se tocan. El cambio de ruta de módulo (F1) no tiene efecto sobre estado de usuario en `~/.axiom`. El espejo Engram del registro es aditivo y versionado por `topic_key`: no se borra ni se reescribe historial.

---

## 8. Dependencias y restricciones

- **Dependencia dura: PR #22 fusionado.** La rama `fix/saneamiento-ci-main` deja los 38 tests y las 49 aserciones E2E en verde, pendiente de `size:exception`. Sin esa base verde, RA-2 no puede distinguir lo heredado de lo absorbido y toda la verificación del incremento pierde su valor probatorio.
- **Dependencia dura: D2.2 resuelta antes de F1**, y F1 antes de F2–F7 (§4.3). Es el camino crítico.
- **Dependencia: D1 resuelta antes de F6.** No bloquea F0–F5 ni F7.
- **Remoto `upstream` configurado** y accesible (`Gentleman-Programming/gentle-ai`), con la medición fechada del 2026-09-18 como conjunto de referencia.
- **Sin dependencias externas nuevas.** Ni librerías, ni servicios, ni cambio de toolchain (Go 1.25.10+).
- **Strict TDD activo** (`strict_tdd: true`, `openspec/config.yaml:16,46`): RED observado antes de implementar, GREEN y refactor, con evidencia registrada por unidad de trabajo. Aplicación matizada en F1, que es una reescritura mecánica sin comportamiento nuevo: su prueba es `go build ./...` más la suite completa, no un test nuevo.
- **Herramientas de verificación:** `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt`, `e2e/e2e_test.sh`. Sin `golangci-lint` configurado.
- **Cobertura parcial declarada:** el módulo `bench/` no queda cubierto por la suite raíz. Se declara, no se oculta.

---

## 9. Decisiones de producto

> **RESUELTAS por el usuario el 2026-09-18.** Esta sección conserva el planteamiento original de cada decisión, con su resolución al principio. Las alternativas descartadas se mantienen porque documentan por qué la elegida lo fue.

### Resumen de lo decidido

| Decisión | Resolución | Consecuencia principal |
|---|---|---|
| **D1** ODD | **Sustitución completa por el ODD de upstream. Destructiva: no se conserva nada de la capa Go.** | ~5073 líneas retiradas; deltas destructivos sobre 4 capacidades vivas archivadas el día anterior |
| **D2.1** Distribución pública | **Cambiar a Axiom**, pero **publicar releases primero** | Sin releases propias, repuntar el instalador lo deja instalando nada |
| **D2.2** Ruta de módulo | **Seguir a upstream: `/v3`** | Mantiene limpios todos los cherry-pick futuros |
| **D2.3** Namespace `contracts/**` | **No tocar** | Es interoperabilidad de cable, no marca |
| **D2.4** Pasarela y shim | **Mantener `gentle-ai` como alias** | Coste cero, ya funciona |
| **D3** Relación con upstream | **Mantenerla** | Refuerza D2.2 y D2.3 |

**Criterio que ordenó D2:** *renombra lo que lee un humano, conserva lo que lee una máquina.*

### Alcance destructivo de D1

La sustitución retira:

| Superficie | Líneas |
|---|---|
| `internal/odd` (producción) | 1534 |
| `internal/odd` (tests) | 1939 |
| `internal/cli/odd_*.go` | 841 |
| Backend de dashboard y pantalla TUI de ODD | 759 |
| **Total** | **~5073** |

Y exige **deltas destructivos** sobre las 4 capacidades vivas que INC-19 archivó el 2026-09-18: `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion`, `odd-ui-integration` (15 requerimientos, 33 escenarios).

**Qué sobrevive:** el contrato del documento `odd/tasks/<feature>.md` y su espejo Engram `odd/<feature>/tasks`, porque **es idéntico en ambas implementaciones**. Los documentos existentes son datos, no código, y no se tocan.

**Qué se gana:** el protocolo ODD de upstream es más completo que el del fork — commits por unidad de trabajo, TDD configurado, continuidad de feature al reanudar, estrategia de entrega. Además pasa a vivir en `internal/components/agentguidance/routing.go`, el inyector de guía de agentes, donde **el `routing.go` del fork tiene hoy cero menciones de ODD**: el protocolo del fork vive en `persona-axiom.md`, un punto de inyección distinto.

**Evidencia que sostuvo la decisión.** La capa Go parsea un documento que escribe el agente: valida, no habilita. Y durante la sesión completa de saneamiento del CI se llevó un documento vivo ODD de 17 tareas sin invocar ni una sola vez `axiom odd create`, `axiom odd status` ni `axiom odd promote` — se escribió con herramientas de fichero y se espejó con `mem_save`. Una capa portante se habría notado por su ausencia.

**Coste asumido conscientemente:** es trabajo de la misma semana. Ya está pagado y es irrecuperable; mantenerlo costaría más que retirarlo.

---

## 9.bis Planteamiento original de las decisiones

### D1 — Reconciliación de ODD: ¿qué es ODD en Axiom?

El fork implementó ODD como **paquete Go** (INC-19: `internal/odd`, CLI `axiom odd create|status|promote`, pantalla TUI, pestaña Web UI; 4 capacidades vivas, 15 requerimientos, 33 escenarios, archivadas el 2026-09-18). Upstream implementó ODD como **instrucciones de agente** (`70c774f8` protocolo obligatorio del orquestador, `1b202d77` continuidad de feature y TDD configurado, `cfc415ce` commit por unidad de trabajo); **no existe `internal/odd` en upstream**. No son la misma cosa: una es producto ejecutable, la otra es doctrina inyectada en el agente.

| Opción | Ventajas | Coste |
|---|---|---|
| **(a) Coexistencia**: conservar el Go y absorber además las instrucciones de upstream | Absorbe los 7 commits sin perder producto; los 17 ficheros (casi todos golden) dejan de divergir. | **Dos definiciones de ODD que mantener coherentes.** Riesgo real de que la instrucción diga una cosa y el binario haga otra. Deltas aditivos en `organic-agent-trigger-rules` y probablemente en las 4 capacidades ODD. |
| **(b) Divergencia declarada**: conservar el Go y **no absorber** los 7 commits | Coste inmediato nulo; producto propio intacto; V7 se convierte en divergencia firme como V1–V6. | 17 ficheros divergen para siempre y **toda reconciliación futura vuelve a pelear el mismo conflicto**. El registro de absorción cargaría 7 filas `descartado-deliberadamente` con motivo permanente. |
| **(c) Sustitución**: eliminar el paquete Go y adoptar las instrucciones de upstream | Alineación total con upstream; desaparece la duplicidad conceptual; menos código propio que mantener. | **Deltas destructivos sobre 4 capacidades archivadas el día anterior** (15 requerimientos, 33 escenarios). Elimina `internal/odd`, la CLI, la pantalla TUI y la pestaña Web UI. Contradice el informe de archivo de INC-19. **Coste tal que debería ser incremento propio, no fase de este.** |
| **(d) Subordinación**: conservar el Go como autoridad y **adaptar** las instrucciones de upstream para que lo referencien | Una sola definición de ODD con dos superficies coherentes: la doctrina apunta al comando que existe. | El mayor trabajo de diseño de las cuatro. Las instrucciones no se copian, se reescriben; la absorción deja de ser cherry-pick y pasa a ser autoría, con churn de golden garantizado. |

**Efecto sobre el plan:** (a), (b) y (d) mantienen F6 como fase. **(c) la saca de este incremento.** No bloquea F0–F5 ni F7.

### D2 — Identidad de distribución: ¿qué nombre sobrevive en cada sitio?

La pregunta de fondo, de la que dependen las cuatro sub-decisiones: **¿Axiom es un producto público con distribución propia, o un fork de código fuente que consume la distribución de upstream?** Hoy el árbol afirma lo segundo sin haberlo decidido nunca.

#### D2.1 — Distribución pública (`install.sh`, `install.ps1`, tap de Homebrew, compuertas de release)

| Opción | Ventajas | Coste |
|---|---|---|
| **Distribución propia de Axiom** | El instalador instala el producto; el rebranding de INC-01 e INC-14 deja de ser ficción; los releases tienen destino. | Exige repositorio de releases propio, artefactos firmados, tap propio y reescritura de las 3 compuertas de preflight. Trabajo real de infraestructura, no de renombrado. |
| **Asumir que el fork es solo código fuente** | Coste cero; se retiran los scripts de instalación en vez de arreglarlos, y se documenta que Axiom se compila. | El producto no se distribuye. Renunciar explícitamente a ello es legítimo, pero debe decirse: hoy está implícito y parece un fallo. |

#### D2.2 — Ruta de módulo Go (**camino crítico**)

686 ficheros citan `github.com/gentleman-programming/gentle-ai/v2`; el fork no cita `/v3` en ningún sitio. El import path **nunca se renombró a Axiom** pese al rebranding.

| Opción | Ventajas | Coste |
|---|---|---|
| **Seguir a upstream a `/v3`** (`2594581e`) | Una reescritura mecánica de 1 línea × 628 ficheros; los cherry-picks posteriores aplican limpios; máxima facilidad de reconciliación futura. | La ruta de importación del producto sigue nombrando a upstream. Identidad incoherente y permanente. |
| **Ruta propia de Axiom** | Identidad coherente de extremo a extremo. | Misma reescritura masiva, pero el **impuesto de reescritura de importaciones en cada cherry-pick se vuelve permanente**: toda reconciliación futura, para siempre, reescribe imports. |
| **Aplazar** | — | **Bloquea F1 y, con ella, F2–F7.** No es una opción neutra: es detener la absorción. |

#### D2.3 — Namespace de protocolo en `contracts/**` (`$id: https://gentle-ai.dev/...`, prefijos `gentle-ai.review-integration/v1`, `gentle-ai.sdd-status/v2`)

**Esto es contrato de interoperabilidad de cable, no marca.** Renombrarlo rompe a todo consumidor compatible con upstream.

| Opción | Ventajas | Coste |
|---|---|---|
| **Conservar** (recomendación por defecto si no hay respuesta) | Interoperabilidad intacta; coste cero; es lo que todo consumidor espera. | La identidad de marca no alcanza el protocolo. |
| **Renombrar** | Coherencia total de identidad. | Ruptura de interoperabilidad. Exige versión mayor de protocolo y migración de consumidores. |
| **Aceptación dual** | Identidad propia sin romper a nadie. | Duplica la superficie de validación de esquemas y de aserciones en tests; complejidad permanente. |

#### D2.4 — Pasarela `gentle-ai` (REQ-14.4) y shim de `crosslane`

| Opción | Ventajas | Coste |
|---|---|---|
| **Conservar** | Ruta de deprecación viva para usuarios existentes; `scripts/crosslane/*.go` sigue funcionando sin tocar su parsing por `words[0]`. | Se mantiene un binario y un nombre que la identidad quiere retirar. |
| **Retirar** | Superficie limpia. | Delta destructivo sobre REQ-14.4 y reescritura del shim y su parsing en `battery.go` y `host.go`. |

### D3 — Relación a largo plazo con upstream

Los 67 commits propios ausentes en upstream no se contribuyen en este incremento (§2.2), pero **la intención determina la estrategia de toda reconciliación futura**: si se pretende contribuir, la divergencia debe mantenerse mínima y estructurada; si la divergencia es permanente, se optimiza por absorción selectiva y se acepta que la distancia crezca.

No bloquea ninguna fase de este incremento. Se plantea porque **es la premisa que hace correcta o incorrecta la opción (b) de D1** y el criterio con el que se escriben los motivos de `descartado-deliberadamente` en el registro.

---

## 10. Criterios de éxito

### 10.1 Reglas antirrecaída (verificables, derivadas de INC-18)

- [ ] **RA-1.** Toda tanda absorbida documenta su lista de ficheros **derivada de `git show <sha-upstream> --stat`**, y todo fichero presente en esa derivación y ausente del diff de la tanda lleva motivo escrito en `docs/upstream-absorption-ledger.md`.
- [ ] **RA-2.** Toda tanda presenta como evidencia de verificación `go build ./...`, `go vet ./...`, `go test ./...` **sin bandera `-run`** y `e2e/e2e_test.sh`. **Ninguna ejecución filtrada por patrón se acepta como evidencia**, y la cobertura no alcanzada (`bench/`) se declara explícitamente.
- [ ] Ningún artefacto de planificación (`design.md`, `tasks.md`) enumera ficheros de una tanda sin citar su derivación.

### 10.2 Absorción completa y auditable

- [ ] `docs/upstream-absorption-ledger.md` contiene las **55 filas** de los commits de upstream sin merges, cada una con estado `absorbido`, `descartado-deliberadamente` o `revertido`, y motivo escrito para los dos últimos.
- [ ] El registro tiene espejo en Engram y su medición está fechada.
- [ ] Toda reversión de tanda actualiza su fila a `revertido`; ninguna fila se borra.

### 10.3 Frontera respetada

- [ ] Ningún diff del incremento revierte, total o parcialmente, ninguna entrada del inventario V1–V8 (§2.3).
- [ ] Ningún diff contiene ficheros bajo `bench/`, `internal/hub/`, `openspec/INDEX.md` u `openspec/config.yaml`.
- [ ] Las 10 journeys de bench en rojo siguen siendo trabajo aparte, inventariado y con incremento sucesor nombrado.

### 10.4 Identidad de distribución

- [ ] Toda entrada del inventario de §1.1(c) tiene una resolución explícita conforme a D2, aplicada o declarada como conservada a propósito.
- [ ] El instalador instala lo que D2.1 decidió, verificado sobre el artefacto real, no sobre el script leído.
- [ ] La ruta de módulo es la que fijó D2.2, con `go build ./...` verde y una sola reescritura masiva en la historia.
- [ ] Si D2.3 resuelve conservar, `contracts/**` queda **byte a byte** sin cambios.

### 10.5 Cobertura del binario real

- [ ] `.github/workflows/ci.yml` construye y ejercita `cmd/axiom`, incluida su superficie exclusiva (`init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui`).
- [ ] El paso es bloqueante para la superficie que toca el contrato de nombre.
- [ ] Si alguna parte queda en carácter informativo, su inventario rojo está escrito y su incremento sucesor nombrado. **Ninguna ventana informativa queda sin fecha ni sucesor.**

### 10.6 Entrega y calidad

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` y `e2e/e2e_test.sh` en verde al cierre de **cada** fase.
- [ ] `gofmt -l` no señala ningún fichero tocado por el incremento.
- [ ] Ningún golden regenerado se acepta sin diff leído en busca de cambio semántico.
- [ ] Ninguna fase supera el presupuesto de 400 líneas sin `size:exception` aceptada; F1 la requiere por construcción y no se mezcla con ninguna otra.
- [ ] Cada fase declara su frontera de rollback y su evidencia de verificación observada.
- [ ] `docs/ROADMAP.md` refleja el alcance real de INC-20, retira el enunciado obsoleto de `inc-20-sdd-engine-contract-retirement` y corrige el contador de la Fase 4.
