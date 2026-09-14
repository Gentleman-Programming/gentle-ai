# Propuesta: Identidad del Binario Axiom y Validación de Topología de Workspace (INC-01)

## Propósito (Intent)

Axiom nace como la evolución paralela de Gentle-AI para dar soporte a equipos, múltiples roles y arquitecturas multirrepositorio. El primer paso fundamental es dotar a Axiom de su propia identidad ejecutable mediante el binario `axiom` (`cmd/axiom/main.go`) y establecer el modelo formal de espacio de trabajo (`workspace`) a través del fichero de configuración canónico `axiom.yaml`.

Este incremento implementa la validación determinista de las tres topologías fundamentales de proyecto:
1. **Monorepo Embebido (`monorepo-embedded`):** Código y especificaciones en un único repositorio Git.
2. **Monorepo Desacoplado (`monorepo-decoupled`):** Código en monorrepo y especificaciones en repositorio independiente bajo la misma carpeta maestra.
3. **Multirepo Federado (`multirepo`):** Múltiples repositorios de código vinculados a roles, exigiendo obligatoriamente un repositorio canónico de especificaciones en la carpeta maestra.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)
- **Punto de entrada ejecutable:** Creación de `cmd/axiom/main.go` que permita compilar el binario nativo `axiom.exe`.
- **Comandos base de la CLI de Axiom:**
  - `axiom --version` / `axiom version`: Impresión de versión y metadata del release de Axiom.
  - `axiom workspace validate [--path <dir>]`: Validador formal del espacio de trabajo y de las reglas de topología.
- **Paquete de dominio `internal/workspace/`:**
  - `types.go`: Definición de structs Go para `axiom.yaml` (`WorkspaceConfig`, `TopologyType`, `RoleConfig`, `GovernanceConfig`).
  - `loader.go`: Carga, parseo y validación sintáctica de ficheros `axiom.yaml`.
  - `validator.go`: Motor de validación semántica de la topología:
    - Comprobación de que la carpeta maestra existe y es accesible.
    - Validación de que todos los repositorios declarados en los roles existen localmente dentro de la carpeta maestra.
    - Cumplimiento de la regla estricta: en topologías `multirepo` y `monorepo-decoupled`, el repositorio de especificaciones (`specs_repository`) DEBE existir dentro de la carpeta maestra común.
- **Pruebas unitarias automáticas:** Suite completa de tests en Go (`internal/workspace/...`) cubriendo casos válidos e inválidos para cada topología.
- **Artefacto de ejemplo:** Generación de una plantilla de referencia `axiom.example.yaml`.

### Fuera de Alcance (Out of Scope)
- Servidor Web Dashboard local (`axiom ui`) — planificado para el INC-04.
- Máquina de estados de handoffs estructurados — planificada para el INC-02.
- Desglose de tareas por rol en el flujo SDD (`tasks.<rol>.md`) — planificado para el INC-03.
- Modificación destructiva de la suite previa de `gentle-ai` (ambos binarios pueden coexistir pacíficamente durante la transición).

---

## Capacidades (Capabilities)

### Nuevas Capacidades
- `axiom-cli-identity`: Punto de entrada CLI oficial `cmd/axiom/main.go` con soporte para banderas y subcomandos iniciales.
- `workspace-topology-engine`: Lógica desacoplada en `internal/workspace` capaz de evaluar la estructura de directorios, roles y repositorios declarados contra las reglas de gobierno de Axiom.

### Capacidades Modificadas
- Ninguna. No se altera el comportamiento de los paquetes existentes de Gentle-AI en este incremento.

---

## Enfoque de Implementación (Approach)

1. **Definición del modelo de datos (`internal/workspace/types.go`):**
   - Declaración de constantes de topología: `TopologyMonorepoEmbedded`, `TopologyMonorepoDecoupled`, `TopologyMultirepo`.
   - Estructuras con tags YAML (`gopkg.in/yaml.v3`) para mapear el esquema acordado de `axiom.yaml`.
2. **Carga y validación sintáctica (`internal/workspace/loader.go`):**
   - Función `LoadConfig(path string) (*WorkspaceConfig, error)` con manejo de errores descriptivos en español.
3. **Motor de validación de topología (`internal/workspace/validator.go`):**
   - Función `ValidateTopology(fs FileSystem, cfg *WorkspaceConfig, workspaceRoot string) (*ValidationReport, error)`.
   - Abstracción de filesystem o rutas relativas para facilitar pruebas unitarias reproducibles en memoria sin tocar disco real.
4. **Punto de entrada CLI (`cmd/axiom/main.go`):**
   - Estructuración limpia de comandos con `internal/app` o flags estándar de Go.
5. **Pruebas y Verificación:**
   - Pruebas unitarias en Go con cobertura de caminos críticos (tabla de escenarios).
   - Verificación de compilación: `go build -o axiom.exe ./cmd/axiom`.

---

## Áreas Afectadas (Affected Areas)

| Área / Archivo | Impacto | Descripción |
| :--- | :--- | :--- |
| `cmd/axiom/main.go` | Nuevo | Punto de entrada del binario ejecutable `axiom` |
| `internal/workspace/types.go` | Nuevo | Tipos y estructuras del esquema `axiom.yaml` |
| `internal/workspace/loader.go` | Nuevo | Carga y deserialización de configuración |
| `internal/workspace/validator.go` | Nuevo | Reglas de topología y verificación de repositorios |
| `internal/workspace/validator_test.go` | Nuevo | Suite de pruebas unitarias para todas las topologías |
| `axiom.example.yaml` | Nuevo | Archivo de referencia documentado del esquema |

---

## Riesgos y Mitigaciones (Risks)

- **Riesgo:** Incompatibilidad de rutas en Windows vs Linux (separadores de directorio `\` vs `/`).  
  *Mitigación:* Usar estrictamente `filepath.Clean` y `filepath.Join` del paquete estándar de Go para normalizar rutas relativas a la carpeta maestra.
- **Riesgo:** Confusión si el comando se ejecuta fuera de la raíz del workspace.  
  *Mitigación:* Implementar detección ascendente buscando `axiom.yaml` en directorios padres si no se especifica `--path`.
