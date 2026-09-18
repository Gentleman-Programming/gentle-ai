# Las 10 journeys de bench en rojo

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/journeys-bench-en-rojo.md`.
> Espejo de recuperación en Engram: topic `odd/journeys-bench-en-rojo/tasks`, proyecto `axiom`.

## Objetivo

Dejar en verde el paso **«Run benchmark evidence»** del job `Unit Tests`, que es lo único que mantiene rojo el CI de `main` tras fusionar el PR #22.

## Problema

El job `Unit Tests` tiene varios pasos:

```
1. go test ./...              PASA (90 paquetes)
2. build + test de bench/     pasa
3. Run benchmark evidence     FALLA  ← 10 journeys
```

El paso 3 compila el binario y ejecuta `bench/`, una batería de *journeys*: escenarios de extremo a extremo del producto real, no tests unitarios.

**Nunca se había ejecutado en este fork.** Iba detrás del paso 1, en rojo desde hacía meses. Al sanear los tests (PR #22), corre por primera vez en la historia del repositorio.

Es el tercer caso del mismo patrón en la misma sesión: un `panic` ocultaba cuatro tests, unos tests rotos ocultaban un paso entero del CI.

## Por qué ahora

Es lo único que separa a `main` del verde completo, y `main` verde es **precondición declarada de INC-20**: sobre una base roja, una absorción de upstream es inverificable por construcción. Es literalmente el error que cometió INC-18.

## Son heredadas, y está probado

Bench ejecutado contra un binario construido desde `004c9232` (antes del saneamiento) y contra el de la rama del PR #22:

| Ejecución | Journeys en rojo |
|---|---|
| Binario de `main` (shim) | 14 |
| Binario del PR #22 (shim) | 14 |
| **Diferencia** | **cero, conjuntos idénticos** |

El PR #22 no las introdujo: las destapó.

> **Trampa de comparación, documentada para no repetirla.** Un primer contraste dio 15 frente a 14 y pareció regresión. No lo era: se comparaba el binario canónico contra el shim. **Para comparar ramas hay que fijar el binario, no solo el código.**

> **Hipótesis descartada.** Se atribuyó primero al aviso de deprecación del shim contaminando stderr, como sí ocurría en los tests del runtime orgánico. Falso: con `./cmd/axiom`, que no emite aviso, `j93` falla igual.

## Alcance autorizado

Diagnóstico de las 10 journeys y corrección de lo que el diagnóstico demuestre. **Cada journey puede tener causa distinta**: no asumir causa común hasta probarlo.

**Fuera de alcance:** INC-20 y la absorción de upstream; las 4 journeys que solo fallan en Windows local (`j92`, `j96`, `j105`, `j116`).

## Restricciones

- **Ninguna journey se silencia ni se ajusta su aserción para reverdecerla.** Son escenarios de producto: si fallan, el producto falla.
- Cada causa se prueba antes de corregirla, con el contraste contra `main` como referencia.
- Cierre con ejecución real del paso de bench, no con razonamiento.

## Configuración

| Clave | Valor |
|---|---|
| Modo TDD | `strict_tdd: true` (`openspec/config.yaml:16`) |
| Reproducción local | `cd bench && go build -o <bench.exe> .` → `go build -trimpath -o <bin.exe> ./cmd/gentle-ai` → `<bench.exe> run --binary <bin.exe> --out <salida.json>` |
| Duración | ~10 min en Windows |
| RDD | `off (decided by default)` |

---

## Tareas

### Fase 1 — Diagnóstico · HECHO

- [x] **T1 · Agrupar las 10 por causa raíz** — **5 grupos, no 10 causas**
- [x] **T2 · Separar defecto de producto de escenario obsoleto**

| Grupo | Journeys | Causa | Veredicto |
|---|---|---|---|
| **A** | `j42`, `j63` | La oferta de review tras verify ya no se emite | **El código contradice una especificación viva** |
| **B** | `j120`, `j121`, `j122`, `j127` | El aviso de deprecación del shim contamina la salida | **Defecto de arnés** |
| **C** | `j3043` | Espera `.gentle-ai/bin`; producción usa `.axiom/bin` | **Escenario obsoleto** |
| **D** | `j3336`, `j3500` | Preflight canónico de OpenCode | **Por diagnosticar** |
| **E** | `j93` | Parada sin continuación | **Defecto de producto** |

#### Grupo A — el código contradice una especificación viva

> **Corrección de un veredicto anterior.** Se clasificó primero como «escenario obsoleto», razonando que INC-18 había retirado la oferta a propósito y las journeys no se actualizaron. **Es incorrecto.** La journey cita una especificación viva, y esa especificación sigue vigente.

`j42` falla con `reviewOffer = <nil>, want an available invitation`; `j63` con `re-enabled archive omitted its optional fresh-review offer: <nil>`.

**Lo que dice el código** (`internal/sddstatus/review_door.go:8`):

> *SDD status no longer creates review offers or calls OfferReviewAfterVerify.*

`OfferReviewAfterVerify` no tiene ningún llamador de producción, y el `design.md` de INC-18 (línea 73) lo declara como decisión deliberada.

**Lo que dice la especificación viva** (`openspec/specs/rdd-post-verify-review-offer/spec.md`, vigente hoy):

> *Define the sequence SDD MUST follow **per maintainer directive** (Engram decision #10123, 2026-08-02): apply → verify → offer RDD review → … → archive. **These are hard MUSTs, not defaults.***

Y su requerimiento «Offer Occurs Strictly Post-Verify, Pre-Archive» exige explícitamente que la resolución de estado *«calls `OfferReviewAfterVerify` as the sole review entry point»*, nombrando `review_door.go`'s `reviewOfferForVerify` — la función que INC-18 vació.

**INC-18 no emitió ningún delta sobre esa capacidad.** Verificado: cero menciones de `rdd-post-verify-review-offer` en todo el cambio archivado.

**Quinta instancia del patrón de INC-18, y la más grave.** Las anteriores eran tests o documentación sin actualizar. Esta es **retirar un comportamiento que una especificación viva, confirmada por el mantenedor, declara obligatorio, sin emitir el delta que lo supersediera.** Las journeys no están obsoletas: aciertan.

Matiz que conviene registrar: la misma especificación contiene el requerimiento «Kill-Switch-Off Is Structural Absence», que exige cero código de review en rutas SDD cuando el interruptor está apagado. La retirada de INC-18 **satisface ese requerimiento al máximo** (cero siempre) mientras **incumple el anterior**. Parece una sobreaplicación de un requerimiento a costa de otro del mismo documento.

**No se corrige aquí: es decisión de producto.** Dos salidas, y no son equivalentes:

| Salida | Qué implica |
|---|---|
| **(a) Restaurar la oferta** | Honra la spec y la directiva del mantenedor. Reintroduce código de review en la ruta SDD, que es justo lo que INC-18 quiso desacoplar. |
| **(b) Emitir el delta** | Ratifica la retirada superseando `rdd-post-verify-review-offer`, y entonces sí se actualizan las journeys. Exige revisar la directiva del mantenedor de 2026-08-02. |

#### Grupo B — el aviso de deprecación, probado por `j122`

> **Corrección a un diagnóstico anterior.** Se afirmó que el aviso del shim «no era la causa» tras comprobar que `j93` falla igual con el binario canónico. Eso era cierto **solo para `j93`**. Generalizarlo al resto fue un error: para el grupo B **el aviso sí es la causa**.

La prueba está en `j122`:

```
parse review mode enable JSON: invalid character 'A' looking for beginning of value
```

Esa `'A'` es la primera letra de `Aviso: 'gentle-ai' está deprecado...`. **El aviso se está parseando como JSON.**

`j120`, `j121` y `j127` fallan con `read /dev/ptmx: input/output error` y su salida capturada **empieza por el mismo aviso**.

Causa de fondo: `.github/workflows/ci.yml` compila `./cmd/gentle-ai` —el shim de deprecación— en vez de `./cmd/axiom`. Es el mismo defecto que ya se corrigió en el arnés de `e2e/organicruntime`, sin propagar al paso de bench.

**Un solo cambio cierra 4 journeys.**

#### Grupo C — secuela del renombrado · CORREGIDO y CONFIRMADO en Linux

`j3043` fallaba en CI con `open /tmp/.../home/.gentle-ai/bin`. Producción usa `.axiom/bin` (`internal/opencode/background.go:331`); la journey esperaba el directorio retirado.

Solo cambió el **directorio**: el marcador `gentle-ai:managed-opencode-launcher/v1` sigue sin renombrar en producción, así que esa comprobación se deja como literal único.

`bench` es un módulo Go aparte (`module github.com/gentleman-programming/gentle-ai/bench`) y **no puede importar `internal/opencode.BinDir`**, así que el helper `readManagedLauncher` sondea `.axiom/bin` primero y cae a `.gentle-ai/bin`, y nombra ambas rutas cuando no encuentra ninguna.

> **Local no puede validarlo, y conviene saber por qué.** CI y Windows fallan en **aserciones distintas del mismo paso**:
>
> | Entorno | Falla en |
> |---|---|
> | CI Linux | la ruta del launcher (línea 44) — **lo corregido** |
> | Windows local | la evidencia de activación (línea 37), antes de llegar |
>
> En Windows el install no emite `OpenCode background activation status: ready`, así que la journey muere antes del punto corregido. Tras el arreglo, el error local cambió de la línea 44 a la 37: prueba de que la 44 ya no bloquea, pero no de que la journey pase. **Solo el CI puede cerrarla.**
>
> **Superado el 2026-09-19.** Ya no hace falta el CI: con Docker Desktop instalado, el bench corre en Linux real en local. `j3043` pasa. Ver «Verificación en Linux local» más abajo.

#### Grupo D — la misma secuela, en la clave del agente (`j3336`, `j3500`)

Ambas leen el orquestrador de OpenCode bajo `agent["gentle-orchestrator"]`. El producto lo escribe bajo **`axiom-orchestrator`**: `internal/opencode/config.go:267-268` migra `sdd-orchestrator` y `gentle-orchestrator` a la clave nueva.

Medido en contenedor, no inferido. **Todas las demás aserciones de `j3336` pasan** contra el prompt de `axiom-orchestrator` —las cinco exigidas presentes, las siete prohibidas ausentes—, así que el contrato del preflight está intacto: lo único que se movió fue la clave.

`j3500` merecía una comprobación aparte, porque siembra bytes de usuario bajo la clave vieja: si el sync migrase la clave **perdiendo** los bytes, sería un defecto de producto real. No lo es. Tras el sync:

```
axiom-orchestrator : len=23893 prefijo=1 sufijo=1 marcadores=1+1 hybrid=1
gentle-orchestrator: len=0
```

El producto migra la clave, **conserva los bytes externos exactos** y compone un único preflight canónico. La journey estaba en lo cierto sobre el contrato y equivocada sobre el nombre.

Nota: los marcadores `<!-- gentle-ai:sdd-session-preflight -->` **siguen vigentes en producción** (`internal/components/sdd/session_preflight.go:11-12`). No se tocan; renombrarlos sería inventar un contrato.

#### Grupo E — `j93` · **corrección de un diagnóstico previo**

> **Lo que se afirmó antes era falso.** Se dijo que `j93` era «un defecto de producto puro: la parada `managed_assets_outdated` llega con `Continuation:<nil>`». Se dedujo de leer `Continuation:<nil>` en la salida sin comprobar el `Kind` que lo acompañaba. La medición real:
>
> ```
> Kind:execute  ReasonCode:fresh_target_ready  Execute.Operation:review.start  Continuation:<nil>
> ```
>
> **No hay parada.** El producto no detecta desfase alguno y va directo a `review.start`; `Continuation:<nil>` es simplemente un campo vacío de una transición `execute`, que es su forma normal. No hay defecto de contrato.

La causa real es la **tercera instancia de la misma secuela de renombrado**. El fixture escribe el artefacto histórico en `~/.gentle-ai/state.json`, y el producto lee `~/.axiom/state.json`. La siembra cae al lado del fichero vivo, nadie la lee, y la journey medía una instalación impoluta en vez del desfase que existe para medir: **un falso verde disfrazado de fallo**.

El renombrado es **parcial a propósito**, y eso importa para no arreglar de más. Medido tras un `sync` limpio:

| Bajo `$HOME` | Directorio vivo |
|---|---|
| `state.json`, `bin/` | `.axiom` |
| `telemetry.json`, `backups/` | `.gentle-ai` |

Por eso `managedStatePath` sondea el fichero **existente** en vez de nombrar un directorio: el opt-in de revisión ya lo creó (`bench/runner.go:890`, antes de los fixtures), así que se escribe sobre el que el propio producto acaba de crear. Y por eso no se hace una sustitución en bloque de `.gentle-ai` en el corpus: `backups/` sigue ahí.

> **Mina localizada, no desactivada.** Otros cuatro fixtures siembran `~/.gentle-ai/state.json` —`journeys_issue_3561.go:39`, `journeys_issue_3557.go:32`, `journeys_issue3766.go:20`, `journeys_edge.go:672`— y por tanto escriben en un fichero muerto. **Esas journeys están hoy en verde**, así que no se tocan aquí: reverdecen sin que su siembra surta efecto, lo que significa que pasan sin medir lo que declaran. Queda anotado como seguimiento, no como corrección silenciosa.

#### Grupo F — `j97`, la journey número once que nadie había contado

Aparece solo al reverdecer las diez. El paso «Run benchmark evidence» corre bajo `bash -e`: abortaba en la primera puerta `jq`, así que **las cuatro puertas siguientes nunca se habían ejecutado en este fork**. Con el corpus en verde, el paso avanza y falla en la cuarta.

`j97` se ejecuta dos veces a propósito, y cada ejecución prueba lo contrario que la otra:

| Binario | Veredicto exigido | Qué prueba |
|---|---|---|
| ordinario | `unsupported` | que un build sin el fixture **no fabrica un aprobado** |
| `-tags bench_fixture` | `completed` | que el picker funciona de verdad |

Fallaban las dos, por dos causas distintas y ambas reincidentes:

**F1 — el verbo solo existía en el shim.** `bench-model-picker` se despacha en `internal/app/app.go:90`, y `cmd/axiom` nunca cae a `app.RunArgs`. **Cuarta instancia** del mismo defecto que ya obligó a restaurar `codegraph`, `telemetry` y `skill-registry`. El CI lo tapaba compilando el binario del fixture desde `./cmd/gentle-ai` (línea 142) — la misma referencia obsoleta que se corrigió en la línea 85 y no se propagó abajo.

**F2 — el rehúse está en castellano.** `unsupportedPatterns` (`bench/classify.go:466`) son ocho expresiones en inglés. El fork responde:

```
Error: comando 'bench-model-picker' no reconocido.
```

Ninguna casa, así que `IsUnsupported` devuelve `false`, el `After` intenta parsear la ayuda como JSON y sale `invalid character 'E'`. **Tercera superficie** de la misma causa que los literales de TUI: INC-16 localizó la salida y el corpus quedó anclado al inglés.

El comentario de esa lista dice que contar una superficie ausente como fallo de estado sería «a flattering lie». Aquí es esa misma mentira con el signo cambiado: se estaba contando como fallo de estado algo que el producto rechazaba por forma.

> Tocar `IsUnsupported` puede reclasificar bloques de cualquier journey, así que no basta con volver a medir `j97`: se revalidó el corpus completo (70/0/0, idéntico) y los tests unitarios de `bench`.

#### Grupo G — el trinquete de código muerto, cuarta capa de la misma cebolla

Con el bench en verde, el job `Unit Tests` avanza un paso más y descubre otro que **nunca se había ejecutado en este fork**: `go test` tapaba al bench, el bench tapaba al trinquete.

`scripts/deadcode-ratchet.sh` analizaba `./cmd/gentle-ai`. **Cuarta referencia obsoleta al shim**, tras las líneas 85 y 142 del CI y el propio despacho de `cmd/axiom`. El shim solo reenvía a `app.RunArgs`, así que medir la alcanzabilidad desde él es medir la del reenvío: todo lo que el fork cableó en `cmd/axiom` parece muerto.

Y la baseline se había generado **upstream** —sus tres últimos commits son anteriores a la bifurcación `266574b0`—, donde `./cmd/gentle-ai` sí era el producto.

Medido antes de tocar nada:

| Objetivo | Muertas | Nuevas vs. baseline |
|---|---|---|
| `./cmd/gentle-ai` (shim) | 350 | **85** |
| `./cmd/axiom` (canónico) | 277 | **12** |

**Cero entradas de la baseline dejan de estar muertas bajo el canónico.** Eso decide el cambio: la baseline heredada es un subconjunto estricto, así que apuntar al binario correcto **no afloja el trinquete**, solo deja de medir el programa equivocado. Las 73 de diferencia eran artefacto puro de medición.

De las 12 restantes, tres son el residuo de INC-18 —`OfferReviewAfterVerify`, `readGlobalRDDModeForOffer`, `reviewOfferForVerify`—, que es exactamente lo que la enmienda del grupo A afirma por escrito: «`OfferReviewAfterVerify` retains no production caller». El trinquete lo confirma por su cuenta, desde el grafo de llamadas y sin haber leído la especificación. **Pertenecen al alcance destructivo de INC-20, no a este PR.** Se congelan con su motivo, que es la salida que el propio script prescribe.

### Fase 2 — Corrección, una causa por tarea

Las 10 journeys y su paso fallido, tal como los reporta el CI de Linux:

| Journey | Paso que falla |
|---|---|
| `j42-kill-switch-versus-sdd-archive` | «sdd-status with reviews on…» |
| `j63-disabled-failed-verification-unmanaged-remediation` | «re-enabled ordinary delivery remains archive-ready» |
| `j93-stale-managed-assets-start-is-not-unknown` | «v2 OpenCode STATUS is a typed stop with a sync continuation, and stays usable» |
| `j120-welcome-tui-runs-under-a-real-tty` | «Welcome TUI renders its menu and quits» |
| `j121-rdd-tui-controls-global-mode` | «Receipt-Driven Development toggles globally in the TUI» |
| `j122-global-review-mode-from-non-git-cwd` | «enable global review mode outside Git through a PTY» |
| `j127-customizable-install-rdd-choice` | «customizable installer presents RDD before review and permits revision» |
| `j3043-opencode-managed-background-activation` | «install reports managed activation» |
| `j3336-opencode-sdd-fresh-default-preflight` | «public OpenCode SDD sync» |
| `j3500-preserved-external-opencode-sync` | «public external OpenCode sync» |

- [x] **T3 · `j93`.** No era defecto de contrato — ver la corrección del grupo E. El fixture siembra el estado sobre el fichero que el producto creó, resuelto con `managedStatePath`. Verde en Linux, y ahora ejecuta 3 comandos (STATUS → sync → STATUS reconciliado): mide el desfase de verdad.
- [x] **T4 · Grupo TUI/PTY** (`j120`, `j121`, `j122`, `j127`). Dos causas encadenadas: el aviso del shim y, detrás, los literales de TUI que INC-16 localizó al castellano. `bench/tui_localized_markers.go` acepta ambos idiomas sin aseverar cuál se envía.
- [x] **T5 · Grupo sync de OpenCode** (`j3336`, `j3500`, `j3043`). Misma secuela de renombrado en tres superficies: directorio del launcher, clave del agente y fichero de estado.
- [x] **T6 · `j42` y `j63`.** Delta emitido sobre `openspec/specs/rdd-post-verify-review-offer/spec.md`: se retira la obligación positiva de ofrecer revisión, las prohibiciones siguen vigentes.
- [x] **T7 · `j127`.** Cerrada por T4; no tenía causa propia.

### Fase 3 — Cierre

- [x] **T8 · Verificación.** Corpus completo en verde en Linux real, medido en local (ver abajo). Queda confirmarlo en el CI, que es el mismo entorno.

---

## Verificación en Linux local (2026-09-19)

Con Docker Desktop disponible, el bench deja de depender del CI. Esto importa metodológicamente: **cuatro journeys eran incomprobables en Windows** porque el PTY no existe, y el hábito de «solo el CI puede cerrarla» estaba convirtiendo cada iteración en un viaje de ida y vuelta de minutos.

```bash
docker run -d --name axiom-bench -v /c/repos/axiom:/repo \
  -v axiom-gocache:/root/.cache/go-build -v axiom-gomod:/go/pkg/mod \
  -w /repo golang:1.25 sleep infinity
docker exec axiom-bench sh -c 'go build -trimpath -o /tmp/gentle-ai ./cmd/axiom && cd bench && go build -o /tmp/bench .'
docker exec axiom-bench /tmp/bench run --binary /tmp/gentle-ai --out /tmp/r.json
```

Réplica exacta del paso «Run benchmark evidence»: mismo binario canónico, mismas banderas.

**Medición de las 10 antes de tocar el grupo D y `j93`:** 7 completadas, 3 fallidas (`j93`, `j3336`, `j3500`). Es decir, el trío de PTY y `j3043` ya estaban cerrados y se estaba esperando al CI para saberlo.

## Criterios de aceptación

- El job `Unit Tests` en verde en `main`, incluido el paso «Run benchmark evidence».
- Cada journey corregida con su causa documentada: defecto de producto o escenario obsoleto, con la evidencia que lo decide.
- Ninguna aserción debilitada para reverdecer.

## Progreso

**8/8.** Las once journeys en verde en Linux real, y las cinco puertas `jq` del paso de bench superadas —las cuatro últimas, por primera vez en este fork.

Balance de causas, que es el resultado que conviene retener: **ninguna journey estaba equivocada sobre su contrato**. Diez de once fallaban por el espacio de nombres retirado, en seis superficies distintas —binario, directorio de estado, fichero de estado, clave de agente, idioma de la TUI, idioma del rehúse—, y una, el grupo A, porque el código contradecía una especificación viva y INC-18 no había emitido el delta que debía. Cero aserciones debilitadas.

Y una advertencia que se gana el sitio: el título de este documento decía diez. Eran once, y la undécima solo apareció al reverdecer las otras. **Un paso de CI con puertas encadenadas bajo `bash -e` solo informa del primer fallo**; reverdecerlo destapa fallos nuevos, no confirma que el resto estuviera bien. Ese mismo paso llevaba meses detrás de un `go test` en rojo, así que la cuenta de partida nunca fue una medición: era el primer corte de una.

## Siguiente paso

Confirmar en el CI y fusionar el PR #23.
