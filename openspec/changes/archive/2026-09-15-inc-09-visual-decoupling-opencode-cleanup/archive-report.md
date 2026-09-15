# Reporte de Archivado: Desacoplamiento Visual, Eliminación del Logo Gentle AI en OpenCode y Neutralización de Temas (INC-09)

**Fecha:** 2026-09-15  
**Incremento:** `inc-09-visual-decoupling-opencode-cleanup`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El Incremento 09 (INC-09) culmina la independencia visual y estética de Axiom respecto a Gentle-AI:
1. **Supresión del Logo Invasivo:** OpenCode ya no recibe el plugin `gentle-logo.tsx` en sus presets estándar, dejando la pantalla de inicio limpia para el usuario.
2. **Respeto a los Temas del Desarrollador:** Se eliminó la inyección forzada de `"theme": "gentleman"` en `settings.json`. Los temas opcionales se renombraron a `axiom.json` y `axiom-dark.json` con paletas modernas y sobrias.
3. **Estandarización de Rutas de Instrucciones:** Los adaptadores de VS Code Copilot (`axiom.instructions.md`), Kiro IDE (`axiom.md`) y Pi (`.pi/axiom/`) adoptaron la nomenclatura canónica de Axiom.
4. **Identidad del Repositorio:** Se rediseñó el `README.md` y se añadió el banner oficial `docs/assets/brand/axiom-banner.svg`, retirando todo rastro de iconografía no deseada.

---

## Artefactos Consolidados y Modificados

- **Código de Producción:**
  - `internal/model/presets.go`
  - `internal/components/uninstall/service.go`
  - `internal/components/theme/inject.go`
  - `internal/agents/vscode/adapter.go`
  - `internal/agents/kiro/adapter.go`
  - `internal/agents/pi/review_routing.go`
  - `README.md`
  - `docs/assets/brand/axiom-banner.svg`
- **Suites de Pruebas:**
  - `internal/model/presets_test.go`
  - `internal/components/uninstall/cleaners_test.go`
  - `internal/components/theme/inject_test.go`
  - `internal/agents/vscode/adapter_test.go`
  - `internal/agents/kiro/adapter_test.go`
  - `internal/agents/pi/review_routing_test.go`
  - `internal/tui/model_test.go`
  - `internal/cli/install_test.go`
  - `internal/cli/run_component_paths_test.go`
  - `internal/cli/sync_test.go`
- **Binario Oficial:** Recompilado e instalado globalmente como `axiom.exe`.
