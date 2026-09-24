# Documento Vivo ODD — Flujo SDD Multi-Rol, Filtros de Dashboard, Git Worktrees, Sincronización y Mantenimiento

> **Feature:** `gobernanza-flujo-multirol-y-dashboard`  
> **Fichero:** `odd/tasks/gobernanza-flujo-multirol-y-dashboard.md` (fuente de verdad)  
> **Espejo Engram:** `odd/gobernanza-flujo-multirol-y-dashboard/tasks`  
> **Creado:** 2026-09-24 · **Ruta:** carril ágil ODD  

---

## 1. Objetivo

Implementar las mejoras de orquestación, experiencia de usuario y mantenimiento continuo en Axiom:
1. Formalizar el flujo multi-rol estricto (Funcional exhaustivo e interactivo -> Arquitecto con diseño unificado -> Roles de implementación autónomos).
2. Preguntas de transición / bifurcación al final de `spec` y `design` (¿Relevo formal `handoff.md` o continuación directa?).
3. Añadir filtros operacionales en la Web UI (`axiom ui`) para clasificar incrementos según el perfil responsable (Funcional, Arquitecto, Rol específico [Back/Front/QA], Pendiente validación global, Pendiente archivar).
4. Integrar el protocolo de poda progresiva y deprecación de especificaciones históricas en la fase de archivado (`archive-compose`).
5. Proveer comando CLI (`axiom semantic reindex`), endpoint y botón en la Web UI para disparar la reindexación de CodeGraph bajo demanda.
6. Establecer la política diferencial de Git entre repositorios:
   - **Repositorio de specs:** directo a `main`/`master` (sin ramas ni worktrees), con commit y push obligatorio al emitir `handoff.md`.
   - **Repositorios de código:** aislamiento en ramas/worktrees, con pre-vuelo interactivo (usar worktree sí/no, nombre de rama, rama base), oferta de commit y propuesta de PR con **selección libre de rama destino** (`main`, `develop`, etc.), más script/herramienta de limpieza post-merge.
7. Sincronización remota continua del repositorio de especificaciones:
   - **En el arranque del agente:** comprobación automática de `git fetch` en el repo de specs al inicio de cada sesión; si hay cambios remotos, notificar y proponer `git pull` antes de operar.
   - **En la Web UI:** sondeo periódico en segundo plano (`fetch`) sobre el repo de specs, con indicador/alerta visual y botón directo para hacer `pull` si se detectan cambios remotos.

---

## 2. Problema y por qué

- **Ambigüedad funcional:** Si las fases tempranas (`explore`, `proposal`, `spec`) no interrogan activamente al usuario sobre *corner cases*, estados límite y restricciones, el diseño posterior y la implementación sufren retrabajo constante.
- **Cuello de botella y sobrecarga del Arquitecto:** Si el arquitecto debe redactar el checklist de tareas de bajo nivel de backend o frontend, se pierde el conocimiento granular de los especialistas y se satura el contexto del modelo. Los roles técnicos deben ser dueños de su `tasks.<rol>.md` y su `verify-report.<rol>.md`.
- **Falta de visibilidad por perfil en la UI:** Actualmente la UI lista los incrementos en un orden plano sin permitir que un desarrollador de backend o frontend filtre inmediatamente qué cambios están desbloqueados y listos para su rol.
- **Divergencia documental (Doble Fuente de la Verdad):** Al consolidar nuevos requisitos en `specs/<dominio>/spec.md`, si la especificación monolítica antigua no se poda o marca como obsoleta durante el archivado, los agentes reciben señales contradictorias.
- **Desincronización de CodeGraph:** Cuando CodeGraph no detecta cambios automáticamente o se producen saltos de rama en Git, los desarrolladores carecen de un mecanismo visual o por CLI para forzar la reindexación semántica.
- **Proliferación de ramas en specs vs desorden en código:** Los agentes a menudo crean ramas o worktrees innecesarios en el repositorio de documentación (donde la evolución debe ser lineal y directa en `main`), mientras que en los repositorios de código se mezclan ramas locales sin aislamiento ni limpieza automatizada tras el merge.
- **Riesgo de trabajo sobre specs obsoletas:** En equipos donde varios compañeros o agentes modifican las especificaciones, iniciar una sesión sin sincronizar el repo de specs conduce a implementar requisitos desactualizados o resolver conflictos dolorosos de Git.
- **Rigidez en destino de Pull Requests:** Los flujos no siempre entregan a `main`; muchos proyectos utilizan `develop`, `release/*` o ramas de integración por sprint. El sistema debe permitir elegir la rama destino.

---

## 3. Alcance autorizado

**Dentro:**
- Actualización de doctrinas y prompts de habilidades (`sdd-explore`, `sdd-propose`, `sdd-spec`) para exigir interrogación interactiva y preguntas de relevo (`handoff` vs continuar).
- Actualización de la doctrina `sdd-design` para preguntar al validar si se emite relevo a roles o se auto-implementa (fullstack).
- Adaptación en `internal/dashboard/service.go` y la Web UI (`index.html`, `app.js`, `style.css`) de los nuevos filtros de estado e incrementos por rol (`all`, `spec_pending`, `design_ready`, `role_pending:<rol>`, `verify_pending`, `archive_pending`).
- Hook o paso en `sdd-archive` y `axiom sdd archive-compose` para registrar secciones deprecadas en specs monolíticas previas.
- Endpoint `POST /api/semantic/reindex`, comando `axiom semantic reindex` y botón reactivo en la pestaña Semántica de la UI.
- Protocolo de Git por tipo de repositorio:
  - Specs: bloqueo de creación de ramas/worktrees en `specs_repository`; commit y push asistido o automático al cerrar `spec` y `design` en `handoff.md`.
  - Código: cuestionario interactivo de arranque de rol para worktree/rama, commit semántico al verificar, propuesta de PR con **selección interactiva de rama destino** (`gh pr create --base <rama-elegida>`) y script auxiliar de limpieza post-merge (`scripts/cleanup-worktree.sh` o CLI).
- Sincronización continua de specs:
  - Directiva de arranque de sesión: `git fetch` en `specs_repository`, detección de `behind > 0`, notificación y propuesta de `pull`.
  - Endpoint `GET /api/workspace/specs/sync-status` y `POST /api/workspace/specs/pull` en `internal/dashboard/service.go`.
  - Componente visual de alerta y botón de `Pull` en la cabecera del dashboard Web.
- Tests unitarios y de integración para los nuevos filtros del servicio de dashboard, el comando semántico, la sincronización de specs y la política de ramas.

**Fuera:**
- Reescribir o modificar los archivos de código fuente de los repositorios de usuario (`backend/`, `frontend/`, `e2e/`).
- Eliminar de forma destructiva o no supervisada ficheros Markdown legacy.
- Reemplazar el binario externo de CodeGraph (solo se orquesta su invocación/reindexación).

---

## 4. Restricciones activas

| Restricción | Valor | Fuente |
|---|---|---|
| Idioma de artefactos | Español (castellano peninsular) estricto | REGLA SUPREMA Axiom |
| No destrucción | Preservación de documentos preexistentes | Filosofía Axiom |
| Determinismo | Estados de filtro derivados de los artefactos en disco | `internal/dashboard/service.go` |
| Verificación | Pruebas funcionales directas en Go (`go test ./...`) | TDD / Verificación Go |
| Git en Specs | Directo a `main` / `master` (sin ramas ni worktrees, siempre sincronizado) | Política de Producto |
| Git en Código | Ramas / Worktrees aislados con ciclo de PR hacia rama configurable | Política de Producto |

---

## 5. Checklist de tareas (IDs estables)

### Bloque A: Doctrinas de Habilidades y Relevos Inter-Fase
- [x] **ODD-1.1 — Directiva de Interrogación Exhaustiva (Funcional).** Modificar los prompts de `sdd-explore`, `sdd-propose` y `sdd-spec` para forzar rondas interactivas de aclaración de *edge cases*, tasas, errores y criterios BDD.
- [x] **ODD-1.2 — Bifurcación Post-Spec.** Añadir la pregunta de decisión al validar `spec`: `¿Generar handoff para Arquitecto o continuar directamente hacia design.md?`.
- [x] **ODD-1.3 — Bifurcación Post-Design.** Añadir la pregunta de decisión al validar `design`: `¿Generar handoff a los roles de implementación o iniciar implementación directa?`.
- [x] **ODD-1.4 — Doctrina de Autonomía de Roles.** Establecer en la guía de `sdd-tasks` que cada rol (`tasks.<rol>.md`) es desglosado por el especialista correspondiente a partir de los contratos de `design.md`.

### Bloque B: Filtros Operacionales en Dashboard Web (`axiom ui`)
- [x] **ODD-2.1 — Modelo de Clasificación de Estados en Go.** Ampliar `internal/dashboard/service.go` para enriquecer `IncrementSummaryDTO` con banderas de estado operativo (`ReadyForDesign`, `WaitingRoles`, `ReadyForGlobalVerify`, `ReadyForArchive`).
- [x] **ODD-2.2 — Filtro Dinámico por Roles.** Calcular en `GetIncrements()` qué roles declarados en `axiom.yaml` tienen tareas pendientes (`tasks.<rol>.md` o compuerta `role-apply:<rol>` sin aprobar).
- [x] **ODD-2.3 — Superficie Web UI.** Añadir en `internal/dashboard/assets/index.html` y `app.js` la botonera de filtros:
  - `Todos`
  - `Pendiente Spec` (Funcional)
  - `Listo para Design` (Arquitecto)
  - Botones dinámicos por rol: `[Backend]`, `[Frontend]`, `[QA]`
  - `Pendiente Verificación Global`
  - `Listo para Archivar`
- [x] **ODD-2.4 — Tests del Servicio de Dashboard.** Añadir tests unitarios que verifiquen el filtrado correcto según la existencia de `proposal.md`, `spec.md`, `design.md`, `tasks.<rol>.md` y compuertas.

### Bloque C: Reindexación de CodeGraph
- [x] **ODD-3.1 — Motor de Reindexación en Go.** Implementar en `internal/semantic/service.go` el método `ReindexCodeGraph(ctx context.Context)` que ejecute la invocación segura de `codegraph index` o su herramienta MCP equivalente con control de timeout.
- [x] **ODD-3.2 — Comando CLI.** Añadir el subcomando `axiom semantic reindex` en `cmd/axiom/main.go`.
- [x] **ODD-3.3 — API REST y Botón en UI.** Crear el endpoint `POST /api/semantic/reindex` en `internal/dashboard/service.go` y añadir el botón «Reindexar CodeGraph» con indicador de carga en `internal/dashboard/assets/index.html` y listener en `app.js`.

### Bloque D: Poda Progresiva en Archivado
- [x] **ODD-4.1 — Gancho de Poda en `archive-compose`.** Extender `axiom sdd archive-compose` con `--supersede` y `--superseded-requirements` para registrar secciones superadas con la anotación `[SUPERSEDED / DEPRECADO]` sin purgarlas a ciegas.
- [x] **ODD-4.2 — Trazabilidad en `archive-report.md`.** Incluir en la plantilla de reporte de archivado de `sdd-archive/SKILL.md` una sección dedicada: `## Especificaciones Históricas Superadas` para documentar qué partes del legado fueron podadas.

### Bloque E: Política Git Diferencial, Sincronización Remota y Ciclo de Vida de Worktrees
- [x] **ODD-5.1 — Política para el Repositorio de Specs (Directo a Main).** Configurar en los agentes, reglas y habilidades que el repositorio `specs_repository` jamás crea ramas ni worktrees; opera directamente en la rama principal (`main`/`master`).
- [x] **ODD-5.2 — Commit y Push en Handoff de Specs/Design.** Actualizar `sdd-spec` y `sdd-design` para comprobar y requerir que los cambios de especificación y diseño estén commiteados y pusheados a remoto antes de ceder el testigo.
- [x] **ODD-5.3 — Sincronización al Inicio de Sesión del Agente.** Directiva de arranque en `AGENTS.md` y `GEMINI.md` para que el agente ejecute `git fetch` en el repositorio de specs. Si `behind > 0`, avisar de inmediato al usuario y solicitar autorización para ejecutar `git pull`.
- [x] **ODD-5.4 — Sincronización Remota Continua en Web UI.** Implementar en `internal/dashboard/service.go` los endpoints `GET /api/workspace/specs/sync-status` y `POST /api/workspace/specs/pull`. Añadir alerta visual destacada (`specs-sync-banner`) en la cabecera de la UI con botón `[Hacer Pull]` si hay cambios remotos.
- [x] **ODD-5.5 — Pre-vuelo de Worktrees en Roles de Código.** En la doctrina de arranque de implementación por rol (`sdd-apply`), formular la pregunta de pre-vuelo: (1) ¿worktree aislado vs árbol actual?, (2) nombre de rama (`feat/...`), (3) rama base (`main`, `develop`, etc.).
- [x] **ODD-5.6 — Cierre de Rol con Selección de Rama Destino en PR.** Al validar el informe `verify-report.<rol>.md` en `PASS`, ofrecer la creación del commit y preguntar la rama destino deseada (`develop`, `main` u otra) antes de proponer `gh pr create --base <rama-destino>`.
- [x] **ODD-5.7 — Script / Herramienta de Limpieza de Worktrees.** Proveer scripts auxiliares (`scripts/cleanup-worktree.sh` y `scripts/cleanup-worktree.ps1`) para que el desarrollador pueda eliminar limpiamente el worktree y la rama local una vez confirmado el merge a la rama destino.

---

## 6. Criterios de aceptación (globales)

1. En la Web UI (`axiom ui`), al pulsar el botón de un rol (ej. `[Backend]`), la lista muestra única y exclusivamente aquellos incrementos que tienen `design.md` validado pero cuyo rol aún no ha completado sus tareas o compuerta.
2. Los agentes de `sdd-explore`, `sdd-propose` y `sdd-spec` nunca crean ramas ni worktrees dentro del repositorio de especificaciones; realizan commit directo a `main` y verifican el `push` al emitir el relevo `handoff.md`.
3. Al arrancar una sesión de agente, se ejecuta `git fetch` en el repositorio de specs y se notifica inmediatamente si existen cambios remotos entrantes antes de que el agente lea o escriba nada.
4. La Web UI realiza comprobaciones de `git fetch` sobre el repositorio de specs y muestra una alerta visual destacada con botón de `[Hacer Pull]` cuando el repositorio local está por detrás del remoto.
5. Al iniciar la implementación de un rol en un repositorio de código, el agente pregunta si crear worktree, el nombre de la nueva rama y la rama base de origen.
6. Al verificar satisfactoriamente el rol (`verdict: pass`), el agente ofrece commitear y pregunta la rama destino antes de proponer la creación del Pull Request.
7. Se incluye un script/herramienta de limpieza que elimina el worktree y la rama local de forma segura tras el merge en la rama base.
8. El comando `axiom semantic reindex` y el botón en la Web UI ejecutan la reindexación de CodeGraph sin errores.
9. Todos los tests de `internal/dashboard`, `internal/semantic` y `cmd/axiom` pasan en verde sin regresiones.

---

## 7. Comprobaciones aplicables

- `go test ./internal/dashboard/...`
- `go test ./internal/semantic/...`
- `go test ./internal/sddstatus/...`
- `go test ./internal/cli/...`
- `go vet ./...`
- `gofmt -l <archivos tocados>`

---

## 8. Decisiones abiertas

- **D-1 (Resuelta):** Los filtros por rol en la UI se derivan dinámicamente de los roles declarados en `axiom.yaml` (o de los roles detectados en los incrementos activos).
- **D-2 (Resuelta):** La poda de la spec original en el archivado es asistida por confirmación, registrando el reemplazo en `archive-report.md`.
- **D-3 (Resuelta):** El repositorio de specs opera estrictamente en `main` sin ramas; los repositorios de código usan aislamiento por rama/worktree y propuesta de PR hacia rama configurable (`develop`, `main`, etc.).
- **D-4 (Resuelta):** La sincronización del repo de specs se monitoriza tanto por CLI al inicio de sesión del agente como por polling pasivo en la Web UI con botón de pull directo.

---

## 9. Progreso

| Unidad | Estado | Evidencia |
|---|---|---|
| Creación y Actualización de Documento Vivo ODD | ✅ | `odd/tasks/gobernanza-flujo-multirol-y-dashboard.md` registrado y sincronizado en Engram |
| Bloque A (Doctrinas & Relevos) | ✅ | Prompts de `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design` y `sdd-tasks` actualizados con interrogación exhaustiva y bifurcaciones post-spec/post-design |
| Bloque B (Filtros UI & Dashboard) | ✅ | Banderas operacionales en `IncrementSummaryDTO`, cálculo en `inspectIncrement()`, botones en `index.html` y filtrado reactivo en `app.js`. Test `TestIncrementSummaryOperationalFlagsAndRoleFiltering` en verde |
| Bloque C (Reindexación CodeGraph) | ✅ | Método `ReindexCodeGraph()`, CLI `axiom semantic reindex`, endpoint `POST /api/semantic/reindex`, botón reactivo en UI. Tests `TestServiceReindexCodeGraph` y `TestSemanticEndpoints` en verde |
| Bloque D (Poda en Archivado) | ✅ | `ComposeOpenSpecCanonicalSpecWithOptions` y banderas `--supersede`/`--superseded-requirements` en `archive-compose`. Sección `## Especificaciones Históricas Superadas` en `sdd-archive/SKILL.md`. Tests `TestComposeOpenSpecCanonicalSpecSupersedePruning` y `TestRunSDDArchiveComposeSupersedeFlag` en verde |
| Bloque E (Git Specs vs Código, Sync & Worktrees) | ✅ | Doctrinas ODD-5.1 a 5.6 en `AGENTS.md`, `GEMINI.md`, `sdd-apply`, `sdd-spec`, `sdd-design`. Endpoints `sync-status` y `pull` en `internal/dashboard` con alerta visual en UI. Scripts `scripts/cleanup-worktree.sh` y `.ps1`. Test `TestSpecsSyncStatusEndpoint` en verde |

---

## 10. Próximo paso

Verificación final completa de suite de pruebas y entrega del informe consolidado.
