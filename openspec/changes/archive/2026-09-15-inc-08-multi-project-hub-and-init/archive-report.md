# Reporte de Archivado: Hub Multi-Proyecto, Selector Dinámico en Dashboard Web y CLI axiom init (INC-08)

**Fecha:** 2026-09-15  
**Incremento:** `inc-08-multi-project-hub-and-init`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El Incremento 8 (INC-08) transforma a Axiom en una plataforma de ingeniería de software **Multi-Proyecto**:
1. **Desacoplamiento Arquitectónico:** Binario global instalado en el sistema (`axiom.exe` en `$PATH`), mientras la gobernanza, especificaciones y roles residen estrictamente en cada proyecto (`axiom.yaml`, `openspec/`).
2. **Hub Centralizado de Workspaces:** Registro atómico en `~/.axiom/workspaces.json` administrando el catálogo de proyectos locales del desarrollador.
3. **CLI `axiom init` & `axiom project`:** Inicialización asistida con auto-detección heurística de lenguajes y frameworks, y comandos para listar, conmutar, añadir y remover proyectos.
4. **Dashboard Web Multi-Proyecto:** El servidor HTTP local y la interfaz web ahora soportan conmutación en caliente de repositorios, registro dinámico y pantalla de bienvenida con inicialización en 1 clic para proyectos no configurados.

---

## Artefactos Consolidados y Promovidos

- **Especificación Viva:** Promovida a `openspec/specs/multi-project-hub/spec.md`.
- **Código de Producción:**
  - `internal/hub/types.go`
  - `internal/hub/detector.go`
  - `internal/hub/manager.go`
  - `internal/hub/init.go`
  - `internal/hub/hub_test.go`
  - `internal/dashboard/service.go`
  - `internal/dashboard/server.go`
  - `internal/dashboard/types.go`
  - `internal/dashboard/dashboard_test.go`
  - `internal/dashboard/assets/index.html`
  - `internal/dashboard/assets/style.css`
  - `internal/dashboard/assets/app.js`
  - `cmd/axiom/main.go`
- **Binario Oficial:** Recompilado e instalado en `$GOPATH/bin/axiom.exe`.
