# Política de compuertas QA — G1 a G6

> **Provenance**: BookStack es la fuente OFICIAL. Este archivo es la ÚNICA
> renderización in-repo.
> Página: "06. Reglas del Agente Orquestador QA" · ID: `3239` ·
> URL: `https://bookstack.sreasons.com/books/gestion-interna/page/06-reglas-del-agente-orquestador-qa` · Recuperado: `2026-09-23`
> Prohibido copiar este texto en otro SKILL.md — refiéranlo por ruta.
> Ante conflicto con BookStack: gana BookStack; corrige este archivo, no el skill.

> **Nota de migración v3 (2026-09-23)**: este archivo es la única fuente de
> las reglas G1-G6 para las skills `qa-*` de `qa-orchestrator-v3`
> (`qa-supervisor`, `qa-explore`, `qa-spec`, `qa-apply`, `qa-verify`,
> `qa-docs`) — ninguna las copia, todas la referencian por ruta. Las reglas
> de negocio (G1-G6) no cambiaron respecto a v2; lo que cambió es el
> mecanismo técnico que las hace cumplir (`qa-status`/`qa-begin`/
> `qa-finish`/`qa-approve`/`qa-validate` sobre `QAStateMachine`, ver
> `docs/migration/qa-orchestrator-v3-design.md`).

## Regla Cero (obligatoria antes de cualquier cosa)

Antes de proponer, escribir una línea de código o delegar una implementación,
**DEBES**:

1. Usar el MCP de **BookStack** (`bookstack_search`) para buscar la
   documentación oficial que sustenta la solicitud (PRD del módulo, criterios
   de aceptación reales del negocio, páginas oficiales del Agente QA) y
   **citar** las páginas/capítulos/secciones usadas.
2. Si la solicitud lo amerita, usar el MCP de **GitLab** para verificar el
   estado real del código que se tocará (último MR, rama, tags).
3. NO inventar convenciones ni supuestos cuando exista documentación oficial
   aplicable.

En la respuesta, enlaza el documento consultado bajo el título "Reglas
aplicables - Documentación consultada": cada página consultada se renderiza
como **ficha de documentación PRD** (formato completo en `qa-doc-reference`).

### La fuente de las reglas

La lógica es operativa, no inventada. Vive en BookStack:

- **Página "06. Reglas del Agente Orquestador QA"** — reglas G1 a G6
  (texto completo en la sección siguiente).
- **Página "07. Plantillas del Agente QA"** — plantilla de respuesta
  (10 secciones) y checklist final.
- **Páginas PRD del módulo** según la solicitud (ej. para Punto de Venta,
  libro `ERP_PV_Punto Venta`).

## Reglas G1 a G6 (obligatorias, priman sobre todo)

Debes respetar y verificar cada una antes de autorizar cualquier
implementación:

- **G1 — Documentación como fuente oficial**: consultar BookStack antes de
  proponer; citar páginas; NO inventar convenciones; si falta/incompleta/
  contradice → detener e informar el vacío; si BookStack difiere del código
  actual, NO decidas tú, presenta la contradicción al humano.
- **G2 — Análisis previo**: buscar tests similares en el módulo; revisar
  fixtures/helpers/Tasks/Questions/Pages reutilizables; verificar
  convenciones de nombres y ubicación; revisar la config de Playwright;
  evaluar setup y prerrequisitos; medir impacto en otras pruebas.
- **G3 — Planificación obligatoria**: para cambios medianos/grandes NO
  implementes directo. Entrega un plan (objetivo, documentación consultada,
  pruebas similares, componentes reutilizables, archivos a crear/modificar,
  riesgos, validaciones, alcance/fuera-de-alcance). La implementación SOLO
  tras aprobación humana registrada (`gentle-ai qa-approve`, ver más abajo —
  el ledger es la única fuente que `qa-apply` consulta, nunca una aprobación
  solo verbal/en el chat).
- **G4 — Manejo de incertidumbre**: distingue hechos-de-BookStack vs
  observados-en-código vs inferencias vs recomendaciones vs
  pendiente-de-confirmar. Si un criterio no está definido, pide aclaración.
  NO conviertas una suposición en regla de negocio.
- **G5 — Control de riesgos**: no toques config global sin autorización; no
  agregues dependencias sin justificar; no elimines código sin analizar
  referencias; no modifiques tests fuera del alcance; no guardes
  secretos/tokens/contraseñas; no ejecutes comandos destructivos; no
  sobreescribas en BookStack durante la primera fase.
- **G6 — Validación de la implementación**: al declarar finalizada una
  implementación, exige: `npx tsc --noEmit`; ejecutar la prueba modificada;
  revisar lint; verificar que no haya credenciales; verificar que no haya
  esperas fijas innecesarias; verificar reutilización de componentes;
  comparar el resultado contra la documentación consultada; entregar el
  comando de ejecución y el resultado.

## Checklist de evidencia G6 (obligatoria, TODA)

Al declarar finalizada una implementación o validación, exige y ejecuta:

1. `npx tsc --noEmit` — tipos compilan.
2. Ejecutar la prueba modificada/creada — pasa.
3. Revisar lint del proyecto — limpio.
4. Verificar que NO haya credenciales/secretos hardcodeados
   (`waitForTimeout`, `sleep`, etc. cuentan como esperas fijas, no como
   credenciales — verificar ambas por separado).
5. Verificar que NO haya esperas fijas innecesarias.
6. Verificar que el test siga **Screenplay+POM** (sin locators crudos en el
   archivo de test; actor/tasks/questions/targets usados según el diseño del
   spec) — reutilizando componentes existentes (Page Objects, Fixtures,
   Helpers, Tasks, Interactions, Questions) cuando aplica, o con la
   estructura nueva creada según SOLID cuando el proyecto no tenía patrón
   previo.
7. Comparar el resultado contra la documentación consultada (BookStack).
8. Entregar el comando de ejecución exacto y el resultado/evidencia
   obtenido (screenshots, traces, videos o salida de reporter de
   Playwright). La revisión automatizada de código/diff (lint, secretos) vive
   fuera de esta checklist funcional — `qa-verify` invoca el ciclo real de
   RDD directamente para eso, ver `skills/qa-verify/SKILL.md` y Fase 3C.3 en
   `docs/migration/qa-orchestrator-v3-design.md`.

## Ledger QA (mecanismo técnico que hace cumplir G3/G5, no una regla de negocio nueva)

Las 5 etapas del flujo (`explore → spec → apply → verify → docs`) y la
aprobación humana de G3 están garantizadas en código por `QAStateMachine`,
no solo por convención de prompt (a diferencia de v2, donde un agente
confundido podía saltarse el orden o el gate de aprobación en el primer
intento de un `{change}` — ver `docs/migration/qa-orchestrator-v2-baseline-checklist.md`).
Toda skill `qa-*` debe:

- Consultar `gentle-ai qa-status --change {change} --cwd <repo>` antes de
  decidir cualquier acción — su `next_action` (`begin`/`finish`/`complete`)
  y `stage` son la única autoridad de ruteo; ninguna skill decide el orden
  por sí misma.
- Usar el mismo `{change}` en todas las etapas (confirmado como
  comportamiento correcto y necesario en el baseline).
- Cerrar cada etapa con `gentle-ai qa-validate` (schema canónico, nunca el
  envelope legacy `schema/facts/observations`) antes de `gentle-ai qa-finish`,
  usando el `artifact_revision` real que `qa-validate` devuelve como
  `--evidence-revision` — nunca un hash autodeclarado.

## Contrato de herramientas QA standalone

Aplica a `qa-locator-hunting`, `qa-doc-reference`, `qa-doc-access` y
`qa-evidence`:

- Puede ejecutarse sin un `{change}` activo.
- No debe mutar el estado del flujo QA (ledger) salvo que quien la invoca la
  adjunte explícitamente a un `{change}`.
- Nunca avanza etapas (`qa-begin`/`qa-finish`) por sí misma — eso es
  responsabilidad exclusiva de las 5 skills de etapa (`qa-explore`/`qa-spec`/
  `qa-apply`/`qa-verify`/`qa-docs`).
- Puede devolver evidencia/artefactos para que una etapa orquestada los
  consuma después, si recibe contexto de `{change}` — ese contexto es
  siempre opcional para su uso standalone.

## Uso de este archivo

- Cada `SKILL.md` de la familia `qa-*` referencia este archivo por ruta
  (`skills/_shared/qa-gate-policy.md`) en vez de copiar el texto de arriba.
- Si necesitas citar una regla puntual en un skill, usa el identificador
  corto (`G1`, `G2`, ... `G6`) y enlaza aquí — nunca reproduzcas el párrafo
  completo.
