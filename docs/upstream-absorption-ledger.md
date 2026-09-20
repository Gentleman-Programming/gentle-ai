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
| `absorbido` | 79 |
| `descartado-deliberadamente` | 12 |
| `revertido` | 0 |
| **Total** | **91 (= universo declarado en la cabecera: 91)** |

## F0 — Identidad de distribución, artefacto de release y cobertura del binario real

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F1 — Migración de la ruta de módulo Go `/v2` → `/v3`

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `2594581e` | fix!: move the Go module path to /v3 so v3.x is installable through go install (#4683) | `absorbido` | `d49cc604` (rama `inc-20/pr7-absorcion-upstream`) | Re-derivado sobre el árbol del fork, no aplicado como parche: de los 629 ficheros distintos que toca en upstream, 623 existen en el fork y los 623 quedan cubiertos por esta reescritura (ver sub-tabla RA-1 para los 6 restantes). Cubre además `go.mod` y otros ~47 ficheros propios del fork que upstream no tiene, y las 8 líneas de import bajo rutas nominalmente prohibidas (`internal/multirole`, `internal/handoff`, `internal/semantic`, `internal/livingdoc`, `internal/components/uninstall/cleaners.go`) cuyo `go build ./...` dependía de esta reescritura. |
| `9bf454d4` | fix(scripts): install from the /v3 module in install.sh and install.ps1 (#4686) | `absorbido` | `d49cc604` (rama `inc-20/pr7-absorcion-upstream`) | Su mitad mecánica (GONOSUMDB/GOPRIVATE/GONOPROXY y `go install ...@latest`) ya la capturaba la derivación de `gentle-ai/v2`. Su otra mitad no: `install.sh`/`install.ps1` construyen el paquete de `go install` por interpolación (`${GITHUB_REPO}/v2/cmd/...`), sin el literal `gentle-ai` delante, así que sobrevivía a cualquier búsqueda de esa cadena. Corregidas ambas líneas y portado el test de regresión de upstream `TestInstallScriptsGoInstallPackageMatchesModuleMajor`, verificado en verde contra el árbol del fork. |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| `internal/components/communitytool/rtk_{runtime,runtime_test,source,source_test}.go` (4) | Sí | Retirados íntegros en la Fase 9 (`110f1371`): nacen en `08206a15` y mueren en `110f1371`, ciclo que el fork nunca absorbió (ya documentado en la sub-tabla RA-1 de F5). |
| `internal/components/sdd/odd_integration_test.go` | Sí | No existe en el fork (ya documentado en la sub-tabla RA-1 de F3). |
| `internal/cli/review_pi_role_routing_test.go` | Sí | Divergencia de re-derivación ya verificada al absorber `55eefed3`/`385e6e03` en F3: la superficie `pi`/`reviewerprovider` del fork tomó una forma distinta de la de upstream (cero rutas `pi` de upstream reintroducidas, `GENTLE_PI_CONFIG_HOME` de 5 a 2 ficheros). No es una omisión de esta fase. |

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
| `6aa37bba` | fix(sdd): report the ledger's remediation revision from status | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito de forma equivalente, no aplicado como parche: el subsistema de remediación que corregía (`runtime_compact.go`) se retira íntegro en `18fa04fb`/`62ce74b7`, así que no queda superficie viva que reportar. |
| `762cddb8` | fix(sdd): treat a budget-exceeded pass as the chain's unremediated attempt | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito de forma equivalente: `runtime_compact.go` y su test de presupuesto no sobreviven a `18fa04fb`. |
| `b8f83adc` | fix(sdd): decide the remediation pointer by chain equality before shape | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito de forma equivalente: el test de igualdad de cadena de remediación no existe ni en el fork ni en el árbol final de upstream. |
| `b55dd6b8` | fix(sdd): derive Claude Code SDD dispatch authority from the session transcript | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: el comportamiento se incorpora a `sdd_preflight_hook.go` e `inject.go` por fusión a tres vías. |
| `e0774e05` | refactor(sdd)!: remove RDD from the SDD lifecycle | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: `status.go` y `status_v2.go` pierden `ReviewOffer`/`ReviewDisabled`; `review_door.go` y `review_offer.go` retirados íntegros. Retira RDD del ciclo SDD (decisión D7). |
| `18fa04fb` | refactor(sdd)!: retire attempt governance and preserve edit grants | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: `internal/cli/sdd_attempt.go` pasa de 737 a 85 líneas conservando **exactamente `grant`**, conforme a la tarea 8.3 y al delta firme de REQ-13.3. `acquire` y `settle` dejan de publicarse (decisión D7). |
| `6377d352` | fix(sdd): remove retired helpers and repair platform checks | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Re-derivado en la misma pasada que `18fa04fb`: limpieza de huérfanos de esa misma retirada. |
| `62ce74b7` | refactor(sdd)!: make verification optional and archive without attestation | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: `verification.go` (730 líneas) retirado íntegro, `resolveDependencies` simplificado, `sdd_verify_validate.go` retirado. La verificación pasa a opcional y el archivo deja de exigir atestación. |
| `15cbbde4` | refactor(sdd): retire unused research capability package | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: `internal/agents/researchcapability` retirado íntegro (tarea 8.8), cero consumidores confirmado antes y después. |
| `ba3ed690` | refactor(sdd)!: replace research admission with optional investigation | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: delta firme de `sdd-research` (tarea 8.7) más el test de investigación opcional. |
| `f6703634` | fix(sdd): scope optional research above the phase gatekeeper | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Re-derivado en la misma pasada que `ba3ed690`. |
| `87d7e65d` | fix(sdd): preserve planning detail without arbitrary artifact caps | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: test de detalle de planificación portado. |
| `67188f78` | fix(sdd): recover from resolved artifacts without duplicate state | `absorbido` | `000b6f03`, `24f24838` (rama `inc-20/pr7-absorcion-upstream`) | Reescrito, no aplicado como parche: test de recuperación de artefactos resueltos portado. |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| `bench/**` (33) | Sí | Ruta prohibida D-10. Módulo Go independiente sin `go.work`, fuera de la cobertura de verificación. Consecuencia registrada: `.github/workflows/ci.yml` **conserva** los 11 journeys `tr*` en vez de adoptar la reducción a 1 de upstream, porque `bench/` sigue intacto y seguiría emitiendo el conjunto completo. |
| `runtime_chain_failed_attempt_budget_test.go`, `runtime_remediation_pointer_equality_test.go`, `status_remediation_chain_revision_test.go` (3) | Sí | Nombres transitorios dentro de la propia secuencia de upstream: no existen ni en el fork ni en `67188f78`. Cero acción necesaria. |
| Resto de los 259 ficheros en alcance | No | Presentes en el diff: 170 adoptados verbatim (fork sin divergencia previa), 68 fusionados a tres vías (32 conflictos resueltos a mano), 17 materializados nuevos, 1 borrado tras revisar su divergencia (`internal/sddstatus/review_door.go`, sin superficie viva). |

## F5 — CLI y community-tools RTK

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `08206a15` | feat(community-tools): add RTK runtime integration | `absorbido` | `f11d9bea` (rama `inc-20/pr7-absorcion-upstream`) | Su efecto queda superado por `110f1371` dentro de esta misma tanda: upstream añade la integración RTK y la retira en la misma secuencia. Lo absorbido es el estado final (RTK retirado), no el arco intermedio, que el fork nunca llegó a tener — verificado byte a byte contra `08206a15^`. |
| `1174de2c` | feat(cli): wire RTK install status and sync | `absorbido` | `f11d9bea` (rama `inc-20/pr7-absorcion-upstream`) | Su efecto queda superado por `110f1371` dentro de esta misma tanda: upstream añade la integración RTK y la retira en la misma secuencia. Lo absorbido es el estado final (RTK retirado), no el arco intermedio, que el fork nunca llegó a tener — verificado byte a byte contra `08206a15^`. |
| `c6824eb3` | fix(community-tools): avoid Pi runtime identity literal | `absorbido` | `f11d9bea` (rama `inc-20/pr7-absorcion-upstream`) | Su efecto queda superado por `110f1371` dentro de esta misma tanda: upstream añade la integración RTK y la retira en la misma secuencia. Lo absorbido es el estado final (RTK retirado), no el arco intermedio, que el fork nunca llegó a tener — verificado byte a byte contra `08206a15^`. El literal de identidad de Pi que este commit corrige **nunca existió en el fork**: `rtk_source.go` era idéntico al estado previo al arco y desaparece entero. |
| `b4084a74` | fix(community-tools): scope RTK setup to selected agents | `absorbido` | `f11d9bea` (rama `inc-20/pr7-absorcion-upstream`) | Su efecto queda superado por `110f1371` dentro de esta misma tanda: upstream añade la integración RTK y la retira en la misma secuencia. Lo absorbido es el estado final (RTK retirado), no el arco intermedio, que el fork nunca llegó a tener — verificado byte a byte contra `08206a15^`. |
| `8c078527` | fix(cli): scope agent artifacts independently of runtime cwd (#4668) (#4673) | `absorbido` | INC-18 (`2026-09-16-inc-18-rdd-decoupling-and-v3-stability-fixes`) | Ya absorbido por INC-18, que lo **re-derivó** en vez de aplicarlo: por eso no es ancestro de `HEAD` y no aparece en `git log`. Verificado por contenido: `componentInjectionDirScoped` existe en `internal/cli/run.go` con la misma firma y 11 usos, y `resolveOpenClawWorkspaceDir` y `openClawWorkspaceConfig`, que este commit retira, están ausentes del fork. No se reaplica aquí: duplicaría. |
| `1a2f6775` | fix(cli): decode SDD status JSON when asserting granted roots on Windows (#4676) | `absorbido` | INC-18 (`2026-09-16-inc-18-rdd-decoupling-and-v3-stability-fixes`) | Ya absorbido por INC-18 por re-derivación. Verificado por contenido: `internal/cli/sdd_attempt_test.go:48-56` decodifica el JSON con `json.Unmarshal` en vez de comparar subcadenas, con el comentario sobre las barras invertidas escapadas en Windows. La tanda F4 de esta misma rama conservó deliberadamente esa versión del fork frente a la de upstream. |
| `110f1371` | refactor(community-tools): retire RTK integration | `absorbido` | `f11d9bea` (rama `inc-20/pr7-absorcion-upstream`) | Re-derivación al estado final, no parche aplicado: RTK retirado por completo (4 ficheros borrados, la constante `CommunityToolRTK` y 3 referencias residuales limpiadas, baseline de código muerto actualizado con 3 borrados y cero adiciones). La mayoría de sus hunks eran no-op sobre el fork, que nunca entró en el arco. |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| `odd/tasks/retire-rtk.md` | Sí | Ruta prohibida D-10 (`odd/tasks/*.md`). |
| `rtk_runtime.go`, `rtk_runtime_test.go` | Sí | Nacen en `08206a15` y mueren en `110f1371`: ciclo completo dentro del arco que el fork nunca absorbió, así que nunca existieron aquí. |
| `tool.go`, `internal/cli/run.go`, `sync.go`, `internal/tui/model.go`, `model_test.go`, `community_tools_test.go`, `run_community_tool_test.go`, `docs/{components,usage,pi,codebase/integrations}.md` | Sí | Cero menciones a RTK en el fork: los hunks de `110f1371` sobre ellos son no-op. Verificado con `diff` vacío contra `08206a15^` en el paquete `communitytool`. |
| `internal/cli/run_component_paths_test.go`, `internal/cli/sdd_attempt_test.go` | Sí | Exclusivos de `8c078527` y `1a2f6775`, ya absorbidos por INC-18; fuera de esta re-derivación. |
| `internal/state/state_test.go` | Sí | Su fixture ya usaba `"jq"` en vez de `"rtk"`: divergencia propia del fork anterior a esta tanda, que hace innecesario el cambio. |

## F6 — Retirada destructiva de la capa Go de ODD

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `1b202d77` | feat(odd): add unified feature continuity and configured TDD (#4608) | `absorbido` | `b6c941d2`, `3a618ad2` (rama `inc-20/pr7-absorcion-upstream`) | Continuidad de *feature* (`mem_context`/`mem_search`/`mem_get_observation`) y TDD configurado, absorbidos hacia la sección `### Organic Driven Development` de `internal/components/agentguidance/routing.go`. |
| `70c774f8` | feat(odd): make ODD the orchestrator's mandatory default protocol (#4644) | `absorbido` | `b6c941d2`, `3a618ad2` (rama `inc-20/pr7-absorcion-upstream`) | Protocolo ODD de 7 pasos como flujo predefinido del orquestador, absorbido hacia `### ODD protocol (MANDATORY…)`. Su carga paralela sobre 12 `sdd-orchestrator.md` y sus goldens queda fuera del alcance que `tasks.md` asigna a F6.1. |
| `cfc415ce` | feat(odd): close each task with a work-unit commit, review per commit or slice, and offer chained delivery (#4714) | `absorbido` | `b6c941d2`, `3a618ad2` (rama `inc-20/pr7-absorcion-upstream`) | Cierre por commit de unidad de trabajo y candidato de revisión por commit o rebanada, absorbidos hacia los pasos 6 y 7 del protocolo. Su carga paralela sobre 2 `SKILL.md` queda fuera de alcance. |
| `794c5326` | docs(odd): explain everyday workflow and optional SDD (#4636) | `absorbido` | `b6c941d2`, `3a618ad2` (rama `inc-20/pr7-absorcion-upstream`) | Doctrina «ODD por defecto, SDD opcional» absorbida hacia los 7 activos de persona y `routing.go`. El README y los `docs/` de upstream no se tocan: ninguno menciona ODD en el fork hoy, así que no se abre ninguna ventana de documentación falsa. |
| `dcd2fa07` | fix(odd): make delegation mandatory with trigger table, route declaration, and long-session backstop | `absorbido` | `b6c941d2`, `3a618ad2` (rama `inc-20/pr7-absorcion-upstream`) | Disparadores obligatorios de delegación absorbidos hacia `### Mandatory Delegation Triggers`, ligados a los mismos umbrales del manifiesto canónico que ya usaban las rutas directa y delegada. |
| `e7729359` | docs(odd): record feature outcome and per-task route/review evidence | `descartado-deliberadamente` | `git show e7729359 --stat` verificado en la tarea 10.2 | Toca **únicamente** `odd/tasks/odd-mandatory-delegation.md`, ruta prohibida por la regla 5 de este registro y por D-10. No se absorbe. |
| `03dc975e` | docs(odd): sync the feature document with the pushed branch tip | `descartado-deliberadamente` | `git show 03dc975e --stat` verificado en la tarea 10.2 | Toca **únicamente** `odd/tasks/review-runtime-context-budget.md`, ruta prohibida por la regla 5 de este registro y por D-10. No se absorbe. |
| `c9e3bdaf` | docs(odd): record RTK retirement delivery readiness | `descartado-deliberadamente` | `git show c9e3bdaf --stat` verificado en la tarea 10.2 | Toca **únicamente** `odd/tasks/retire-rtk.md`, ruta prohibida por la regla 5 de este registro y por D-10. No se absorbe. |
| `6aa08748` | docs(odd): clarify RTK evidence scope | `descartado-deliberadamente` | `git show 6aa08748 --stat` verificado en la tarea 10.2 | Toca **únicamente** `odd/tasks/retire-rtk.md`, ruta prohibida por la regla 5 de este registro y por D-10. No se absorbe. |
| `b2e5ec6b` | docs(odd): make RTK rollback evidence reproducible | `descartado-deliberadamente` | `git show b2e5ec6b --stat` verificado en la tarea 10.2 | Toca **únicamente** `odd/tasks/retire-rtk.md`, ruta prohibida por la regla 5 de este registro y por D-10. No se absorbe. |
| `163e6a3f` | docs(odd): record the pi in-process reviewer and assess transition feature outcome | `descartado-deliberadamente` | `git show 163e6a3f --stat` verificado en la tarea 10.2 | Toca **únicamente** `odd/tasks/pi-inprocess-reviewer-assess-transition.md`, ruta prohibida por la regla 5 de este registro y por D-10. No se absorbe. |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| Los 6 ficheros `odd/tasks/*.md` de los commits descartados | Sí | Ruta prohibida D-10. |
| `README.md`, `docs/{agents,components,intended-usage,pi,quickstart,trigger-rules,usage}.md` | Sí | Existen en el fork pero **ninguno menciona ODD hoy**, verificado con grep: dejarlos sin tocar no abre ninguna ventana de documentación falsa, y `tasks.md` no asigna escritura sobre ellos en F6.1. |
| 12 `internal/assets/*/sdd-orchestrator.md`, `sdd-orchestrator-sections.md`, sus goldens y `orchestrator_shared_sections_test.go`, `skills/{chained-pr,work-unit-commits}/SKILL.md` | Sí | Carga paralela de `70c774f8` y `cfc415ce` fuera del alcance que la partición de fases asigna a F6.1. |
| `odd_integration_test.go` | Sí | No existe en el fork. |

### Retiradas destructivas F6.2–F6.4 (autoría del fork, no commits de upstream)

Las Fases 11 a 15 no absorben ningún commit de upstream: retiran la capa Go de ODD del fork, que es la entrada **V7** del inventario de no-reversión. REQ-20.3 autoriza esa retirada **solo a F6** y **solo mediante los deltas de las capacidades `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion` y `odd-ui-integration`**, aplicados en la Fase 15. Se registran aquí por trazabilidad, con el commit del fork como evidencia.

| Fase | Qué retira | Evidencia | Tamaño |
|---|---|---|---|
| F6.2a (11) | Subcomandos `axiom odd create|status|promote`: 6 ficheros de `internal/cli` y los puntos de `main.go` | `09602ab7` | 9 ficheros, 1140 bajas |
| F6.2b (12) | Web UI: `odd_service.go`, 4 rutas y manejadores de `server.go`, 3 DTOs, superficie de `assets/` | `28298be3` | 8 ficheros, 1092 bajas |
| F6.2c (13) | TUI: `odd_features.go`, los puntos de `model.go`, `router.go` y `governance.go` | `1b7cdd5f` | 9 ficheros, 529 bajas |
| F6.3 (14) | `internal/odd/**` íntegro. Un paquete Go no se borra a medias sin romper su compilación: la prueba de ejecución **es** `go build ./...` | `79dca9bc` | 19 ficheros, 3537 bajas |
| F6.4 (15) | Deltas destructivos sobre las 4 capacidades `odd-*` que autoriza REQ-20.3 | `ba8fb27e` | 6 ficheros, 79+/266− |

**Evidencia inversa de D-02, ejecutada sobre el binario construido tras la Fase 14**: `axiom odd status` responde `Error: comando 'odd' no reconocido.`, frente al estado real que devolvía al cerrar la Fase 10. La ventana entre doctrina retirada y verbo vivo queda cerrada.

**`odd/tasks/*.md` no se toca**: es ruta prohibida por D-10, así que los documentos vivos permanecen aunque el comando que los leía ya no exista. Cero ficheros de `odd/` en el diff de estas fases.

**Corrección de frontera aplicada por el orquestador.** El commit del trinquete borraba cinco funciones que esta retirada dejó huérfanas, pero **tres vivían en rutas prohibidas**: `Manager.Save` en `internal/hub/manager.go` — que es además la entrada **V8** — y `Service.GetSpecsRoot`/`Service.GetIndexPath` en `internal/livingdoc/service.go`. Se restauraron ambos ficheros y las tres funciones quedan **declaradas en `.deadcode-baseline.txt`** (260 → 263 entradas), que no es ruta prohibida, conforme a la regla 8 del plan: las correcciones transversales sobre rutas prohibidas van en su propia PR tras fusionar la cadena. Las dos huérfanas en alcance (`Service.GetRoles` y `Service.SetHubManager`, en `internal/dashboard/service.go`) sí se borraron. Evidencia: `cfad7866`; `./scripts/deadcode-ratchet.sh` en modo CI devuelve `no new unreachable functions`.

## F7 — Cierre del registro y documentación

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `80c927ae` | fix(sync): accept a symlinked agent config root for telemetry runtime | `absorbido` | `4d88405f` (rama `inc-20/pr7-absorcion-upstream`) | Cherry-pick limpio. |
| `cd5a3af6` | docs(readme): correct what SDD Verify and Archive actually do | `absorbido` | `b50b0a40` (rama `inc-20/pr7-absorcion-upstream`) | Conflicto textual en `README.md` resuelto a mano: la prosa propia del fork se sustituye por la corregida de upstream, coherente con el `alt` y el SVG. El fork sí tiene sección `### SDD` y `sdd-cycle.svg`, así que este commit sí tiene dónde aplicarse, a diferencia de sus hermanos de ODD. |
| `5b82c00d` | test(mcp): gate native Claude discovery on the v2.1.154 reporting floor (#4157) | `absorbido` | `a0e02a84` (rama `inc-20/pr7-absorcion-upstream`) | Cherry-pick limpio. |
| `27d71d57` | fix(sync): prevent Gentle Logo from configuring unselected OpenCode agent (#4780) | `absorbido` | `044e5e8d` (rama `inc-20/pr7-absorcion-upstream`) | Cherry-pick limpio. **V4 verificado intacto**: el fix actúa en la capa de ejecución (`componentApplyStep.Run`, `componentSyncStep.Run`), no en `internal/model/presets.go`, donde `installSafePresetVisualComponents()` sigue excluyendo `ComponentOpenCodeGentleLogo` a propósito. |
| `11f6c000` | fix(skills): keep contributor skills out of the default preset (#4669) (#4671) | `absorbido` | INC-18 (`2026-09-16-inc-18-rdd-decoupling-and-v3-stability-fixes`, tarea `task-18-5-upstream-skills-presets-hygiene`) | Ya absorbido por INC-18, que lo **re-derivó** en vez de aplicarlo: no es ancestro de `HEAD`. Verificado por contenido: `internal/components/skills/presets.go` es byte a byte el resultado de este diff (mismas `contributorSkills`, `selectableFoundationSkills`, `excludeSkills`), `docs/components.md` ya distingue Foundation de contributor y el golden de presets no menciona las skills retiradas. No se reaplica. |
| `82a6de96` | test(windows): set USERPROFILE in the V2 catalog harness and shell-quote the expected assess --cwd token (#4789) | `absorbido` | `3afa11be` (rama `inc-20/pr7-absorcion-upstream`) | **Absorción parcial.** Su hunk sobre `internal/assets/opencode_v2_plugins_test.go` entra limpio. El hunk sobre `internal/cli/review_assess_test.go` **queda fuera**: modifica `assertReviewAssessNextTransition` y `TestReviewAssessHumanReadableOutputNamesDueTransition`, que solo introduce `71a47477`, descartado por la decisión D6. Cherry-pick real → `CONFLICT (content)`. Es el commit de la propia etiqueta `v3.4.0`. |
| `95867aa4` | fix(tui): remove dead community tool runner | `descartado-deliberadamente` | Cherry-pick real → `CONFLICT`; llamadores verificados en el árbol | **La función que este commit borra sigue viva en el fork.** `runCommunityToolCommand` (`internal/tui/model.go:3711`) la consume `communitytool.RunnerFunc(runCommunityToolCommand)` en `startCommunityToolInstallation` (`:3654`), invocada desde el bucle Update (`:4753`) y ejercitada por `model_test.go:2418`. Upstream pudo borrarla porque su arco RTK (`08206a15`→`110f1371`) la dejó huérfana allí; el fork **nunca entró en ese arco**, como estableció la Fase 9. Absorberlo rompería `go build`. |
| `0fbd8dd8` | docs(readme): present ODD as a feature with its own cycle diagram | `descartado-deliberadamente` | Encabezados de `README.md` y `docs/usage.md` verificados en el árbol | El README del fork **no tiene sección `### ODD`** (sus encabezados son Engram, SDD, RDD, Deterministic, Gentle Shell, 16 agentes, Also in the box), `docs/usage.md` menciona ODD **cero veces** —el enlace `docs/usage.md#organic-driven-development-odd` quedaría roto— y `docs/assets/diagrams/odd-cycle.svg` no existe. Absorberlo no sería absorber sino **fabricar documentación** de una superficie ejecutable que las Fases 11–14 acaban de retirar, que es justo lo que D-02 prohíbe. |
| `a6ab6ddb` | docs: improve ODD workflow diagram | `descartado-deliberadamente` | `git show a6ab6ddb --stat` | Toca **únicamente** `docs/assets/diagrams/odd-cycle.svg`, que no existe en el fork ni se va a crear por el motivo de la fila anterior. |
| `9f15bc44` | test(bench): adapt community tool navigation | `descartado-deliberadamente` | `git show 9f15bc44 --stat` | Toca **únicamente** `bench/journeys_issue4377.go`, ruta prohibida por D-10 y por la regla 5 de este registro. |

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
| `bench/journeys_issue4377.go` | Sí | Ruta prohibida D-10; único fichero de `9f15bc44`. |
| `docs/assets/diagrams/odd-cycle.svg`, sección `### ODD` de `README.md` | Sí | No existen en el fork; ver las filas de `0fbd8dd8` y `a6ab6ddb`. |
| `internal/cli/review_assess_test.go` | Sí | Hunk de `82a6de96` dependiente de `71a47477`, descartado por D6. |
| `internal/tui/model.go` (hunk de `95867aa4`) | Sí | La función que borra sigue teniendo llamador vivo en el fork. |
| `internal/components/skills/presets.go`, `presets_test.go`, `skills-presets.json`, `e2e/e2e_test.sh`, `docs/components.md` | Sí | Exclusivos de `11f6c000`, ya absorbido por INC-18; reaplicarlos duplicaría. |
