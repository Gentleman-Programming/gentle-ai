# Roadmap de Incrementos — Axiom

> **Visión y Arquitectura:** [AXIOM_ENTERPRISE_VISION.md](architecture/AXIOM_ENTERPRISE_VISION.md)  
> **Estado General:**  
> - **Fase 1: Fundación de la Plataforma e Identidad Base:** ✅ 100% Completada (7/7 Incrementos Archivados)  
> - **Fase 2: Autonomía, Identidad, Multi-Proyecto y Orquestación SDD:** ✅ 100% Completada (8/8 Incrementos Archivados)  
> **Total de Incrementos Archivados:** 15 de 15 (INC-01 a INC-15)  
> **Última Actualización:** 2026-09-16

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






