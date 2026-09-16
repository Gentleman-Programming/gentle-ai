# Tareas de Implementación: Unificación de TUI Bubbletea, Comandos de Ecosistema en CLI axiom y Pasarela de gentle-ai (INC-14)

## Fase 1: Identidad Visual y Estilos de la TUI

- [x] T-01 Rediseñar `internal/tui/styles/logo.go` implementando el arte ASCII tipográfico de AXIOM con gradiente de 5 bandas.
- [x] T-02 Actualizar `internal/tui/styles/styles.go` con el lema institucional de Axiom en `Tagline()`.
- [x] T-03 Ajustar microtextos y encabezados en `internal/tui/screens/welcome.go`.
- [x] T-04 Validar pruebas existentes de estilos y renderizado en `internal/tui/...`.

## Fase 2: Enrutamiento de Comandos y TUI en `cmd/axiom/main.go`

- [x] T-05 Implementar detección de TTY para la invocación sin argumentos en `cmd/axiom/main.go`, lanzando la TUI interactiva o mostrando la ayuda según corresponda.
- [x] T-06 Conectar el subcomando explícito `axiom tui` para lanzar la TUI interactiva.
- [x] T-07 Integrar enrutamiento a `internal/app` para los comandos `install`, `sync`, `upgrade`, `doctor`, `backup`, `restore` y `uninstall`.
- [x] T-08 Actualizar la función `printHelp()` en `cmd/axiom/main.go` documentando la TUI y los nuevos comandos del ecosistema.

## Fase 3: Pasarela de Compatibilidad y Pruebas Unitarias

- [x] T-09 Convertir `cmd/gentle-ai/main.go` en un wrapper con aviso de deprecación hacia `axiom` en `stderr`.
- [x] T-10 Crear/actualizar pruebas unitarias en `cmd/axiom/main_test.go` verificando el enrutamiento de comandos y ayuda.
- [x] T-11 Crear pruebas en `cmd/gentle-ai/main_test.go` verificando la emisión del aviso de deprecación y la ejecución delegada.
- [x] T-12 Ejecutar la batería completa de pruebas unitarias (`go test ./cmd/... ./internal/tui/...`).

## Fase 4: Verificación Formal SDD, Documentación Viva y Cierre

- [x] T-13 Redactar `verify-report.md` y validar conformidad con `axiom sdd-verify-validate`.
- [x] T-14 Redactar `archive-report.md`, archivar formalmente el cambio en `openspec/changes/archive/` y consolidar la especificación viva `openspec/specs/axiom-tui-branding/spec.md`.
- [x] T-15 Sincronizar el catálogo maestro de especificaciones vivas con `axiom archive sync`.
