# Progreso de Implementación: Conector Semántico de Código con Serena MCP y CodeGraph (INC-06)

## Estado de Fases y Tareas

### Fase 1: Motor de Dominio Semántico (`internal/semantic/`)
- [x] **T-01** Crear `internal/semantic/types.go`: Modelos para conectores (`ConnectorType`), símbolos (`SymbolItem`, `SymbolKind`), dependencias (`DependencyRelation`), estado de agentes (`AgentToolStatus`) y diagnóstico (`SemanticStatus`).
- [x] **T-02** Crear `internal/semantic/detector.go`: Detector de entornos semánticos para Serena MCP y CodeGraph en configuraciones de agentes y PATH.
- [x] **T-03** Crear `internal/semantic/engine.go`: Motor de análisis semántico nativo Go AST (`go/parser`, `go/ast`) para extracción de structs, interfaces, funciones, métodos y dependencias.
- [x] **T-04** Crear `internal/semantic/service.go`: Capa de servicio con resolución de conector, filtrado por query/kind/role y deduplicación de rutas.
- [x] **T-05** Crear `internal/semantic/semantic_test.go`: 3 pruebas unitarias exhaustivas con 100% PASS.

### Fase 2: Integración con el Dashboard Web Local (`internal/dashboard/`)
- [x] **T-06** Actualizar `internal/dashboard/service.go`: Integración con `semantic.Service`.
- [x] **T-07** Actualizar `internal/dashboard/server.go`: Endpoints REST `/api/semantic/status`, `/api/semantic/symbols` y `/api/semantic/dependencies`.
- [x] **T-08** Actualizar `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`): Nueva pestaña "Semántica & Grafo", tarjetas de estado, buscador en tiempo real y visor de dependencias.
- [x] **T-09** Actualizar `internal/dashboard/dashboard_test.go`: Incorporación de `TestSemanticEndpoints` (10/10 tests PASS).

### Fase 3: Integración CLI (`cmd/axiom/main.go`)
- [x] **T-10** Implementar subcomandos `axiom semantic status`, `axiom semantic symbols` y `axiom semantic inspect`.

### Fase 4: Verificación y Archivado
- [x] **T-11** Batería de pruebas en Go (38/38 tests PASS) y compilación limpia de `axiom.exe`.
- [x] **T-12** Validación en vivo de comandos CLI `axiom semantic status|symbols|inspect`.
- [ ] **T-13** Generación de informes de verificación y archivo.
- [ ] **T-14** Promoción a especificación viva y archivado del incremento.
