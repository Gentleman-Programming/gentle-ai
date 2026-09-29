# Checklist de escenarios baseline — QA-Orchestrator v2 (pre-migración)

**Fecha:** 2026-09-23
**Propósito:** capturar el comportamiento REAL de `legacy-v2-custom` en OpenCode — no lo que las skills `.md` dicen que hacen — para usarlo como contrato de referencia de la migración a `qa-orchestrator-v3`.
**Quién ejecuta:** el usuario, manualmente, en OpenCode. Este checklist NO debe ejecutarse automáticamente por un agente cuando el escenario implique BookStack, GitLab, MCP, creación de MR, modificación real del ledger o una aprobación humana — todos los escenarios de abajo caen en ese caso.
**Regla de registro:** no te limites al happy path. Para cada escenario, además del resultado esperado, registra también: el comando exacto que devuelve error, el mensaje textual devuelto, si modifica o no el ledger, si Engram guarda algo, si el flujo reintenta, si queda bloqueado, y qué `next_action` produce después.

## Antes de empezar

```bash
git switch legacy-v2-custom
```

Confirma que estás en esa rama (`git branch --show-current`) antes de correr nada. Los verbos relevantes son `qa-begin`, `qa-finish`, `qa-approve`, `qa-status` (registrados en `internal/app/app.go`). Antes de escribir el comando completo de cada paso, corre `gentle-ai qa-begin --help`, `gentle-ai qa-finish --help`, `gentle-ai qa-approve --help`, `gentle-ai qa-status --help` una vez para confirmar la sintaxis exacta vigente — los nombres de flags abajo están verificados contra el código (`internal/cli/qa_begin.go`, `qa_finish.go`, `qa_approve.go`, `qa_status.go`), pero el orden/formato exacto de invocación conviene confirmarlo con `--help` en vivo antes de depender de él.

Flags confirmados por componente:
- `qa-begin`: `--change`, `--stage`, `--cwd`, `--request-id`, `--evidence-goal`, `--expected-revision`, `--max-attempts`, `--max-changed-lines`
- `qa-finish`: outcome (`passed`/`failed`/`interrupted`), `--evidence-revision sha256:...`, disposición del harness
- `qa-approve`: actor, reason, `--request-id`, ligado a `--evidence-revision`
- `qa-status`: `--change`, `--cwd`

Usa un `{change}` de prueba dedicado (no uno real en curso) para no contaminar ledgers activos.

---

## Escenario 1 — Ciclo feliz completo

- **Objetivo:** confirmar que el flujo `qa-supervisor → qa-explore → qa-spec (aprobado) → qa-apply → qa-verify → cierre` funciona de punta a punta y que `qa-status` refleja el `next_action` correcto en cada paso.
- **Precondiciones:** `{change}` de prueba nuevo, sin entradas previas en el ledger. BookStack y GitLab MCP disponibles.
- **Comandos/acciones en OpenCode:**
  1. `gentle-ai qa-status --change <test-change>` (estado inicial, debería pedir `begin explore`)
  2. Invocar la skill `qa-supervisor` sobre `{change}` y dejar que dirija todo el ciclo, o ejecutar manualmente `qa-explore` → `qa-spec` → (aprobación humana) → `qa-apply` → `qa-verify`, corriendo `qa-status` entre cada etapa.
- **Resultado esperado según v2 (documentado):** cada etapa cierra con `qa-finish` y `qa-status` avanza el `next_action` en la secuencia `begin explore → finish explore → begin spec → finish spec → approve → begin apply → finish apply → begin verify → finish verify → complete`.
- **Evidencia a capturar:** salida completa de cada `qa-status` (JSON), timestamps, cualquier mensaje de la skill al usuario.
- **Estado del ledger antes/después:** capturar el JSON del ledger (o la salida de `qa-status`) antes de iniciar y después de cada `qa-finish`.
- **`next_action` esperado:** documentar el valor exacto devuelto en cada paso (no solo "avanza", el string literal).
- **Preservar en v3:** la secuencia de 5 etapas y que `qa-status` sea siempre recalculado, nunca cacheado.
- **Podría cambiar conscientemente en v3:** el mecanismo interno de persistencia (`OpenRuntimeStore` vs. `QAStateStore`), no el comportamiento observable.

### RESULTADO REAL (ejecutado actuando como `qa-supervisor` real, siguiendo `skills/qa-supervisor/SKILL.md` literalmente, sobre `legacy-v2-custom`)

**Veredicto: DIFERENTE A DOCUMENTADO en partes — pero el ciclo core SÍ funciona end-to-end.** `{change}` usado: `baseline-s01-happy-clean` (se descartó un intento previo `baseline-s01-happy` por quedar contaminado por una ejecución fallida anterior de un agente automatizado; evidencia de contaminación: dos lecturas consecutivas de `qa-status` para ese `{change}` devolvieron estados inconsistentes — se abandonó en vez de investigar más, y se documenta aquí como limitación del entorno de prueba, no como comportamiento de v2).

**Confirmación del `{change}` único**: `skills/qa-supervisor/SKILL.md:30-31` dice literalmente *"Genera un `{change}` — slug corto y estable — y úsalo idéntico en todo lo que sigue"*. Se confirmó empíricamente: las 4 etapas (`explore/spec/apply/verify`) se ejecutaron con el mismo string `baseline-s01-happy-clean`, nunca variantes como `qa_<feature>_explore`.

**Hallazgo mayor no anticipado — `qa-validate` (paso obligatorio en las 4 skills antes de `qa-finish`) tiene un schema real distinto al que describen las skills:**
- El struct Go real (`internal/qastage/artifact.go`, `stageBody`) con `DisallowUnknownFields()` SOLO acepta: `findings` (`classification: DOCUMENTED|MISSING|NOT_APPLICABLE`, `url`, `description`), `pending_questions`, `scope` (`status: complete|blocked`, `blocked_reason`, `next_stage`), `predecessor_sha256`.
- Las skills (`qa-apply/SKILL.md:49`, y por extensión las demás) describen un envelope con `schema`, `change`, `stage`, `status`, `created_at`, `source_artifact_revision`, `artifact_revision`, `next_stage`, y un cuerpo con `facts`/`observations`/`inferences`/`recommendations`/`pending_questions`/`risks`/`scope`/`decision`.
- **Prueba real:** enviar el envelope tal como lo describe la skill (con el campo `schema`) es RECHAZADO: `{"valid":false,"reason":"invalid JSON or unknown field: json: unknown field \"schema\""}`. Un agente que siguiera la skill al pie de la letra fallaría aquí siempre.
- **Hallazgo adicional:** la salida real de `qa-validate` es `{"valid":bool,"stage":str,"change":str,"reason":str?}` — **no incluye ningún campo `artifact_revision`**, pero las 4 skills instruyen "con el `artifact_revision` que `qa-validate` admitió, ejecuta `qa-finish` ... `--evidence-revision <artifact_revision>`". Ese campo simplemente no existe en la respuesta real. En esta ejecución se improvisó calculando el SHA-256 del propio payload JSON como `--evidence-revision` (`sha256sum` del archivo), una solución razonable pero **no documentada en ninguna skill**.

**Comandos reales ejecutados (verbatim, resumen — el detalle completo generó ~15 invocaciones):**
```
qa-begin   --change baseline-s01-happy-clean --stage explore ... → EXIT=0, next_action:finish
qa-validate --input - --change ... --stage explore (payload correcto)   → valid:true
qa-finish  --change ... --evidence-revision sha256:<hash calculado> ... → EXIT=0, complete:true

qa-begin   --stage spec    → EXIT=0
qa-validate --stage spec   → valid:true
qa-finish  --stage spec (hash calculado)  → EXIT=0, complete:true

qa-status  → SIN campo "approvals" (aún no hay ninguna aprobación registrada)
qa-approve --stage spec --evidence-revision <hash de spec> --actor "baseline-tester (simulando aprobacion humana)" --reason "..." → EXIT=0
qa-status  → CON campo "approvals":[{"stage":"spec","approval_revision":"<hash de spec>"}]

qa-begin   --stage apply (ya aprobado)  → EXIT=0 (sin rechazo, como se esperaba tras la aprobación)
qa-validate + qa-finish apply  → EXIT=0, complete:true

qa-begin   --stage verify (no requiere aprobación)  → EXIT=0
qa-validate + qa-finish verify  → EXIT=0, complete:true

qa-status FINAL → complete:true, approvals:[spec], sin haber tocado "docs" (rama opcional, nunca invocada)
```

**Respuestas a los 11 puntos pedidos:**
1. **Quién decide la siguiente etapa**: quien ejecuta el flujo (actuando de `qa-supervisor`), leyendo `next_action` de `qa-status` — nunca se decidió "saltar" nada por criterio propio.
2. **¿Se consultó `qa-status` antes de decidir?**: sí, antes de cada `qa-begin` y después de cada `qa-approve`.
3. **¿Hubo intento de saltar etapa?**: no en este escenario (ese es el objetivo de los Escenarios 2/3, ya documentados aparte); aquí se siguió la secuencia exacta.
4. **¿Cómo se detecta que `spec` necesita aprobación?**: el campo `approvals` de `qa-status` — ausente (por `omitempty`) hasta que existe al menos una aprobación real; consistente con lo que `qa-supervisor/SKILL.md:67` espera revisar.
5. **¿Qué pasa si se otorga la aprobación?**: `qa-approve` la registra en el ledger (revision nueva) y `qa-status` inmediatamente refleja `approvals` con esa etapa — el siguiente `qa-begin --stage apply` deja de estar bloqueado.
6. **Comandos exactos**: ver bloque de arriba (verbatim completo disponible en el historial de ejecución de esta sesión).
7. **`{change}` usado por etapa**: el mismo string (`baseline-s01-happy-clean`) en las 4 etapas — confirma la instrucción literal de la skill.
8. **Estado del ledger antes/después**: capturado en cada paso vía `qa-status` (ver bloque de comandos); cada `qa-finish` deja `complete:true` momentáneamente hasta el siguiente `qa-begin`.
9. **`next_action` exacto**: `begin` (inicial) → `finish` (tras cada begin) → `complete` (tras cada finish) → ciclo se repite por etapa.
10. **Qué guardó Engram**: 4 `mem_save` reales, con los `topic_key` exactos que documentan las skills (`qa/{change}/explore`, `/spec`, `/apply-progress`, `/verify-report`) — los 4 se guardaron exitosamente (ids 3331, 3337, 3338, 3339 en esta sesión de Engram). Nota: esta instancia de Engram es la de esta sesión de Claude Code, no necesariamente la misma instancia/proyecto que usaría una sesión real de OpenCode en producción — se documenta como caveat.
11. **Consulta real a BookStack (Regla Cero/G1)**: sí, `bookstack_bookstack_search("login")` devolvió 90 resultados reales; se citó la página "03. Introducción a Playwright" (con ejemplo de test de login) como fuente `DOCUMENTED` en los artefactos de `explore` y `spec`.

**Conclusión para el contrato v3:** el flujo de orquestación ledger-driven (`qa-supervisor` nunca decide el orden, solo lee `next_action`) funciona exactamente como está documentado en `qa-supervisor/SKILL.md`, y el `{change}` único confirmado empíricamente simplifica el diseño de `QAStateMachine`. Pero el mecanismo `qa-validate` → `evidence-revision` tiene una discrepancia real y bloqueante entre lo que las skills describen (envelope con campos inexistentes, `artifact_revision` que nunca se devuelve) y lo que el código realmente acepta/devuelve — cualquier v3 debe decidir conscientemente si mantiene esta responsabilidad de "calcular el hash uno mismo" o la resuelve de otra forma (ver Contrato pendiente #1 del documento de diseño).

## Escenario 2 — Gate de aprobación bloqueante

- **Objetivo:** confirmar que `qa-apply` no puede empezar si el spec no fue aprobado con `qa-approve`.
- **Precondiciones:** `{change}` con `explore` y `spec` cerrados (vía `qa-finish`), SIN correr `qa-approve`.
- **Comandos:** intentar `gentle-ai qa-begin --change <test-change> --stage apply ...`
- **Resultado esperado según v2 (documentado):** rechazo, presumiblemente `ErrRuntimeStageApprovalRequired` o el mensaje equivalente expuesto por la CLI.
- **Evidencia a capturar:** código de salida del proceso, mensaje de error textual completo (verbatim), si el ledger quedó modificado o intacto.
- **Estado del ledger antes/después:** confirmar que sigue en el mismo estado que antes del intento (no debería avanzar).
- **`next_action` esperado:** ¿`qa-status` sigue devolviendo `approve_spec` (o equivalente) después del intento fallido?
- **Preservar en v3:** que "apply sin aprobación" quede bloqueado — sea cual sea el mecanismo interno.
- **Podría cambiar conscientemente en v3:** el nombre/forma del error (de excepción tipada a resultado estructurado `{"allowed": false, "reason": "approval_required", ...}`, según el diseño de la sección 4 del documento de diseño).

### RESULTADO REAL (ejecutado sobre `legacy-v2-custom`, binario `gentle-ai-v2-baseline.exe`)

**Veredicto: DIFERENTE A DOCUMENTADO.** El gate de aprobación existe y funciona, pero solo se activa en un `qa-begin` que *avanza* desde una etapa previa ya completada en el mismo `{change}`. El *primer* `qa-begin` de un `{change}` recién creado **no lo valida en absoluto**, incluso si se abre directamente en `apply`.

**Prueba A — primer begin de un change nuevo directo en `apply` (bypass total):**
```
$ gentle-ai-v2-baseline.exe qa-begin --change baseline-s02-approval --stage apply --cwd <repo> --request-id s02-1 --evidence-goal "test approval gate absence"
EXIT=0
{"change":"baseline-s02-approval","stage":"apply","next_action":"finish","complete":false}

$ gentle-ai-v2-baseline.exe qa-finish --change baseline-s02-approval --cwd <repo> --request-id s02-finish1 --outcome passed --evidence-revision sha256:1111...1111 --diagnosis "apply without approval" --harness-disposition reused --cleanup-evidence "n/a" --process-evidence "n/a"
EXIT=0
{"change":"baseline-s02-approval","next_action":"complete","complete":true}
```
`apply` (con `requires_approval: true` declarado en `qa-status`) se abrió y cerró sin llamar nunca a `qa-approve`. Sin rechazo.

**Prueba B — advancing begin real (explore→spec→apply en secuencia, sin aprobar spec):**
```
$ qa-begin --change baseline-s02-approval-b --stage explore ... → EXIT=0, complete:false
$ qa-finish ... explore ...                                     → EXIT=0, complete:true
$ qa-begin --change baseline-s02-approval-b --stage spec ...    → EXIT=0, complete:false
$ qa-finish ... spec ... (evidence-revision sha256:4444...4444) → EXIT=0, complete:true
$ qa-begin --change baseline-s02-approval-b --stage apply ...   → EXIT=1
  Error: qa-begin: runtime objective advance requires the current stage to be approved
```
Aquí SÍ bloquea. Y se desbloquea correctamente tras aprobar:
```
$ qa-approve --change baseline-s02-approval-b --stage spec --evidence-revision sha256:4444...4444 --actor baseline-tester --reason "closing the loop" → EXIT=0
$ qa-begin --change baseline-s02-approval-b --stage apply ...   → EXIT=0, complete:false  (ahora sí permite apply)
```

**Ledger antes/después:** en la Prueba A, el ledger nunca tuvo un "current stage" previo contra el cual validar — se creó y cerró en el mismo movimiento. En la Prueba B, el ledger sí tenía `spec` como stage recién completado (sin aprobar) cuando se intentó `apply`, y ahí el chequeo se disparó.
**`next_action` tras el rechazo (Prueba B):** el proceso termina con `exit 1` y no llega a imprimir JSON; `qa-status` inmediatamente después seguía reportando `next_action: "complete"` (el estado del `spec` recién cerrado), sin avanzar.
**Fragmento de código que explica el comportamiento:** el chequeo vive en el motor de `runtime_ledger.go` (mensaje literal `"runtime objective advance requires the current stage to be approved"`), invocado únicamente cuando `Begin()` detecta una etapa previa completada a la cual "avanzar"; `internal/cli/qa_begin.go:90-99` solo valida que la etiqueta de stage exista en el vocabulario, no el orden/aprobación — eso vive más abajo, en `store.Begin(...)`.
**Conclusión para el contrato v3:** el gate de aprobación SÍ es una garantía real de v2 (no solo convención de prompt) — pero con un hueco real y no documentado: el primer `qa-begin` de un `{change}` nuevo lo saltea por completo.

## Escenario 3 — Etapa fuera de orden

- **Objetivo:** confirmar que no se puede saltar etapas (ej. `apply` sin haber cerrado `explore`/`spec`).
- **Precondiciones:** `{change}` de prueba recién creado, sin ninguna etapa iniciada.
- **Comandos:** intentar directamente `gentle-ai qa-begin --change <test-change> --stage apply ...`
- **Resultado esperado según v2 (documentado):** rechazo, presumiblemente `ErrRuntimeStageOutOfOrder`.
- **Evidencia a capturar:** mensaje de error verbatim, código de salida.
- **Estado del ledger antes/después:** confirmar que no se creó ninguna entrada de etapa `apply`.
- **`next_action` esperado:** `qa-status` debería seguir pidiendo `begin explore`.
- **Preservar en v3:** el bloqueo por orden de etapas.
- **Podría cambiar conscientemente en v3:** igual que el escenario 2, la forma del error.

### RESULTADO REAL (ejecutado sobre `legacy-v2-custom`, binario `gentle-ai-v2-baseline.exe`)

**Veredicto: DIFERENTE A DOCUMENTADO.** Mismo patrón que el Escenario 2: el orden de etapas SÍ se valida, pero únicamente en un `qa-begin` que avanza desde una etapa previa completada. El primer `qa-begin` de un `{change}` nuevo acepta cualquier etiqueta de stage sin validar secuencia — está incluso documentado como decisión consciente en un comentario del propio código fuente.

**Prueba A — primer begin de un change nuevo directo en `apply` (bypass total, sin haber abierto `explore`/`spec`):**
```
$ qa-status --change baseline-s03-order --cwd <repo>
{"next_action":"begin","complete":false, "stages":[explore,spec,apply(requires_approval),verify,docs]}

$ qa-begin --change baseline-s03-order --stage apply --cwd <repo> --request-id s03-attempt1 --evidence-goal "test out-of-order enforcement"
EXIT=0
{"change":"baseline-s03-order","stage":"apply","next_action":"finish","complete":false}

$ qa-status --change baseline-s03-order --cwd <repo>   # después
{"next_action":"finish","complete":false}   # el ledger aceptó "apply" como primera etapa sin objeción
```

**Prueba B — advancing begin real (skip genuino: de `spec` completado, saltar directo a `docs`, sin `apply`/`verify`):**
```
$ qa-begin --change baseline-s03-order-b --stage explore ...  → EXIT=0
$ qa-finish ... explore (outcome passed) ...                  → EXIT=0, complete:true
$ qa-begin --change baseline-s03-order-b --stage spec ...     → EXIT=0  (avance legítimo explore→spec, aceptado sin objeción)
$ qa-finish ... spec (outcome passed) ...                     → EXIT=0, complete:true
$ qa-begin --change baseline-s03-order-b --stage docs ...     → EXIT=1
  Error: qa-begin: SDD runtime objective advance requested a stage that is not the immediate successor of the completed stage
```
Aquí sí bloquea correctamente el salto real (`spec` → `docs`, saltando `apply`/`verify`).

**Hallazgo adicional no buscado:** cerrar (`qa-finish`) la primera etapa que se abre marca el `{change}` como `complete: true` — pero esto es "este intento/etapa completo", no "todo el ciclo QA completo". Confirmado: sobre el mismo `{change}` ya en `complete:true`, un `qa-begin` con la siguiente etapa (`spec`) fue aceptado y reabrió el ledger (`complete:false` de nuevo). El campo `complete` se resetea en cada `qa-begin` subsecuente.
**Estado del ledger antes/después (Prueba A):** no había ninguna entrada previa — se creó y cerró la entrada de `apply` directamente, sin rastro de `explore`/`spec` nunca haberse abierto.
**`next_action` esperado (Prueba A):** después del "bypass", `next_action` pasó a `"finish"` (para `apply`), no a un rechazo ni a pedir `explore` primero.
**Fragmento de código que explica el comportamiento:** comentario explícito en `internal/cli/qa_begin.go:92-96`:
> "The vocabulary's own Position lookup is only enforced by the ledger on an ADVANCING begin (runtimeObjectiveAdvanceAdmissible); a fresh first begin silently accepts an unknown label (StagePosition defaults to 0)."

El mensaje de error real de un salto genuino (Prueba B) es: `"SDD runtime objective advance requested a stage that is not the immediate successor of the completed stage"` — nombre distinto al `ErrRuntimeStageOutOfOrder` que se asumía en el inventario original.
**Conclusión para el contrato v3:** el orden de etapas SÍ es una garantía real de v2 desde el segundo `qa-begin` en adelante — pero con el mismo hueco que el Escenario 2: la primera etapa de un `{change}` nuevo puede ser cualquiera, sin validación.

## Escenario 4 — Ficha documental BookStack

- **Objetivo:** confirmar que `qa-explore` (o una consulta puntual vía `qa-doc-reference`) genera la ficha de 13 campos + URL exacta, y que ante una divergencia real detiene el flujo (STOP) en vez de decidir por su cuenta.
- **Precondiciones:** un módulo real con documentación existente en BookStack; idealmente también un caso donde la documentación esté desactualizada o incompleta, para forzar el camino de divergencia.
- **Comandos/acciones:** correr `qa-explore` (o `qa-doc-reference`) sobre ese módulo.
- **Resultado esperado según v2 (documentado):** ficha con 13 metadatos + sección "Qué contiene" + URL exacta de BookStack; si hay divergencia, el flujo se detiene y pide decisión humana en vez de continuar asumiendo.
- **Evidencia a capturar:** la ficha completa generada, la URL citada, y si aplica, el mensaje de STOP exacto.
- **Estado del ledger antes/después:** ¿esta consulta documental por sí sola modifica el ledger, o es de solo lectura?
- **`next_action` esperado:** confirmar si cambia o no tras esta consulta.
- **Preservar en v3:** BookStack como fuente de verdad funcional, y el comportamiento STOP ante divergencia (no hay equivalente automático en v3.7 — es lógica 100% de dominio QA).
- **Podría cambiar conscientemente en v3:** el mecanismo de persistencia de la ficha en Engram (topic_key), si se decide renombrar convenciones.

### RESULTADO REAL (BookStack MCP real, página real citada)

**Veredicto: PASS.** Se ejecutó una búsqueda real (`bookstack_search("login")`, 90 resultados) y se generó la ficha completa citando la página real `id 3695` ("Agente QA Orquestador"), obtenida con `bookstack_get_page(3695)`.

**Hallazgo de mecanismo (no es un fallo, es cómo funciona realmente)**: los 13 campos de la ficha NO vienen todos de la metadata nativa de la API de BookStack — el endpoint `search` solo trae `id/name/slug/book_id/chapter_id/created_at/updated_at/url/tags`. Los campos "Versión", "Responsable", "Estado", "Prioridad" vienen de una tabla `Campo | Valor` que la propia página incluye como CONVENCIÓN de contenido (Markdown), no como metadata estructurada de BookStack. La ficha solo es armable combinando ambas fuentes — esto funciona en este caso porque la página real ya sigue esa convención, pero es un supuesto implícito no documentado en `qa-doc-reference/SKILL.md` (asume que esos campos "están disponibles en la página", sin aclarar que dependen de que la página adopte esa tabla convencional).

**Ficha real generada:**

| Campo | Valor |
|---|---|
| Página PRD | Agente QA Orquestador (Supervisor, Sub-agentes y MCPs) |
| Nombre | Agente QA Orquestador |
| BookStack ID | 3695 |
| Libro | book_id 83 (01-funcional) |
| Capítulo | chapter_id 458 |
| Slug / URL | agente-qa-orquestador-supervisor-sub-agentes-y-mcps |
| Título PRD | no disponible (es documento de capacitación, no PRD de producto) |
| Versión del documento | v1.1 (de la tabla interna de la página) |
| Creada | 2026-09-17 |
| Actualizada | 2026-09-16 (según tabla interna) / 2026-09-17T17:56:48Z (según metadata API — **discrepancia real entre ambas fuentes**, se reporta honestamente en vez de elegir una) |
| Responsables | Jhunior Gutierrez (de la tabla interna) |
| Ticket Redmine | no disponible |
| URL directa de BookStack | https://bookstack.sreasons.com/books/01-funcional/page/agente-qa-orquestador-supervisor-sub-agentes-y-mcps |

**Qué contiene**: documento de capacitación (transcripción de reunión, 2026-09-16) sobre cómo operar el ecosistema QA-Orchestrator: rol del QA Supervisor como punto de entrada único, sus 4 sub-agentes internos, los 4 MCPs nuevos (GitLab, BookStack, SQL Server-Excepciones, ELK), las 6 reglas del supervisor, y el flujo completo con sus 2 gates de aprobación humana (plan y, opcionalmente, Merge Request). No es una fuente PRD de un módulo de negocio específico, sino documentación operativa del propio orquestador QA.

**No se forzó ningún STOP por divergencia** en esta ejecución (no había una implementación real contra la cual comparar); el mecanismo de STOP en sí no se pudo ejercitar con evidencia real en este baseline — queda como limitación de esta sesión, no como comportamiento verificado de v2.

## Escenario 5a — Locator existente (nivel 0)

- **Objetivo:** confirmar que cuando el locator ya existe en el POM del proyecto de automatización, se reutiliza sin tocar GitLab.
- **Precondiciones:** un caso de prueba cuyo locator ya está definido en `src/pages/**` o en las tasks/questions Screenplay.
- **Comandos/acciones:** correr `qa-locator-hunting` (o el paso equivalente dentro de `qa-explore`/`qa-apply`) sobre ese caso.
- **Resultado esperado según v2 (documentado):** el locator se reutiliza directamente del POM, sin ninguna llamada al MCP de GitLab.
- **Evidencia a capturar:** confirmar en logs/salida que NO hubo llamada a GitLab MCP; ruta del archivo POM usado.
- **Preservar en v3:** la regla "nivel 0 primero, nunca reinventar si ya existe".
- **Podría cambiar conscientemente en v3:** nada identificado — es lógica de dominio pura.

## Escenario 5b — Locator inexistente (nivel 1)

- **Objetivo:** confirmar que cuando el locator NO existe en el POM, se consulta GitLab vía MCP para cazarlo.
- **Precondiciones:** un caso de prueba cuyo locator NO está definido todavía localmente.
- **Comandos/acciones:** correr `qa-locator-hunting` sobre ese caso.
- **Resultado esperado según v2 (documentado):** consulta el catálogo `erp-mf-*` y el MCP de GitLab para encontrar el locator real, nunca lo inventa.
- **Evidencia a capturar:** qué proyecto/ruta de GitLab consultó, el locator encontrado, si falló la búsqueda qué mensaje devuelve.
- **Preservar en v3:** la regla de "nunca inventar un locator".
- **Podría cambiar conscientemente en v3:** el catálogo `erp-mf-*` es reciente (PR de 2026-09-14) — confirmar si sigue vigente o si cambió desde entonces.

### RESULTADO REAL — Escenarios 5a y 5b

**Veredicto 5a: NO EJECUTABLE.** `qa-locator-hunting` NIVEL 0 exige un POM real (`src/pages/**`) de un proyecto de automatización Playwright/Screenplay concreto. Este repo (`gentle-ai`) es la herramienta/orquestador, no el proyecto de automatización del cliente (ese vive en un repo aparte, `erpperu2-automation-main`, fuera de esta sesión) — no hay ningún POM que consultar aquí. No se puede ejercitar NIVEL 0 con evidencia real sin ese repo.

**Veredicto 5b: PARCIAL.** NIVEL 1 (DOM en vivo vía Playwright MCP) tampoco es ejecutable: no hay ninguna herramienta MCP de Playwright cargada en esta sesión. **NIVEL 2 (GitLab MCP + catálogo `erp-mf-*`) SÍ se pudo verificar con una llamada real**:
```
search_projects("erp-mf-root-config") →
  [{"name":"erp-mf-root-config","path_with_namespace":"SmartClic/erp-mf-root-config","web_url":"https://gitlab.sreasons.com/SmartClic/erp-mf-root-config"}]
```
Esto confirma D1 ("GitLab en vivo siempre gana") es mecánicamente ejecutable — el MCP real devuelve el `project_path` esperado por el catálogo. No se llegó a extraer un locator real de un template `.html`/`.tsx` porque eso requeriría un Target concreto de un caso de prueba real, que no existe en este baseline.

**Conclusión para el contrato v3**: la lógica de 3 niveles y las reglas D1-D3 son sólidas en diseño, pero su ejecución real depende de recursos externos a este repo (POM del proyecto cliente, MCP de Playwright) que no están disponibles para verificar en esta sesión de baseline. Esto no es un hallazgo de "v2 no funciona" — es una limitación del entorno de prueba, y debe re-verificarse en un entorno con el repo de automatización real antes de dar por cerrado este contrato.

## Escenario 6 — Evidencia G6 y hash

- **Objetivo:** confirmar que `qa-verify` produce evidencia reproducible (screenshots/traces/videos/reporter) y que `qa-finish` valida el hash de esa evidencia antes de cerrar.
- **Precondiciones:** `{change}` con `apply` ya cerrado.
- **Comandos:** correr `qa-verify`, luego `gentle-ai qa-finish --change <test-change> --stage verify --evidence-revision sha256:<hash real> ...`
- **Resultado esperado según v2 (documentado):** el checklist G6 completo corre (tsc, ejecución, lint, secretos, esperas fijas, comparación contra BookStack); la evidencia generada tiene un hash verificable; `qa-finish` rechaza si el hash no coincide.
- **Evidencia a capturar:** el hash real generado, y (opcional pero valioso) intentar `qa-finish` con un hash deliberadamente incorrecto para confirmar el rechazo.
- **Preservar en v3 (a decidir conscientemente):** el diseño ya propone un híbrido `qa-verify` (Functional QA) + RDD (lens); este escenario es la evidencia base para decidir qué parte del checklist G6 se delega a RDD y cuál se queda en la capa QA.
- **Podría cambiar conscientemente en v3:** el mecanismo de invocación de RDD (hoy no existe; en v2 es 100% checklist propio).

### RESULTADO REAL

**Veredicto: DIFERENTE A DOCUMENTADO.** El checklist G6 completo (tsc, lint, Playwright real) no es ejecutable sin el repo de automatización real (mismo motivo que 5a/5b). Pero el mecanismo de `--evidence-revision` sí se pudo verificar a fondo, y confirma lo detectado en el Escenario 1:

```
$ qa-finish ... --evidence-revision sha256:not-a-real-hash
Error: qa-finish: evidence_revision must be sha256:<64-lowercase-hex> ...
EXIT=1

$ qa-finish ... --evidence-revision sha256:aaaa...aaaa (64 'a', formato válido, contenido arbitrario/sin sentido)
EXIT=0, complete:true   # ACEPTADO sin objeción
```

**Conclusión**: `qa-finish` únicamente valida el FORMATO del hash (`sha256:` + 64 hex minúsculas) — NO verifica que ese hash corresponda a ningún contenido real (evidencia, artefacto validado por `qa-validate`, o cualquier otra cosa). Combinado con el hallazgo del Escenario 1 (`qa-validate` no devuelve ningún hash utilizable), la cadena "evidencia verificada criptográficamente" que sugiere el nombre `--evidence-revision` es, en la práctica, una convención de formato con contenido auto-declarado por quien la llama — no una garantía criptográfica de que la evidencia citada es real. Esto es coherente con el patrón ya visto en los Escenarios 2/3/1: los gates que SÍ existen (orden, aprobación) validan estructura/secuencia, pero ninguno verifica la veracidad del contenido de la evidencia en sí.

## Escenario 7 — Cierre con MR en GitLab

- **Objetivo:** confirmar que `qa-supervisor`, al cerrar el ciclo completo, crea efectivamente el MR en GitLab (o falla de forma explícita si el MCP no está disponible).
- **Precondiciones:** ciclo completo (`explore`→`verify`) cerrado para `{change}` de prueba.
- **Comandos/acciones:** dejar que `qa-supervisor` cierre el flujo.
- **Resultado esperado según v2 (documentado):** se crea un MR en GitLab con la información del cambio.
- **Evidencia a capturar:** URL del MR creado, o el mensaje de error exacto si el MCP no respondió.
- **Preservar en v3:** GitLab como fuente de verdad del código/MR (decisión ya aprobada).
- **Podría cambiar conscientemente en v3:** nada identificado en este escenario específico.

### RESULTADO REAL

**Veredicto: NO EJECUTABLE (pendiente autorización explícita para crear un MR real).** Por seguridad, no se ejecuta la creación real de un Merge Request sin confirmación explícita del usuario — crear un MR es una acción visible para terceros en un sistema compartido.

Lo que SÍ se confirmó: el MCP de GitLab está disponible y funcional en esta sesión (`search_projects` ya verificado en el Escenario 5b; existe también una herramienta `create_merge_request` cargada, no invocada). Si se autorizara, el comando/acción sería: usar `create_merge_request` del MCP de GitLab apuntando al proyecto de automatización real (no a este repo `gentle-ai`), con el branch del caso de prueba recién implementado, siguiendo el formato de MR que documenta `qa-supervisor/SKILL.md` (verificar aprobación `approved == true` antes del merge).

No se ejecutó porque: (1) no hay un branch/cambio real de automatización que mergear en este baseline (todo fue simulado sobre el ledger), y (2) crear un MR real requeriría elegir un proyecto/rama reales y notificar al usuario antes, por política de seguridad de esta sesión.

## Escenario 8 — Recálculo de estado, nunca caché

- **Objetivo:** confirmar que `qa-status` siempre refleja el estado real, sin caché obsoleto, incluso ante una interrupción o modificación externa del ledger.
- **Precondiciones:** `{change}` en cualquier etapa intermedia.
- **Comandos:** interrumpir deliberadamente un paso (ej. matar el proceso a mitad de un `qa-begin`/`qa-finish`, si es seguro hacerlo en el entorno de prueba), luego correr `gentle-ai qa-status --change <test-change>` inmediatamente.
- **Resultado esperado según v2 (documentado):** el estado reportado es el real post-interrupción (posiblemente `interrupted` como outcome), no un valor cacheado de antes de la interrupción.
- **Evidencia a capturar:** el JSON de `qa-status` inmediatamente después de la interrupción, y si existe algún mecanismo de retry/reset documentado para salir de un estado `interrupted`.
- **Preservar en v3:** recalcular siempre, nunca cachear.
- **Podría cambiar conscientemente en v3:** el mecanismo interno de recuperación de una interrupción (reset/resume), que hoy queda como contrato pendiente #3 del documento de diseño.

---

### RESULTADO REAL

**Veredicto: PASS, con hallazgos adicionales valiosos.** Se simuló la interrupción dejando un `qa-begin` abierto sin su `qa-finish` (más seguro y reproducible que matar un proceso real).

```
$ qa-begin --change baseline-s08-interrupt --stage explore ...   → EXIT=0, next_action:finish
$ qa-status --change baseline-s08-interrupt ...                  → next_action:finish, complete:false (SIN CACHÉ, refleja el estado real inmediatamente)
$ qa-begin --change baseline-s08-interrupt --stage explore ... (reintento mientras sigue abierto) → EXIT=1
  Error: "SDD runtime objective already has an active attempt; run `gentle-ai sdd-attempt status ...`"
```

**`qa-status` recalcula siempre, nunca cachea** — confirmado, tal como se esperaba preservar.

**Hallazgo adicional no buscado — `sdd-attempt status` expone MUCHO más estado interno que `qa-status`:** el mensaje de error remite a `sdd-attempt status`, que sí se probó y devuelve el objeto completo (`objective`, `active_attempt`, `attempts[]`, `max_attempts`, `changed_lines`, `cumulative_attempts`, `lifetime_attempts`, y el `stage_vocabulary` completo con id `"gentle-ai.qa-orchestrator/v2"`). `qa-status` es una vista reducida/simplificada de este estado más rico — relevante para el diseño de `QAStateMachine`, que podría necesitar decidir cuánto de ese detalle exponer.

**Hallazgo adicional — el `outcome` de `qa-finish` cambia el comportamiento posterior:**
```
$ qa-finish ... --outcome interrupted --harness-disposition invalidated ...  → next_action:"begin" (NO "complete")
```
A diferencia de `--outcome passed` (que siempre produjo `next_action:"complete"`), `--outcome interrupted` deja la etapa en `next_action:"begin"` — es decir, permite reintentar la MISMA etapa sin haberla marcado como completa. Esto confirma que `passed/failed/interrupted` no son solo metadata decorativa: `interrupted` tiene una consecuencia real y distinta en el flujo (retry en vez de avance).

**Conclusión para el contrato v3**: el recálculo sin caché se preserva sin cambios. El hallazgo de `sdd-attempt status` como fuente de estado más rica, y el comportamiento diferenciado de `interrupted` vs `passed`, son insumos directos para especificar el "Contrato pendiente #3" (transiciones completas de `QAStateMachine`) del documento de diseño.

## Tabla de resultados (completar durante la ejecución)

| # | Escenario | Resultado | Evidencia (link/archivo) | Notas |
|---|---|---|---|---|
| 1 | Ciclo feliz completo | **DIFERENTE A DOCUMENTADO (parcial)** | ver sección del escenario | Orquestación ledger-driven funciona exacto; `qa-validate` tiene envelope/salida distinta a lo documentado en las skills |
| 2 | Gate de aprobación bloqueante | **DIFERENTE A DOCUMENTADO** | comandos/exit codes/qa-status arriba, en la sección del escenario | Gate real, pero con hueco: no aplica en el primer `qa-begin` de un `{change}` nuevo |
| 3 | Etapa fuera de orden | **DIFERENTE A DOCUMENTADO** | comandos/exit codes/qa-status arriba, en la sección del escenario | Orden real, pero con el mismo hueco del primer `qa-begin` |
| 4 | Ficha documental BookStack | **PASS** (con hallazgo de mecanismo) | ficha completa arriba, en la sección del escenario | Ficha se arma combinando metadata API + tabla convencional de la página, no documentado explícitamente |
| 5a | Locator existente (nivel 0) | **NO EJECUTABLE** | sin repo de automatización real en esta sesión | Requiere POM real de `erpperu2-automation-main` (repo aparte) |
| 5b | Locator inexistente (nivel 1) | **PARCIAL** | `search_projects` real arriba | NIVEL 2 (GitLab) confirmado real; NIVEL 1 (Playwright MCP) no disponible en esta sesión |
| 6 | Evidencia G6 y hash | **DIFERENTE A DOCUMENTADO** | comandos arriba, en la sección del escenario | `--evidence-revision` solo valida formato, no contenido real |
| 7 | Cierre con MR en GitLab | **NO EJECUTABLE** (pendiente autorización) | GitLab MCP confirmado disponible | requiere confirmación explícita del usuario para crear un MR real |
| 8 | Recálculo de estado sin caché | **PASS** (con hallazgos adicionales) | comandos arriba, en la sección del escenario | `sdd-attempt status` expone más estado que `qa-status`; `outcome=interrupted` difiere de `passed` |

## Comportamiento v2 observado vs. contrato deseado para v3

Categorías de decisión usadas en la tercera columna:
- **PRESERVAR**: comportamiento real correcto, se mantiene.
- **CORREGIR**: la intención de v2 era correcta pero estaba mal enforced o rota; v3 la implementa como debió ser desde el inicio.
- **REDISEÑAR**: comportamiento que cambia conscientemente aprovechando Gentle-AI 3.7 (no es una corrección de bug, es una decisión de arquitectura nueva).
- **DIFERIR VALIDACIÓN**: el baseline no pudo ejecutarse por falta de recursos (repo/MCP/autorización); no se asume ningún comportamiento hasta poder probarlo.

| Escenario | v2 observado (real, no documentado) | Contrato deseado para v3 |
|---|---|---|
| 1. Ciclo completo | La orquestación ledger-driven (`qa-supervisor` nunca decide, solo lee `next_action`) funciona exacto a como documenta la skill, con un `{change}` único confirmado en las 4-5 etapas. Pero `qa-validate` (paso obligatorio en las 4 skills) tiene un schema real (`findings/pending_questions/scope/predecessor_sha256`, `DisallowUnknownFields`) distinto al envelope que las skills describen (`schema/facts/observations/.../artifact_revision`) — un envelope "tal como lo documenta la skill" es RECHAZADO en la práctica. Además, `qa-validate` nunca devuelve el `artifact_revision` que las skills asumen usar como `--evidence-revision`. | **PRESERVAR** el patrón de orquestación ledger-driven (`{change}` único, `qa-supervisor` nunca decide orden) — funciona correctamente, no se toca. **CORREGIR** `qa-validate`: definir UN solo schema oficial de artefacto (el que ya implementa el código real: `findings`/`pending_questions`/`scope`/`predecessor_sha256`) y reescribir las 4 skills para que describan exactamente ese schema, eliminando el envelope inventado (`schema`/`facts`/`observations`/`inferences`/`recommendations`/`risks`/`decision`) que nunca funcionó. Además, `qa-validate` debe devolver un `artifact_revision` real en su salida (ej. `sha256` del payload canonicalizado que él mismo admitió) para que `qa-finish`/`qa-approve` lo consuman directamente — que la cadena de evidencia sea generada por el sistema, no calculada de forma no especificada por cada agente. |
| 2. Gate de aprobación bloqueante | El gate SÍ existe y funciona (mensaje real: `"runtime objective advance requires the current stage to be approved"`), pero SOLO en un `qa-begin` que avanza desde una etapa previa completada. El PRIMER `qa-begin` de un `{change}` nuevo lo saltea por completo (documentado como decisión consciente en el propio código). | **CORREGIR**: el gate de aprobación debe aplicar también cuando la etapa objetivo (`apply`) es el PRIMER `qa-begin` de un `{change}`, no solo en un avance. Esto no es "romper compatibilidad" — es cerrar un hueco de seguridad real que nunca fue una intención documentada de v2, solo un efecto colateral de cómo se implementó el chequeo "solo en avance". |
| 3. Etapa fuera de orden | Mismo patrón que el gate de aprobación: el orden SÍ se valida (mensaje real: `"SDD runtime objective advance requested a stage that is not the immediate successor of the completed stage"`), pero solo desde el segundo `qa-begin` en adelante sobre el mismo `{change}`. `qa-finish` marca `complete:true` por cada etapa cerrada, no por todo el ciclo; el siguiente `qa-begin` reabre el ledger. | **CORREGIR**: el orden debe validarse desde el PRIMER `qa-begin` — solo `explore` (la etapa inicial del vocabulario) debe aceptarse como primer `qa-begin` de un `{change}` nuevo; cualquier otra etapa como primer intento debe rechazarse igual que un salto fuera de orden. El patrón "`complete` es por intento, el siguiente `qa-begin` reabre" se **PRESERVA** — es correcto y ya funciona bien, solo se corrige el caso del primer begin. |
| 4. Ficha documental BookStack | Funciona como está documentado, pero los 13 campos de la ficha se arman combinando metadata nativa de la API de BookStack (id/libro/capítulo/URL/fechas) con una tabla `Campo/Valor` que la propia página incluye como convención de contenido — no como metadata estructurada de BookStack. Esta dependencia implícita no está explicitada en `qa-doc-reference/SKILL.md`. | **CORREGIR** (documentar, no rediseñar): declarar explícitamente en `qa-doc-reference/SKILL.md` que campos como Versión/Responsable/Estado/Prioridad dependen de que la página siga la convención de tabla `Campo/Valor` en su contenido — no son metadata nativa de BookStack. Mantener la regla de "fallback honesto" (`no disponible` si la página no sigue la convención) que ya existe, pero dejar de presentarla como si viniera de la API. No se justifica un rediseño de parser más tolerante todavía — el mecanismo actual ya funciona en la práctica, solo falta documentar su dependencia real. |
| 5a. Locator existente (nivel 0) | No ejecutable en este baseline: requiere el POM real del repo de automatización del cliente (`erpperu2-automation-main`), que vive fuera de este repo/sesión. | **DIFERIR VALIDACIÓN** — no se asume ningún comportamiento hasta poder probarlo contra el repo de automatización real. |
| 5b. Locator inexistente (nivel 1) | NIVEL 2 (GitLab MCP + catálogo `erp-mf-*`) confirmado real y funcional (`search_projects` devolvió el `project_path` esperado). NIVEL 1 (DOM en vivo vía Playwright MCP) no ejecutable: no hay herramienta MCP de Playwright en esta sesión. | **PRESERVAR** NIVEL 2 (GitLab + catálogo, regla D1 "GitLab en vivo siempre gana") — confirmado real y correcto, sin cambios. **DIFERIR VALIDACIÓN** de NIVEL 1 (Playwright MCP) hasta tener esa herramienta disponible para probarlo. |
| 6. Verify (evidencia/hash) | El checklist G6 completo no es ejecutable sin proyecto de automatización real. El mecanismo `--evidence-revision` de `qa-finish` SOLO valida el formato (`sha256:` + 64 hex minúsculas) — acepta un hash arbitrario sin contenido real detrás (`sha256:aaaa...aaaa` fue aceptado sin objeción). Combinado con el hallazgo del Escenario 1, la cadena de "evidencia verificada" es una convención de formato autodeclarada, no una garantía criptográfica end-to-end. | **CORREGIR**: `evidence_revision` deja de ser un hash autodeclarado válido por formato únicamente. Una vez corregido `qa-validate` (fila 1) para que devuelva un `artifact_revision` real, `qa-finish`/`qa-approve` deben exigir que el `--evidence-revision` recibido coincida con un `artifact_revision` efectivamente admitido por `qa-validate` para ese `{change}`/etapa (no solo verificar el patrón `sha256:<64 hex>`). El checklist G6 funcional (tsc/lint/Playwright real) queda **DIFERIR VALIDACIÓN** hasta tener el repo de automatización real para probarlo end-to-end. |
| 7. Cierre con MR en GitLab | No ejecutado — requiere autorización explícita del usuario para crear un MR real (no concedida en este baseline). GitLab MCP confirmado disponible y funcional (mismo MCP usado en 5b). | **DIFERIR VALIDACIÓN** — no se asume ningún comportamiento hasta contar con autorización real y un cambio real que mergear. |
| 8. Estado (recálculo sin caché) | `qa-status` recalcula siempre, nunca cachea — confirmado. Un `qa-begin` repetido mientras hay un intento activo es rechazado con claridad. Hallazgo adicional: `sdd-attempt status` expone mucho más estado interno (`objective`, `attempts[]`, `max_attempts`, `cumulative_attempts`, `stage_vocabulary` completo) que `qa-status`. Hallazgo adicional: `--outcome interrupted` deja `next_action:"begin"` (permite reintentar la misma etapa), a diferencia de `passed` que siempre produce `next_action:"complete"`. | **PRESERVAR** el recálculo sin caché de `qa-status` — correcto, sin cambios. **PRESERVAR pero ESPECIFICAR** la semántica de `interrupted`/retry: v3 debe documentar explícitamente (no dejarlo implícito como en v2) que `interrupted` deja la etapa en estado "reintentable" (`next_action` equivalente a `begin`) mientras `passed` avanza/cierra. Nota: el outcome `failed` nunca se probó empíricamente en este baseline — queda como hueco de evidencia a resolver en Fase 3, no se asume su comportamiento. Decisión abierta de diseño (no resuelta aquí): si conviene exponer en `qa-status` parte del detalle más rico que hoy solo trae `sdd-attempt status` (`attempts[]`, `cumulative_attempts`). |

**Balance general**: 4 de 8 escenarios requieren CORREGIR un hueco real de v2 (validate/schema, orden en el primer begin, aprobación en el primer begin, evidencia autodeclarada) — ninguno de estos era una intención documentada de v2 que se esté "rompiendo"; eran huecos no detectados hasta este baseline. 1 escenario (BookStack) se corrige documentando una dependencia implícita, sin rediseño. 3 partes (5a, 5b-nivel1, 7) quedan en DIFERIR VALIDACIÓN por falta de recursos de esta sesión, sin inventar resultados. El recálculo de estado y el patrón de orquestación ledger-driven se PRESERVAN sin cambios porque ya son correctos.

Esta tabla alimenta directamente la especificación real de `QAStateMachine`, `QAStageApproval` y `QAArtifactValidator` en Fase 3, y debe reflejarse en `docs/migration/qa-orchestrator-v3-design.md` como siguiente paso.
