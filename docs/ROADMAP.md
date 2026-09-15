# Roadmap de Incrementos — Axiom

> **Visión y Arquitectura:** [AXIOM_ENTERPRISE_VISION.md](architecture/AXIOM_ENTERPRISE_VISION.md)  
> **Estado General:** Fase 1 — Fundación de la Plataforma e Identidad (100% Completada — 7/7 Incrementos Archivados)  
> **Última Actualización:** 2026-09-15

---

## Leyenda de Estados

- `⏳ En progreso`: Incremento actualmente en desarrollo activo dentro de su ciclo SDD.
- `📋 Planificado`: Incremento definido con alcance y listo para iniciar cuando corresponda.
- `✅ Archivado`: Incremento completado, verificado al 100% y formalmente archivado en la especificación viva.
- `⏸️ Pausado / En espera`: Incremento a la espera de dependencias previas o decisiones de producto.

---

## Catálogo de Incrementos

| ID | Incremento | Estado | Responsabilidad | Descripción Resumida |
| :--- | :--- | :---: | :---: | :--- |
| **INC-01** | `axiom-identity-workspace-topology` | ✅ Archivado | Arquitectura / Core | Creación del binario `axiom`, esquema `axiom.yaml`, validación de topologías (monorepo-embedded, monorepo-decoupled, multirepo) y repositorio canónico de specs. |
| **INC-02** | `structured-handoffs-lifecycle` | ✅ Archivado | Core SDD / Workflow | Esquema canónico `handoff.md`, motor Go `internal/handoff`, subcomandos CLI `axiom handoff show/create/validate` y espejo Engram. |
| **INC-03** | `multi-role-sdd-fan-out` | ✅ Archivado | Core SDD / Roles | Concurrencia de roles en `Design`, `tasks.<rol>.md`, políticas de compuerta (`blocking`/`deferred`), barrera de sincronización y migración diferida de tareas acumulativas. |
| **INC-04** | `axiom-local-web-dashboard` | ✅ Archivado | UI / Experiencia | Servidor HTTP local embebido en Go con dashboard web: tablero de incrementos, estado de roles, visor de handoffs y buzón de skills. |
| **INC-05** | `autoskills-catalog-and-mining` | ✅ Archivado | Skills / Inteligencia | Catálogo de skills por tecnología detectada (midudev/autoskills) y minería heurística de código local con gobernanza Human-in-the-Loop. |
| **INC-06** | `semantic-code-serena-codegraph` | ✅ Archivado | Semántica / Herramientas | Conector local con Serena MCP y CodeGraph para navegación y consultas semánticas de código en `Explore` y `Design`. |
| **INC-07** | `archive-living-documentation-engine` | ✅ Archivado | Documentación / SDD | `Archive` como mantenedor continuo de especificaciones existentes y generador incremental de documentación viva en proyectos no documentados. |

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





