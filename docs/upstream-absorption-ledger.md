# Registro de absorción upstream — Axiom

> **Medido el:** 2026-09-19 · **Ancestro común:** `266574b0` · **`upstream/main`:** `82a6de96ca6e1cb4f6bf603fe0c08ef1c2039833`
> **Techo congelado:** etiqueta `v3.4.0` de upstream, que es exactamente el `upstream/main` de arriba (decisión D4: no se re-mide)
> **Universo:** 91 commits de `266574b0..upstream/main` **sin merges**
> **Comando:** `git rev-list --count --no-merges 266574b0..upstream/main`

> **Nota de alcance — universo congelado, no vivo.** 91 es el número de commits sin merge entre `266574b0` y `82a6de96` (`upstream/main` en el instante de abrir este registro), congelado a esa fecha y a ese `sha`, no un valor que se reconsulta en cada lectura. La propuesta había medido 55 el 2026-09-18 (`proposal.md:71,195,471`); el diseño remidió el universo el 2026-09-19 y obtuvo 87 **con** merges (`design.md` §10, "Estado de las mediciones" y §10.1), y acotó la cifra real sin merges en el intervalo `[55, 65]` **sin fijarla**, precisamente para que la apertura de este registro la resolviera con el comando exacto de arriba. El valor final, 91, se obtuvo tras la publicación de `v3.4.0` de upstream esa misma jornada (etiqueta fechada 2026-09-19, en la punta de `upstream/main` en el momento de medir). Todo commit que upstream publique después de `82a6de96` pertenece a un incremento de reconciliación futuro, no a `inc-20-upstream-reconciliation`.

> **Decisión de producto D4 — el techo es `v3.4.0` y no se mueve.** El universo de este incremento se cierra en la etiqueta `v3.4.0` de upstream (`82a6de96`), y esa frontera **no se re-mide** aunque upstream siga publicando mientras las rebanadas restantes aterrizan. El motivo es de terminación, no de comodidad: un universo que se reconsulta en cada fase nunca se cierra, porque upstream avanza más rápido de lo que se absorbe — la propia historia de esta cabecera lo demuestra, con el conteo pasando de 55 a 91 en una sola jornada. Absorber un blanco móvil es un trabajo sin criterio de fin.
>
> En consecuencia: ninguna tanda de este incremento incorpora commits posteriores a `82a6de96`, y una tanda que los encuentre en su derivación los deja fuera con motivo escrito en su sub-tabla, no los absorbe «de paso». La integración de versiones posteriores a `v3.4.0` se aborda con un flujo propio, a diseñar **una vez Axiom esté terminado**; ese flujo es trabajo futuro y no pertenece a `inc-20-upstream-reconciliation`.

## Reglas de aceptación

Adaptadas de `docs/releases/v2.2.0-closure-ledger.md:11-21` a la forma de este registro (D-06). Gobiernan todo veredicto de las tablas siguientes.

1. **Derivación obligatoria (RA-1).** Toda tanda deriva su lista de ficheros de `git show <sha> --stat` para cada commit de upstream que la compone. Un fichero derivado y ausente del diff de la tanda lleva motivo escrito en su sub-tabla "Ficheros derivados y ausentes"; sin motivo, la tanda se rechaza.
2. **"El código parece relacionado" no es evidencia.** Ni lo es un fichero compartido ni un asunto de commit parecido. El estado `absorbido` exige una referencia de verificación concreta (PR del fork o commit de re-derivación), no una impresión de similitud.
3. **Verificación sin filtrar (RA-2).** Ninguna tanda se marca `absorbido` sin `go build ./...`, `go vet ./...`, `go test ./...` sin `-run`, y `e2e/e2e_test.sh`, todos en verde — salvo el único fallo aceptado y saltado de este mismo paquete (`TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose`) hasta el cierre de la Fase 16. La cobertura no alcanzada por `bench/` (módulo Go independiente, sin `go.work`) se declara explícitamente, nunca se omite.
4. **Inventario de no-reversión (V1–V8).** Ninguna tanda distinta de F6 revierte, total o parcialmente, las entradas V1–V6 u V8. La entrada V7 (ODD como paquete Go) solo la retira F6, y solo mediante los deltas de especificación que esa fase autoriza.
5. **Rutas prohibidas.** Ninguna tanda toca `bench/`, `internal/hub/`, `internal/workspace/`, `internal/multirole/`, `internal/handoff/`, `internal/semantic/`, `internal/livingdoc/`, `internal/components/uninstall/cleaners.go`, `openspec/INDEX.md`, `openspec/config.yaml`, `openspec/changes/archive/**`, `docs/releases/**` ni `odd/tasks/*.md`.
6. **Donde la disposición no puede establecerse con evidencia, el veredicto es "no está claro — requiere confirmación del autor".** Adivinar es peor que admitir incertidumbre.
7. **Una tanda revertida actualiza el estado de sus filas a `revertido`, conservando el resto de sus campos. Ninguna fila se borra nunca.**

## Recuento

| Estado | Filas |
|---|---|
| `absorbido` | 46 |
| `descartado-deliberadamente` | 2 |
| `revertido` | 0 |
| **Total** | **48 (= universo declarado en la cabecera: 91)** |

## F0 — Identidad de distribución, artefacto de release y cobertura del binario real

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F1 — Migración de la ruta de módulo Go `/v2` → `/v3`

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F2 — Telemetría VictoriaMetrics

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `b6308292` | fix(telemetry): run the collector in WAL, anchor maintenance to UTC midnight, and report busy storage (#4718) | `absorbido` | `37670801` (rama `inc-20/pr7-absorcion-upstream`) | Import `gentle-ai/v3` en `runtime_storage.go` reconciliado a mano a `/v2`; la Fase 17 lo reescribirá con el resto del árbol. Colisión no listada entre los cinco SHAs candidatos de la nota cruzada D-01. |
| `eae8fadd` | fix(telemetry): key the rate limiter on a parsed address, budget runtime separately, and paginate GitHub downloads (#4724) | `absorbido` | `6c5c2bb5` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `c09b1a34` | feat(telemetry): expose runtime telemetry as Prometheus counters | `absorbido` | `1a64eb9a` (rama `inc-20/pr7-absorcion-upstream`) | Imports `gentle-ai/v3` en `metrics.go` y `metrics_test.go` reconciliados a mano a `/v2` conforme a la nota cruzada D-01; la Fase 17 los reescribirá. |
| `e0445434` | feat(telemetry): add a runtime-store mode that skips raw rows behind delivery-id dedup | `absorbido` | `f7274597` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `6d4ef5ba` | docs(telemetry): document the runtime metrics exposition and the runtime-store flag | `absorbido` | `a4a320f2` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `133c6dfb` | feat(telemetry): install VictoriaMetrics with the deploy kit and provision its Grafana datasource | `absorbido` | `465bfffb` (rama `inc-20/pr7-absorcion-upstream`) | Colisión con el renombrado de unidades de la Fase 4 (V1): upstream parchea `gentle-telemetry-backup` y `.test.sh`; los hunks se aplicaron sobre `axiom-telemetry-backup*` y `install.sh` conservó las líneas de identidad Axiom. Rutas de máquina (`/usr/local/bin/gentle-telemetry`, `$GENTLE_TELEMETRY_*`) intactas por decisión de la Fase 4. |
| `a60b541b` | feat(telemetry): add a VictoriaMetrics backfill for the raw runtime tables | `absorbido` | `a77001f4` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `ca27a09c` | feat(telemetry): move the runtime dashboard panels to VictoriaMetrics | `absorbido` | `daa58bbe` (rama `inc-20/pr7-absorcion-upstream`) | Aplicó sin conflicto pese a tocar el dashboard y `runtime_dashboard_test.go` a la vez; identidad Axiom del dashboard verificada explícitamente (título, fila y descripciones) antes de aceptar el auto-merge. `uid` `gentle-ai-usage` y datasource `gentle-telemetry-sqlite` preservados como contratos de máquina. |
| `68ed179f` | feat(telemetry): select the runtime store from an environment file the installer writes | `absorbido` | `fa417b26` (rama `inc-20/pr7-absorcion-upstream`) | Colisión con el renombrado de la Fase 4 (V1): upstream parchea `gentle-telemetry.service`; el hunk se aplicó sobre `axiom-telemetry.service` y `install.sh` conservó `systemctl enable --now axiom-telemetry.service`. |
| `0fc3845b` | fix(telemetry): stream the VictoriaMetrics backfill and arm metrics mode only after a healthy install | `absorbido` | `24dfde4b` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `4ed6238e` | fix(telemetry): allow mincore in the VictoriaMetrics unit syscall filter (#4733) | `absorbido` | `977a4e8f` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `7f112eac` | fix(telemetry): install the Prometheus plugin and hand the Grafana plugins directory to grafana (#4735) | `absorbido` | `b583bae7` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `70ac39de` | fix(telemetry): deduplicate VictoriaMetrics samples and always release the backup snapshot (#4740) | `absorbido` | `71e7690a` (rama `inc-20/pr7-absorcion-upstream`) | Colisión con el renombrado de la Fase 4 (V1) en `axiom-telemetry-backup` y su `.test.sh`; resuelta tomando la sustancia de upstream sobre la grafía Axiom. |
| `9dc5fc7a` | fix(telemetry): archive the VictoriaMetrics snapshot data instead of its symlinks (#4742) | `absorbido` | `5d9ffeb8` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `1933540e` | fix(telemetry): raise the VictoriaMetrics scrape size cap for the collector exposition | `absorbido` | `000bf6b0` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `4251bd8f` | fix(telemetry): give runtime delivery ids their own short retention and purge them in batches | `absorbido` | `6f81d2de` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `56cad8c0` | feat(telemetry): truncate the WAL and vacuum the database after the daily purge | `absorbido` | `54a89a1d` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `96d442d0` | docs(telemetry): describe the dedup retention, the daily compaction and the offline first vacuum | `absorbido` | `f35bd185` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `99ae347f` | fix(telemetry): bound the online vacuum, checkpoint passively and validate the dedup window | `absorbido` | `a1ace3ae` (rama `inc-20/pr7-absorcion-upstream`) | — |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| — | Ninguno | Los 34 ficheros derivados de los 19 commits aparecen todos en el diff de la tanda; no hay ninguna ausencia que justificar. |

## F3 — Reviewer y parsing de OpenCode

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `f927da73` | fix(review): classify OpenCode reviewer task outcomes explicitly | `absorbido` | `dd7bdb6d` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `25c1dc78` | fix(review): harden OpenCode task wrapper parsing to the shipped grammar | `absorbido` | `5af347e4` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `a160dafd` | fix(review): require complete Task frames before reporting host states | `absorbido` | `fe075346` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `bae15d4f` | fix(review): admit the complete no-result frame for non-completed states | `absorbido` | `0c5997c2` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `9ec0cf44` | fix(opencode): restore compatible review consent (#4584) | `absorbido` | `3c5d79db` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `cd95b782` | fix(review): suppress consumed target re-review (#4737) | `absorbido` | `c5df8d52` (rama `inc-20/pr7-absorcion-upstream`) | Importaciones `gentle-ai/v3` reconciliadas a mano a `/v2`; la Fase 17 las reescribirá con el resto del árbol. |
| `e8811b53` | fix(review): gate active status on asset freshness (#4747) | `absorbido` | `3becff09` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `e28af0fd` | feat(opencode): add version-aware v2 beta support (#4728) | `absorbido` | `5a395e45` (rama `inc-20/pr7-absorcion-upstream`) | Reconciliación manual en `internal/opencode/config.go`: se combinó el parseo nuevo de upstream (`model.ParseModelReference` con fallback de `Effort`) con el espejo de claves legacy del fork hacia `axiom-orchestrator`. Único solape con INC-18 (`internal/cli/run.go`), en regiones disjuntas del fichero. Importaciones `/v3` reconciliadas a mano a `/v2`. |
| `b5851c32` | fix(review): freeze generated-path interpretation in snapshots | `absorbido` | `8c8aefd2` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `fdaf2625` | fix(review): summarize frozen generated paths without content hunks | `absorbido` | `1c0efd99` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `c2174348` | fix(review): enforce runtime input budgets before starting authority | `absorbido` | `f9192220` (rama `inc-20/pr7-absorcion-upstream`) | Importaciones `gentle-ai/v3` reconciliadas a mano a `/v2`; la Fase 17 las reescribirá con el resto del árbol. |
| `f7d737aa` | fix(review): bound complete role prompts and corrective retries | `absorbido` | `9c8d89b3` (rama `inc-20/pr7-absorcion-upstream`) | Importaciones `gentle-ai/v3` reconciliadas a mano a `/v2`; la Fase 17 las reescribirá con el resto del árbol. |
| `c78b411b` | fix(review): keep legacy authorities valid against live evidence | `absorbido` | `9c6ef0d5` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `da6cc368` | fix(review): summarize generated paths for refuter and validator too | `absorbido` | `7d7fc422` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `99df73fb` | test(review): cover the reported candidate shapes end to end | `absorbido` | `13071414` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `3c2d6f1c` | fix(review): stop promising the validator content it is not handed | `absorbido` | `dba0ca81` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `812d152a` | fix(review): measure the role envelope START admits a candidate under | `absorbido` | `e93c2907` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `bf969778` | fix(review): charge the frozen policy in the role envelope floor | `absorbido` | `7f6b9036` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `af0008c1` | test(review): prove recover keeps a non-destructive exit for over-budget lineages | `absorbido` | `8411c331` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `a9e74085` | fix(review): stop promising an exit the recovered lineage can lose | `absorbido` | `9392767e` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `22b67765` | fix(review): classify the correction-stage budget refusal and name its exit | `absorbido` | `af765fdd` (rama `inc-20/pr7-absorcion-upstream`) | Importaciones `gentle-ai/v3` reconciliadas a mano a `/v2`; la Fase 17 las reescribirá con el resto del árbol. |
| `b93c9ea1` | fix(review): keep the new stop row inside Pi's facade-only contract | `absorbido` | `889b6164` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `15ea98ed` | fix(review): relay provider-owned OpenCode lens tasks (#4765) | `absorbido` | `18d26672` (rama `inc-20/pr7-absorcion-upstream`) | Añade `capabilities-v2.6.schema.json` y `status-v8.schema.json` bajo `contracts/`: ficheros **nuevos**, permitidos por la decisión D6 porque no modifican ningún fichero preexistente y REQ-20.10 protege el estado previo. 2 ficheros de `bench/` derivados y ausentes por ruta prohibida. Importaciones `/v3` reconciliadas a mano a `/v2`. |
| `08d14841` | feat(review): accept host-submitted refuter and validator results through --input | `absorbido` | `1cd17006` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `55eefed3` | feat(review): make pi refuter and validator captures host-mediated and stop spawning pi | `absorbido` | `385e6e03` (rama `inc-20/pr7-absorcion-upstream`) | Añade `status-v9.schema.json` (fichero nuevo, decisión D6). Acepta el borrado de upstream de `internal/agents/pi/review_routing.go` y su test: eran los únicos portadores de `AXIOM_PI_CONFIG_HOME` y `.pi/axiom/`, y desaparecen con la función que configuraban, no revierten a la grafía de upstream. Verificado: cero rutas pi de upstream reintroducidas y `GENTLE_PI_CONFIG_HOME` baja de 5 ficheros a 2. Importaciones `/v3` reconciliadas a mano a `/v2`. |
| `070f82ed` | fix(review): carry the release command on the correction budget stop | `absorbido` | `13e89ff3` (rama `inc-20/pr7-absorcion-upstream`) | — |
| `55a1a072` | fix(review): charge the lens context terminator against its own budget | `absorbido` | `e8b2ac08` (rama `inc-20/pr7-absorcion-upstream`) | Importaciones `gentle-ai/v3` reconciliadas a mano a `/v2`; la Fase 17 las reescribirá con el resto del árbol. |
| `71a47477` | feat(review): report review_due and the exact preflight transition from review assess | `descartado-deliberadamente` | Decisión D6 del mantenedor, 2026-09-19 | Modifica `contracts/review-integration/v2/schemas/assess.schema.json`, fichero **preexistente** que declara `additionalProperties: false`, añadiendo `review_due`, `review_due_reason` y `consumed` a `required`. REQ-20.10 (decisión D2.3) exige que `contracts/**` quede byte a byte idéntico a su estado previo para no romper a los consumidores externos que validan contra `gentle-ai.review-integration/v2`. Absorber su Go sin el esquema tampoco podía quedar verde: `internal/cli/review_assess_test.go:154` valida contra el esquema publicado. Descarte decidido por el usuario (decisión D6). |
| `972446f1` | docs(review): make the post-commit review rule follow assess review_due and next_transition | `descartado-deliberadamente` | Decisión D6 del mantenedor, 2026-09-19 | Documenta exclusivamente `review_due` y `next_transition`, la función que introduce `71a47477`. Absorberlo dejaría documentación de una función que el fork no tiene. Descarte decidido por el usuario (decisión D6). |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| `bench/{journeys_atomic_review.go, journeys_atomic_review_test.go, journeys_capture_evidence_v5.go, journeys_intended_untracked.go, journeys_issue_2138.go, main.go, runner.go, runtime_fixture.go, runtime_fixture_test.go}` (9) | Sí | Ruta prohibida D-10 (`bench/`). Módulo Go independiente sin `go.work`, declarado fuera de la cobertura de verificación. |
| `odd/tasks/{rdd-terminal-consumption.md, opencode-rdd-provider-task.md, opencode-v2-support.md, 4504-active-lineage-asset-freshness.md, review-runtime-context-budget.md}` (5) | Sí | Ruta prohibida D-10 (`odd/tasks/*.md`). |
| `contracts/review-integration/v2/schemas/assess.schema.json`, `internal/cli/review_assess.go`, `internal/cli/review_assess_test.go` (3) | Sí | Exclusivos de `71a47477`, descartado deliberadamente (decisión D6). Verificado byte a byte idénticos a `f3-base`. |
| `docs/usage.md`, `internal/components/agentguidance/routing.go`, `internal/components/agentguidance/routing_test.go` (3) | Sí | Exclusivos de `972446f1`, descartado deliberadamente (decisión D6). Verificado byte a byte idénticos a `f3-base`. |

## F4 — Poda y refactor SDD

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F5 — CLI y community-tools RTK

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F6 — Retirada destructiva de la capa Go de ODD

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F7 — Cierre del registro y documentación

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
