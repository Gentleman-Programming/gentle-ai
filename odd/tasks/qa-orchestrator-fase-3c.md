# Fase 3C — Integraciones reales (QA-Orchestrator v3)

## Objetivo

Completar las integraciones reales que Fase 3B dejó deliberadamente diferidas,
en el orden que pidió el usuario, sin reabrir Core ni skills salvo una
incompatibilidad concreta encontrada durante la integración. Sin merge a
`main` hasta que se decida explícitamente.

## Por qué / contexto

Fase 3B (cerrada en commit `5a730728`) portó y empaquetó las 9 skills QA con
un smoke test E2E real pero **simbólico**: sin Engram/BookStack MCP
conectados, sin Playwright MCP, sin RDDAdapter real, sin MR real. Fase 3C
ejercita cada una de esas integraciones contra MCPs/sistemas reales.

## Alcance y orden (instrucción explícita del usuario)

1. **3C.1** — Engram + BookStack MCP
2. **3C.2** — locator-hunting NIVEL 0/1 + Playwright MCP
3. **3C.3** — QACodeReviewer + RDDAdapter real
4. **3C.4** — G6 funcional completo / qa-verify E2E
5. **3C.5** — creación real de MR vía GitLab MCP
6. **3C.6** — smoke E2E final sobre un repositorio de automatización real

TDD/reviews acotadas por bloque (candidato = un work-unit commit, nunca el
branch acumulado). Detenerse solo cuando se necesite autorización real
(credenciales/MCP/acciones externas). No revisar candidatos históricos
gigantes (base-diff del branch completo) — declinar sin volver a preguntar,
tal como ya se estableció.

## Mapeo técnico inicial (workflow de scoping, 2026-09-23)

Confirmado con evidencia real de código/MCP (no supuesto):

| Sub-fase | ¿Lista ahora? | Bloqueo real |
|---|---|---|
| 3C.1 | Sí | Ninguno — BookStack MCP y Engram ya conectados en este entorno |
| 3C.2 | Parcial | NIVEL 0 (POM local) y NIVEL 2 (GitLab) listos; NIVEL 1 requiere Playwright MCP (no conectado) + autorización explícita de navegar a un entorno real |
| 3C.3 | Sí — cerrado con cambio de diseño | Ninguno. `QACodeReviewer`/`RDDAdapter` (Go) retirado; `qa-verify` invoca el ciclo real de RDD directamente a nivel de skill. |
| 3C.4 | Parcial | Los ítems 1,2,5,6,7,8 del checklist G6 ya son responsabilidad 100% de `qa-verify`; los ítems 3-4 (lint/secretos) dependen de 3C.3; el cierre completo también depende de 3C.6 (repo real) |
| 3C.5 | Parcial | Adaptador/plumbing (interfaz + stub + tests contra un cliente GitLab falso) sí puede construirse ahora; la invocación real de `create_merge_request` exige autorización explícita del usuario en cada ocasión — nunca general |
| 3C.6 | No | Depende de que el usuario indique qué repositorio real de automatización usar — decisión de producto, no técnica |

Decisiones de producto/arquitectura pendientes (se preguntan cuando se llegue
a esa sub-fase, no todas de una vez):
- 3C.5: ¿la autorización de MR real se vuelve un gate auditable en código
  (como `qa-approve`) o se queda como stop-and-ask conversacional (ya
  demostrado en 3B.8)?
- 3C.6: ¿qué repositorio real de automatización se usa?
- 3C.2: pendiente confirmar en sesión nueva que Playwright MCP conecta, y
  autorizar navegar a un entorno real de dev/staging.

Resueltas: 3C.3 (ver sección de esa sub-fase — retiro de `QACodeReviewer`/
`RDDAdapter`, integración a nivel de skill); 3C.2 Playwright MCP ya
conectado (pendiente solo sesión nueva + autorización de entorno).

## Tareas

### 3C.1 — Engram + BookStack MCP

- [x] Verificar en vivo (no solo prosa) que el nombre real de la tool
  BookStack es `bookstack_search`, no `bookstack_bookstack_search` — corregido
  en `skills/qa-explore`, `qa-spec`, `qa-apply`, `qa-supervisor`,
  `skills/_shared/qa-gate-policy.md` (+ mirrors en `internal/assets/skills`).
  Verificado con llamadas reales a `bookstack_search`/`bookstack_get_page`.
- [x] Rellenar los placeholders `TODO-MAINTAINER-FILL-BOOKSTACK-*` de
  `qa-gate-policy.md` con la página real verificada (ID `3239`,
  `https://bookstack.sreasons.com/books/gestion-interna/page/06-reglas-del-agente-orquestador-qa`).
- [x] Documentar en `qa-doc-reference` el paso de resolución Libro/Capítulo
  (`bookstack_get_book`/`bookstack_get_chapter`, ya que `bookstack_get_page`
  solo devuelve IDs numéricos) y aclarar que "Responsables"/"Ticket
  Redmine"/"Versión del documento" no existen como campos estructurados en la
  API real — se reportan "no disponible" salvo que la página los declare en
  su propio texto.
- [x] Corregir la nota de migración obsoleta en `qa-docs` que afirmaba que
  `qa-supervisor/qa-explore/qa-spec/qa-apply/qa-verify` "todavía no existen
  en esta rama" (falso desde el commit `5a730728`).
- [x] **Hallazgo de arquitectura no anticipado** (fuera del scoping original,
  planteado por el usuario): ¿qué hace `qa-explore` cuando BookStack no
  documenta una funcionalidad *existente* y el código tampoco basta para
  conocer el comportamiento real de UI? Resuelto 100% a nivel de skill, cero
  cambios de Core (verificado leyendo `internal/qastage/state_machine.go` y
  `artifact.go` directamente): fallback opcional de captura Codegen dirigida
  por el humano, cerrado con `qa-finish --outcome interrupted` sobre un
  artefacto `scope.status:"blocked"` (primitivos ya existentes en el Core
  desde 3A.6), retomado en un `qa-begin` nuevo que extrae hechos "OBSERVED
  (Codegen)" al cuerpo libre de Engram — nunca al `findings[]` validado por
  el Core (esa clasificación sigue siendo estrictamente
  `DOCUMENTED|MISSING|NOT_APPLICABLE`, verificado en código). Ver memoria
  Engram `architecture/codegen-human-capture-fallback-for-qa-explore...`
  para el análisis completo. Implementado de forma aditiva (sin renumerar ni
  tocar los pasos 0-7 existentes de `qa-explore`) en `qa-explore` (paso
  nuevo condicional), `qa-spec` y `qa-apply` (guardrails de una línea cada
  uno) + sus mirrors.
- [ ] (Opcional, no bloqueante) Ejercitar un flujo real completo tipo 3B.8
  pero con Engram+BookStack conectados de verdad, para confirmar que no
  queda ningún otro mismatch de prosa-vs-API — se puede diferir sin bloquear
  el cierre de 3C.1.
- [x] **Corrección post-revisión RDD** (candidato `review-1d6ef9a33aa80aae`,
  aprobado, hallazgo informativo no bloqueante): el fallback Codegen de
  `qa-explore` insertaba su propia lista numerada 1-4 entre los pasos
  principales 2 y 3, duplicando números de paso cerca uno del otro —
  ambigüedad real para un agente siguiendo la skill secuencialmente.
  Corregido: el fallback ahora es un `###` con sub-pasos con letra (a-d), sin
  colisión con la numeración 0-7 del flujo principal. Aplicado en
  `skills/qa-explore` + mirror.

**Ruta/commits**: pendiente de commit inicial de 3C.1 (bookstack tool-name +
gate-policy + doc-reference + docs stale note + fallback Codegen).

**Verificación**: `go test ./internal/assets/...` y
`./internal/components/skills/...` verdes tras cada tanda de ediciones;
`diff -q` confirmando paridad byte-a-byte entre `skills/qa-*` e
`internal/assets/skills/qa-*` en cada archivo tocado.

### 3C.2 — locator-hunting NIVEL 0/1 + Playwright MCP

- [x] Re-verificar (sin cambio de código) que NIVEL 0 (POM local) y NIVEL 2
  (GitLab) siguen correctos; ejercitado NIVEL 2 de verdad vía
  `search_projects("erp-mf-seguridad")` → `SmartClic/erp-mf-seguridad`,
  coincide exacto con la fila del catálogo, cero drift. NIVEL 0 no se puede
  ejercitar dentro de `gentle-ai` (no es un proyecto de automatización
  consumidor, no tiene `src/pages/**`) — correctamente diferido a 3C.6.
- [x] Re-confirmar paridad de mirror entre `skills/qa-locator-hunting/**` e
  `internal/assets/skills/qa-locator-hunting/**` (`diff -q`, idénticos).
- [x] Preguntado y conectado: `claude mcp add playwright -s local -- npx -y
  @playwright/mcp@latest` (scope local, no comiteado). Verificado con un
  proceso `claude mcp list` aparte que conecta de verdad. **Limitación
  real**: esta sesión arrancó antes de agregarlo — `session_connectors_status`
  confirma que no lo ve todavía; un servidor agregado a mitad de sesión no
  se conecta en caliente, requiere sesión nueva. El usuario eligió seguir
  con 3C.3 y retomar NIVEL 1 en la próxima sesión (ya lo verá solo).
- [ ] **PENDIENTE (próxima sesión)**: confirmar que Playwright MCP aparece
  conectado, y pedir la autorización explícita del humano para navegar a un
  entorno real de dev/staging específico antes de ejercitar NIVEL 1 de
  verdad (la skill lo exige, es un requisito aparte de "el servidor está
  conectado").
- [x] No tratar GitLab/NIVEL 2 como sustituto de NIVEL 1 — las Hard Rules de
  la skill prohíben saltar niveles (confirmado, sin cambios necesarios).

### 3C.3 — QACodeReviewer + RDDAdapter real — CERRADO CON CAMBIO DE DISEÑO

- [x] Presentada al usuario la incompatibilidad real (no solo "en qué
  paquete vive"): el ciclo real de RDD es un protocolo con estado
  (STATUS→START→consentimiento humano→capturas por lente→acknowledge),
  verificado usándolo dos veces en esta sesión — no cabe en una llamada Go
  síncrona sin depender igual de un agente externo instalado y autenticado
  (`internal/reviewerprovider` ya hace `os/exec` sobre `claude`/`codex`/
  `opencode`/`pi`). Además `internal/cli` ya importa `internal/qastage`
  (ciclo de imports si el adapter viviera ahí).
- [x] **Decisión del usuario**: retirar `QACodeReviewer`/`RDDAdapter`
  (código muerto, nada en producción lo llamaba) y mover la integración a
  nivel de skill — `qa-verify` invoca el ciclo real de RDD directamente.
- [x] Eliminados `internal/qastage/reviewer.go` y `reviewer_test.go`.
  `go build ./...`, `go vet ./...`, `go test ./internal/qastage/...` verdes
  tras el borrado.
- [x] `skills/qa-verify/SKILL.md` (+ mirror) reescrito: nueva sección
  "Revisión de código real (RDD)" con el protocolo real (preflight STATUS,
  relay de consentimiento sin decidir, capturas por lente, acknowledge
  exactamente una vez, chequeo de `review mode status` primero). Checklist
  G6 ítems 3-4 actualizados. Guardrails corregidos (ya no dicen "no invoques
  RDD directamente" — ahora es exactamente lo que se pide).
- [x] `skills/_shared/qa-gate-policy.md` actualizado (ya no menciona
  `QACodeReviewer`).
- [x] `docs/migration/qa-orchestrator-v3-design.md` sección Fase 3C
  actualizada con el cierre real de 3C.1-3C.3.

### 3C.4 — G6 funcional completo / qa-verify E2E

- [ ] Agregar el requisito de cobertura negativa (estilo E2/E3) a
  `qa-verify/SKILL.md` y `qa-gate-policy.md` (prosa, sin código).
- [ ] Recortar la narrativa G6 de `qa-gate-policy.md` que duplica lo que RDD
  ya cubre mejor (una vez 3C.3 esté resuelto).
- [ ] No declarar "G6 completo" cerrado hasta que 3C.1 (ya real), 3C.3 y
  3C.6 estén todos en su lugar.

### 3C.5 — Creación real de MR vía GitLab MCP

- [ ] Diseñar e implementar `QAMergeRequestCreator` (interfaz Go + stub),
  mapeando un `{change}` cerrado a `project_path/source_branch/
  target_branch/title/description`.
- [ ] Tests unitarios contra un cliente GitLab falso — ninguna llamada real
  en esta tarea.
- [ ] Preguntar al usuario: ¿la descripción del MR se auto-puebla desde las
  4 secciones de `qa-docs`? ¿la autorización se vuelve un gate de código
  auditable o se queda conversacional?
- [ ] Nunca invocar `create_merge_request`/`update_merge_request`/
  `approve_merge_request` reales sin confirmación explícita del usuario en
  ese momento puntual.

### 3C.6 — Smoke E2E final sobre un repositorio real

- [ ] Preguntar al usuario qué repositorio real de automatización usar.
- [ ] Confirmar acceso (checkout local, `project_path`/`target_branch` de
  GitLab, y si se autoriza un entorno real de dev/staging para NIVEL 1).
- [ ] Correr el ciclo completo real `qa-explore → qa-spec → qa-apply →
  qa-verify → qa-docs`, ejercitando 3C.1-3C.5 de verdad.
- [ ] Documentar la evidencia en
  `docs/migration/qa-orchestrator-v3-design.md`, siguiendo el patrón de
  3B.8.

### Transversal — Distinción Stages vs. Herramientas QA standalone (qa-evidence, comandos `/qa-*` en OpenCode)

Surgida en sesión (2026-09-24) a partir de una propuesta del usuario, no parte
del scoping original de 3C.1-3C.6. Separa arquitectónicamente:

- **A. Stages/executors del flujo orquestado** (`qa-explore`, `qa-spec`,
  `qa-apply`, `qa-verify`, `qa-docs`): dependen de `QAStateMachine`, `{change}`
  y `next_action`. Confirmado por lectura directa de los 5 `SKILL.md`.
- **B. Herramientas QA independientes** (`qa-locator-hunting`,
  `qa-doc-reference`, `qa-doc-access`, `qa-evidence` — nueva): invocables
  fuera de un flujo activo y reutilizadas por las stages cuando corresponda.
  Confirmado por lectura directa que las 3 primeras ya son standalone-safe
  hoy (cero referencia a `{change}`/`QAStateMachine`/verbos de ledger); nunca
  deben requerir `{change}` obligatorio ni avanzar el ledger por sí mismas.

`qa-evidence` se evaluó y descartó en 3B por falta de uso confirmado (ver
`docs/migration/qa-orchestrator-v3-design.md:289`). Decisión del usuario en
esta sesión: **sí implementarla ahora**, con alcance acotado — productor del
bundle de evidencia reproducible G6 (ítems 1,2,5,6,7,8: tsc, ejecución,
esperas fijas, Screenplay+POM, comparación BookStack, comando+resultado/
capturas/traces/videos), NUNCA los ítems 3-4 (lint/secretos, que siguen
siendo RDD real dentro de `qa-verify`, sin duplicar — decisión ya cerrada en
3C.3).

- [x] Crear `skills/qa-evidence/SKILL.md` (+ mirror
  `internal/assets/skills/qa-evidence/SKILL.md`): productor standalone del
  bundle de evidencia G6 (ítems 1,2,5,6,7,8), invocable sin `{change}` o
  reutilizado por `qa-verify` cuando lo tiene. Sesión 2026-09-24 (Block 2)
  además: acortó `description` a 159 chars (el frontmatter lint de
  `internal/assets` exige <=160 y el original de 223 lo rompía — hallazgo
  nuevo, no reportado por el work-unit previo) y convirtió la lista
  ordenada 1/2/5/6/7/8 de "Alcance" a bullets `- **Ítem N**` para que
  CommonMark no la renumere 1-6 y desalinee la numeración real de G6.
  Mirrors verificados byte-idénticos (`diff -q`).
- [x] Registrar `SkillQAEvidence` en las 3 fuentes de verdad, mismo patrón
  exacto que las 9 skills existentes: `internal/model/types.go` (const
  block, tras `SkillQADocAccess`), `internal/components/skills/presets.go`
  (`selectableFoundationSkills`), `internal/catalog/skills.go`
  (`MVPSkills()`). Confirmado por lectura directa de los 3 archivos.
- [x] Actualizar `skills/qa-verify/SKILL.md` (+ mirror): delegar los ítems
  1,2,5,6,7,8 del checklist funcional G6 a `qa-evidence` en vez de
  ejecutarlos inline; ítems 3-4 (RDD) sin cambios. Sesión 2026-09-24
  (Block 2) además: el ítem 6 todavía repetía en prosa completa el criterio
  Screenplay+POM que ya vive en `qa-evidence` — se recortó a un puntero de
  una línea (`Screenplay+POM — criterio completo en
  skills/qa-evidence/SKILL.md (Ítem 6)`), sin tocar `qa-gate-policy.md`.
  Mirrors verificados byte-idénticos (`diff -q`).
- [x] Añadir sección "Standalone QA Tool Contract" en
  `skills/_shared/qa-gate-policy.md`, tras "Ledger QA" y antes de "Uso de
  este archivo": may run without active change; must not mutate QA workflow
  state unless explicitly attached to a change; must never advance stages;
  may return evidence/artifacts for later consumption by orchestrated
  stages. Referenciar desde los 4 `SKILL.md` de herramientas standalone
  (`qa-locator-hunting`, `qa-doc-reference`, `qa-doc-access`, `qa-evidence`).
  Confirmado presente como "Contrato de herramientas QA standalone"
  (líneas 129-142).
- [x] Documentar la distinción A/B en `docs/migration/qa-orchestrator-v3-design.md`,
  nueva subsección tras el cierre de 3C.3 (línea 345). Confirmado presente:
  "### Transversal — Stages vs. herramientas QA standalone (2026-09-24)".
- [x] Go: replicar el patrón de `internal/components/sdd/commands.go` +
  `SDDCommandsAssetDir` (`internal/assets/commands.go`) para exponer
  comandos públicos `/qa-*` en OpenCode — SOLO para las 4 herramientas
  standalone + `/qa-supervisor` como entrypoint del flujo completo. NUNCA
  para `qa-explore/spec/apply/verify/docs` (son stages del state machine,
  no comandos standalone). Sesión 2026-09-24 (Block 2): implementado como
  `internal/components/skills/qa_commands.go`
  (`InjectQACommands`/`QACommandPaths`), NO reutilizando
  `internal/assets/opencode/commands/` — ese directorio lo barre
  `sdd.Inject()` paso "2. Write slash commands" vía `fs.ReadDir` sin
  gating de selección de skills (SDD se instala independiente de qué
  skills QA se eligieron), lo que habría escrito los 5 `/qa-*` sin
  respetar selección y roto el conteo exacto `len==13` de
  `TestOpenCodeEmbeddedAssetLayout` (`internal/assets/assets_test.go`).
  Los 5 `.md` reales viven en el directorio nuevo
  `internal/assets/opencode/qa-commands/` en su lugar, con el mismo
  contrato de lectura/escritura (`assets.Read` + `filemerge.WriteFileAtomic`)
  que usa `sdd.Inject()`. Desviación deliberada del texto literal de esta
  tarea y del diseño — documentada aquí y en el reporte de la sesión.
- [x] Wiring en el pipeline real de instalación/sync (mismos call sites que
  SDD: `internal/cli/run.go`, `internal/cli/sync.go`) para que estos
  comandos QA se escriban igual que los de SDD, sin tocar el pipeline SDD
  existente. Sesión 2026-09-24 (Block 2): `skills.InjectQACommands` llamado
  junto a `skills.Inject` en ambos `case model.ComponentSkills:` (run.go y
  sync.go); `skills.QACommandPaths` añadido a
  `componentPathsWithWorkspaceScoped` (`case model.ComponentSkills:`, usado
  por install/uninstall y por `sync.go` vía `syncComponentPathsWithWorkspace`)
  y a `syncAdapterSkillBackupTargets` (snapshot de respaldo pre-sync), para
  que uninstall/rollback no dejen huérfanos ni un backup sin cubrir.
- [x] Tests para invocación standalone (comando `/qa-*` sin `{change}`
  activo) — replicando el patrón de
  `internal/components/sdd/commands_test.go`: nuevo
  `internal/components/skills/qa_commands_test.go` con 5 tests
  (`TestInjectQACommandsWritesStandaloneToolCommandsForOpenCode`,
  `TestStageSkillsNeverGetStandaloneCommands`,
  `TestInjectQACommandsSkipsClaudeCode`,
  `TestInjectQACommandsSelectionGating`,
  `TestQACommandPathsMatchesInjectQACommands`).
  [ ] Pendiente genuino: no se agregó un test a nivel de contenido que
  verifique la invocación desde el flujo orquestado (`qa-verify` delegando
  a `qa-evidence` con `{change}` presente) — no existía antes de esta
  sesión (confirmado por grep) y no estaba en el alcance RED/GREEN
  explícito de esta tarea; queda fuera de este work-unit.
- [x] Actualizar conteos de skills afectados por el 10º skill QA
  (`e2e/e2e_test.sh`, `internal/tui/screens/skill_picker_test.go`,
  `testdata/golden/skills-presets.json`, goldens de TUI) — mismo patrón que
  la corrección de CI ya hecha para las 9 skills originales (PR #32).
  Confirmado ya reflejado en los 3 archivos por lectura directa (trabajo
  previo a esta sesión).

## Ruteo de delegación

Exploración/mapeo inicial de 3C.1-3C.6: delegado vía Workflow (5 lentes en
paralelo + síntesis, `wf_144039f0-626`), evidencia real de código y MCP en
cada hallazgo. Escritura de las correcciones de 3C.1 (skills): directa
inline — ediciones mecánicas de varios archivos, más el diseño aditivo del
fallback Codegen (arquitectura ya resuelta por el orquestador antes de
escribir, no delegada).

## TDD

Modo: estricto (RED→GREEN→REFACTOR), fuente: `CLAUDE.md` del usuario
("Strict TDD Mode: enabled"). 3C.1 no tuvo código Go que cambiar (confirmado
por lectura directa de `internal/qastage`), así que no aplica ciclo
RED/GREEN aquí — solo verificación funcional (tests de paquete +
`diff -q` de paridad). 3C.3 sí tendrá Go real y usará TDD estricto cuando se
resuelva su decisión de arquitectura.

## Próximo paso

Commit del work-unit de 3C.1, luego continuar con 3C.2 (slice sin
Playwright) en la misma sesión o la siguiente.
