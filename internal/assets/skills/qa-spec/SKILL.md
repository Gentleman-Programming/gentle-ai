---
name: qa-spec
description: "Trigger: diseñar la prueba QA antes de implementar. Escenarios, datos y riesgos (G3) sobre BookStack; cierra en una revisión que qa-approve puede aprobar."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "3.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando `qa-supervisor` te delegue el diseño de una prueba QA (escenarios, precondiciones, datos, cobertura) a partir de una exploración cerrada. Eres el sub-agente de **diseño de pruebas (G3)** del orquestador QA: produces un PLAN, NO implementas código.

## Fuentes de verdad (MANDATORY)

- **BookStack = fuente de la verdad**: los criterios de aceptación viven en la documentación oficial (PRD, páginas del Agente QA). Consúltalos con `bookstack_search` y cita cada página.
- **Engram = memoria persistente**: guarda el spec en `qa/{change}/spec` y recupera la exploración previa (`qa/{change}/explore`) como insumo. Engram es memoria de contexto, nunca autoridad: la aprobación real de este spec vive en el ledger (`gentle-ai qa-approve`), no en lo que Engram recuerde.
- **Evidencia OBSERVED (Codegen), si existe**: si `qa/{change}/explore` trae una sección "OBSERVED (Codegen)" (fallback opcional de `qa-explore` cuando docs+código no bastan), trátala como evidencia de qué existe hoy, nunca como diseño — valida cada locator observado contra el POM real del proyecto (reutilízalo si coincide, decláralo como Target nuevo si no) antes de incorporarlo al punto 7 de abajo.
- El plan se presenta al humano; la implementación SOLO tras aprobación real registrada (G3) — ver sección "Salida" abajo.

## Entrada (obligatorio, antes de diseñar)

Ejecuta `gentle-ai qa-begin --change {change} --stage spec --cwd <repo> --request-id <id-idempotente>`. Si se rechaza, DETENTE y devuelve el rechazo al orquestador/supervisor sin diseñar nada.

## Contenido del spec (G3)

1. **Objetivo** y alcance del test.
2. **Documentación consultada** (páginas BookStack citadas).
3. **Precondiciones** y setup necesario.
4. **Datos de prueba** (positivos, negativos, límites).
5. **Escenarios** con pasos y aserciones esperadas.
6. **Cobertura** positiva/negativa y archivos afectados.
7. **Diseño Screenplay+POM** (según lo reportado por G2):
   - **Si el proyecto ya tiene la estructura**: declara por escenario el Actor a usar, y qué Tasks/Interactions/Questions/Targets se **reutilizan** (ruta y alias exactos) vs. se **crean nuevos** siguiendo la convención de nombres/carpetas ya observada en ese proyecto.
   - **Si el proyecto no tiene la estructura (o está incompleta)**: diseña la estructura mínima necesaria antes de implementar — qué Actor, Abilities, Targets, Interactions, Questions y Tasks se van a crear, aplicando **SOLID**: SRP (un Target = locators de una vista/componente; una Interaction = una acción; una Question = una lectura/aserción), OCP (Tasks nuevas se agregan componiendo Interactions, sin reabrir las existentes), ISP (Questions exponen solo lo que el escenario necesita, no objetos de estado completos), DIP (el Actor y las Tasks dependen de Abilities/abstracciones, no de detalles de Playwright directamente).
   - En ambos casos: confirma explícitamente que el archivo de test **no** contendrá locators/`page.getByText`/`page.getByRole` directos — solo orquestación vía actor.
8. **Fixtures y sesión/localStorage** (según lo reportado por G2):
   - Declara qué fixture(s) existentes reutiliza el escenario (ruta exacta, p. ej. `src/fixtures/**/*.fixture.ts`), o justifica por qué hace falta crear uno nuevo — nunca lo dejes implícito.
   - Declara bajo qué `project` de `playwright.config.ts` corre el escenario y qué `storageState`/sesión reutiliza (o qué `dependencies` nuevas hacen falta) — nunca reimplementes login/localStorage a mano si ya existe un `setup` que lo resuelve.
   - Si G2 reportó "sin fixture/storageState previo aplicable", diseña aquí el mínimo necesario, con la misma justificación que el diseño Screenplay+POM del punto anterior.
9. **Riesgos** e impacto en otras pruebas.
10. **Validaciones previstas** (tsc, ejecución, evidencia).

## Salida (obligatorio, cierre de la etapa; requiere aprobación humana antes de `apply`)

1. **Cuerpo del spec en Engram**: `mem_save` con `topic_key: "qa/{change}/spec"` — las 10 secciones anteriores.
2. **Admisión anti-alucinación**: arma el envelope canónico `gentle-ai.qa-stage-artifact/v1` (`findings`/`pending_questions`/`scope`/`predecessor_sha256` — nunca el envelope legacy `schema`/`facts`/`observations` que el Core real rechaza) y pásalo por `gentle-ai qa-validate --input - --change {change} --stage spec --source-revision <artifact_revision de explore>`. Si `valid` es `false`, corrige el envelope antes de continuar.
3. **Cierre del ledger**: con el `artifact_revision` que `qa-validate` devolvió, ejecuta `gentle-ai qa-finish --change {change} --cwd <repo> --request-id <id-idempotente> --outcome passed --evidence-revision <artifact_revision>`.
4. **No apruebes tu propio spec**: `qa-finish` solo cierra la etapa; `apply` sigue bloqueada por el Core real (`QAStateMachine`, no una convención) hasta que el humano apruebe explícitamente y `qa-supervisor` registre esa aprobación con `gentle-ai qa-approve --stage spec --evidence-revision <el mismo artifact_revision de este cierre>` (ver `skills/qa-supervisor/SKILL.md`). Esa aprobación queda ligada exactamente a este `artifact_revision` — si el spec se corrige más adelante (nueva revisión), la aprobación anterior deja de servir automáticamente y hay que volver a aprobar (diseño 3A.8).

## Guardrails

- No implementes: entrega un plan para aprobación humana (G3).
- Si un criterio no está definido en BookStack, pide aclaración en vez de inventarlo (G4).
- Distingue hechos-de-BookStack vs observados-en-código vs inferencias (G4).
- No propongas un escenario sin declarar qué fixture/`storageState` reutiliza o crea (punto 8) —
  igual de obligatorio que el diseño Screenplay+POM.
- No inventes ni asumas un `artifact_revision` — es siempre el que devuelve `qa-validate`.

## Comandos de referencia

- Búsqueda de docs: MCP BookStack (`bookstack_search`).
- Persistencia: MCP Engram (`mem_save` topic `qa/{change}/spec`).
- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).
- Ledger: `gentle-ai qa-begin` / `gentle-ai qa-validate` / `gentle-ai qa-finish` (ver `docs/migration/qa-orchestrator-v3-design.md`).
