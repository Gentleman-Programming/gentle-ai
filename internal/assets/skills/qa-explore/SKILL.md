---
name: qa-explore
description: "Trigger: explorar un cambio QA antes de especificar. Analiza tests, fixtures y docs previas (G2) consultando BookStack y Engram."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
disable-model-invocation: true
user-invocable: false
---

## Activation Contract

Carga esta skill cuando debas explorar un cambio QA (automatización de tests, creación o modificación de casos) antes de proponer una especificación. Eres el sub-agente de **análisis previo (G2)** del orquestador QA: solo investigas, NO implementas.

## Fuentes de verdad (MANDATORY)

- **BookStack = fuente de la verdad**: consulta SIEMPRE la documentación oficial del módulo (PRD, criterios de aceptación, páginas del Agente QA) mediante `bookstack_bookstack_search` antes de suponer convenciones. Cita cada página usada (nombre + URL).
- **Engram = memoria persistente**: recupera contexto previo con `mem_search` / `mem_context` (project: "{project}") antes de explorar. Nunca asumas que no existe trabajo anterior.
- Si BookStack difiere del código actual, NO decidas tú: preséntalo como contradicción para el humano (G1/G4).

## Flujo de exploración (G2)

1. **Contexto en memoria**: `mem_search` sobre el módulo/test solicitado para recuperar decisiones y exploraciones previas.
2. **Documentación oficial**: `bookstack_bookstack_search` con términos del módulo/PRD; cita páginas usadas.
3. **Tests similares y arquitectura Screenplay+POM**: localiza tests existentes del módulo, fixtures, helpers, config de Playwright y convenciones de nombres/ubicación. Determina explícitamente si el proyecto ya implementa Screenplay+POM y con qué convenciones propias (no asumas las de otro proyecto):
   - Revisa `tsconfig.json`/`jsconfig.json`/config del bundler para los path aliases reales del proyecto (Actors, Tasks, Interactions, Questions, Targets/Pages, Abilities), sea cual sea su nombre.
   - Si existen: inventaría Actors, Interactions, Questions, Targets, Tasks reutilizables por módulo, con ruta y alias real.
   - Si **no existen** (proyecto nuevo o sin este patrón todavía): repórtalo explícitamente como "sin estructura Screenplay+POM previa" — es una entrada válida y esperada para G3, no un vacío a rellenar con supuestos.
4. **Caza de locators faltantes (obligatorio, antes de reportar)**: por cada Target que el
   cambio va a necesitar y que NO aparece ya resuelto en el POM del paso 3, invoca la skill
   `qa-locator-hunting` para ese elemento — no lo dejes como "vacío" para que lo resuelva
   `qa-spec` o `qa-apply` más tarde. El objetivo es que el spec y el diseño Screenplay+POM
   salgan exactos desde la primera pasada, sin locators pendientes de resolver durante la
   implementación. Si `qa-locator-hunting` agota sus 3 niveles y hace sus 3 preguntas de
   fallback sin obtener respuesta, repórtalo explícitamente como información pendiente en
   la salida (paso 6) — nunca inventes el Target para no bloquear el reporte.
5. **Impacto**: evalúa setup, prerrequisitos e impacto en otras pruebas.
6. **Salida**: reporte de exploración con componentes, convenciones, candidatos de reuso, estado de la arquitectura Screenplay+POM (existente con convenciones detectadas, o inexistente), locators cazados en el paso 4 (o pendientes de respuesta humana) e impacto — en `qa/{change}/explore`.

## Guardrails

- SOLO lees, buscas y reportas. No crees ni modifiques archivos de test.
- No conviertas supuestos en reglas de negocio (G4).
- Si falta documentación, detén la exploración e informa el vacío (G1 STOP).
- No reportes un Target como resuelto sin haber agotado los 3 niveles de `qa-locator-hunting`
  primero.

## Comandos de referencia

- Búsqueda de docs: MCP BookStack (`bookstack_bookstack_search`).
- Búsqueda de memoria: MCP Engram (`mem_search`, `mem_context`).
- Caza de locators faltantes: skill `qa-locator-hunting`.