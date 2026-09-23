# Diseño: Reconciliación con Upstream e Identidad de Distribución (inc-20-upstream-reconciliation)

> **Incremento:** `inc-20-upstream-reconciliation`
> **Fase:** `sdd-design` · **Fecha:** 2026-09-19
> **Fuente de alcance:** `openspec/changes/inc-20-upstream-reconciliation/proposal.md`
> **Decisiones de producto:** D1, D2.1–D2.4 y D3 **resueltas** (propuesta §9). Este diseño no las reabre.
> **Idioma:** Español (castellano peninsular). Identificadores Go, rutas, banderas y nombres de capacidad en inglés.

---

## 0. Resumen del diseño

| Pregunta | Respuesta |
|---|---|
| ¿Cuál es el cambio de orden respecto a la propuesta? | **F1 (ruta de módulo) pasa de primera a última.** El motivo no es el impuesto de reescritura —**remedido el 2026-09-19 en 5 commits recurrentes**, §2 D-01— sino que F1-primero invierte el orden de reversión respecto al de aplicación en exactamente un nodo [D-01]. |
| ¿Cómo se retira ODD sin romper al usuario? | **La doctrina muere antes que el verbo.** `internal/assets/assets_test.go:3067-3073` fija las cadenas `axiom odd create|status|promote` en siete activos de persona; borrar el Go primero deja el árbol **verde** y la guía **falsa** [D-02]. |
| ¿Dónde aplica la tolerancia dual? | **Regla de la raíz única**: lectura tolerante dentro del accesor; escritura **siempre por accesor, nunca por literal** —ni el retirado ni el vigente—; una sola grafía en el cable [D-03]. |
| ¿Qué defecto vivo destapó esa regla? | **El producto escribe respaldos en la raíz que él mismo llama retirada.** Cinco escritores de producción esquivan `internal/backup`; cuatro clavan `.gentle-ai/backups` y uno `.axiom/backups`. El estado del usuario queda partido **por superficie**, no por versión [D-03]. |
| ¿Se rompen los respaldos existentes? | No. **No se mueve nada**: lectura heredada permanente —que ya está implementada y especificada desde INC-12— y escritura canónica. El arreglo es de escritores y nada más [D-12]. |
| ¿Qué impide que reaparezca el patrón `cmd/gentle-ai`? | Una guarda en `go test ./...` que compara los dos *switches* de despacho como conjuntos. Es la raíz —autoridad declarada, uso no obligado— y los respaldos de [D-03] son **la misma raíz**, así que van al mismo fichero como quinta aserción [D-04]. |
| ¿En qué idioma se escribe cada delta? | **En el del documento que enmienda.** No es una preferencia: es el precedente verificado del propio fork en `rdd-post-verify-review-offer/spec.md:7,13` [D-05]. |
| ¿Qué forma tiene el registro de absorción? | La de `docs/releases/v2.2.0-closure-ledger.md`: reglas de aceptación antes de los veredictos, recuento que cuadra, procedencia fechada. Verificado por un test, no por lectura humana [D-06]. |
| ¿Qué destapó este diseño que la propuesta no contabilizó? | **`.goreleaser.yaml:13-14` publica el shim.** La distribución no se arregla tocando el instalador: el artefacto que instalaría se construye desde `./cmd/gentle-ai` [D-08]. |
| ¿Qué NO toca ninguna rebanada? | `bench/`, `internal/hub`, `internal/workspace`, `internal/multirole`, `internal/handoff`, `internal/semantic`, `internal/livingdoc`, `odd/tasks/*.md`, `openspec/changes/archive/**`, `docs/releases/**`, `openspec/INDEX.md`, `openspec/config.yaml` [D-10]. |

---

## 1. Enfoque técnico

### 1.1 La forma general: una cadena LIFO con dos guardas ejecutables

La propuesta eligió *absorción por tandas temáticas* (§4.2) y la estrategia de entrega *stacked-to-main* (§4.7). Este diseño no cambia ni la una ni la otra. Cambia **el orden de la cadena** y añade **las dos únicas piezas de código nuevo** que el incremento necesita: una guarda de binario canónico [D-04] y un validador del registro de absorción [D-06].

```
main
 └─ F0 identidad + goreleaser + CI ─► F2 telemetría ─► F3 reviewer ─► F4 poda SDD ─► F5 CLI/RTK ─┐
                                              └─► F6 ODD (destructiva) ────────────────────────────┴─► F7 cierre ─► F1 ruta de módulo
```

Tres propiedades que esta forma garantiza y la original no:

| Propiedad | Cómo se obtiene |
|---|---|
| **Orden de reversión == orden inverso de aplicación** | F1 deja de ser el nodo cuya reversión invalida a todos los posteriores (propuesta §7.1), porque no tiene posteriores [D-01]. |
| **La absorción arranca sin decisión pendiente** | D2.2 ya está resuelta (`/v3`), pero F0 tampoco depende de ella: el contrato de nombre y la ruta de módulo tocan **líneas distintas de los mismos ficheros** [D-01]. |
| **El instrumento de medida existe antes de medir** | F0 entrega la guarda de binario canónico y el paso de CI sobre `cmd/axiom`. Toda tanda posterior se verifica observando el binario real [D-07]. |

### 1.2 Correspondencia con la propuesta

| Regla rectora | Materialización en este diseño |
|---|---|
| **RA-1** — derivación obligatoria de la lista de ficheros (§4.1) | Ninguna tabla de §4 de este documento enumera ficheros de una tanda de absorción. Las tablas de §4 cubren solo F0, F6 y F1, que son **autoría del fork**, no cherry-pick: su lista de ficheros se deriva del árbol actual y cada ruta de este documento está verificada con su número de línea. |
| **RA-2** — verificación sin filtrar (§4.1) | Las dos guardas nuevas viven dentro de `go test ./...` [D-04, D-06]. Una tanda no puede presentar la evidencia de RA-2 y a la vez dejar su fila del registro sin escribir: es el mismo comando. |
| Inventario V1–V8 de no-reversión (§2.3) | Elevado a lista de rutas prohibidas ejecutable, ampliada con las cuatro clases que §7.3 omitía [D-10]. |
| La partición de §4.4 (medir sí, reparar no) | El inventario rojo vive en un comentario adyacente al paso de CI que lo produce, siguiendo el convenio ya vigente en `.github/workflows/ci.yml:78-84,189-194` y `scripts/deadcode-ratchet.sh:1-21`. Cero documentos nuevos [D-07]. |
| Re-derivación de F4 en vez de cherry-pick (§4.5) | Sin cambios. Se le añade la retirada del residuo de INC-18, que pertenece a su misma zona [D-09]. |

---

## 2. Decisiones de arquitectura

### D-01 — F1 (ruta de módulo) se ejecuta **la última**, no la primera

**Elección.** La cadena es `F0 → F2 → F3 → F4 → F5 → F6 → F7 → F1`. F1 migra `github.com/gentleman-programming/gentle-ai/v2` a `/v3` (D2.2, commit upstream `2594581e`) **después** de que la totalidad del universo esté absorbida y el registro cerrado.

**El impuesto de `/v3`, remedido el 2026-09-19.**

> **Medición ejecutada por el orquestador** sobre `upstream/main` recién traído, el 2026-09-19. Reproducible:
>
> ```
> git log --oneline -G'gentle-ai/v3' 266574b0..upstream/main -- '*.go'   ->  6
> git log --oneline -S'gentle-ai/v3' 266574b0..upstream/main -- '*.go'   ->  6
> ```
>
> `-G` (el diff contiene el patrón) y `-S` (cambia el número de apariciones) **coinciden en 6**, así que la cifra no depende del modo de búsqueda. Los seis:
>
> | `sha` | Asunto |
> |---|---|
> | `15ea98ed` | `fix(review): relay provider-owned OpenCode lens tasks (#4765)` |
> | `110f1371` | `refactor(community-tools): retire RTK integration` |
> | `e28af0fd` | `feat(opencode): add version-aware v2 beta support (#4728)` |
> | `cd95b782` | `fix(review): suppress consumed target re-review (#4737)` |
> | `c09b1a34` | `feat(telemetry): expose runtime telemetry as Prometheus counters` |
> | `2594581e` | `fix!: move the Go module path to /v3 so v3.x is installable (#4683)` |
>
> `2594581e` **es la propia migración**, así que no es impuesto: es el commit que F1 materializa. El impuesto recurrente real es de **5 commits**.

Esa medición **refuerza** esta decisión en vez de contradecirla. La propuesta (§4.3) había retirado ya su justificación original —«55 reescrituras»— y la había sustituido por «3 añaden líneas `/v3`, 5 lo mencionan con contexto». El remedido confirma el orden de magnitud **con el universo ampliado**: el impuesto sigue siendo de 5 commits aunque el conjunto a absorber haya crecido. En proporción es **menor** que cuando se escribió la propuesta.

**Por qué la propuesta la ponía primera, y qué se sostiene de esos argumentos.**

Con esa cifra, la propuesta dejó el orden en pie por otros dos argumentos. Los dos se miden aquí:

1. *«Es mecánica pura y de diff enorme; hacerla pronto saca el ruido del camino.»* **Cierto pero neutro respecto al orden.** El ruido es el mismo en cualquier posición. Lo que sí cambia con la posición es la *demostrabilidad*: F1 al final es la única tanda sin contenido de absorción, así que su diff es comprobablemente una línea por fichero y su revisión se reduce a contar líneas. F1 al principio también lo es, pero entonces las siete tandas siguientes se aplican sobre un árbol que acaba de mover 682 ficheros, y cualquier conflicto posterior es indistinguible de un residuo de la migración.

2. *«Sus 4 ficheros no mecánicos son exactamente los que F0 toca.»* **Medido falso tal como está enunciado.** El inventario real de `gentle-ai/v2` fuera de `*.go` no son 4 ficheros sino 17:

   | Fichero | Línea(s) | Clase |
   |---|---|---|
   | `go.mod` | declaración de módulo | F1, mecánico |
   | `.goreleaser.yaml` | `:29` (ldflags) | F1, mecánico — **no citado en la propuesta** |
   | `scripts/install.sh` | `:287-289` (`GONOSUMDB`, `GOPRIVATE`, `GONOPROXY`) | F1, mecánico |
   | `scripts/install.ps1` | `:32` (`go install ...@latest`), `:101-103` | F1, semi-mecánico |
   | `README.md`, `docs/quickstart.md`, `docs/platforms.md`, `docs/release-signing.md`, `TRADEMARKS.md` | varias | F1, documentación |
   | `openspec/changes/archive/**` (5 ficheros), `docs/releases/v2.2.0-closure-ledger.md`, `odd/tasks/saneamiento-ci-main.md` | varias | **Prohibidos** [D-10] |

   Y en `scripts/crosslane/`, lo que F1 toca (`battery.go:14`, `host.go:12` — líneas de importación) **no es lo que F0 toca** (`battery.go:274`, `host.go:119` — `words[0] != "gentle-ai"`). Dos tandas que editan líneas distintas del mismo fichero producen un rebase, no un conflicto. La premisa de vecindad no se sostiene.

**El argumento decisivo, derivado de la propia propuesta.** §7.1 declara: *«Revertir F1 invalida toda tanda posterior ya aplicada: debe revertirse en último lugar.»* Con F1 en primera posición, el grafo de reversión es el inverso del de aplicación **para todos los nodos menos uno**. Esa es la forma que más se equivoca bajo presión: un mantenedor que revierte en LIFO —lo natural— rompe el árbol en el primer paso. Con F1 al final, aplicación y reversión son estrictamente LIFO y §7.1 deja de necesitar una excepción escrita.

**Coste aceptado y acotado.** **Cinco** commits —`15ea98ed`, `110f1371`, `e28af0fd`, `cd95b782`, `c09b1a34`— exigirán reconciliar a mano una línea de importación durante el cherry-pick. Es una corrección de una línea dentro de un *hunk* que el operador ya está leyendo, y queda registrada como tal en el registro de absorción. El coste es **absoluto, no proporcional**: no crece aunque el universo crezca, porque depende de cuántos commits tocan la ruta de módulo, no de cuántos hay.

**Alternativas descartadas.**

1. *F1 primera (orden de la propuesta).* Descartada por la inversión del grafo de reversión y por la premisa de vecindad de ficheros, medida falsa arriba.
2. *F1 intermedia, tras F2 (la «válvula» de §4.3).* Se queda con lo peor de las dos: rompe la propiedad LIFO **y** sigue pagando parte del impuesto de importación en F3–F7. La válvula de §4.3 existía para desbloquear la absorción mientras D2.2 estaba pendiente; **D2.2 está resuelta**, así que la válvula ya no tiene función.
3. *No migrar, quedarse en `/v2`.* Contradice D2.2, que está resuelta a favor de `/v3`, y convierte el impuesto de 5 commits en permanente y creciente con cada reconciliación futura.

**Consecuencia sobre los riesgos de la propuesta.** R3 («D2.2 bloquea el camino crítico») queda **cerrado**: D2.2 está resuelta y F1 ya no es prerrequisito de nada. R4 («doble reescritura masiva») queda **cerrado**: D2.2 fijó el destino final antes de que F1 se ejecute, que era su única mitigación.

---

### D-02 — La retirada destructiva de ODD va de consumidor a proveedor, y **la doctrina muere antes que el verbo**

**La trampa, verificada.** `internal/assets/assets_test.go:3063-3134` define `axiomODDWorkflowRequired` y el test `TestPersonaAxiomAssetsDescribeODDWorkflowDeterministically`, que **exige** que los siete activos de persona contengan literalmente:

```
"`axiom odd create <nombre>`"
"`axiom odd status [--json] [--check-mirror]`"
"`axiom odd promote <feature> [--dry-run] [--name <nombre>]`"
```

Los siete activos son `internal/assets/{claude,opencode,kiro,hermes,generic}/persona-axiom.md` y `internal/assets/{claude,kimi}/output-style-axiom.md` (contenido verificado en `claude/persona-axiom.md:33-38`).

Si la capa Go se retira primero, ocurre esto: `go build ./...`, `go vet ./...`, `go test ./...` y `e2e/e2e_test.sh` quedan **todos en verde** —el test de activos solo comprueba el texto de los activos, no que el comando exista— mientras cada agente instalado le dice al usuario que ejecute `axiom odd create`, que responde `Error: comando 'odd' no reconocido` y sale con 1.

Eso es exactamente la forma de INC-18: suite verde, documentación falsa. Y es un **exit equivocado**, que `systemic-issue-triage` clasifica por encima de un exit ausente: *«Un dead end le dice al usuario que pare; un consejo que no funciona lo manda en círculos culpándose a sí mismo.»*

**Elección: cuatro rebanadas, en este orden.**

| # | Rebanada | Qué entra | Qué sigue funcionando al cerrarla |
|---|---|---|---|
| **F6.1** | **Doctrina** | Reescritura de los 7 activos de persona para que proyecten el protocolo ODD de upstream; reescritura de `axiomODDWorkflowRequired` y `axiomODDWorkflowForbiddenTriggers` (`assets_test.go:3067-3105`); absorción de `70c774f8`, `1b202d77`, `cfc415ce` hacia `internal/components/agentguidance/routing.go` —que hoy tiene **cero menciones de ODD**, verificado. | **Todo.** `axiom odd create|status|promote` sigue existiendo y funcionando. La guía deja de nombrarlo antes de que desaparezca. |
| **F6.2** | **Superficies de usuario** | `cmd/axiom/main.go`: `case "odd"` (`:356-357`), `runODD` (`:1789-1823`), `dashboardScaffolder` (`:1825-1866`), import (`:25`), ayuda (`:95-97`, `:123`). `internal/cli/odd_{create,promote,status}.go` + sus 3 tests. TUI: `internal/tui/screens/odd_features.go`, `router.go`, y los 5 puntos de `model.go`. Web UI: `internal/dashboard/{odd_service.go,odd_service_test.go}`, las 4 rutas de `server.go:58-61`, los DTO de `types.go`, `assets/index.html`, `assets/app.js`. | Nada de ODD en la CLI, la TUI ni la Web UI. **Ningún texto del producto lo nombra tampoco**: F6.1 ya los retiró. |
| **F6.3** | **Dominio** | `internal/odd/**` (18 ficheros). `internal/dashboard/name_parity_test.go`. El campo `ProposalBody` de `CreateIncrementRequest` y su rama en `service.go` **solo si** ningún otro consumidor lo usa; en caso contrario permanece, y su test de caracterización con él. | El árbol compila sin `internal/odd`. |
| **F6.4** | **Especificaciones y registro** | Deltas destructivos sobre `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion`, `odd-ui-integration`. Delta `MODIFIED` sobre `dashboard-sdd-orchestration` [D-11]. Delta sobre `organic-agent-trigger-rules`. Filas del registro para los 7 commits de F6. | — |

**Criterio de aceptación de F6.2, y es el que cierra la trampa.** Siguiendo la regla de `systemic-issue-triage` —*«al retirar un comando o verbo, haz grep de su CADENA de cara al usuario en todo el repositorio, no solo de su grafo de llamadas de símbolos»*—, F6.2 no cierra hasta que:

```
rg -n 'axiom odd|internal/odd|/api/odd|ScreenODD|tab-odd'
```

no devuelve **ninguna** coincidencia fuera de `openspec/changes/archive/**`, `odd/tasks/*.md` y `docs/ROADMAP.md`. El inventario medido hoy es de **50 ficheros**, frente a las 4 superficies que la tabla de alcance destructivo de la propuesta (§9) enumera. `systemic-issue-triage` advierte de exactamente esta proporción: *«Espera que un inventario hecho en tiempo de diseño subestime el alcance real por un factor de 2 a 5.»*

**Qué pasa con los `odd/tasks/*.md` existentes.** `odd/tasks/saneamiento-ci-main.md` y `odd/tasks/journeys-bench-en-rojo.md` **no se tocan, byte a byte**, y `odd/tasks/` entra en la lista de rutas prohibidas [D-10]. Justificación, tomada de la propia propuesta (§9): el contrato del documento `odd/tasks/<feature>.md` y su espejo Engram `odd/<feature>/tasks` son **idénticos en ambas implementaciones**. Los documentos son datos; lo que se retira es un validador, no un almacén. Su consumidor —el agente que los lee y escribe— sobrevive intacto y pasa a estar descrito por `routing.go` en vez de por `persona-axiom.md`.

**Alternativas descartadas.**

1. *Proveedor primero (borrar `internal/odd`, luego la CLI, luego los activos).* Es el orden que produce la ventana de exit equivocado descrita arriba, y la produce **con el árbol en verde**, que es lo que la hace indetectable por RA-2.
2. *Todo en una sola rebanada atómica.* La ventana desaparece, pero la rebanada toca 50 ficheros en cuatro capas y ninguna revisión de 400 líneas la admite. Además hace irrevertible por separado la parte de absorción (los 3 commits de upstream) y la parte de borrado, que tienen riesgos muy distintos.
3. *Conservar `axiom odd` como alias que imprime un aviso de deprecación.* Añade un verbo y un estado —lo que el test de sobreingeniería de `systemic-issue-triage` prohíbe— para un comando que la evidencia de D1 muestra que nadie invocó ni una vez durante una sesión completa de trabajo ODD.
4. *Dejar la doctrina apuntando a los comandos y no retirarla (D1 opción b, divergencia declarada).* D1 está **resuelta por (c)**. No se reabre.

---

### D-03 — La tolerancia dual se aplica a lo que el producto **lee**, nunca a lo que **escribe**

**El patrón establecido, verificado en cuatro sitios.**

| Sitio | Forma | Lado |
|---|---|---|
| `internal/opencode/config.go:347-352` | `managedOpenCodeAgentKeys()` devuelve `{"axiom-orchestrator", "gentle-orchestrator", "sdd-orchestrator", …}`, **grafía vigente primero** | Lectura |
| `internal/opencode/config.go:267-269` | Normaliza `sdd-orchestrator` y `gentle-orchestrator` a `axiom-orchestrator` al decodificar | Lectura |
| `internal/components/uninstall/cleaners.go:39-42` | Acepta `<!-- axiom:` y `<!-- gentle-ai:`, se queda con el índice menor | Lectura |
| `bench/forked_namespace.go:28,49-60` | `orchestratorAgentKeys` y `managedStatePath`, con la regla escrita en el comentario `:10-21` | Lectura |

Los cuatro son lectores decidiendo cuál de dos grafías encontraron en disco. **Ninguno emite las dos.**

**La primera redacción de esta decisión se quedó corta, y la medición del 2026-09-19 lo demuestra.** De esos cuatro lectores se extrajo «lectura sí, escritura no», que constriñe **cuántas** grafías puede emitir un escritor (una) pero no dice nada de **cómo** la obtiene. Ese hueco deja pasar los dos modos de fallo que están hoy en producción. La regla corregida cubre el eje entero:

| # | Forma en el lado de escritura | Veredicto | ¿En producción hoy? |
|---|---|---|---|
| E1 | Emitir **ambas** grafías | **Prohibida** — bifurca el estado; ningún lector puede decidir cuál es la buena | No |
| E2 | Literal de la grafía **retirada** en el escritor | **Prohibida** | **Sí — cuatro sitios** |
| E3 | Literal de la grafía **vigente** en el escritor | **Prohibida** | **Sí — un sitio** |
| E4 | Resolver por el **accesor del paquete propietario** | **La única correcta** | Un solo llamador de producción |

E3 es la que ninguna formulación anterior capturaba y la que hace la regla verificable. `internal/dashboard/service.go:1190` escribe en el directorio **correcto** por el procedimiento **equivocado**: una regla que solo prohibiera la grafía retirada lo bendeciría, y se rompería igual el día que la raíz vuelva a moverse. El invariante, por tanto, no es sobre *qué* grafía usa un escritor — es sobre si un escritor **nombra una grafía siquiera**:

> **Regla de la raíz única.**
> **Lectura:** tolera ambas grafías, vigente primero. **Escritura:** resuelve la ruta por el accesor del paquete propietario; nunca por un literal, ni el retirado ni el vigente. **Cable:** una sola grafía, y no es renombrable.
>
> **Invariante verificable:** el literal de una raíz de estado de usuario (`".axiom"`, `".gentle-ai"` seguidos de un subdirectorio de estado) aparece **únicamente dentro del paquete que posee esa raíz**. La tolerancia dual es una propiedad del accesor, no de sus llamadores, y por tanto existe **exactamente una vez por raíz**.

**El caso medido: la raíz de respaldos está partida en dos y el producto escribe en la que él mismo llama retirada.**

`internal/backup/manifest.go:170-186` declara el contrato en sus propios comentarios: `backupRoot()` devuelve `~/.axiom/backups` y está documentada como *«the expected parent directory for all backups»*; `legacyBackupRoot()` devuelve `~/.gentle-ai/backups` y está documentada como *legacy*. Ese contrato lo fijó INC-12 (`openspec/changes/archive/2026-09-16-inc-12-unified-axiom-user-state-and-env/design.md:51-56`, Decisión 3).

| Sitio | Lado | Grafía que usa | ¿Por accesor? |
|---|---|---|---|
| `internal/backup/manifest.go:213-225` (`isRootDirUnderBackupRoot`) | Lectura | Ambas, vigente primero | **Sí** — `BackupRootFn()` en `:214` |
| `internal/app/app.go:1097-1100` (`ListBackups`) | Lectura | Ambas, vigente primero | No — dos literales |
| `internal/cli/sync.go:509` | **Escritura** | Retirada | No — literal (E2) |
| `internal/cli/run.go:709` (+ `os.MkdirAll` en `:715`) | **Escritura** | Retirada | No — literal (E2) |
| `internal/update/upgrade/executor.go:488` (snapshot) y `:514` (poda) | **Escritura** | Retirada | No — literal (E2) |
| `internal/components/uninstall/service.go:178` (+ `os.MkdirAll` en `:179`) | **Escritura** | Retirada | No — literal (E2) |
| `internal/dashboard/service.go:1190` (`CreateBackup`, + `os.MkdirAll` en `:1191`) | **Escritura** | **Vigente** | No — literal (**E3**) |

Son **cinco** escritores de producción, no cuatro, y **no coinciden entre sí**: un `axiom sync` deja el respaldo en `.gentle-ai/backups`; crear un respaldo desde la Web UI lo deja en `.axiom/backups`. El estado del usuario queda partido por superficie, no por versión.

**Por qué los tests no lo vieron.** El corpus **replica el literal de producción en vez de verificarlo**. `internal/update/upgrade/executor_test.go:893` escribe `filepath.Join(homeDir, ".gentle-ai", "backups")`, exactamente la misma expresión que `executor.go:514`. Un test que reconstruye la constante que debería comprobar no puede detectar que la constante es la equivocada: no hay dos fuentes que contrastar, hay una copiada dos veces. Es el patrón que `systemic-issue-triage` nombra —*«Roto en producción pero verde en tests = sospecha que al test se le enseñó a estar de acuerdo»*— en su forma más simple: no se anuló una comprobación, se duplicó el dato.

> **Corrección de atribución (2026-09-19).** Una redacción anterior de este párrafo afirmaba que los paquetes externos *«sobreescriben `BackupRootFn` y después ignoran su valor»*. **Es falso y no estaba medido.** `rg -n "BackupRootFn" internal/update/upgrade/` no devuelve **ninguna** coincidencia: ese paquete no la referencia. La afirmación se dedujo del comentario de `manifest.go:188-190` en vez de comprobarse contra el árbol. El mecanismo real es la duplicación del literal, que es más simple y menos defendible que el que se describió.

**Hallazgo adyacente que sí se sostiene: el comentario nombra un consumidor inexistente.** `manifest.go:188-190` justifica la exportación de `BackupRootFn` diciendo *«Exported so tests in other packages (e.g. internal/update/upgrade) can override it»*. Ese paquete **no la sobreescribe**. Los nueve sitios que sí lo hacen viven todos dentro de `internal/backup` (`manifest_test.go:433-435`, `restore_test.go:16-22,77-83,154-160,358-360,426-428`, `retention_test.go:340-342,376-378,477-479`, `snapshot_dir_fsync_test.go:17-22`), donde exportarla no hace falta. El comentario documenta un acoplamiento que nunca existió, y esa ficción es la que hacía verosímil el error de atribución de arriba. Se limpia en [D-12].

**Es la misma raíz que [D-04], no una nueva.** Allí eran dos *switches* de despacho y ninguna guarda que obligara a que uno contuviera al otro; aquí es un accesor canónico con un llamador y cinco atajos. En los dos casos la autoridad existe y **nada obliga a usarla**. Por eso la guarda es una aserción más en el mismo fichero, no una guarda nueva [D-04].

**Dónde aplica en INC-20.**

| Superficie | ¿Dual? | Motivo |
|---|---|---|
| Raíz de respaldos (`~/.axiom/backups` vigente, `~/.gentle-ai/backups` retirada) | **Sí, en el accesor** | Contrato de INC-12. Lectura dual; escritura **solo** por accesor. Mecánica de migración en [D-12] |
| `telemetry.json` | **No** | Vive exclusivamente en `.gentle-ai`. Es una grafía única, no un par: no hay nada que tolerar. Fuera del alcance de INC-20 |
| Estado de instalación bajo `$HOME` (`.axiom/state.json`, `.axiom/bin/`) | **Sí** | Lectura de estado escrito por una versión anterior. `bench/forked_namespace.go:43-48` documenta por qué la grafía viva es la única válida para *escribir* y ambas para *localizar* |
| `scripts/crosslane/battery.go:274`, `host.go:119` (`words[0] != "gentle-ai"`) | **Sí** | Parsean un comando que **el producto imprimió**. D2.4 conserva `gentle-ai` como alias, así que ambas grafías pueden aparecer. Debe aceptar las dos, vigente primero. |
| `scripts/crosslane/hostopencode.go:243` (shim `gentle-ai` en `$PATH`) | **Sí** | Mismo caso: el plugin de OpenCode hace `spawn("gentle-ai")` (`opencode.go:382`). El shim debe resolver ambos nombres mientras D2.4 mantenga el alias. |
| `contracts/**/*.schema.json` (94 ficheros) y las **351 apariciones de `gentle-ai.*/vN` en 150 ficheros `.go`** | **No** | Son valores que el producto **escribe en el cable**. Un validador que acepte dos no ayuda a ningún consumidor; solo ayudaría emitir dos, y eso es bifurcar el protocolo. D2.3 resolvió «no tocar» y la propuesta ya costeó y descartó la aceptación dual («duplica la superficie de validación… complejidad permanente»). |
| `<!-- gentle-ai:sdd-session-preflight -->` (`internal/components/sdd/session_preflight.go:11-15`) | **No** | El fichero **ya gastó** su tolerancia dual en una migración interna a la grafía `gentle-ai`: `legacySDDSessionPreflightMarker` (`:14-15`). Añadir una grafía `axiom:` sería una tercera generación para un marcador cuyo único consumidor es el inyector del propio producto. D2.4 conserva `gentle-ai`; el marcador se queda tal cual. |
| Ruta de módulo Go (`/v2` → `/v3`, F1) | **No** | `go.mod` admite exactamente una. No es una decisión de diseño. |

**Corolario antirrecaída, y es el que importa.** Los seis hallazgos del patrón `cmd/gentle-ai` [D-04] y los cinco escritores de respaldos son **todos de lado de escritura**. La tolerancia dual los habría **ocultado** —un escritor que acepta dos rutas nunca falla— mientras que el invariante de la raíz única los expone a los once. Aplicar tolerancia dual donde no toca no es una redundancia inocua: es el mecanismo exacto por el que un defecto sobrevive meses con la suite en verde.

**Alternativas descartadas.**

1. *Tolerancia dual universal, «por si acaso».* Convierte cada aserción de identidad en no-falsable. Es el anti-patrón que este corolario describe.
2. *Retirar la tolerancia dual existente ahora que el renombrado avanza.* Rompe a todo usuario con estado escrito por una versión anterior, y contradice V2 del inventario de no-reversión (§2.3).
3. *Enunciar la regla solo sobre la grafía retirada («ningún escritor nombra `.gentle-ai`»).* Es la formulación intuitiva y **deja pasar E3**: `internal/dashboard/service.go:1190` escribe `.axiom` literal y pasaría la guarda, para romperse idéntico en la siguiente migración de raíz. El invariante correcto prohíbe **nombrar una grafía**, no una grafía concreta.
4. *Aceptación dual en `contracts/**`.* Descartada por el usuario al resolver D2.3 como «no tocar». No se reabre.

---

### D-04 — Una guarda ejecutable contra la reaparición del patrón `cmd/gentle-ai`, atacada en la **raíz**

**El patrón, medido.** El fork tiene o tenía **seis** referencias al shim deprecado donde el binario canónico es lo correcto:

| # | Sitio | Estado hoy |
|---|---|---|
| 1 | `.github/workflows/ci.yml:85` (construcción de evidencia de *bench*) | **Corregido** |
| 2 | `.github/workflows/ci.yml:144` (construcción con `-tags bench_fixture`) | **Corregido** |
| 3 | `cmd/axiom/main.go:396-415` (faltaban `codegraph`, `telemetry`, `skill-registry`, `bench-model-picker`) | **Corregido**, con el motivo escrito en `:391-395` |
| 4 | `scripts/deadcode-ratchet.sh:41` (`DEADCODE_TARGET`) | **Corregido**, con el motivo escrito en `:33-40` |
| 5 | `.github/workflows/ci.yml:266` (binario de las pruebas de bloqueo de release en Windows) | **Vivo** |
| 6 | `.goreleaser.yaml:13-14` (**el artefacto que se publica**) | **Vivo** [D-08] |

**La raíz, no las instancias.** `systemic-issue-triage` es tajante: *«Dos o más incidencias que comparten raíz = UN arreglo en la raíz. N incidencias nunca justifican N parches.»* La raíz aquí es medible: **existen dos *switches* de despacho de nivel superior y ninguna guarda comprueba que uno contenga al otro.** `internal/app/app.go:84-145` despacha 24 verbos; `cmd/axiom/main.go` despacha esos y además toda la capa de producto del fork (`init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui`, `odd`). Los cuatro comandos del hallazgo 3 existían solo en `internal/app`, así que eran alcanzables por el shim y no por el binario canónico.

**Elección: un test en Go, dentro de `go test ./...`.** Fichero nuevo `cmd/axiom/canonical_binary_test.go` (`package main`, junto a `main_test.go` que ya existe). Cuatro aserciones, de la raíz hacia las instancias:

```go
// TestAppDispatchIsSubsetOfCanonicalDispatch es la aserción de raíz: todo verbo
// alcanzable por el shim deprecado DEBE ser alcanzable por el binario canónico.
// Es la guarda que habría detectado que codegraph, telemetry, skill-registry y
// bench-model-picker solo vivían en internal/app.
func TestAppDispatchIsSubsetOfCanonicalDispatch(t *testing.T)

// TestReleaseArtifactBuildsCanonicalBinary fija .goreleaser.yaml builds[].main.
func TestReleaseArtifactBuildsCanonicalBinary(t *testing.T)

// TestWorkflowsBuildCanonicalBinary recorre .github/workflows/*.yml y rechaza
// './cmd/gentle-ai' salvo en líneas cuya razón esté escrita en una lista de
// excepciones con motivo.
func TestWorkflowsBuildCanonicalBinary(t *testing.T)

// TestDeadcodeRatchetTargetsCanonicalBinary fija el valor por defecto de
// DEADCODE_TARGET en scripts/deadcode-ratchet.sh.
func TestDeadcodeRatchetTargetsCanonicalBinary(t *testing.T)

// TestUserStateRootsResolveThroughOwningPackage es la quinta aserción, añadida
// por la medición del 2026-09-19 [D-03]. Misma raíz que la primera: una
// autoridad que existe y que nada obliga a usar. Recorre internal/**/*.go de
// producción y falla ante cualquier filepath.Join cuyos argumentos contengan el
// literal ".axiom" o ".gentle-ai" seguido de un subdirectorio de estado, fuera
// del paquete propietario de esa raíz.
func TestUserStateRootsResolveThroughOwningPackage(t *testing.T)
```

**Por qué una aserción más y no una guarda nueva.** El coordinador pidió explícitamente no duplicar. La forma del defecto de [D-03] —literal en vez del accesor del paquete— **no** está cubierta por las cuatro aserciones anteriores, que miran objetivos de construcción y verbos de despacho. Pero es **la misma raíz**: autoridad declarada, uso no obligado. Una guarda separada partiría en dos ficheros una comprobación que responde a un único invariante, y la segunda se dejaría de mantener. Va al mismo fichero.

**Lista de excepciones: exactamente una entrada.** Para que sea así, `internal/app/app.go:1097-1100` deja de construir sus dos literales y pasa a llamar a `backup.BackupRoots(home)` [D-12]. Entonces los literales de la raíz de respaldos existen **una sola vez en el árbol**, dentro de `internal/backup/manifest.go`, que es su paquete propietario. Una lista de excepciones de una entrada es una guarda mucho más fuerte que una de cinco: cada excepción es una grieta por la que el patrón vuelve.

La primera se implementa con `go/parser` y `go/ast` sobre los dos ficheros, extrayendo los literales de cadena de las cláusulas `case` del *switch* de nivel superior y comparándolos como conjuntos. Las otras tres son lectura de fichero y comparación de cadena, el mismo estilo que `internal/assets/assets_test.go` ya usa para fijar texto de activos.

**Por qué un test y no un paso de CI.** RA-2 declara que la evidencia de verificación de toda tanda es `go build`, `go vet`, `go test ./...` sin filtrar y `e2e/e2e_test.sh`. Una guarda que vive solo en un *job* de GitHub Actions **no aparece en esa evidencia**, y un operador que ejecute RA-2 en local verá verde con la guarda sin ejecutar. Meterla en `go test ./...` la vuelve inseparable de la evidencia que el incremento exige.

**Test de sobreingeniería.** ¿Añade un estado, un verbo, una bandera de configuración, una compuerta o una representación paralela de una verdad existente? **No.** Añade cuatro aserciones sobre ficheros que ya existen. La lista de excepciones con motivo es el único grado de libertad, y es la misma forma que `.deadcode-baseline.txt` ya usa en este repositorio.

**Alternativas descartadas.**

1. *Un *job* de CI nuevo.* Invisible a RA-2, como se argumenta arriba; y añade una compuerta, que es lo que el test de sobreingeniería penaliza.
2. *Borrar `cmd/gentle-ai`.* Cerraría la clase entera de golpe, pero **D2.4 está resuelta a favor de conservar `gentle-ai` como alias**. Fuera de alcance por decisión de producto, no por criterio técnico.
3. *Una fila de checklist en el registro de absorción.* Es exactamente lo que INC-18 tenía, y falló. Una lista no es una guarda.
4. *Unificar los dos *switches* en uno.* Es el arreglo correcto a largo plazo y elimina la clase por construcción, pero reescribe el despacho de dos binarios en medio de una reconciliación de decenas de commits. Se deja registrado como pregunta abierta O-2 (§9), no se hace aquí.

---

### D-05 — El idioma de un delta es el del **documento que enmienda**

**El precedente, verificado, no inventado.** El propio fork ya resolvió esta pregunta y la resolvió así:

| Documento | Origen | Idioma | Evidencia |
|---|---|---|---|
| `openspec/specs/rdd-post-verify-review-offer/spec.md` | Upstream | Inglés — **incluida la enmienda del fork** | `:7` y `:13` abren con `**Amendment (INC-18 ratification, 2026-09-18, Axiom fork): …**`, en inglés, dentro de un requerimiento en inglés |
| `openspec/specs/organic-agent-trigger-rules/spec.md` | Upstream | Inglés | `:1-9`, «Gentle AI's canonical implementation-routing facts» |
| `openspec/specs/sdd-research/spec.md`, `rdd-sdd-receipt-consumption/spec.md` | Upstream | Inglés | `:1,3` (`# … Specification`, `## Purpose`) |
| `openspec/specs/odd-cli-commands/spec.md` | Fork (INC-19) | Español | `:1-30`, incluida la marca de procedencia generada |
| `openspec/specs/axiom-sdd-cli-integration/spec.md`, `axiom-tui-branding/spec.md`, `rdd-decoupling-v3-stability/spec.md` | Fork | Español | `:3,5` (`# Especificación…`, `## Propósito`) |

**Elección: el delta hereda el idioma del documento.** No es una preferencia estética. Una especificación viva es un contrato continuo que se lee de arriba abajo; una enmienda en castellano dentro de un requerimiento en inglés obliga a cambiar de idioma a mitad de requerimiento y parte en dos el corpus para cualquier `rg` sobre palabras clave normativas (`MUST`, `DEBE`).

**Tabla de aplicación para INC-20.**

| Capacidad | Origen | Idioma del delta | Fase |
|---|---|---|---|
| `upstream-absorption-protocol` (nueva) | Fork | **Español** | F0 |
| `axiom-distribution-identity` (nueva) | Fork | **Español** | F0 |
| `axiom-binary-ci-coverage` (nueva) | Fork | **Español** | F0 |
| `axiom-sdd-cli-integration` | Fork (INC-13) | **Español** | F4 |
| `rdd-sdd-receipt-consumption` | Upstream | **Inglés** | F4 |
| `sdd-research` | Upstream | **Inglés** | F4 |
| `rdd-post-verify-review-offer` | Upstream | **Inglés** | F4 [D-09] |
| `organic-agent-trigger-rules` | Upstream | **Inglés** | F6.4 |
| `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion`, `odd-ui-integration` | Fork (INC-19) | **Español** | F6.4 |
| `dashboard-sdd-orchestration` | Fork | **Español** | F6.4 [D-11] |
| `axiom-tui-branding` | Fork (INC-14) | **Español** | F0, si D2.4 lo exige |
| `rdd-decoupling-v3-stability` | Fork (INC-18) | **Español** | F3, si la medición de solape lo exige |

**Sin tensión con el contrato de idioma del repositorio.** `openspec/config.yaml:14` exige castellano para «todos los artefactos de SDD». Los artefactos de SDD de este incremento —`proposal.md`, `design.md`, `tasks.md`, `spec.md` bajo `openspec/changes/inc-20-*/`— **son castellano** y este documento lo es. Las especificaciones vivas bajo `openspec/specs/` no son artefactos del cambio: son el contrato publicado, 49 dominios de los cuales una parte sustantiva es upstream. La regla del config gobierna lo que el incremento **escribe como suyo**; la regla de D-05 gobierna lo que el incremento **enmienda de otro**.

**Alternativas descartadas.**

1. *Todo en castellano, traduciendo el documento anfitrión al enmendarlo.* Convierte cada delta en una traducción completa de un documento upstream, multiplica el diff por diez y garantiza que la próxima absorción entre en conflicto con cada línea traducida. Contradice directamente D2.2/D3, que optimizan por facilidad de reconciliación futura.
2. *Todo en inglés, incluidas las capacidades propias del fork.* Rompe 49 dominios vivos cuya mitad ya está en castellano por INC-01 a INC-19, y contradice V6 del inventario de no-reversión.
3. *Castellano en el cuerpo, inglés en las palabras clave RFC 2119.* Es lo que `odd-cli-commands/spec.md:17` ya hace a medias (`DEBE proveer`), pero como regla general produce documentos bilingües por línea. El precedente verificado no hace eso: elige un idioma por documento.

---

### D-06 — El registro de absorción copia la forma de `docs/releases/v2.2.0-closure-ledger.md`, y lo verifica un test

**El precedente existe en el repositorio.** `docs/releases/v2.2.0-closure-ledger.md` es un registro durable de disposición y supersesión, y tiene exactamente las tres propiedades que el registro de absorción necesita:

| Propiedad | Dónde está | Qué se copia |
|---|---|---|
| **Reglas de aceptación antes de cualquier veredicto** | `:11-21`, siete reglas numeradas, cada una un criterio de rechazo | RA-1 y RA-2 reenunciadas como reglas del registro, más la comprobación V1–V8 y «una tanda revertida actualiza su fila, nunca la borra» |
| **Recuento que tiene que cuadrar** | `:23-37`, tabla de disposiciones con totales | Los estados deben sumar el **universo medido al abrir el registro**, nunca una constante escrita en tiempo de diseño |
| **Procedencia fechada en la cabecera** | `:3-9`, *released-as*, rama, fecha, alcance medido | Ancestro común `266574b0`, `sha` de `upstream/main`, fecha de medición **2026-09-18** |

Dos reglas de aquel documento se adoptan literalmente porque son antídotos directos de INC-18: *«"El código parece relacionado" no es evidencia»* (`:16`) y *«Donde la supersesión no puede establecerse con evidencia, el veredicto es "no está claro — requiere confirmación del autor". Adivinar es peor que admitir incertidumbre»* (`:21`).

**Elección de forma.** `docs/upstream-absorption-ledger.md`, **en castellano** (su lector es el próximo mantenedor de Axiom, y `docs/ROADMAP.md:1` fija el idioma de `docs/`; el *closure ledger* está en inglés porque `:9` declara que se escribió en respuesta a un lector de upstream). SHAs, rutas y comandos, verbatim.

Estructura: **una sección por tanda con su tabla**, no una tabla plana. Motivo: la unidad de revisión y de reversión es la tanda, no el commit; una tabla plana obliga a filtrar mentalmente decenas de filas para revisar siete.

**El universo es un parámetro, no una constante.** Se resuelve **al abrir el registro** (primera tarea de F0), con este comando exacto, y se escribe junto con la fecha en que se ejecutó:

```
git rev-list --count --no-merges 266574b0..upstream/main
```

`--no-merges` es obligatorio y no es un detalle: la propuesta excluye los merges del alcance a propósito (§2.2, «se absorbe contenido, no topología de historia») y hace del recuento sin merges un criterio de aceptación (§10.2). Un total tomado **con** merges no puede cuadrar nunca contra un registro cuyas filas son, por construcción, commits sin merge.

```markdown
# Registro de absorción upstream — Axiom

> **Medido el:** <AAAA-MM-DD> · **Ancestro común:** `266574b0` · **`upstream/main`:** `<sha>`
> **Universo:** <N> commits de `266574b0..upstream/main` **sin merges**
> **Comando:** `git rev-list --count --no-merges 266574b0..upstream/main`

## Reglas de aceptación
1. …

## Recuento
| Estado | Filas |
|---|---|
| `absorbido` | N |
| `descartado-deliberadamente` | N |
| `revertido` | N |
| **Total** | **\<N\> (= universo declarado en la cabecera)** |

## F2 — Telemetría VictoriaMetrics
| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|
| `abc1234` | … | `absorbido` | PR #NN | — |

### Ficheros derivados y ausentes (RA-1)
| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|
```

**Cómo se verifica.** Un paquete nuevo mínimo, `internal/absorptionledger`, con `Parse([]byte) ([]Row, error)` y un test que lee `docs/upstream-absorption-ledger.md` y afirma:

1. El total de filas es exactamente el **universo declarado en la cabecera** del propio registro. La aserción es **relativa al documento**, no a una constante compilada: un `Universe` distinto en cada reconciliación es lo normal, un total que no cuadra con él es el defecto.
2. Todo estado pertenece a `{absorbido, descartado-deliberadamente, revertido}`.
3. Toda fila cuyo estado no sea `absorbido` tiene motivo no vacío.
4. La tabla de recuento cuadra con el conteo real de filas por estado.
5. Ninguna cabecera de sección de tanda se repite y todas pertenecen a `{F0…F7}`.
6. Todo `sha` es único y coincide con `^[0-9a-f]{7,40}$`.

**Lo que este test NO prueba, dicho sin adornos.** No prueba que los `sha` listados sean los correctos, ni que el universo declarado en la cabecera sea el que `git rev-list --count --no-merges` devolvería hoy, ni que una fila marcada `absorbido` lo esté de verdad. Las tres cosas exigen `git` contra un remoto ya traído y quedan fuera de lo que `go test ./...` puede afirmar sin red. El test prueba que el registro es **internamente coherente y completo respecto a lo que él mismo declara**; la verdad de cada fila es trabajo de RA-1, por tanda y en revisión. Confundir las dos cosas sería fabricar exactamente el tipo de certificado que INC-18 emitió.

**Por qué esto sí cierra el bucle.** RA-2 declara que la evidencia de toda tanda es un `go test ./...` sin filtrar. El validador del registro vive dentro de ese mismo comando. Una tanda **no puede** presentar evidencia de RA-2 y a la vez dejar su fila sin escribir: son la misma ejecución. Ese acoplamiento es el entregable, no el fichero Markdown.

**Alternativas descartadas.**

1. *Revisión humana del registro.* Es lo que INC-18 tuvo. `verify-report.md` certificó «PASS 100 %» sobre una suite filtrada y nadie lo cruzó con nada.
2. *El registro como artefacto del cambio, en `openspec/changes/inc-20-*/`.* Ya descartada en la propuesta §4.6 con el motivo correcto: un artefacto de cambio desaparece en el archivo y deja de ser consultable como estado actual. Se confirma.
3. *Solo espejo en Engram, sin fichero.* El espejo es aditivo y versionado por `topic_key` (§7.4), lo que es una virtud para el histórico y un defecto para la verificación: no hay forma de que `go test ./...` lo lea sin red ni subproceso. El fichero es la fuente; Engram es el espejo. Es la misma jerarquía que INC-19 fijó para `odd/tasks/*.md`.
4. *Tabla plana de una fila por commit.* Descartada arriba: la unidad de revisión es la tanda.
6. *Fijar el universo como constante en el código del validador.* Es el defecto que la medición del 2026-09-19 destapó: el universo creció entre la propuesta y esta fase, y volverá a crecer (R8). Una constante compilada obliga a tocar Go cada vez que upstream avanza, y garantiza que el registro nazca sin cuadrar. El universo lo declara el documento; el test lo lee de ahí.
5. *Un esquema JSON en `contracts/`.* D2.3 congela ese directorio y el registro no es contrato de cable. Además obliga a un generador para producir el Markdown legible, que es lo único que un mantenedor va a leer.

---

### D-07 — La cobertura de `cmd/axiom` es **humo de superficie**, y su ventana informativa vive junto al paso que la produce

**Elección.** F0 añade a `.github/workflows/ci.yml` un paso que construye `./cmd/axiom` y ejercita cada verbo exclusivo en su forma de ayuda o sin argumentos, afirmando dos cosas por verbo: el código de salida esperado y que **stderr no contiene el aviso de deprecación** (`cmd/gentle-ai/main.go:14`). Esa segunda aserción es la que distingue «he ejercitado el binario canónico» de «he ejercitado el shim», y es la que ningún paso de CI hacía.

Verbos cubiertos: `init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui`, más los cuatro recién recableados (`codegraph`, `telemetry`, `skill-registry`, `bench-model-picker`), más `odd` hasta que F6.2 lo retire.

**La partición bloqueante/informativa (§4.4, §10.5).**

| Clase | Trato | Motivo |
|---|---|---|
| La construcción de `./cmd/axiom` | **Bloqueante** | Sin binario no hay medida |
| `--version`, `--help` y los verbos que el contrato de nombre toca | **Bloqueante** | Es la superficie que F0 modifica; si F0 la rompe, F0 no puede pasar |
| El resto de la superficie exclusiva | **Informativa, acotada y fechada** | 1919 líneas nunca ejercitadas; §4.4 entrega el instrumento, no la reparación |

**Dónde vive el inventario rojo, y por qué ahí.** En un bloque de comentario **dentro del propio paso de CI**, con fecha y con el incremento sucesor nombrado (INC-21), más una fila en `docs/ROADMAP.md`. Cero documentos nuevos.

Es el convenio que este repositorio ya usa para exactamente esta clase de cosa: `.github/workflows/ci.yml:78-84` explica ahí mismo por qué el *bench* construye el binario canónico; `:189-194` explica ahí mismo por qué el `timeout` es 20 y no 30; `scripts/deadcode-ratchet.sh:1-21` explica ahí mismo qué captura el trinquete y qué no. El motivo junto al código que lo necesita es la forma que no se desincroniza.

**Alternativas descartadas.**

1. *`docs/cmd-axiom-coverage-gap.md`, documento propio.* Segundo documento que mantener, a una distancia de la que se desincroniza en cuanto el paso cambie.
2. *Una sección dentro del registro de absorción.* El registro declara un universo cerrado de commits de upstream y su test comprueba que los totales cuadran con él [D-06]. Meter dentro una superficie que no tiene nada que ver con upstream diluye el recuento y rompe esa aserción.
3. *Hacer bloqueante toda la superficie.* Arrastra al incremento una superficie roja de tamaño desconocido en 1919 líneas. Es literalmente la historia de las 11 *journeys* de *bench*, repetida a propósito.
4. *No acotar la ventana.* Un paso permanentemente rojo se desactiva. La propuesta ya lo llama «olor» (§4.4) y exige fecha y sucesor.

---

### D-08 — `.goreleaser.yaml` entra en F0: **el artefacto publicado se construye desde el shim**

**El hallazgo, verificado, y no está en el inventario de la propuesta.**

```yaml
# .goreleaser.yaml
project_name: gentle-ai          # :3
builds:
  - main: ./cmd/gentle-ai        # :13   ← única construcción del pipeline
    binary: gentle-ai            # :14
```

Y `cmd/gentle-ai/main.go` es un shim de 27 líneas que escribe `deprecationNotice` en stderr **en cada invocación** (`:14,17`) y delega en `app.RunArgs` (`:19`). `internal/app/app.go:84-145` no tiene caso para `odd`, `init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive` ni `ui`.

**La consecuencia, dicha sin rodeos: D2.1 no se satisface tocando el instalador.** Repuntar `scripts/install.sh:17-19` hacia un repositorio de releases de Axiom, con `.goreleaser.yaml` tal como está, instalaría un binario que anuncia su propia deprecación en cada ejecución y al que le falta la capa de producto entera del fork. El defecto de distribución que la propuesta identificó (§1.1c) tiene **dos capas**, y la de abajo no está en su inventario.

**Elección: dos construcciones, no un renombrado.** D2.4 conserva `gentle-ai` como alias, así que F0 deja:

```yaml
project_name: axiom
builds:
  - id: axiom
    main: ./cmd/axiom
    binary: axiom
  - id: gentle-ai-deprecated
    main: ./cmd/gentle-ai
    binary: gentle-ai
```

Dos archivos publicados. `axiom` es el producto; `gentle-ai` es la pasarela que D2.4 conserva y que `scripts/crosslane/hostopencode.go:243` y `opencode.go:382` necesitan viva.

**Orden dentro de F0.** `.goreleaser.yaml` **antes** que `scripts/install.sh`. Un instalador que apunta a un artefacto que aún no existe es un fallo ruidoso y trivialmente detectable. Un instalador que apunta a un artefacto que existe y es el equivocado es lo que el fork tiene hoy, y ha durado meses.

**Alternativas descartadas.**

1. *Renombrar `main: ./cmd/axiom` y punto.* Retira el binario `gentle-ai` del release, que es un delta destructivo sobre REQ-14.4 y rompe el shim de `crosslane`. D2.4 lo prohíbe.
2. *Dejarlo para INC-21 con el resto de la superficie de `cmd/axiom`.* Confunde dos cosas distintas: aquello es superficie no ejercitada de tamaño desconocido; esto es **una línea, medida, y es la razón por la que la distribución no funciona**. El criterio de §4.4 —medir es barato, reparar no— no aplica a un arreglo de una línea con causa conocida.
3. *`.goreleaser.yaml` como fase propia.* Toca el mismo fichero y la misma decisión de identidad que F0. Separarlos garantiza el conflicto que §4.4 ya argumenta para el paso de CI.

---

### D-09 — El residuo de INC-18 se retira en **F4**, y el trinquete de código muerto es su evidencia

**El residuo, verificado.** Tres símbolos muertos, congelados en `.deadcode-baseline.txt` y ya ratificados como muertos por la especificación viva:

| Símbolo | Fichero | Línea del baseline |
|---|---|---|
| `OfferReviewAfterVerify` | `internal/reviewtransaction/review_offer.go` | `:166` |
| `readGlobalRDDModeForOffer` | `internal/reviewtransaction/review_offer.go` | `:167` |
| `reviewOfferForVerify` | `internal/sddstatus/review_door.go` | `:232` |

`openspec/specs/rdd-post-verify-review-offer/spec.md:13` ya lo declara: *«`OfferReviewAfterVerify` retains no production caller.»*

**Corrección de alcance.** La retirada son **cinco** líneas del baseline, no tres. `review_door.go` aporta además `resetReviewEntryHookCallCountForTest` (`:230`) y `reviewEntryHookCallCountForTest` (`:231`), que son ganchos de prueba del tercero y mueren con él.

**Elección: pertenece a F4.** `internal/sddstatus/review_door.go` está en el conjunto de ficheros de F4 (propuesta §5.1) y el commit upstream `e0774e05` —que elimina RDD del ciclo de vida SDD— es un commit de F4. Sacarlo a una fase propia obligaría a dos tandas a tocar `review_door.go`.

**Evidencia, y es una evidencia con signo.** `scripts/deadcode-ratchet.sh:64-70` reporta las entradas que dejan de aparecer como un `note:` y pide `--update`. F4 ejecuta `scripts/deadcode-ratchet.sh --update` y **el diff de `.deadcode-baseline.txt` debe contener exactamente cinco borrados y cero adiciones**. Una adición significa que F4 creó código muerto nuevo, y entonces F4 no aterriza.

`systemic-issue-triage` advierte de que *«el trinquete de una rebanada de borrado puede legítimamente ir en POSITIVO neto cuando el consumidor muere antes que su proveedor»*. Aquí no: los tres símbolos ya no tienen consumidor —ese es todo el punto— así que el delta es negativo puro. Si sale positivo, el diagnóstico es que F4 arrastró más de lo que creía y hay que medirlo antes de seguir.

**Delta de especificación.** `rdd-post-verify-review-offer` necesita un delta porque `:13` cita el símbolo por su nombre: «no conserva ningún llamador de producción» pasa a «no existe». En **inglés** [D-05].

---

### D-10 — La frontera de ficheros prohibidos, ampliada con lo que la medición encontró

**El problema.** §7.3 de la propuesta convierte en criterio de **aborto** que ningún diff contenga ficheros bajo `bench/`, `internal/hub/`, `openspec/INDEX.md` u `openspec/config.yaml`. Pero §2.3 (V8) y §5.1 protegen un conjunto mayor. Un criterio de aborto más estrecho que el inventario que dice proteger es un criterio que no puede disparar sobre una reversión de V8.

**Elección: una sola lista, y es la que se comprueba por tanda.**

| Ruta prohibida | Origen de la prohibición | Qué la haría saltar |
|---|---|---|
| `bench/` | §2.2, §10.3 | Las *journeys* en rojo son incremento aparte |
| `internal/hub/`, `internal/workspace/`, `internal/multirole/`, `internal/handoff/`, `internal/semantic/`, `internal/livingdoc/` | **V8**, §5.1 — §7.3 solo citaba `internal/hub/` | Upstream no tiene equivalente: cualquier diff aquí es una reversión accidental |
| `internal/components/uninstall/cleaners.go` | **V2**, §5.1 | Detección dual de marcadores [D-03] |
| `openspec/INDEX.md`, `openspec/config.yaml` | §2.2, §10.3 | Índice: fase de archivado. Config: solo lectura |
| `openspec/changes/archive/**` | **Medido** — 5 ficheros archivados citan `gentle-ai/v2` | F1 los reescribiría y falsificaría el registro histórico de 19 incrementos |
| `docs/releases/**` | **Medido** — `v2.2.0-closure-ledger.md` cita `gentle-ai/v2` | Registro histórico fechado; misma clase que el anterior |
| `odd/tasks/*.md` | **D1 (§9)**, y `saneamiento-ci-main.md` cita `gentle-ai/v2` | Son datos. F6 preserva el contrato del documento; F1 no reescribe una sesión pasada |

Las tres últimas clases son **hallazgos de este diseño**, no de la propuesta, y las tres son de la misma familia: **F1 es una reescritura mecánica de una cadena, y una cadena dentro de un registro histórico no es una importación, es una cita.** Un `sd` sobre todo el árbol las tocaría todas.

**Alternativas descartadas.**

1. *Mantener la lista corta de §7.3 y confiar en la revisión.* Un criterio de aborto que no cubre V8 no aborta ante una reversión de V8, que es el riesgo R2 de la propuesta, con probabilidad **alta** y severidad **alta**.
2. *Restringir F1 con una lista de inclusión en vez de una de exclusión.* Enumerar 682 ficheros a mano es precisamente lo que RA-1 prohíbe. La derivación correcta es «todo `*.go` más go.mod, menos las rutas prohibidas».
3. *Permitir que F1 reescriba los archivados «por coherencia».* Un incremento archivado es una afirmación fechada sobre lo que era cierto ese día. Reescribirlo no lo hace coherente: lo hace falso.

---

### D-11 — Tres capacidades que la propuesta no contabilizó

**Elección: se añaden al contrato con `sdd-spec`.**

| Capacidad | Qué se midió | Delta |
|---|---|---|
| `dashboard-sdd-orchestration` | `spec.md:48` cita `axiom odd promote` dentro de un escenario de `POST /api/increments` | **`MODIFIED`, no destructivo.** El requerimiento (que `proposal_body` se acepte) sigue en pie; lo que deja de ser cierto es la ilustración de quién lo siembra. El delta estrecha el ejemplo, no retira el requerimiento. F6.4, en español |
| `organic-agent-trigger-rules` | §3.2 lo marca «condicional a D1». **D1 está resuelta por (c)** | El delta pasa de condicional a **firme**. Upstream hizo de ODD el protocolo obligatorio del orquestador y esta capacidad es exactamente la proyección de enrutado (`spec.md:1-24`, verificado en inglés). F6.4, en inglés |
| `rdd-post-verify-review-offer` | `spec.md:13` cita `OfferReviewAfterVerify` por su nombre | Delta al retirar el símbolo [D-09]. F4, en inglés |

**Por qué importa que estén.** La propuesta §3.3 verifica explícitamente qué capacidades **no** cambian «para evitar deltas espurios en la fase de especificación». Ese cuidado se anula si el inventario de las que **sí** cambian está incompleto: una capacidad viva que describe un comando inexistente es documentación falsa, que es el segundo de los dos defectos que INC-18 dejó (§4.1).

---

### D-12 — Los respaldos existentes **no se mueven**: lectura heredada permanente, escritura canónica por accesor

**El problema.** Cerrar [D-03] mueve la escritura de respaldos de `~/.gentle-ai/backups` a `~/.axiom/backups`. Todo usuario que haya ejecutado `axiom sync`, `axiom install`, `axiom upgrade` o `axiom uninstall` con una versión anterior tiene respaldos bajo la raíz retirada. **Un respaldo es exactamente el artefacto al que un usuario recurre cuando algo ha ido mal**: romperlo es el peor fallo posible de este incremento.

**Elección: los respaldos existentes se quedan donde están, y se siguen leyendo para siempre.**

| Aspecto | Resolución |
|---|---|
| Respaldos bajo `.gentle-ai/backups` | **Intocados.** Siguen listándose, restaurándose y borrándose |
| Respaldos nuevos | `.axiom/backups`, resueltos por accesor |
| Lectura | Ambas raíces, vigente primero — **ya implementado** y ya especificado (INC-12 D3; `manifest.go:213-225`; `app.go:1097-1100`) |
| Retención / poda | Se aplica **solo a la raíz canónica**. La heredada queda congelada y exenta |
| Borrado | Ya funciona en ambas: `isRootDirUnderBackupRoot` (`manifest.go:213-225`) valida las dos raíces antes de `DeleteBackup` |

**Por qué no hay migración, dicho con precisión.** El lado de lectura **ya hace lo correcto**: INC-12 lo especificó y el código lo implementa. Adoptar esta opción significa que **el arreglo es de escritores y nada más** — el diff más pequeño que cierra el defecto, sin tocar una sola ruta de lectura ni un solo byte del estado del usuario.

**La poda es el único coste real, y se paga explícito.** `internal/update/upgrade/executor.go:514-517` invoca hoy `backup.Prune(backupRoot, backup.DefaultRetentionCount)` sobre la raíz **retirada**. Con los escritores migrados, la poda pasa a la canónica y la raíz heredada **deja de podarse**. Eso es aceptable y deliberado por dos motivos:

1. El conjunto heredado es **finito y congelado**: nada vuelve a escribir ahí, así que no crece.
2. Podarlo sería **borrar respaldos anteriores a la migración como efecto colateral de un renombrado**. Eso es destrucción de datos de usuario sin petición, que §7.2 de la propuesta y el criterio de rollback de este diseño prohíben.

El usuario conserva la salida autoservicio: `axiom backup` los lista y `DeleteBackup` los borra, porque la validación de contención ya acepta las dos raíces. Ningún bloqueo humano nuevo, conforme al presupuesto de bloqueo de `systemic-issue-triage`.

**La corrección de mensaje que esto arrastra.** `cmd/axiom/main.go:141` imprime hoy «No hay respaldos registrados en `~/.axiom/backups/`» mientras `app.ListBackups` (`:1091-1100`) escanea **dos** raíces. El mensaje nombra un directorio que no es el único consultado, y hasta [D-03] nombraba uno en el que ningún escritor escribía. Se corrige nombrando **las dos raíces escaneadas**. Es un arreglo de **mensaje**: `systemic-issue-triage` es explícito en que un bloqueo cuya reparación es nombrar bien la salida no debe generar maquinaria a su alrededor.

**Alternativas descartadas.**

1. *Migración en el arranque (mover `~/.gentle-ai/backups` a `~/.axiom/backups`).* Introduce una operación de sistema de ficheros que puede fallar a medias en el momento más frágil del producto. Un movimiento parcial deja manifiestos cuyo `root_dir` apunta a donde los datos ya no están; y como `manifest.go:213-225` valida la contención de `root_dir` **antes** de `DeleteBackup`, un respaldo medio movido se vuelve **imborrable desde el producto**. Se cambiaría un defecto de escritura por un modo de corrupción.
2. *Enlace simbólico o *junction* de la raíz retirada a la canónica.* En Windows —plataforma de desarrollo primaria de este repositorio— exige privilegios o modo desarrollador, y `isDirUnderRoot` (`manifest.go:193-207`) ya resuelve enlaces con `filepath.EvalSymlinks`, de modo que la contención pasaría a depender del estado del enlace. Además, INC-12 D2 (`design.md:45-47`) documenta que este producto ya tuvo que defenderse de bucles por rutas antiguas presentes en `PATH`; añadir enlaces entre raíces de estado va en la dirección contraria.
3. *Migración perezosa: mover cada respaldo la primera vez que se lee.* Convierte una lectura en una escritura. Una operación de listado que muta el disco sorprende, y `axiom backup` se ejecuta a menudo justo cuando el usuario ya sospecha que algo está mal.
4. *Ratificar la grafía retirada como canónica para respaldos.* Contradice el contrato que INC-12 fijó y que los comentarios del propio `manifest.go:170,179` declaran, y dejaría a `internal/dashboard/service.go:1190` escribiendo en la otra: no elimina la partición, solo elige el otro lado.
5. *Retirar `legacyBackupRoot()` y la lectura dual.* Es lo que haría verdad la frase «raíz única», y **borra del mapa los respaldos de todo usuario existente**. La tolerancia de lectura es permanente por diseño [D-03].

**Lo que además limpia D-12: el comentario caduco de `manifest.go:188-190`.** Justifica la exportación de `BackupRootFn` por un consumidor —`internal/update/upgrade`— que **no la referencia**. Con los accesores nuevos, `BackupRootFn` conserva un único motivo honesto para existir: las nueve sobreescrituras internas de `internal/backup`. El comentario pasa a decir eso y solo eso. No es cosmética: un comentario que inventa un acoplamiento es la clase de documentación falsa que este incremento existe para eliminar (§4.1 de la propuesta, segundo defecto de INC-18), y en este caso concreto ya indujo un error de análisis.

**El radio de impacto en el corpus, medido: 18 aserciones en 9 ficheros clavan el literal retirado; cero clavan el vigente.**

| Fichero | Sitios | Qué hacen |
|---|---|---|
| `internal/app/app_test.go` | `:39, :87, :151, :498` | Siembran manifiestos bajo la raíz |
| `internal/cli/dedup_prune_test.go` | `:20, :90, :158, :226` | Siembran (`MkdirAll` en `:21`) |
| `internal/cli/backup_metadata_test.go` | `:35, :92, :139` | Siembran directorios de *snapshot* |
| `internal/update/upgrade/effective_method_routing_test.go` | `:258, :403` | Siembran |
| `internal/update/upgrade/executor_test.go` | `:893` | Siembra |
| `internal/cli/restore_test.go` | `:19` | Siembra |
| `internal/cli/sync_test.go` | `:1867` | Lee el contenido (`os.ReadDir`) |
| **`internal/cli/compatibility_skills_test.go`** | **`:131`** | **Aserta AUSENCIA** (`:132-133`, «agentless compatibility dry-run created backup root») |
| **`internal/cli/compatibility_transaction_windows_test.go`** | **`:275`** | **Aserta AUSENCIA** (`:276-277`, «backup started after compatibility refusal») |

Que sean 18 y no 2 **mide cuánto se había normalizado el defecto**: dieciocho veces alguien escribió la ruta heredada a mano y nadie se preguntó por qué no venía del paquete.

**Las 18 no son homogéneas, y las dos últimas necesitan trato distinto.** Los 16 sitios que siembran o leen **fallan ruidosamente** al migrar los escritores: es la señal que confirma que la migración surtió efecto. Los **dos que asertan ausencia no fallan** — al contrario, pasan más fácil, porque comprueban que un directorio no se crea y nadie va a crearlo ya. Ese es el modo de fallo peor: **se vuelven vacíos**. Un test que no puede fallar por el motivo para el que se escribió es peor que un test borrado, porque sigue contando como cobertura. Los dos se **reapuntan a la raíz canónica**, que es donde ahora sí podría aparecer un respaldo indebido, y solo entonces vuelven a guardar lo que decían guardar.

> El coordinador señaló `compatibility_transaction_windows_test.go:275`. Comprobado: **son dos**, no uno. `compatibility_skills_test.go:131-133` tiene exactamente la misma forma (`os.Stat` + `!os.IsNotExist` + `t.Fatalf`) y el mismo riesgo de quedarse vacío.

**Consecuencia explícita para `sdd-tasks`.** La corrección de [D-03] y [D-12] es, en masa: 3 accesores nuevos, 5 escritores migrados, 1 lector migrado, 1 comentario, 1 mensaje de usuario, 16 aserciones reapuntadas, 2 aserciones reapuntadas **con cambio de raíz**, 1 aserción de guarda y los tests de esa guarda. **No cabe en un PR de 400 líneas.** Con `auto-chain`, la partición natural de F0 son dos rebanadas:

| Rebanada | Contenido | Por qué corta ahí |
|---|---|---|
| **F0.a** | Accesores en `internal/backup`, comentario corregido, aserción de guarda y sus tests | Deja el árbol verde sin cambiar comportamiento: nadie llama todavía a los accesores. La guarda entra **en rojo declarado** y es el RED de `strict_tdd` para F0.b |
| **F0.b** | 5 escritores + 1 lector + 18 aserciones + mensaje de `main.go:141` | Es el cambio de comportamiento, y pone la guarda de F0.a en verde |

El resto del contrato de nombre de F0 (`.goreleaser.yaml`, instalador, compuertas, *crosslane*, paso de CI) es una tercera rebanada independiente. **El pronóstico de líneas y la partición definitiva corresponden a `sdd-tasks`**; esto es la masa medida que debe heredar, no su plan.

**Fase.** F0: es contrato de identidad de estado de usuario, la misma familia que el contrato de nombre, y su guarda vive en el fichero que F0 ya crea [D-04].

**Nota de alcance verificada.** `internal/components/uninstall/service.go:178` es uno de los cinco escritores. La propuesta §5.1 marca `internal/components/uninstall` como «Intocado (prohibido)», pero el motivo que da es V2, que protege la **detección dual de marcadores de `cleaners.go`**. [D-10] ya enunció esa prohibición **con alcance de fichero** (`internal/components/uninstall/cleaners.go`), no de paquete. `service.go` queda por tanto dentro de alcance, y `cleaners.go` sigue intocable.

---

## 3. Flujo de datos

### 3.1 El protocolo de absorción de una tanda

```
upstream/main @ <sha fijado 2026-09-18>
        │
        ├─ RA-1: git show <sha> --stat  ──►  lista DERIVADA de ficheros
        │                                            │
        │                                            ▼
        ├─ cherry-pick agrupado (F2,F3,F5,F7) ──► diff de la tanda
        │   o re-derivación sobre el fork (F4, §4.5)      │
        │   o autoría propia (F0, F6, F1)                 │
        │                                                 ▼
        │                                    contraste derivada vs diff
        │                                                 │
        │                    ┌────────────────────────────┴───────────────┐
        │                    ▼                                            ▼
        │           fichero presente en ambos                fichero derivado y AUSENTE
        │                    │                                            │
        │                    │                              MOTIVO ESCRITO en el registro
        │                    │                              (sin motivo ⇒ la tanda se rechaza)
        │                    ▼                                            │
        ├─ comprobación de frontera [D-10]  ◄──────────────────────────────┘
        │       ninguna ruta prohibida en el diff  (si la hay ⇒ ABORTO, §7.3)
        │
        ├─ comprobación de no-reversión V1–V8 (§2.3)
        │
        └─ RA-2: evidencia SIN filtrar
              ├─ go build ./...
              ├─ go vet   ./...
              ├─ go test  ./...            ← aquí viven las DOS guardas nuevas
              │     ├─ cmd/axiom/canonical_binary_test.go        [D-04]
              │     └─ internal/absorptionledger/ledger_test.go  [D-06]
              ├─ gofmt -l  (solo sobre los ficheros tocados, R14)
              └─ e2e/e2e_test.sh
                    │
                    ▼
        docs/upstream-absorption-ledger.md  ──► espejo Engram (aditivo, por topic_key)
```

Las dos guardas nuevas están **dentro** de `go test ./...` a propósito: es lo que hace imposible presentar la evidencia de RA-2 con el registro sin escribir o con el binario canónico desconectado.

### 3.2 El orden de retirada de ODD, y la ventana que evita

```
ESTADO INICIAL
  doctrina (7 activos)  ──nombra──►  axiom odd create|status|promote  ──existe──►  internal/odd
        assets_test.go fija las cadenas                                      50 ficheros

F6.1  doctrina reescrita hacia routing.go
  doctrina  ──nombra──►  protocolo ODD de upstream            axiom odd …  sigue existiendo
  ┌─────────────────────────────────────────────────────────────────────────────────┐
  │ Ventana: un verbo vivo que ninguna guía menciona. INVISIBLE e INOCUA.            │
  └─────────────────────────────────────────────────────────────────────────────────┘

F6.2  superficies de usuario retiradas
  doctrina  ──nombra──►  protocolo de upstream                axiom odd …  NO existe
  Criterio de cierre:  rg 'axiom odd|internal/odd|/api/odd|ScreenODD|tab-odd'
                       ⇒ cero fuera de archive/, odd/tasks/ y ROADMAP

F6.3  dominio retirado        F6.4  deltas de especificación + filas del registro


ORDEN INVERSO (proveedor primero) — POR QUÉ SE DESCARTA
  internal/odd borrado  ──►  axiom odd NO existe  ──►  doctrina SIGUE nombrándolo
  ┌─────────────────────────────────────────────────────────────────────────────────┐
  │ go build ✓   go vet ✓   go test ./... ✓   e2e ✓      ← EL ÁRBOL ESTÁ VERDE      │
  │ assets_test.go pasa: solo comprueba el TEXTO de los activos, no que el comando  │
  │ exista. Cada agente instalado manda al usuario a un comando que sale con 1.     │
  │ Suite verde + documentación falsa = la forma exacta de INC-18.                  │
  └─────────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 La cadena de entrega y su reversión

```
APLICACIÓN           F0 ─► F2 ─► F3 ─► F4 ─► F5 ─► F6 ─► F7 ─► F1
REVERSIÓN (LIFO)     F1 ─► F7 ─► F6 ─► F5 ─► F4 ─► F3 ─► F2 ─► F0
                     └─ estrictamente inverso, sin excepciones [D-01]

Con F1 primera (orden de la propuesta):
APLICACIÓN           F1 ─► F0 ─► F2 ─► … ─► F7
REVERSIÓN            F7 ─► … ─► F2 ─► F0 ─► F1     ← F1 es el único nodo cuya
                                             posición de reversión no es la
                                             inversa de su aplicación (§7.1)
```

Toda reversión marca sus commits como `revertido` en el registro y **no borra la fila** (§7.2). Esa es la diferencia entre que la próxima reconciliación empiece midiendo y que empiece adivinando.

---

## 4. Cambios de ficheros

> **Nota de cumplimiento de RA-1.** Las tablas de esta sección cubren **únicamente** F0, F6 y F1 — las tres fases de **autoría propia**, cuya lista de ficheros se deriva del árbol actual y está verificada ruta a ruta con su número de línea. Las fases de absorción (F2, F3, F4, F5, F7) **no llevan tabla aquí a propósito**: su lista se deriva de `git show <sha> --stat` en el momento de ejecutarlas, y enumerarla ahora sería el defecto exacto que RA-1 prohíbe y el criterio de aborto de §7.3 castiga.

### 4.1 F0 — Identidad de distribución, artefacto de release y cobertura del binario real

| Fichero | Acción | Descripción |
|---|---|---|
| `.goreleaser.yaml` | Modificar | `project_name`, y **dos** construcciones: `./cmd/axiom`→`axiom` y `./cmd/gentle-ai`→`gentle-ai` (alias de D2.4). Tap de Homebrew (`:129-133`) [D-08] |
| `scripts/install.sh` | Modificar | `GITHUB_OWNER`, `GITHUB_REPO`, `BINARY_NAME`, `BREW_TAP`, `BREW_FORMULA_REF` (`:17-21`); cabecera de uso (`:5-14`) |
| `scripts/install.ps1` | Modificar | Equivalentes; `$STABLE_SOURCE_COMMAND` (`:32`) apunta al binario canónico |
| `scripts/verify-release-assets.sh` | Modificar | `:11`, literal `Gentleman-Programming/gentle-ai`; `:84`, identidad del comentario de confianza |
| `scripts/release-preflight.sh` | Modificar | `:18`, mismo literal |
| `scripts/promote-stable-preflight.sh` | Modificar | `:18`, mismo literal |
| `scripts/crosslane/battery.go` | Modificar | `:274`, `words[0] != "gentle-ai"` → tolerancia dual, vigente primero [D-03] |
| `scripts/crosslane/host.go` | Modificar | `:119`, ídem |
| `scripts/crosslane/hostopencode.go` | Modificar | `:231-243`, el shim de `$PATH` resuelve ambos nombres [D-03] |
| `.github/workflows/ci.yml` | Modificar | `:266` al binario canónico [D-04]; paso nuevo de humo sobre `cmd/axiom` con su comentario de ventana informativa fechada [D-07] |
| `deploy/telemetry/*.service`, *dashboards* Grafana | Modificar | Nombres de servicio (§5.2) |
| `cmd/axiom/canonical_binary_test.go` | **Crear** | **Cinco** aserciones; la primera compara los dos *switches* como conjuntos, la quinta prohíbe literales de raíz de estado fuera del paquete propietario [D-04, D-03] |
| `internal/backup/manifest.go` | Modificar | `BackupRootFor(home)`, `LegacyBackupRootFor(home)`, `BackupRoots(home)`. Único sitio del árbol con los literales de la raíz de respaldos [D-03, D-12] |
| `internal/cli/sync.go:509`, `internal/cli/run.go:709` | Modificar | Escritores: literal retirado → accesor (E2) [D-03] |
| `internal/update/upgrade/executor.go:488,514` | Modificar | Escritor de snapshot y poda: literal retirado → accesor (E2). La poda pasa a la raíz canónica [D-12] |
| `internal/components/uninstall/service.go:178` | Modificar | Escritor: literal retirado → accesor (E2). `cleaners.go` sigue intocable (V2) [D-10] |
| `internal/dashboard/service.go:1190` | Modificar | Escritor: literal **vigente** → accesor (E3). Escribía bien por el medio equivocado [D-03] |
| `internal/app/app.go:1097-1100` | Modificar | Lector: dos literales → `backup.BackupRoots(homeDir)`. Deja la excepción de la guarda en una sola entrada [D-04] |
| `cmd/axiom/main.go:141` | Modificar | El mensaje del caso vacío nombra las **dos** raíces que `ListBackups` escanea. Arreglo de mensaje, sin maquinaria [D-12] |
| **18 aserciones en 9 ficheros de prueba** (`app_test.go:39,87,151,498`; `dedup_prune_test.go:20,90,158,226`; `backup_metadata_test.go:35,92,139`; `effective_method_routing_test.go:258,403`; `executor_test.go:893`; `restore_test.go:19`; `sync_test.go:1867`; `compatibility_skills_test.go:131`; `compatibility_transaction_windows_test.go:275`) | Modificar | El corpus **replica** el literal retirado en vez de verificarlo; por eso el defecto era verde. 16 pasan al accesor; **2 asertan ausencia y se reapuntan a la raíz canónica** o quedarían vacías [D-12] |
| `internal/absorptionledger/ledger.go` | **Crear** | `Parse([]byte) ([]Row, error)`. Única pieza con conocimiento del formato [D-06] |
| `internal/absorptionledger/ledger_test.go` | **Crear** | Seis aserciones sobre `docs/upstream-absorption-ledger.md` [D-06] |
| `docs/upstream-absorption-ledger.md` | **Crear** | Cabecera con procedencia fechada, reglas de aceptación, recuento y ocho secciones vacías (una por fase) [D-06] |
| `contracts/**` | **Sin cambios** | D2.3 resuelta: no se toca. Criterio de aceptación byte a byte (§10.4) |

### 4.2 F6 — Retirada destructiva de la capa Go de ODD

**F6.1 — Doctrina.**

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/assets/{claude,opencode,kiro,hermes,generic}/persona-axiom.md` | Modificar | Retirar `## Flujo Dual: ODD y SDD` (`:33-38` en `claude/`) |
| `internal/assets/{claude,kimi}/output-style-axiom.md` | Modificar | Ídem |
| `internal/assets/assets_test.go` | Modificar | `axiomODDWorkflowRequired` (`:3067-3074`), `axiomODDWorkflowForbiddenTriggers` (`:3081-…`) y `TestPersonaAxiomAssetsDescribeODDWorkflowDeterministically` (`:3111-…`) apuntan al protocolo de upstream |
| `internal/components/agentguidance/routing.go` | Modificar | Punto de inyección del protocolo ODD de upstream. **Hoy tiene cero menciones de ODD**, verificado |

**F6.2 — Superficies de usuario.**

| Fichero | Acción | Descripción |
|---|---|---|
| `cmd/axiom/main.go` | Modificar | Importación (`:25`); ayuda (`:95-97`, `:123`); `case "odd"` (`:356-357`); `runODD` (`:1789-1823`); `newDashboardScaffolder` y `dashboardScaffolder` (`:1825-1866`) |
| `cmd/axiom/main_test.go` | Modificar | Casos que ejercitan `runODD` |
| `internal/cli/odd_{create,promote,status}.go` + sus 3 `_test.go` | **Borrar** | 6 ficheros |
| `internal/tui/screens/odd_features.go` | **Borrar** | — |
| `internal/tui/{model.go,router.go,model_test.go}` | Modificar | Los cinco puntos de integración que el diseño de INC-19 enumeró en su §4.5, más la ruta de retroceso |
| `internal/tui/screens/governance.go` | Modificar | Retirar la entrada 6 y recolocar «Volver» |
| `internal/dashboard/{odd_service.go,odd_service_test.go}` | **Borrar** | — |
| `internal/dashboard/server.go` | Modificar | Cuatro rutas (`:58-61`), sus manejadores (`:370-…`) y el campo `oddExporter` (`:28`) |
| `internal/dashboard/types.go` | Modificar | `ODDCreateRequest`, `ODDPromoteRequest`, `ODDMirrorRequest` |
| `internal/dashboard/assets/{index.html,app.js}` | Modificar | Pestaña `tab-odd` y sus funciones |
| `internal/dashboard/dashboard_test.go` | Modificar | Casos de las rutas retiradas |

**F6.3 — Dominio.**

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/odd/**` | **Borrar** | 18 ficheros (9 de producción, 9 de prueba) |
| `internal/dashboard/name_parity_test.go` | **Borrar** | Propiedad de paridad de nombres ODD↔incremento; sin ODD no tiene sujeto |
| `internal/dashboard/{types.go,service.go}` | Modificar **condicional** | `ProposalBody` y su rama solo si ningún otro consumidor los usa. **Medición pendiente** (§9, O-1) |
| `internal/dashboard/create_increment_characterization_test.go` | **Sin cambios** | Protege REQ-15.1, independiente de ODD |
| `odd/tasks/*.md` | **Sin cambios — prohibido** | Son datos [D-02, D-10] |

**F6.4 — Especificaciones y registro.**

| Fichero | Acción | Idioma |
|---|---|---|
| `openspec/specs/odd-{living-document,cli-commands,sdd-promotion,ui-integration}/spec.md` | Delta **destructivo** (15 requerimientos, 33 escenarios) | Español [D-05] |
| `openspec/specs/dashboard-sdd-orchestration/spec.md` | Delta `MODIFIED` sobre el escenario de `:48` | Español [D-05, D-11] |
| `openspec/specs/organic-agent-trigger-rules/spec.md` | Delta firme (ya no condicional) | **Inglés** [D-05, D-11] |
| `docs/upstream-absorption-ledger.md` | 7 filas de F6 | Español |

### 4.3 F1 — Ruta de módulo (última de la cadena)

| Conjunto | Acción | Descripción |
|---|---|---|
| `go.mod` + todo `*.go` que importe `github.com/gentleman-programming/gentle-ai/v2` | Modificar | **Derivado**, no enumerado: `rg -l 'gentle-ai/v2' -g '*.go'` menos las rutas prohibidas. Una línea por fichero |
| `.goreleaser.yaml:29` | Modificar | `ldflags` con la ruta de módulo. **No citado en la propuesta** |
| `scripts/install.sh:287-289` | Modificar | `GONOSUMDB`, `GOPRIVATE`, `GONOPROXY` |
| `scripts/install.ps1:32,101-103` | Modificar | `go install …@latest` y los tres patrones de entorno |
| `README.md`, `docs/{quickstart,platforms,release-signing}.md`, `TRADEMARKS.md` | Modificar | Documentación |
| `openspec/changes/archive/**`, `docs/releases/**`, `odd/tasks/*.md` | **Prohibido** | Citas históricas, no importaciones [D-10] |

Verificación específica de F1: `gofmt -l` **solo sobre su propio conjunto**, no sobre el árbol (R14: `gofmt -l .` ya señalaba 18 ficheros no canónicos preexistentes).

---

## 5. Interfaces y contratos

### 5.1 La guarda de binario canónico

```go
package main

// canonicalBinaryPath es el único objetivo de construcción del producto.
// Toda referencia a ./cmd/gentle-ai en un objetivo de construcción es un
// defecto, no una alternativa: ese paquete es un shim de deprecación cuyo
// main() escribe un aviso en stderr y delega en app.RunArgs, de modo que
// carece de toda la capa de producto del fork.
const canonicalBinaryPath = "./cmd/axiom"

// deprecatedShimPath se nombra para poder rechazarlo, no para usarlo.
const deprecatedShimPath = "./cmd/gentle-ai"

// buildTargetException documenta por qué un sitio concreto puede seguir
// nombrando el shim. Sin motivo escrito, la guarda falla.
type buildTargetException struct {
	File   string // ruta relativa a la raíz del repositorio
	Reason string // por qué el shim es correcto aquí; nunca vacío
}

// canonicalDispatchVerbs extrae los literales de las cláusulas case del switch
// de despacho de nivel superior de un fichero Go, con go/parser + go/ast.
func canonicalDispatchVerbs(t *testing.T, file string) map[string]struct{}
```

### 5.1.bis El accesor de la raíz de respaldos

Los cinco escritores reciben `homeDir` **como parámetro**, mientras que `backupRoot()` (`manifest.go:171-177`) resuelve el home por su cuenta con `os.UserHomeDir()`. Por eso «llamad a la función que ya existe» no compila como arreglo: hace falta la variante que toma el home. Ese desajuste de firma es, literalmente, por qué los cinco escritores se construyeron su propio literal.

```go
package backup

// BackupRootFor devuelve la raíz canónica de respaldos bajo home.
// Es el ÚNICO origen válido de esa ruta para un escritor [D-03].
func BackupRootFor(home string) string

// LegacyBackupRootFor devuelve la raíz retirada bajo home. Existe para la
// lectura y para la validación de contención; ningún escritor la invoca.
func LegacyBackupRootFor(home string) string

// BackupRoots devuelve ambas raíces, VIGENTE PRIMERO. Ese orden es el contrato:
// un lector que las recorra en otro orden puede resolver un ID duplicado hacia
// la copia retirada. Es el único punto del árbol donde conviven las dos grafías.
func BackupRoots(home string) []string
```

`backupRoot()`, `legacyBackupRoot()` y `BackupRootFn` se conservan como envoltorios sin argumentos: los tests de `internal/backup` los sobreescriben en nueve sitios (`manifest_test.go:433-435`, `restore_test.go:16-22,77-83,154-160,358-360,426-428`, `retention_test.go:340-342,376-378,477-479`, `snapshot_dir_fsync_test.go:17-22`) y romper esa costura no aporta nada a este incremento. **Los nueve están dentro de `internal/backup`**, así que el comentario de `manifest.go:188-190` que justifica la exportación por un consumidor externo se corrige a la vez [D-12].

Migración de llamadores, cinco escritores y un lector:

| Sitio | Pasa a |
|---|---|
| `internal/cli/sync.go:509` | `backup.BackupRootFor(homeDir)` |
| `internal/cli/run.go:709` | `backup.BackupRootFor(homeDir)` |
| `internal/update/upgrade/executor.go:488,514` | `backup.BackupRootFor(homeDir)` |
| `internal/components/uninstall/service.go:178` | `backup.BackupRootFor(homeDir)` |
| `internal/dashboard/service.go:1190` | `backup.BackupRootFor(home)` — hoy escribe la grafía correcta por el medio incorrecto (E3) |
| `internal/app/app.go:1097-1100` | `backup.BackupRoots(homeDir)` — deja la lista de excepciones de la guarda en **una sola entrada** |

La aserción de raíz de [D-04] es una contención de conjuntos:

```go
app   := canonicalDispatchVerbs(t, "../../internal/app/app.go")
axiom := canonicalDispatchVerbs(t, "main.go")
for verb := range app {
    if _, ok := axiom[verb]; !ok {
        t.Errorf("el verbo %q es alcanzable por el shim deprecado y no por el binario canónico", verb)
    }
}
```

### 5.2 El parseador del registro de absorción

```go
// Package absorptionledger conoce el formato de docs/upstream-absorption-ledger.md
// y nada más. No habla con git, no habla con la red, no habla con Engram: su
// única entrada son los bytes del registro. Esa frontera es deliberada — lo que
// el registro AFIRMA se verifica aquí; que lo afirmado sea CIERTO es trabajo de
// RA-1, por tanda y en revisión.
package absorptionledger

// State es el estado de absorción de un commit de upstream. Los valores se
// escriben literalmente en el registro y los lee un humano, así que van en
// castellano; el identificador Go va en inglés (precedente: internal/odd.Status
// en el diseño de INC-19, D-14).
type State string

const (
	StateAbsorbed            State = "absorbido"
	StateDeliberatelyDropped State = "descartado-deliberadamente"
	StateReverted            State = "revertido"
)

// Row es una fila del registro: un commit de upstream y su disposición.
type Row struct {
	SHA      string // sha corto o largo de upstream
	Subject  string
	Phase    string // F0..F7
	State    State
	Evidence string // referencia de PR o de commit del fork
	Reason   string // obligatorio salvo con StateAbsorbed
}

// Ledger es el registro completo, con su recuento declarado.
type Ledger struct {
	MeasuredOn     string // fecha de la medición, AAAA-MM-DD
	CommonAncestor string
	UpstreamHead   string
	// Universe es el número de commits sin merge que el registro DECLARA cubrir,
	// leído de su cabecera. No es una constante del paquete a propósito: crece
	// cada vez que upstream avanza (R8), y una constante compilada obligaría a
	// tocar Go en cada reconciliación y garantizaría que el registro naciera sin
	// cuadrar. Se mide con `git rev-list --count --no-merges 266574b0..upstream/main`
	// al abrir el registro; `--no-merges` es obligatorio (propuesta §2.2, §10.2).
	Universe       int
	Rows           []Row
	DeclaredCounts map[State]int // la tabla "Recuento", para contrastarla
}

// Parse decodifica el registro. Devuelve un error envuelto con %w sobre un
// centinela por cada clase de defecto de forma, para que el test pueda
// distinguirlas con errors.Is.
func Parse(raw []byte) (*Ledger, error)

var (
	ErrUnknownState    = errors.New("estado de absorción no reconocido")
	ErrMissingReason   = errors.New("un estado distinto de absorbido exige motivo escrito")
	ErrCountMismatch   = errors.New("el recuento declarado no cuadra con las filas")
	ErrUniverseMismatch = errors.New("el número de filas no cubre el universo medido")
	ErrDuplicateSHA    = errors.New("sha repetido en el registro")
)
```

Los errores se envuelven con `%w` en todo punto de propagación, conforme a `skills/axiom-idiomatic-error-wrapping`.

### 5.3 El contrato de la ventana informativa de `cmd/axiom`

No es una interfaz de código. Es una forma de comentario, y su contrato es que **no puede existir sin fecha ni sucesor**:

```yaml
# Ventana informativa — cobertura de cmd/axiom (INC-20 F0, abierta 2026-09-XX)
#
# Superficie roja inventariada en esta fecha:
#   <verbo>: <síntoma observado>
#
# Sucesor: INC-21 (docs/ROADMAP.md). Esta ventana no se amplía: todo síntoma
# nuevo pertenece al sucesor, no a esta lista.
- name: Smoke the canonical binary surface
```

La capacidad `axiom-binary-ci-coverage` debe exigir esas tres cosas —fecha, inventario, sucesor nombrado— como requerimiento, no como convención (§10.5).

---

## 6. Estrategia de pruebas

`strict_tdd: true` (`openspec/config.yaml:16,46`): RED observado antes de implementar, GREEN, refactor, con evidencia por unidad de trabajo.

| Capa | Qué se prueba | Enfoque | Fase |
|---|---|---|---|
| **Unidad** | `internal/app` ⊆ `cmd/axiom` como conjuntos de verbos | `go/parser` + `go/ast` sobre ambos ficheros. RED: el test falla hoy si se retira cualquiera de los cuatro `case` de `main.go:396-415` | F0 |
| **Unidad** | Objetivo de construcción de `.goreleaser.yaml`, de los workflows y del trinquete | Lectura de fichero y comparación de cadena, estilo `assets_test.go`. **RED real hoy**: `.goreleaser.yaml:13` y `ci.yml:266` lo hacen fallar antes de arreglarlos | F0 |
| **Unidad** | Ningún literal de raíz de estado fuera del paquete propietario | `go/ast` sobre `internal/**/*.go` de producción. **RED real hoy**: los seis sitios de [D-03] lo hacen fallar antes de arreglarlos | F0 |
| **Unidad** | Los cinco escritores resuelven por accesor | Afirmar la ruta **contra `backup.BackupRootFor(home)`**, nunca contra un literal reconstruido en el test. **Es el test que faltaba**: los 18 sitios actuales copian la constante de producción en vez de contrastarla, y una copia no puede detectar que el original es el equivocado | F0 |
| **Unidad** | Las dos aserciones de ausencia siguen pudiendo fallar | Reapuntadas a la raíz canónica, sembrar un respaldo indebido ahí y comprobar que el test **falla**. Un control negativo explícito, porque su modo de fallo al migrar no es romperse sino **quedarse vacías** [D-12] | F0 |
| **Integración** | Un respaldo bajo la raíz retirada sigue listándose, restaurándose y borrándose | Sembrar `~/.gentle-ai/backups/<id>` con su manifiesto y ejercitar `ListBackups`, restauración y `DeleteBackup` [D-12] | F0 |
| **Unidad** | Forma y completitud del registro de absorción | Tabla de casos sobre `absorptionledger.Parse`: estado desconocido, motivo ausente, recuento descuadrado, `sha` repetido, universo incompleto | F0 |
| **Unidad** | El registro real cuadra | Un test que lee `docs/upstream-absorption-ledger.md`. RED mientras el registro esté incompleto; **es lo que impide cerrar F7 con filas sin escribir** | F0→F7 |
| **Caracterización** | La compuerta `verify → archive` **antes** de tocar nada en F4 | Mitigación declarada de R6. Se escribe primero, sobre el comportamiento **vigente**, comparando la salida íntegra, no por subcadena (precedente: INC-19 D-03) | F4 |
| **Caracterización** | Los 7 activos de persona, byte a byte, antes de reescribirlos | Congela lo vigente antes de F6.1 para que el diff de doctrina sea legible | F6.1 |
| **Integración** | Las 4 rutas `/api/odd` desaparecen y el resto del dashboard responde igual | `net/http/httptest`, ya disponible (`config.yaml:58-60`) | F6.2 |
| **TUI** | `ScreenODDFeatures` desaparece sin dejar el cursor fuera de rango en Gobernanza | `tea.KeyMsg` directo sobre `model.Update()` (`config.yaml:65-68`). **Riesgo real**: el diseño de INC-19 (D-12, D-13) documenta que `ScreenSDDIncrements` ya tiene aritmética de índices desincronizada; retirar una entrada de Gobernanza la mueve | F6.2 |
| **Ratchet** | Cinco borrados y cero adiciones en `.deadcode-baseline.txt` | `scripts/deadcode-ratchet.sh --update` y lectura del diff [D-09] | F4 |
| **E2E** | `e2e/e2e_test.sh` completo, sin `-run`, al cierre de **cada** fase | RA-2. 49 aserciones sobre la base verde del PR #22 | Todas |
| **Humo de superficie** | Cada verbo exclusivo de `cmd/axiom` responde y **no emite el aviso de deprecación** | Paso de CI nuevo. La segunda aserción es la que distingue el binario del shim [D-07] | F0 |

**Cobertura declarada, no ocultada (RA-2).** El módulo `bench/` es un segundo módulo Go sin `go.work` y **no lo alcanza `go test ./...` desde la raíz**. Las 11 *journeys* en rojo quedan fuera de la evidencia de toda tanda y eso se declara en cada informe de verificación, no se omite. La consecuencia de diseño: `bench/forked_namespace.go` **no puede importar `internal/opencode`**, así que la tolerancia dual de `orchestratorAgentKeys` (`:28`) y la de `managedOpenCodeAgentKeys` (`internal/opencode/config.go:347-352`) son **dos listas que deben mantenerse coherentes a mano**. El propio comentario de `forked_namespace.go:19-21` lo reconoce. Ninguna fase de INC-20 puede tocar una sin leer la otra, y `bench/` es ruta prohibida, así que si F0 cambia una grafía, la incoherencia resultante pertenece al incremento de las *journeys*, no a este. **Se declara, no se resuelve.**

---

## 7. Matriz de amenazas

Aplicable: el diseño modifica enrutado de comandos (`cmd/axiom`), *scripts* de shell, objetivos de construcción, un *shim* ejecutable en `$PATH` e integración de procesos.

| Frontera | Casos adversarios mínimos | Aplicabilidad | Respuesta de diseño | Pruebas RED planificadas |
|---|---|---|---|---|
| **Rutas con aspecto de documentación** | `.goreleaser.yaml`, `.github/workflows/*.yml`, `scripts/install.sh`, `scripts/install.ps1` — ficheros ejecutables por contenido que una clasificación por extensión trataría como configuración inerte; y `docs/upstream-absorption-ledger.md`, un documento pasivo que **un test parsea** | **Aplicable** | La guarda de [D-04] los trata como **objetivos de construcción**, no como documentación: su contenido decide qué binario se publica. El registro se trata como **dato estructurado**: `absorptionledger.Parse` falla cerrado ante forma inesperada en vez de asumir el valor cero | Una por clase: (a) `.goreleaser.yaml` con `main: ./cmd/gentle-ai` ⇒ fallo; (b) workflow con `./cmd/gentle-ai` sin excepción con motivo ⇒ fallo; (c) registro con estado desconocido ⇒ `ErrUnknownState`; (d) registro con recuento descuadrado ⇒ `ErrCountMismatch` |
| **Selección de repositorio Git** | `git -C`, ruta relativa, ruta absoluta, cwd heredado | **Aplicable** | RA-1 exige `git show <sha> --stat`. Toda derivación se toma contra la **raíz del repositorio** y contra el `sha` de `upstream/main` **fijado el 2026-09-18**, nunca contra `cwd` heredado ni contra un remoto recién traído. R8 cierra el universo en la medición fechada: lo posterior es la siguiente reconciliación, no ampliación de alcance | Un caso por selector: derivación tomada desde un subdirectorio debe producir la misma lista que desde la raíz, o la tanda se rechaza |
| **Estado de commit** | Índice preparado, `commit -a`, índice vacío, marcadores de conflicto de un cherry-pick a medias | **Aplicable** | La evidencia de RA-2 se produce sobre **árbol limpio**. Un cherry-pick con conflicto sin resolver deja marcadores que `go build ./...` detecta, pero un conflicto resuelto **a medias** compila: por eso la comprobación de frontera [D-10] y el contraste derivada-vs-diff se ejecutan **antes** de RA-2, no después | Un caso por estado: diff con ruta prohibida ⇒ aborto (§7.3); fichero derivado ausente del diff sin motivo en el registro ⇒ rechazo de la tanda |
| **Estado de *push*** | Rama de seguimiento, primer *push*, *refspec* explícito | **N/A** | Este incremento **no escribe automatización de *push***. Las compuertas de release que F0 toca (`verify-release-assets.sh:11`, `release-preflight.sh:18`, `promote-stable-preflight.sh:18`) son **aserciones de identidad de repositorio**, no selectores de destino: comparan `$GITHUB_REPOSITORY` con un literal y abortan si no coincide. Cambiar el literal no cambia a dónde se empuja | Ninguna. F0 cubre el cambio de literal con el test de identidad de [D-04] |
| **Comandos de PR** | `--head` explícito, prefijo de entorno, comandos compuestos | **N/A** | No se escribe automatización de PR. La entrega es la cadena *stacked-to-main* bajo política ordinaria del repositorio (§4.7), y la `size:exception` de F1 es una acción humana de mantenedor (`skills/gentle-ai-collab-perfect`, regla dura 4) | Ninguna |

Las tres filas `Aplicable` se trasladan **sin modificar** a `tasks.md`, y sus pruebas RED se escriben antes que el código de producción. Las dos `N/A` no generan tarea.

---

## 8. Migración y despliegue

**No hay migración de datos.** Ni esquema persistido, ni formato de fichero de usuario, ni estado en `~/.axiom`. Los 19 incrementos archivados no se tocan [D-10].

| Aspecto | Plan |
|---|---|
| **Ruta de módulo `/v2` → `/v3`** | Sin consumidores instalados por esa vía: el instalador del fork nunca instaló este módulo (§1.1c, R11). El defecto de distribución elimina el riesgo de migración. Se documenta, no se mitiga |
| **Artefacto publicado** | Dos binarios a partir de F0: `axiom` (producto) y `gentle-ai` (pasarela de D2.4). Ningún usuario pierde su nombre de comando [D-08] |
| **Estado de usuario bajo `$HOME`** | Intocado. La lectura sigue siendo dual donde ya lo era [D-03] |
| **`odd/tasks/*.md` y sus espejos Engram** | Intocados. El contrato del documento sobrevive a la retirada del validador [D-02] |
| **Espejo del registro en Engram** | Aditivo, versionado por `topic_key`. No se borra ni se reescribe histórico (§7.4) |
| **Banderas de funcionalidad** | Ninguna. El test de sobreingeniería las rechaza: una bandera para el orden de una cadena de PRs es un estado que nadie retiraría después |

**Criterios de aborto** (§7.3, ampliados por [D-10]). Se detiene el incremento y se revierte al último estado verde si:

- Un diff revierte, total o parcialmente, cualquier entrada de V1–V8.
- Un diff contiene cualquier ruta de la lista prohibida de [D-10].
- Una tanda se declara verificada con `go test -run <patrón>` o sin `e2e/e2e_test.sh` (RA-2).
- Una tanda declara su lista de ficheros sin derivarla de `git show --stat` (RA-1).
- La suite raíz sin filtrar queda en rojo al cierre de una tanda y el fallo no está inventariado como preexistente y ajeno.
- `.deadcode-baseline.txt` gana entradas en una tanda de borrado [D-09].

---

## 9. Preguntas abiertas

- [ ] **O-1 — ¿Tiene `ProposalBody` algún consumidor fuera de ODD?** `internal/dashboard.CreateIncrementRequest.ProposalBody` se introdujo en INC-19 exclusivamente para la promoción ODD. F6.3 lo retira **solo si** nada más lo usa; si `POST /api/increments` lo acepta desde la Web UI o desde un cliente externo, es superficie de API publicada y **permanece**, junto con su test de caracterización. Se resuelve con una medición de una línea al abrir F6.3, no aquí.
- [ ] **O-2 — ¿Se unifican los dos *switches* de despacho?** `internal/app/app.go` y `cmd/axiom/main.go` mantienen dos tablas de verbos y su divergencia es la raíz del patrón de [D-04]. La guarda de contención la **detecta**; unificarlas la **eliminaría por construcción**. Es el arreglo correcto y es incremento propio: reescribe el despacho de dos binarios, y hacerlo en medio de esta reconciliación contradice el criterio de §4.4 (medir es barato, reparar no).
- [x] **O-3 — ¿Cuántos commits exigen reconciliación manual de importaciones con F1 al final? — RESUELTA.** **Medida por el orquestador el 2026-09-19** sobre `upstream/main` recién traído: `git log --oneline -G'gentle-ai/v3' 266574b0..upstream/main -- '*.go'` y su equivalente con `-S` devuelven ambos **6**, de los cuales `2594581e` es la propia migración ⇒ **5 commits de impuesto recurrente**, enumerados en [D-01]. La cifra **confirma** [D-01] con el universo ya ampliado: el coste es absoluto, no proporcional. Ningún reajuste de orden es necesario.
- [ ] **O-4 — ¿Cuál es el tamaño real de la superficie roja de `cmd/axiom`?** Desconocido por construcción: son 1919 líneas nunca ejercitadas. F0 lo convierte en medible. El inventario se escribe cuando el paso corra por primera vez, no antes [D-07].

---

## 10. Estado de las mediciones

Se declaran en vez de estimarse. Dos de ellas las **cerró el orquestador el 2026-09-19** —tenía shell, esta fase no— y quedan marcadas como resueltas con su procedencia.

| Medición | Por qué no se hizo | Efecto sobre el diseño |
|---|---|---|
| Recuento de líneas de las superficies ODD (las ~5073 de §9 de la propuesta) | La herramienta de shell no está disponible en esta sesión | Ninguno: [D-02] ordena la retirada por **dependencia**, no por tamaño. El recuento importa para el pronóstico de `sdd-tasks`, no para el orden |
| `git show <sha> --stat` de cualquier commit de upstream | Ídem. Ninguna tabla de este documento enumera ficheros de una tanda de absorción, así que no hace falta — y enumerarlos habría violado RA-1 | Ninguno |
| ~~Re-medición de los commits que mencionan `/v3`~~ | **RESUELTA 2026-09-19 por el orquestador**, no por esta fase | O-3 cerrada. 6 commits, 5 de impuesto recurrente. Incorporada a [D-01] con su comando reproducible |
| ~~Verificación de que el universo medido el 2026-09-18 sigue vigente~~ | **RESUELTA 2026-09-19 por el orquestador**: `git rev-list --count 266574b0..upstream/main` → **87** | **No vigente, y por eso el universo se parametrizó** [D-06]. Ver la nota de métrica justo debajo |
| El universo **sin merges** vigente hoy | La medición recibida es `rev-list --count` **sin** `--no-merges` | Ninguno sobre el diseño: [D-06] ya no escribe ninguna constante. El número lo resuelve F0 al abrir el registro |
| Ejecución de las *journeys* `j117`/`j118` tras migrar los escritores de respaldos | `bench/` es módulo aparte y ruta prohibida [D-10]; además no lo alcanza `go test ./...` | **Ninguno, verificado por lectura.** `bench/journeys_issue_3561.go:114-119` y `journeys_issue_3557.go:100-105` asertan la **ausencia** de `~/.gentle-ai/backups` tras `doctor` («doctor unexpectedly created»). Migrar los escritores hace esa aserción **más** fácil de cumplir, no menos. Quedan fijando una grafía muerta, lo que las debilita como test: seguimiento para el incremento de *bench*, no bloqueo aquí |
| Conteo exacto de `case` en `internal/app.RunArgs` frente a `cmd/axiom/main.go` | Se leyeron ambos *switches* y se verificó la **contención rota** (los cuatro verbos del hallazgo 3 y la ausencia de `odd`, `init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui` en `internal/app`). El conteo exacto lo produce el propio test | Ninguno: es lo que la guarda calcula en tiempo de ejecución |

### 10.1 Nota de métrica sobre el universo: 87 **no** es el sucesor de 55

La medición del 2026-09-19 devolvió `git rev-list --count 266574b0..upstream/main` → **87**. Esa cifra **incluye merges**. La cifra comparable de la propuesta, medida el 2026-09-18, es **77**, no 55:

| Fuente | Medida | Valor |
|---|---|---|
| `proposal.md:32` | commits de upstream ausentes, **con** merges | 77 |
| `proposal.md:85` | commits de merge excluidos a propósito | 22 |
| `proposal.md:71,195,471` | universo del registro, **sin** merges | 55 (= 77 − 22) |
| Orquestador, 2026-09-19 | `rev-list --count`, **con** merges | **87** |

El crecimiento real de upstream entre ambas fechas es por tanto de **87 − 77 = 10 commits totales**, de los cuales un número aún no medido son merges. **El universo sin merges vigente hoy no se ha medido**: está acotado en el intervalo `[55, 65]`, pero el valor exacto lo devuelve `git rev-list --count --no-merges 266574b0..upstream/main`.

Esto **no debilita la instrucción de parametrizar; la refuerza**. Escribir `87` como universo del registro habría sustituido una constante equivocada por otra, y con un defecto peor: el registro cuenta filas sin merge por construcción (§2.2 excluye la topología de historia; §10.2 hace del recuento sin merges un criterio de aceptación), de modo que un total tomado con merges **no puede cuadrar jamás** — que es exactamente el fallo que la parametrización venía a evitar.

Consecuencia operativa, y es la única: la primera tarea de F0 ejecuta el comando **con `--no-merges`**, escribe `<N>` y la fecha en la cabecera del registro, y el validador de [D-06] comprueba desde entonces la coherencia interna del documento contra ese valor declarado.
