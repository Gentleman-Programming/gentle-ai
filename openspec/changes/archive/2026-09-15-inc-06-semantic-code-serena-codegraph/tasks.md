# Tareas: Conector Semántico de Código con Serena MCP y CodeGraph (INC-06)

## Fase 1: Motor de Dominio Semántico (`internal/semantic/`)

- [x] T-01 Crear `internal/semantic/types.go`: Definir estructuras de datos para conectores (`ConnectorType`, `ConnectorSerena`, `ConnectorCodeGraph`, `ConnectorNativeAST`, `ConnectorAuto`), símbolos de código (`SymbolItem`, `SymbolKind`), dependencias entre paquetes (`DependencyRelation`), estado de herramientas de agentes (`AgentToolStatus`) e informe de diagnóstico semántico (`SemanticStatus`).
- [x] T-02 Crear `internal/semantic/detector.go`: Implementar detector de conectores semánticos que inspeccione configuraciones MCP locales de agentes (`Antigravity`, `Claude Code`, `Kiro`, `Cursor`, etc.) y presencia de comandos `codegraph` o `serena` en el `PATH`.
- [x] T-03 Crear `internal/semantic/engine.go`: Implementar motor de análisis semántico nativo en Go utilizando `go/parser`, `go/token` y `go/ast` para recorrer paquetes, extraer structs, interfaces, funciones, métodos y dependencias de importación.
- [x] T-04 Crear `internal/semantic/service.go`: Implementar la capa de servicio que integra el detector y el motor, resolviendo el conector activo según `governance.semantic_analysis` de `axiom.yaml` y aplicando filtros por `query`, `kind` y `role`.
- [x] T-05 Crear `internal/semantic/semantic_test.go`: Suite completa de pruebas unitarias para el detector, el motor AST, extracción de símbolos e interfaces, relaciones de importación y fallback automático a `native-ast`.

## Fase 2: Integración con el Dashboard Web Local (`internal/dashboard/`)

- [x] T-06 Actualizar `internal/dashboard/types.go` y `service.go`: Incorporar DTOs para el subsistema semántico e integrar `semantic.Service` en la capa de agregación del dashboard.
- [x] T-07 Actualizar `internal/dashboard/server.go`: Exponer endpoints REST `/api/semantic/status` (GET), `/api/semantic/symbols` (GET con filtros `query`, `kind`, `role`) y `/api/semantic/dependencies` (GET).
- [x] T-08 Actualizar `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`): Crear la pestaña "Semántica & Grafo" en el menú de navegación, con tarjeta de salud del conector, buscador reactivo de símbolos, tabla interactiva y visualizador de dependencias entre paquetes.
- [x] T-09 Actualizar `internal/dashboard/dashboard_test.go`: Incorporar pruebas unitarias HTTP para los nuevos endpoints semánticos.

## Fase 3: Integración CLI (`cmd/axiom/main.go`)

- [x] T-10 Implementar el grupo de subcomandos `axiom semantic`: `axiom semantic status`, `axiom semantic symbols` (con banderas `--query`, `--kind`, `--role`, `--path`) y `axiom semantic inspect` (con banderas `--role`, `--path`) en `cmd/axiom/main.go`.

## Fase 4: Verificación, Cierre y Archivado SDD

- [x] T-11 Ejecutar suite completa `go test -v ./internal/workspace/... ./internal/handoff/... ./internal/multirole/... ./internal/dashboard/... ./internal/autoskill/... ./internal/semantic/...` y compilar el binario `axiom.exe`.
- [x] T-12 Verificación en vivo de subcomandos `axiom semantic` y validación de respuestas REST del servidor local.
- [x] T-13 Generar artefactos de trazabilidad: `apply-progress.md`, `verify-report.md` y `archive-report.md`.
- [x] T-14 Promover especificación viva a `openspec/specs/semantic-code/spec.md`, formalizar el archivado en `openspec/changes/archive/2026-09-15-inc-06-semantic-code-serena-codegraph/` y actualizar `docs/ROADMAP.md`.
