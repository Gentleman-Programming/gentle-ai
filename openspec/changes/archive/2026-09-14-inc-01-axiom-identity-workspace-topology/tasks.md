# Tareas: Identidad de Axiom y Topología de Workspace (INC-01)

## Fase 1: Modelo de Dominio y Cargador de Configuración (`internal/workspace`)

- [x] T-01 Crear `internal/workspace/types.go`: Definir constantes de topología (`TopologyMonorepoEmbedded`, `TopologyMonorepoDecoupled`, `TopologyMultirepo`) y structs YAML (`WorkspaceConfig`, `WorkspaceSection`, `RoleConfig`, `RepositoryEntry`, `GovernanceConfig`, `ValidationReport`).
- [x] T-02 Crear `internal/workspace/loader.go`: Implementar `LoadConfig(filePath string)` y `ParseConfig(data []byte)` con validación de sintaxis y mensajes de error en español.
- [x] T-03 Escribir `internal/workspace/loader_test.go`: Pruebas unitarias guiadas por tabla verificando deserialización correcta de YAML válido y rechazo de documentos truncados o malformados.

## Fase 2: Motor de Validación de Topología (`internal/workspace`)

- [x] T-04 Crear `internal/workspace/validator.go`: Definir interfaz `FS` (abstracción del sistema de archivos) y función `Validate(fs FS, cfg *WorkspaceConfig, baseDir string)` evaluando:
  - Existencia y validez de la carpeta maestra.
  - Topología `monorepo-embedded` (specs en `.` o subcarpeta interna).
  - Topología `monorepo-decoupled` (specs obligatorio en carpeta independiente).
  - Topología `multirepo` (specs obligatorio y verificación de existencia de todos los repos de roles).
- [x] T-05 Escribir `internal/workspace/validator_test.go`: Suite completa de pruebas con mock de `FS` cubriendo:
  - Monorepo embebido válido.
  - Monorepo desacoplado con/sin repositorio de specs.
  - Multirepo válido con múltiples roles y repositorios.
  - Multirepo sin repositorio canónico de especificaciones (rechazo obligatorio).
  - Multirepo con repositorio de rol inexistente en la carpeta maestra.

## Fase 3: Punto de Entrada CLI y Plantilla de Referencia (`cmd/axiom`)

- [x] T-06 Crear `cmd/axiom/main.go`: Configurar despachador CLI con comandos de versión (`axiom --version`, `axiom -v`, `axiom version`) y menú de ayuda (`axiom --help`).
- [x] T-07 Implementar subcomando `axiom workspace validate`: Soporte de bandera `--path`, reporte formateado legible en consola (COMPLIANT / NON-COMPLIANT) y códigos de salida `0` / `1`.
- [x] T-08 Crear `axiom.example.yaml`: Plantilla canónica de referencia en la raíz del repositorio con comentarios explicativos en español.

## Fase 4: Verificación Integral y Ejecución

- [x] T-09 Ejecutar suite de pruebas unitarias `go test -v ./internal/workspace/...` asegurando que todos los escenarios de la especificación pasan en verde.
- [x] T-10 Compilar binario nativo `go build -o axiom.exe ./cmd/axiom` y verificar ejecución real en terminal (`axiom --version` y `axiom workspace validate`).
