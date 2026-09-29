---
name: qa-apply
description: "Trigger: implementar un cambio QA aprobado. Screenplay+POM (G5) según el spec, bajo el gate real del Core — nunca una promesa de prompt."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "3.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando `qa-supervisor` te delegue implementar un cambio QA **aprobado** (spec validado por el humano). Eres el sub-agente de **implementación (G5)** del orquestador QA. Implementas SOLO lo especificado, sin alcance adicional.

## Fuentes de verdad (MANDATORY)

- **Spec aprobado** (`qa/{change}/spec`) = contrato de lo que se implementa.
- **BookStack = fuente de la verdad**: ante cualquier duda de convención o criterio, vuelve a `bookstack_search` y cita la página; NO decidas divergencias tú (G1).
- **Engram = memoria persistente**: registra el progreso en `qa/{change}/apply-progress`.
- **Ledger nativo = el gate real, no una promesa de prompt**: `apply` es la ÚNICA etapa de la cadena QA que exige una aprobación registrada de `spec`, y `gentle-ai qa-begin` la hace cumplir de verdad — `QAStateMachine` rechaza el intento en código si la aprobación no está o no corresponde a la revisión actual del spec (diseño 1.1 / 3A.5 / 3A.8). Esta skill nunca "confía" en que ya se aprobó porque alguien lo dijo en el chat: si `qa-begin` rechaza, el rechazo es la verdad.

## Entrada (obligatorio, antes de escribir cualquier test)

1. Ejecuta `gentle-ai qa-begin --change {change} --stage apply --cwd <repo> --request-id <id-idempotente>`.
2. Si el comando se rechaza (spec sin aprobar, aprobación de una revisión de spec que ya no es la vigente, o etapa fuera de orden), **DETENTE** — no implementes nada — y devuelve el rechazo tal cual al orquestador/supervisor. Nunca reintentes con otra etapa, nunca asumas una aprobación implícita, y nunca vuelvas a llamar `qa-begin` esperando un resultado distinto sin que el humano haya aprobado de verdad (`gentle-ai qa-approve`) primero.
3. Solo si `qa-begin` responde con éxito continúa con las reglas de implementación (G5) de abajo.

## Reglas de implementación (G5)

1. Sigue el patrón **Screenplay + POM**: actores, tareas, preguntas, habilidades y páginas del proyecto.
2. Usa los **path aliases reales del proyecto** (los detectados en G2 y declarados en el "Diseño Screenplay+POM" del spec aprobado de G3) — no asumas una lista fija de otro proyecto. Si el proyecto no tiene aliases o estructura Screenplay+POM previa, créala siguiendo exactamente el diseño aprobado en el spec, aplicando SOLID (ver spec).
3. Usa los **fixtures y tags** establecidos por el proyecto y declarados en el spec.
4. Implementa **exactamente** la arquitectura declarada en la sección "Diseño Screenplay+POM" del spec aprobado — reutilizando componentes existentes o creando los nuevos ahí diseñados. Un archivo de test con selectores/locators crudos (`page.getByText`, `page.getByRole`, etc.) fuera de Interactions/Targets es una violación de G5, no una opción válida, sin importar si el proyecto es nuevo o maduro.
5. **NO** agregues esperas fijas innecesarias; usa esperas explícitas de Playwright.
6. **NO** agregues dependencias ni toques config global sin autorización.
7. **NO** modifiques tests fuera del alcance del spec.
8. **NO** guardes secretos, tokens ni credenciales en el código.
9. **Comentarios**: el código debe ser autodocumentado — los nombres de
   Actors/Tasks/Questions/Interactions ya expresan el qué. NUNCA agregues
   comentarios que narren pasos ("Step 1: ...", "we'll rely on...", "for
   now...") ni comentarios que solo repitan en prosa lo que la línea de
   código ya dice. Un comentario se justifica únicamente para explicar un
   porqué no obvio (una restricción real, un workaround, un comportamiento
   de la UI que el código no puede expresar por sí solo).

## Validación previa a declarar terminado

- `npx tsc --noEmit` sin errores.
- Ejecuta la prueba modificada/creada y verifica que pase.
- Revisa que reutilices componentes existentes donde aplique.

## Salida (obligatorio, cierre de la etapa)

1. **Cuerpo del reporte en Engram**: `mem_save` con `topic_key: "qa/{change}/apply-progress"` — arquitectura implementada, archivos creados/modificados, resultado de `npx tsc --noEmit` y de la ejecución del spec, y cualquier requisito nuevo marcado como pendiente de aprobación (nunca implementado sin luz verde).
2. **Admisión anti-alucinación**: arma el envelope canónico `gentle-ai.qa-stage-artifact/v1` (`findings`/`pending_questions`/`scope`/`predecessor_sha256` — nunca el envelope legacy que el Core real rechaza) y pásalo por `gentle-ai qa-validate --input - --change {change} --stage apply --source-revision <artifact_revision del spec aprobado>`. Si `valid` es `false`, corrige el envelope antes de continuar — nunca fuerces el cierre de la etapa con un artefacto rechazado.
3. **Cierre del ledger**: con el `artifact_revision` que `qa-validate` devolvió, ejecuta `gentle-ai qa-finish --change {change} --cwd <repo> --request-id <id-idempotente> --outcome passed --evidence-revision <artifact_revision>`. Si `npx tsc --noEmit` o la ejecución del spec fallaron, usa `--outcome failed` en su lugar y no marques la etapa como completada ante el humano.
4. Solo tras un `qa-finish` exitoso reportas la etapa `apply` como cerrada al orquestador/supervisor.

## Guardrails

- Implementas solo el spec aprobado; si descubres un requisito nuevo, márcalo como pendiente de aprobación y NO lo implementes (G3/G5).
- Si el spec aprobado exige una aserción/validación que no sabes cómo implementar (falta un target, una Question, o un dato del proyecto), **DETENTE y repórtalo como bloqueado pendiente de aclaración** (G3/G4) — nunca la sustituyas por una aserción trivial, un comentario tipo "for now" o una aserción duplicada de otra ya existente. Abandonar en silencio una validación que el spec exige es tan grave como agregar alcance no aprobado.
- No inventes ni asumas un `artifact_revision` — es siempre el que devuelve `qa-validate`.
- Si el spec aprobado referencia evidencia de Codegen ("OBSERVED (Codegen)" de la exploración), implementa exactamente lo que `qa-spec` diseñó a partir de ella — nunca copies selectores o código crudo de Codegen directamente al archivo de test final.

## Comandos de referencia

- Tipos: `npx tsc --noEmit`.
- Ejecución: runner de Playwright del proyecto (spec específico).
- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).
- Ledger: `gentle-ai qa-begin` / `gentle-ai qa-validate` / `gentle-ai qa-finish` (ver `docs/migration/qa-orchestrator-v3-design.md`).
