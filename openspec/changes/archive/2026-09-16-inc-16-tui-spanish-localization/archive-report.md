# Reporte de Archivado: Localización Integral en Castellano de la TUI y Desacoplamiento de Advisories (INC-16)

**Fecha:** 2026-09-16  
**Incremento:** `inc-16-tui-spanish-localization`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El **Incremento 16 (INC-16: `inc-16-tui-spanish-localization`)** resuelve dos deudas históricas del fork respecto a la identidad y localización de Axiom:

1. **Desacoplamiento Definitivo de Comunicados y Advisories Upstream (`internal/update/`):**
   - Se redirigió `advisoryURL` en `internal/update/advisory.go` para apuntar a `https://github.com/IGutierrezZ/axiom/releases/download/advisory/advisory.json`.
   - Se eliminaron las dos líneas de cabecera de la TUI que mostraban alertas de Gentle AI v3.0.0 (`Latest release:` y `Advisory:`), manteniendo una apertura limpia con el banner propio de Axiom y la versión local del sistema en modo fail-open.

2. **Localización Integral al Castellano del Menú Principal (`internal/tui/screens/welcome.go`):**
   - Traducidas las 14/15 opciones oficiales de `WelcomeOptions`: "Iniciar instalación", "Actualizar herramientas", "Sincronizar configuraciones", "Actualizar y sincronizar", "Configurar modelos", "Crear agente personalizado", "Plugins comunitarios de OpenCode", "Desinstalar plugin de OpenCode", "Perfiles SDD de OpenCode", "Gestionar respaldos", "Reiniciar almacén de revisiones", "Revisión formal RDD", "Desinstalación gestionada", "Herramientas y plugins comunitarios", "Salir".
   - Encabezado `Menú` y barra de ayuda inferior: `j/k: navegar • enter: seleccionar • q: salir`.
   - Acciones atómicas y modos responsive: `Ir` y `q` para terminales ultraestrechos.
   - Microtextos de scroll y prefijos de avisos en español (`RePág/AvPág: desplazar`, `Aviso: `, `Última versión: `).

3. **Localización de Submenús y Pantallas de Configuración:**
   - **`screens/model_config.go`:** "Configuración de Modelos", "Configurar modelos de Claude", "Configurar modelos de OpenCode", "Configurar modelos de Kiro", "Configurar modelos de Codex", "Volver".
   - **`screens/backups.go`:** "Gestión de Respaldos", "Restaurar", "Cancelar", "Eliminar", "Volver", "Renombrar Respaldo", scroll "↑ más" / "↓ más".
   - **`screens/detection.go`:** "Detección del Sistema", "Continuar", "Volver", etiquetas de sistema en español ("SO", "Intérprete", "Compatible", "Sí/No", "encontrada/no encontrada").
   - **`screens/preset.go`:** "Seleccionar Preset del Ecosistema", botón "Volver".
   - **`screens/persona.go`:** "Elige tu Persona", subtítulo de Axiom y botón "Volver".
   - **`screens/common.go`:** Indicador de espera: "Por favor, espera...".

4. **Preservación Determinista de la Máquina de Estados Bubbletea:**
   - La navegación y activación de pantallas subordinadas en `internal/tui/model.go` opera exclusivamente por índices numéricos del cursor, asegurando total invarianza funcional tras la traducción.
   - Se actualizaron las suites de prueba (`screens/welcome_test.go`, `screens/welcome_internal_test.go`, `screens/model_config_test.go`, `screens/backups_test.go`, `internal/tui/model_test.go`, `internal/tui/review_store_reset_test.go`) con 100% de éxito.

---

## Artefactos Consolidados y Modificados

- **Desacoplamiento de Avisos:**
  - `internal/update/advisory.go`
- **TUI y Pantallas Bubbletea:**
  - `internal/tui/screens/welcome.go`
  - `internal/tui/screens/model_config.go`
  - `internal/tui/screens/backups.go`
  - `internal/tui/screens/detection.go`
  - `internal/tui/screens/preset.go`
  - `internal/tui/screens/persona.go`
  - `internal/tui/screens/common.go`
- **Suites de Pruebas Unitarias:**
  - `internal/tui/screens/welcome_test.go`
  - `internal/tui/screens/welcome_internal_test.go`
  - `internal/tui/screens/model_config_test.go`
  - `internal/tui/screens/backups_test.go`
  - `internal/tui/model_test.go`
  - `internal/tui/review_store_reset_test.go`
- **Ciclo SDD de INC-16:**
  - `openspec/changes/inc-16-tui-spanish-localization/proposal.md`
  - `openspec/changes/inc-16-tui-spanish-localization/spec.md`
  - `openspec/changes/inc-16-tui-spanish-localization/design.md`
  - `openspec/changes/inc-16-tui-spanish-localization/tasks.md`
  - `openspec/changes/inc-16-tui-spanish-localization/verify-report.md`
  - `openspec/changes/inc-16-tui-spanish-localization/archive-report.md`
- **Especificaciones Vivas:**
  - `openspec/specs/tui-spanish-localization/spec.md`
