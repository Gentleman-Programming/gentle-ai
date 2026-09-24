# ODD: Mejoras de UX, Gobernanza de Roles, Diagnóstico Semántico y Seguridad de Sincronización en Axiom UI

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/ui-dashboard-ux-governance-and-sync-fixes.md`.  
> Espejo de recuperación en Engram: topic `odd/ui-dashboard-ux-governance-and-sync-fixes/tasks`, proyecto `axiom`.

## Objetivo

Implementar de forma completa y coherente el conjunto de correcciones arquitectónicas, de usabilidad (UX) y de gobernanza identificadas en `axiom ui`:
1. **Gobernanza de Roles de Axiom:** Configurar formalmente `axiom.yaml` con el rol canónico `fullstack`, purgando los roles redundantes (`core`, `qa`, `e2e`).
2. **Tablero de Incrementos:** Corregir el filtro «Todos» para acotarlo a incrementos activos, incorporar clasificación visual e insignias para «Bugs / Fixes» y botón de filtro dedicado.
3. **Visor de Handoffs:** Restringir el selector exclusivamente a incrementos activos, evitando ruido de cambios archivados e inmutables.
4. **Buzón y Catálogo de Skills:**
   - Corregir el bug visual que mostraba la barra de acciones de incrementos («Continuar SDD», «Validar Verificación», «Redactar Handoff») sobre el modal de vista previa de `SKILL.md`.
   - Extender el backend de la UI (`GetSkills()`) para descubrir dinámicamente skills en arquitecturas multirrepo a través de `skillregistry`.
5. **Reubicación de Topología y Salud:** Mover la sección de topología y salud de repositorio a un módulo integrado dentro de «Ecosistema & Herramientas», desaturando la barra de navegación principal.
6. **Optimización de Monitor Multi-Rol:** Adaptar la pestaña para que en proyectos mono-rol (`fullstack`) refleje limpiamente el modo unificado sin generar confusión con barreras fan-in vacías.
7. **Semántica y Grafo (Detección a Nivel de Proyecto y Diagnóstico Inmediato):**
   - Extender el detector semántico para auditar configuraciones de agentes a nivel de proyecto (`.mcp.json`, `mcp_config.json`, `.claude/mcp/`, etc.) además del directorio de usuario.
   - Exponer e ilustrar en tarjetas visuales si `codegraph` y `serena` están instalados en PATH y configurados en el workspace activo de un vistazo.
8. **Seguridad y Coherencia en Sincronización de Workspace:**
   - Garantizar que el servidor web (`runUI` y `RunSync`) sincronice siempre el directorio de trabajo del proceso (`os.Chdir`) con la raíz del proyecto activo, evitando desincronizaciones entre el proyecto visualizado y el directorio de ejecución de `sync`.

---

## Tareas

- [x] **T1 · Gobernanza de Roles (`axiom.yaml`)**
  - Reemplazar roles `core`, `qa`, `e2e` por el rol único canónico `fullstack`.
- [x] **T2 · Seguridad de Directorio y Sincronización (`cmd/axiom/main.go` & `internal/dashboard/`)**
  - Forzar `os.Chdir(baseDir)` al iniciar `runUI`.
  - En `service.go` (`RunSync`), robustecer el cambio de directorio y validación contra `s.getRootPath()`.
- [x] **T3 · Descubrimiento Multirrepo de Skills en Dashboard (`internal/dashboard/service.go`)**
  - Conectar `GetSkills()` con `skillregistry.DiscoverSkills(root)` y las rutas declaradas en `axiom.yaml`.
- [x] **T4 · Diagnóstico Semántico a Nivel de Proyecto (`internal/semantic/` & `internal/dashboard/`)**
  - Parametrizar `Detector.DetectAgents(projectRoot)` y agregar verificación de presencia/instalación de `codegraph` y `serena`.
  - Exponer indicadores estructurados en `SemanticStatusDTO`.
- [x] **T5 · Interfaz Web: Tableros, Modales, Topología y Handoffs (`assets/index.html`, `app.js`, `style.css`)**
  - Ajustar filtro "all" a activos, añadir filtro e insignia de "Bugs / Fixes".
  - Filtrar visor de handoffs solo para incrementos activos.
  - Ocultar toolbar SDD en modal de previsualización de skills.
  - Mover "Topología & Salud" a "Ecosistema & Herramientas".
  - Adaptar "Monitor Multi-Rol" en mono-rol y renderizar tarjetas de CodeGraph/Serena.
- [x] **T6 · Pruebas Automatizadas y Validación**
  - Actualizar y ejecutar tests unitarios de dashboard, semántica y CLI.
- [x] **T7 · Publicación, Instalación y Sincronización**
  - Commit a `main`, push a remoto, `go install ./cmd/axiom` y `axiom sync`.

---

## Verificación Ejecutable
- `go test ./internal/dashboard/... -v` -> PASS
- `go test ./internal/semantic/... -v` -> PASS
- `go test ./cmd/axiom/... -v` -> PASS
