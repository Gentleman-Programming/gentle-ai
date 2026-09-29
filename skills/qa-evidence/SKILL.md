---
name: qa-evidence
description: "Trigger: producir el bundle de evidencia G6 (tipos, test, sin esperas fijas, Screenplay+POM, comparación docs, comando+resultado) para un caso QA, standalone."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
---

## Activation Contract

Carga esta skill cuando necesites producir el paquete de evidencia
reproducible G6 para un caso de prueba QA. Es una herramienta **standalone**:
funciona con o sin un `{change}` activo. Si hay un `{change}` activo, puede
guardar/adjuntar su salida como evidencia de ese `{change}` (por ejemplo bajo
`qa/{change}/verify-report`); si no lo hay, produce el mismo paquete para uso
puntual. Ese contexto de `{change}` es **siempre opcional**, nunca requerido
para ejecutar esta skill.

## Alcance (MANDATORY)

Esta skill cubre exactamente los ítems 1, 2, 5, 6, 7 y 8 del checklist de
evidencia G6 (ver `skills/_shared/qa-gate-policy.md`, fuente única in-repo
para el checklist completo y la numeración de ítems):

- **Ítem 1**: Los tipos compilan.
- **Ítem 2**: La prueba modificada/creada pasa.
- **Ítem 5**: No hay esperas fijas innecesarias.
- **Ítem 6**: La prueba sigue Screenplay+POM.
- **Ítem 7**: El resultado se comparó contra la documentación de BookStack
  consultada.
- **Ítem 8**: Se entregó el comando de ejecución exacto y el
  resultado/evidencia obtenido.

Los ítems 3 (lint) y 4 (credenciales/secretos) están explícitamente FUERA de
alcance — esos quedan bajo la invocación real de RDD que ya implementa
`qa-verify` (ver `skills/qa-verify/SKILL.md`). NUNCA reimplementes ni
dupliques esa revisión aquí.

## Execution Steps

1. Identifica el/los archivo(s) de prueba bajo evidencia — el modificado o
   creado para este caso.
2. **Ítem 1**: ejecuta `npx tsc --noEmit` (o el comando de typecheck
   equivalente del proyecto) y registra pass/fail más la salida exacta.
3. **Ítem 2**: ejecuta la prueba modificada/creada con su comando exacto de
   Playwright y registra pass/fail más la salida exacta.
4. **Ítem 5**: revisa el archivo de prueba y confirma que no tenga esperas
   fijas innecesarias (`waitForTimeout`, `sleep`, etc.); si encuentras alguna,
   repórtala como fallo del ítem en vez de ignorarla.
5. **Ítem 6**: confirma que la prueba siga **Screenplay+POM** — sin locators
   crudos en el archivo de test; actor/tasks/questions/targets usados según
   el diseño del spec, reutilizando Page Objects/Fixtures/Helpers/Tasks/
   Interactions/Questions existentes cuando apliquen, o componentes nuevos con
   estructura SOLID cuando el proyecto no tenía patrón previo.
6. **Ítem 7**: compara el resultado observado contra la documentación de
   BookStack consultada durante spec/apply (cita las páginas usadas, con el
   formato de ficha de `qa-doc-reference` cuando aplique).
7. **Ítem 8**: recopila el comando de ejecución exacto y su resultado/
   evidencia (screenshots, traces, videos o salida del reporter de
   Playwright).
8. Si hay un `{change}` activo, opcionalmente persiste el paquete como
   evidencia en `qa/{change}/verify-report` (o un topic key equivalente de
   Engram) — si no, devuélvelo inline en la respuesta.

## Output Contract

Devuelve un reporte estructurado, una fila por ítem:

| Ítem | Chequeo | Estado | Comando exacto | Resultado / evidencia |
|---|---|---|---|---|
| 1 | Tipos compilan | pass/fail | ... | ... |
| 2 | Corrida de la prueba | pass/fail | ... | ... |
| 5 | Sin esperas fijas | pass/fail | ... | ... |
| 6 | Screenplay+POM | pass/fail | ... | ... |
| 7 | Comparación con docs | pass/fail | ... | ... |
| 8 | Comando + evidencia | pass/fail | ... | ... |

NUNCA marques un ítem como `pass` sin adjuntar el comando exacto y su
resultado observado.

## Guardrails (MANDATORY)

- NUNCA llames `qa-begin`/`qa-finish` ni avances el ledger QA por tu cuenta —
  esta skill es una productora standalone de evidencia, no una etapa del
  ledger.
- NUNCA toques los ítems 3 (lint) o 4 (credenciales/secretos) — esos quedan
  bajo la invocación real de RDD de `qa-verify`; no reimplementes ni evites
  RDD aquí.
- NUNCA inventes la salida de un comando ni marques un ítem como `pass` sin
  haberlo ejecutado realmente.
- El contexto de `{change}` es siempre opcional: nunca bloquees la producción
  de evidencia por la ausencia de un `{change}` activo.

## Referencias

- Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única
  in-repo).
- Contrato de herramienta standalone (uso sin `{change}` activo, nunca
  avanza el ledger): sección "Contrato de herramientas QA standalone" del
  mismo archivo.
- Invocación real de RDD para los ítems 3-4: `skills/qa-verify/SKILL.md`.
