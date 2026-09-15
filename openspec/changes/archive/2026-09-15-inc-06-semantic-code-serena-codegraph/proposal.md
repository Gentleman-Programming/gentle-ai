# Propuesta: Conector Semántico de Código con Serena MCP y CodeGraph (INC-06)

## Propósito (Intent)

En el ciclo de desarrollo asistido por IA de Axiom —especialmente durante las fases de **Exploración (`Explore`)**, **Diseño de Arquitectura (`Design`)** y **Verificación de Impacto**— los agentes de codificación requieren razonar sobre la estructura profunda del software: jerarquías de tipos, contratos de interfaces, grafo de llamadas (*call hierarchy*), referencias cruzadas y relaciones de dependencias entre componentes y paquetes en topologías monorrepo o multirrepo.

Actualmente, los flujos estándar se ven forzados a recurrir predominantemente a búsquedas de texto plano (`grep`, `find_by_name`), lo que provoca:
1. **Ruido cognitivo y alucinaciones:** Coincidencias accidentales de nombres de variables o comentarios sin relación tipada.
2. **Ceguera arquitectónica en repositorios múltiples:** Incapacidad de rastrear qué roles o módulos consumen una interfaz o struct determinado a través de la frontera de paquetes.
3. **Alto coste de contexto:** Lectura innecesaria de archivos completos sólo para descubrir la signatura de una función o sus consumidores directos.

Para solucionar esto de raíz y cumplir el pilar 2.5 de la Visión Maestra de Axiom, **el Incremento 6 (INC-06: `semantic-code-serena-codegraph`)** dota a la plataforma de:
- Un motor de integración y diagnóstico para **Serena MCP** (servidor semántico basado en Tree-sitter y LSP) y **CodeGraph** (herramienta de grafos de conocimiento de código).
- Un analizador semántico en Go (`internal/semantic/`) con **motor fallback nativo AST** (`go/parser`, `go/token`, `go/ast`) que garantiza comprensión semántica de primer nivel (símbolos, interfaces, funciones exportadas, grafo de importaciones) incluso en entornos donde el servidor MCP externo no esté instalado.
- Integración nativa con `axiom.yaml` (`governance.semantic_analysis: "serena" | "codegraph" | "auto"`).
- Comandos CLI dedicados bajo el grupo `axiom semantic` (`status`, `symbols`, `inspect`).
- Endpoints REST y panel interactivo de navegación semántica en el **Dashboard Web local** (`axiom ui`).

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Paquete de dominio semántico en Go (`internal/semantic/`):**
   - `types.go`: Modelos para representar el conector semántico (`ConnectorType`: `serena`, `codegraph`, `ast`), símbolos del código (`SymbolItem`: structs, interfaces, funciones, métodos, constantes), dependencias internas (`DependencyRelation`), diagnósticos de conector (`SemanticStatus`) y consultas semánticas (`SemanticQuery`).
   - `detector.go`: Detector de disponibilidad del entorno semántico. Inspecciona configuraciones MCP de agentes soportados (`.gemini/antigravity/mcp_config.json`, `.claude.json`, `.kiro/`, etc.) y presencia de binarios o comandos locales para Serena y CodeGraph.
   - `engine.go`: Motor de resolución semántica. Orquesta la fuente configurada o detectada:
     - Conector Serena / CodeGraph para consultas semánticas vía protocolo MCP o CLI.
     - Motor nativo Go AST para análisis estático determinista sin dependencias externas: extrae catálogo de tipos, signaturas de interfaces, implementaciones y grafo de imports del workspace.
   - `service.go`: Fachada de servicios semánticos que expone diagnósticos de salud, catálogo de símbolos indexados y mapa de dependencias por rol.

2. **Integración con Configuración de Workspace (`internal/workspace/`):**
   - Vinculación con `governance.semantic_analysis` en `axiom.yaml` (valores permitidos: `serena`, `codegraph`, `auto`).
   - Diagnóstico en `axiom workspace validate` que advierta si el motor configurado no está disponible localmente, sugiriendo el modo `auto` (fallback AST).

3. **Comandos CLI en `cmd/axiom/main.go` (`axiom semantic`):**
   - `axiom semantic status [--path <dir>]`: Reporta el estado de salud, conector activo, agentes compatibles detectados y cobertura de indexación.
   - `axiom semantic symbols [--query <nombre>] [--kind <tipo>] [--role <rol>] [--path <dir>]`: Consulta interactiva y filtrado de símbolos (estructuras, interfaces, funciones).
   - `axiom semantic inspect [--role <rol>] [--path <dir>]`: Muestra el grafo de dependencias de paquetes y relaciones arquitectónicas.

4. **Dashboard Web Local (`internal/dashboard/`):**
   - Nuevos endpoints REST en `internal/dashboard/server.go`:
     - `GET /api/semantic/status`: Diagnóstico y conector en uso.
     - `GET /api/semantic/symbols?query=...&role=...`: Catálogo de símbolos estructurados.
     - `GET /api/semantic/dependencies`: Grafo de dependencias de paquetes.
   - Frontend SPA (`assets/`):
     - Nueva vista / pestaña "Semántica & Grafo":
       - Tarjeta de estado del conector semántico (con insignia de motor activo: Serena MCP, CodeGraph o Fallback AST nativo).
       - Buscador reactivo de símbolos, funciones e interfaces con su ubicación en el filesystem (`file:///...`).
       - Explorador visual de relaciones entre paquetes del workspace.

5. **Suite de Pruebas Unitarias (`internal/semantic/semantic_test.go`):**
   - Pruebas del detector de conectores (simulación de configuraciones MCP y comandos en PATH).
   - Pruebas del motor AST nativo con código Go de prueba (extracción de structs, métodos, interfaces y grafo de imports).
   - Pruebas de integración del servicio semántico y manejo de errores o workspaces vacíos.
   - Pruebas de los endpoints REST en `internal/dashboard/`.

### Fuera de Alcance (Out of Scope)

- Modificación o reimplementación de los binarios externos upstream de Serena o CodeGraph (Axiom actúa como conector, orquestador y validador de gobernanza).
- Generación automática de especificaciones vivas completas para proyectos sin documentar (Cold Start, reservado para **INC-07**).

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `semantic-connector-detection`: Capacidad de diagnosticar e interactuar con Serena MCP y CodeGraph en las configuraciones de agentes del usuario.
- `semantic-native-ast-engine`: Motor nativo Go sin dependencias para indexación y consulta rápida de símbolos, interfaces y relaciones estructurales de código.
- `axiom-cli-semantic`: Interfaz de línea de comandos `axiom semantic status|symbols|inspect`.
- `axiom-dashboard-semantic-explorer`: Panel interactivo en la Web UI local para exploración semántica del código sin salir del navegador.

---

## Enfoque de Implementación (Approach)

1. **Definir el modelo semántico (`internal/semantic/types.go`):**
   - Tipos de símbolos (`Struct`, `Interface`, `Function`, `Method`), estados de salud (`SemanticStatus`), relaciones de dependencia (`DependencyEdge`).
2. **Implementar el detector de conectores (`detector.go`):**
   - Inspección de `axiom.yaml`, detección de archivos MCP de agentes y validación de herramientas locales.
3. **Implementar el motor AST nativo en Go (`engine.go`):**
   - Uso de paquetes estándar `go/parser`, `go/token` y `go/ast` para parsear código en el workspace y generar el árbol semántico determinista.
4. **Implementar la capa de servicio (`service.go`):**
   - Orquestar diagnósticos, búsquedas filtradas y construcción de grafo de dependencias.
5. **Crear batería de pruebas unitarias (`semantic_test.go`):**
   - Pruebas con fixtures controladas asegurando 100% PASS.
6. **Registrar subcomandos en `cmd/axiom/main.go`:**
   - Comandos `axiom semantic status`, `axiom semantic symbols`, `axiom semantic inspect`.
7. **Exponer endpoints y panel en Dashboard Web (`internal/dashboard/`):**
   - Endpoints `/api/semantic/*` y actualización del frontend SPA (`index.html`, `style.css`, `app.js`).
8. **Verificación formal en OpenSpec y arnés SDD:**
   - Crear especificación viva en `openspec/specs/semantic-code/spec.md`, validar con `gentle-ai sdd-verify-validate` y consolidar el incremento.
