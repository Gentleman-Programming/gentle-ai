# Documento de Diseño Técnico: Identidad de Axiom y Topología de Workspace (INC-01)

## Resumen del Diseño

Este diseño detalla la arquitectura de software e interfaces en Go para materializar el punto de entrada oficial `cmd/axiom` y el subsistema de validación de topología `internal/workspace`.

---

## 1. Arquitectura de Paquetes y Componentes

```text
axiom/
├── cmd/
│   └── axiom/
│       └── main.go                  # Punto de entrada de la CLI de Axiom
├── internal/
│   └── workspace/
│       ├── types.go                 # Modelos de dominio y estructuras YAML
│       ├── loader.go                # Lectura y deserialización segura de axiom.yaml
│       ├── validator.go             # Reglas semánticas de topología y verificación
│       └── validator_test.go        # Suite de pruebas unitarias exhaustiva
└── axiom.example.yaml               # Plantilla de referencia para proyectos
```

---

## 2. Modelo de Dominio (`internal/workspace/types.go`)

### Tipos y Constantes
```go
package workspace

// TopologyType define las 3 topologías admitidas en Axiom
type TopologyType string

const (
    TopologyMonorepoEmbedded  TopologyType = "monorepo-embedded"
    TopologyMonorepoDecoupled TopologyType = "monorepo-decoupled"
    TopologyMultirepo         TopologyType = "multirepo"
)

// WorkspaceSection encapsula los metadatos y topología del workspace
type WorkspaceSection struct {
    Name            string       `yaml:"name"`
    Topology        TopologyType `yaml:"topology"`
    SpecsRepository string       `yaml:"specs_repository"`
    Root            string       `yaml:"root,omitempty"`
}

// RepositoryEntry representa una carpeta o repositorio Git local
type RepositoryEntry struct {
    Path string `yaml:"path"`
}

// RoleConfig define las responsabilidades y carpetas asignadas a un rol
type RoleConfig struct {
    Name         string            `yaml:"name"`
    Repositories []RepositoryEntry `yaml:"repositories"`
    Tech         []string          `yaml:"tech,omitempty"`
}

// GovernanceConfig establece los parámetros transversales de gobierno
type GovernanceConfig struct {
    Language         string `yaml:"language,omitempty"`
    SharedMemory     string `yaml:"shared_memory,omitempty"`
    SemanticAnalysis string `yaml:"semantic_analysis,omitempty"`
}

// WorkspaceConfig es la raíz del archivo axiom.yaml
type WorkspaceConfig struct {
    Workspace  WorkspaceSection      `yaml:"workspace"`
    Roles      map[string]RoleConfig `yaml:"roles"`
    Governance GovernanceConfig      `yaml:"governance,omitempty"`
}

// ValidationReport resume el resultado de la validación
type ValidationReport struct {
    Valid         bool         `json:"valid"`
    Topology      TopologyType `json:"topology"`
    WorkspaceRoot string       `json:"workspace_root"`
    CheckedPaths  []string     `json:"checked_paths"`
    Errors        []string     `json:"errors"`
    Warnings      []string     `json:"warnings"`
}
```

---

## 3. Abstracción del Sistema de Archivos y Carga (`loader.go` y `validator.go`)

Para permitir pruebas unitarias 100% deterministas en Go sin depender de la creación física de carpetas en disco, se define la interfaz:

```go
type FS interface {
    Stat(name string) (os.FileInfo, error)
    IsDir(path string) (bool, error)
}
```

### Reglas de Validación Semántica (`Validate`)
1. **Validación de la Configuración:**
   - `Workspace.Name` no puede estar vacío.
   - `Workspace.Topology` debe pertenecer al conjunto `{monorepo-embedded, monorepo-decoupled, multirepo}`.
   - Debe existir al menos un rol en `Roles`.
2. **Validación de la Carpeta Maestra:**
   - La ruta base del workspace debe existir y ser un directorio válido.
3. **Validación del Repositorio Canónico de Especificaciones (`SpecsRepository`):**
   - Si la topología es `monorepo-embedded`: `SpecsRepository` puede ser `"."` o una subcarpeta válida del monorrepo.
   - Si la topología es `monorepo-decoupled` o `multirepo`: `SpecsRepository` es **estrictamente obligatorio**. Debe apuntar a un subdirectorio existente dentro de la carpeta maestra común. Si no existe, se añade un error de rechazo.
4. **Validación de Repositorios por Rol:**
   - Cada repositorio declarado en `Roles[r].Repositories` debe existir localmente dentro de la carpeta maestra. Si alguna ruta no existe, se reporta el error específico asociándolo al rol afectado.

---

## 4. Punto de Entrada CLI (`cmd/axiom/main.go`)

El punto de entrada procesa los argumentos de línea de comandos mediante un despachador limpio:

```text
axiom [flags] <command> [subcommand] [arguments]

Comandos:
  version, --version, -v      Muestra versión, commit y runtime
  workspace validate          Valida el archivo axiom.yaml y la topología
  help, --help, -h            Muestra la ayuda de comandos
```

### Formato de Salida de `axiom workspace validate`:
- Si la validación es exitosa:
  ```text
  [OK] Espacio de trabajo Axiom válido (Topología: multirepo)
  - Nombre: PlataformaEnterprise
  - Directorio base: C:\repos\workspace
  - Repositorio canónico de specs: C:\repos\workspace\especificacion (Encontrado)
  - Roles validados: 2 (frontend, backend)
  - Repositorios comprobados: 3/3 operativos
  Resultado: COMPLIANT
  ```
- Si hay errores:
  ```text
  [ERROR] Espacio de trabajo no conforme:
  - Error: El repositorio canónico de especificaciones 'especificacion' no existe en la carpeta maestra.
  - Error: El repositorio 'backend-core' declarado en el rol 'backend' no fue encontrado.
  Resultado: NON-COMPLIANT
  ```
- Código de salida: `0` para éxito, `1` para error.

---

## 5. Estrategia de Pruebas (TDD)

1. **`internal/workspace/loader_test.go`:**
   - Test deserialización YAML con campos válidos.
   - Test fallo con YAML malformado o campos faltantes.
2. **`internal/workspace/validator_test.go`:**
   - Test monorepo-embedded conforme.
   - Test monorepo-decoupled con repo de specs presente vs ausente.
   - Test multirepo con todos los repositorios presentes (éxito).
   - Test multirepo sin repo de specs (fallo obligatorio).
   - Test multirepo con un repo de rol faltante (fallo específico).
3. **Compilación integral:**
   - `go build ./cmd/axiom` generando el binario ejecutable `axiom.exe`.
   - Ejecución de prueba con `axiom --version`.
