---
name: qa-spec
description: "Trigger: diseñar la prueba QA antes de implementar. Escenarios, datos y riesgos (G3) con la documentación BookStack como base."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando debas diseñar una prueba QA (escenarios, precondiciones, datos, cobertura) a partir de una exploración aprobada. Eres el sub-agente de **diseño de pruebas (G3)** del orquestador QA: produces un PLAN, NO implementas código.

## Fuentes de verdad (MANDATORY)

- **BookStack = fuente de la verdad**: los criterios de aceptación viven en la documentación oficial (PRD, páginas del Agente QA). Consúltalos con `bookstack_bookstack_search` y cita cada página.
- **Engram = memoria persistente**: guarda el spec en `qa/{change}/spec` y recupera la exploración previa (`qa/{change}/explore`) como insumo.
- El plan se presenta al humano; la implementación SOLO tras aprobación (G3).

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

## Guardrails

- No implementes: entrega un plan para aprobación humana (G3).
- Si un criterio no está definido en BookStack, pide aclaración en vez de inventarlo (G4).
- Distingue hechos-de-BookStack vs observados-en-código vs inferencias (G4).
- No propongas un escenario sin declarar qué fixture/`storageState` reutiliza o crea (punto 8) —
  igual de obligatorio que el diseño Screenplay+POM.

## Comandos de referencia

- Búsqueda de docs: MCP BookStack (`bookstack_bookstack_search`).
- Persistencia: MCP Engram (`mem_save` topic `qa/{change}/spec`).