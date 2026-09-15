# Tareas: Desacoplamiento Visual, Eliminación del Logo Gentle AI en OpenCode y Neutralización de Temas (INC-09)

## Fase 1: Supresión de Gentle Logo en OpenCode (`internal/model/`, `internal/components/`)

- [x] T-01 Modificar `internal/model/presets.go`: Retirar `ComponentOpenCodeGentleLogo` de `installSafePresetVisualComponents()` para que los presets de instalación no instalen el logo por defecto.
- [x] T-02 Actualizar `internal/components/uninstall/service.go`: Asegurar que el desinstalador y sincronizador purguen `gentle-logo.tsx` y retiren su referencia de `tui.json`.
- [x] T-03 Actualizar tests de presets e instalación en `internal/model/presets_test.go`, `internal/tui/model_test.go` e `internal/cli/install_test.go`.

## Fase 2: Neutralización de Temas y Renombrado Axiom (`internal/components/theme/`)

- [x] T-04 Modificar `internal/components/theme/inject.go`: Neutralizar `Inject()` para no forzar `"theme": "gentleman"` en el `settings.json` de OpenCode y Claude Code durante la instalación y sincronización.
- [x] T-05 Actualizar `internal/components/theme/inject.go`: Renombrar los archivos de temas opcionales en `VisualThemePaths()` a `axiom.json` y `axiom-dark.json`, actualizando los modelos de tema a la marca Axiom.
- [x] T-06 Actualizar tests en `internal/components/theme/inject_test.go` y referencias en `internal/cli/run_component_paths_test.go`.

## Fase 3: Alineación de Nomenclatura en Adaptadores (`internal/agents/`)

- [x] T-07 Actualizar `internal/agents/vscode/adapter.go`: Cambiar `SystemPromptFile` para apuntar a `axiom.instructions.md` y actualizar `adapter_test.go`.
- [x] T-08 Actualizar `internal/agents/kiro/adapter.go`: Cambiar `SystemPromptFile` para apuntar a `axiom.md` y actualizar `adapter_test.go`.
- [x] T-09 Actualizar `internal/agents/pi/review_routing.go`: Buscar modelos en `~/.pi/axiom/models.json` con soporte para `AXIOM_PI_CONFIG_HOME` y fallback de compatibilidad a `.pi/gentle-ai/`, actualizando tests.

## Fase 4: Identidad Visual del Repositorio y Cierre SDD

- [x] T-10 Actualizar `README.md`: Reemplazar referencias, enlaces y el banner neón de Gentle AI por la identidad oficial y sobria de Axiom.
- [x] T-11 Crear banner e imagen limpia de Axiom en `docs/assets/brand/`.
- [x] T-12 Ejecutar suite completa de tests en `internal/...` y compilar el binario `axiom`.
- [x] T-13 Generar reportes SDD `verify-report.md` y `archive-report.md` para completar y archivar INC-09.
