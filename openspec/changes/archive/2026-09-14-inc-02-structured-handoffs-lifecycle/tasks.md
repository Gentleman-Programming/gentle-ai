# Tareas: Handoffs Estructurados y Ciclo de Vida de Transición (INC-02)

## Fase 1: Dominio, Parser y Formateador (`internal/handoff`)

- [x] T-01 Crear `internal/handoff/types.go`: Definir constantes de fases SDD (`PhaseExplore`, `PhasePropose`, etc.), estados (`StatusReady`, `StatusBlocked`, `StatusNeedsClarification`), structs `Metadata`, `Sections` y `Handoff`.
- [x] T-02 Crear `internal/handoff/parser.go`: Implementar lectura de frontmatter YAML y extracción estructurada de las cinco secciones obligatorias en español con validación de sintaxis.
- [x] T-03 Crear `internal/handoff/writer.go`: Implementar formateador canónico de `handoff.md` con encabezado YAML delimitado por `---` y las cinco secciones formateadas.

## Fase 2: Motor de Validación Semántica y Espejo Engram (`internal/handoff`)

- [x] T-04 Crear `internal/handoff/validator.go`: Implementar reglas de validación semántica de completitud, máquina de estados de transiciones hacia adelante y retrocesos por remediación, y comprobación de roles contra `axiom.yaml`.
- [x] T-05 Crear `internal/handoff/mirror.go`: Implementar generador de payload para Engram MCP bajo `topic_key: sdd/{change}/handoff`.
- [x] T-06 Crear `internal/handoff/handoff_test.go`: Suite completa de pruebas unitarias cubriendo parsing round-trip, validaciones exitosas, rechazo de campos/secciones faltantes, transiciones ilegales y mapeo a Engram.

## Fase 3: Integración de Subcomandos CLI (`cmd/axiom`)

- [x] T-07 Ampliar `cmd/axiom/main.go`: Incorporar el grupo de comandos `axiom handoff` con los subcomandos:
  - `axiom handoff show [--change <nombre>] [--path <directorio>]`
  - `axiom handoff create --change <nombre> --from <fase> --to <fase> --from-role <rol> --to-role <rol> [--status <estado>] [--path <directorio>]`
  - `axiom handoff validate [--change <nombre>] [--path <directorio>]`
- [x] T-08 Manejar códigos de salida estándar (0 éxito, 1 error) y mensajes de diagnóstico en español.

## Fase 4: Verificación Integral y Ejecución

- [x] T-09 Ejecutar suite completa `go test -v ./internal/handoff/...` y `go test ./...` asegurando 100% de tests en verde.
- [x] T-10 Recompilar binario `axiom.exe` y verificar en terminal los subcomandos `axiom handoff create`, `show` y `validate`.

