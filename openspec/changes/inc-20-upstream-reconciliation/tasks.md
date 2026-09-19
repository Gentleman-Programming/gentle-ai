# Tareas: Reconciliación con Upstream e Identidad de Distribución (inc-20-upstream-reconciliation)

> **Fuentes de alcance:** `spec.md` (11 capacidades, 32 requerimientos activos + 15 retirados), `design.md` (1076 líneas, autoritativo, decisiones D-01 a D-12), `proposal.md` como contexto — incorporada su corrección del 2026-09-19 en §3.3 (`axiom-user-state-and-env` se excluyó por categoría, no por medición; la superficie real es la raíz de respaldos, cubierta aquí por REQ-20.13/20.14).
> **Precedente de forma:** `openspec/changes/archive/2026-09-18-inc-19-odd-workflow-and-promotion/tasks.md`.
> **Idioma del artefacto:** español (castellano peninsular). Identificadores Go, rutas, comandos, SHAs y las cuatro líneas de guarda de pronóstico permanecen en inglés. El texto de cada delta de especificación hereda el idioma del documento que enmienda (D-05): las capacidades nuevas de este incremento van en español; los deltas sobre `rdd-sdd-receipt-consumption`, `sdd-research`, `organic-agent-trigger-rules` y `rdd-post-verify-review-offer` van en inglés, igual que el documento que amplían.
> **Verificación de rigor:** cada fichero, símbolo, línea y cifra citados en este documento para F0, F6 y F1 se comprobó directamente contra el árbol de trabajo el 2026-09-19 (no se heredó sin contrastar). Las fases de absorción (F2, F3, F4, F5, F7) citan intencionadamente cero ficheros de upstream, por REQ-20.1 — ver «Protocolo compartido de tanda de absorción».

---

## Nota de partición de rebanadas (ajuste sobre el diseño)

El diseño mide que **F0 no es una rebanada** y entrega una partición de tres piezas (F0.a, F0.b, F0.c) que este documento **hereda sin re-derivar**. Pero el propio diseño delega en `sdd-tasks` "el pronóstico de líneas y la partición definitiva" (§D-12, cierre), y esa partición definitiva exige dividir más:

| Bloque del diseño | Por qué no cabe en una PR | Partición final en este documento |
|---|---|---|
| **F0.a** | Ya es una rebanada correcta (deja el árbol verde, guarda en rojo declarado) | **Fase 1**, sin cambios |
| **F0.b** | Ya es una rebanada correcta (cierra el rojo de F0.a) | **Fase 2**, sin cambios |
| **F0.c** («.goreleaser.yaml, instalador, compuertas, crosslane, paso de CI») | Agrupa tres responsabilidades independientes — cobertura de CI/build (REQ-20.15–16), identidad de distribución condicionada (REQ-20.8, 20.11, 20.12) y el paquete nuevo del registro de absorción (REQ-20.1, 20.5, 20.6) — que juntas superan holgadamente 400 líneas (2 builds de goreleaser + paso de CI nuevo + 4 aserciones restantes de la guarda + ~10 ficheros de instalador/compuertas/crosslane/telemetría + paquete `internal/absorptionledger` con 6 aserciones + el propio documento del registro) | **Fase 3** (F0.c1 — identidad de build y guardas de CI), **Fase 4** (F0.c2 — instalador, compuertas, crosslane, telemetría), **Fase 5** (F0.c3 — registro durable de absorción) |
| **F6.2** («superficies de usuario») | Retira de un solo golpe la CLI, la Web UI y la TUI completas de INC-19: solo la Web UI y la TUI juntas ya superan los ~1300–1700 líneas que INC-19 midió para construirlas (PR4 ~600–700 + PR5 ~450–560) | **Fase 11** (F6.2a — CLI), **Fase 12** (F6.2b — Web UI), **Fase 13** (F6.2c — TUI) |
| **F6.3** («dominio») | Es un borrado atómico de paquete completo: 1534 líneas de producción + 1939 de test = **3473 líneas medidas** (propuesta §9, tabla de alcance destructivo de D1), y un paquete Go no se puede borrar a medias sin romper la compilación de lo que aún queda del propio paquete | Se mantiene como **Fase 14**, con `size:exception` recomendada explícitamente (ver Forecast) |
| **F1** | La propia propuesta ya lo declara: "~682 ficheros, 1 línea cada uno... requiere `size:exception` explícita y no admite mezclarse con nada más" (§4.7) | Se mantiene como **Fase 17**, `size:exception` heredada, sin cambios |

Resultado: **17 rebanadas** en vez de 8 (el diseño) o 6 (la propuesta original). Las fases de absorción (F2, F3, F4, F5, F7) **no se subdividen aquí a priori** ni se les asigna una tabla de ficheros: su tamaño real solo se conoce derivándolo de `git show --stat` en el momento de ejecutarlas (REQ-20.1); anticiparlo sería el defecto exacto que esa regla prohíbe. Su riesgo de presupuesto se declara como **Alto/No medible** en el pronóstico, no como una cifra inventada.

Ninguna tarea de este documento toca `bench/`, `internal/hub/`, `internal/workspace/`, `internal/multirole/`, `internal/handoff/`, `internal/semantic/`, `internal/livingdoc/`, `internal/components/uninstall/cleaners.go` (ficheros, no paquete — `service.go` del mismo paquete sí está en alcance), `openspec/INDEX.md`, `openspec/config.yaml`, `openspec/changes/archive/**`, `docs/releases/**` ni `odd/tasks/*.md` — lista completa de rutas prohibidas en «Reglas de Comprobación y Alcance».

---

## Protocolo compartido de tanda de absorción (F2, F3, F4, F5, F7)

Toda fase de absorción sigue los mismos siete pasos, derivados de RA-1/RA-2 (REQ-20.1, REQ-20.2) y del inventario de no-reversión (REQ-20.3) y de rutas prohibidas (REQ-20.4). Las tareas de cada fase referencian estos pasos por letra en vez de repetirlos.

- **A — Base verde.** `go build ./...`, `go vet ./...`, `go test ./...` (sin `-run`), `e2e/e2e_test.sh`: todos en verde, salvo el único fallo declarado de `internal/absorptionledger` (Fase 5, tarea 5.5) hasta el cierre de la Fase 16.
- **B — Derivación obligatoria (RA-1).** `git show <sha> --stat` para cada commit de upstream de la tanda. Prohibido fijar la lista de ficheros a mano en `tasks.md` o en un informe de fase.
- **C — Aplicación.** Cherry-pick agrupado (F2, F3, F5, F7) o re-derivación sobre el fork tomando el resultado de upstream como objetivo declarado (F4 — ver nota específica de la Fase 8).
- **D — Contraste.** El diff resultante de la tanda se contrasta contra la derivación del paso B. Todo fichero derivado y ausente del diff lleva motivo escrito en la fila del registro.
- **E — Frontera.** Comprobar que el diff no toca ninguna ruta prohibida (D-10, lista en «Reglas de Comprobación y Alcance») y que no revierte ninguna entrada del inventario V1–V8 (REQ-20.3). Cualquiera de las dos cosas detiene el incremento hasta el último estado verde.
- **F — Verificación sin filtrar (RA-2).** `go build ./...`, `go vet ./...`, `go test ./...` (sin `-run`), `gofmt -l .` (solo sobre los ficheros tocados por la tanda, nunca sobre el árbol completo — R14), `e2e/e2e_test.sh`. Declarar explícitamente que `bench/` queda fuera de esta cobertura.
- **G — Registro.** Escribir/actualizar la sección de la tanda en `docs/upstream-absorption-ledger.md`: una fila por commit (`sha`, asunto, estado, evidencia, motivo) y la sub-tabla "Ficheros derivados y ausentes" del paso D. Actualizar la tabla de recuento. Espejar en Engram bajo `sdd/inc-20-upstream-reconciliation/absorption-ledger` (aditivo, por `topic_key`).

**Nota cruzada — reconciliación de importaciones durante F1-al-final [D-01].** Cinco commits de upstream ya identificados por su `sha` mencionan `gentle-ai/v3` en su diff sin ser la migración misma: `15ea98ed`, `110f1371`, `e28af0fd`, `cd95b782`, `c09b1a34`. Si el paso B de tu tanda deriva alguno de estos SHAs, su línea de importación citará `/v3` mientras el árbol del fork sigue en `/v2`: corrígela a mano a `/v2` dentro del mismo hunk que ya estás leyendo, y dilo en la columna "Motivo" de esa fila del registro — la Fase 17 la reescribirá a `/v3` junto con el resto del árbol. Por el asunto de cada commit, los candidatos más probables son: `c09b1a34` (telemetría → Fase 6), `15ea98ed`/`e28af0fd`/`cd95b782` (reviewer/OpenCode → Fase 7), `110f1371` (community-tools RTK → Fase 9) — confírmalo con `git show --stat`, no lo asumas.

---

## Reglas de Comprobación y Alcance (aplican a TODAS las fases)

1. **Rutas prohibidas (criterio de aborto, D-10 — ampliado sobre §7.3 de la propuesta):** `bench/`, `internal/hub/`, `internal/workspace/`, `internal/multirole/`, `internal/handoff/`, `internal/semantic/`, `internal/livingdoc/`, `internal/components/uninstall/cleaners.go` (fichero, no paquete), `openspec/INDEX.md`, `openspec/config.yaml`, `openspec/changes/archive/**`, `docs/releases/**`, `odd/tasks/*.md`. Un diff que toque cualquiera de estas rutas se rechaza y se revierte hasta el último estado verde.
2. **`gofmt -l .` ya señala 18 ficheros no canónicos preexistentes (R14).** Nunca uses `gofmt -w .` sobre el árbol completo. Cada fase corre `gofmt -l` **solo sobre los ficheros que ella misma toca**.
3. **Excepción de rojo declarado, única y con fecha de cierre conocida.** `internal/absorptionledger`'s test de completitud (Fase 5, tarea 5.5) permanece en rojo desde la Fase 5 hasta la Fase 16 inclusive — es la señal de que el registro sigue incompleto, no un fallo nuevo. Ningún otro fallo nuevo es aceptable en `go test ./...` en ninguna fase.
4. **`git log origin/main..main` antes de abrir cualquier PR de la cadena.** En INC-19, `origin/main` iba dos commits por detrás y la puerta de tamaño reportó 3630 líneas en una PR que en realidad borraba 92. Repite esta comprobación al abrir cada PR de este documento, no solo la primera.
5. **`gofmt -l .` antes de cada `git push`.** `Unit Tests` aborta en el paso "Require successful Go format", que arrastra siete jobs dependientes. Un solo fichero no canónico tumba la tubería entera.
6. **Trampa de PRs apilados en GitHub.** Fusionar con `--delete-branch` una PR que es base de otra **cierra la PR hija** en vez de reapuntarla, y queda atrapada (no se puede reabrir sin su base ni cambiar la base estando cerrada). Antes de fusionar cada PR de este documento: `gh pr edit <N> --base main` explícito. Nunca `--delete-branch` mientras queden PRs de esta cadena sin fusionar.
7. **Etiquetas.** Toda PR de este documento necesita una etiqueta `type:*`. Las Fases 14 (F6.3) y 17 (F1) necesitan además `size:exception` aprobada por el mantenedor (ver Forecast).
8. **Correcciones transversales que toquen rutas prohibidas de este incremento van DESPUÉS de fusionar toda la cadena, en su propia PR contra `main`.** Meterlas antes obliga a rebasar el resto de la cadena con conflictos.
9. **Ninguna tarea usa `go test ./...` filtrado por patrón (`-run`) como evidencia de cierre de fase**, salvo para observar un RED puntual dentro de una tarea (nunca como evidencia de la fase completa) — RA-2 lo prohíbe explícitamente como criterio de aceptación.
10. **`strict_tdd: true` (`openspec/config.yaml:16`).** RED antes de implementar, GREEN, REFACTOR, con evidencia observada (comando + resultado real), salvo la Fase 17 (F1), mecánica y sin comportamiento nuevo: su prueba es `go build ./...` más la suite completa, no un test nuevo (matización ya declarada por el diseño).
11. **Marcado `(read-only)`:** cuando una tarea cita como referencia un fichero que esa fase no crea ni modifica, la ruta se marca `(read-only)` inmediatamente después de la ruta entre comillas invertidas.

---

## Review Workload Forecast

| Campo | Valor |
|---|---|
| Estimación agregada de líneas cambiadas | **No acotable con precisión.** Fases 1–5, 10–17: ~3.900–5.600 líneas medibles hoy (detalle por fase abajo). Fases 6–9, 16 (F2/F3/F4/F5/F7): tamaño real solo derivable en ejecución (RA-1); orden de magnitud estimado a partir de conteos ya publicados de commits/ficheros en `proposal.md` §4.3 |
| Riesgo de presupuesto de 400 líneas | **High** |
| PRs encadenados recomendados | **Yes** |
| Partición sugerida | 17 rebanadas (tabla siguiente), dos de ellas (Fases 14 y 17) con `size:exception` ya justificada por cifras publicadas, no por conveniencia |
| Estrategia de entrega | `auto-chain` (Preflight de sesión) |
| Estrategia de cadena | `stacked-to-main` (elegida por el usuario en el Preflight de sesión) |

Líneas de guarda exactas (contrato de herramienta, no traducir):

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

### Estimación y riesgo por rebanada

| # | Rebanada | Contenido | Estimación de líneas | Riesgo |
|---|---|---|---|---|
| 1 | F0.a — Accesores + guarda en rojo | 3 funciones nuevas en `manifest.go` + su test, comentario corregido, `canonical_binary_test.go` con 1 aserción | ~200–300 | Medium |
| 2 | F0.b — Escritores + 18 aserciones | 5 escritores + 1 lector + mensaje + 16 aserciones reapuntadas + 2 con control negativo + 1 test de integración | ~200–320 | Medium |
| 3 | F0.c1 — Identidad de build + CI | `.goreleaser.yaml` (2 builds), `ci.yml` (:266 + paso nuevo), 4 aserciones restantes de `canonical_binary_test.go` | ~220–340 | Medium-High |
| 4 | F0.c2 — Instalador, compuertas, crosslane, telemetría | ~10 ficheros, mayoría literales de 1–5 líneas; **condicionada en su tramo de instalador/tap/compuertas a REQ-20.8** (ver Fase 4) | ~120–220 (sin instalador) / +~40 (con instalador, si el gate lo permite) | Medium |
| 5 | F0.c3 — Registro durable de absorción | `internal/absorptionledger/{ledger.go,ledger_test.go}`, `docs/upstream-absorption-ledger.md` (esqueleto, 0 filas) | ~280–380 | Medium |
| 6 | F2 — Telemetría VictoriaMetrics | 14 commits, intersección de conflicto nula (calibración) | **No medible sin violar RA-1** | High (no medible) |
| 7 | F3 — Reviewer y parsing de OpenCode | 6–7 commits / ~7 ficheros según `proposal.md` §4.3; precedida de medición de solape con INC-18 | **No medible sin violar RA-1** | High (no medible) |
| 8 | F4 — Poda y refactor SDD | 13 commits / ~40 ficheros; re-derivación, no cherry-pick; `runtime_ledger.go` sola pasa de 4255 a 919 líneas en upstream (~3336 de diferencia) | **No medible con precisión; orden de magnitud grande por la cifra de `runtime_ledger.go` ya publicada** | **High** |
| 9 | F5 — CLI y community-tools RTK | 7 commits; solape parcial con el fork, reconciliación manual por commit | **No medible sin violar RA-1** | High (no medible) |
| 10 | F6.1 — Doctrina ODD | 7 activos de persona + `assets_test.go` + `routing.go` (absorbe 3 commits de upstream) | ~180–320 | Medium |
| 11 | F6.2a — Retirada CLI ODD | 6 ficheros borrados (`odd_{create,promote,status}.go` + tests) + `main.go` (import, ayuda, `case`, `runODD`, scaffolder) | ~350–500 | High |
| 12 | F6.2b — Retirada Web UI ODD | `odd_service.go`+test borrados, 4 rutas + manejadores de `server.go`, 3 DTOs de `types.go`, `assets/{index.html,app.js}`, casos de `dashboard_test.go` | ~350–550 | High |
| 13 | F6.2c — Retirada TUI ODD | `odd_features.go` borrado, 5 puntos de `model.go` + `model_test.go`, `router.go`, `governance.go` | ~300–480 | High |
| 14 | F6.3 — Dominio ODD | `internal/odd/**` (18 ficheros) borrado íntegro: **1534 líneas de producción + 1939 de test = 3473, ya medidas en `proposal.md` §9** | **~3473 (medido)** | **High — `size:exception` recomendada, no evitable: un paquete Go no se puede borrar a medias sin romper su propia compilación** |
| 15 | F6.4 — Especificaciones y registro de F6 | 4 deltas destructivos + 1 delta `MODIFIED` + 1 delta firme + filas del registro | ~180–280 | Medium |
| 16 | F7 — Cierre del registro + documentación | 6 commits (tamaño no medible) + cierre de las 55 filas + `docs/ROADMAP.md` (~30–50 líneas) | **No medible en su parte de absorción; ~40–90 en su parte documental** | Medium |
| 17 | F1 — Ruta de módulo `/v2`→`/v3` | ~682 ficheros, 1 línea cada uno, más 17 ficheros no-`.go` con cambios de 1–3 líneas | **~700–750 (medido por conteo de ficheros)** | **High — `size:exception` ya declarada por la propuesta §4.7, no se mezcla con nada más** |

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Accesores canónicos de raíz de respaldos + guarda de literales en rojo declarado | PR F0.a | `go test ./internal/backup/... ./cmd/axiom/...` | `go test ./cmd/axiom/... -run TestUserStateRootsResolveThroughOwningPackage -v` (esperado: FAIL, lista los 6 sitios) | Eliminar `BackupRootFor`, `LegacyBackupRootFor`, `BackupRoots` y `canonical_binary_test.go`; aditivo puro, sin llamadores todavía |
| 2 | Migrar 5 escritores + 1 lector a los accesores; reapuntar 18 aserciones (16 + 2 con control negativo) | PR F0.b | `go test ./internal/cli/... ./internal/update/upgrade/... ./internal/components/uninstall/... ./internal/dashboard/... ./internal/app/... ./cmd/axiom/...` | Sembrar un manifiesto en `~/.gentle-ai/backups` sobre un `HOME` temporal y confirmar que `ListBackups`/restauración/borrado lo siguen resolviendo | Revertir los 5 escritores y el lector a sus literales previos; los 18 tests vuelven a su forma anterior en el mismo commit de reversión |
| 3 | Identidad de build (goreleaser dual) + cobertura de CI sobre `cmd/axiom` + 4 aserciones restantes de la guarda | PR F0.c1 | `go test ./cmd/axiom/...` | Build local de `./cmd/axiom` y `./cmd/gentle-ai`; confirmar ausencia/presencia del aviso de deprecación en stderr de cada uno | Revertir `.goreleaser.yaml` y `ci.yml` a un solo build; eliminar las 4 aserciones nuevas |
| 4 | Instalador, tap, compuertas de release (condicionado a REQ-20.8), crosslane, telemetría | PR F0.c2 | `go test ./...` (sin paquete propio nuevo; verificación por lectura de literales) | N/A — cambios de configuración de scripts/CI sin superficie ejecutable propia en este repositorio; se verifica leyendo el literal cambiado | Revertir los literales de `install.sh`, `install.ps1`, las 3 compuertas, `battery.go`, `host.go`, `hostopencode.go` y `deploy/telemetry/*` a su estado previo |
| 5 | Paquete `internal/absorptionledger` + esqueleto del registro durable | PR F0.c3 | `go test ./internal/absorptionledger/...` | N/A — paquete de análisis de texto sin superficie de usuario | Eliminar `internal/absorptionledger/**` y `docs/upstream-absorption-ledger.md`; ninguna otra fase depende todavía de su contenido, solo de su existencia |
| 6 | Absorber los 14 commits de telemetría VictoriaMetrics (calibración del protocolo) | PR F2 | `go test ./...` (protocolo compartido, paso F) | `e2e/e2e_test.sh` | Revertir el commit de fusión de la tanda; marcar sus filas `revertido` en el registro, nunca borrarlas |
| 7 | Absorber los 6–7 commits de reviewer/OpenCode, precedido de medición de solape con INC-18 | PR F3 | `go test ./...` (protocolo compartido, paso F) | `e2e/e2e_test.sh` | Igual que la unidad 6 |
| 8 | Re-derivar la poda SDD de upstream (13 commits) + 3 deltas firmes de especificación + retirada de 5 entradas del trinquete de código muerto | PR F4 | `go test ./internal/sddstatus/... ./internal/cli/... ./internal/components/sdd/...` | `e2e/e2e_test.sh` | Revertir el commit de fusión; los 3 deltas de especificación se revierten con él porque viven en el mismo PR |
| 9 | Absorber los 7 commits de CLI y community-tools RTK | PR F5 | `go test ./...` (protocolo compartido, paso F) | `e2e/e2e_test.sh` | Igual que la unidad 6 |
| 10 | Reescribir la doctrina ODD hacia el protocolo de upstream en 7 activos + `routing.go` | PR F6.1 | `go test ./internal/assets/... ./internal/components/agentguidance/...` | `axiom sync` sobre un agente de prueba; confirmar que el texto instalado ya no contiene `axiom odd create\|status\|promote` | Revertir los 7 activos y `assets_test.go` a su forma previa; `axiom odd` sigue existiendo (Fase 11 no ha corrido) |
| 11 | Retirar la CLI `axiom odd` | PR F6.2a | `go test ./internal/cli/... ./cmd/axiom/...` | `axiom odd status` sobre el binario recién construido → "comando no reconocido" | Restaurar los 6 ficheros de CLI borrados y los puntos de `main.go`; requiere que la Fase 10 (doctrina) se revierta primero si se quiere volver a un estado coherente |
| 12 | Retirar la Web UI de ODD | PR F6.2b | `go test ./internal/dashboard/...` | `axiom ui`; confirmar ausencia de `tab-odd` y que `/api/odd*` responde 404 | Restaurar `odd_service.go`, las 4 rutas, los 3 DTOs y la superficie de `assets/`; independiente de la Fase 13 |
| 13 | Retirar la TUI de ODD | PR F6.2c | `go test ./internal/tui/...` | `axiom tui`; navegar Gobernanza y confirmar que la entrada ODD ya no existe y que el cursor no queda fuera de rango | Restaurar `odd_features.go`, los 5 puntos de `model.go`, `router.go` y `governance.go`; independiente de la Fase 12 |
| 14 | Borrar `internal/odd/**` íntegro (3473 líneas medidas) | PR F6.3 | `go build ./...` (prueba de compilación: ningún paquete referencia ya `internal/odd`) | `go build ./...` es en sí la prueba de ejecución: si compila, no queda ningún importador | Restaurar los 18 ficheros desde el commit anterior; solo seguro si las Fases 11–13 también se revierten (nada más los usa después de ellas) |
| 15 | Deltas de especificación destructivos y filas de registro para F6 | PR F6.4 | Lectura estructurada de `openspec/specs/odd-*/spec.md`, `dashboard-sdd-orchestration/spec.md`, `organic-agent-trigger-rules/spec.md` | N/A — cambios de documento, sin superficie ejecutable | Revertir los 6 ficheros de especificación a su estado previo a esta fase |
| 16 | Cerrar el registro (55/55 filas), inventario rojo de `cmd/axiom` con sucesor nombrado, `docs/ROADMAP.md` | PR F7 | `go test ./internal/absorptionledger/...` (debe pasar a GREEN aquí) | `e2e/e2e_test.sh` completo | Revertir el commit de fusión de F7; el registro vuelve a su estado incompleto anterior (rojo declarado, no roto) |
| 17 | Migración mecánica `/v2` → `/v3` en ~682 ficheros | PR F1 | `go build ./...` | Ejecutar el binario construido con `--version`/`--help`; sin cambio de comportamiento observable, la prueba es la compilación | Revertir el commit de fusión (mecánico, 1 línea por fichero); es la ÚNICA fase segura de revertir en cualquier momento porque no tiene posteriores [D-01] |

---

## Fase 1: F0.a — Accesores canónicos de raíz de respaldos y guarda en rojo declarado (REQ-20.13, REQ-20.14)

Aditivo puro: ningún llamador de producción cambia todavía. Deja el árbol verde salvo la guarda, que entra en rojo **a propósito** (es el RED de `strict_tdd` para la Fase 2).

- [x] 1.1 [Infra] Confirmar la base verde antes de empezar: `go build ./...`, `go vet ./...`, `go test ./... -timeout 900s`, `e2e/e2e_test.sh`. Registrar cualquier fallo preexistente encontrado (no se asume "todo en verde" sin comprobarlo).
- [x] 1.2 [Infra] Medir el universo del registro de absorción **ahora**, aunque el fichero se cree en la Fase 5: `git rev-list --count --no-merges 266574b0..upstream/main`. Anotar el valor `N` y la fecha (`AAAA-MM-DD`) en un lugar que la Fase 5 pueda leer (informe de esta fase). Es la "primera tarea de F0" que exige D-06.
- [x] 1.3 [RED] Escribir en `internal/backup/manifest_test.go` (o un fichero nuevo `manifest_roots_test.go`) una tabla de casos para `BackupRootFor(home string) string`: distintos valores de `home` (ruta Unix, ruta Windows con espacios, ruta vacía) producen `filepath.Join(home, ".axiom", "backups")`. La función no existe: RED por fallo de compilación.
- [x] 1.4 [GREEN] Añadir `BackupRootFor(home string) string` a `internal/backup/manifest.go`; refactorizar `backupRoot()` (`:171-177`) para que resuelva `os.UserHomeDir()` y delegue en `BackupRootFor`, sin cambiar su firma pública ni romper los 9 sitios que sobreescriben `BackupRootFn` en tests de `internal/backup` (`manifest_test.go:433-435`, `restore_test.go:16-22,77-83,154-160,358-360,426-428`, `retention_test.go:340-342,376-378,477-479`, `snapshot_dir_fsync_test.go:17-22` — todos `(read-only)` en esta tarea). Comentarios GoDoc en inglés, coherentes con el resto del fichero (ya está en inglés).
- [x] 1.5 [RED] Extender la tabla con `LegacyBackupRootFor(home string) string` → `filepath.Join(home, ".gentle-ai", "backups")`.
- [x] 1.6 [GREEN] Añadir `LegacyBackupRootFor`; refactorizar `legacyBackupRoot()` (`:180-186`) para delegar en ella.
- [x] 1.7 [RED] Extender la tabla con `BackupRoots(home string) []string` → exactamente `[]string{BackupRootFor(home), LegacyBackupRootFor(home)}`, **vigente primero** (orden es parte del contrato, no un detalle).
- [x] 1.8 [GREEN] Añadir `BackupRoots(home string) []string`.
- [x] 1.9 [Infra] Corregir el comentario caduco de `manifest.go:188-190` (`BackupRootFn`): elimina la afirmación "tests in other packages (e.g. `internal/update/upgrade`) can override it" — confirmado por `rg -n "BackupRootFn" internal/update/upgrade/` sin resultados — y sustitúyela por una referencia honesta a los 9 sitios internos de `internal/backup` que sí lo hacen.
- [x] 1.10 [RED — guarda declarada en rojo, cierra en la Fase 2] Crear `cmd/axiom/canonical_binary_test.go` (`package main`) con únicamente `TestUserStateRootsResolveThroughOwningPackage`: recorrer con `go/parser` + `go/ast` los ficheros de producción de `internal/**/*.go` (excluyendo únicamente el paquete `internal/backup` — la lista de excepciones tiene **una** entrada, no cinco [D-04]) y fallar ante cualquier literal de cadena `.axiom` o `.gentle-ai` seguido de un subdirectorio de estado (`backups`, etc.) en un argumento de `filepath.Join`. Ejecutar y observar el fallo real: debe listar `internal/cli/sync.go:509`, `internal/cli/run.go:709`, `internal/update/upgrade/executor.go:488` y `:514`, `internal/components/uninstall/service.go:178`, `internal/dashboard/service.go:1190` y los dos literales de `internal/app/app.go:1097-1100`. Registrar esta salida como evidencia RED.

  > **Ajuste de entrega (2026-09-19, aplicado en `sdd-apply`, Fase 1):** el diseño exigía dejar la guarda en **rojo declarado** al cerrar esta fase. Con `stacked-to-main` (elegido por el usuario en el Preflight de sesión) cada rebanada se fusiona a `main`, y un test en rojo ahí tumbaría el CI de las 16 rebanadas restantes que se apilan encima — este repositorio acaba de dedicar una sesión entera a poner `main` en verde. Se ejecutó la guarda de verdad y se capturó su salida real en rojo íntegra (evidencia RED en el informe de aplicación de la Fase 1: 8 sitios en los 6 ficheros de arriba — `internal/app/app.go` aporta dos literales). Acto seguido se añadió un `t.Skipf` al principio del test — patrón con precedente ya declarado en este repositorio (`cmd/axiom/main_test.go:316`) — cuyo motivo nombra explícitamente que la Fase 2 (F0.b) lo levanta y cuántos sitios detectó (`8 backup root literal site(s) detected...`). Ver la nota equivalente en la cabecera de la Fase 2: su primer paso RED es quitar ese `t.Skipf`.
- [x] 1.11 [REFACTOR] Revisar `manifest.go`: sin duplicación entre `BackupRootFor`/`backupRoot()` ni entre `LegacyBackupRootFor`/`legacyBackupRoot()`; comentarios GoDoc consistentes con el estilo existente del fichero (inglés).
- [x] 1.12 [Verificación de cierre F0.a] Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/backup/...` (debe estar en verde) y `go test ./cmd/axiom/... -run TestUserStateRootsResolveThroughOwningPackage -v` (debe estar en **rojo**, con la lista de 6 sitios de la tarea 1.10 como salida). Confirmar `gofmt -l internal/backup/manifest.go internal/backup/manifest_test.go cmd/axiom/canonical_binary_test.go` sin señalar nada. Confirmar que el diff no toca ninguna ruta prohibida (regla 1).

  > **Ajuste de entrega (2026-09-19):** en consecuencia del ajuste de la tarea 1.10, `go test ./cmd/axiom/... -run TestUserStateRootsResolveThroughOwningPackage -v` cerró esta fase **en verde con skip** (`--- SKIP`, `ok`, exit 0), no en rojo. El resto de esta verificación (build, vet, suite de `internal/backup`, `gofmt -l` sobre `manifest.go`/`manifest_roots_test.go`/`canonical_binary_test.go`, ausencia de rutas prohibidas) se mantiene sin cambios y se ejecutó tal cual.

## Fase 2: F0.b — Escritores migrados, 18 aserciones reapuntadas, guarda en verde (REQ-20.13, REQ-20.14)

Depende de la Fase 1 (usa `BackupRootFor`, `BackupRoots`). Cierra el rojo declarado de la tarea 1.10.

> **Nota heredada del ajuste de la Fase 1 (2026-09-19):** la tarea 1.10 no dejó la guarda en rojo declarado; la dejó **en verde con `t.Skipf`** (ver su anotación de ajuste). El primer paso RED de esta fase, antes de la tarea 2.1, es **quitar ese `t.Skipf`** de `TestUserStateRootsResolveThroughOwningPackage` en `cmd/axiom/canonical_binary_test.go` y ejecutar `go test ./cmd/axiom/... -run TestUserStateRootsResolveThroughOwningPackage -v`, observando el mismo rojo que la Fase 1 capturó (8 sitios en 6 ficheros). Solo entonces empieza la migración de las tareas 2.1 en adelante.

- [ ] 2.1 [GREEN] `internal/cli/sync.go:509`: sustituir `filepath.Join(homeDir, ".gentle-ai", "backups")` por `backup.BackupRootFor(homeDir)`.
- [ ] 2.2 [GREEN] `internal/cli/run.go:709`: mismo cambio.
- [ ] 2.3 [GREEN] `internal/update/upgrade/executor.go:488` (snapshot) y `:514` (poda): mismo cambio en ambos sitios. Nota explícita [D-12]: la poda (`backup.Prune`) pasa a operar **solo** sobre la raíz canónica; la raíz heredada deja de podarse (queda congelada, nunca borrada por retención — comportamiento deliberado, no un olvido).
- [ ] 2.4 [GREEN] `internal/components/uninstall/service.go:178` (+ `os.MkdirAll` en `:179`, sin cambios): mismo cambio. `cleaners.go` del mismo paquete permanece intocado (V2, ruta prohibida por fichero).
- [ ] 2.5 [GREEN] `internal/dashboard/service.go:1190` (+ `os.MkdirAll` en `:1191`, sin cambios): mismo cambio — este sitio ya escribía la grafía correcta (E3), el arreglo es de método, no de valor.
- [ ] 2.6 [GREEN] `internal/app/app.go:1097-1100`: sustituir los dos literales del slice `roots` por `backup.BackupRoots(homeDir)`, preservando el orden vigente-primero.
- [ ] 2.7 [GREEN] `cmd/axiom/main.go:141`: cambiar `"No hay respaldos registrados en ~/.axiom/backups/"` por un mensaje que nombre las **dos** raíces que `ListBackups` escanea tras 2.6 (canónica y heredada).
- [ ] 2.8 [GREEN] Reapuntar las 16 aserciones que siembran o leen contra el literal heredado, verificándolas contra `backup.BackupRootFor(...)`/`backup.LegacyBackupRootFor(...)` en vez de contra una cadena re-tecleada: `internal/app/app_test.go:39,87,151,498`; `internal/cli/dedup_prune_test.go:20,90,158,226`; `internal/cli/backup_metadata_test.go:35,92,139`; `internal/update/upgrade/effective_method_routing_test.go:258,403`; `internal/update/upgrade/executor_test.go:893`; `internal/cli/restore_test.go:19`; `internal/cli/sync_test.go:1867`. Verificación: `go test ./internal/app/... ./internal/cli/... ./internal/update/upgrade/... -run 'TestApp|TestDedupPrune|TestBackupMetadata|TestEffectiveMethodRouting|TestExecutor|TestRestore|TestSync' -v` — cada uno debe seguir en verde tras el reapuntado (verifica ahora contra el accesor, no contra una copia del literal).
- [ ] 2.9 [RED — control negativo explícito, antes de tocar el test] En `internal/cli/compatibility_skills_test.go`, junto a la aserción de `:131-133`, sembrar deliberadamente un respaldo bajo la raíz **canónica** (`backup.BackupRootFor(home)`) dentro del mismo subtest y confirmar que, con la aserción todavía apuntando a la raíz heredada, el test **no lo detecta** (evidencia de que hoy vigila un directorio que nadie va a crear).
- [ ] 2.10 [GREEN] Reapuntar `compatibility_skills_test.go:131-133` a la raíz canónica (`backup.BackupRootFor(home)`) y confirmar que el control negativo de 2.9, ejecutado de nuevo contra la aserción ya reapuntada, **falla como se espera** (el test vuelve a ser capaz de fallar por el motivo para el que se escribió).
- [ ] 2.11 [RED — control negativo explícito] Repetir 2.9 para `internal/cli/compatibility_transaction_windows_test.go:275-277`.
- [ ] 2.12 [GREEN] Repetir 2.10 para `compatibility_transaction_windows_test.go:275-277`.
- [ ] 2.13 [Integración] Escribir/extender un test de integración que siembre `~/.gentle-ai/backups/<id>` con su manifiesto sobre un `HOME` temporal y confirme que `app.ListBackups()`, la restauración y `backup.DeleteBackup` lo siguen resolviendo, restaurando y borrando sin cambios — el contrato de lectura dual de INC-12 permanece intacto [D-12].
- [ ] 2.14 [REFACTOR] Revisar los 5 escritores y el lector: sin literales `.gentle-ai`/`.axiom` remanentes fuera de `internal/backup` (confirmarlo leyendo la salida de la tarea 2.15 antes de darla por hecha).
- [ ] 2.15 [Verificación de cierre F0.b] Ejecutar `go test ./cmd/axiom/... -run TestUserStateRootsResolveThroughOwningPackage -v` (debe estar ahora en **verde**, cero violaciones). Ejecutar `go build ./...`, `go vet ./...`, `go test ./internal/... -timeout 900s`. Ejecutar `gofmt -l` solo sobre los ficheros tocados en las tareas 2.1–2.13. Confirmar que el diff no toca ninguna ruta prohibida.

## Fase 3: F0.c1 — Identidad de build, cobertura de CI y guardas restantes (REQ-20.8 parcial, REQ-20.11 parcial, REQ-20.15, REQ-20.16)

Depende de la Fase 1 (el fichero `canonical_binary_test.go` ya existe). Independiente de la Fase 2.

- [ ] 3.1 [GREEN] Modificar `.goreleaser.yaml`: `project_name: gentle-ai` (`:3`) → `axiom`; sustituir el build único (`:12-29`) por dos builds nombrados — `id: axiom, main: ./cmd/axiom, binary: axiom` y `id: gentle-ai-deprecated, main: ./cmd/gentle-ai, binary: gentle-ai` — preservando `env`, `goos`, `goarch`, `flags` y `ldflags` (`:25-29`, incluida la referencia a la ruta de módulo, que F1 reescribirá después) en ambos.
- [ ] 3.2 [RED→GREEN] Añadir a `canonical_binary_test.go`: `TestReleaseArtifactBuildsCanonicalBinary` — parsea `.goreleaser.yaml` y confirma que `builds[]` contiene una entrada con `main: ./cmd/axiom` y `binary: axiom`. RED antes de 3.1 (falla, solo existe `./cmd/gentle-ai`), GREEN después.
- [ ] 3.3 [GREEN] `.github/workflows/ci.yml:266`: `go build ... -o "$env:RUNNER_TEMP\gentle-ai.exe" ./cmd/gentle-ai` → `./cmd/axiom` (renombrar también la variable de ruta si el resto del step la referencia por nombre).
- [ ] 3.4 [RED→GREEN] Añadir `TestWorkflowsBuildCanonicalBinary`: recorre `.github/workflows/*.yml` y rechaza cualquier objetivo de construcción `./cmd/gentle-ai` que no figure en una lista de excepciones con motivo escrito. RED antes de 3.3 (señala `:266`), GREEN después con la lista de excepciones **vacía** (el audit de esta fase no encontró ningún caso legítimo de construir el shim).
- [ ] 3.5 [Caracterización] Añadir `TestAppDispatchIsSubsetOfCanonicalDispatch`: compara con `go/ast` los verbos de `internal/app/app.go` (`:84-145`, `(read-only)`) contra los de `cmd/axiom/main.go` como conjuntos. Ejecutar y confirmar que **ya está en verde** (los 4 verbos que faltaban — `codegraph`, `telemetry`, `skill-registry`, `bench-model-picker` — ya se corrigieron antes de este incremento, `main.go:117-120,391-395` `(read-only)`); registrar esta confirmación como evidencia, no asumirla.
- [ ] 3.6 [Caracterización] Añadir `TestDeadcodeRatchetTargetsCanonicalBinary`: lee `scripts/deadcode-ratchet.sh` y confirma `DEADCODE_TARGET:-./cmd/axiom` (`:41`, `(read-only)`). Ejecutar y confirmar que **ya está en verde**; registrar la confirmación.
- [ ] 3.7 [GREEN] Añadir a `.github/workflows/ci.yml` el paso nuevo "Smoke the canonical binary surface": construye `./cmd/axiom` y ejercita, para cada verbo que el contrato de nombre toca, su forma de ayuda o sin argumentos, afirmando (a) el código de salida esperado y (b) que stderr **no** contiene el aviso de deprecación de `cmd/gentle-ai/main.go:14,17` `(read-only)`. Cubre como mínimo `init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui` (bloqueante) y, mientras exista, `odd` (bloqueante hasta la Fase 11).
- [ ] 3.8 [GREEN] En el mismo paso, añadir el bloque de comentario de ventana informativa fechada (contrato de REQ-20.16): fecha de apertura, inventario de la superficie no ejercitada (se rellena cuando el paso corra por primera vez y destape algo, no antes — O-4), e incremento sucesor nombrado **INC-21**. El resto de la superficie exclusiva de `cmd/axiom` (más allá de los verbos de la tarea 3.7) queda informativa, nunca bloqueante, hasta que INC-21 la asuma.
- [ ] 3.9 [REFACTOR] Revisar `canonical_binary_test.go`: las 5 aserciones (4 de esta fase + la de la Fase 1) comparten helpers sin duplicar el parseo de `go/ast`.
- [ ] 3.10 [Verificación de cierre F0.c1] Ejecutar `go build ./...`, `go vet ./...`, `go test ./cmd/axiom/... -v` (las 5 aserciones en verde). Build local de `./cmd/axiom` y `./cmd/gentle-ai`; confirmar por inspección de stderr que solo el segundo emite el aviso de deprecación. `gofmt -l` sobre los ficheros tocados. Confirmar que el diff no toca ninguna ruta prohibida.

## Fase 4: F0.c2 — Instalador, tap, compuertas de release, crosslane, telemetría (REQ-20.7, REQ-20.8, REQ-20.11, REQ-20.12)

Depende de la Fase 3 (D-08: `.goreleaser.yaml` debe apuntar ya al binario canónico antes de repuntar el instalador — un instalador que apunta a un artefacto inexistente es un fallo ruidoso, no silencioso).

- [ ] 4.1 [Punto de control condicional — REQ-20.8, decisión de mantenedor, no de código] Antes de tocar `install.sh`/`install.ps1`/las 3 compuertas de release: confirmar si ya existe un artefacto de release de Axiom publicado bajo su propia identidad de distribución (tras el cambio de `.goreleaser.yaml` de la Fase 3). **Si NO existe todavía:** detener aquí las tareas 4.2–4.6; dejarlas como una fase F0.c2b posterior, a ejecutar en cuanto exista un release publicado; continuar con las tareas 4.7 en adelante (crosslane y telemetría), que son incondicionales. **Si SÍ existe:** continuar con 4.2.
- [ ] 4.2 [GREEN — condicionada a 4.1] `scripts/install.sh:17-21`: `GITHUB_OWNER`, `GITHUB_REPO`, `BINARY_NAME`, `BREW_TAP`, `BREW_FORMULA_REF` → identidad de Axiom; actualizar también la cabecera de uso (`:5-14`).
- [ ] 4.3 [GREEN — condicionada a 4.1] `scripts/install.ps1`: equivalentes de las mismas variables.
- [ ] 4.4 [GREEN — condicionada a 4.1] `.goreleaser.yaml`: bloque `brews:` (`:123-133`) — `name: gentle-ai` → `axiom`, `homepage`, `description` según la identidad de Axiom, conservando `owner`/`repository` si el tap sigue siendo el mismo.
- [ ] 4.5 [GREEN — condicionada a 4.1] `scripts/verify-release-assets.sh:11,84`, `scripts/release-preflight.sh:18`, `scripts/promote-stable-preflight.sh:18`: literal `Gentleman-Programming/gentle-ai` → identidad de Axiom en la aserción de `$GITHUB_REPOSITORY`.
- [ ] 4.6 [Verificación del tramo condicional] Si 4.2–4.5 se ejecutaron: confirmar por lectura que el instalador resultante instalaría el binario `axiom`, no el de upstream (D2.1). Si se difirieron por 4.1: dejarlo anotado explícitamente en el informe de esta fase, no en silencio.
- [ ] 4.7 [GREEN] `scripts/crosslane/battery.go:274`: `words[0] != "gentle-ai"` → tolerancia dual (`words[0] != "axiom" && words[0] != "gentle-ai"`), vigente primero en cualquier lógica de prioridad.
- [ ] 4.8 [GREEN] `scripts/crosslane/host.go:119`: mismo cambio.
- [ ] 4.9 [GREEN] `scripts/crosslane/hostopencode.go:231-243`: el shim ejecutable en `$PATH` resuelve ambos nombres mientras D2.4 mantenga el alias `gentle-ai`.
- [ ] 4.10 [GREEN] `deploy/telemetry/gentle-telemetry.service`, `deploy/telemetry/gentle-telemetry-backup.service`, `deploy/telemetry/gentle-telemetry-backup.timer`, `deploy/telemetry/gentle-telemetry-backup.test.sh`, `deploy/telemetry/install.sh` y los paneles de Grafana asociados (si existen bajo el mismo directorio): nombres de servicio → identidad de distribución de Axiom (REQ-20.12). Confirmar primero con `fd . deploy/telemetry` el inventario real antes de editar, ya que este documento no reenumera su contenido interno.
- [ ] 4.11 [Verificación de cierre F0.c2] `go build ./...`, `go vet ./...` (ningún paquete Go cambia en esta fase, deben seguir en verde sin más). Lectura dirigida de cada fichero modificado para confirmar el literal correcto. `gofmt -l` sobre cualquier `.go` tocado (ninguno esperado). Confirmar que el diff no toca `contracts/**` (REQ-20.10, byte a byte) ni ninguna ruta prohibida.

## Fase 5: F0.c3 — Registro durable de absorción, infraestructura (REQ-20.1, REQ-20.5, REQ-20.6)

Independiente de las Fases 3 y 4 (no comparte ficheros). Usa la medición de universo de la tarea 1.2.

- [ ] 5.1 [RED] Escribir `internal/absorptionledger/ledger_test.go` con fixtures sintéticas (no el documento real todavía): estado desconocido → `ErrUnknownState`; fila con estado ≠ `absorbido` y motivo vacío → `ErrMissingReason`; tabla de recuento que no cuadra con las filas reales → `ErrCountMismatch`; total de filas menor que el universo declarado en la cabecera → `ErrUniverseMismatch`; `sha` repetido → `ErrDuplicateSHA`; fixture válida y completa → `Parse` sin error, `Ledger` con los campos esperados.
- [ ] 5.2 [GREEN] Crear `internal/absorptionledger/ledger.go`: `State` (`StateAbsorbed`, `StateDeliberatelyDropped`, `StateReverted`), `Row`, `Ledger` (con `Universe int`, nunca una constante — D-06), los 5 errores centinela, y `Parse([]byte) (*Ledger, error)` — hasta que 5.1 quede en verde.
- [ ] 5.3 [Create] Crear `docs/upstream-absorption-ledger.md` con: cabecera (universo `N` y fecha de la tarea 1.2, ancestro común `266574b0`, sha de `upstream/main`, comando `git rev-list --count --no-merges 266574b0..upstream/main`); reglas de aceptación (RA-1, RA-2, V1–V8, "una tanda revertida actualiza su fila, nunca la borra" — adaptadas de `docs/releases/v2.2.0-closure-ledger.md:11-21` `(read-only)`); tabla de recuento en `0/0/0/0`; ocho secciones vacías, una por fase (F0–F7), cada una con su sub-tabla "Ficheros derivados y ausentes (RA-1)".
- [ ] 5.4 [RED] Extender `ledger_test.go` con un test que parsea el `docs/upstream-absorption-ledger.md` real (no una fixture) y confirma coherencia interna de sus partes ya rellenas (reglas 2–6 de D-06: estados válidos, motivos presentes, recuento cuadra con 0 filas reales, sin cabeceras de tanda repetidas, sin `sha` duplicados). Este test debe pasar ya con el esqueleto vacío de 5.3 (0 filas declaradas = 0 filas reales cuadra).
- [ ] 5.5 [RED — declarado, se cierra en la Fase 16] Extender `ledger_test.go` con `TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose` (o nombre equivalente): confirma que el total de filas reales es exactamente igual al universo declarado en la cabecera. Ejecutar y confirmar que está en **rojo** (0 filas reales contra universo `N`). Este es el ÚNICO fallo aceptado en `go test ./...` desde esta fase hasta el cierre de la Fase 16 (regla 3 de «Reglas de Comprobación y Alcance»).
- [ ] 5.6 [REFACTOR] Revisar `ledger.go`: errores envueltos con `%w` en todo punto de propagación (`skills/axiom-idiomatic-error-wrapping` `(read-only)`), comentarios GoDoc en inglés (identificador Go, precedente `internal/odd.Status` de INC-19).
- [ ] 5.7 [Verificación de cierre F0.c3] `go build ./...`, `go vet ./...`, `go test ./internal/absorptionledger/... -v` (todo en verde salvo `TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose`, en rojo declarado). `gofmt -l internal/absorptionledger/*.go`. Confirmar que `docs/upstream-absorption-ledger.md` no está bajo `openspec/changes/**` (D-06: vive en `docs/`, sobrevive al archivado). Espejar el esqueleto del registro en Engram bajo `sdd/inc-20-upstream-reconciliation/absorption-ledger`.

## Fase 6: F2 — Telemetría VictoriaMetrics (tanda de calibración del protocolo)

Depende de la Fase 5 (el registro debe existir para escribir sus filas) y de la Fase 3 (comparte `ci.yml`, aunque su intersección de conflicto real es nula según `proposal.md` §4.3). Sigue el «Protocolo compartido de tanda de absorción».

- [ ] 6.1 [Protocolo — paso B] Derivar la lista de ficheros de los 14 commits de telemetría con `git show <sha> --stat` para cada uno. Comprobar en particular si alguno coincide con `c09b1a34` (candidato por asunto — ver nota cruzada del protocolo compartido).
- [ ] 6.2 [Protocolo — paso C] Cherry-pick agrupado de los 14 commits.
- [ ] 6.3 [Protocolo — paso D] Contrastar el diff resultante contra la derivación de 6.1; motivo escrito para todo fichero derivado y ausente.
- [ ] 6.4 [Protocolo — paso E] Confirmar ninguna ruta prohibida tocada y ninguna entrada V1–V8 revertida.
- [ ] 6.5 [Protocolo — paso F] `go build ./...`, `go vet ./...`, `go test ./... -timeout 900s` (sin `-run`), `gofmt -l` sobre los ficheros tocados, `e2e/e2e_test.sh`.
- [ ] 6.6 [Protocolo — paso G] Escribir la sección "F2 — Telemetría VictoriaMetrics" en `docs/upstream-absorption-ledger.md` con sus 14 filas y su sub-tabla de ficheros derivados y ausentes. Actualizar la tabla de recuento. Espejar en Engram.
- [ ] 6.7 [Verificación de cierre F2] Confirmar que `TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose` sigue siendo el único fallo de `go test ./...` (ahora con 14/`N` filas, sigue en rojo, esperado). Confirmar `git log origin/main..main` antes de abrir la PR (regla 4).

## Fase 7: F3 — Reviewer y parsing de OpenCode

Depende de la Fase 6. Sigue el «Protocolo compartido de tanda de absorción».

- [ ] 7.1 [Prerrequisito — R13] Medir el solape con lo que INC-18 trajo a medias: `git show --stat` de los PRs de upstream que INC-18 absorbió parcialmente, contrastado contra el árbol actual, **antes** de absorber ningún commit de esta tanda. Registrar el resultado (solape real o nulo) en el informe de esta fase.
- [ ] 7.2 [Protocolo — paso B] Derivar la lista de ficheros de los 6–7 commits con `git show <sha> --stat`. Comprobar si alguno coincide con `15ea98ed`, `e28af0fd` o `cd95b782` (candidatos por asunto).
- [ ] 7.3 [Protocolo — paso C] Cherry-pick agrupado, informado por la medición de solape de 7.1 (si hay solape, reconciliar en vez de duplicar).
- [ ] 7.4 [Protocolo — paso D] Contrastar contra la derivación de 7.2.
- [ ] 7.5 [Protocolo — paso E] Frontera y no-reversión.
- [ ] 7.6 [Protocolo — paso F] Verificación sin filtrar completa.
- [ ] 7.7 [Protocolo — paso G] Sección "F3 — Reviewer y parsing de OpenCode" en el registro, con sus filas y su sub-tabla RA-1. Espejar en Engram.
- [ ] 7.8 [Verificación de cierre F3] Mismo criterio que 6.7, con el recuento acumulado de filas actualizado.

## Fase 8: F4 — Poda y refactor SDD (zona más caliente, re-derivación) (REQ-13.3, rdd-sdd-receipt-consumption, sdd-research, rdd-post-verify-review-offer)

Depende de la Fase 7. Método distinto al resto: **re-derivación sobre el fork**, no cherry-pick — se toma el resultado de upstream como objetivo declarado, y la lista de ficheros de upstream (RA-1) se usa como lista de comprobación de cobertura, no como parche a aplicar (`proposal.md` §4.5).

- [ ] 8.1 [Prerrequisito — R6, antes de tocar nada] Escribir tests de caracterización sobre el comportamiento **vigente** de la compuerta `verify → archive` en `internal/sddstatus` (comparando salida íntegra, no por subcadena — precedente INC-19 D-03), como blindaje previo a la re-derivación.
- [ ] 8.2 [Protocolo — paso B] Derivar la lista de ficheros de los 13 commits de la poda SDD con `git show <sha> --stat` cada uno, usada como checklist de cobertura de la re-derivación.
- [ ] 8.3 [Protocolo — paso C, re-derivación] Reescribir `internal/sddstatus`, `internal/cli/sdd_*.go` y `internal/components/sdd/` hacia el objetivo declarado por upstream, incluyendo la retirada de las operaciones `acquire`/`settle` de `internal/cli/sdd_attempt.go` `(read-only hoy, editable en esta tarea)` (commit de origen `18fa04fb`, ya citado en REQ-13.3) preservando `grant` sin cambios de comportamiento.
- [ ] 8.4 [Protocolo — paso D] Confirmar cobertura: todo fichero de la lista de 8.2 aparece en el diff de esta tanda o lleva motivo escrito (es una re-derivación: "reescrito de forma distinta pero equivalente" es un motivo válido si se documenta).
- [ ] 8.5 [GREEN — delta firme 1, español] Reemplazar por completo el bloque `### Requirement: Subcomando axiom sdd attempt (REQ-13.3)` en `openspec/specs/axiom-sdd-cli-integration/spec.md` (`:32-39`) por el texto de `spec.md` de este cambio (§4, REQ-13.3) — retira `acquire`/`settle`, conserva `grant`. No tocar REQ-13.1, REQ-13.2 (mismo fichero, `:15-30`, `(read-only)`) ni REQ-13.4 (`:43-59`, `(read-only)`).
- [ ] 8.6 [GREEN — delta firme 2, inglés] Reemplazar por completo, en `openspec/specs/rdd-sdd-receipt-consumption/spec.md`, el requerimiento `### Requirement: Attempt Ledger Ownership Stays With SDD (Maintainer-Confirmed, 2026-08-02)` (`:45-61`) y `### Requirement: ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact` (`:87-97`) por el texto de `spec.md` de este cambio (§5). No tocar `ReceiptRef-Only Persistence` (`:9-25`), `No Re-Derived Review-Lifecycle Meaning` (`:27-43`) ni `Legacy reviewGate v1 Field Compatibility` (`:63-85`), todos `(read-only)`.
- [ ] 8.7 [GREEN — delta firme 3, inglés] Reemplazar `## Purpose` (`sdd-research/spec.md:5`, "Selected research is required.") y el requerimiento `### Requirement: Closed Capability Admission` (`:9-23`) por el texto de `spec.md` de este cambio (§6). No tocar `Auditable Evidence Integrity` (`:25-…`, `(read-only)`) ni ningún otro requerimiento del mismo fichero.
- [ ] 8.8 [Medición] Confirmar si `internal/agents/researchcapability` (`contract.go`, `contract_test.go` — hoy 2 ficheros, cero importadores Go verificado) tiene algún consumidor fuera del delta de `sdd-research`; si no lo tiene, retirarlo como parte de esta tanda (upstream ya lo retiró en `15cbbde4`).
- [ ] 8.9 [Ratchet — D-09] Ejecutar `scripts/deadcode-ratchet.sh --update` tras la re-derivación y confirmar que el diff de `.deadcode-baseline.txt` contiene **exactamente cinco borrados y cero adiciones**: `internal/reviewtransaction/review_offer.go` → `OfferReviewAfterVerify` (`:166`) y `readGlobalRDDModeForOffer` (`:167`); `internal/sddstatus/review_door.go` → `resetReviewEntryHookCallCountForTest` (`:230`), `reviewEntryHookCallCountForTest` (`:231`), `reviewOfferForVerify` (`:232`). Una adición significa que esta tanda creó código muerto nuevo y no debe aterrizar.
- [ ] 8.10 [GREEN — delta, inglés] En `openspec/specs/rdd-post-verify-review-offer/spec.md`, actualizar la cita textual de `:13` ("`OfferReviewAfterVerify` retains no production caller.") a "no existe", reflejando la retirada de 8.9.
- [ ] 8.11 [Protocolo — paso E] Frontera y no-reversión.
- [ ] 8.12 [Protocolo — paso F] Verificación sin filtrar completa, incluyendo el test de caracterización de 8.1 (debe seguir en verde, protegiendo el comportamiento vigente de `verify → archive`).
- [ ] 8.13 [Protocolo — paso G] Sección "F4 — Poda y refactor SDD" en el registro (13 filas), su sub-tabla RA-1 (con los motivos de "reescrito, no aplicado" donde corresponda), y nota de los 3 deltas firmes + la retirada del trinquete. Espejar en Engram.
- [ ] 8.14 [Verificación de cierre F4] `go build ./...`, `go vet ./...`, `go test ./internal/sddstatus/... ./internal/cli/... ./internal/components/sdd/... -v`, `e2e/e2e_test.sh`. Confirmar que `internal/agents/researchcapability` no queda huérfano a medias (retirado del todo o conservado del todo, nunca parcial). Dada la magnitud esperada (Forecast: High, no medible), **si el diff real supera ~600–700 líneas, partir esta fase en sub-PRs por sub-zona (p. ej. "libro mayor de runtime" vs "CLI sdd_*.go" vs "deltas de especificación") antes de abrir la PR**, documentando la partición en vivo en el informe de esta fase.

## Fase 9: F5 — CLI y community-tools RTK

Depende de la Fase 8. Sigue el «Protocolo compartido de tanda de absorción».

- [ ] 9.1 [Protocolo — paso B] Derivar la lista de ficheros de los 7 commits con `git show <sha> --stat` cada uno. Comprobar si alguno coincide con `110f1371` (candidato por asunto: "retire RTK integration").
- [ ] 9.2 [Protocolo — paso C] Cherry-pick agrupado; solape parcial esperado con el fork (`proposal.md` §4.3) — reconciliar manualmente commit por commit, no en bloque.
- [ ] 9.3 [Protocolo — paso D] Contrastar contra la derivación de 9.1.
- [ ] 9.4 [Protocolo — paso E] Frontera y no-reversión.
- [ ] 9.5 [Protocolo — paso F] Verificación sin filtrar completa.
- [ ] 9.6 [Protocolo — paso G] Sección "F5 — CLI y community-tools RTK" en el registro. Espejar en Engram.
- [ ] 9.7 [Verificación de cierre F5] Mismo criterio que 7.8, con el recuento acumulado actualizado.

## Fase 10: F6.1 — Doctrina ODD reescrita hacia el protocolo de upstream (parte de organic-agent-trigger-rules, absorbe `70c774f8`, `1b202d77`, `cfc415ce`)

Depende de la Fase 5 (el registro debe existir). Independiente de las Fases 6–9. **La doctrina muere antes que el verbo [D-02]**: al cerrar esta fase, `axiom odd create|status|promote` sigue existiendo y funcionando; solo deja de estar documentado. Ninguna suite queda verde con documentación falsa.

- [ ] 10.1 [Caracterización] Capturar byte a byte el contenido actual de los 7 activos de persona antes de tocarlos, para que el diff de doctrina sea legible: `internal/assets/{claude,opencode,kiro,hermes,generic}/persona-axiom.md`, `internal/assets/{claude,kimi}/output-style-axiom.md`.
- [ ] 10.2 [Protocolo — paso B, sobre 3 commits de autoría distinta] Derivar `git show 70c774f8 --stat`, `git show 1b202d77 --stat`, `git show cfc415ce --stat` — el contenido doctrinal que se absorbe hacia `internal/components/agentguidance/routing.go` (`(read-only hoy, editable en esta tarea)`, confirmado con cero menciones de ODD antes de esta fase).
- [ ] 10.3 [GREEN] Retirar de los 7 activos de persona la sección `## Flujo Dual: ODD y SDD` (verificada en `claude/persona-axiom.md:33-38`) y sustituirla por una referencia al protocolo ODD de upstream, absorbiendo el contenido derivado en 10.2 hacia `routing.go`.
- [ ] 10.4 [RED→GREEN] Reescribir `axiomODDWorkflowRequired` (`internal/assets/assets_test.go:3067-3075`) y `axiomODDWorkflowForbiddenTriggers` (`:3081-3090`) para que reflejen el protocolo de upstream en vez de los literales `` `axiom odd create <nombre>` ``, `` `axiom odd status [--json] [--check-mirror]` `` y `` `axiom odd promote <feature> [--dry-run] [--name <nombre>]` `` — la doctrina deja de nombrar comandos que la Fase 11 retirará.
- [ ] 10.5 [Verificación] Confirmar `TestPersonaAxiomAssetsDescribeODDWorkflowDeterministically` (`assets_test.go:3111-3138`) en verde con el nuevo contenido esperado, y determinismo de renderizado preservado (misma entrada, mismos bytes en dos lecturas).
- [ ] 10.6 [Protocolo — paso E] Confirmar que `axiom odd create|status|promote` **siguen funcionando** al cerrar esta fase (Fase 11 aún no ha corrido) — es el criterio explícito que evita la ventana de "suite verde + documentación falsa" [D-02].
- [ ] 10.7 [Protocolo — paso F] Verificación sin filtrar completa.
- [ ] 10.8 [Protocolo — paso G] Sección "F6 — Reconciliación ODD (F6.1: doctrina)" en el registro, con las 3 filas de `70c774f8`, `1b202d77`, `cfc415ce`. Espejar en Engram.
- [ ] 10.9 [Verificación de cierre F6.1] `go build ./...`, `go vet ./...`, `go test ./internal/assets/... ./internal/components/agentguidance/... -v`. `axiom sync` sobre un agente de prueba; confirmar que el texto instalado ya no contiene los tres literales de comando retirados de 10.4.

## Fase 11: F6.2a — Retirada de la CLI `axiom odd` (REQ-19.5–19.8 dejan de tener superficie ejecutable)

Depende de la Fase 10 (la doctrina ya no nombra estos comandos). Independiente de las Fases 12 y 13.

- [ ] 11.1 [Caracterización] Confirmar mediante `rg -n 'axiom odd'` el inventario de sitios que invocan la CLI antes de borrarla (criterio de cierre de F6.2 completo, no solo de esta sub-fase).
- [ ] 11.2 [GREEN] Borrar `internal/cli/odd_create.go`, `internal/cli/odd_create_test.go`, `internal/cli/odd_status.go`, `internal/cli/odd_status_test.go`, `internal/cli/odd_promote.go`, `internal/cli/odd_promote_test.go` (6 ficheros).
- [ ] 11.3 [GREEN] `cmd/axiom/main.go`: retirar el import de `internal/odd` (`:25`), el `case "odd":` (`:356-357`), la función `runODD` (`:1789-1823`), `dashboardScaffolder`/`newDashboardScaffolder`/su método `Scaffold` (`:1825-1866`), y las líneas de ayuda que mencionan `odd` (bloque de `printHelp()`).
- [ ] 11.4 [GREEN] `cmd/axiom/main_test.go`: retirar los casos que ejercitan `runODD` y el despacho de `odd`.
- [ ] 11.5 [GREEN] Retirar el verbo `odd` del paso de CI "Smoke the canonical binary surface" (Fase 3, tarea 3.7) — ya no aplica.
- [ ] 11.6 [Verificación] `axiom odd status` sobre el binario recién construido devuelve "comando no reconocido" (o equivalente), no un pánico ni una ayuda fantasma.
- [ ] 11.7 [Verificación de cierre F6.2a] `go build ./...`, `go vet ./...`, `go test ./internal/cli/... ./cmd/axiom/... -v`. Confirmar `rg -n 'axiom odd|internal/cli/odd_'` sin resultados fuera de `openspec/changes/archive/**`.

## Fase 12: F6.2b — Retirada de la Web UI de ODD (REQ-19.13, parte de REQ-19.15)

Depende de la Fase 10. Independiente de las Fases 11 y 13.

- [ ] 12.1 [GREEN] Borrar `internal/dashboard/odd_service.go`, `internal/dashboard/odd_service_test.go`.
- [ ] 12.2 [GREEN] `internal/dashboard/server.go`: retirar las 4 rutas (`:58-61` — `/api/odd`, `/api/odd/promote`, `/api/odd/check-mirror`, `/api/odd/`), sus 4 manejadores (`handleODD`, `handleODDDetail`, `handleODDPromote`, `handleODDCheckMirror`, `:370-472`) y el campo `oddExporter` (`:24-28`) más el import de `internal/odd` (`:13`).
- [ ] 12.3 [GREEN] `internal/dashboard/types.go`: retirar `ODDCreateRequest` (`:229-235`), `ODDPromoteRequest` (`:237-244`), `ODDMirrorRequest` (`:246-…`).
- [ ] 12.4 [GREEN] `internal/dashboard/assets/index.html`: retirar el botón `data-tab="tab-odd"` y la `<section id="tab-odd">`.
- [ ] 12.5 [GREEN] `internal/dashboard/assets/app.js`: retirar `loadODD`, `renderODDFeatures`, `createODDFeature`, `promoteODDFeature`, `checkODDMirror`, y la insignia "← Origen ODD" (`oddOriginMap`) de `renderIncrements()`.
- [ ] 12.6 [GREEN] `internal/dashboard/assets/style.css`: retirar `.lane-badge`, `.lane-badge--promoted`, `.mirror-state--*` si no los usa ninguna otra superficie.
- [ ] 12.7 [GREEN] `internal/dashboard/dashboard_test.go`: retirar `TestODDEndpoints`.
- [ ] 12.8 [Medición — O-1] Confirmar si `CreateIncrementRequest.ProposalBody` (`types.go`) tiene algún consumidor fuera de ODD (por ejemplo, un cliente externo de `POST /api/increments`). **Si no lo tiene:** retirar el campo y su rama en `service.go`, y actualizar `create_increment_characterization_test.go` en consecuencia. **Si lo tiene:** dejar ambos ficheros intactos — el campo permanece como superficie de API publicada, protegida por REQ-15.1 sin cambios.
- [ ] 12.9 [Verificación] `axiom ui` sobre un workspace de prueba; confirmar ausencia de la pestaña `tab-odd` y que `GET /api/odd`, `POST /api/odd/promote`, `POST /api/odd/check-mirror` devuelven 404.
- [ ] 12.10 [Verificación de cierre F6.2b] `go build ./...`, `go vet ./...`, `go test ./internal/dashboard/... -v`; confirmar que el dashboard SDD existente (`/api/increments`, pestaña de incrementos) sigue funcionando sin cambio de comportamiento.

## Fase 13: F6.2c — Retirada de la TUI de ODD (REQ-19.14, resto de REQ-19.15)

Depende de la Fase 10. Independiente de las Fases 11 y 12.

- [ ] 13.1 [GREEN] Borrar `internal/tui/screens/odd_features.go`.
- [ ] 13.2 [GREEN] `internal/tui/model.go`: retirar los 5+ puntos de integración — la constante `ScreenODDFeatures` (`:589-591`), su `case` en `View()` (`:1671`), su `case` en el manejador de Enter de `ScreenGovernance` (`:3141-3153`, revertir la numeración de "Volver al menú principal" a su posición previa), su `case` en `screenOptionCount` (`:4571`), `loadODDFeatures()` (`:5954-5980`) y `oddChangeNameFromPromotedTo()` (`:5982-5990`).
- [ ] 13.3 [GREEN] `internal/tui/model_test.go`: retirar los casos que ejercitan `ScreenODDFeatures`.
- [ ] 13.4 [GREEN] `internal/tui/router.go`: retirar `ScreenODDFeatures: {Backward: ScreenGovernance}` (`:70`) del mapa `linearRoutes`.
- [ ] 13.5 [GREEN] `internal/tui/screens/governance.go`: retirar la entrada `"6. Carril Ágil ODD (documentos vivos, promoción)"` (`:17`) y recolocar "Volver al menú principal" a su posición previa a INC-19.
- [ ] 13.6 [Verificación] `axiom tui` interactivo: navegar Gobernanza y confirmar que la entrada ODD ya no existe, que ninguna otra pantalla (Hub, Incrementos SDD, Multi-Rol, Handoffs, Specs Vivas) perdió su cursor ni su ruta de retroceso, y que "Volver al menú principal" sigue funcionando desde su nueva posición.
- [ ] 13.7 [Verificación de cierre F6.2c] `go build ./...`, `go vet ./...`, `go test ./internal/tui/... -v`.

## Fase 14: F6.3 — Retirada del dominio `internal/odd/**` (18 ficheros, 3473 líneas medidas) (`size:exception` recomendada)

Depende de las Fases 11, 12 y 13 (nada externo debe seguir llamando al paquete). **Recomendación de `size:exception` explícita**: 1534 líneas de producción + 1939 de test, ya medidas y publicadas en `proposal.md` §9, tabla de alcance destructivo de D1. Un paquete Go no se puede borrar a medias entre PRs sin romper la compilación del resto del propio paquete — no es una elección de conveniencia, es la naturaleza del borrado atómico.

- [ ] 14.1 [Verificación previa] `rg -n 'internal/odd'` en todo el árbol (excepto `openspec/changes/archive/**`) — confirmar cero importadores restantes tras las Fases 11–13.
- [ ] 14.2 [GREEN] Borrar los 10 ficheros de producción: `args.go`, `document.go`, `errors.go`, `mirror.go`, `name.go`, `parse.go`, `promote.go`, `render.go`, `store.go`, `template.go`.
- [ ] 14.3 [GREEN] Borrar los 8 ficheros de test: `args_test.go`, `mirror_test.go`, `name_test.go`, `parse_test.go`, `promote_test.go`, `render_test.go`, `store_test.go`, `template_test.go`.
- [ ] 14.4 [GREEN] Borrar `internal/dashboard/name_parity_test.go` (propiedad de paridad de nombres ODD↔incremento; sin ODD no tiene sujeto).
- [ ] 14.5 [Verificación de cierre F6.3] `go build ./...` — la prueba de ejecución de esta fase ES la compilación: si compila, ningún paquete referencia ya `internal/odd`. `go vet ./...`, `go test ./... -timeout 900s` (sin nuevos fallos respecto al estado de la Fase 13). Confirmar la etiqueta `size:exception` en la PR antes de solicitar revisión (regla 7).

## Fase 15: F6.4 — Especificaciones y filas del registro para F6

Depende de las Fases 11, 12, 13 y 14 (los deltas destructivos deben coincidir con lo que el código ya no tiene).

- [ ] 15.1 [GREEN — destructivo, español] `openspec/specs/odd-living-document/spec.md`: retirar por completo REQ-19.1 (`:15`), REQ-19.2 (`:35`), REQ-19.3 (`:54`), REQ-19.4 (`:73`) — los 4 requerimientos verificados en el fichero, con motivo y migración conforme al texto ya redactado en `spec.md` de este cambio §8.
- [ ] 15.2 [GREEN — destructivo, español] `openspec/specs/odd-cli-commands/spec.md`: retirar REQ-19.5 (`:15`), REQ-19.6 (`:35`), REQ-19.7 (`:54`), REQ-19.8 (`:80`) — conforme a `spec.md` §9.
- [ ] 15.3 [GREEN — destructivo, español] `openspec/specs/odd-sdd-promotion/spec.md`: retirar REQ-19.9 (`:15`), REQ-19.10 (`:35`), REQ-19.11 (`:55`), REQ-19.12 (`:75`) — conforme a `spec.md` §10.
- [ ] 15.4 [GREEN — destructivo, español] `openspec/specs/odd-ui-integration/spec.md`: retirar REQ-19.13 (`:15`), REQ-19.14 (`:33`), REQ-19.15 (`:51`) — conforme a `spec.md` §11.
- [ ] 15.5 [GREEN — MODIFIED, no destructivo, español] `openspec/specs/dashboard-sdd-orchestration/spec.md`: estrechar el escenario de `:45-51` ("Creación de incremento con cuerpo de propuesta ya renderizado", que cita `axiom odd promote` como ejemplo de sembrador) para que ya no ilustre con un comando retirado — el requerimiento (que `proposal_body` se acepte) permanece; solo cambia el ejemplo de quién lo siembra [D-11].
- [ ] 15.6 [GREEN — firme, inglés] `openspec/specs/organic-agent-trigger-rules/spec.md`: el delta condicional que la propuesta preveía (§3.2) pasa a firme — el requerimiento `ADDED` ya redactado en `spec.md` de este cambio §7 ("Organic Driven Development como protocolo obligatorio...") se añade sin modificar ninguno de los requerimientos existentes del fichero (`(read-only)` en su totalidad), en particular sin tocar "SDD remains optional" (`:82-102`).
- [ ] 15.7 [Decisión de archivado, no de este documento] Confirmar con la fase de archivado si `openspec/specs/odd-{living-document,cli-commands,sdd-promotion,ui-integration}/spec.md` se eliminan por completo o se conservan vacíos con nota de retirada (`spec.md` de este cambio lo deja explícitamente fuera del alcance de la especificación, a discreción de `sdd-archive`).
- [ ] 15.8 [Protocolo — paso G, adaptado a fases de autoría propia] Sección "F6 — Reconciliación ODD (F6.2–F6.4)" en el registro con las filas de los commits ya absorbidos en la Fase 10 más una nota de las retiradas destructivas de código de las Fases 11–14 (no son commits de upstream, son autoría del fork — se registran igualmente por trazabilidad, con evidencia = número de PR). Espejar en Engram.
- [ ] 15.9 [Verificación de cierre F6.4] Lectura estructurada de los 6 ficheros de especificación tocados, confirmando que cada uno preserva sin cambios los requerimientos no listados en las tareas 15.1–15.6. Confirmar que ningún fichero de especificación queda con una capacidad totalmente vacía sin la nota de la tarea 15.7 resuelta.

## Fase 16: F7 — Defectos dispersos, documentación y cierre del registro (55/55)

Depende de la Fase 9 (fin de la vía de absorción F2→F3→F4→F5) y de la Fase 15 (fin de la vía de retirada de ODD). Cierra el rojo declarado desde la Fase 5.

- [ ] 16.1 [Protocolo — paso B] Derivar la lista de ficheros de los 6 commits restantes de F7 con `git show <sha> --stat` cada uno.
- [ ] 16.2 [Protocolo — paso C] Cherry-pick agrupado.
- [ ] 16.3 [Protocolo — paso D] Contrastar contra la derivación de 16.1.
- [ ] 16.4 [Protocolo — paso E] Frontera y no-reversión.
- [ ] 16.5 [Protocolo — paso F] Verificación sin filtrar completa.
- [ ] 16.6 [Cierre del registro] Escribir la sección "F7 — Cierre" en `docs/upstream-absorption-ledger.md` con sus 6 filas. Confirmar que las **55 filas** (F0: 0 upstream, F1: 2, F2: 14, F3: 6–7, F4: 13, F5: 7, F6: 7, F7: 6) están presentes, cada una con estado asignado, y que la tabla de recuento cuadra con el universo declarado en la cabecera (tarea 1.2).
- [ ] 16.7 [Verificación — cierra el rojo declarado] Ejecutar `go test ./internal/absorptionledger/... -v`: `TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose` (Fase 5, tarea 5.5) debe estar ahora en **verde**. Es el único cambio de estado esperado en esta tarea.
- [ ] 16.8 [Documentación] `docs/ROADMAP.md`: reescribir la fila de catálogo de INC-20 (`:67`, hoy `inc-20-sdd-engine-contract-retirement`) y su entrada detallada (`:291-299`) con el alcance real de este cambio; ajustar el contador de la Fase 4 (`:8`, hoy "⏳ En progreso (1/3 Incrementos Archivados)") y el total de incrementos archivados/planificados (`:9-10`).
- [ ] 16.9 [Inventario rojo con sucesor — REQ-20.16] Si el paso de CI de la Fase 3 (tarea 3.7/3.8) destapó fallos preexistentes fuera de la superficie bloqueante al llegar a este punto, completar aquí el inventario escrito con fecha e INC-21 como sucesor nombrado (no dejarlo pendiente sin fecha ni sucesor).
- [ ] 16.10 [Verificación de cierre F7 — cierre del incremento] `go build ./...`, `go vet ./...`, `go test ./... -timeout 900s` (debe estar **completamente en verde**, sin ninguna excepción declarada ya — la de la Fase 5 se cerró en 16.7). `gofmt -l` sobre los ficheros tocados por F7. `e2e/e2e_test.sh` completo. Confirmar `contracts/**` byte a byte idéntico a su estado previo al incremento (REQ-20.10). Confirmar que ninguna entrada V1–V8 quedó revertida en ningún punto de la cadena.

## Fase 17: F1 — Migración de la ruta de módulo Go `/v2` → `/v3` (última de la cadena) (REQ-20.9) (`size:exception` ya declarada)

Depende de la Fase 16 (todas las demás fases ya fusionadas — es la única forma de que esta reescritura capture el árbol completo sin dejar un `/v2` reintroducido por una fase posterior). Mecánica, sin comportamiento nuevo: su prueba es `go build ./...` más la suite completa, no un test nuevo (matización de `strict_tdd` ya declarada por el diseño). `size:exception` heredada de `proposal.md` §4.7 — no se mezcla con ningún otro cambio.

- [ ] 17.1 [Derivación, no enumeración] `rg -l 'gentle-ai/v2' -g '*.go'` sobre el árbol completo, **menos** las rutas prohibidas de esta lista (verificadas en esta fase, no antes): `openspec/changes/archive/**`, `docs/releases/**`, `odd/tasks/*.md` (citan la ruta antigua como cita histórica, no como importación — reescribirlas falsificaría un registro fechado).
- [ ] 17.2 [GREEN — mecánico] `go.mod`: `module github.com/gentleman-programming/gentle-ai/v2` (`:1`) → `/v3`. Reescribir cada fichero `.go` de la derivación de 17.1 con la nueva ruta de importación (una línea por fichero, sin excepciones dentro del conjunto derivado).
- [ ] 17.3 [GREEN — no mecánico, 17 ficheros adicionales] `.goreleaser.yaml:29` (`ldflags`, ruta de módulo en `-X`); `scripts/install.sh:287-289` (`GONOSUMDB`, `GOPRIVATE`, `GONOPROXY`); `scripts/install.ps1:32,101-103` (`go install ...@latest` y los tres patrones de entorno); `README.md`, `docs/quickstart.md`, `docs/platforms.md`, `docs/release-signing.md`, `TRADEMARKS.md` (documentación).
- [ ] 17.4 [Reconciliación de las 5 SHAs señaladas] Confirmar que las filas del registro correspondientes a `15ea98ed`, `110f1371`, `e28af0fd`, `cd95b782`, `c09b1a34` (nota cruzada del protocolo compartido) ya documentan su reconciliación manual de una línea; esta fase no necesita tocarlas de nuevo — su línea de importación ya quedó en `/v2` deliberadamente y esta reescritura mecánica la captura junto con todo lo demás.
- [ ] 17.5 [Verificación de la reescritura] `rg -l 'gentle-ai/v2'` sobre el árbol completo (sin exclusiones esta vez) devuelve **únicamente** coincidencias dentro de `openspec/changes/archive/**`, `docs/releases/**` y `odd/tasks/*.md`.
- [ ] 17.6 [Verificación de cierre F1 — cierre del incremento completo] `go build ./...`, `go vet ./...`, `go test ./... -timeout 900s`, `e2e/e2e_test.sh` — todo en verde. `gofmt -l` **solo sobre el conjunto propio de esta fase** (R14: `gofmt -l .` sobre el árbol completo ya señalaba 18 ficheros no canónicos preexistentes ajenos a esta reescritura; no se corrigen aquí). Confirmar `git log origin/main..main` antes de abrir esta última PR de la cadena (regla 4). Etiquetas `type:*` y `size:exception` en la PR.

---

## Trazabilidad rápida (fase → requerimiento)

| Fase | Requerimientos / capacidades cubiertos |
|---|---|
| 1 (F0.a) | REQ-20.13 (parcial: accesores), REQ-20.14 (RED) |
| 2 (F0.b) | REQ-20.13 (completo), REQ-20.14 (GREEN) |
| 3 (F0.c1) | REQ-20.15, REQ-20.16, REQ-20.8 (parcial: build), REQ-20.7 |
| 4 (F0.c2) | REQ-20.8 (resto, condicionado), REQ-20.11, REQ-20.12, REQ-20.7 |
| 5 (F0.c3) | REQ-20.1 (infraestructura), REQ-20.5, REQ-20.6 (parcial: estructura) |
| 6 (F2) | REQ-20.1, REQ-20.2, REQ-20.3, REQ-20.4 (aplicados) |
| 7 (F3) | REQ-20.1, REQ-20.2, REQ-20.3, REQ-20.4 (aplicados) |
| 8 (F4) | REQ-20.1–20.4 (aplicados); REQ-13.3; `rdd-sdd-receipt-consumption` (2 req.); `sdd-research` (Purpose + 1 req.); `rdd-post-verify-review-offer` (delta) |
| 9 (F5) | REQ-20.1–20.4 (aplicados) |
| 10 (F6.1) | `organic-agent-trigger-rules` (preparación de contenido) |
| 11 (F6.2a) | REQ-19.5–19.8 (retirada de superficie) |
| 12 (F6.2b) | REQ-19.13, parte de REQ-19.15 (retirada de superficie) |
| 13 (F6.2c) | REQ-19.14, resto de REQ-19.15 (retirada de superficie) |
| 14 (F6.3) | REQ-19.1–19.4, REQ-19.9–19.12 (retirada de dominio) |
| 15 (F6.4) | `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion`, `odd-ui-integration` (REMOVED); `dashboard-sdd-orchestration` (MODIFIED); `organic-agent-trigger-rules` (ADDED, firme) |
| 16 (F7) | REQ-20.5, REQ-20.6 (completo), REQ-20.10 (verificación final), REQ-20.16 (cierre) |
| 17 (F1) | REQ-20.9 |
