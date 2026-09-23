# Roadmap de Incrementos — Axiom

> **Visión y Arquitectura:** [AXIOM_ENTERPRISE_VISION.md](architecture/AXIOM_ENTERPRISE_VISION.md)  
> **Estado General:**  
> - **Fase 1: Fundación de la Plataforma e Identidad Base:** ✅ 100% Completada (7/7 Incrementos Archivados)  
> - **Fase 2: Autonomía, Identidad, Multi-Proyecto y Orquestación SDD:** ✅ 100% Completada (8/8 Incrementos Archivados)  
> - **Fase 3: Experiencia de Usuario, Localización y Ecosistema:** ✅ 100% Completada (2/2 Incrementos Archivados)  
> - **Fase 4: Evolución Arquitectónica, Flujo Orgánico (ODD), Absorción Upstream v3 y Ecosistema:** ✅ 100% Completada (5/5 Incrementos Archivados)  
> **Total de Incrementos Archivados:** 22 de 22 (INC-01 a INC-22)  
> **Total de Incrementos En Progreso:** 0  
> **Última Actualización:** 2026-09-23 (Archivado definitivo)

---

## Leyenda de Estados

- `⏳ En progreso`: Incremento actualmente en desarrollo activo dentro de su ciclo SDD.
- `📋 Planificado`: Incremento definido con alcance y listo para iniciar cuando corresponda.
- `✅ Archivado`: Incremento completado, verificado al 100% y formalmente archivado en la especificación viva.
- `⏸️ Pausado / En espera`: Incremento a la espera de dependencias previas o decisiones de producto.

---

## Catálogo de Incrementos

### Fase 1: Fundación de la Plataforma e Identidad Base

| ID | Incremento | Estado | Responsabilidad | Descripción Resumida |
| :--- | :--- | :---: | :---: | :--- |
| **INC-01** | `axiom-identity-workspace-topology` | ✅ Archivado | Arquitectura / Core | Creación del binario `axiom`, esquema `axiom.yaml`, validación de topologías (monorepo-embedded, monorepo-decoupled, multirepo) y repositorio canónico de specs. |
| **INC-02** | `structured-handoffs-lifecycle` | ✅ Archivado | Core SDD / Workflow | Esquema canónico `handoff.md`, motor Go `internal/handoff`, subcomandos CLI `axiom handoff show/create/validate` y espejo Engram. |
| **INC-03** | `multi-role-sdd-fan-out` | ✅ Archivado | Core SDD / Roles | Concurrencia de roles en `Design`, `tasks.<rol>.md`, políticas de compuerta (`blocking`/`deferred`), barrera de sincronización y migración diferida de tareas acumulativas. |
| **INC-04** | `axiom-local-web-dashboard` | ✅ Archivado | UI / Experiencia | Servidor HTTP local embebido en Go con dashboard web: tablero de incrementos, estado de roles, visor de handoffs y buzón de skills. |
| **INC-05** | `autoskills-catalog-and-mining` | ✅ Archivado | Skills / Inteligencia | Catálogo de skills por tecnología detectada (midudev/autoskills) y minería heurística de código local con gobernanza Human-in-the-Loop. |
| **INC-06** | `semantic-code-serena-codegraph` | ✅ Archivado | Semántica / Herramientas | Conector local con Serena MCP y CodeGraph para navegación y consultas semánticas de código en `Explore` y `Design`. |
| **INC-07** | `archive-living-documentation-engine` | ✅ Archivado | Documentación / SDD | `Archive` como mantenedor continuo de especificaciones existentes y generador incremental de documentación viva en proyectos no documentados. |

### Fase 2: Autonomía, Identidad, Multi-Proyecto y Orquestación SDD

| ID | Incremento | Estado | Responsabilidad | Descripción Resumida |
| :--- | :--- | :---: | :---: | :--- |
| **INC-08** | `multi-project-hub-and-init` | ✅ Archivado | Hub / Multi-Proyecto | Registro central de workspaces (`~/.axiom/workspaces.json`), CLI `axiom init` con autodetección tecnológica, `axiom project` y selector de proyectos en Dashboard Web. |
| **INC-09** | `visual-decoupling-opencode-cleanup` | ✅ Archivado | Identidad Visual | Retiro de `gentle-logo.tsx` en OpenCode, neutralización de inyección forzada de temas (`axiom.json`), y normalización de rutas limpias de agentes (`axiom.*`). |
| **INC-10** | `axiom-persona-and-spanish-contract` | ✅ Archivado | Agentes / Identidad | Nueva Persona oficial `axiom` (arquitecto riguroso en castellano peninsular), eliminación de voseo y contrato estricto de artefactos SDD en español. |
| **INC-11** | `orchestrator-slash-commands-and-markers` | ✅ Archivado | Orquestador / SDD | Renombrado a `axiom-orchestrator` en OpenCode, normalización de comandos slash `/sdd-*` en Claude Code, migración dual de marcadores `axiom:` y centinela `.axiom-instance`. |
| **INC-12** | `unified-axiom-user-state-and-env` | ✅ Archivado | Core / Configuración | Unificación de estado bajo `~/.axiom/`, variables de entorno canónicas `AXIOM_*` y migración automática transparente desde `~/.gentle-ai/`. |
| **INC-13** | `sdd-commands-axiom-cli-integration` | ✅ Archivado | CLI / Ciclo de Vida | Integración de comandos SDD (`axiom sdd status/continue/attempt/verify-validate/archive-compose`) y ciclo de revisión RDD nativamente en el binario `axiom`. |
| **INC-14** | `axiom-tui-branding-and-cli-cutover` | ✅ Archivado | TUI & Ecosistema | Unificación de la TUI interactiva Bubbletea (`axiom tui`), comandos de aprovisionamiento (`install`, `sync`, `upgrade`, `doctor`, `backup`, `restore`) y pasarela gentle-ai. |
| **INC-15** | `dashboard-interactive-sdd-orchestration` | ✅ Archivado | Web UI & Orquestación | Orquestación interactiva SDD en el Dashboard Web (`axiom ui`) con acciones en un clic (`continue`, `verify-validate`, `archive-compose`) y CLI canónico `axiom change create`. |

### Fase 3: Experiencia de Usuario, Localización y Ecosistema

| ID | Incremento | Estado | Responsabilidad | Descripción Resumida |
| :--- | :--- | :---: | :---: | :--- |
| **INC-16** | `tui-spanish-localization` | ✅ Archivado | TUI & Experiencia | Localización completa al castellano peninsular de menús y pantallas de la TUI, y desacoplamiento de feeds de avisos externos. |
| **INC-17** | `bidirectional-tui-ui-parity` | ✅ Archivado | TUI, Web UI & Ecosistema | Paridad bidireccional de funcionalidades: panel de ecosistema (doctor, sync, upgrade, backups, modelos) en Web UI y gobernanza SDD (proyectos, incrementos, roles, handoffs, living docs) en TUI. |

### Fase 4: Evolución Arquitectónica, Flujo Orgánico (ODD) y Absorción Upstream v3

> [!NOTE]
> **Repositorio de Referencia Upstream (v3):** El código fuente de la versión más reciente de Gentle-AI (v3.0.2+) se encuentra disponible localmente en `c:/repos/gentle-ai` para consultas de código, diffs y análisis de Git.

| ID | Incremento | Estado | Responsabilidad | Descripción Resumida |
| :--- | :--- | :---: | :---: | :--- |
| **INC-18** | `rdd-decoupling-and-v3-stability-fixes` | ✅ Archivado | Core SDD / Estabilidad | Desacoplamiento de RDD del motor de estados SDD y absorción de parches de estabilidad upstream (rutas JSON Windows, aislamiento CWD, saneamiento de presets de skills y Engram session recovery). |
| **INC-19** | `inc-19-odd-workflow-and-promotion` | ✅ Archivado | ODD & Puerta de Promoción / Experiencia | Carril ágil ODD operativo (documento vivo `odd/tasks/<feature>.md`, espejo de solo lectura en Engram, CLI `axiom odd create`, `status` y `promote`, superficie en Web UI y TUI) y puerta de promoción no destructiva hacia el carril formal SDD. |
| **INC-20** | `inc-20-upstream-reconciliation` | ✅ Archivado | Arquitectura / Integración Upstream / Distribución | Reconciliación auditada del fork con `Gentleman-Programming/gentle-ai` (91 commits absorbidos); publicación de release `v3.5.0` bajo identidad de Axiom; binarios y procedencia firmada; migración Go module `/v3`. |
| **INC-21** | `inc-21-upfront-flow-governance` | ✅ Archivado | Orquestador / Gobernanza SDD | Determinación temprana de carril ODD/SDD; sellado de kickoff (`axiom sdd kickoff seal`); cuatro compuertas de revisión por bloque (`axiom sdd gate record/show`); precondición de integración verificable antes de archive; e inmutabilidad post-archive. |
| **INC-22** | `inc-22-axiom-updater-and-skills-index-governance` | ✅ Archivado | Actualizador / Skills / Ecosistema | Actualizador autónomo y resiliente (`sourceBuildUpgrade`); encadenamiento `upgrade` ➔ `sync` en Web UI y TUI; índice unificado de skills en tres destinos (`AGENTS.md`, registro y Engram); y retiro de la exigencia obsoleta de issue aprobado. |


---

## Registro de Cambios y Entregables

### [INC-01] axiom-identity-workspace-topology (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-01-axiom-identity-workspace-topology/`
- **Especificación viva:** `openspec/specs/workspace-topology/spec.md`
- **Entregables clave:**
  1. Punto de entrada `cmd/axiom/main.go` con comandos CLI (`axiom --version`, `axiom workspace validate`).
  2. Especificación canónica del esquema `axiom.yaml` con roles (1 a N) y mapeo a rutas locales.
  3. Validación de la regla de topologías (comprobación de carpeta maestra y presencia de repo de specs en multirepo/monorepo desacoplado).
  4. Suite de pruebas unitarias en Go para el validador de topología de workspace.

### [INC-02] structured-handoffs-lifecycle (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-02-structured-handoffs-lifecycle/`
- **Especificación viva:** `openspec/specs/structured-handoffs/spec.md`
- **Entregables clave:**
  1. Paquete de dominio Go `internal/handoff/`: tipos, parser bidireccional, formateador canónico, motor de validación semántica de transiciones de fase y exportador de espejo para Engram MCP.
  2. Subcomandos CLI `axiom handoff show`, `create` y `validate` en `cmd/axiom/main.go`.
  3. Batería completa de pruebas unitarias para `internal/handoff/` con cobertura exhaustiva de casos válidos y rechazo de errores.
  4. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 7/7 requerimientos, 13/13 escenarios BDD).

### [INC-03] multi-role-sdd-fan-out (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-03-multi-role-sdd-fan-out/`
- **Especificación viva:** `openspec/specs/multi-role-fan-out/spec.md`
- **Entregables clave:**
  1. Paquete de dominio Go `internal/multirole/`: modelos de gobernanza (`GatePolicy`, `RoleAssignment`, `BarrierReport`), extractor de roles en `design.md` y evaluador de barrera de sincronización (*Fan-In*).
  2. Mecanismo de compuertas diferenciadas: roles obligatorios (`blocking`) vs roles asíncronos/diferidos (`deferred`).
  3. Motor de migración diferida de tareas pendientes (ej. E2E) hacia el backlog acumulativo continuo `openspec/changes/e2e-cumulative/tasks.md` con etiqueta de trazabilidad `[Ref: <cambio>]`.
  4. Subcomandos CLI `axiom role list`, `axiom role status` y `axiom role barrier` (con bandera `--migrate-deferred`) en `cmd/axiom/main.go`.
  5. Batería de 10 pruebas unitarias con 100% PASS en `internal/multirole/`.
  6. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 8/8 requerimientos, 13/13 escenarios BDD).

### [INC-04] axiom-local-web-dashboard (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-04-axiom-local-web-dashboard/`
- **Especificación viva:** `openspec/specs/local-web-dashboard/spec.md`
- **Entregables clave:**
  1. Paquete Go `internal/dashboard/`: servidor HTTP `net/http` con router REST JSON (`/api/workspace`, `/api/increments`, `/api/increments/{name}`, `/api/roles`, `/api/handoffs`, `/api/skills`), capa de servicio agregadora y fallback automático ante puertos ocupados.
  2. Frontend SPA responsivo en `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`) embebido directamente en el binario Go mediante `//go:embed` con cero dependencias de NodeJS o NPM.
  3. Subcomando CLI `axiom ui` con banderas `--port`, `--no-browser` y `--path`, apertura automática del navegador del sistema operativo y *graceful shutdown* con `Ctrl+C`.
  4. Batería de 9 pruebas unitarias con 100% PASS en `internal/dashboard/`.
  5. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 8/8 requerimientos, 15/15 escenarios BDD).

### [INC-05] autoskills-catalog-and-mining (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-14-inc-05-autoskills-catalog-and-mining/`
- **Especificación viva:** `openspec/specs/autoskills/spec.md`
- **Entregables clave:**
  1. Paquete de dominio Go `internal/autoskill/`: catálogo de tecnologías (`SKILLS_MAP`), cliente HTTP nativo con verificación criptográfica **SHA-256** contra el registro oficial de `midudev/autoskills`, motor de detección multi-rol, analizador heurístico de código local (`table-driven-tests`, `internal-layering`, `idiomatic-error-wrapping`) y gestor de buzón transitorio (`.axiom/skills/inbox/`).
  2. Extensión del servidor y API REST de `internal/dashboard/` con rutas `/api/skills/inbox`, `/api/skills/scan`, `/api/skills/approve` y `/api/skills/reject`.
  3. Panel interactivo en el Dashboard Web SPA (`index.html`, `style.css`, `app.js`) con insignias visuales de procedencia (`midudev auditado` vs `minería local`), validación SHA-256, visor de directrices y botones de aprobación/rechazo en un clic.
  4. Subcomandos CLI `axiom skill scan`, `axiom skill list [--inbox]`, `axiom skill approve <nombre>` y `axiom skill reject <nombre>` en `cmd/axiom/main.go`.
  5. Baterías completas de pruebas unitarias en `internal/autoskill/` (5 tests) e `internal/dashboard/` (9 tests) con 100% PASS.
  6. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 13/13 requerimientos, 17/17 escenarios BDD).

### [INC-06] semantic-code-serena-codegraph (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-15-inc-06-semantic-code-serena-codegraph/`
- **Especificación viva:** `openspec/specs/semantic-code/spec.md`
- **Entregables clave:**
  1. Paquete de dominio Go `internal/semantic/`: modelo canónico de símbolos (`types.go`), detector de conectores externos y agentes compatibles (`detector.go`), motor semántico nativo Go AST sin dependencias (`engine.go`) y servicio de alto nivel con fallback automático y deduplicación (`service.go`).
  2. Extensión del servidor y API REST de `internal/dashboard/` con rutas `/api/semantic/status`, `/api/semantic/symbols` y `/api/semantic/dependencies`.
  3. Panel interactivo en el Dashboard Web SPA (`index.html`, `style.css`, `app.js`) con nueva pestaña "Semántica & Grafo", tarjetas de estado del conector, detector de agentes MCP, buscador reactivo de símbolos con filtros por tipo (`struct`, `interface`, `func`, `method`) y visor de dependencias entre paquetes.
  4. Subcomandos CLI `axiom semantic status`, `axiom semantic symbols [--query <filtro>] [--kind <tipo>]` y `axiom semantic inspect` en `cmd/axiom/main.go`.
  5. Baterías completas de pruebas unitarias en `internal/semantic/` (3 tests) e `internal/dashboard/` (10 tests) con 100% PASS.
  6. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 11/11 requerimientos, 14/14 escenarios BDD).

### [INC-07] archive-living-documentation-engine (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-15-inc-07-archive-living-documentation-engine/`
- **Especificación viva:** `openspec/specs/living-documentation/spec.md`
- **Catálogo maestro:** `openspec/INDEX.md` (35 especificaciones vivas, 254 requerimientos canónicos, 412 escenarios BDD)
- **Entregables clave:**
  1. Paquete de dominio Go `internal/livingdoc/`: modelos de catálogo y sincronización (`types.go`), indexador sintáctico determinista de specs y generador de `INDEX.md` (`indexer.go`), motor de adopción orgánica *Zero-Doc Cold Start* (`synthesizer.go`) y capa de servicio del workspace (`service.go`).
  2. Extensión del servidor y API REST de `internal/dashboard/` con rutas `/api/archive/specs`, `/api/archive/specs/{domain}` y `/api/archive/sync`.
  3. Panel interactivo en el Dashboard Web SPA (`index.html`, `style.css`, `app.js`) con nueva pestaña "Especificaciones Vivas", métricas agregadas de dominios y requerimientos, selector de especificaciones, visor de Markdown y botón de sincronización reactiva con un clic.
  4. Subcomandos CLI `axiom archive sync`, `axiom archive list`, `axiom archive show` y `axiom archive coldstart` en `cmd/axiom/main.go`.
  5. Baterías completas de pruebas unitarias en `internal/livingdoc/` (3 tests) e `internal/dashboard/` (11 tests) con 100% PASS.
  6. Generación del catálogo maestro inicial `openspec/INDEX.md` integrando los 7 incrementos fundacionales y especificaciones de dominio de la plataforma.
  7. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 10/10 requerimientos, 13/13 escenarios BDD).
  8. **HITO HISTÓRICO:** Culminación completa de la Fase 1 del Roadmap de Axiom (7 de 7 incrementos concluidos y archivados).

---

### [INC-08] multi-project-hub-and-init (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-15-inc-08-multi-project-hub-and-init/`
- **Especificación viva:** `openspec/specs/multi-project-hub/spec.md`
- **Entregables clave:**
  1. Paquete de dominio Go `internal/hub/`: gestor central `HubManager`, detector tecnológico `Detector` (Go, Node/TypeScript, .NET, Rust, Python), andamiaje y registro `~/.axiom/workspaces.json`.
  2. Subcomandos CLI `axiom init` (con detección automática de stack y generación de `axiom.yaml`) y `axiom project [list|switch|add|remove]`.
  3. Evolución del Dashboard Web (`axiom ui`) a Hub Multi-Proyecto: selector interactivo de proyectos en el Navbar, cambio en caliente y pantalla de bienvenida para carpetas sin inicializar (*Zero-Doc Welcome Screen*).
  4. Baterías completas de pruebas unitarias en `internal/hub/` (6 tests) y `internal/dashboard/` (11 tests) con 100% PASS.
  5. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 6/6 requerimientos, 10/10 escenarios BDD).

### [INC-09] visual-decoupling-opencode-cleanup (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-15-inc-09-visual-decoupling-opencode-cleanup/`
- **Especificación viva:** `openspec/specs/visual-decoupling/spec.md`
- **Entregables clave:**
  1. Retiro completo del plugin de logo heredado (`gentle-logo.tsx`) en OpenCode y purga en instalador y desinstalador.
  2. Neutralización de inyección de temas: eliminación de `"theme": "gentleman"` forzado en `settings.json` y renombrado de temas a `axiom.json` y `axiom-dark.json`.
  3. Desacoplamiento de rutas de agentes: actualización a `axiom.instructions.md` (VS Code Copilot), `~/.kiro/steering/axiom.md` (Kiro) y `~/.pi/axiom/` (Pi).
  4. Saneamiento de activos de marca en documentación y `README.md`.
  5. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 5/5 requerimientos, 8/8 escenarios BDD).

### [INC-10] axiom-persona-and-spanish-contract (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-15-inc-10-axiom-persona-and-spanish-contract/`
- **Especificación viva:** `openspec/specs/persona-spanish-contract/spec.md`
- **Entregables clave:**
  1. Incorporación de la Persona oficial `PersonaAxiom = "axiom"` en `internal/model/types.go` y activos de prompt para todos los agentes soportados.
  2. Tono de identidad: Arquitecto de sistemas principal, riguroso, pedagógico, en **castellano peninsular de España** con tuteo profesional técnico (eliminando el voseo rioplatense forzado).
  3. Adopción de `PersonaAxiom` como predeterminada en presets y menús interactivos de selección.
  4. Reforma del contrato lingüístico de orquestadores (`sdd-orchestrator-sections.md`): todos los artefactos SDD se generan por defecto en español, preservando inglés únicamente para identificadores de código.
  5. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 6/6 requerimientos, 9/9 escenarios BDD).

### [INC-11] orchestrator-slash-commands-and-markers (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-11-orchestrator-slash-commands-and-markers/`
- **Especificación viva:** `openspec/specs/orchestrator-commands-markers/spec.md`
- **Entregables clave:**
  1. Renombrado del orquestador canónico en OpenCode a `axiom-orchestrator`, preservando alias retrocompatible de transición.
  2. Normalización de comandos slash de Claude Code: retiro del prefijo heredado `gentle-sdd-` y despliegue canónico de 11 comandos `/sdd-*` (`/sdd-explore`, `/sdd-propose`, `/sdd-spec`, `/sdd-design`, `/sdd-tasks`, `/sdd-apply`, `/sdd-verify`, `/sdd-archive`, etc.).
  3. Migración de marcadores de sección Markdown hacia `<!-- axiom: ... -->` con lectura tolerante y centinela de workspace `.axiom-instance`.
  4. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 4/4 requerimientos, 5/5 escenarios BDD).

### [INC-12] unified-axiom-user-state-and-env (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-12-unified-axiom-user-state-and-env/`
- **Especificación viva:** `openspec/specs/axiom-user-state-and-env/spec.md`
- **Entregables clave:**
  1. Unificación de todo el estado de usuario bajo `~/.axiom/` (`state.json`, `workspaces.json`, backups y skills).
  2. Soporte para variables de entorno canónicas `AXIOM_*` (`AXIOM_HOME`, `AXIOM_CONFIG_DIR`, `AXIOM_STATE_DIR`, `AXIOM_LOG_DIR`, `AXIOM_BACKUP_DIR`).
  3. Motor de migración automática transparente y no destructiva de datos preexistentes desde `~/.gentle-ai/`.
  4. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 3/3 requerimientos, 5/5 escenarios BDD).

### [INC-13] sdd-commands-axiom-cli-integration (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-13-sdd-commands-axiom-cli-integration/`
- **Especificación viva:** `openspec/specs/axiom-sdd-cli-integration/spec.md`
- **Entregables clave:**
  1. Integración de comandos SDD bajo `axiom`: `axiom sdd status`, `axiom sdd continue`, `axiom sdd attempt`, `axiom sdd verify-validate`, `axiom sdd archive-compose`, `axiom sdd task-result` y `axiom sdd preflight-hook`.
  2. Integración del ciclo de revisión RDD: `axiom review mode`, `start`, `resume`, `step` y `validate`.
  3. Soporte para alias planos directos (`axiom sdd-status`, `axiom sdd-continue`, etc.).
  4. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 4/4 requerimientos, 5/5 escenarios BDD).

### [INC-14] axiom-tui-branding-and-cli-cutover (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-14-axiom-tui-branding-and-cli-cutover/`
- **Especificación viva:** `openspec/specs/axiom-tui-branding/spec.md`
- **Entregables clave:**
  1. Unificación de la TUI interactiva Bubbletea bajo `axiom tui` y arranque automático en terminales TTY interactivos (`axiom`).
  2. Integración de los comandos de gestión del ecosistema en el binario `axiom`: `axiom install`, `axiom sync`, `axiom upgrade`, `axiom doctor`, `axiom backup`, `axiom restore`, `axiom uninstall`.
  3. Renovación de banners y estilos visuales de la TUI con identidad limpia Axiom.
  4. Pasarela transparente de fallback para comandos `gentle-ai` redirigiéndolos de forma segura a `axiom`.
  5. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 4/4 requerimientos, 9/9 escenarios BDD).

### [INC-15] dashboard-interactive-sdd-orchestration (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-15-dashboard-interactive-sdd-orchestration/`
- **Especificación viva:** `openspec/specs/dashboard-sdd-orchestration/spec.md`
- **Entregables clave:**
  1. Orquestación interactiva del ciclo de vida SDD desde el Dashboard Web local (`axiom ui`):
     - Botón para avanzar fase (`sdd continue`) con refresco en tiempo real.
     - Botón para validar reportes de verificación (`sdd verify-validate`).
     - Botón para componer entregables de archivado formal (`sdd archive-compose`).
     - Modal de creación visual de nuevos incrementos (`proposal.md` canónico en español) con validación sintáctica kebab-case.
  2. Subcomando canónico en CLI: `axiom change create <nombre> [--intent "..."]`.
  3. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 5/5 requerimientos, 8/8 escenarios BDD).
  4. **HITO HISTÓRICO:** Culminación completa de la Fase 2 del Roadmap de Axiom (8 de 8 incrementos concluidos y archivados).

### [INC-16] tui-spanish-localization (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-16-tui-spanish-localization/`
- **Especificación viva:** `openspec/specs/tui-spanish-localization/spec.md`
- **Entregables clave:**
  1. Localización íntegra al castellano peninsular de todas las pantallas, menús y diálogos de la TUI interactiva (`welcome`, `model_config`, `backups`, `detection`, `preset`, `persona`, `common`).
  2. Desacoplamiento de avisos de seguridad externos (`advisoryURL` redirigido a repositorio de Axiom con fail-open limpio y sin advertencias ajenas).
  3. Mantenimiento del comportamiento determinista y responsive en terminales estrechos (modo compacto `Ir` / completo `Iniciar instalación`).
  4. Batería completa de pruebas unitarias en `internal/tui/...` adaptadas y verificadas al 100% PASS.
  5. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 4/4 requerimientos, 9/9 escenarios BDD).

### [INC-17] bidirectional-tui-ui-parity (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-17-bidirectional-tui-ui-parity/`
- **Especificación viva:** `openspec/specs/tui-ui-parity/spec.md`
- **Entregables clave:**
  1. Paridad TUI ➔ Web UI:
     - Nuevo panel y pestaña `⚙️ Ecosistema & Herramientas` en el Dashboard Web (`axiom ui`).
     - Diagnóstico de salud del sistema (`axiom doctor`) con indicadores visuales y recomendaciones.
     - Sincronización de configuraciones (`sync`) y actualización de herramientas (`upgrade`) con consola interactiva en tiempo real.
     - Gestión visual de respaldos en disco (`~/.axiom/backups/`) con listado cronológico, creación y restauración interactiva.
     - Inspección de asignaciones de modelos de IA y niveles de esfuerzo de razonamiento.
  2. Paridad Web UI ➔ TUI:
     - Submenú `📁 Proyectos y Gobernanza SDD ➔` en el menú principal (`welcome.go`) y pantalla dedicada `ScreenGovernance`.
     - Gestión multi-proyecto del Hub global (`ScreenHubProjects`) con conmutación en caliente, vinculación e inicialización (`axiom init`).
     - Tablero interactivo de ciclo de vida SDD (`ScreenSDDIncrements`) con progreso de tareas y avance de fase (`continue`).
     - Monitor multi-rol y evaluación de barrera Fan-In (`ScreenMultiRole`) con compuertas `blocking` vs `deferred`.
     - Visor estructurado de relevos formales (`ScreenHandoffs`) con metadatos y secciones en castellano.
     - Explorador de especificaciones vivas (`ScreenLivingDoc`) con atajo `s` para sincronización en caliente.
  4. Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 10/10 requerimientos, 11/11 escenarios BDD).

---

### [INC-18] rdd-decoupling-and-v3-stability-fixes (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-16-inc-18-rdd-decoupling-and-v3-stability-fixes/`
- **Especificación viva:** `openspec/specs/rdd-decoupling-v3-stability/spec.md`
- **Entregables clave:**
  1. **Desacoplamiento total de RDD (*Review-Driven Development*):**
     - Neutralización de `applyReviewOfferRouting` en `internal/sddstatus/status.go` y stub limpio en `review_door.go`, garantizando que `status.ReviewOffer` sea `nil` y se omita de forma transparente en JSON (`omitempty`).
     - Desacoplamiento de `internal/cli/sdd_status.go` del callback de kill-switch de RDD (`ReviewDisabledForWorkspace`). La compuerta de archivado (`archive: ready`) opera de forma completamente autónoma.
     - Las herramientas de revisión (`axiom review ...`) permanecen 100% operativas como utilidades de auditoría independientes gobernadas por el usuario.
  2. **Absorción de correcciones críticas de upstream v3:**
     - **Fix de rutas en Windows (`1a2f6775`):** Deserialización estructurada de `StatusV2Projection` en `internal/cli/sdd_attempt_test.go` inmune a barras invertidas escapadas.
     - **Aislamiento de CWD (`8c078527`):** Cálculo canónico de rutas de agentes en `internal/cli/run.go` y `sync.go` contra `homeDir` y `componentInjectionDir`, eliminando dependencias de CWD.
     - **Saneamiento del catálogo de skills (`11f6c000`):** Segregación estricta de las 6 skills internas de colaboración (`contributorSkills`) fuera del preset básico de usuarios en `internal/components/skills/presets.go`.
     - **Resiliencia en Engram (`59e6705f`, `90992285`):** Protocolo documentado con pautas explícitas de resolución y desambiguación ante `ambiguous_project` en `internal/assets/engram/protocol.md`.
  3. **Decisión de producto sobre RTK:**
     - Descarte ratificado de la integración de RTK (`rtk-ai/rtk`) debido a sus hooks forzados en herramientas de consola y carencia de soporte para Windows en upstream.
  4. **Verificación formal en arnés OpenSpec aprobada (veredicto PASS: 6/6 requerimientos, 6/6 escenarios BDD).**

### [INC-19] inc-19-odd-workflow-and-promotion (✅ Archivado)
- **Directorio de cambio SDD archivado:** `openspec/changes/archive/2026-09-18-inc-19-odd-workflow-and-promotion/`
- **Especificaciones vivas:** `openspec/specs/odd-living-document/`, `openspec/specs/odd-cli-commands/`, `openspec/specs/odd-sdd-promotion/`, `openspec/specs/odd-ui-integration/` (4 capacidades nuevas, 15 requerimientos, 33 escenarios), mas `openspec/specs/dashboard-sdd-orchestration/spec.md` (REQ-15.1 ampliado con 2 escenarios)
- **Responsabilidad:** ODD & Puerta de Promoción / Experiencia
- **Nota de corrección:** el enunciado original de este incremento (visible en el historial de este documento) afirmaba una poda simultánea del motor SDD (*attempts*, presupuestos de tokens, contratos de admisión de investigación). Esa poda se verificó **falsa contra el código** al arrancar el trabajo — no existe presupuesto de tokens de LLM ni contrato de investigación conectado en `internal/sddstatus` — y el incremento se reencuadró a lo que realmente entrega: el carril ODD y su puerta de promoción. La poda diferida y honestamente reencuadrada como retirada de un contrato ya publicado se registra en **INC-20**.
- **Alcance real entregado (Fases 1-8):**
  1. **Motor del documento vivo** (`internal/odd`, paquete hoja sin dependencias hacia `dashboard`/`cli`/`tui`): estructura canónica de doce secciones en castellano peninsular, identidad estable de *feature* con denylist de nombres reservados de Windows, identificadores estables de tarea (`T1`, `T2`, …), progreso derivado vía `multirole.CountTasks` sobre la sola sección de checklist, y espejo de recuperación de solo lectura en Engram (`odd/<feature>/tasks`) con tres estados (`sincronizado`, `divergente`, `no disponible`) que nunca altera el código de salida.
  2. **CLI `axiom odd create`, `axiom odd status` (`--json`, `--check-mirror`) y `axiom odd promote`** (`--dry-run`, `--name`): promoción en dos fases — crear el cambio SDD primero, marcar el documento ODD después — que nunca deriva capacidades inventadas y nunca produce un cambio SDD duplicado.
  3. **Extensión protegida de `CreateIncrement`** (REQ-15.1 modificado): campo opcional `proposal_body` que la promoción siembra, protegida por un test de caracterización byte a byte que preserva intacto el comportamiento vigente cuando el campo está vacío.
  4. **Web UI y TUI:** pestaña `tab-odd` con listado, filtros, creación, promoción y comprobación de espejo desde el navegador; pantalla `ScreenODDFeatures` alcanzable desde Gobernanza en la TUI; conmutación explícita y visible entre el carril ágil (ODD) y el carril formal (SDD) en ambas superficies.
  5. **Directrices ODD/SDD en castellano peninsular bajo la Persona `axiom`**, distribuidas en `internal/assets/` con renderizado determinista y sin vínculo evento → acción.

### [INC-20] inc-20-upstream-reconciliation (⏳ En progreso)
- **Responsabilidad:** Arquitectura / Integración Upstream / Distribución
- **Sustituye por completo el alcance planificado anterior:** el INC-20 original (`inc-20-sdd-engine-contract-retirement`, retirada del contrato de *attempts*, presupuesto y remediación) quedó obsoleto antes de ejecutarse — upstream ya había ejecutado esa misma poda (commits `18fa04fb`, `15cbbde4`, `ba3ed690`, `62ce74b7`, `e0774e05`) — y se reescribió como lo que el terreno exigía: reconciliar el fork con `Gentleman-Programming/gentle-ai` de forma auditada, por tandas temáticas, sin perder los 19 incrementos de producto propio ya archivados. La preproposal original (topic de Engram `sdd/inc-20-sdd-engine-contract-retirement/preproposal`) queda obsoleta y sustituida.
- **Alcance real entregado:**
  1. **Protocolo de absorción por tandas** (`upstream-absorption-protocol`): deriva obligatoriamente la lista de ficheros de cada tanda desde `git show --stat`, exige verificación sin filtrar (`go build`/`go vet`/`go test ./...` sin `-run`, `e2e/e2e_test.sh`), respeta el inventario de no-reversión V1–V8 y las rutas protegidas, y sostiene el **registro durable de absorción** (`docs/upstream-absorption-ledger.md`, espejado en Engram) con una fila por cada uno de los **91 commits de upstream** sin fusiones entre el ancestro común `266574b0` y el techo congelado `82a6de96` (etiqueta `v3.4.0`, Decisión D4), absorbidos en las tandas temáticas F0 y F2–F7, más la migración final F1.
  2. **Identidad de distribución** (`axiom-distribution-identity`): taxonomía de interoperabilidad vs. identidad (Decisiones D2.1–D2.4) aplicada al instalador y tap, las compuertas de release, el workflow de CI, el shim de `crosslane`, el namespace de protocolo en `contracts/**` (conservado byte a byte, REQ-20.10) y la ruta de módulo Go, cerrando el defecto vivo de que el instalador del fork instalaba upstream y no Axiom.
  3. **Cobertura de CI sobre el binario real** (`axiom-binary-ci-coverage`): el pipeline pasa a ejercitar `cmd/axiom` en vez de validar únicamente el wrapper deprecado `cmd/gentle-ai`.
  4. **Retirada de la gobernanza de *attempts*** (`axiom-sdd-cli-integration`, REQ-13.3 reescrito): `axiom sdd attempt acquire`/`settle` dejan de estar soportados, siguiendo la retirada ya ejecutada upstream (`18fa04fb`); `axiom sdd attempt grant` se conserva sin cambio de comportamiento.
  5. **Sustitución destructiva de ODD** (Decisión D1): retirada completa de la CLI, la Web UI, la TUI y el paquete Go `internal/odd` propios, en favor del protocolo ODD de upstream basado en instrucciones de agente y elevado a protocolo obligatorio por defecto del orquestador (`organic-agent-trigger-rules`).
  6. **Migración de la ruta de módulo Go `/v2` → `/v3`**, última fase de la cadena, absorbiendo además los 2 commits de upstream que ejecutan esa misma migración aguas arriba (`2594581e`, `9bf454d4`).
- **Preproposal (obsoleta, sustituida por completo):** el análisis original persistido en Engram bajo el topic `sdd/inc-20-sdd-engine-contract-retirement/preproposal` ya no describe el alcance real de este incremento; ver la propuesta vigente `inc-20-upstream-reconciliation`.

### [INC-21] inc-21-upfront-flow-governance (⏳ En progreso)
- **Responsabilidad:** Orquestador / Gobernanza SDD
- **Directorio de cambio SDD (activo, sin archivar):** `openspec/changes/inc-21-upfront-flow-governance/`
- **Alcance real entregado:**
  1. **Paso 0 del protocolo ODD** (`internal/components/agentguidance/routing.go`): evaluación de alcance y pregunta bloqueante de selección de carril ODD/SDD antes de crear cualquier artefacto de trabajo; entrada directa al cuestionario de pre-vuelo de SDD para alcance inequívocamente arquitectónico, sin la pregunta binaria (REQ-21.1-21.3).
  2. **Paquete de dominio nuevo `internal/kickoff`** (hoja, sin consumidores fuera de `internal/cli`/`internal/sddstatus`): esquema y sellado de escritura única de `kickoff.yaml` (`Seal`/`Load`, `PublishFileNoReplace`), regla de retro-sellado para cambios preexistentes que congela el rol que la detección de roles vigente ya infiere (nunca `fullstack`, Decisión O-1), ledger solo-anexar de compuertas (`gates.yaml`, `AppendGate`/`LoadGates`) y digest estable de artefactos (`ArtifactDigest`), máquina de estados pura de compuertas con coste cero en modalidad continua, reapertura por digest y `approved` terminal (D-05/D-08), predicado de cierre de último rol (`LastRoleClosed`) y guarda de raíz archivada (`RefuseArchivedRoot`, D-14).
  3. **Verbos CLI nuevos `axiom sdd kickoff seal|show` y `axiom sdd gate record|show`** (`internal/cli/sdd_kickoff.go`, `internal/cli/sdd_gate.go`): sellado explícito o inferido de modalidad de avance, política de relevos y roster de roles antes de `proposal.md` (REQ-21.5); asignación obligatoria del rol reservado `fullstack` sin subdivisión (REQ-21.6, D-06/D-07, `multirole.ResolveRoster`); registro de decisiones de las cuatro compuertas de bloque con motivo obligatorio en rechazo (REQ-21.8-21.12).
  4. **Envoltorio tipado `gentle-ai.sdd-governance.gate/v1` y proyección en `internal/sddstatus`** (`governance.go`, `status.go`, `status_v2.go`): enrutamiento `await-gate` para una compuerta pendiente, retorno a la fase propietaria para un rechazo genuino, y frontera normativa explícita frente a *receipt-driven development* — una aprobación de compuerta SDD nunca es un recibo, nunca se llama `lineage`/`candidate`/`acknowledge`/`burn`, y la única vía de aprobación es `axiom sdd gate record --decision approved` (D-05, D-08, D-09; guardián estructural en `internal/kickoff/rdd_boundary_test.go`).
  5. **Aviso formal de cierre del último rol activo y relevo de integración** (`internal/kickoff/closure.go`, cableado en `sdd_gate.go`): construcción de `handoff.md` (`apply` → `verify`) consolidando los artefactos de todos los roles del roster con la regla de resolución de `to_role` de D-11, y bloqueo del `verify` global hasta que el relevo esté `ready` (REQ-21.13-21.15, D-10/D-12).
  6. **Precondición de integración o despliegue para `archive`** (REQ-21.16): fontanería git local de solo lectura `(SnapshotBuilder).RevisionIsAncestor` sobre `git merge-base --is-ancestor` (sin `fetch` ni `ls-remote`, `internal/reviewtransaction/snapshot.go`), verificación honesta de evidencia `internal/kickoff.VerifyIntegrationEvidence` (`pr_merged` comprobado localmente; `deployment`/`attestation` registrados como declarados, nunca comprobados, D-13/T-11), banderas `--evidence-kind`/`--commit`/`--base-ref`/`--evidence` en `axiom sdd gate record`, y `dependencies.Archive` del resolutor de `sddstatus` condicionada a la compuerta `integration` aprobada solo cuando hay kickoff sellado.
  7. **Doctrina y activos por agente:** sección compartida nueva `SDD Change Kickoff and Block Gates` en `internal/assets/skills/_shared/sdd-orchestrator-sections.md`, referenciada desde los doce activos `sdd-orchestrator*.md`; actualización de `sdd-tasks/SKILL.md` (producción de `tasks.<rol>.md` por rol en roster multi-rol) y de `sdd-archive/SKILL.md` (precondición de integración citada explícitamente); ampliación del árbol y la tabla de rutas de `openspec-convention.md` con `kickoff.yaml`, `gates.yaml` y `handoff.md`.
- **Inmutabilidad post-archive (REQ-21.18):** toda ruta de escritura propia de Axiom (`kickoff seal`, `gate record`) rechaza una raíz de cambio ya archivada, nombrando la vía de bug o nuevo incremento como única gestión posterior; alcance declarado honestamente — Axiom no impide una edición manual bajo `archive/` con un editor de texto, solo que ninguna de sus propias rutas de escritura lo haga.

