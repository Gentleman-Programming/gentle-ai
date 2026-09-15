# Especificación de Requerimientos: Conector Semántico de Código (Serena MCP & CodeGraph) (INC-06)

## Propósito

Definir de forma ejecutable, determinista y verificable los requerimientos funcionales, no funcionales y escenarios BDD para la integración semántica de código en Axiom: diagnóstico de conectores externos (Serena MCP y CodeGraph), motor semántico nativo en Go con análisis de AST (`go/parser`, `go/ast`), subcomandos CLI `axiom semantic` y la extensión del Dashboard Web local (`axiom ui`) con la pestaña de exploración semántica.

---

## 1. Capacidad: `semantic-connector-detection`

El paquete `internal/semantic` provee capacidades de detección, inspección y diagnóstico del entorno semántico local, analizando las herramientas disponibles en el sistema y en las configuraciones de los agentes de IA compatibles.

### Requirement: Detección y Diagnóstico de Serena MCP (REQ-1.1)

El detector DEBE inspeccionar las configuraciones de MCP de los agentes en el sistema del usuario (tales como `~/.gemini/antigravity/mcp_config.json`, `~/.claude.json` o `~/.kiro/settings/mcp.json`) para determinar si el servidor MCP de Serena (`serena`, `serena-mcp`) se encuentra configurado y disponible.

#### Scenario: Serena MCP detectado en la configuración de un agente soportado
- **DADO** un archivo de configuración MCP que registra la clave `serena` o `serena-mcp`
- **CUANDO** el detector semántico ejecuta el diagnóstico de agentes
- **ENTONCES** reporta a Serena como conector configurado
- **Y** documenta el archivo de configuración de origen y el estado del agente

#### Scenario: Serena MCP no configurado
- **DADO** un entorno donde ningún agente tiene configurado el servidor Serena MCP
- **CUANDO** el detector ejecuta la comprobación de Serena
- **ENTONCES** reporta a Serena como no detectado (`missing`) sin producir errores fatales

---

### Requirement: Detección de CodeGraph CLI y Configuración (REQ-1.2)

El detector DEBE verificar la disponibilidad de CodeGraph comprobando la presencia del binario o comando `codegraph` en el `PATH` del sistema y en las configuraciones de herramientas de agentes.

#### Scenario: CodeGraph disponible en el PATH del sistema
- **DADO** un entorno local con el ejecutable `codegraph` instalado y accesible en PATH
- **CUANDO** el detector semántico evalúa la disponibilidad de CodeGraph
- **ENTONCES** reporta CodeGraph como conector disponible y registra la ruta del binario

#### Scenario: CodeGraph no instalado en el sistema
- **DADO** un entorno sin el binario `codegraph` en PATH
- **CUANDO** el detector semántico verifica CodeGraph
- **ENTONCES** reporta CodeGraph como ausente (`missing`) y propone el motor nativo Go AST como alternativa

---

### Requirement: Resolución del Conector Semántico Activo (REQ-1.3)

El servicio semántico DEBE resolver el conector activo basándose en la configuración `governance.semantic_analysis` de `axiom.yaml` (`serena`, `codegraph` o `auto`):
1. Si se configura `serena` y está disponible, se selecciona Serena.
2. Si se configura `codegraph` y está disponible, se selecciona CodeGraph.
3. Si se configura `auto` (o el conector explícito no está disponible), el sistema activa de forma transparente y determinista el **motor nativo Go AST** (`native-ast`).

#### Scenario: Resolución automática con fallback a AST nativo
- **DADO** un archivo `axiom.yaml` con `semantic_analysis: "auto"` (o no especificado) y sin servidores MCP externos configurados
- **CUANDO** se inicializa el servicio semántico
- **ENTONCES** se selecciona el conector `native-ast`
- **Y** se asegura la operatividad semántica del 100% de consultas sobre repositorios Go

#### Scenario: Conector explícito no disponible advierte y activa fallback
- **DADO** `axiom.yaml` con `semantic_analysis: "serena"` pero sin Serena MCP instalado
- **CUANDO** se consulta el estado semántico
- **ENTONCES** el estado reporta una advertencia indicando que Serena no está disponible
- **Y** conmuta al motor nativo `native-ast` para evitar el bloqueo del agente

---

## 2. Capacidad: `semantic-native-ast-engine`

El motor semántico en Go proporciona análisis de código estático y extracción determinista de grafos de conocimiento de código utilizando los paquetes estándar de Go (`go/parser`, `go/token`, `go/ast`).

### Requirement: Extracción de Símbolos de Código en Go (REQ-2.1)

El motor DEBE recorrer los archivos `.go` en los directorios indicados del workspace y extraer los siguientes tipos de símbolos (`SymbolItem`):
- `struct`: Declaraciones de estructuras con sus campos.
- `interface`: Declaraciones de interfaces con la lista de métodos requeridos.
- `func`: Funciones exportadas e internas con su signatura de parámetros y retornos.
- `method`: Métodos asociados a tipos receptores (*receivers*).
- `type`: Definiciones y alias de tipos.

Cada símbolo DEBE incluir: `Name`, `Kind`, `Package`, `FilePath`, `LineNumber`, `Signature` y `DocComment` si existe.

#### Scenario: Extracción de interfaces y structs con métodos
- **DADO** un paquete Go que define la interfaz `Detector` y el struct `Manager` con métodos asociados
- **CUANDO** el motor AST analiza el directorio del paquete
- **ENTONCES** se identifican correctamente ambos símbolos
- **Y** para el método se registra el tipo receptor y la signatura completa
- **Y** se preserva el número de línea exacto dentro del archivo

---

### Requirement: Análisis del Grafo de Dependencias de Paquetes (REQ-2.2)

El motor DEBE inspeccionar las cláusulas `import` de cada archivo analizado y construir la lista de dependencias directas (`DependencyRelation`), identificando el paquete de origen, el paquete importado y si se trata de un paquete interno del workspace o de la librería estándar / externa.

#### Scenario: Mapeo de dependencias internas de un rol
- **DADO** un paquete en `internal/dashboard` que importa `internal/autoskill` y `internal/workspace`
- **CUANDO** el motor semántico analiza las dependencias
- **ENTONCES** genera aristas de dependencia vinculando `internal/dashboard` con `internal/autoskill` e `internal/workspace`
- **Y** clasifica estas dependencias como internas al proyecto

---

### Requirement: Búsqueda y Filtrado de Símbolos (REQ-2.3)

El servicio DEBE permitir filtrar los símbolos extraídos mediante parámetros opcionales de búsqueda:
- `Query`: Coincidencia que contenga la cadena en el nombre del símbolo (case-insensitive).
- `Kind`: Filtrado por tipo de símbolo (`struct`, `interface`, `func`, `method`).
- `Role`: Restricción al ámbito de los repositorios asociados a un rol específico de `axiom.yaml`.

#### Scenario: Búsqueda de interfaces en el workspace
- **DADO** un workspace con múltiples paquetes y tipos
- **CUANDO** se consultan los símbolos con filtro `kind="interface"` y `query="detector"`
- **ENTONCES** retorna únicamente los símbolos de tipo `interface` cuyo nombre contiene "detector"
- **Y** la lista resultante está ordenada alfabéticamente por nombre

---

## 3. Capacidad: `axiom-cli-semantic`

La interfaz de línea de comandos de Axiom en `cmd/axiom/main.go` expone el grupo de comandos `axiom semantic`.

### Requirement: Comando `axiom semantic status` (REQ-3.1)

El comando `axiom semantic status [--path <dir>]` DEBE mostrar:
1. El conector semántico configurado en `axiom.yaml`.
2. El conector activo en ejecución (`serena`, `codegraph` o `native-ast`).
3. La disponibilidad de Serena MCP y CodeGraph en los agentes compatibles del sistema.
4. El total de paquetes Go y símbolos indexados en el workspace.

#### Scenario: Ejecución de `axiom semantic status`
- **DADO** un workspace válido de Axiom
- **CUANDO** el usuario ejecuta `axiom semantic status`
- **ENTONCES** la CLI imprime un informe detallado con código de salida 0
- **Y** muestra el estado del conector activo y los agentes verificados

---

### Requirement: Comando `axiom semantic symbols` (REQ-3.2)

El comando `axiom semantic symbols [--query <texto>] [--kind <tipo>] [--role <rol>] [--path <dir>]` DEBE listar los símbolos encontrados en formato tabular o de lista estructurada.

#### Scenario: Consulta de símbolos con filtro de texto
- **DADO** un workspace con paquetes Go
- **CUANDO** el usuario ejecuta `axiom semantic symbols --query "Service"`
- **ENTONCES** la CLI imprime los símbolos que coinciden con "Service" con su tipo, paquete, archivo y línea

---

### Requirement: Comando `axiom semantic inspect` (REQ-3.3)

El comando `axiom semantic inspect [--role <rol>] [--path <dir>]` DEBE presentar el mapa de relaciones y dependencias entre paquetes del workspace o de un rol determinado.

#### Scenario: Inspección de dependencias de un rol
- **DADO** un rol configurado en `axiom.yaml`
- **CUANDO** el usuario ejecuta `axiom semantic inspect --role backend`
- **ENTONCES** la CLI lista las dependencias internas entre los paquetes de dicho rol

---

## 4. Capacidad: `axiom-dashboard-semantic-explorer`

El Dashboard Web local (`internal/dashboard/`) expone los datos semánticos a través de su API REST JSON y los renderiza en una nueva vista de la interfaz SPA.

### Requirement: Endpoints REST Semánticos en el Servidor Web (REQ-4.1)

El servidor web DEBE exponer las siguientes rutas REST:
- `GET /api/semantic/status`: Retorna el objeto `SemanticStatus` serializado en JSON (conector activo, conectores disponibles, estadísticas).
- `GET /api/semantic/symbols`: Acepta query params `query`, `kind` y `role`, retornando el listado JSON de `[]SymbolItem`.
- `GET /api/semantic/dependencies`: Retorna la lista JSON de relaciones de dependencia `[]DependencyRelation`.

#### Scenario: Petición HTTP a `/api/semantic/status`
- **DADO** el servidor del Dashboard Web en ejecución
- **CUANDO** se efectúa una petición `GET` a `/api/semantic/status`
- **ENTONCES** responde con código HTTP 200 y cabecera `Content-Type: application/json`
- **Y** el cuerpo contiene el conector activo y las métricas del motor semántico

---

### Requirement: Interfaz Web SPA con Panel Semántico y Buscador (REQ-4.2)

La interfaz SPA embebida (`assets/index.html`, `app.js`, `style.css`) DEBE incluir una nueva sección de navegación "Semántica & Grafo" que permita:
1. Visualizar la tarjeta de salud del conector semántico con badges distintivos (`Serena MCP`, `CodeGraph`, `AST Nativo Go`).
2. Disponer de un buscador en tiempo real para filtrar símbolos por texto y tipo.
3. Explorar la tabla de símbolos con enlaces relativos al filesystem y signaturas de funciones/métodos.
4. Visualizar las dependencias internas de los módulos.

#### Scenario: Exploración de símbolos desde el navegador web
- **DADO** el usuario con el Dashboard abierto en la pestaña "Semántica & Grafo"
- **CUANDO** escribe un término en el campo de búsqueda de símbolos
- **ENTONCES** la tabla de resultados se filtra dinámicamente mostrando los símbolos coincidentes
- **Y** se indican su tipo, paquete y línea sin requerir recargar la página
