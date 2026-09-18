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
| **A** | `j42`, `j63` | La oferta de review tras verify ya no se emite | **Escenario obsoleto** |
| **B** | `j120`, `j121`, `j122`, `j127` | El aviso de deprecación del shim contamina la salida | **Defecto de arnés** |
| **C** | `j3043` | Espera `.gentle-ai/bin`; producción usa `.axiom/bin` | **Escenario obsoleto** |
| **D** | `j3336`, `j3500` | Preflight canónico de OpenCode | **Por diagnosticar** |
| **E** | `j93` | Parada sin continuación | **Defecto de producto** |

#### Grupo A — probado documentalmente

`j42` falla con `reviewOffer = <nil>, want an available invitation`; `j63` con `re-enabled archive omitted its optional fresh-review offer: <nil>`.

Ambas exigen un comportamiento que **INC-18 retiró a propósito**:

- `internal/sddstatus/review_door.go:8` — *«SDD status no longer creates review offers or calls OfferReviewAfterVerify.»*
- `design.md` de INC-18, línea 73 — *«Eliminación de llamadas activas a `OfferReviewAfterVerify`.»*
- `OfferReviewAfterVerify` no tiene **ningún llamador de producción**.

**Cuarta instancia del patrón de absorción parcial de INC-18**: se cambió la producción y no se actualizaron las journeys que asertaban lo viejo. Invisible porque el paso de bench nunca corría.

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

#### Grupo C — secuela del renombrado

`j3043`: `open /tmp/.../home/.gentle-ai/bin`. Producción usa `.axiom/bin` (`internal/opencode/background.go:331`). La journey espera el directorio viejo.

### Fase 2 — Corrección

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

- [ ] **T3 · `j93` — defecto de contrato ya identificado.** La parada `managed_assets_outdated` llega con `Continuation:<nil>`. Ese código de parada **exige** nombrar su comando de recuperación; sin él el actor se queda sin salida runnable, que es justo lo que el contrato de paradas existe para impedir. Es el único diagnóstico ya hecho.
- [ ] **T4 · Grupo TUI/PTY** (`j120`, `j121`, `j122`). Comparten superficie: renderizado bajo TTY real y conmutación de RDD desde la interfaz.
- [ ] **T5 · Grupo sync de OpenCode** (`j3336`, `j3500`, posiblemente `j3043`).
- [ ] **T6 · `j42` y `j63`**, ambas sobre la interacción entre el interruptor de RDD y el archivado SDD.
- [ ] **T7 · `j127`**, instalador personalizable y orden de presentación de RDD.

### Fase 3 — Cierre

- [ ] **T8 · Verificación.** Paso de bench en verde en el CI de Linux. Las 4 journeys que solo fallan en Windows quedan declaradas como ruido ambiental, no resueltas aquí.

---

## Criterios de aceptación

- El job `Unit Tests` en verde en `main`, incluido el paso «Run benchmark evidence».
- Cada journey corregida con su causa documentada: defecto de producto o escenario obsoleto, con la evidencia que lo decide.
- Ninguna aserción debilitada para reverdecer.

## Progreso

**0/8.** Sin iniciar.

## Siguiente paso

T1 — ejecutar el bench con salida completa y agrupar las 10 por causa raíz.
