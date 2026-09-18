# Propuesta: Flujo ODD y Promoción a SDD (inc-19-odd-workflow-and-promotion)

> **Incremento:** `inc-19-odd-workflow-and-promotion`
> **Fase del roadmap:** Fase 4 — Evolución Arquitectónica, Flujo Orgánico (ODD) y Absorción Upstream v3
> **Responsabilidad:** Flujo ODD / Experiencia & Arquitectura
> **Fecha:** 2026-09-17
> **Idioma:** Español (castellano peninsular)

---

## 0. Resumen ejecutivo

Axiom predica un flujo dual ODD/SDD (`AGENTS.md:4,12-13`) que hoy **no existe en forma ejecutable**: no hay directorio `odd/`, ni paquete Go, ni subcomando, ni superficie en las interfaces. Este incremento construye el carril ágil y, sobre todo, **la puerta entre carriles**: `axiom odd promote`.

| Pregunta | Respuesta |
|---|---|
| ¿Qué se entrega? | El documento vivo `odd/tasks/<feature>.md`, los subcomandos `axiom odd create\|status\|promote`, su superficie en Web UI y TUI, y las directrices en castellano. |
| ¿Qué hace real el «alternar con fluidez» del roadmap? | La promoción: `axiom odd promote <feature>` siembra `openspec/changes/<nombre>/proposal.md` a partir del documento vivo. Sin ella, ODD y SDD son dos silos. |
| ¿Qué queda fuera y por qué? | Toda la poda del motor SDD (`internal/sddstatus`, `axiom sdd attempt`). Se verificó que **no es burocracia huérfana sino contrato de runtime publicado**: merece un incremento propio (INC-20). |
| ¿Cuál es el riesgo dominante? | Que tras promover queden **dos carriles vivos** para el mismo trabajo. Se mitiga cerrando el documento ODD con marca de promoción. |
| ¿Está listo para ejecutar? | **Sí, con una salvedad**: queda una sola decisión de producto abierta (§9), que no bloquea las tres primeras rebanadas. |

---

## 1. Propósito (Intent)

### 1.1 El problema

- **La metodología está escrita pero no respaldada por código.** `AGENTS.md:13` ordena usar `odd/tasks/<feature>.md` para el trabajo cotidiano y reservar SDD para cuando se pida explícitamente. `GEMINI.md:17-25` repite la doctrina. No hay nada que la sostenga: ni directorio, ni paquete Go, ni `case "odd"` en `cmd/axiom/main.go:162`, ni rutas `/api/odd/*`, ni pantalla TUI. La regla depende hoy de que cada agente la recuerde.
- **Toda tarea paga el peaje formal o no se traza.** Corregir una errata entra por el ciclo de siete fases o queda sin registro. No hay término medio operativo.
- **No hay puerta entre carriles.** El roadmap pide «alternar con fluidez entre modo ágil (ODD) y modo formal (SDD)» (`docs/ROADMAP.md:288`). Sin un mecanismo de promoción, alternar significa copiar a mano el contexto de un documento a otro y perder la evidencia ya acumulada. Es exactamente el punto donde el usuario abandona el carril ágil.
- **El roadmap describe mal este incremento.** `docs/ROADMAP.md:278-289` y la fila de catálogo `:66` enuncian el alcance en términos que no se corresponden con el código (ver §1.3).

### 1.2 Por qué ahora

El INC-18 dejó la compuerta de archivado operando de forma autónoma y despejó el terreno. La exploración posterior demostró que la mitad de poda del INC-19 original **no es limpieza técnica** sino retirada de un contrato publicado, con migración coordinada de activos distribuidos y deltas que superseden requerimientos vivos. Esa mitad se difiere a INC-20 y se trata con el cuidado que merece.

La mitad ODD, en cambio, es **puramente aditiva y sin colisiones**: no existe nada de ODD ejecutable, así que puede aterrizar sin tocar ningún contrato vigente. Separarlas desbloquea el valor inmediato sin arrastrar el riesgo del contrato.

### 1.3 Correcciones verificadas al alcance declarado en el roadmap

Esta propuesta **no reproduce** los enunciados erróneos de `docs/ROADMAP.md:278-289`; los rectifica y los corrige en el propio fichero como entregable (§2.1, punto 5).

| Enunciado del roadmap | Realidad verificada en el árbol |
|---|---|
| «presupuestos de tokens» | No existe ningún presupuesto de tokens de LLM. El presupuesto mide intentos (`DefaultRuntimeAttemptLimit = 2`) y líneas cambiadas (`DefaultRuntimeChangedLines = 200`), en `internal/sddstatus/runtime_ledger.go:30-33`. |
| «contratos de admisión de investigación» | No existen dentro de `internal/sddstatus` (cero coincidencias). El contrato real vive en `internal/agents/researchcapability/contract.go`, respaldado por la especificación viva `openspec/specs/sdd-research/spec.md` y por `internal/assets/skills/sdd-research/SKILL.md`. |
| «sobrecarga burocrática» a eliminar | Los activos que Axiom distribuye a los agentes **invocan ese contrato**: `internal/assets/claude/sdd-orchestrator-workflow.md:101,104`, `internal/assets/skills/_shared/sdd-orchestrator-sections.md:23,25` y `internal/assets/skills/_shared/sdd-status-contract.md:22,24,127`, con aserciones en `internal/assets/assets_test.go:2331-2371`. Además lo exigen `openspec/specs/axiom-sdd-cli-integration/spec.md:32` (REQ-13.3) y `openspec/specs/rdd-sdd-receipt-consumption/spec.md:45` (confirmado por mantenedor). |

**Regla derivada, aplicable más allá de este incremento:** en este repositorio, «cero importadores Go» no basta para declarar código muerto. Hay que comprobar además `openspec/specs/` (especificaciones vivas) e `internal/assets/` (contratos que se inyectan en los agentes del usuario).

### 1.4 Cómo se ve el éxito

Un desarrollador abre trabajo cotidiano con un comando, lo sigue desde la TUI o el navegador sin invocar el aparato formal, y —cuando ese trabajo demuestra merecer rigor archivable— lo promueve a SDD **sin perder nada de lo ya hecho**.

---

## 2. Alcance (Scope)

### 2.1 Dentro de alcance

1. **Documento vivo ODD.** `odd/tasks/<feature>.md` con estructura canónica de 12 secciones (§4.2) e identidad estable de *feature*. Espejo de recuperación en Engram bajo el topic `odd/<feature>/tasks`, con ámbito del proyecto actual. El fichero es la fuente de verdad; el espejo lo mantiene el agente por MCP.
2. **Superficie CLI.** `axiom odd create <nombre>`, `axiom odd status` y `axiom odd promote <feature>`, montados replicando el patrón manual existente: nuevo `case "odd"` en el `switch` de `cmd/axiom/main.go:162`, función `runODD()` análoga a `runSDD()` (`:1704`) y a `runChange()` (`:390`), ficheros `internal/cli/odd_*.go`, y entrada en `printHelp()` (`:56`).
3. **Promoción ODD → SDD.** `axiom odd promote` siembra `openspec/changes/<nombre>/proposal.md` a partir del documento vivo, con el mapeo de §4.3, reutilizando el motor de creación de incrementos ya existente (`dashboard.Service.CreateIncrement`, `internal/dashboard/service.go:765`) en lugar de duplicarlo. El documento ODD **se conserva y se marca como promovido**.
4. **Integración reactiva en ambas interfaces.** Web UI (`axiom ui`): rutas `/api/odd*` registradas en `internal/dashboard/server.go:42-71`, DTOs en `types.go`, métodos en `service.go`, superficie SPA en `internal/dashboard/assets/{index.html,app.js,style.css}`. TUI (`axiom tui`): nueva pantalla en `internal/tui/screens/`, campo en `Model`, registro en `router.go:66` y entrada en el menú de Gobernanza (`internal/tui/model.go:3114-3134`). En ambas, **conmutación visible entre carril ágil (ODD) y carril formal (SDD)**.
5. **Directrices, prompts y plantillas en castellano peninsular** bajo la Persona `axiom`, en `internal/assets`.
6. **Actualización de `docs/ROADMAP.md`:** reescritura de la entrada de INC-19 (`:278-289`) y de su fila de catálogo (`:66`) con el alcance real; **alta de la entrada de INC-20** para la retirada del contrato del motor SDD; ajuste del contador de la Fase 4 (`:8`, hoy «1/2»).

### 2.2 Fuera de alcance

| Excluido | Motivo |
|---|---|
| Cualquier modificación de `internal/sddstatus` o de `internal/cli/sdd_*.go` | Contrato de runtime publicado. Se difiere íntegro a **INC-20**. |
| `axiom sdd attempt` y los activos de instrucciones que lo invocan | Mismo motivo. Retirarlo es retirar un contrato, no borrar código. → INC-20. |
| `internal/agents/researchcapability` | Cero importadores Go, pero respaldado por `openspec/specs/sdd-research/spec.md` y por el activo distribuido `internal/assets/skills/sdd-research/SKILL.md`. → INC-20. |
| Camino de escritura Go hacia Engram | No existe: el único acceso es de solo lectura vía `exec.Command("engram", "export", ...)` (`internal/sddstatus/status.go:1077-1104`). El lado Go de ODD será de solo lectura. |
| Unificación de la duplicación estructural TUI ↔ Web UI | Ambas re-derivan el estado escaneando ficheros de forma independiente (`internal/dashboard/service.go:159,200` frente a `internal/tui/model.go:5882,5915`). ODD replica el patrón; extraer el derivador común es un incremento propio. |
| Adopción de Cobra u otro enrutador de CLI | El `switch` manual se conserva; `axiom odd` lo replica. |
| Migración de los 18 incrementos SDD archivados a formato ODD | Permanecen intactos. |
| Cobertura del módulo independiente `bench/` | Segundo módulo Go sin `go.work`, no alcanzado por `go test ./...` desde la raíz. |
| Edición de `openspec/INDEX.md` | Catálogo de especificaciones vivas: lo actualiza la fase de archivado, no esta. |
| Edición de `openspec/config.yaml` | Fichero mantenido a mano; fuente de solo lectura. |

---

## 3. Capacidades (Capabilities)

> Contrato con la fase `sdd-spec`. Nombres verificados contra `openspec/specs/`.

### 3.1 Capacidades nuevas

- `odd-living-document`: ciclo de vida del documento vivo `odd/tasks/<feature>.md` — estructura canónica, identidad estable de *feature*, identificadores de tarea estables, derivación de progreso, y contrato del espejo Engram `odd/<feature>/tasks` incluida la política de divergencia.
- `odd-cli-commands`: contrato de `axiom odd create|status|promote` — argumentos, banderas, salida en texto y `--json`, códigos de salida, y presencia en `printHelp()`.
- `odd-sdd-promotion`: semántica de la frontera entre carriles — mapeo documento ODD → `proposal.md`, validación y colisión de nombres, marca de promoción, estado del documento ODD tras promover, y prohibición de fabricar contenido de especificación.
- `odd-ui-integration`: exposición reactiva del estado ODD en Web UI y TUI, con paridad entre ambas superficies y conmutación explícita de carril.

### 3.2 Capacidades modificadas

- `dashboard-sdd-orchestration`: **REQ-15.1** define hoy la creación de incrementos con una plantilla de `proposal.md` fija e incrustada (`internal/dashboard/service.go:805-836`), parametrizada solo por nombre, intención y tipo. La promoción exige que el motor admita **un cuerpo de propuesta ya renderizado**, preservando la plantilla actual sin cambio alguno cuando no se aporta. Requiere delta explícito.

### 3.3 Capacidades revisadas que **no** requieren delta

Verificado para evitar que la fase de especificación produzca deltas espurios:

| Capacidad | Por qué no cambia |
|---|---|
| `tui-ui-parity` (REQ-17.5) | Exige que el menú de bienvenida incorpore la opción de Gobernanza y conduzca a un submenú navegable; **no enumera de forma cerrada sus entradas**. Añadir una pantalla ODD no contradice el requerimiento. |
| `local-web-dashboard` | Añadir rutas no altera los requerimientos existentes del dashboard. |
| `organic-agent-trigger-rules` | Sus requerimientos fijan determinismo del renderizado, instalación solo-texto y ausencia de router de ciclo de vida. Las directrices ODD nuevas **deben cumplirlos**, no los modifican. |
| `sdd-orchestrator-assets` | Trata la paridad de guía de estrategia de cadena entre Claude y OpenCode. Intacta. |
| `axiom-sdd-cli-integration`, `rdd-sdd-receipt-consumption` | Fuera de alcance por decisión (§2.2). Sus deltas pertenecen a INC-20. |

---

## 4. Enfoque (Approach)

### 4.1 Principios rectores

1. **Aditivo antes que invasivo.** Salvo el `switch` de `cmd/axiom/main.go`, `printHelp()` y la extensión opcional de `CreateIncrement`, todo el incremento añade ficheros nuevos. Ninguna rebanada toca `internal/sddstatus`.
2. **El fichero manda; Engram es espejo de recuperación.** Consecuencia directa de que Go no tiene camino de escritura hacia Engram. Go **nunca escribe** en el espejo y **nunca resuelve** una divergencia: la informa. Cuando Engram no está disponible, el espejo se declara *pendiente*; jamás se reclama éxito ni se bloquea el trabajo local.
3. **Reutilizar lo que ya funciona.** El progreso usa `multirole.CountTasks` (`internal/multirole/barrier.go:16`). La promoción usa el motor de creación de incrementos existente, con su validación de nombre (`validIncrementNameRegex`, `service.go:762`) y sus guardas de colisión contra activos y archivados (`:776-788`). ODD no introduce una segunda arquitectura de estado ni un segundo andamiador.
4. **La promoción transporta lo conocido; nunca fabrica rigor.** Siembra propósito, alcance, enfoque y estado heredado. **No rellena la sección de capacidades**: deja un marcador explícito para `sdd-spec`. Una propuesta que aparente estar completa sin haberlo estado es peor que no promover.

### 4.2 Estructura canónica del documento vivo

Un único documento por *feature*: ni plan aparte, ni registro aparte.

| Sección | Contenido |
|---|---|
| Objetivo | Resultado buscado, en una frase. |
| Problema | Qué falla o falta hoy. |
| Porqué | Motivo de acometerlo ahora. |
| Alcance | Qué entra y qué no. |
| Restricciones | Límites técnicos, de producto o de tiempo. |
| Alcance autorizado | Rutas y superficies que el trabajo puede tocar. |
| Checklist accionable | Tareas con **identificador estable** (`T1`, `T2`, …), que sobreviven a reordenaciones. |
| Criterios de aceptación | Condición observable de «hecho». |
| Comprobaciones aplicables | Comandos concretos de verificación. |
| Progreso | Derivado del checklist mediante `multirole.CountTasks`. |
| Evidencia de verificación | Comando ejecutado y resultado observado, por tarea. |
| Siguiente paso | Qué hacer al retomar. |

### 4.3 Mapeo de la promoción: documento ODD → `proposal.md`

| Origen (documento ODD) | Destino (`proposal.md` sembrado) |
|---|---|
| Objetivo + Problema + Porqué | `## Propósito (Intent)` |
| Alcance (entra) + Alcance autorizado | `### Dentro de Alcance` |
| Alcance (no entra) + Restricciones | `### Fuera de Alcance` |
| Checklist con su estado | `## Enfoque (Approach)` (secuencia) y apéndice `## Estado heredado de ODD` |
| Criterios de aceptación | `## Criterios de Éxito` |
| Comprobaciones aplicables + Evidencia de verificación | `## Criterios de Éxito` (comprobaciones) y apéndice de estado heredado (evidencia observada) |
| Nombre de *feature* | Nombre del cambio, verbatim si es kebab-case válido; sobreescribible con `--name`; fallo ruidoso si no valida o si colisiona |
| — (no se deriva de nada) | `## Capacidades`: marcador explícito para `sdd-spec`. **La promoción no inventa capacidades.** |

**Procedencia declarada.** La propuesta sembrada indica en cabecera que procede de una promoción ODD y de qué documento, para que ningún revisor la confunda con una propuesta redactada desde cero.

**Previsualización.** `--dry-run` renderiza la propuesta por salida estándar sin escribir nada, para revisar el mapeo antes de crear el cambio.

### 4.4 Qué ocurre con el documento ODD tras la promoción

**Se conserva y se cierra; no se borra ni se mueve.**

| Efecto | Detalle |
|---|---|
| Permanencia | El documento sigue en `odd/tasks/<feature>.md`. Es **dato del usuario**, no código: ninguna operación de la CLI lo elimina. |
| Marca de promoción | Se le añade la referencia al cambio SDD creado (`openspec/changes/<nombre>/`) y su estado pasa a `promovido`. |
| Carril activo | A partir de ahí el carril formal es el activo. `axiom odd status` y ambas interfaces muestran el documento como **promovido (cerrado)**, no como trabajo en curso, y ofrecen el salto al cambio SDD. |
| Idempotencia | Una segunda promoción del mismo *feature* se rechaza de forma explícita apoyándose en esa marca. No hay creación silenciosa de duplicados. |
| Espejo | La actualización del espejo Engram la realiza el agente; la CLI la declara *pendiente*, nunca la da por hecha. |

**Alternativa descartada:** mover el documento a `odd/archive/`. Se descarta porque rompe las referencias ya escritas al documento, exige un segundo escaneo en ambas interfaces y no aporta nada que la marca de estado no resuelva. El cierre es semántico, no de sistema de ficheros.

### 4.5 Política de divergencia entre fichero y espejo

- `axiom odd status` lee **siempre y solo el fichero** por defecto. Es la ruta rápida que consumen la TUI y el sondeo del dashboard.
- La comparación con el espejo es **opcional y explícita** (`--check-mirror` en la CLI, acción deliberada en las interfaces), porque implica invocar un binario externo y no debe pagarse en cada refresco.
- Estados informados: `sincronizado`, `divergente`, `no disponible`. `no disponible` **no es un error** y no altera el código de salida del comando.
- Ante divergencia, Go informa y se detiene. La reconciliación la decide el usuario o el agente; nunca la CLI.

### 4.6 Entrega: cadena de PR apilados contra `main`

Estimación: 12-18 ficheros para ODD y 3-5 para la promoción. Supera el presupuesto de 400 líneas por PR. La estrategia de entrega de la sesión es `auto-chain`; la estrategia de cadena es **`stacked-to-main`**, no *feature branch chain*: cada rebanada deja el árbol compilando, verde y útil por sí sola, y un tracker de larga vida solo añadiría divergencia y un diff final ingobernable.

| # | Rebanada | Contenido | Aterriza sola |
|---|---|---|---|
| P1 | Núcleo del documento vivo | Paquete `internal/odd`: modelo, plantilla canónica en castellano, análisis del documento, progreso vía `multirole.CountTasks` | Sí (aditivo puro) |
| P2 | CLI `create` y `status` | `internal/cli/odd_create.go`, `odd_status.go`, `case "odd"` y `runODD()` en `cmd/axiom/main.go`, `printHelp()` | Sí |
| P3 | Promoción | `axiom odd promote`, renderizador ODD → propuesta, extensión opcional de `CreateIncrement`, marca de promoción, `--dry-run` | Sí |
| P4 | Web UI | Rutas `/api/odd*`, DTOs, métodos de servicio y superficie SPA | Sí |
| P5 | TUI | Pantalla ODD, `router.go`, menú de Gobernanza, conmutación de carril | Sí |
| P6 | Activos y documentación | Directrices y plantillas ODD en `internal/assets`; `docs/ROADMAP.md` (INC-19 reescrito, INC-20 dado de alta, contador de Fase 4) | Sí |

```
main
 └─ P1 núcleo ──► P2 CLI create|status ──► P3 promoción ──┬─► P4 Web UI ──┐
                                                          └─► P5 TUI ─────┴─► P6 activos y docs
```

P4 y P5 son independientes entre sí: ambas dependen de P3 y ninguna de la otra. Se apilan en ese orden por simplicidad de revisión, no por dependencia técnica.

La partición definitiva y el pronóstico formal de líneas corresponden a `sdd-tasks`; aquí se fija la frontera.

---

## 5. Áreas afectadas

### 5.1 Paquetes `internal/` afectados (regla `rules.proposal` de `openspec/config.yaml`)

| Paquete | Impacto | Detalle |
|---|---|---|
| `internal/odd` | **Nuevo** | Motor del documento vivo: modelo, plantilla, análisis, progreso, renderizador de promoción. |
| `internal/cli` | **Nuevo (solo ficheros `odd_*.go`)** | `odd_create.go`, `odd_status.go`, `odd_promote.go` y sus tests. **Ningún `sdd_*.go` se toca.** |
| `internal/dashboard` | **Modificado** | `types.go` (DTOs ODD + campo opcional de cuerpo sembrado en `CreateIncrementRequest`), `service.go` (`CreateIncrement` `:765`, lectura de estado ODD), `server.go` (`Router()` `:36`, registros `:42-71`), `assets/{index.html,app.js,style.css}`. |
| `internal/tui` | **Modificado** | `model.go` (campo de estado, cargador, menú de Gobernanza `:3114-3134`), `screens/odd_*.go` (nuevo), `router.go:66`. |
| `internal/assets` | **Modificado** | Directrices, prompts y plantillas ODD en castellano bajo la Persona `axiom`. No se toca ningún activo de `sdd attempt`. |
| `internal/multirole` | **Consumido, sin cambios** | `CountTasks` (`barrier.go:16`) reutilizado para el progreso ODD. |
| `internal/sddstatus` | **Intocado (prohibido)** | Fuera de alcance por decisión de producto. |
| `internal/agents/researchcapability` | **Intocado (prohibido)** | Fuera de alcance por decisión de producto. |

### 5.2 Fuera de `internal/`

| Ruta | Impacto | Detalle |
|---|---|---|
| `cmd/axiom/main.go` | Modificado | `case "odd"` en el `switch` (`:162`), `runODD()`, entrada en `printHelp()` (`:56`). |
| `odd/tasks/` | Nuevo | Directorio raíz de documentos vivos ODD. |
| `openspec/specs/` | Modificado | Delta de `dashboard-sdd-orchestration` únicamente. |
| `docs/ROADMAP.md` | Modificado | Entrada de INC-19 (`:278-289`), fila de catálogo (`:66`), alta de INC-20, contador de Fase 4 (`:8`). |
| `AGENTS.md`, `GEMINI.md` | Modificado (menor) | Referencia a los comandos ya existentes, si procede. La doctrina ya está escrita. |
| `openspec/INDEX.md`, `openspec/config.yaml` | **Intocados (prohibido)** | El índice lo actualiza la fase de archivado; el config es de solo lectura. |

---

## 6. Riesgos

| # | Riesgo | Probabilidad | Severidad | Mitigación |
|---|---|---|---|---|
| R1 | **Dos carriles vivos** para el mismo trabajo tras promover, con el usuario editando el documento ODD y el cambio SDD en paralelo. | Media | Media | El documento promovido se marca y se presenta como **cerrado** en `axiom odd status` y en ambas interfaces. Segunda promoción rechazada de forma explícita. |
| R2 | **Divergencia** entre `odd/tasks/<feature>.md` y su espejo Engram: escrituras no atómicas y Go sin camino de escritura. | Alta | Media | Contrato explícito (§4.5): el fichero es autoritativo, el espejo se declara *pendiente* si Engram falla, Go informa la divergencia pero nunca la resuelve ni bloquea el trabajo local. |
| R3 | La extensión de `CreateIncrement` rompe `axiom change create` o `POST /api/increments` (REQ-15.1). | Media | Alta | Campo **opcional**: su ausencia debe producir la plantilla actual byte a byte. Test de caracterización sobre la plantilla vigente **antes** de tocar la función, en la misma rebanada P3. |
| R4 | La promoción **fabrica rigor inexistente**: una propuesta sembrada que aparenta estar completa. | Media | Media | La promoción nunca rellena `## Capacidades`; deja marcador para `sdd-spec` y declara su procedencia ODD en cabecera. |
| R5 | Colisión o invalidez de nombre al promover (nombre de *feature* no válido como nombre de cambio, o cambio ya existente activo o archivado). | Media | Baja | Reutilización de `validIncrementNameRegex` y de las guardas de colisión ya probadas (`service.go:762,776-788`). Bandera `--name` para renombrar. Fallo ruidoso, nunca silencioso. |
| R6 | Duplicación estructural agravada: ODD replica en TUI y Web UI el mismo patrón ya duplicado para SDD. | Alta | Baja | Aceptada conscientemente. Extraer el derivador común es un incremento propio (§2.2); mezclarlo aquí duplicaría el riesgo de ambas mitades. |
| R7 | Las directrices ODD nuevas en `internal/assets` incumplen `organic-agent-trigger-rules` (renderizado no determinista o router de ciclo de vida encubierto). | Media | Media | Los activos nuevos se someten a la batería de `internal/assets/assets_test.go`; renderizado determinista verificado por test, sin vínculo evento → acción. |
| R8 | Ciclo de verificación lento: `go test ./...` tarda varios minutos sobre 811 ficheros de test, lo que desincentiva la comprobación por rebanada. | Alta | Baja | Verificación focalizada por paquete durante el trabajo (`go test ./internal/odd/... ./internal/cli/... ./internal/dashboard/...`); suite completa al cierre de cada rebanada. |
| R9 | Ruido de formato: `gofmt -l .` señala hoy 18 ficheros no canónicos (preexistente, ajeno a este incremento) que pueden contaminar diffs y falsear el recuento de líneas. | Alta | Baja | Normalizar **solo** los ficheros tocados por el incremento. No se emprende normalización global del árbol. |
| R10 | Arrastre de alcance hacia la poda diferida durante la ejecución («ya que estoy aquí»). | Media | Alta | Prohibición explícita por ruta (§5.1, §5.2). Criterio de aceptación por rebanada: el diff no contiene ningún fichero bajo `internal/sddstatus/` ni `internal/cli/sdd_*.go`. |

---

## 7. Plan de rollback

### 7.1 Reversión por rebanada

Cada rebanada es una unidad de trabajo revertible de forma aislada mediante `git revert` de su commit de fusión, sin arrastrar trabajo ajeno.

| Rebanada | Alcance del rollback | Orden |
|---|---|---|
| P1 | Elimina el paquete `internal/odd`. Aditivo puro. | Revertir **después** de P2. |
| P2 | Elimina `internal/cli/odd_*.go`, el `case "odd"`, `runODD()` y la entrada de ayuda. El binario recupera su superficie previa. | Revertir después de P3. |
| P3 | Elimina `axiom odd promote` y revierte `CreateIncrement` a su firma y comportamiento previos. **Los cambios SDD ya promovidos se conservan**: son datos ya creados en `openspec/changes/`, no artefactos del código. | Revertir antes de P2. |
| P4 | Elimina rutas `/api/odd*` y la superficie SPA. El dashboard recupera su comportamiento SDD íntegro. | Independiente. |
| P5 | Elimina la pantalla ODD y su entrada de menú. | Independiente. |
| P6 | Revierte activos y documentación. Trivial. | Independiente. |

### 7.2 Dato de usuario: nunca se revierte

Los documentos `odd/tasks/*.md` creados por el usuario **se conservan en todo rollback**. Son datos, no código. Lo mismo aplica a los directorios `openspec/changes/<nombre>/` generados por una promoción ya ejecutada.

### 7.3 Criterios de aborto

Se detiene el incremento y se revierte hasta el último estado verde si se cumple cualquiera de estas condiciones:

- `axiom change create` o `POST /api/increments` dejan de producir la plantilla vigente cuando no se aporta cuerpo sembrado (regresión de REQ-15.1).
- `axiom ui` o `axiom tui` dejan de reflejar correctamente la fase de un cambio SDD existente.
- Un diff de rebanada contiene ficheros bajo `internal/sddstatus/` o `internal/cli/sdd_*.go`.
- La CLI escribe en Engram o modifica el espejo por su cuenta.

### 7.4 Rollback de datos

No hay migración de datos ni cambio de esquema persistido. Los 18 incrementos archivados no se tocan. El espejo Engram es aditivo y versionado por `topic_key`: no se borra ni se reescribe historial.

---

## 8. Dependencias y restricciones

- **Ninguna dependencia externa nueva.** Ni librerías, ni servicios, ni cambio de toolchain (Go 1.25.10+).
- **Precedencia interna:** P1 → P2 → P3; P4 y P5 dependen de P3 y son independientes entre sí; P6 cierra.
- **Strict TDD activo** (`strict_tdd: true` en `openspec/config.yaml`): cada rebanada exige RED observado antes de implementar, GREEN y refactor. La evidencia se registra por unidad de trabajo, no se infiere.
- **Herramientas de verificación disponibles:** `go test ./...`, `go vet ./...` (hoy limpio), `gofmt`. Sin `golangci-lint` configurado.
- **Engram MCP** disponible para el espejo. Su indisponibilidad degrada el espejo, no bloquea el incremento.
- **Cobertura parcial declarada:** el módulo independiente `bench/` no queda cubierto por la verificación raíz. Ninguna rebanada lo toca.

---

## 9. Decisión de producto abierta

> Una sola, y **no bloquea P1, P2 ni P3**. Afecta a `.gitignore` y a la documentación de P6.

### D1 — ¿Se versionan los documentos `odd/tasks/*.md` en Git?

`.gitignore` no menciona hoy `odd/` y `AGENTS.md:13` no se pronuncia.

| Opción | Ventajas | Inconvenientes |
|---|---|---|
| **(a) Versionar** | El trabajo cotidiano queda trazado y revisable por el equipo; la promoción hereda un historial auditable; coherente con que `openspec/changes/` también se versiona. | Ruido en los diffs; los documentos ODD suman líneas al presupuesto de 400 por PR; conflictos de fusión en documentos muy editados. |
| **(b) Ignorar** (`odd/` en `.gitignore`) | Carril ágil sin fricción ni ruido; el documento es un cuaderno local. | El trabajo cotidiano no deja rastro compartido; al promover, la única huella durable es el apéndice de estado heredado; en equipo, cada quien tiene su carril invisible. |
| **(c) Versionar solo los promovidos** | Compromiso: lo efímero queda local, lo que escaló a SDD queda trazado. | Regla de dos estados difícil de explicar y de automatizar en `.gitignore`; probablemente se incumpla. |

**Recomendación: (a) versionar.** El propósito declarado de ODD es «progreso que merece la pena recuperar tras una interrupción»; un documento no versionado no sobrevive a un cambio de máquina ni es visible para nadie más. El coste (ruido en diffs) se contiene excluyendo los documentos ODD del recuento de líneas de revisión, igual que hoy se hace con los *goldens* generados. **Requiere confirmación del usuario antes de P6.**

---

## 10. Criterios de éxito

### 10.1 Carril ODD operativo

- [ ] `axiom odd create <nombre>` crea `odd/tasks/<nombre>.md` con las 12 secciones canónicas en castellano peninsular.
- [ ] `axiom odd status` informa del progreso leyendo el fichero, con salida en texto y `--json`, sin escribir jamás en Engram.
- [ ] `axiom odd status --check-mirror` informa `sincronizado`, `divergente` o `no disponible`, y `no disponible` no altera el código de salida.
- [ ] `axiom odd` aparece en `printHelp()`.

### 10.2 Promoción efectiva

- [ ] `axiom odd promote <feature>` crea `openspec/changes/<nombre>/proposal.md` con el mapeo de §4.3 y la procedencia declarada en cabecera.
- [ ] La propuesta sembrada **no** contiene capacidades inventadas: deja marcador explícito para `sdd-spec`.
- [ ] `--dry-run` renderiza sin escribir.
- [ ] El documento ODD se conserva, queda marcado como promovido y una segunda promoción se rechaza.
- [ ] `axiom change create` y `POST /api/increments` producen la plantilla vigente sin cambio alguno cuando no se aporta cuerpo sembrado.

### 10.3 Paridad de interfaces

- [ ] El Dashboard Web y la TUI muestran el estado ODD (documentos, progreso, promovidos) con paridad de información.
- [ ] Ambas permiten conmutar de forma visible entre carril ODD y carril SDD.

### 10.4 Frontera respetada

- [ ] Ningún diff del incremento contiene ficheros bajo `internal/sddstatus/`, `internal/cli/sdd_*.go` o `internal/agents/researchcapability/`.
- [ ] `openspec/INDEX.md` y `openspec/config.yaml` permanecen sin modificar.
- [ ] `docs/ROADMAP.md` refleja el alcance real de INC-19, da de alta INC-20 y corrige el contador de la Fase 4.

### 10.5 Entrega y calidad

- [ ] `go build ./...`, `go vet ./...` y `go test ./...` en verde al cierre de **cada** rebanada.
- [ ] `gofmt -l` no señala ningún fichero tocado por el incremento.
- [ ] Ninguna rebanada supera el presupuesto de 400 líneas modificadas sin `size:exception` aceptada.
- [ ] Cada rebanada declara su frontera de rollback y su evidencia de verificación observada.
