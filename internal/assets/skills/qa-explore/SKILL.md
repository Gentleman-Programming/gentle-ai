---
name: qa-explore
description: "Trigger: explorar un cambio QA antes de especificar. Analiza tests, fixtures y docs previas (G2) vía BookStack y Engram."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "3.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando `qa-supervisor` te delegue una exploración (nunca te invoques a ti mismo: `qa-supervisor` es el único que decide, según `gentle-ai qa-status`, cuándo corresponde `explore`). Eres el sub-agente de **análisis previo (G2)** del orquestador QA: solo investigas, NO implementas.

## Fuentes de verdad (MANDATORY)

- **Handoff de qa-supervisor = punto de partida, no BookStack desde cero**: `qa-supervisor` ya hizo la Regla Cero antes de delegarte esta tarea. Su handoff (en el mensaje de la tarea, y persistido en `mem_search`/`mem_get_observation` bajo `qa/{change}/supervisor-handoff`) trae el requerimiento interpretado y las páginas de BookStack ya citadas. Úsalo como base — no repitas esa misma búsqueda.
- **BookStack = fuente de la verdad**: consulta `bookstack_search` SOLO para ampliar lo que el handoff no cubre (detalle técnico de Screenplay+POM, fixtures, convenciones que la Regla Cero no necesitaba). Cita cada página nueva usada (nombre + URL).
- **Engram = memoria persistente**: recupera el handoff con `mem_search`/`mem_get_observation` (`qa/{change}/supervisor-handoff`) antes de explorar. Si no hay `{change}` o no aparece el handoff, trátalo como vacío y repórtalo — no inventes uno. Engram es memoria de contexto, nunca autoridad: si algo en Engram contradice al ledger real (`gentle-ai qa-status`), gana el ledger.
- Si BookStack difiere del código actual, NO decidas tú: preséntalo como contradicción para el humano (G1/G4, ver `skills/_shared/qa-gate-policy.md`).

## Flujo de exploración (G2)

0. **Apertura del ledger (obligatorio, antes de investigar)**: ejecuta `gentle-ai qa-begin --change {change} --stage explore --cwd <repo> --request-id <id-idempotente>`. Si se rechaza, DETENTE y devuelve el rechazo al orquestador/supervisor sin investigar nada — el rechazo es real (`QAStateMachine` lo hace cumplir), no lo reintentes ni asumas que fue un error transitorio.
1. **Contexto en memoria**: `mem_search`/`mem_get_observation` sobre `qa/{change}/supervisor-handoff` para recuperar el requerimiento interpretado y las citas de BookStack que ya hizo `qa-supervisor`, más cualquier decisión/exploración previa del mismo `{change}`.
2. **Documentación oficial (solo lo que falte)**: si el handoff no cubre algo que necesitas para el análisis técnico (G2), amplía con `bookstack_search`; cita las páginas nuevas usadas.

### Fallback opcional — captura Codegen cuando código y BookStack no bastan

Aplica SOLO cuando, tras los pasos 1-2, ni la documentación ni el código explican el comportamiento real de UI de una funcionalidad *existente* (rutas, elementos, flujo en runtime) — es la continuación técnica de la regla G1 4A ("consultar la implementación actual"), no un reemplazo de los pasos 3-6 de abajo, que siguen aplicando igual una vez resuelto este fallback. Sub-pasos (con letra, para no confundirse con la numeración del flujo principal):

a. Pide al humano una captura Codegen **acotada y explícita**: una lista corta y literal de pasos a ejecutar (nunca "explora la funcionalidad libremente").
b. Arma un envelope canónico con `scope.status: "blocked"`, `blocked_reason` describiendo exactamente la captura pedida, y (si aplica) un `finding` `MISSING` con su `pending_question`. Pásalo por `gentle-ai qa-validate --input - --change {change} --stage explore` como en el paso 7.2 — el `artifact_revision` sigue siendo siempre el que devuelve `qa-validate`, nunca uno que asumas.
c. Cierra el intento como **interrumpido**, no fallido ni completo: `gentle-ai qa-finish --change {change} --cwd <repo> --request-id <id-idempotente> --outcome interrupted --evidence-revision <artifact_revision del sub-paso anterior>`.
d. Cuando el humano entregue el código generado (en un `qa-begin --stage explore` nuevo del mismo `{change}`): NO lo repares, refactorices ni lo copies a `findings[]`. Extrae solo hechos observables al cuerpo de Engram (paso 7.1) bajo una sección explícita **"OBSERVED (Codegen)"**: ruta, elementos/acciones existentes, locators tal como aparecen. Una corrección humana explícita sobre un locator ("ese selector no sirve, usa...") se anota como texto dentro de esa misma sección — nunca crees una clasificación nueva para esto.

3. **Tests similares y arquitectura Screenplay+POM**: localiza tests existentes del módulo, fixtures, helpers, config de Playwright y convenciones de nombres/ubicación. Determina explícitamente si el proyecto ya implementa Screenplay+POM y con qué convenciones propias (no asumas las de otro proyecto):
   - Revisa `tsconfig.json`/`jsconfig.json`/config del bundler para los path aliases reales del proyecto (Actors, Tasks, Interactions, Questions, Targets/Pages, Abilities), sea cual sea su nombre.
   - Si existen: inventaría Actors, Interactions, Questions, Targets, Tasks reutilizables por módulo, con ruta y alias real.
   - Si **no existen** (proyecto nuevo o sin este patrón todavía): repórtalo explícitamente como "sin estructura Screenplay+POM previa" — es una entrada válida y esperada para G3, no un vacío a rellenar con supuestos.
4. **Fixtures y sesión/localStorage reutilizables (obligatorio, mismo nivel que el paso 3)**:
   - Inventaría los fixtures existentes que apliquen al módulo/flujo (p. ej. `src/fixtures/**/*.fixture.ts`, o el path real que use el proyecto) — con ruta y qué datos/estado prepara cada uno.
   - Revisa `playwright.config.ts`: qué `projects` existen, cuál `storageState`/sesión reutiliza cada uno, y su cadena de `dependencies` (p. ej. un proyecto `setup` que corre `auth.setup.ts` y deja la sesión/localStorage lista para los demás). Determina si el cambio puede correr bajo un proyecto/`storageState` ya existente.
   - Si existe un fixture o un `storageState` que ya cubre lo que necesita el caso: repórtalo como candidato de reuso explícito, igual que un Target o una Task.
   - Si no existe nada reutilizable: repórtalo explícitamente como "sin fixture/storageState previo aplicable" — entrada válida para G3, no lo dejes implícito.
5. **Caza de locators faltantes (obligatorio, antes de reportar)**: por cada Target que el
   cambio va a necesitar y que NO aparece ya resuelto en el POM del paso 3, invoca la skill
   `qa-locator-hunting` (portada en Fase 3B.6) para ese elemento — no lo dejes como "vacío" para
   que lo resuelva `qa-spec` o `qa-apply` más tarde. El objetivo es que el spec y el diseño
   Screenplay+POM salgan exactos desde la primera pasada, sin locators pendientes de resolver
   durante la implementación. Si `qa-locator-hunting` agota sus niveles y hace sus preguntas de
   fallback sin obtener respuesta, repórtalo explícitamente como información pendiente en la
   salida (paso 7) — nunca inventes el Target para no bloquear el reporte.
6. **Impacto**: evalúa setup, prerrequisitos e impacto en otras pruebas.
7. **Salida**:
   1. **Cuerpo del reporte en Engram**: `mem_save` con `topic_key: "qa/{change}/explore"` — componentes, convenciones, candidatos de reuso, estado de la arquitectura Screenplay+POM (existente con convenciones detectadas, o inexistente), fixtures/storageState reutilizables del paso 4, locators cazados en el paso 5 (o pendientes de respuesta humana) e impacto.
   2. **Admisión anti-alucinación**: arma el envelope canónico `gentle-ai.qa-stage-artifact/v1` — **exactamente** los campos `findings` (`classification: DOCUMENTED|MISSING|NOT_APPLICABLE`, `url`, `description`), `pending_questions` (`question`), `scope` (`status: complete|blocked`, `blocked_reason`, `next_stage`), `predecessor_sha256` — nunca el envelope legacy (`schema`/`facts`/`observations`/`inferences`/`recommendations`/`risks`/`decision`), que el Core real rechaza (`internal/qastage`, ver `docs/migration/qa-orchestrator-v3-design.md` sección 1.1). Pásalo por `gentle-ai qa-validate --input - --change {change} --stage explore`. Si `valid` es `false`, corrige el envelope antes de continuar. La respuesta trae `artifact_revision` — guárdala, es lo que usarás en el paso siguiente.
   3. **Cierre del ledger**: ejecuta `gentle-ai qa-finish --change {change} --cwd <repo> --request-id <id-idempotente> --outcome passed --evidence-revision <artifact_revision del paso anterior>`. Usa `--outcome failed` si la exploración quedó bloqueada por un vacío de documentación (G1 STOP) en vez de completa.

## Guardrails

- SOLO lees, buscas y reportas. No crees ni modifiques archivos de test.
- No conviertas supuestos en reglas de negocio (G4).
- Si falta documentación, detén la exploración e informa el vacío (G1 STOP).
- No reportes un Target como resuelto sin haber agotado los niveles de `qa-locator-hunting`
  primero.
- No reportes el caso como "sin fixtures/storageState" sin haber revisado `playwright.config.ts`
  y la carpeta de fixtures real del proyecto primero.
- No inventes un `artifact_revision` — es siempre el que devuelve `qa-validate`, nunca un hash que tú calcules o supongas.

## Comandos de referencia

- Búsqueda de docs: MCP BookStack (`bookstack_search`).
- Búsqueda de memoria: MCP Engram (`mem_search`, `mem_context`).
- Caza de locators faltantes: skill `qa-locator-hunting`.
- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).
- Ledger: `gentle-ai qa-begin` / `gentle-ai qa-validate` / `gentle-ai qa-finish` (ver `docs/migration/qa-orchestrator-v3-design.md` para el flujo completo y la disposición de flags — `--evidence-goal`, `--max-attempts` y `--max-changed-lines` ya no existen).
